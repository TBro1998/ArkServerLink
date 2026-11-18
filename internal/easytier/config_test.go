package easytier

import (
	"slices"
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{
		NetworkName: "room-1",
		Credential:  "BdQIvXlW3kAlnhPiyAtXq1OlSD5jISNyDhMGzLOz2t4=",
		Peers:       []string{"tcp://et.arkfgf.com:11010"},
		Forwards:    []Forward{{Proto: "udp", LocalPort: 7777, RemoteIP: "10.126.126.1", RemotePort: 7777}},
	}
}

func TestArgsSecurityFlags(t *testing.T) {
	args := validConfig().Args(15999)
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"--secure-mode", "--no-tun", "--p2p-only", "--dhcp",
		"--tcp-whitelist 0", "--udp-whitelist 0",
		"--rpc-portal 127.0.0.1:15999",
		"--port-forward udp://127.0.0.1:7777/10.126.126.1:7777",
		"--peers tcp://et.arkfgf.com:11010",
		"--hostname " + DefaultHostname, // 不能泄露计算机名
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("参数缺少 %q\n%s", want, joined)
		}
	}
	// 凭证只能走环境变量
	if slices.Contains(args, "BdQIvXlW3kAlnhPiyAtXq1OlSD5jISNyDhMGzLOz2t4=") || strings.Contains(joined, "--credential") {
		t.Errorf("凭证出现在命令行参数里: %s", joined)
	}
	if env := validConfig().Env(); !slices.Contains(env, "ET_CREDENTIAL=BdQIvXlW3kAlnhPiyAtXq1OlSD5jISNyDhMGzLOz2t4=") {
		t.Errorf("环境变量缺少凭证: %v", env)
	}
	if strings.Contains(joined, "--bind-device") {
		t.Errorf("公网节点不应关闭 bind-device: %s", joined)
	}
}

func TestArgsLoopbackPeer(t *testing.T) {
	for _, peer := range []string{"tcp://127.0.0.1:21010", "udp://localhost:21010", "tcp://[::1]:21010"} {
		c := validConfig()
		c.Peers = []string{peer}
		if !strings.Contains(strings.Join(c.Args(1), " "), "--bind-device false") {
			t.Errorf("%s: 回环地址应关闭 bind-device", peer)
		}
	}
}

func TestValidate(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("合法配置报错: %v", err)
	}
	cases := map[string]func(*Config){
		"无网络名": func(c *Config) { c.NetworkName = "" },
		"无凭证":  func(c *Config) { c.Credential = "" },
		"凭证过短": func(c *Config) { c.Credential = "c2VjcmV0" },
		"凭证非法": func(c *Config) { c.Credential = "not base64!" },
		"无节点":  func(c *Config) { c.Peers = nil },
		"无转发":  func(c *Config) { c.Forwards = nil },
		"协议错误": func(c *Config) { c.Forwards[0].Proto = "icmp" },
		"端口越界": func(c *Config) { c.Forwards[0].LocalPort = 70000 },
		"IP错误": func(c *Config) { c.Forwards[0].RemoteIP = "server" },
		"IPv6": func(c *Config) { c.Forwards[0].RemoteIP = "fd00::1" },
	}
	for name, mutate := range cases {
		c := validConfig()
		mutate(&c)
		if c.Validate() == nil {
			t.Errorf("%s: 应该报错", name)
		}
	}
}
