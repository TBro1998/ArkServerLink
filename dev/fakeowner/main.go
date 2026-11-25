// fakeowner 在本地联调时代替 Commander：向网站登记一个联机房，把 dev/docker-compose.yml 里的
// 服主节点 srv 按房间的 network_name 重建，然后每 10 秒心跳一次——按 want_credentials 用 srv 的
// easytier-cli 签发凭证上传，按 revoke 吊销并回报 revoked_ack。
//
// 用法（在 ArkServerLink 目录下）：
//
//	go run ./dev/fakeowner -api http://127.0.0.1:8080/api/v1
//
// 房间 ID 和令牌保存在 dev/bin/fakeowner.json，重启后继续用同一个房间。Ctrl+C 退出时发送
// online:false 心跳；加 -delete 则下架房间。
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

const credTTL = 30 * 24 * time.Hour // 与 Commander 一致：30 天

var (
	apiBase  = flag.String("api", envOr("ARKFGF_LINK_API", envOr("ARKFGF_API", "http://127.0.0.1:8080/api/v1")), "联机大厅接口（/link/...）的根地址，默认取 ARKFGF_LINK_API，没有时取 ARKFGF_API")
	compose  = flag.String("compose", filepath.Join("dev", "docker-compose.yml"), "docker compose 文件")
	name     = flag.String("name", "本地联调房间", "房间名")
	stateArg = flag.String("state", filepath.Join("dev", "bin", "fakeowner.json"), "保存房间 ID 和令牌的文件")
	interval = flag.Duration("interval", 10*time.Second, "心跳间隔")
	players  = flag.Int("players", 1, "心跳上报的在线人数")
	maxBatch = flag.Int("batch", 20, "每轮最多签发的凭证数")
	del      = flag.Bool("delete", false, "退出时下架房间（DELETE）")
	noDocker = flag.Bool("no-docker", false, "不操作 docker（只测网站接口，凭证用随机值）")
)

type state struct {
	ID          int64  `json:"id"`
	Token       string `json:"token"`
	NetworkName string `json:"network_name"`
}

type credential struct {
	ID        string `json:"id"`
	Secret    string `json:"secret"`
	ExpiresAt int64  `json:"expires_at"`
}

func main() {
	flag.Parse()
	log.SetFlags(log.Ltime)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	st := loadState()
	if err := register(ctx, &st); err != nil {
		log.Fatalf("登记房间失败: %v", err)
	}
	if err := ensureServer(ctx, st.NetworkName); err != nil {
		log.Fatalf("启动服主节点失败: %v", err)
	}

	var pending []credential // 已签发、还没上传成功的凭证
	var acks []string        // 已吊销、还没回报的凭证
	for {
		resp, err := heartbeat(ctx, st, true, pending, acks)
		switch {
		case err == nil:
			if len(pending) > 0 || len(acks) > 0 {
				log.Printf("已上传 %d 个凭证，回报吊销 %d 个", len(pending), len(acks))
			}
			pending, acks = nil, nil
		case errors.Is(err, errInvalidToken):
			log.Printf("令牌失效，重新登记")
			st = state{}
			pending, acks = nil, nil
			if err := register(ctx, &st); err != nil {
				log.Printf("重新登记失败: %v", err)
			} else if err := ensureServer(ctx, st.NetworkName); err != nil {
				log.Printf("重建服主节点失败: %v", err)
			}
		case ctx.Err() != nil:
		default:
			log.Printf("心跳失败: %v", err)
		}

		if resp != nil {
			if resp.NetworkName != "" && resp.NetworkName != st.NetworkName {
				log.Printf("网站返回的网络名变了: %s → %s", st.NetworkName, resp.NetworkName)
				st.NetworkName = resp.NetworkName
				saveState(st)
				if err := ensureServer(ctx, st.NetworkName); err != nil {
					log.Printf("重建服主节点失败: %v", err)
				}
			}
			for _, id := range resp.Revoke {
				if err := revoke(ctx, id); err != nil {
					log.Printf("吊销 %s 失败: %v", id, err)
					continue
				}
				log.Printf("已吊销 %s", id)
				acks = append(acks, id)
			}
			for i := 0; i < min(resp.WantCredentials, *maxBatch); i++ {
				c, err := generate(ctx)
				if err != nil {
					log.Printf("签发凭证失败: %v", err)
					break
				}
				pending = append(pending, c)
			}
		}

		// 有待上传的内容就马上再发一次，否则等下一轮
		wait := *interval
		if len(pending) > 0 || len(acks) > 0 {
			wait = 200 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			shutdown(st)
			return
		case <-time.After(wait):
		}
	}
}

func shutdown(st state) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if *del {
		if err := call(ctx, http.MethodDelete, fmt.Sprintf("/link/rooms/%d", st.ID), st.Token, nil, nil); err != nil {
			log.Printf("下架失败: %v", err)
		} else {
			log.Printf("房间 %d 已下架", st.ID)
			os.Remove(*stateArg)
		}
		return
	}
	if _, err := heartbeat(ctx, st, false, nil, nil); err != nil {
		log.Printf("发送离线心跳失败: %v", err)
	} else {
		log.Printf("已发送离线心跳")
	}
}

func register(ctx context.Context, st *state) error {
	body := map[string]any{
		"name": *name, "description": "fakeowner 登记的本地联调房间，服务器是 udpecho。", "tags": []string{"联调"},
		"qq_group": "", "discord": "", "server_ip": "10.126.126.1",
		"servers": []map[string]any{{
			"key": "1", "name": *name, "map": "TheIsland", "max_players": 70, "password": false, "pvp": false,
			"arkapi": false, "cluster": "", "mods": []string{"731604991"}, "version": "358.24",
			"rate_xp": 2, "rate_tame": 3, "rate_harvest": 2, "rate_breeding": 5,
			"game_port": 7777, "query_port": 27015,
		}},
	}
	var resp struct {
		ID          int64    `json:"id"`
		Token       string   `json:"token"`
		NetworkName string   `json:"network_name"`
		Peers       []string `json:"peers"`
		URL         string   `json:"url"`
	}
	err := call(ctx, http.MethodPost, "/link/rooms", st.Token, body, &resp)
	if errors.Is(err, errInvalidToken) && st.Token != "" {
		*st = state{}
		err = call(ctx, http.MethodPost, "/link/rooms", "", body, &resp)
	}
	if err != nil {
		return err
	}
	if resp.Token != "" {
		st.Token = resp.Token
	}
	st.ID, st.NetworkName = resp.ID, resp.NetworkName
	saveState(*st)
	log.Printf("房间 #%d 已登记: network_name=%s peers=%v url=%s", st.ID, st.NetworkName, resp.Peers, resp.URL)
	log.Printf("客户端加入链接: arkfgf-link://join/%d", st.ID)
	return nil
}

type heartbeatResp struct {
	WantCredentials int      `json:"want_credentials"`
	Revoke          []string `json:"revoke"`
	NetworkName     string   `json:"network_name"`
	Peers           []string `json:"peers"`
}

func heartbeat(ctx context.Context, st state, online bool, creds []credential, acks []string) (*heartbeatResp, error) {
	n := 0
	if online {
		n = *players
	}
	body := map[string]any{
		"online":      online,
		"servers":     []map[string]any{{"key": "1", "online": online, "players": n}},
		"credentials": nonNil(creds), "revoked_ack": nonNil(acks),
	}
	var resp heartbeatResp
	if err := call(ctx, http.MethodPost, fmt.Sprintf("/link/rooms/%d/heartbeat", st.ID), st.Token, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

var errInvalidToken = errors.New("房间令牌无效")

func call(ctx context.Context, method, path, token string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(*apiBase, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ArkServerLink-fakeowner/dev")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		return errInvalidToken
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: %d %s (Retry-After=%q)", method, path, resp.StatusCode, strings.TrimSpace(string(data)), resp.Header.Get("Retry-After"))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// ensureServer 用房间的网络名启动（或重建）srv；compose 发现 ROOM_NETWORK 变了会自动重建容器。
func ensureServer(ctx context.Context, network string) error {
	if *noDocker {
		return nil
	}
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", *compose, "up", "-d", "--wait", "srv")
	cmd.Env = append(os.Environ(), "ROOM_NETWORK="+network)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	log.Printf("服主节点 srv 已按网络 %s 运行", network)
	// 等 RPC 就绪，否则第一次签发会失败
	for i := 0; i < 20; i++ {
		if _, err := srvCLI(ctx, "-o", "json", "credential", "list"); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("等待 srv 的 easytier RPC 就绪超时")
}

func srvCLI(ctx context.Context, args ...string) ([]byte, error) {
	full := append([]string{"compose", "-f", *compose, "exec", "-T", "srv", "easytier-cli"}, args...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func generate(ctx context.Context) (credential, error) {
	id := uuid()
	expires := time.Now().Add(credTTL).Unix()
	if *noDocker {
		b := make([]byte, 32)
		rand.Read(b)
		return credential{ID: id, Secret: base64.StdEncoding.EncodeToString(b), ExpiresAt: expires}, nil
	}
	out, err := srvCLI(ctx, "-o", "json", "credential", "generate",
		"--ttl", fmt.Sprint(int(credTTL.Seconds())), "--credential-id", id, "--reusable", "false")
	if err != nil {
		return credential{}, err
	}
	var res struct {
		ID     string `json:"credential_id"`
		Secret string `json:"credential_secret"`
	}
	if err := json.Unmarshal(out, &res); err != nil || res.Secret == "" {
		return credential{}, fmt.Errorf("无法解析 easytier-cli 输出: %s", out)
	}
	return credential{ID: res.ID, Secret: res.Secret, ExpiresAt: expires}, nil
}

func revoke(ctx context.Context, id string) error {
	if *noDocker {
		return nil
	}
	// 凭证不存在时 success=false，也算吊销完成
	_, err := srvCLI(ctx, "-o", "json", "credential", "revoke", id)
	return err
}

func uuid() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func loadState() state {
	var st state
	if b, err := os.ReadFile(*stateArg); err == nil {
		json.Unmarshal(b, &st)
	}
	return st
}

func saveState(st state) {
	os.MkdirAll(filepath.Dir(*stateArg), 0o755)
	b, _ := json.MarshalIndent(st, "", "  ")
	if err := os.WriteFile(*stateArg, b, 0o600); err != nil {
		log.Printf("保存 %s 失败: %v", *stateArg, err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
