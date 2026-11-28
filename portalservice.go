package main

import (
	"context"
	"errors"
	"sync"

	"arkfgf.com/link/internal/arkfgf"
	"arkfgf.com/link/internal/portal"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// PortalService 暴露给前端：读取服主的玩家站（公告、服务器状态、商店）。
// 请求在 Go 里发出，不把服主的网页加载进助手的窗口（窗口里能调用助手的方法）。
// 返回的是玩家站接口的 JSON 原文，前端按 ArkPlayerPortal/docs/snapshot.md 解析，只当纯文本显示。
type PortalService struct {
	client *portal.Client
	api    *arkfgf.Client
	auth   *AuthService

	mu      sync.Mutex
	pending string // 通过 arkfgf-link://portal 链接请求打开、前端还没取走的玩家站地址
}

func (s *PortalService) ServiceName() string { return "PortalService" }

// Portal 站点资料、公告和服务器
func (s *PortalService) Portal(ctx context.Context, base string) (string, error) {
	return s.client.Portal(ctx, base)
}

// Shop 一台服务器的商店；没有商店时返回空字符串
func (s *PortalService) Shop(ctx context.Context, base, key string) (string, error) {
	body, err := s.client.Shop(ctx, base, key)
	if errors.Is(err, portal.ErrNoShop) {
		return "", nil
	}
	return body, err
}

// Me 当前玩家在这个玩家站上的数据（JSON 原文）。助手登录了 arkfgf.com 时自动用它登录玩家站，
// 没登录时返回的 user 为 null
func (s *PortalService) Me(ctx context.Context, base string) (string, error) {
	var issue portal.IssueFunc
	if token := s.auth.currentToken(); token != "" {
		issue = func(ctx context.Context, audience string) (string, error) {
			res, err := s.api.PortalToken(ctx, token, audience)
			if err != nil {
				if arkfgf.IsUnauthorized(err) {
					s.auth.unauthorized()
				}
				return "", err
			}
			return res.Token, nil
		}
	} else {
		s.client.Forget()
	}
	return s.client.Me(ctx, base, issue)
}

// Normalize 校验玩家粘贴的玩家站地址，返回规范化后的地址
func (s *PortalService) Normalize(raw string) (string, error) {
	return portal.NormalizeBase(raw)
}

// OpenInBrowser 在系统浏览器里打开玩家站（只接受 http(s)）
func (s *PortalService) OpenInBrowser(raw string) error {
	base, err := portal.NormalizeBase(raw)
	if err != nil {
		return err
	}
	return application.Get().Browser.OpenURL(base)
}

// OpenLink 在系统浏览器里打开玩家站里的链接（站点链接、Discord、Mod 页面），只接受 http(s)
func (s *PortalService) OpenLink(raw string) error {
	if _, err := portal.NormalizeBase(raw); err != nil {
		return err
	}
	return application.Get().Browser.OpenURL(raw)
}

// TakePending 取走链接请求打开的玩家站地址（没有时为空）。前端启动时和收到 deeplink:portal 事件时调用
func (s *PortalService) TakePending() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.pending
	s.pending = ""
	return u
}

func (s *PortalService) requestOpen(u string) {
	s.mu.Lock()
	s.pending = u
	s.mu.Unlock()
	if app := application.Get(); app != nil {
		app.Event.Emit(EventDeepLinkPortal, u)
	}
}
