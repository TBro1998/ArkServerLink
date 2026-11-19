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

// enterMu 串行化 EnterGame，避免连点重复拉起一个游戏进程。
var enterMu sync.Mutex

// EnterGame 让 Steam 启动 ARK 并直接连进当前房间（等同游戏内控制台的 open 命令）。
// Steam 只接受游戏端口的地址。
func (s *RoomService) EnterGame() error {
	st := s.manager.Status()
	if st.Phase != session.PhaseConnected || st.LocalPort <= 0 {
		return errors.New("还没有和服务器直连，暂时无法进服")
	}
	enterMu.Lock()
	defer enterMu.Unlock()
	// 游戏已经开着时 Steam 会忽略连接参数，把命令放进剪贴板，让玩家自己粘贴
	if steam.ArkRunning() {
		application.Get().Clipboard.SetText(st.Command)
		return errors.New("ARK 已经在运行，Steam 没法把进服参数交给它。进服命令已经复制，在游戏里按 Tab 打开控制台粘贴回车即可。")
	}
	addr := fmt.Sprintf("127.0.0.1:%d", st.LocalPort)
	if err := application.Get().Browser.OpenURL(steam.ConnectURL(addr)); err != nil {
		return fmt.Errorf("无法启动 Steam: %w", err)
	}
	return nil
}
