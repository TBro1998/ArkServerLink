package session

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"arkfgf.com/link/internal/arkfgf"
	"arkfgf.com/link/internal/easytier"
)

// 端到端：假网站 + 真实 easytier-core + dev/docker-compose.yml 里的公共节点和服主节点。
// 环境变量同 internal/easytier 的 TestIntegrationConnect：
//
//	ET_IT_PEER=tcp://127.0.0.1:21010 ET_IT_NETWORK=room-dev ET_IT_CREDENTIAL=<凭证> go test -run Integration ./internal/session
func TestIntegrationJoin(t *testing.T) {
	peer := os.Getenv("ET_IT_PEER")
	if peer == "" {
		t.Skip("未设置 ET_IT_PEER")
	}
	dir, _ := filepath.Abs(filepath.Join("..", "..", "bin", "easytier"))
	if _, err := os.Stat(dir); err != nil {
		t.Skip("bin/easytier 不存在")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"room":{"id":1,"name":"联调"},"network_name":%q,"credential":%q,"credential_expires_at":0,
			"peers":[%q],"server_ip":"10.126.126.1","game_port":7777,"query_port":27015}`,
			os.Getenv("ET_IT_NETWORK"), os.Getenv("ET_IT_CREDENTIAL"), peer)
	}))
	defer srv.Close()

	runner := easytier.NewRunner(dir)
	m := NewManager(Options{API: arkfgf.New(srv.URL, "it"), Runner: runner, Token: func() string { return "jwt" }, Poll: 500 * time.Millisecond})
	defer m.Leave()
	if err := m.Join(1, ""); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(40 * time.Second)
	for m.Status().Phase != PhaseConnected {
		if time.Now().After(deadline) || m.Status().Phase == PhaseError {
			t.Fatalf("没有直连: %+v\n%s", m.Status(), strings.Join(runner.Logs(), "\n"))
		}
		time.Sleep(300 * time.Millisecond)
	}
	st := m.Status()
	t.Logf("状态: %+v", st)

	conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", st.LocalPort))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	buf := make([]byte, 256)
	// 转发刚建好时偶尔丢第一个包，多试几次
	for i := 0; ; i++ {
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		conn.Write([]byte("ping"))
		n, err := conn.Read(buf)
		if err == nil {
			t.Logf("回包: %s", buf[:n])
			break
		}
		if i == 3 {
			t.Fatalf("游戏端口不通: %v", err)
		}
	}

	m.Leave()
	if running, _ := runner.Running(); running {
		t.Fatal("Leave 之后 easytier-core 仍在运行")
	}
}
