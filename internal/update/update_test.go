package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"arkfgf.com/link/internal/arkfgf"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"0.2.0", "0.1.0", true},
		{"v0.2.0", "0.1.9", true},
		{"0.1.0", "0.1.0", false},
		{"0.1.0", "0.2.0", false},
		{"1.0.0", "0.99.99", true},
		{"0.2.0", "0.2.0-beta.1", true},
		{"0.2.0-beta.2", "0.2.0-beta.1", true},
		{"0.2.0-beta.10", "0.2.0-beta.9", true},
		{"0.2.0-rc.1", "0.2.0-beta.9", true},
		{"0.2.0-beta.1", "0.2.0", false},
		{"0.2.0-beta", "0.2.0-beta.1", false},
		{"0.2.0+build5", "0.2.0", false},
		{"0.2.0", "dev", false},
		{"garbage", "0.1.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestAssetKind(t *testing.T) {
	cases := map[string]string{
		"ArkServerLink-0.2.0-windows-x64-setup.exe":    arkfgf.AssetInstaller,
		"ArkServerLink-0.2.0-windows-x64-portable.zip": arkfgf.AssetPortable,
		"SHA256SUMS.txt": arkfgf.AssetChecksums,
		"notes.md":       arkfgf.AssetOther,
	}
	for name, want := range cases {
		if got := AssetKind(name); got != want {
			t.Errorf("AssetKind(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestSources(t *testing.T) {
	mirrors := []arkfgf.Mirror{{Name: "直连", Prefix: ""}, {Name: "A", Prefix: "https://a/"}, {Name: "B", Prefix: "https://b/"}}
	names := func(ss []Source) string {
		var n []string
		for _, s := range ss {
			n = append(n, s.Name)
		}
		return strings.Join(n, ",")
	}
	if got := names(Sources("https://github.com/x", mirrors, "")); got != "直连,A,B" {
		t.Errorf("自动: %s", got)
	}
	if got := names(Sources("https://github.com/x", mirrors, "https://b/")); got != "B,直连,A" {
		t.Errorf("指定 B: %s", got)
	}
	if got := names(Sources("https://github.com/x", mirrors, MirrorDirect)); got != "直连,A,B" {
		t.Errorf("直连: %s", got)
	}
	if got := Sources("https://github.com/x", mirrors, "https://b/")[0].URL; got != "https://b/https://github.com/x" {
		t.Errorf("加速地址: %s", got)
	}
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestDownloadFallsBackAndVerifies(t *testing.T) {
	payload := []byte(strings.Repeat("ArkServerLink", 50000))
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer good.Close()
	// 加速站出错时返回 200 的错误页面，只能靠校验发现
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>rate limited</html>"))
	}))
	defer bad.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer down.Close()

	asset := arkfgf.ReleaseAsset{Name: "setup.exe", Size: int64(len(payload)), SHA256: sum(payload)}
	dir := t.TempDir()
	var last Progress
	path, err := Download(context.Background(), http.DefaultClient, asset,
		[]Source{{Name: "down", URL: down.URL}, {Name: "bad", URL: bad.URL}, {Name: "good", URL: good.URL}},
		dir, func(p Progress) { last = p })
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if sum(got) != asset.SHA256 {
		t.Fatal("内容不对")
	}
	if last.Source != "good" || last.Done != asset.Size {
		t.Errorf("进度 %+v", last)
	}
	if _, err := os.Stat(path + ".part"); !os.IsNotExist(err) {
		t.Error("临时文件没删除")
	}

	// 已下载并校验通过的文件直接复用，不再请求
	path2, err := Download(context.Background(), http.DefaultClient, asset, []Source{{Name: "down", URL: down.URL}}, dir, nil)
	if err != nil || path2 != path {
		t.Fatalf("复用失败: %v", err)
	}
}

func TestDownloadAllFail(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("tampered"))
	}))
	defer bad.Close()
	asset := arkfgf.ReleaseAsset{Name: "setup.exe", SHA256: sum([]byte("original"))}
	_, err := Download(context.Background(), http.DefaultClient, asset, []Source{{Name: "bad", URL: bad.URL}}, t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "校验") {
		t.Fatalf("应校验失败: %v", err)
	}
	asset.SHA256 = ""
	if _, err := Download(context.Background(), http.DefaultClient, asset, []Source{{Name: "bad", URL: bad.URL}}, t.TempDir(), nil); err == nil {
		t.Fatal("没有 sha256 时不应下载")
	}
}

func TestRank(t *testing.T) {
	data := make([]byte, probeBytes)
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		w.Write(data)
	}))
	defer fast.Close()
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Write(data)
	}))
	defer slow.Close()
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html>error</html>"))
	}))
	defer html.Close()

	got := Rank(context.Background(), http.DefaultClient,
		[]Source{{Name: "html", URL: html.URL}, {Name: "slow", URL: slow.URL}, {Name: "fast", URL: fast.URL}}, 5*time.Second)
	if got[0].Name != "fast" || got[1].Name != "slow" || got[2].Name != "html" {
		t.Errorf("排序 %v", got)
	}
}

func TestPrefs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "update.json")
	if p := LoadPrefs(path); p.Mirror != "" {
		t.Fatal("默认应为自动")
	}
	if err := SavePrefs(path, Prefs{Mirror: "https://a/"}); err != nil {
		t.Fatal(err)
	}
	if p := LoadPrefs(path); p.Mirror != "https://a/" {
		t.Fatalf("读回 %+v", p)
	}
}
