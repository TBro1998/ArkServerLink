package portal

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeBase(t *testing.T) {
	for in, want := range map[string]string{
		"http://127.0.0.1:8090/":           "http://127.0.0.1:8090",
		" https://ark.example.com/a/?x=1 ": "https://ark.example.com/a",
	} {
		if got, err := NormalizeBase(in); err != nil || got != want {
			t.Errorf("NormalizeBase(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"javascript:alert(1)", "file:///c:/x", "ftp://x", "http://", "https://u:p@x.com"} {
		if _, err := NormalizeBase(bad); err == nil {
			t.Errorf("NormalizeBase(%q) 应当报错", bad)
		}
	}
}

func TestClient(t *testing.T) {
	hits := map[string]int{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		switch r.URL.Path {
		case "/api/v1/portal":
			w.Header().Set("ETag", `"v1"`)
			if r.Header.Get("If-None-Match") == `"v1"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			_, _ = w.Write([]byte(`{"site":{"name":"x"}}`))
		case "/api/v1/shops/3":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case "/api/v1/shops/4":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"这台服务器没有商店","code":"shop_not_found"}`))
		case "/big/api/v1/portal":
			_, _ = w.Write([]byte(`"` + strings.Repeat("a", maxBody) + `"`))
		case "/html/api/v1/portal":
			_, _ = w.Write([]byte(`<html>`))
		case "/redirect/api/v1/portal":
			http.Redirect(w, r, "http://example.com/", http.StatusFound)
		}
	}))
	defer ts.Close()
	c := New("test")
	ctx := context.Background()

	body, err := c.Portal(ctx, ts.URL+"/")
	if err != nil || body != `{"site":{"name":"x"}}` {
		t.Fatalf("portal: %q %v", body, err)
	}
	// 第二次带 If-None-Match，304 时用缓存
	if body, err := c.Portal(ctx, ts.URL); err != nil || body != `{"site":{"name":"x"}}` || hits["/api/v1/portal"] != 2 {
		t.Fatalf("304: %q %v %d", body, err, hits["/api/v1/portal"])
	}
	if body, err := c.Shop(ctx, ts.URL, "3"); err != nil || body != `{"items":[]}` {
		t.Fatalf("shop: %q %v", body, err)
	}
	if _, err := c.Shop(ctx, ts.URL, "4"); !errors.Is(err, ErrNoShop) {
		t.Fatalf("no shop: %v", err)
	}
	if _, err := c.Shop(ctx, ts.URL, "../x"); !errors.Is(err, ErrNoShop) {
		t.Fatalf("bad key: %v", err)
	}
	for _, p := range []string{"/big", "/html", "/redirect"} {
		if _, err := c.Portal(ctx, ts.URL+p); err == nil {
			t.Errorf("%s 应当报错", p)
		}
	}
}

// fakePortal 模拟玩家站的登录接口
func fakePortal(t *testing.T, priv ed25519.PrivateKey, logins *int) *httptest.Server {
	t.Helper()
	pub := priv.Public().(ed25519.PublicKey)
	sum := sha256.Sum256(pub)
	audience := hex.EncodeToString(sum[:])
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/identity":
			nonce := r.URL.Query().Get("nonce")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"public_key": base64.RawURLEncoding.EncodeToString(pub),
				"audience":   audience,
				"signature":  base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(identityContext+nonce))),
			})
		case "/api/v1/login":
			var in struct{ Token string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.Token != "tok-for-"+audience {
				w.WriteHeader(401)
				_, _ = w.Write([]byte(`{"error":"令牌不是给这个玩家站的","code":"invalid_token"}`))
				return
			}
			*logins++
			_, _ = w.Write([]byte(fmt.Sprintf(`{"session":"s%d"}`, *logins)))
		case "/api/v1/me":
			// 第一次登录的会话 s1 当作已过期，s2 有效
			if r.Header.Get("Authorization") == "Bearer s2" {
				_, _ = w.Write([]byte(`{"user":{"steam_id":"76561198000000001"},"data":{"3":{"points":120}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"user":null,"link_login":true}`))
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestMeLogsInWithArkfgfToken(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	seed[1] = 3
	priv := ed25519.NewKeyFromSeed(seed)
	logins := 0
	ts := fakePortal(t, priv, &logins)
	c := New("test")
	var audiences []string
	issue := func(_ context.Context, audience string) (string, error) {
		audiences = append(audiences, audience)
		return "tok-for-" + audience, nil
	}

	// 没登录 arkfgf.com：不登录玩家站
	if body, err := c.Me(context.Background(), ts.URL, nil); err != nil || !strings.Contains(body, `"user":null`) || logins != 0 {
		t.Fatalf("not logged in: %s %v %d", body, err, logins)
	}
	// 登录后第一个会话无效，自动重新登录一次
	body, err := c.Me(context.Background(), ts.URL, issue)
	if err != nil || !strings.Contains(body, `"points":120`) || logins != 2 || len(audiences) != 2 {
		t.Fatalf("me: %s %v logins=%d", body, err, logins)
	}
	// 会话有缓存
	if _, err := c.Me(context.Background(), ts.URL, issue); err != nil || logins != 2 {
		t.Fatalf("session should be cached: %v %d", err, logins)
	}
	c.Forget()
	if len(c.sessions) != 0 {
		t.Fatal("Forget should clear sessions")
	}
}

func TestIdentityMustMatch(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	priv := ed25519.NewKeyFromSeed(seed)
	other := ed25519.NewKeyFromSeed(append([]byte{9}, seed[1:]...))
	// 玩家站声称的公钥和签名用的私钥不一致
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pub := priv.Public().(ed25519.PublicKey)
		sum := sha256.Sum256(pub)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"public_key": base64.RawURLEncoding.EncodeToString(pub),
			"audience":   hex.EncodeToString(sum[:]),
			"signature":  base64.RawURLEncoding.EncodeToString(ed25519.Sign(other, []byte(identityContext+r.URL.Query().Get("nonce")))),
		})
	}))
	defer ts.Close()
	called := false
	_, err := New("test").Me(context.Background(), ts.URL, func(context.Context, string) (string, error) {
		called = true
		return "x", nil
	})
	if err == nil || called {
		t.Fatalf("签名不对时不应当去 arkfgf.com 要令牌: %v %v", err, called)
	}
}
