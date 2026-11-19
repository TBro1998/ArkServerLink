package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"arkfgf.com/link/internal/arkfgf"
)

// Source 是一条下载线路：加速站名称和完整下载地址。
type Source struct {
	Name string
	URL  string
}

// Sources 按选择排出下载线路。choice 是 Prefs.Mirror：空串表示自动，返回全部线路，由 Rank 测速排序；
// 指定了线路时它排第一，其余作为失败后的备用。
func Sources(assetURL string, mirrors []arkfgf.Mirror, choice string) []Source {
	var first, rest []Source
	for _, m := range mirrors {
		s := Source{Name: m.Name, URL: m.Prefix + assetURL}
		if choice != "" && (m.Prefix == choice || (choice == MirrorDirect && m.Prefix == "")) {
			first = append(first, s)
		} else {
			rest = append(rest, s)
		}
	}
	return append(first, rest...)
}

// probeBytes 测速时每条线路下载的字节数。
const probeBytes = 256 << 10

// Rank 同时向每条线路请求文件开头的 256 KB，按速度从快到慢排序；失败的排在最后。
// 加速站偶尔返回错误页面，最终以 sha256 校验为准，这里只用来挑线路。
func Rank(ctx context.Context, client *http.Client, sources []Source, timeout time.Duration) []Source {
	type result struct {
		src   Source
		speed float64 // 字节/秒，失败为 0
	}
	results := make([]result, len(sources))
	var wg sync.WaitGroup
	for i, s := range sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = result{src: s, speed: probe(ctx, client, s.URL, timeout)}
		}()
	}
	wg.Wait()
	sort.SliceStable(results, func(a, b int) bool { return results[a].speed > results[b].speed })
	out := make([]Source, len(results))
	for i, r := range results {
		out[i] = r.src
	}
	return out
}

func probe(ctx context.Context, client *http.Client, url string, timeout time.Duration) float64 {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", probeBytes-1))
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0
	}
	// 加速站出错时常返回 HTML 页面
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		return 0
	}
	n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, probeBytes))
	if n == 0 {
		return 0
	}
	return float64(n) / max(time.Since(start).Seconds(), 0.001)
}

// Progress 是下载进度。
type Progress struct {
	Source string // 当前线路名称
	Done   int64
	Total  int64
}

// stallTimeout 连续这么久没有收到数据就换下一条线路。
const stallTimeout = 20 * time.Second

// ErrChecksum 表示下载的文件和发布的 sha256 不一致。
var ErrChecksum = errors.New("文件校验失败")

// Download 依次尝试各条线路下载 asset 到 dir，校验大小和 sha256 后返回文件路径。
// 一条线路失败（连接失败、停滞、校验不通过）就换下一条，全部失败时返回最后的错误。
func Download(ctx context.Context, client *http.Client, asset arkfgf.ReleaseAsset, sources []Source, dir string, onProgress func(Progress)) (string, error) {
	want := strings.ToLower(asset.SHA256)
	if len(want) != 64 {
		return "", errors.New("这个版本没有提供 sha256，无法校验，请到官网手动下载")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, filepath.Base(asset.Name))
	// 之前下载过且校验通过就直接用
	if sum, err := fileSHA256(dest); err == nil && sum == want {
		return dest, nil
	}
	if len(sources) == 0 {
		return "", errors.New("没有可用的下载线路")
	}
	var lastErr error
	for _, s := range sources {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		err := downloadOnce(ctx, client, s, asset.Size, want, dest, onProgress)
		if err == nil {
			return dest, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		lastErr = fmt.Errorf("%s: %w", s.Name, err)
	}
	return "", lastErr
}

func downloadOnce(ctx context.Context, client *http.Client, s Source, size int64, want, dest string, onProgress func(Progress)) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	total := size
	if total <= 0 {
		total = resp.ContentLength
	}

	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer os.Remove(tmp) // 成功时已改名，删除失败无影响

	// 停滞检测：每收到数据就把计时器往后推
	errStalled := errors.New("下载停滞")
	stall := time.AfterFunc(stallTimeout, func() { cancel(errStalled) })
	defer stall.Stop()

	h := sha256.New()
	buf := make([]byte, 64<<10)
	var done int64
	var lastReport time.Time
	report := func(force bool) {
		if onProgress != nil && (force || time.Since(lastReport) >= 200*time.Millisecond) {
			lastReport = time.Now()
			onProgress(Progress{Source: s.Name, Done: done, Total: total})
		}
	}
	report(true)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			stall.Reset(stallTimeout)
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				return err
			}
			h.Write(buf[:n])
			done += int64(n)
			if size > 0 && done > size {
				f.Close()
				return fmt.Errorf("文件大小不对（超过 %d 字节）", size)
			}
			report(false)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			if c := context.Cause(ctx); c != nil && errors.Is(c, errStalled) {
				return c
			}
			return rerr
		}
	}
	report(true)
	if err := f.Close(); err != nil {
		return err
	}
	if size > 0 && done != size {
		return fmt.Errorf("文件不完整（%d / %d 字节）", done, size)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return ErrChecksum
	}
	return os.Rename(tmp, dest)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
