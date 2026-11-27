// Package session 管理「加入一个联机房」的完整过程：向网站申请凭证、选本机端口、
// 启动 easytier-core、监视和服务器的直连状态。同一时间只连一个房间。
package session

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"arkfgf.com/link/internal/arkfgf"
	"arkfgf.com/link/internal/easytier"
)

// API 是 session 用到的网站接口。
type API interface {
	Join(ctx context.Context, token string, id int64) (*arkfgf.JoinResult, error)
	GetRoom(ctx context.Context, id int64) (*arkfgf.RoomDetail, error)
}

// Runner 是 easytier.Runner 的接口，测试里用假实现。
type Runner interface {
	Start(cfg easytier.Config) error
	Stop() error
	Running() (bool, error)
	Peers(ctx context.Context) ([]easytier.Peer, error)
}

// Phase 是连接阶段。
type Phase string

const (
	PhaseIdle       Phase = "idle"
	PhaseJoining    Phase = "joining"    // 正在向网站申请（含凭证不足时的等待重试）
	PhaseConnecting Phase = "connecting" // easytier-core 已启动，还没和服务器直连
	PhaseConnected  Phase = "connected"  // 已和服务器 P2P 直连
	PhaseNotDirect  Phase = "not_direct" // 超过宽限期仍没直连（仍在后台重试）
	PhaseError      Phase = "error"      // 失败，已停止
)

// 客户端自己产生的错误码（网站的错误码见 arkfgf 包）。
const (
	CodeNotDirect         = "not_direct"
	CodeServerUnreachable = "server_unreachable"
	CodeProcessExited     = "process_exited"
	CodeLocal             = "local_error"
)

// ServerStatus 是房间里一台服务器在本机的转发情况：游戏里 open 127.0.0.1:<local_port>。
type ServerStatus struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Map  string `json:"map"`
	// ase 或 asa。ASA 服务器只能在游戏控制台里用 Command 进服，没有 RawPort、QueryPort 和 Address
	Game      arkfgf.Game `json:"game"`
	LocalPort int         `json:"local_port"`
	RawPort   int         `json:"raw_port"`
	QueryPort int         `json:"query_port"`
	Command   string      `json:"command"`
	// Steam 里连接用的地址（查询端口）：收藏里添加它，或 steam://connect/<address>；ASA 服务器为空
	Address string `json:"address"`
}

// steamAddress 返回 Steam 连接这台服务器用的本机地址。Steam 先查询这个端口，再按回复里的游戏端口进服。
// ASA 服务器不能通过 Steam 进服，返回空字符串。
func steamAddress(game arkfgf.Game, p Ports) string {
	if game == arkfgf.GameASA {
		return ""
	}
	port := p.Query
	if port <= 0 {
		port = p.Game
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}

// Status 是给界面看的连接状态。
type Status struct {
	Phase     Phase  `json:"phase"`
	RoomID    int64  `json:"room_id"`
	RoomName  string `json:"room_name"`
	LocalPort int    `json:"local_port"` // 游戏里 open 127.0.0.1:<local_port>
	RawPort   int    `json:"raw_port"`
	QueryPort int    `json:"query_port"`
	Command   string `json:"command"`
	// 房间里每台服务器各一组本机端口；上面的 local_port 等是第一台服务器的（旧界面兼容）
	Servers   []ServerStatus `json:"servers"`
	LatencyMs float64        `json:"latency_ms"`
	Message   string         `json:"message"`
	ErrorCode string         `json:"error_code"`
	RetryIn   int            `json:"retry_in"` // 等待重试的剩余秒数
	// 服主的玩家站在本机的地址（http://127.0.0.1:<端口>），房间没有开启玩家站时为空。直连后才能打开
	PortalURL string `json:"portal_url"`
}

// Options 配置 Manager。零值字段使用默认值。
type Options struct {
	API            API
	Runner         Runner
	RunnerErr      error         // Runner 为 nil 时加入返回这个错误（找不到 easytier-core）
	Token          func() string // 当前登录令牌
	OnUnauthorized func()        // 网站返回 401 时调用（清除登录状态）
	Grace          time.Duration // 启动后多久还没直连就判定为无法直连，默认 40 秒
	Poll           time.Duration // 读取 peer 表的间隔，默认 2 秒
	MaxRetries     int           // 凭证不足（503）时的自动重试次数，默认 3
	PickPorts      func(servers []ServerPorts) ([]Ports, error)
	// PickTCPPort 为玩家站选一个本机 TCP 端口（优先和服主一样），taken 是已经分给游戏转发的端口
	PickTCPPort func(preferred int, taken map[int]bool) (int, error)
}

// Manager 管理当前的联机会话。
type Manager struct {
	opt    Options
	joinMu sync.Mutex // 串行化 Join / Leave

	mu  sync.Mutex
	cur *session
}

type session struct {
	cancel  context.CancelFunc
	done    chan struct{}
	status  Status
	retryAt time.Time
}

func NewManager(opt Options) *Manager {
	if opt.PickTCPPort == nil {
		opt.PickTCPPort = PickTCPPort
	}
	if opt.Grace == 0 {
		opt.Grace = 40 * time.Second
	}
	if opt.Poll == 0 {
		opt.Poll = 2 * time.Second
	}
	if opt.MaxRetries == 0 {
		opt.MaxRetries = 3
	}
	if opt.PickPorts == nil {
		opt.PickPorts = PickAllPorts
	}
	if opt.Token == nil {
		opt.Token = func() string { return "" }
	}
	if opt.OnUnauthorized == nil {
		opt.OnUnauthorized = func() {}
	}
	return &Manager{opt: opt}
}

// Status 返回当前状态。
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cur == nil {
		return Status{Phase: PhaseIdle}
	}
	st := m.cur.status
	if !m.cur.retryAt.IsZero() {
		if d := time.Until(m.cur.retryAt); d > 0 {
			st.RetryIn = int(d.Seconds() + 0.999)
		}
	}
	return st
}

// Join 开始加入房间（异步，进度看 Status）。已经在这个房间里时不做任何事；
// 在别的房间里时先退出。name 只用于在网站返回前显示。
func (m *Manager) Join(id int64, name string) error {
	if m.opt.Runner == nil {
		if m.opt.RunnerErr != nil {
			return m.opt.RunnerErr
		}
		return errors.New("找不到 easytier-core")
	}
	if id <= 0 {
		return errors.New("房间 ID 无效")
	}
	m.joinMu.Lock()
	defer m.joinMu.Unlock()

	m.mu.Lock()
	cur := m.cur
	same := cur != nil && cur.status.RoomID == id && cur.status.Phase != PhaseError
	m.mu.Unlock()
	if same {
		return nil
	}
	m.leaveLocked()

	ctx, cancel := context.WithCancel(context.Background())
	s := &session{
		cancel: cancel,
		done:   make(chan struct{}),
		status: Status{Phase: PhaseJoining, RoomID: id, RoomName: name, Message: "正在向 arkfgf.com 申请加入…"},
	}
	m.mu.Lock()
	m.cur = s
	m.mu.Unlock()
	go m.run(ctx, s, id)
	return nil
}

// Leave 断开当前房间并等待 easytier-core 退出。
func (m *Manager) Leave() {
	m.joinMu.Lock()
	defer m.joinMu.Unlock()
	m.leaveLocked()
}

func (m *Manager) leaveLocked() {
	m.mu.Lock()
	s := m.cur
	m.cur = nil
	m.mu.Unlock()
	if s == nil {
		return
	}
	s.cancel()
	select {
	case <-s.done:
	case <-time.After(10 * time.Second):
	}
}

func (m *Manager) update(s *session, fn func(st *Status)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(&s.status)
}

func (m *Manager) fail(s *session, code, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.retryAt = time.Time{}
	s.status.Phase = PhaseError
	s.status.ErrorCode = code
	s.status.Message = msg
	s.status.LatencyMs = 0
}

func (m *Manager) run(ctx context.Context, s *session, id int64) {
	defer close(s.done)

	res, err := m.join(ctx, s, id)
	if err != nil {
		if ctx.Err() == nil {
			code := arkfgf.ErrorCode(err)
			if arkfgf.IsUnauthorized(err) {
				m.opt.OnUnauthorized()
			}
			m.fail(s, code, err.Error())
		}
		return
	}

	if len(res.Servers) == 0 {
		m.fail(s, CodeLocal, "房间里没有可连接的服务器")
		return
	}
	wanted := make([]ServerPorts, len(res.Servers))
	for i, sv := range res.Servers {
		if arkfgf.NormalizeGame(sv.Game) == arkfgf.GameASA {
			wanted[i] = ServerPorts{Game: sv.GamePort, ASA: true}
		} else {
			wanted[i] = ServerPorts{Game: sv.GamePort, Query: sv.QueryPort}
		}
	}
	ports, err := m.opt.PickPorts(wanted)
	if err != nil {
		m.fail(s, CodeLocal, "选择本机端口失败: "+err.Error())
		return
	}
	if len(ports) != len(res.Servers) {
		m.fail(s, CodeLocal, "选择本机端口失败: 端口数量和服务器数量不一致")
		return
	}
	cfg := BuildConfig(res, ports)
	portalURL := ""
	if res.PortalPort > 0 && res.PortalPort < 65536 {
		taken := map[int]bool{}
		for _, p := range ports {
			for _, port := range []int{p.Game, p.Raw, p.Query, p.Upstream} {
				taken[port] = true
			}
		}
		// 玩家站只是附加功能，选不到端口时照常进服
		if local, err := m.opt.PickTCPPort(res.PortalPort, taken); err == nil {
			cfg.Forwards = append(cfg.Forwards, easytier.Forward{Proto: "tcp", LocalPort: local, RemoteIP: res.ServerIP, RemotePort: res.PortalPort})
			portalURL = fmt.Sprintf("http://127.0.0.1:%d", local)
		}
	}
	if err := m.opt.Runner.Start(cfg); err != nil {
		m.fail(s, CodeLocal, err.Error())
		return
	}
	defer m.opt.Runner.Stop()
	for _, p := range ports {
		if p.Upstream <= 0 || p.Query <= 0 {
			continue
		}
		qp, err := startQueryProxy(p.Query, p.Upstream, p.Game)
		if err != nil {
			m.fail(s, CodeLocal, fmt.Sprintf("无法监听本机查询端口 %d: %v", p.Query, err))
			return
		}
		defer qp.Close()
	}

	m.update(s, func(st *Status) {
		if res.Room.Name != "" {
			st.RoomName = res.Room.Name
		}
		st.Phase = PhaseConnecting
		st.Servers = make([]ServerStatus, len(res.Servers))
		for i, sv := range res.Servers {
			game := arkfgf.NormalizeGame(sv.Game)
			st.Servers[i] = ServerStatus{
				Key: sv.Key, Name: sv.Name, Map: sv.Map, Game: game,
				LocalPort: ports[i].Game, RawPort: ports[i].Raw, QueryPort: ports[i].Query,
				Command: fmt.Sprintf("open 127.0.0.1:%d", ports[i].Game),
				Address: steamAddress(game, ports[i]),
			}
		}
		st.LocalPort, st.RawPort, st.QueryPort = ports[0].Game, ports[0].Raw, ports[0].Query
		st.Command = st.Servers[0].Command
		st.PortalURL = portalURL
		st.Message = "正在通过 P2P 打洞连接服务器…"
	})
	m.monitor(ctx, s, id, res.ServerIP)
}

// join 调用加入接口；凭证池空（503 no_credentials）时按 Retry-After 自动重试几次。
func (m *Manager) join(ctx context.Context, s *session, id int64) (*arkfgf.JoinResult, error) {
	for attempt := 0; ; attempt++ {
		res, err := m.opt.API.Join(ctx, m.opt.Token(), id)
		if err == nil {
			m.mu.Lock()
			s.retryAt = time.Time{}
			m.mu.Unlock()
			return res, nil
		}
		var ae *arkfgf.APIError
		if !errors.As(err, &ae) || ae.Code != arkfgf.CodeNoCredentials || attempt >= m.opt.MaxRetries {
			return nil, err
		}
		wait := ae.RetryAfter
		if wait <= 0 {
			wait = 30 * time.Second
		}
		wait = min(max(wait, time.Second), 2*time.Minute)
		m.mu.Lock()
		s.retryAt = time.Now().Add(wait)
		s.status.Message = fmt.Sprintf("房间的入场名额暂时用完，正在等服主补充（第 %d/%d 次重试）", attempt+1, m.opt.MaxRetries)
		m.mu.Unlock()
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
		m.update(s, func(st *Status) { st.Message = "正在向 arkfgf.com 申请加入…" })
	}
}

func (m *Manager) monitor(ctx context.Context, s *session, roomID int64, serverIP string) {
	started := time.Now()
	var lastDirect, lastRoomCheck time.Time
	roomOffline := false
	tick := time.NewTicker(m.opt.Poll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		running, exitErr := m.opt.Runner.Running()
		if !running {
			msg := "EasyTier 意外退出"
			if exitErr != nil {
				msg += ": " + exitErr.Error()
			}
			m.fail(s, CodeProcessExited, msg+"。可以展开日志查看原因，然后重试。")
			return
		}
		peers, err := m.opt.Runner.Peers(ctx)
		if err != nil {
			continue // 刚启动时 RPC 端口可能还没就绪
		}
		server := findServer(peers, serverIP)
		now := time.Now()
		if server != nil && strings.EqualFold(server.Cost, "p2p") {
			lastDirect = now
			lat, _ := strconv.ParseFloat(strings.TrimSpace(server.LatencyMs), 64)
			m.update(s, func(st *Status) {
				st.Phase, st.ErrorCode, st.LatencyMs = PhaseConnected, "", lat
				st.Message = "已和服务器直连"
			})
			continue
		}
		ref, reconnecting := started, !lastDirect.IsZero()
		if reconnecting {
			ref = lastDirect
		}
		if now.Sub(ref) < m.opt.Grace {
			m.update(s, func(st *Status) {
				st.Phase, st.ErrorCode, st.LatencyMs = PhaseConnecting, "", 0
				if reconnecting {
					st.Message = "和服务器的直连中断了，正在重新连接…"
				} else {
					st.Message = "正在通过 P2P 打洞连接服务器…"
				}
			})
			continue
		}
		// 找不到服务器时查一下房间是否离线（只读接口，不会分配新凭证），每 30 秒最多一次
		if server == nil && now.Sub(lastRoomCheck) >= roomCheckInterval {
			lastRoomCheck = now
			roomOffline = m.roomOffline(ctx, roomID)
		}
		m.update(s, func(st *Status) {
			st.Phase, st.LatencyMs = PhaseNotDirect, 0
			if server == nil && roomOffline {
				st.ErrorCode = arkfgf.CodeRoomOffline
				st.Message = "房间已离线（服务器停止运行了）。仍在后台重试，服务器恢复后会自动连上。"
			} else if server == nil {
				st.ErrorCode = CodeServerUnreachable
				st.Message = "找不到服务器节点。可能是房间刚刚离线、你被移出了房间，或者网络连不上 arkfgf.com 的打洞节点。仍在后台重试。"
			} else {
				st.ErrorCode = CodeNotDirect
				st.Message = "你和服务器之间无法打洞直连（两边的网络都是对称型 NAT 或有严格防火墙时会这样）。本服务不提供中继，可以换个网络（例如手机热点）再试。仍在后台重试。"
			}
		})
	}
}

const roomCheckInterval = 30 * time.Second

// roomOffline 查询房间是否已离线或下架；查询失败时当作在线（不改变提示）。
func (m *Manager) roomOffline(ctx context.Context, id int64) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	room, err := m.opt.API.GetRoom(ctx, id)
	if err != nil {
		return arkfgf.ErrorCode(err) == arkfgf.CodeNotFound
	}
	return !room.Online
}

func findServer(peers []easytier.Peer, serverIP string) *easytier.Peer {
	for i := range peers {
		ip, _, _ := strings.Cut(peers[i].IPv4, "/")
		if ip == serverIP {
			return &peers[i]
		}
	}
	return nil
}

// BuildConfig 按约定生成玩家节点参数：房间里每台 ASE 服务器各转发 L→port、L+1→port+1、Q→query_port，
// ASA 服务器只转发 L→port；ports[i] 对应 res.Servers[i]，全部合在同一个 easytier-core 进程里。
// 需要查询代理时查询端口改为从 Upstream 转发（Q 由代理监听）。
func BuildConfig(res *arkfgf.JoinResult, ports []Ports) easytier.Config {
	var fw []easytier.Forward
	for i, sv := range res.Servers {
		if i >= len(ports) {
			break
		}
		p := ports[i]
		fw = append(fw, easytier.Forward{Proto: "udp", LocalPort: p.Game, RemoteIP: res.ServerIP, RemotePort: sv.GamePort})
		if arkfgf.NormalizeGame(sv.Game) == arkfgf.GameASA {
			continue
		}
		fw = append(fw, easytier.Forward{Proto: "udp", LocalPort: p.Raw, RemoteIP: res.ServerIP, RemotePort: sv.GamePort + 1})
		if p.Query > 0 {
			fw = append(fw, easytier.Forward{Proto: "udp", LocalPort: p.queryListen(), RemoteIP: res.ServerIP, RemotePort: sv.QueryPort})
		}
	}
	return easytier.Config{
		NetworkName: res.NetworkName,
		Credential:  res.Credential,
		Peers:       res.Peers,
		Forwards:    fw,
	}
}
