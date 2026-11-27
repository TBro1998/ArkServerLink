package portal

import (
	"context"
	"errors"
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
