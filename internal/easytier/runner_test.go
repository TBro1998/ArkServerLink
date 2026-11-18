package easytier

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 这些测试需要先运行 go run ./scripts/fetch-easytier。

func testDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "bin", "easytier"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, binName("easytier-core"))); err != nil {
		t.Skip("bin/easytier 不存在，先运行 go run ./scripts/fetch-easytier")
	}
	return dir
}

func (r *Runner) pid() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd == nil {
		return 0
	}
	return r.cmd.Process.Pid
}

// 客户端被强制结束时，easytier-core 必须跟着退出（Windows 靠作业对象）。
func TestChildDiesWithParent(t *testing.T) {
	if os.Getenv("ET_HELPER") == "1" {
		r := NewRunner(os.Getenv("ET_DIR"))
		cfg := validConfig()
		cfg.Peers = []string{"tcp://127.0.0.1:1"} // 连不上也没关系，进程会一直重试
		if err := r.Start(cfg); err != nil {
			fmt.Println("ERR", err)
			os.Exit(1)
		}
		fmt.Println("PID", r.pid())
		time.Sleep(time.Minute)
		os.Exit(0)
	}

	dir := testDir(t)
	helper := exec.Command(os.Args[0], "-test.run=^TestChildDiesWithParent$")
	helper.Env = append(os.Environ(), "ET_HELPER=1", "ET_DIR="+dir)
	out, _ := helper.StdoutPipe()
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	line, _ := bufio.NewReader(out).ReadString('\n')
	pidStr, ok := strings.CutPrefix(strings.TrimSpace(line), "PID ")
	if !ok {
		helper.Process.Kill()
		t.Fatalf("helper 输出异常: %q", line)
	}
	corePid, _ := strconv.Atoi(pidStr)
	time.Sleep(time.Second)
	if !processAlive(corePid) {
		t.Fatal("easytier-core 没有启动起来")
	}

	helper.Process.Kill() // 模拟崩溃 / 任务管理器强制结束
	helper.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for processAlive(corePid) {
		if time.Now().After(deadline) {
			t.Fatalf("父进程被结束后 easytier-core (pid %d) 仍在运行", corePid)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// 端到端：需要按 README「本地联调」起好 Docker 里的公共节点和服主节点，再设置环境变量：
//
//	ET_IT_PEER=tcp://127.0.0.1:21010 ET_IT_NETWORK=room-it ET_IT_CREDENTIAL=<凭证> go test -run Integration ./internal/easytier
func TestIntegrationConnect(t *testing.T) {
	peer := os.Getenv("ET_IT_PEER")
	if peer == "" {
		t.Skip("未设置 ET_IT_PEER")
	}
	r := NewRunner(testDir(t))
	localPort := freeUDPPort(t)
	cfg := Config{
		NetworkName: os.Getenv("ET_IT_NETWORK"),
		Credential:  os.Getenv("ET_IT_CREDENTIAL"),
		Peers:       []string{peer},
		Forwards:    []Forward{{Proto: "udp", LocalPort: localPort, RemoteIP: "10.126.126.1", RemotePort: 7777}},
	}
	if err := r.Start(cfg); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	// 等服务器直连
	var server *Peer
	deadline := time.Now().Add(30 * time.Second)
	for server == nil || server.Cost != "p2p" {
		if time.Now().After(deadline) {
			t.Fatalf("30 秒内没有和服务器直连，peer=%+v\n%s", server, strings.Join(r.Logs(), "\n"))
		}
		time.Sleep(time.Second)
		peers, err := r.Peers(context.Background())
		if err != nil {
			continue
		}
		for i := range peers {
			if peers[i].IPv4 == "10.126.126.1" {
				server = &peers[i]
			}
		}
	}
	t.Logf("服务器: %+v", *server)

	conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", localPort))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	conn.Write([]byte("ping"))
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("游戏端口不通: %v", err)
	}
	t.Logf("回包: %s", buf[:n])

	pid := r.pid()
	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
	if running, _ := r.Running(); running || processAlive(pid) {
		t.Fatal("Stop 之后进程仍在运行")
	}
}

func freeUDPPort(t *testing.T) int {
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}
