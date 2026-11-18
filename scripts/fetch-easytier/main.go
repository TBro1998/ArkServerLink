// fetch-easytier 下载固定版本的 EasyTier 到 bin/easytier，校验 sha256 后只解压客户端需要的文件。
//
// 用法（在项目根目录）: go run ./scripts/fetch-easytier
// 升级 EasyTier：改 version 和 assets 里的 sha256（GitHub Release 页面的 digest），
// 再按 README 重新验证一遍组网参数。
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
)

const version = "v2.6.4"

type asset struct {
	name   string
	sha256 string
	files  []string // 只解压这些文件；no-tun 模式不需要 wintun.dll 和 WinDivert 驱动
}

var assets = map[string]asset{
	"windows/amd64": {
		name:   "easytier-windows-x86_64-" + version + ".zip",
		sha256: "27af91e270e554709b048bd32327fefd2dfce5062ae1e8701af7550c6f525f84",
		files:  []string{"easytier-core.exe", "easytier-cli.exe", "Packet.dll"},
	},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fetch-easytier:", err)
		os.Exit(1)
	}
}

func run() error {
	key := runtime.GOOS + "/" + runtime.GOARCH
	if v := os.Getenv("TARGET"); v != "" {
		key = v
	}
	a, ok := assets[key]
	if !ok {
		return fmt.Errorf("暂不支持平台 %s", key)
	}

	outDir := filepath.Join("bin", "easytier")
	stamp := filepath.Join(outDir, ".version")
	if b, err := os.ReadFile(stamp); err == nil && string(b) == a.name {
		fmt.Println("已是", version, "，跳过下载")
		return nil
	}

	u := "https://github.com/EasyTier/EasyTier/releases/download/" + version + "/" + a.name
	fmt.Println("下载", u)
	data, err := download(u)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != a.sha256 {
		return fmt.Errorf("sha256 不匹配: 期望 %s，实际 %s", a.sha256, got)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	want := map[string]bool{}
	for _, f := range a.files {
		want[f] = true
	}
	for _, f := range zr.File {
		base := path.Base(f.Name)
		if !want[base] {
			continue
		}
		if err := extract(f, filepath.Join(outDir, base)); err != nil {
			return err
		}
		delete(want, base)
		fmt.Println("  ", base)
	}
	if len(want) > 0 {
		return fmt.Errorf("压缩包里缺少文件: %v", want)
	}
	return os.WriteFile(stamp, []byte(a.name), 0o644)
}

func download(u string) ([]byte, error) {
	client := &http.Client{Transport: &http.Transport{Proxy: proxyFunc()}}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载失败: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// proxyFunc 优先用 HTTPS_PROXY 等环境变量，没设置时用 Windows 系统代理。
func proxyFunc() func(*http.Request) (*url.URL, error) {
	if os.Getenv("HTTPS_PROXY") != "" || os.Getenv("https_proxy") != "" {
		return http.ProxyFromEnvironment
	}
	if p := systemProxy(); p != nil {
		fmt.Println("使用系统代理", p)
		return http.ProxyURL(p)
	}
	return http.ProxyFromEnvironment
}

func extract(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
