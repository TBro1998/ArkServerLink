// Package deeplink 解析网站「用联机助手加入」按钮使用的 arkfgf-link:// 链接。
package deeplink

import (
	"net/url"
	"strconv"
	"strings"
)

// Scheme 是安装包注册的自定义协议。
const Scheme = "arkfgf-link"

// ParseJoin 解析 arkfgf-link://join/<房间 ID>，返回房间 ID；不是合法的加入链接时返回 false。
// 浏览器可能在末尾加斜杠，也可能把整个链接再加一层引号。
func ParseJoin(raw string) (int64, bool) {
	raw = strings.Trim(strings.TrimSpace(raw), `"'`)
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, Scheme) {
		return 0, false
	}
	// arkfgf-link://join/12 → Host=join Path=/12；arkfgf-link:join/12 → Opaque=join/12
	path := u.Opaque
	if path == "" {
		path = u.Host + u.Path
	}
	action, id, ok := strings.Cut(strings.Trim(path, "/"), "/")
	if !ok || !strings.EqualFold(action, "join") {
		return 0, false
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// FindJoin 在命令行参数里找加入链接（Windows 上协议唤起时链接是第一个参数）。
func FindJoin(args []string) (int64, bool) {
	for _, a := range args {
		if id, ok := ParseJoin(a); ok {
			return id, true
		}
	}
	return 0, false
}

// ParsePortal 解析 arkfgf-link://portal?url=<玩家站地址>（网站服务器详情页「用联机助手打开」），
// 返回玩家站地址；不是合法的链接或地址不是 http(s) 时返回 false
func ParsePortal(raw string) (string, bool) {
	raw = strings.Trim(strings.TrimSpace(raw), `"'`)
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, Scheme) {
		return "", false
	}
	path := u.Opaque
	if path == "" {
		path = u.Host + u.Path
	}
	if before, _, _ := strings.Cut(path, "?"); !strings.EqualFold(strings.Trim(before, "/"), "portal") {
		return "", false
	}
	target := u.Query().Get("url")
	t, err := url.Parse(target)
	if err != nil || (t.Scheme != "http" && t.Scheme != "https") || t.Host == "" || t.User != nil || len(target) > 512 {
		return "", false
	}
	return t.String(), true
}

// FindPortal 在命令行参数里找打开玩家站的链接
func FindPortal(args []string) (string, bool) {
	for _, a := range args {
		if u, ok := ParsePortal(a); ok {
			return u, true
		}
	}
	return "", false
}
