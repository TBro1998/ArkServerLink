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

// EnterGame 直接启动本机的 ARK 客户端并连进当前房间。
//
// 用 -ExecCmds 让游戏启动后执行控制台的 open 命令，不经过 Steam 的启动器：
// Steam 的 +connect / steam://run 走它的服务器查询流程，对 127.0.0.1 会报
// "Unable to query server info for invite"，不会真的发出游戏报文（已实测）。
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
	if target == nil || target.LocalPort <= 0 {
		return errors.New("找不到这台服务器的本机端口")
	}
	enterMu.Lock()
	defer enterMu.Unlock()
	// 游戏已经开着时新进程会退出，把命令放进剪贴板让玩家自己粘贴
	if steam.ArkRunning() {
		application.Get().Clipboard.SetText(target.Command)
		return errors.New("ARK 已经在运行，没法再启动一个。进服命令已经复制，在游戏里按 Tab 打开控制台粘贴回车即可。")
	}
	exe, err := steam.FindARK()
	if err != nil {
		return err
	}
	cmd := steam.ARKCommand(exe, target.LocalPort)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法启动 ARK: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}
