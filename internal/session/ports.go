package session

import (
	"errors"
	"fmt"
	"net"
	"strconv"
)

// Ports 是本机用来转发的三个 UDP 端口：L→game_port、L+1→game_port+1、Q→query_port。
//
// Steam 进服（收藏、steam://connect）先向 Q 查询 A2S_INFO，再连「127.0.0.1 + 回复里的游戏端口」，
// 回复里是服务器自己的 game_port。L 和 game_port 不同时（玩家电脑上 7777 被占用），
// Q 由助手的查询代理监听，把回复里的游戏端口改成 L 后交给 Steam；easytier 改为在 Upstream 上转发查询端口。
type Ports struct {
	Game     int // L
	Raw      int // L+1
	Query    int // Q；服务器查询端口与游戏端口重叠时为 0（不单独转发）
	Upstream int // 需要查询代理时 easytier 转发查询端口用的本机端口，不需要时为 0
}

// queryListen 返回 easytier 转发查询端口时监听的本机端口。
func (p Ports) queryListen() int {
	if p.Upstream > 0 {
		return p.Upstream
	}
	return p.Query
}

// udpFree 判断本机 UDP 端口是否空闲：127.0.0.1 和 0.0.0.0 都要能绑上
// （本机开着 ARK 服务器时它监听 0.0.0.0:7777，只试 127.0.0.1 在某些系统上会误判为空闲）。
func udpFree(port int) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	for _, host := range []string{"127.0.0.1", "0.0.0.0"} {
		c, err := net.ListenPacket("udp4", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			return false
		}
		c.Close()
	}
	return true
}

// randomUDPPort 让系统分配一个空闲 UDP 端口。
func randomUDPPort() (int, error) {
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port, nil
}

// PickPorts 选本机端口：优先和服务器一样（L=game_port、Q=query_port），这样玩家习惯的
// `open 127.0.0.1:7777` 也能用；被占用时换成随机端口，保证 L 和 L+1 都空闲。
func PickPorts(gamePort, queryPort int) (Ports, error) {
	return pickPorts(gamePort, queryPort, udpFree, randomUDPPort)
}

func pickPorts(gamePort, queryPort int, free func(int) bool, random func() (int, error)) (Ports, error) {
	if gamePort <= 0 || gamePort >= 65535 {
		return Ports{}, fmt.Errorf("服务器游戏端口无效: %d", gamePort)
	}
	var p Ports
	if free(gamePort) && free(gamePort+1) {
		p.Game = gamePort
	} else {
		for i := 0; i < 50 && p.Game == 0; i++ {
			l, err := random()
			if err != nil {
				return Ports{}, err
			}
			if l < 65535 && free(l) && free(l+1) {
				p.Game = l
			}
		}
		if p.Game == 0 {
			return Ports{}, errors.New("找不到两个相邻的空闲 UDP 端口")
		}
	}
	p.Raw = p.Game + 1

	if queryPort <= 0 || queryPort == gamePort || queryPort == gamePort+1 {
		return p, nil
	}
	used := func(q int) bool { return q == p.Game || q == p.Raw || q == p.Query }
	if !used(queryPort) && free(queryPort) {
		p.Query = queryPort
	} else if q, err := randomFree(used, free, random); err != nil {
		return Ports{}, err
	} else {
		p.Query = q
	}
	if p.Game != gamePort {
		u, err := randomFree(used, free, random)
		if err != nil {
			return Ports{}, err
		}
		p.Upstream = u
	}
	return p, nil
}

// randomFree 随机选一个空闲、没被 used 占用的 UDP 端口。
func randomFree(used, free func(int) bool, random func() (int, error)) (int, error) {
	for range 50 {
		q, err := random()
		if err != nil {
			return 0, err
		}
		if !used(q) && free(q) {
			return q, nil
		}
	}
	return 0, errors.New("找不到空闲的 UDP 端口")
}

// ServerPorts 是房间里一台服务器要转发的端口（服务器上的游戏端口和查询端口）。
type ServerPorts struct{ Game, Query int }

// PickAllPorts 为房间里的每台服务器各选一组本机端口，按 servers 的顺序返回。
// 每台服务器仍然优先用和服务器一样的端口；已经分给前面服务器的端口不会再被选中，
// 所以多台服务器的 L、L+1、Q 两两不重叠。
func PickAllPorts(servers []ServerPorts) ([]Ports, error) {
	return pickAllPorts(servers, udpFree, randomUDPPort)
}

func pickAllPorts(servers []ServerPorts, free func(int) bool, random func() (int, error)) ([]Ports, error) {
	if len(servers) == 0 {
		return nil, errors.New("房间里没有服务器")
	}
	taken := map[int]bool{}
	available := func(p int) bool { return !taken[p] && free(p) }
	out := make([]Ports, 0, len(servers))
	for _, s := range servers {
		p, err := pickPorts(s.Game, s.Query, available, random)
		if err != nil {
			return nil, err
		}
		for _, port := range []int{p.Game, p.Raw, p.Query, p.Upstream} {
			if port > 0 {
				taken[port] = true
			}
		}
		out = append(out, p)
	}
	return out, nil
}
