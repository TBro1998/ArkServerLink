package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"arkfgf.com/link/internal/arkfgf"
)

// Repo 是发布联机助手的 GitHub 仓库。
const Repo = "TBro1998/ArkServerLink"

// DefaultMirrors 是网站接口不可用时使用的加速站，与网站后端的默认列表一致。
var DefaultMirrors = []arkfgf.Mirror{
	{Name: "GitHub 直连", Prefix: ""},
	{Name: "ghfast.top", Prefix: "https://ghfast.top/"},
	{Name: "gh-proxy.org（香港）", Prefix: "https://hk.gh-proxy.org/"},
	{Name: "gh-proxy.org（CDN）", Prefix: "https://cdn.gh-proxy.org/"},
	{Name: "gh-proxy.org（EdgeOne）", Prefix: "https://edgeone.gh-proxy.org/"},
	{Name: "gh-proxy.org（IPv6）", Prefix: "https://v6.gh-proxy.org/"},
}

// notesCut 之后是发布工作流追加的下载说明和 GitHub 自动生成的更新列表，只在 GitHub 上显示。
const notesCut = "<!-- arkfgf:downloads -->"

// TrimNotes 截掉 Release 说明里只给 GitHub 页面看的部分（网站接口已经截过）。
func TrimNotes(body string) string {
	body, _, _ = strings.Cut(body, notesCut)
	return strings.TrimSpace(body)
}

// AssetKind 按文件名判断安装包种类。
func AssetKind(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasSuffix(n, "-setup.exe"):
		return arkfgf.AssetInstaller
	case strings.HasSuffix(n, "-portable.zip"):
		return arkfgf.AssetPortable
	case n == "sha256sums.txt":
		return arkfgf.AssetChecksums
	}
	return arkfgf.AssetOther
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		Size               int64  `json:"size"`
		Digest             string `json:"digest"` // "sha256:<hex>"
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (g ghRelease) convert() *arkfgf.ClientRelease {
	r := &arkfgf.ClientRelease{
		Version:     strings.TrimPrefix(g.TagName, "v"),
		Tag:         g.TagName,
		Name:        g.Name,
		PublishedAt: g.PublishedAt,
		Notes:       TrimNotes(g.Body),
		HTMLURL:     g.HTMLURL,
		Prerelease:  g.Prerelease,
	}
	for _, a := range g.Assets {
		r.Assets = append(r.Assets, arkfgf.ReleaseAsset{
			Name:   a.Name,
			Kind:   AssetKind(a.Name),
			Size:   a.Size,
			SHA256: strings.TrimPrefix(a.Digest, "sha256:"),
			URL:    a.BrowserDownloadURL,
		})
	}
	return r
}

// FetchGitHub 直接查询 GitHub 的最新正式版，网站接口不可用时的后备。
// 匿名调用每个 IP 每小时 60 次，只在后备时使用。
func FetchGitHub(ctx context.Context, client *http.Client, userAgent string) (*arkfgf.ClientReleases, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回 %d", resp.StatusCode)
	}
	var g ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&g); err != nil {
		return nil, err
	}
	return &arkfgf.ClientReleases{
		Repo:        Repo,
		ReleasesURL: "https://github.com/" + Repo + "/releases",
		Stable:      g.convert(),
		Mirrors:     DefaultMirrors,
	}, nil
}
