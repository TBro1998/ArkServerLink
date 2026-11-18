package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewState(t *testing.T) {
	re := regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)
	a, _ := NewState()
	b, _ := NewState()
	if !re.MatchString(a) || a == b {
		t.Errorf("state 不合约定或不随机: %q %q", a, b)
	}
}

// fakeBrowser 模拟浏览器：从登录地址取出 client_port 和 state，按 rewrite 修改后请求回调。
func fakeBrowser(t *testing.T, calls func(port, state string) []string, got chan<- []string) func(string) error {
	return func(loginURL string) error {
		u, err := url.Parse(loginURL)
		if err != nil {
			return err
		}
		port, state := u.Query().Get("client_port"), u.Query().Get("state")
		go func() {
			var bodies []string
			for _, cb := range calls(port, state) {
				resp, err := http.Get(cb)
				if err != nil {
					bodies = append(bodies, "ERR "+err.Error())
					continue
				}
				b, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				bodies = append(bodies, resp.Status+" "+string(b))
			}
			got <- bodies
		}()
		return nil
	}
}

func loginURL(port int, state string) string {
	return "https://arkfgf.com/api/v1/auth/steam/login?" + url.Values{"client_port": {strconv.Itoa(port)}, "state": {state}}.Encode()
}

func TestLoopbackLogin(t *testing.T) {
	got := make(chan []string, 1)
	open := fakeBrowser(t, func(port, state string) []string {
		base := "http://127.0.0.1:" + port + "/callback?"
		return []string{
			// 伪造的回调（state 不对）必须被忽略
			base + url.Values{"token": {"attacker"}, "state": {"wrong-state-0000000"}}.Encode(),
			base + url.Values{"token": {"good.jwt.token"}, "state": {state}}.Encode(),
		}
	}, got)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tok, err := LoopbackLogin(ctx, loginURL, open)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "good.jwt.token" {
		t.Errorf("token = %q", tok)
	}
	bodies := <-got
	if !strings.HasPrefix(bodies[0], "400") || !strings.HasPrefix(bodies[1], "200") || !strings.Contains(bodies[1], "登录成功") {
		t.Errorf("回调页面: %v", bodies)
	}
}

func TestLoopbackLoginRejectsWrongStateAndTimesOut(t *testing.T) {
	got := make(chan []string, 1)
	open := fakeBrowser(t, func(port, state string) []string {
		return []string{"http://127.0.0.1:" + port + "/callback?token=x&state=" + state + "x"}
	}, got)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := LoopbackLogin(ctx, loginURL, open)
	if !errors.Is(err, ErrCanceled) {
		t.Errorf("err = %v", err)
	}
	if b := <-got; !strings.HasPrefix(b[0], "400") {
		t.Errorf("错误 state 应返回 400: %v", b)
	}
}

func TestLoopbackLoginListensOnLoopbackOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := LoopbackLogin(ctx, loginURL, func(u string) error {
		parsed, _ := url.Parse(u)
		p := parsed.Query().Get("client_port")
		if p == "" || p == "0" {
			t.Errorf("client_port = %q", p)
		}
		cancel()
		return nil
	})
	if !errors.Is(err, ErrCanceled) {
		t.Errorf("err = %v", err)
	}
}

func TestLoopbackLoginOpenFails(t *testing.T) {
	_, err := LoopbackLogin(context.Background(), loginURL, func(string) error { return errors.New("no browser") })
	if err == nil || !strings.Contains(err.Error(), "浏览器") {
		t.Errorf("err = %v", err)
	}
}

func TestParseClaims(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"uid":3,"name":"Rex","exp":1797600000}`))
	c, ok := ParseClaims("eyJhbGciOiJIUzI1NiJ9." + payload + ".sig")
	if !ok || c.Name != "Rex" || c.ExpiresAt.Unix() != 1797600000 {
		t.Errorf("claims = %+v ok=%v", c, ok)
	}
	if _, ok := ParseClaims("garbage"); ok {
		t.Error("非法令牌不应解析成功")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "token")
	s := NewStore(path)
	if tok, err := s.Load(); err != nil || tok != "" {
		t.Fatalf("空存储: %q %v", tok, err)
	}
	if err := s.Save("header.payload.signature"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if runtime.GOOS == "windows" && bytes.Contains(raw, []byte("payload")) {
		t.Error("Windows 上令牌应被 DPAPI 加密")
	}
	if tok, err := s.Load(); err != nil || tok != "header.payload.signature" {
		t.Fatalf("读回: %q %v", tok, err)
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if tok, _ := s.Load(); tok != "" {
		t.Errorf("清除后仍有令牌: %q", tok)
	}
	if err := s.Clear(); err != nil {
		t.Errorf("重复清除: %v", err)
	}
}

func TestStoreCorruptFile(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("只有 Windows 会校验密文")
	}
	path := filepath.Join(t.TempDir(), "token")
	os.WriteFile(path, []byte("not dpapi"), 0o600)
	if tok, err := NewStore(path).Load(); err != nil || tok != "" {
		t.Errorf("损坏的文件应视为未登录: %q %v", tok, err)
	}
}
