package portal

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// 复用 arkfgf.com 的登录登录玩家站（协议见 ArkPlayerPortal/docs/api.md 的「登录」）：
//  1. GET /api/v1/identity?nonce=… 让玩家站证明它持有身份私钥，audience 必须是公钥的 SHA-256
//  2. 用 arkfgf.com 的登录换一个只对这个 audience 有效的短期令牌（调用方传入 issue）
//  3. POST /api/v1/login 换玩家站的会话；之后 GET /api/v1/me 带 Authorization: Bearer <会话>
// 会话只在内存里，按玩家站地址存

// identityContext 和玩家站签名的前缀一致
const identityContext = "arkfgf-portal-identity\n"

// ErrNotLoggedIn 助手还没有登录 arkfgf.com
var ErrNotLoggedIn = errors.New("还没有登录")

// IssueFunc 用 arkfgf.com 的登录换给 audience 的令牌
type IssueFunc func(ctx context.Context, audience string) (string, error)

type identityProof struct {
	PublicKey string `json:"public_key"`
	Audience  string `json:"audience"`
	Signature string `json:"signature"`
}

// verifyIdentity 让玩家站对随机 nonce 签名，校验签名和 audience，返回 audience
func (c *Client) verifyIdentity(ctx context.Context, base string) (string, error) {
	var b [24]byte
	_, _ = rand.Read(b[:])
	nonce := base64.RawURLEncoding.EncodeToString(b[:])
	body, status, err := c.send(ctx, http.MethodGet, base, "/api/v1/identity?nonce="+nonce, "", nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", portalError(body, status)
	}
	var p identityProof
	if json.Unmarshal(body, &p) != nil {
		return "", errors.New("玩家站的身份证明格式不对")
	}
	pub, err := base64.RawURLEncoding.DecodeString(p.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return "", errors.New("玩家站的身份证明格式不对")
	}
	sig, err := base64.RawURLEncoding.DecodeString(p.Signature)
	if err != nil || !ed25519.Verify(pub, []byte(identityContext+nonce), sig) {
		return "", errors.New("玩家站的身份证明无效")
	}
	sum := sha256.Sum256(pub)
	if p.Audience != hex.EncodeToString(sum[:]) {
		return "", errors.New("玩家站的身份证明无效")
	}
	return p.Audience, nil
}

// login 向玩家站换会话
func (c *Client) login(ctx context.Context, base string, issue IssueFunc) (string, error) {
	audience, err := c.verifyIdentity(ctx, base)
	if err != nil {
		return "", err
	}
	token, err := issue(ctx, audience)
	if err != nil {
		return "", err
	}
	payload, _ := json.Marshal(map[string]string{"token": token})
	body, status, err := c.send(ctx, http.MethodPost, base, "/api/v1/login", "", payload)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", portalError(body, status)
	}
	var out struct {
		Session string `json:"session"`
	}
	if json.Unmarshal(body, &out) != nil || out.Session == "" {
		return "", errors.New("玩家站返回的数据无法解析")
	}
	return out.Session, nil
}

// Me 当前玩家在这个玩家站的数据（/api/v1/me 的 JSON 原文）。issue 为 nil 时（助手没登录）不登录，
// 返回的 user 为 null。会话无效时重新登录一次
func (c *Client) Me(ctx context.Context, rawBase string, issue IssueFunc) (string, error) {
	base, err := NormalizeBase(rawBase)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	session := c.sessions[base]
	c.mu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if session == "" && issue != nil {
			if session, err = c.login(ctx, base, issue); err != nil {
				return "", err
			}
			c.mu.Lock()
			c.sessions[base] = session
			c.mu.Unlock()
		}
		body, status, err := c.send(ctx, http.MethodGet, base, "/api/v1/me", session, nil)
		if err != nil {
			return "", err
		}
		if status != http.StatusOK || !json.Valid(body) {
			return "", portalError(body, status)
		}
		var me struct {
			User json.RawMessage `json:"user"`
		}
		_ = json.Unmarshal(body, &me)
		loggedOut := len(me.User) == 0 || string(me.User) == "null"
		if loggedOut && session != "" && issue != nil && attempt == 0 {
			// 会话失效（玩家站换了身份密钥、过期）：重新登录
			session = ""
			continue
		}
		return string(body), nil
	}
	return "", errors.New("登录玩家站失败")
}

// Forget 忘掉所有会话（退出 arkfgf.com 登录时）
func (c *Client) Forget() {
	c.mu.Lock()
	clear(c.sessions)
	c.mu.Unlock()
}

// send 不带缓存的请求，返回状态码和内容
func (c *Client) send(ctx context.Context, method, base, path, session string, payload []byte) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var rd io.Reader
	if payload != nil {
		rd = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, rd)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if session != "" {
		req.Header.Set("Authorization", "Bearer "+session)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("连不上玩家站：%w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, 0, fmt.Errorf("读取玩家站失败：%w", err)
	}
	if len(body) > maxBody {
		return nil, 0, errors.New("玩家站返回的内容太大")
	}
	return body, resp.StatusCode, nil
}

func portalError(body []byte, status int) error {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &e)
	if e.Error != "" && len(e.Error) < 200 {
		return errors.New(e.Error)
	}
	return fmt.Errorf("玩家站返回了错误（%d）", status)
}
