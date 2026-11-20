package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"arkfgf.com/link/internal/arkfgf"
	"arkfgf.com/link/internal/easytier"
	"arkfgf.com/link/internal/session"
	"arkfgf.com/link/internal/steam"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// RoomService 暴露给前端：浏览房间、加入/退出、连接状态和日志。
type RoomService struct {
	api  *arkfgf.Client
	auth *AuthService

	runner  *easytier.Runner
	manager *session.Manager

	mu          sync.Mutex
	pendingJoin int64 // 通过 arkfgf-link:// 链接请求加入、前端还没取走的房间
}

func (s *RoomService) ServiceName() string { return "RoomService" }

func (s *RoomService) ServiceStartup(context.Context, application.ServiceOptions) error {
	opt := session.Options{
		API:            s.api,
		Token:          s.auth.currentToken,
		OnUnauthorized: s.auth.unauthorized,
	}
	if dir, err := easytier.FindDir(); err != nil {
		// 不阻止程序启动，加入时再把错误告诉玩家
		opt.RunnerErr = err
	} else {
		s.runner = easytier.NewRunner(dir)
		opt.Runner = s.runner
	}
	s.manager = session.NewManager(opt)
	return nil
}

func (s *RoomService) ServiceShutdown() error {
	if s.manager != nil {
		s.manager.Leave()
	}
	if s.runner != nil {
		return s.runner.Stop()
	}
	return nil
}

// List 列出在线房间。
func (s *RoomService) List(ctx context.Context, q string, page int) (*arkfgf.RoomList, error) {
	return s.api.ListRooms(ctx, q, "", page)
}

// Get 返回房间详情。
func (s *RoomService) Get(ctx context.Context, id int64) (*arkfgf.RoomDetail, error) {
	return s.api.GetRoom(ctx, id)
}

// Join 开始加入房间，进度通过 Status 查询。正在连别的房间时会先断开。
func (s *RoomService) Join(id int64, name string) error {
	return s.manager.Join(id, name)
}

// Leave 断开当前房间。
func (s *RoomService) Leave() {
	s.manager.Leave()
}

// Status 返回当前连接状态。
func (s *RoomService) Status() session.Status {
	return s.manager.Status()
}

// Logs 返回 easytier-core 最近的输出，用于排查问题。
func (s *RoomService) Logs() []string {
	if s.runner == nil {
		return nil
	}
	return s.runner.Logs()
}

// TakePendingJoin 取走链接请求加入的房间 ID（没有时为 0）。
// 前端启动时和收到 deeplink:join 事件时调用。
func (s *RoomService) TakePendingJoin() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.pendingJoin
	s.pendingJoin = 0
	return id
}

func (s *RoomService) requestJoin(id int64) {
	s.mu.Lock()
	s.pendingJoin = id
	s.mu.Unlock()
	if app := application.Get(); app != nil {
		app.Event.Emit(EventDeepLinkJoin, id)
	}
}

// EnterGame 让 Steam 连接当前房间里的一台服务器（steam://connect/127.0.0.1:<查询端口>）。
//
// 和玩家在 Steam 收藏里添加 127.0.0.1:<查询端口> 再连接是同一个流程（已实测能进服）。
// 以前直接启动 ShooterGame.exe 带 -ExecCmds="open ..." 进不了服。
func (s *RoomService) EnterGame(key string) error {
	st := s.manager.Status()
	if st.Phase != session.PhaseConnected {
		return errors.New("还没有和服务器直连，暂时无法进服")
	}
	// 房间里可能有多台服务器，key 是 Commander 里的服务器 ID；留空时选第一台
	var target *session.ServerStatus
	for i := range st.Servers {
		if key == "" || st.Servers[i].Key == key {
			target = &st.Servers[i]
			break
		}
	}
	if target == nil || target.Address == "" {
		return errors.New("找不到这台服务器的本机端口")
	}
	if err := application.Get().Browser.OpenURL(steam.ConnectURL(target.Address)); err != nil {
		return fmt.Errorf("无法打开 Steam: %w", err)
	}
	return nil
}
