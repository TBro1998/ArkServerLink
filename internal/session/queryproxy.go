package session

import (
	"bytes"
	"errors"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"
)

// queryIdle 是查询代理里一个客户端多久没有回包就释放它的上游连接。
const queryIdle = 2 * time.Minute

// queryProxy 监听本机查询端口，把 Steam 的服务器查询转给 easytier 的查询端口转发，
// 并把 A2S_INFO 回复里的游戏端口改成本机游戏端口（见 Ports）。
//
// 每个查询来源用一个单独的上游连接，同一来源的查询和回复（含 A2S 的 challenge）走同一个源端口。
type queryProxy struct {
	conn     *net.UDPConn
	upstream *net.UDPAddr
	gamePort uint16

	mu      sync.Mutex
	closed  bool
	clients map[netip.AddrPort]*net.UDPConn
	wg      sync.WaitGroup
}

// startQueryProxy 在 127.0.0.1:listen 上启动查询代理，转发到 127.0.0.1:upstream。
func startQueryProxy(listen, upstream, gamePort int) (*queryProxy, error) {
	loopback := net.IPv4(127, 0, 0, 1)
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: loopback, Port: listen})
	if err != nil {
		return nil, err
	}
	p := &queryProxy{
		conn:     conn,
		upstream: &net.UDPAddr{IP: loopback, Port: upstream},
		gamePort: uint16(gamePort),
		clients:  map[netip.AddrPort]*net.UDPConn{},
	}
	p.wg.Add(1)
	go p.serve()
	return p, nil
}

func (p *queryProxy) serve() {
	defer p.wg.Done()
	buf := make([]byte, 65535)
	for {
		n, from, err := p.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			// Windows 上发往已关闭端口的回包会让下一次读取报 WSAECONNRESET，忽略即可
			continue
		}
		up, err := p.client(from)
		if err != nil {
			continue
		}
		_, _ = up.Write(buf[:n])
	}
}

// client 返回 from 对应的上游连接，没有时新建一个并开始转发回包。
func (p *queryProxy) client(from netip.AddrPort) (*net.UDPConn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, net.ErrClosed
	}
	if up := p.clients[from]; up != nil {
		return up, nil
	}
	up, err := net.DialUDP("udp4", nil, p.upstream)
	if err != nil {
		return nil, err
	}
	p.clients[from] = up
	p.wg.Add(1)
	go p.relay(from, up)
	return up, nil
}

// relay 把上游的回包改写后发回 to。空闲超时或出错时释放这个上游连接，下次查询会重新建立。
func (p *queryProxy) relay(to netip.AddrPort, up *net.UDPConn) {
	defer p.wg.Done()
	defer func() {
		p.mu.Lock()
		if p.clients[to] == up {
			delete(p.clients, to)
		}
		p.mu.Unlock()
		up.Close()
	}()
	buf := make([]byte, 65535)
	for {
		_ = up.SetReadDeadline(time.Now().Add(queryIdle))
		n, err := up.Read(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrDeadlineExceeded) {
				return
			}
			// easytier 的转发还没就绪时会收到 WSAECONNRESET，继续等
			continue
		}
		_, _ = p.conn.WriteToUDPAddrPort(rewriteInfoPort(buf[:n], p.gamePort), to)
	}
}

// Close 停止代理并等待所有转发结束。
func (p *queryProxy) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	err := p.conn.Close()
	for _, up := range p.clients {
		up.Close()
	}
	p.mu.Unlock()
	p.wg.Wait()
	return err
}

// rewriteInfoPort 把 A2S_INFO 回复里 EDF 带的游戏端口改成 port（就地修改），其他报文原样返回。
//
// 格式：FF FF FF FF 'I'、协议版本、名称、地图、目录、游戏（都以 \0 结尾）、
// ID(2)、人数、最大人数、机器人、类型、系统、是否有密码、VAC，版本（\0 结尾），
// EDF，EDF & 0x80 时紧跟小端序的 2 字节游戏端口。
func rewriteInfoPort(b []byte, port uint16) []byte {
	if len(b) < 6 || !bytes.Equal(b[:5], []byte{0xff, 0xff, 0xff, 0xff, 'I'}) {
		return b
	}
	i := 6
	for range 4 {
		j := bytes.IndexByte(b[i:], 0)
		if j < 0 {
			return b
		}
		i += j + 1
	}
	i += 9
	if i >= len(b) {
		return b
	}
	j := bytes.IndexByte(b[i:], 0)
	if j < 0 {
		return b
	}
	i += j + 1
	if i+3 > len(b) || b[i]&0x80 == 0 {
		return b
	}
	b[i+1], b[i+2] = byte(port), byte(port>>8)
	return b
}
