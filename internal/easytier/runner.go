package easytier

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Peer 对应 `easytier-cli -o json peer` 的一行。
type Peer struct {
	ID          string `json:"id"`
	IPv4        string `json:"ipv4"`
	Hostname    string `json:"hostname"`
	Cost        string `json:"cost"` // "Local"、"p2p"、"relay(n)"
	LatencyMs   string `json:"lat_ms"`
	LossRate    string `json:"loss_rate"`
	RxBytes     string `json:"rx_bytes"`
	TxBytes     string `json:"tx_bytes"`
	TunnelProto string `json:"tunnel_proto"`
	NATType     string `json:"nat_type"`
	Version     string `json:"version"`
}

// Runner 管理一个 easytier-core 子进程，同一时间只跑一个。
type Runner struct {
	dir string

	mu      sync.Mutex
	cmd     *exec.Cmd
	rpcPort int
	done    chan struct{}
	exitErr error
	logs    *ringLog
}

// FindDir 查找 easytier-core 所在目录：安装后在程序旁的 easytier/，开发时在 bin/easytier/。
func FindDir() (string, error) {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "easytier"))
	}
	candidates = append(candidates, filepath.Join("bin", "easytier"))
	for _, d := range candidates {
		if _, err := os.Stat(filepath.Join(d, binName("easytier-core"))); err == nil {
			return filepath.Abs(d)
		}
	}
	return "", fmt.Errorf("找不到 %s，开发环境请先运行 wails3 task easytier:fetch", binName("easytier-core"))
}

func NewRunner(dir string) *Runner {
	return &Runner{dir: dir, logs: newRingLog(500)}
}

// Start 启动 easytier-core。已在运行时返回错误。
func (r *Runner) Start(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd != nil {
		return errors.New("已经在连接中，请先断开")
	}

	port, err := freeTCPPort()
	if err != nil {
		return fmt.Errorf("分配 RPC 端口失败: %w", err)
	}

	cmd := exec.Command(filepath.Join(r.dir, binName("easytier-core")), cfg.Args(port)...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), cfg.Env()...)
	configureCmd(cmd)

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	go r.collectLogs(pr)

	r.logs.add("启动 easytier-core: " + cfg.Redacted())
	if err := cmd.Start(); err != nil {
		pw.Close()
		return fmt.Errorf("启动 easytier-core 失败: %w", err)
	}
	if err := attachProcess(cmd.Process); err != nil {
		// 只影响崩溃时的兜底清理，不阻止连接
		r.logs.add("警告: 无法把子进程加入作业对象: " + err.Error())
	}

	done := make(chan struct{})
	r.cmd, r.rpcPort, r.done, r.exitErr = cmd, port, done, nil
	go func() {
		err := cmd.Wait()
		pw.Close()
		r.mu.Lock()
		if r.cmd == cmd {
			r.cmd = nil
			r.exitErr = err
		}
		r.mu.Unlock()
		if err != nil {
			r.logs.add("easytier-core 已退出: " + err.Error())
		} else {
			r.logs.add("easytier-core 已退出")
		}
		close(done)
	}()
	return nil
}

// Stop 结束子进程并等待退出。未运行时直接返回。
func (r *Runner) Stop() error {
	r.mu.Lock()
	cmd, done := r.cmd, r.done
	r.cmd = nil
	r.mu.Unlock()
	if cmd == nil {
		return nil
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("等待 easytier-core 退出超时")
	}
}

// Running 返回子进程是否在运行；已退出时第二个返回值是退出原因。
func (r *Runner) Running() (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cmd != nil, r.exitErr
}

// Peers 通过 easytier-cli 读取当前 peer 表。
func (r *Runner) Peers(ctx context.Context) ([]Peer, error) {
	r.mu.Lock()
	running, port := r.cmd != nil, r.rpcPort
	r.mu.Unlock()
	if !running {
		return nil, errors.New("未连接")
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(r.dir, binName("easytier-cli")),
		"-p", fmt.Sprintf("127.0.0.1:%d", port), "-o", "json", "peer")
	configureCmd(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("读取 peer 表失败: %w", err)
	}
	var peers []Peer
	if err := json.Unmarshal(out, &peers); err != nil {
		return nil, fmt.Errorf("解析 peer 表失败: %w", err)
	}
	return peers, nil
}

// Logs 返回最近的子进程输出。
func (r *Runner) Logs() []string { return r.logs.snapshot() }

func (r *Runner) collectLogs(rd io.Reader) {
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		r.logs.add(sc.Text())
	}
	if err := sc.Err(); err != nil {
		r.logs.add("读取 easytier-core 输出失败: " + err.Error())
		io.Copy(io.Discard, rd) // 继续消费，避免子进程写管道阻塞
	}
}

func freeTCPPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

type ringLog struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func newRingLog(max int) *ringLog { return &ringLog{max: max} }

func (l *ringLog) add(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, s)
	if len(l.lines) > l.max {
		l.lines = l.lines[len(l.lines)-l.max:]
	}
}

func (l *ringLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}
