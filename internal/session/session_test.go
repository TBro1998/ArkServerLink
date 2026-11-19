package session

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"arkfgf.com/link/internal/arkfgf"
	"arkfgf.com/link/internal/easytier"
)

const testCred = "BdQIvXlW3kAlnhPiyAtXq1OlSD5jISNyDhMGzLOz2t4="

func TestPickPortsPrefersServerPorts(t *testing.T) {
	p, err := pickPorts(7777, 27015, func(int) bool { return true }, func() (int, error) { return 0, errors.New("不应调用") })
	if err != nil || p != (Ports{Game: 7777, Raw: 7778, Query: 27015}) {
		t.Fatalf("p = %+v err = %v", p, err)
	}
}

func TestPickPortsFallback(t *testing.T) {
	busy := map[int]bool{7778: true, 27015: true, 40001: true}
	randoms := []int{40000, 50000, 50001, 50002}
	i := 0
	random := func() (int, error) { i++; return randoms[i-1], nil }
	p, err := pickPorts(7777, 27015, func(port int) bool { return !busy[port] }, random)
	if err != nil {
		t.Fatal(err)
	}
	// 7778 被占 → 随机；40000 的 +1 被占 → 换 50000；查询端口被占，50001 与 L+1 冲突 → 50002
	if p != (Ports{Game: 50000, Raw: 50001, Query: 50002}) {
		t.Errorf("p = %+v", p)
	}
}

func TestPickPortsOverlappingQueryPort(t *testing.T) {
	p, err := pickPorts(7777, 7778, func(int) bool { return true }, nil)
	if err != nil || p.Query != 0 || p.Game != 7777 {
		t.Errorf("p = %+v err = %v", p, err)
	}
	if _, err := pickPorts(0, 27015, nil, nil); err == nil {
		t.Error("无效端口应报错")
	}
}

func TestPickPortsReal(t *testing.T) {
	// 占住一个端口，PickPorts 应该绕开它
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	taken := c.LocalAddr().(*net.UDPAddr).Port
	if udpFree(taken) {
		t.Fatal("已占用的端口被判为空闲")
	}
	p, err := PickPorts(taken, taken+100)
	if err != nil {
		t.Fatal(err)
	}
	if p.Game == taken || p.Raw != p.Game+1 || !udpFree(p.Game) || !udpFree(p.Raw) || p.Query == 0 {
		t.Errorf("p = %+v taken = %d", p, taken)
	}
}

// 多台服务器：各自优先用服务器的端口；已分给前面服务器的端口不会再分出去
func TestPickAllPorts(t *testing.T) {
	all := func(int) bool { return true }
	ps, err := pickAllPorts([]ServerPorts{{Game: 7777, Query: 27015}, {Game: 7779, Query: 27017}}, all, nil)
	if err != nil || len(ps) != 2 || ps[0] != (Ports{Game: 7777, Raw: 7778, Query: 27015}) || ps[1] != (Ports{Game: 7779, Raw: 7780, Query: 27017}) {
		t.Fatalf("ps = %+v err = %v", ps, err)
	}

	// 第二台的游戏端口 7778 正好是第一台的 L+1，第二台的查询端口和第一台的相同：都要换成随机端口
	randoms := []int{40000, 40002}
	i := 0
	random := func() (int, error) { i++; return randoms[i-1], nil }
	ps, err = pickAllPorts([]ServerPorts{{Game: 7777, Query: 27015}, {Game: 7778, Query: 27015}}, all, random)
	if err != nil {
		t.Fatal(err)
	}
	if ps[1] != (Ports{Game: 40000, Raw: 40001, Query: 40002}) {
		t.Fatalf("ps = %+v", ps)
	}
	seen := map[int]bool{}
	for _, p := range ps {
		for _, port := range []int{p.Game, p.Raw, p.Query} {
			if seen[port] {
				t.Fatalf("端口 %d 重复: %+v", port, ps)
			}
			seen[port] = true
		}
	}

	if _, err := pickAllPorts(nil, all, nil); err == nil {
		t.Error("没有服务器时应报错")
	}
}

func TestJoinMultipleServers(t *testing.T) {
	r := &fakeRunner{}
	m := newTestManager(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(joinBodyMulti("集群"))) }, r, nil)
	if err := m.Join(12, ""); err != nil {
		t.Fatal(err)
	}
	st := waitPhase(t, m, PhaseConnecting)
	if len(st.Servers) != 2 {
		t.Fatalf("st = %+v", st)
	}
	a, b := st.Servers[0], st.Servers[1]
	if a.Key != "1" || a.Name != "集群 A" || a.Command != "open 127.0.0.1:7777" || a.QueryPort != 27015 {
		t.Errorf("server a = %+v", a)
	}
	if b.Key != "2" || b.Map != "Ragnarok" || b.Command != "open 127.0.0.1:7779" || b.RawPort != 7780 {
		t.Errorf("server b = %+v", b)
	}
	// 顶层字段是第一台的
	if st.LocalPort != 7777 || st.Command != a.Command {
		t.Errorf("st = %+v", st)
	}
	// 一个 easytier-core 进程，两台服务器各 3 条转发
	if len(r.cfg.Forwards) != 6 {
		t.Errorf("forwards = %+v", r.cfg.Forwards)
	}

	// 一个节点的直连就够了：所有服务器都在同一个虚拟 IP 上
	r.set(func(f *fakeRunner) { f.peers = []easytier.Peer{{IPv4: "10.126.126.1", Cost: "p2p", LatencyMs: "10"}} })
	waitPhase(t, m, PhaseConnected)
}

func TestJoinEmptyRoom(t *testing.T) {
	r := &fakeRunner{}
	m := newTestManager(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"room":{"id":12,"name":"x"},"network_name":"ark-0123456789abcdef","credential":"` + testCred + `",
			"credential_expires_at":1797600000,"peers":["tcp://127.0.0.1:21010"],"server_ip":"10.126.126.1","servers":[]}`))
	}, r, nil)
	m.Join(12, "")
	st := waitPhase(t, m, PhaseError)
	if st.ErrorCode != CodeLocal {
		t.Errorf("st = %+v", st)
	}
	if running, _ := r.Running(); running {
		t.Error("房间里没有服务器时不应启动 easytier")
	}
}

func TestBuildConfig(t *testing.T) {
	res := &arkfgf.JoinResult{NetworkName: "ark-1", Credential: testCred, Peers: []string{"tcp://et.arkfgf.com:11010"},
		ServerIP: "10.126.126.1",
		Servers:  []arkfgf.JoinServer{{Key: "1", GamePort: 7777, QueryPort: 27015}, {Key: "2", GamePort: 7779, QueryPort: 27017}}}
	cfg := BuildConfig(res, []Ports{{Game: 7787, Raw: 7788, Query: 27020}, {Game: 7789, Raw: 7790, Query: 27021}})
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	args := strings.Join(cfg.Args(1), " ")
	for _, want := range []string{
		"--port-forward udp://127.0.0.1:7787/10.126.126.1:7777",
		"--port-forward udp://127.0.0.1:7788/10.126.126.1:7778",
		"--port-forward udp://127.0.0.1:27020/10.126.126.1:27015",
		"--port-forward udp://127.0.0.1:7789/10.126.126.1:7779",
		"--port-forward udp://127.0.0.1:7790/10.126.126.1:7780",
		"--port-forward udp://127.0.0.1:27021/10.126.126.1:27017",
		"--network-name ark-1",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("缺少 %q: %s", want, args)
		}
	}
}

// fakeRunner 模拟 easytier.Runner。
type fakeRunner struct {
	mu      sync.Mutex
	running bool
	exitErr error
	cfg     easytier.Config
	peers   []easytier.Peer
	stops   int
}

func (f *fakeRunner) Start(cfg easytier.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.running {
		return errors.New("已经在连接中")
	}
	f.running, f.cfg, f.exitErr = true, cfg, nil
	return nil
}

func (f *fakeRunner) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running = false
	f.stops++
	return nil
}

func (f *fakeRunner) Running() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running, f.exitErr
}

func (f *fakeRunner) Peers(context.Context) ([]easytier.Peer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]easytier.Peer(nil), f.peers...), nil
}

func (f *fakeRunner) set(fn func(f *fakeRunner)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func joinBody(name string) string {
	return `{"room":{"id":12,"name":"` + name + `"},"network_name":"ark-0123456789abcdef","credential":"` + testCred + `",
		"credential_expires_at":1797600000,"peers":["tcp://127.0.0.1:21010"],"server_ip":"10.126.126.1",
		"servers":[{"key":"1","name":"` + name + `","map":"TheIsland","online":true,"game_port":7777,"query_port":27015}]}`
}

// joinBodyMulti 返回一个房间里有两台服务器的加入响应
func joinBodyMulti(name string) string {
	return `{"room":{"id":12,"name":"` + name + `"},"network_name":"ark-0123456789abcdef","credential":"` + testCred + `",
		"credential_expires_at":1797600000,"peers":["tcp://127.0.0.1:21010"],"server_ip":"10.126.126.1",
		"servers":[{"key":"1","name":"` + name + ` A","map":"TheIsland","online":true,"game_port":7777,"query_port":27015},
		{"key":"2","name":"` + name + ` B","map":"Ragnarok","online":true,"game_port":7779,"query_port":27017}]}`
}

func newTestManager(t *testing.T, h http.HandlerFunc, r *fakeRunner, unauthorized func()) *Manager {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	m := NewManager(Options{
		API:            arkfgf.New(srv.URL, "test"),
		Runner:         r,
		Token:          func() string { return "jwt" },
		OnUnauthorized: unauthorized,
		Grace:          300 * time.Millisecond,
		Poll:           20 * time.Millisecond,
		PickPorts: func(s []ServerPorts) ([]Ports, error) {
			out := make([]Ports, len(s))
			for i, p := range s {
				out[i] = Ports{Game: p.Game, Raw: p.Game + 1, Query: p.Query}
			}
			return out, nil
		},
	})
	t.Cleanup(m.Leave)
	return m
}

func waitPhase(t *testing.T, m *Manager, want Phase) Status {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		st := m.Status()
		if st.Phase == want {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待 %s 超时，当前 %+v", want, st)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestJoinConnectNotDirectLeave(t *testing.T) {
	r := &fakeRunner{}
	var offline atomic.Bool
	m := newTestManager(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == "/link/rooms/12" {
			fmt.Fprintf(w, `{"id":12,"online":%v}`, !offline.Load())
			return
		}
		if req.URL.Path != "/link/rooms/12/join" || req.Header.Get("Authorization") != "Bearer jwt" {
			t.Errorf("unexpected %s %q", req.URL.Path, req.Header.Get("Authorization"))
		}
		w.Write([]byte(joinBody("孤岛")))
	}, r, nil)

	if err := m.Join(12, "加载中"); err != nil {
		t.Fatal(err)
	}
	st := waitPhase(t, m, PhaseConnecting)
	if st.RoomName != "孤岛" || st.LocalPort != 7777 || st.Command != "open 127.0.0.1:7777" {
		t.Errorf("st = %+v", st)
	}
	if len(r.cfg.Forwards) != 3 || r.cfg.NetworkName != "ark-0123456789abcdef" {
		t.Errorf("cfg = %+v", r.cfg)
	}

	// 服务器出现但走的是中继：宽限期后判定为无法直连
	r.set(func(f *fakeRunner) {
		f.peers = []easytier.Peer{{IPv4: "10.126.126.1/24", Cost: "relay(2)"}}
	})
	st = waitPhase(t, m, PhaseNotDirect)
	if st.ErrorCode != CodeNotDirect {
		t.Errorf("st = %+v", st)
	}

	// 打洞成功
	r.set(func(f *fakeRunner) { f.peers = []easytier.Peer{{IPv4: "10.126.126.1", Cost: "p2p", LatencyMs: "23.5"}} })
	st = waitPhase(t, m, PhaseConnected)
	if st.LatencyMs != 23.5 || st.ErrorCode != "" {
		t.Errorf("st = %+v", st)
	}

	// 同一房间再次加入不重启
	if err := m.Join(12, ""); err != nil {
		t.Fatal(err)
	}
	if r.stops != 0 {
		t.Error("重复加入同一房间不应重启")
	}

	// 直连中断 → 先显示重连中
	r.set(func(f *fakeRunner) { f.peers = nil })
	st = waitPhase(t, m, PhaseConnecting)
	if !strings.Contains(st.Message, "中断") {
		t.Errorf("st = %+v", st)
	}
	st = waitPhase(t, m, PhaseNotDirect)
	if st.ErrorCode != CodeServerUnreachable {
		t.Errorf("st = %+v", st)
	}

	// 找不到服务器且房间已离线时提示离线
	offline.Store(true)
	m.Leave()
	if err := m.Join(12, ""); err != nil {
		t.Fatal(err)
	}
	st = waitPhase(t, m, PhaseNotDirect)
	if st.ErrorCode != arkfgf.CodeRoomOffline || !strings.Contains(st.Message, "离线") {
		t.Errorf("房间离线时 st = %+v", st)
	}

	m.Leave()
	if m.Status().Phase != PhaseIdle {
		t.Errorf("退出后状态 %+v", m.Status())
	}
	if running, _ := r.Running(); running {
		t.Error("退出后 easytier 仍在运行")
	}
}

func TestJoinErrors(t *testing.T) {
	cases := []struct {
		status int
		body   string
		code   string
	}{
		{409, `{"error":"离线","code":"room_offline"}`, arkfgf.CodeRoomOffline},
		{403, `{"error":"封禁","code":"banned"}`, arkfgf.CodeBanned},
		{404, `{"error":"不存在","code":"not_found"}`, arkfgf.CodeNotFound},
	}
	for _, tc := range cases {
		r := &fakeRunner{}
		m := newTestManager(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		}, r, nil)
		m.Join(12, "")
		st := waitPhase(t, m, PhaseError)
		if st.ErrorCode != tc.code || st.Message == "" {
			t.Errorf("%d: %+v", tc.status, st)
		}
		if running, _ := r.Running(); running {
			t.Errorf("%d: 失败时不应启动 easytier", tc.status)
		}
	}
}

func TestJoinUnauthorized(t *testing.T) {
	var called atomic.Bool
	m := newTestManager(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"invalid token"}`))
	}, &fakeRunner{}, func() { called.Store(true) })
	m.Join(12, "")
	st := waitPhase(t, m, PhaseError)
	if st.ErrorCode != arkfgf.CodeUnauthorized || !called.Load() {
		t.Errorf("st = %+v called = %v", st, called.Load())
	}
}

func TestJoinRetriesNoCredentials(t *testing.T) {
	var calls atomic.Int32
	r := &fakeRunner{}
	m := newTestManager(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) <= 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(503)
			w.Write([]byte(`{"error":"凭证不足","code":"no_credentials"}`))
			return
		}
		w.Write([]byte(joinBody("x")))
	}, r, nil)
	m.Join(12, "")
	time.Sleep(200 * time.Millisecond)
	st := m.Status()
	if st.Phase != PhaseJoining || st.RetryIn < 1 || !strings.Contains(st.Message, "1/3") {
		t.Errorf("等待重试时 st = %+v", st)
	}
	waitPhase(t, m, PhaseConnecting)
	if calls.Load() != 3 {
		t.Errorf("calls = %d", calls.Load())
	}
}

func TestJoinGivesUpAfterRetries(t *testing.T) {
	var calls atomic.Int32
	m := newTestManager(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(503)
		w.Write([]byte(`{"code":"no_credentials"}`))
	}, &fakeRunner{}, nil)
	m.opt.MaxRetries = 2
	m.Join(12, "")
	deadline := time.Now().Add(5 * time.Second)
	for m.Status().Phase != PhaseError && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if st := m.Status(); st.Phase != PhaseError || st.ErrorCode != arkfgf.CodeNoCredentials || calls.Load() != 3 {
		t.Errorf("st = %+v calls = %d", st, calls.Load())
	}
}

func TestLeaveCancelsRetryWait(t *testing.T) {
	m := newTestManager(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(503)
		w.Write([]byte(`{"code":"no_credentials"}`))
	}, &fakeRunner{}, nil)
	m.Join(12, "")
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	m.Leave()
	if time.Since(start) > time.Second || m.Status().Phase != PhaseIdle {
		t.Errorf("Leave 没有及时取消等待: %v %+v", time.Since(start), m.Status())
	}
}

func TestSwitchRoomAndProcessExit(t *testing.T) {
	r := &fakeRunner{}
	m := newTestManager(t, func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(joinBody("x"))) }, r, nil)
	m.Join(12, "")
	waitPhase(t, m, PhaseConnecting)
	if err := m.Join(13, "另一个"); err != nil {
		t.Fatal(err)
	}
	if r.stops == 0 {
		t.Error("切换房间应先断开旧的")
	}
	st := waitPhase(t, m, PhaseConnecting)
	if st.RoomID != 13 {
		t.Errorf("st = %+v", st)
	}

	r.set(func(f *fakeRunner) { f.running, f.exitErr = false, errors.New("exit status 1") })
	st = waitPhase(t, m, PhaseError)
	if st.ErrorCode != CodeProcessExited || !strings.Contains(st.Message, "exit status 1") {
		t.Errorf("st = %+v", st)
	}
}

func TestJoinWithoutRunner(t *testing.T) {
	m := NewManager(Options{RunnerErr: errors.New("找不到 easytier-core.exe")})
	if err := m.Join(1, ""); err == nil || !strings.Contains(err.Error(), "easytier") {
		t.Errorf("err = %v", err)
	}
}
