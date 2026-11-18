package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"arkfgf.com/link/internal/arkfgf"
	"arkfgf.com/link/internal/auth"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// AuthState 是登录状态。
type AuthState struct {
	LoggedIn bool         `json:"logged_in"`
	User     *arkfgf.User `json:"user"`
	Message  string       `json:"message"` // 例如暂时连不上网站
}

// AuthService 暴露给前端：Steam 登录、退出、查询当前用户。
type AuthService struct {
	api   *arkfgf.Client
	store *auth.Store
	app   func() *application.App

	mu    sync.Mutex
	token string
	user  *arkfgf.User

	loginMu     sync.Mutex
	loginCancel context.CancelFunc
}

const loginTimeout = 5 * time.Minute

func (s *AuthService) ServiceName() string { return "AuthService" }

func (s *AuthService) ServiceStartup(context.Context, application.ServiceOptions) error {
	if s.store == nil {
		return nil
	}
	tok, err := s.store.Load()
	if err != nil {
		return nil // 读不出来就当未登录
	}
	s.mu.Lock()
	s.token = tok
	s.mu.Unlock()
	return nil
}

// Status 返回登录状态；有令牌时向网站确认，令牌失效则清除。
func (s *AuthService) Status(ctx context.Context) AuthState {
	s.mu.Lock()
	tok, user := s.token, s.user
	s.mu.Unlock()
	if tok == "" {
		return AuthState{}
	}
	claims, _ := auth.ParseClaims(tok)
	if !claims.ExpiresAt.IsZero() && time.Now().After(claims.ExpiresAt) {
		s.clear(tok)
		return AuthState{Message: "登录已过期，请重新登录"}
	}
	if user != nil {
		return AuthState{LoggedIn: true, User: user}
	}
	u, err := s.api.Me(ctx, tok)
	switch {
	case err == nil:
		s.mu.Lock()
		if s.token == tok {
			s.user = u
		}
		s.mu.Unlock()
		return AuthState{LoggedIn: true, User: u}
	case arkfgf.IsUnauthorized(err):
		s.clear(tok)
		return AuthState{Message: "登录已过期，请重新登录"}
	default:
		// 网络问题：保留登录，先用令牌里的名字
		return AuthState{LoggedIn: true, User: &arkfgf.User{Name: claims.Name}, Message: err.Error()}
	}
}

// Login 打开浏览器走 Steam 登录，等待网站回调到本机（最多 5 分钟）。
func (s *AuthService) Login(ctx context.Context) (AuthState, error) {
	s.loginMu.Lock()
	if s.loginCancel != nil {
		s.loginCancel()
	}
	ctx, cancel := context.WithTimeout(ctx, loginTimeout)
	s.loginCancel = cancel
	s.loginMu.Unlock()
	defer cancel()

	tok, err := auth.LoopbackLogin(ctx, s.api.SteamLoginURL, s.openURL)
	if err != nil {
		return AuthState{}, err
	}
	if err := s.store.Save(tok); err != nil {
		// 保存失败只影响下次启动，本次仍然算登录成功
		application.Get().Logger.Warn("保存登录令牌失败", "error", err)
	}
	s.mu.Lock()
	s.token, s.user = tok, nil
	s.mu.Unlock()

	st := s.Status(context.Background())
	if !st.LoggedIn {
		return st, errors.New(st.Message)
	}
	s.emit(st)
	focusMainWindow()
	return st, nil
}

// CancelLogin 取消正在等待的登录。
func (s *AuthService) CancelLogin() {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.loginCancel != nil {
		s.loginCancel()
		s.loginCancel = nil
	}
}

// Logout 退出登录（只清除本机令牌，不影响正在连接的房间）。
func (s *AuthService) Logout() error {
	s.mu.Lock()
	tok := s.token
	s.mu.Unlock()
	return s.clear(tok)
}

// currentToken 给加入房间用（未导出，不暴露给前端）。
func (s *AuthService) currentToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// unauthorized 在网站返回 401 时调用。
func (s *AuthService) unauthorized() {
	s.clear(s.currentToken())
}

// clear 清除令牌；令牌已被替换（例如刚重新登录）时不动。
func (s *AuthService) clear(tok string) error {
	s.mu.Lock()
	if s.token != tok {
		s.mu.Unlock()
		return nil
	}
	s.token, s.user = "", nil
	s.mu.Unlock()
	err := s.store.Clear()
	s.emit(AuthState{})
	return err
}

func (s *AuthService) emit(st AuthState) {
	if app := application.Get(); app != nil {
		app.Event.Emit(EventAuthChanged, st)
	}
}

func (s *AuthService) openURL(url string) error {
	return application.Get().Browser.OpenURL(url)
}
