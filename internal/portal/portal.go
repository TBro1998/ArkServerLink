// Package portal 读取服主的玩家站（ArkPlayerPortal）的只读接口：站点资料、公告、服务器、商店。
// 接口见 ArkPlayerPortal/docs/api.md。内容来自服主的机器，当作不可信的输入：限制超时和大小，
// 只接受 JSON，界面上只当纯文本显示。
package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	maxBody        = 4 << 20
	requestTimeout = 10 * time.Second
)

var keyPattern = regexp.MustCompile(`^[0-9]{1,10}$`)

// ErrNoShop 这台服务器没有商店
var ErrNoShop = errors.New("这台服务器没有商店")

// Client 带 ETag 缓存：内容没变时玩家站返回 304，直接用上一次的内容
type Client struct {
	http      *http.Client
	userAgent string

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	etag string
	body string
}

// New version 用在 User-Agent 里
func New(version string) *Client {
	return &Client{
		// 不跟随跳转到别的主机：玩家站不会跳转，跟随了可能被引到别处
		http: &http.Client{
			Timeout: requestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		userAgent: "ArkServerLink/" + version,
		cache:     map[string]cached{},
	}
}

// NormalizeBase 校验并规范化玩家站地址：只接受 http(s)，去掉末尾的斜杠、查询参数和片段
func NormalizeBase(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || len(raw) > 512 {
		return "", errors.New("玩家站地址要以 http:// 或 https:// 开头")
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}

// Portal 返回 /api/v1/portal 的 JSON 原文
func (c *Client) Portal(ctx context.Context, base string) (string, error) {
	return c.get(ctx, base, "/api/v1/portal")
}

// Shop 返回 /api/v1/shops/<key> 的 JSON 原文
func (c *Client) Shop(ctx context.Context, base, key string) (string, error) {
	if !keyPattern.MatchString(key) {
		return "", ErrNoShop
	}
	return c.get(ctx, base, "/api/v1/shops/"+key)
}

func (c *Client) get(ctx context.Context, rawBase, path string) (string, error) {
	base, err := NormalizeBase(rawBase)
	if err != nil {
		return "", err
	}
	full := base + path
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	c.mu.Lock()
	prev, hasPrev := c.cache[full]
	c.mu.Unlock()
	if hasPrev && prev.etag != "" {
		req.Header.Set("If-None-Match", prev.etag)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("连不上玩家站：%w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return "", fmt.Errorf("读取玩家站失败：%w", err)
	}
	if len(body) > maxBody {
		return "", errors.New("玩家站返回的内容太大")
	}
	switch {
	case resp.StatusCode == http.StatusNotModified && hasPrev:
		return prev.body, nil
	case resp.StatusCode == http.StatusOK:
		if !json.Valid(body) {
			return "", errors.New("玩家站返回的不是有效的数据")
		}
		c.mu.Lock()
		if len(c.cache) > 64 {
			clear(c.cache)
		}
		c.cache[full] = cached{etag: resp.Header.Get("ETag"), body: string(body)}
		c.mu.Unlock()
		return string(body), nil
	}
	var e struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	_ = json.Unmarshal(body, &e)
	if e.Code == "shop_not_found" {
		return "", ErrNoShop
	}
	if e.Error != "" && len(e.Error) < 200 {
		return "", errors.New(e.Error)
	}
	return "", fmt.Errorf("玩家站返回了错误（%d）", resp.StatusCode)
}
