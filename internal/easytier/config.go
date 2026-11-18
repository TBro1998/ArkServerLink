// Package easytier 管理玩家端的 easytier-core 子进程。
//
// 参数组合在 EasyTier v2.6.4 上实测过，改动前请先看 README 的「组网方案」一节。
package easytier

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

// Forward 把本机端口转发到虚拟网里服务器的端口。
// 玩家端不建 TUN 网卡，游戏通过 127.0.0.1:LocalPort 连服务器。
type Forward struct {
	Proto      string `json:"proto"` // "udp" 或 "tcp"
	LocalPort  int    `json:"localPort"`
	RemoteIP   string `json:"remoteIp"`
	RemotePort int    `json:"remotePort"`
}

// DefaultHostname 是玩家节点在服主 peer 表里显示的名字。
const DefaultHostname = "arkfgf-player"

// Config 是玩家加入一个联机房需要的全部参数，由 arkfgf.com 下发。
type Config struct {
	NetworkName string    `json:"networkName"`
	Credential  string    `json:"credential"` // 服主签发的凭证私钥（base64），代替 network_secret
	Peers       []string  `json:"peers"`      // 公共节点地址，如 wss://arkfgf.com/et/
	Forwards    []Forward `json:"forwards"`
	Hostname    string    `json:"hostname"` // 在服主 peer 表里显示的名字，可空
}

func (c Config) Validate() error {
	if c.NetworkName == "" {
		return errors.New("缺少房间网络名")
	}
	if c.Credential == "" {
		return errors.New("缺少凭证")
	}
	// 凭证是 X25519 私钥，base64 后必须是 32 字节；格式不对 easytier-core 会直接退出
	if key, err := base64.StdEncoding.DecodeString(c.Credential); err != nil || len(key) != 32 {
		return errors.New("凭证格式不正确")
	}
	if len(c.Peers) == 0 {
		return errors.New("缺少公共节点地址")
	}
	if len(c.Forwards) == 0 {
		return errors.New("缺少端口转发")
	}
	for _, f := range c.Forwards {
		if f.Proto != "udp" && f.Proto != "tcp" {
			return fmt.Errorf("端口转发协议只能是 udp 或 tcp，收到 %q", f.Proto)
		}
		if !validPort(f.LocalPort) || !validPort(f.RemotePort) {
			return fmt.Errorf("端口超出范围: %d -> %d", f.LocalPort, f.RemotePort)
		}
		if ip, err := netip.ParseAddr(f.RemoteIP); err != nil || !ip.Is4() {
			return fmt.Errorf("服务器虚拟 IP 无效: %q", f.RemoteIP)
		}
	}
	return nil
}

func validPort(p int) bool { return p > 0 && p < 65536 }

// Args 生成 easytier-core 的命令行参数。凭证不放在这里，见 Env。
func (c Config) Args(rpcPort int) []string {
	args := []string{
		"--network-name", c.NetworkName,
		// 凭证入网需要 secure-mode，公共节点和服主节点也必须开
		"--secure-mode",
		// 虚拟 IP 自动分配
		"--dhcp",
		// 不建虚拟网卡：不需要管理员权限，也不用装驱动
		"--no-tun",
		// 只和直连的节点通信：打洞失败就连不上，不走任何中继
		"--p2p-only",
		// 拒绝所有入站，避免其他玩家访问本机端口；自己发起的连接回包不受影响
		"--tcp-whitelist", "0",
		"--udp-whitelist", "0",
		// 随机端口监听，避免和玩家本机已有的 EasyTier（默认 11010）冲突
		"--listeners", "tcp://0.0.0.0:0", "udp://0.0.0.0:0",
		"--rpc-portal", fmt.Sprintf("127.0.0.1:%d", rpcPort),
		"--console-log-level", "warn",
	}
	// 不指定时 easytier 用 Windows 计算机名，服主在 peer 表里能看到，所以默认给一个中性名字
	hostname := c.Hostname
	if hostname == "" {
		hostname = DefaultHostname
	}
	args = append(args, "--hostname", hostname)
	for _, p := range c.Peers {
		args = append(args, "--peers", p)
	}
	// easytier 默认把连接绑定到物理网卡，连不上本机回环地址（本地联调时公共节点在 127.0.0.1）
	if c.hasLoopbackPeer() {
		args = append(args, "--bind-device", "false")
	}
	for _, f := range c.Forwards {
		// 只监听回环地址，不把服务器端口暴露给玩家所在的局域网
		args = append(args, "--port-forward",
			fmt.Sprintf("%s://127.0.0.1:%d/%s:%d", f.Proto, f.LocalPort, f.RemoteIP, f.RemotePort))
	}
	return args
}

func (c Config) hasLoopbackPeer() bool {
	for _, p := range c.Peers {
		u, err := url.Parse(p)
		if err != nil {
			continue
		}
		if h := u.Hostname(); h == "localhost" {
			return true
		} else if ip, err := netip.ParseAddr(h); err == nil && ip.IsLoopback() {
			return true
		}
	}
	return false
}

// Env 返回需要额外设置的环境变量。凭证走环境变量，不出现在进程命令行里。
func (c Config) Env() []string {
	return []string{"ET_CREDENTIAL=" + c.Credential}
}

// Redacted 返回可以写进日志的参数描述。
func (c Config) Redacted() string {
	return fmt.Sprintf("network=%s peers=%s forwards=%d", c.NetworkName, strings.Join(c.Peers, ","), len(c.Forwards))
}
