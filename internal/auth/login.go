// Package auth 实现客户端的 Steam 登录（本机回环回调）和登录令牌的本地保存。
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strings"
	"time"
)

// ErrCanceled 表示玩家取消了登录或等待超时。
var ErrCanceled = errors.New("登录已取消或超时，请重试")

// NewState 生成回调用的随机 state（43 位 base64url，符合约定的 16-64 位 [A-Za-z0-9_-]）。
func NewState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// LoopbackLogin 在 127.0.0.1 随机端口等待网站回调：
// loginURL(port, state) 生成网站登录入口，open 打开系统浏览器，
// 网站完成 Steam 登录后跳转到 http://127.0.0.1:<port>/callback?token=<jwt>&state=<state>。
// state 不匹配的请求一律忽略，避免别的网页伪造回调把玩家登录成别人的账号。
// 超时由 ctx 控制。
func LoopbackLogin(ctx context.Context, loginURL func(port int, state string) string, open func(url string) error) (string, error) {
	state, err := NewState()
	if err != nil {
		return "", err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("无法监听本机端口: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	type result struct {
		token string
		err   error
	}
	results := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 {
			writePage(w, http.StatusBadRequest, false, "登录链接无效或已过期，请回到联机助手重新点击登录。")
			return
		}
		token := strings.TrimSpace(q.Get("token"))
		if token == "" {
			msg := "网站没有返回登录凭据"
			if e := q.Get("error"); e != "" {
				msg = "登录失败：" + e
			}
			writePage(w, http.StatusBadRequest, false, msg+"。请回到联机助手重试。")
			select {
			case results <- result{err: errors.New(msg)}:
			default:
			}
			return
		}
		select {
		case results <- result{token: token}:
			writePage(w, http.StatusOK, true, "登录成功，可以关闭这个页面，回到 ARKFGF 联机助手。")
		default:
			// 已经处理过一次回调（例如浏览器刷新）
			writePage(w, http.StatusOK, true, "已登录，可以关闭这个页面。")
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer func() {
		// 给浏览器留一点时间收完成功页
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := open(loginURL(port, state)); err != nil {
		return "", fmt.Errorf("无法打开浏览器: %w", err)
	}

	select {
	case res := <-results:
		return res.token, res.err
	case <-ctx.Done():
		return "", ErrCanceled
	}
}

var pageTmpl = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>ARKFGF 联机助手</title>
<style>
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#030712;color:#e5e7eb;font-family:"Segoe UI","Microsoft YaHei UI",system-ui,sans-serif}
.card{max-width:420px;padding:36px 40px;border:1px solid #1f2937;border-radius:14px;background:#0b1120;text-align:center}
.mark{width:44px;height:44px;margin:0 auto 18px;border-radius:10px;display:flex;align-items:center;justify-content:center;font-weight:900;font-size:22px;color:#030712;background:linear-gradient(135deg,#f97316,#fbbf24)}
.bad .mark{background:#7f1d1d;color:#fecaca}
h1{font-size:18px;margin:0 0 10px}p{margin:0;color:#9ca3af;line-height:1.6}
</style></head>
<body class="{{if not .OK}}bad{{end}}"><div class="card"><div class="mark">{{if .OK}}✓{{else}}!{{end}}</div>
<h1>{{if .OK}}登录成功{{else}}登录未完成{{end}}</h1><p>{{.Message}}</p></div></body></html>`))

func writePage(w http.ResponseWriter, status int, ok bool, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	pageTmpl.Execute(w, struct {
		OK      bool
		Message string
	}{ok, msg})
}

// Claims 是从 JWT 里读出的、不需要验证签名的展示信息。
type Claims struct {
	Name      string
	ExpiresAt time.Time // 零值表示没有 exp
}

// ParseClaims 解析 JWT 的载荷（不校验签名，只用于离线时展示名字和判断是否过期）。
func ParseClaims(token string) (Claims, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return Claims{}, false
	}
	var v struct {
		Name string  `json:"name"`
		Exp  float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &v); err != nil {
		return Claims{}, false
	}
	c := Claims{Name: v.Name}
	if v.Exp > 0 {
		c.ExpiresAt = time.Unix(int64(v.Exp), 0)
	}
	return c, true
}
