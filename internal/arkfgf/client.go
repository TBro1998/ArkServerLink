// Package arkfgf 是 arkfgf.com 联机房接口（/api/v1/link，约定见 docs/p2p-link.md）的客户端。
package arkfgf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL 是线上接口地址，可用环境变量 ARKFGF_API 覆盖（本地联调用）。
const DefaultBaseURL = "https://arkfgf.com/api/v1"

// BaseURLFromEnv 返回 ARKFGF_API 或默认地址，去掉末尾的斜杠。
func BaseURLFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("ARKFGF_API")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return DefaultBaseURL
}

const maxBody = 4 << 20

// Client 调用网站接口。需要登录的接口由调用方传入 JWT。
type Client struct {
	base      string
	userAgent string
	http      *http.Client
}

// New 创建客户端。version 用在 User-Agent 里。
func New(base, version string) *Client {
	return &Client{
		base:      strings.TrimRight(base, "/"),
		userAgent: "ArkServerLink/" + version,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

// BaseURL 返回接口根地址，例如 https://arkfgf.com/api/v1。
func (c *Client) BaseURL() string { return c.base }

// ListRooms 列出在线房间。page 从 1 开始，0 表示第一页。
func (c *Client) ListRooms(ctx context.Context, q, mapName string, page int) (*RoomList, error) {
	v := url.Values{}
	if q = strings.TrimSpace(q); q != "" {
		v.Set("q", q)
	}
	if mapName != "" {
		v.Set("map", mapName)
	}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	path := "/link/rooms"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out RoomList
	if err := c.do(ctx, http.MethodGet, path, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetRoom 返回房间详情，离线房间也能查到。
func (c *Client) GetRoom(ctx context.Context, id int64) (*RoomDetail, error) {
	var out RoomDetail
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/link/rooms/%d", id), "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Join 申请加入房间，返回入网参数。
func (c *Client) Join(ctx context.Context, token string, id int64) (*JoinResult, error) {
	if token == "" {
		return nil, &APIError{Status: http.StatusUnauthorized, Code: CodeUnauthorized, Message: messages[CodeUnauthorized]}
	}
	var out JoinResult
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/link/rooms/%d/join", id), token, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Me 返回当前登录用户。
func (c *Client) Me(ctx context.Context, token string) (*User, error) {
	var out User
	if err := c.do(ctx, http.MethodGet, "/me", token, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SteamLoginURL 返回客户端回环登录的入口地址。
func (c *Client) SteamLoginURL(clientPort int, state string) string {
	v := url.Values{"client_port": {strconv.Itoa(clientPort)}, "state": {state}}
	return c.base + "/auth/steam/login?" + v.Encode()
}

func (c *Client) do(ctx context.Context, method, path, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &NetworkError{Err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return &NetworkError{Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return parseError(resp, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("arkfgf.com 返回的数据无法解析: %w", err)
	}
	return nil
}

// 错误码，与网站约定一致；unauthorized、rate_limited 由客户端按状态码补上。
const (
	CodeUnauthorized  = "unauthorized"
	CodeNotFound      = "not_found"
	CodeRoomOffline   = "room_offline"
	CodeBanned        = "banned"
	CodeNoCredentials = "no_credentials"
	CodeRateLimited   = "rate_limited"
)

var messages = map[string]string{
	CodeUnauthorized:  "登录已过期，请重新用 Steam 登录",
	CodeNotFound:      "房间不存在或已下架",
	CodeRoomOffline:   "房间当前离线（服务器没有运行），请稍后再试",
	CodeBanned:        "你已被该房间的服主封禁",
	CodeNoCredentials: "房间的入场名额暂时用完了，正在等服主补充",
	CodeRateLimited:   "操作太频繁，请稍后再试",
}

// APIError 是网站返回的错误。Message 是可以直接给玩家看的中文说明。
type APIError struct {
	Status     int
	Code       string
	Message    string
	RetryAfter time.Duration // 响应带 Retry-After 时有值
}

func (e *APIError) Error() string { return e.Message }

// NetworkError 表示没连上网站（断网、DNS、超时等）。
type NetworkError struct{ Err error }

func (e *NetworkError) Error() string {
	return "无法连接 arkfgf.com，请检查网络: " + e.Err.Error()
}
func (e *NetworkError) Unwrap() error { return e.Err }

// IsUnauthorized 判断错误是否表示登录失效。
func IsUnauthorized(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusUnauthorized
}

// ErrorCode 返回错误码，非接口错误返回空。
func ErrorCode(err error) string {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

func parseError(resp *http.Response, body []byte) *APIError {
	var payload struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	_ = json.Unmarshal(body, &payload)
	e := &APIError{Status: resp.StatusCode, Code: payload.Code, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	if e.Code == "" {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			e.Code = CodeUnauthorized
		case http.StatusNotFound:
			e.Code = CodeNotFound
		case http.StatusTooManyRequests:
			e.Code = CodeRateLimited
		}
	}
	// 401 一律当作登录失效：网站现有的鉴权中间件只返回英文 error、不带 code
	if resp.StatusCode == http.StatusUnauthorized {
		e.Code = CodeUnauthorized
	}
	switch {
	case messages[e.Code] != "":
		e.Message = messages[e.Code]
	case payload.Error != "":
		e.Message = payload.Error
	case resp.StatusCode >= 500:
		e.Message = fmt.Sprintf("arkfgf.com 暂时不可用（%d），请稍后再试", resp.StatusCode)
	default:
		e.Message = fmt.Sprintf("请求失败（%d）", resp.StatusCode)
	}
	if e.Code == CodeRateLimited && e.RetryAfter > 0 {
		e.Message = fmt.Sprintf("操作太频繁，请 %d 秒后再试", int(e.RetryAfter.Round(time.Second).Seconds()))
	}
	return e
}

func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
