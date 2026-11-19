package arkfgf

import (
	"context"
	"net/http"
	"time"
)

// 联机助手的版本信息（GET /link/client/releases），网站从 GitHub Release 缓存而来，
// 下载页和客户端自动更新共用。字段约定见 docs/p2p-link.md「联机助手发布」。

// 安装包种类，按文件名后缀区分（见 .github/workflows/release.yml）。
const (
	AssetInstaller = "installer" // ArkServerLink-<版本>-windows-x64-setup.exe
	AssetPortable  = "portable"  // ArkServerLink-<版本>-windows-x64-portable.zip
	AssetChecksums = "checksums" // SHA256SUMS.txt
	AssetOther     = "other"
)

// ReleaseAsset 是 Release 里的一个文件。URL 是 GitHub 原始下载地址，加速站把前缀拼在前面。
type ReleaseAsset struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
}

// ClientRelease 是一个版本。
type ClientRelease struct {
	Version     string         `json:"version"` // 不带 v，例如 0.2.0
	Tag         string         `json:"tag"`
	Name        string         `json:"name"`
	PublishedAt time.Time      `json:"published_at"`
	Notes       string         `json:"notes"` // Markdown
	HTMLURL     string         `json:"html_url"`
	Prerelease  bool           `json:"prerelease"`
	Assets      []ReleaseAsset `json:"assets"`
}

// Asset 返回指定种类的第一个文件。
func (r *ClientRelease) Asset(kind string) *ReleaseAsset {
	if r == nil {
		return nil
	}
	for i := range r.Assets {
		if r.Assets[i].Kind == kind {
			return &r.Assets[i]
		}
	}
	return nil
}

// Mirror 是 GitHub 下载加速站。下载地址 = Prefix + GitHub 原始地址；Prefix 为空表示直连。
type Mirror struct {
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

// ClientReleases 是 /link/client/releases 的响应。
type ClientReleases struct {
	Repo        string         `json:"repo"`
	ReleasesURL string         `json:"releases_url"`
	Stable      *ClientRelease `json:"stable"`
	Prerelease  *ClientRelease `json:"prerelease"` // 只有比稳定版新时才有
	Mirrors     []Mirror       `json:"mirrors"`
}

// ClientReleases 查询联机助手的最新版本。
func (c *Client) ClientReleases(ctx context.Context) (*ClientReleases, error) {
	var out ClientReleases
	if err := c.do(ctx, http.MethodGet, "/link/client/releases", "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}
