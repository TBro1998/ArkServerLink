package arkfgf

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL+"/api/v1/", "1.2.3")
}

func TestListRooms(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/link/rooms" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "孤岛" {
			t.Errorf("q = %q", got)
		}
		if got := r.URL.Query().Get("page"); got != "2" {
			t.Errorf("page = %q", got)
		}
		if ua := r.Header.Get("User-Agent"); ua != "ArkServerLink/1.2.3" {
			t.Errorf("User-Agent = %q", ua)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("公开接口不应带令牌")
		}
		w.Write([]byte(`{"items":[{"id":12,"name":"孤岛","map":"TheIsland","players":3,"max_players":70,
			"password":true,"pvp":false,"mods":["731604991"],"tags":["新手"],"rates":{"xp":2,"tame":3,"harvest":1,"breeding":1},
			"online":true,"last_seen":"2026-11-18T10:00:00Z","created_at":1797600000,"country":"CN","province":"浙江"}],
			"total":1,"page":2,"pages":2,"stats":{"rooms":4,"players":9}}`))
	})
	res, err := c.ListRooms(context.Background(), " 孤岛 ", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Items) != 1 || res.Pages != 2 {
		t.Fatalf("res = %+v", res)
	}
	r := res.Items[0]
	if r.ID != 12 || r.MaxPlayers != 70 || !r.Password || r.Rates.Tame != 3 || r.Mods[0] != "731604991" || r.Province != "浙江" {
		t.Errorf("room = %+v", r)
	}
	if res.Stats == nil || res.Stats.Players != 9 {
		t.Errorf("stats = %+v", res.Stats)
	}
	if r.LastSeen != UnixTime(time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC).Unix()) || r.CreatedAt != 1797600000 {
		t.Errorf("时间解析错误: %d %d", r.LastSeen, r.CreatedAt)
	}
}

func TestGetRoom(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/link/rooms/12":
			w.Write([]byte(`{"id":12,"name":"x","last_seen":"0001-01-01T00:00:00Z","online":false,"mod_list":[{"id":"1","title":"Structures Plus"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"房间不存在","code":"not_found"}`))
		}
	})
	d, err := c.GetRoom(context.Background(), 12)
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != 12 || d.Online || d.LastSeen != 0 || len(d.ModList) != 1 || d.ModList[0].Title != "Structures Plus" {
		t.Errorf("detail = %+v", d)
	}
	_, err = c.GetRoom(context.Background(), 13)
	if ErrorCode(err) != CodeNotFound {
		t.Errorf("err = %v", err)
	}
}

func TestJoin(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/link/rooms/12/join" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer jwt-1" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{"room":{"id":12,"name":"孤岛"},"network_name":"ark-0123456789abcdef","credential":"abc=",
			"credential_expires_at":1797600000,"peers":["tcp://et.arkfgf.com:11010","udp://et.arkfgf.com:11010"],
			"server_ip":"10.126.126.1","servers":[{"key":"3","name":"孤岛 A","map":"TheIsland","online":true,"game_port":7777,"query_port":27015},
			{"key":"5","name":"孤岛 B","map":"Ragnarok","online":false,"game_port":7779,"query_port":27017}]}`))
	})
	res, err := c.Join(context.Background(), "jwt-1", 12)
	if err != nil {
		t.Fatal(err)
	}
	if res.Room.Name != "孤岛" || res.NetworkName != "ark-0123456789abcdef" || len(res.Peers) != 2 ||
		res.ServerIP != "10.126.126.1" || res.CredentialExpiresAt != 1797600000 || len(res.Servers) != 2 {
		t.Errorf("res = %+v", res)
	}
	if s := res.Servers[1]; s.Key != "5" || s.Map != "Ragnarok" || s.Online || s.GamePort != 7779 || s.QueryPort != 27017 {
		t.Errorf("server = %+v", s)
	}
	// 没有 game 字段（旧版网站）按 ASE
	if res.Servers[0].Game != GameASE || res.Servers[1].Game != GameASE {
		t.Errorf("servers = %+v", res.Servers)
	}

	if _, err := c.Join(context.Background(), "", 12); !IsUnauthorized(err) {
		t.Errorf("没有令牌应视为未登录: %v", err)
	}
}

// ASA 服务器：game 为 asa，没有查询端口；房间详情和房间列表里的服务器也带 game
func TestJoinASA(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/link/rooms/12/join":
			w.Write([]byte(`{"room":{"id":12,"name":"飞升"},"network_name":"ark-0123456789abcdef","credential":"abc=",
				"credential_expires_at":1797600000,"peers":["tcp://et.arkfgf.com:11010"],"server_ip":"10.126.126.1",
				"servers":[{"key":"1","name":"A","map":"TheIsland","game":"ase","online":true,"game_port":7777,"query_port":27015},
				{"key":"2","name":"B","map":"TheIsland_WP","game":"asa","online":true,"game_port":7787,"query_port":0},
				{"key":"3","name":"C","map":"ScorchedEarth_WP","game":"ASA","online":true,"game_port":7789,"query_port":0},
				{"key":"4","name":"D","map":"TheIsland","game":"","online":true,"game_port":7791,"query_port":27017}]}`))
		case "/api/v1/link/rooms/12":
			w.Write([]byte(`{"id":12,"name":"飞升","servers":[{"name":"A","map":"TheIsland"},{"name":"B","map":"TheIsland_WP","game":"asa"}]}`))
		case "/api/v1/link/rooms":
			w.Write([]byte(`{"items":[{"id":12,"name":"飞升","servers":[{"name":"A"},{"name":"B","game":"asa"}]}],"total":1,"page":1,"pages":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	res, err := c.Join(context.Background(), "jwt-1", 12)
	if err != nil {
		t.Fatal(err)
	}
	want := []Game{GameASE, GameASA, GameASA, GameASE}
	for i, g := range want {
		if res.Servers[i].Game != g {
			t.Errorf("server %d game = %q, want %q", i, res.Servers[i].Game, g)
		}
	}
	if s := res.Servers[1]; s.GamePort != 7787 || s.QueryPort != 0 {
		t.Errorf("server = %+v", s)
	}

	d, err := c.GetRoom(context.Background(), 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Servers) != 2 || d.Servers[0].Game != GameASE || d.Servers[1].Game != GameASA {
		t.Errorf("servers = %+v", d.Servers)
	}

	l, err := c.ListRooms(context.Background(), "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if s := l.Items[0].Servers; len(s) != 2 || s[0].Game != GameASE || s[1].Game != GameASA {
		t.Errorf("servers = %+v", s)
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		status     int
		header     string
		body       string
		wantCode   string
		wantRetry  time.Duration
		wantMsgHas string
	}{
		{409, "", `{"error":"房间离线","code":"room_offline"}`, CodeRoomOffline, 0, "离线"},
		{403, "", `{"error":"被封禁","code":"banned"}`, CodeBanned, 0, "封禁"},
		{503, "30", `{"error":"凭证不足","code":"no_credentials"}`, CodeNoCredentials, 30 * time.Second, "名额"},
		{401, "", `{"error":"invalid token"}`, CodeUnauthorized, 0, "登录"},
		{429, "12", `{"error":"too many"}`, CodeRateLimited, 12 * time.Second, "12 秒"},
		{400, "", `{"error":"参数不对","code":"bad_request"}`, "bad_request", 0, "参数不对"},
		{502, "", `<html>bad gateway</html>`, "", 0, "暂时不可用"},
	}
	for _, tc := range cases {
		c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if tc.header != "" {
				w.Header().Set("Retry-After", tc.header)
			}
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		})
		_, err := c.Join(context.Background(), "t", 1)
		var ae *APIError
		if !errors.As(err, &ae) {
			t.Fatalf("%d: err = %T %v", tc.status, err, err)
		}
		if ae.Status != tc.status || ae.Code != tc.wantCode || ae.RetryAfter != tc.wantRetry || !strings.Contains(ae.Message, tc.wantMsgHas) {
			t.Errorf("%d: %+v", tc.status, ae)
		}
		if IsUnauthorized(err) != (tc.status == 401) {
			t.Errorf("%d: IsUnauthorized = %v", tc.status, IsUnauthorized(err))
		}
	}
}

func TestNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	_, err := New(srv.URL, "x").ListRooms(context.Background(), "", "", 0)
	var ne *NetworkError
	if !errors.As(err, &ne) || !strings.Contains(err.Error(), "无法连接") {
		t.Errorf("err = %v", err)
	}
}

func TestMeAcceptsBothFieldStyles(t *testing.T) {
	for _, body := range []string{
		`{"steam_id":"76561198000000001","name":"Rex","avatar_url":"https://a/b.jpg"}`,
		`{"ID":3,"SteamID":"76561198000000001","Name":"Rex","AvatarURL":"https://a/b.jpg","CreatedAt":"2026-01-01T00:00:00Z"}`,
	} {
		c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/me" || r.Header.Get("Authorization") != "Bearer tok" {
				t.Errorf("unexpected %s %q", r.URL.Path, r.Header.Get("Authorization"))
			}
			w.Write([]byte(body))
		})
		u, err := c.Me(context.Background(), "tok")
		if err != nil {
			t.Fatal(err)
		}
		if *u != (User{SteamID: "76561198000000001", Name: "Rex", AvatarURL: "https://a/b.jpg"}) {
			t.Errorf("user = %+v", u)
		}
	}
}

func TestSteamLoginURL(t *testing.T) {
	got := New("https://arkfgf.com/api/v1", "x").SteamLoginURL(54321, "abc_DEF-123")
	want := "https://arkfgf.com/api/v1/auth/steam/login?client_port=54321&state=abc_DEF-123"
	if got != want {
		t.Errorf("got %s", got)
	}
}

func TestSteamOpenIDURL(t *testing.T) {
	const openid = "https://steamcommunity.com/openid/login?openid.mode=checkid_setup&openid.return_to=https%3A%2F%2Farkfgf.com%2Fcb"
	cases := []struct {
		name     string
		status   int
		location string
		wantErr  bool
	}{
		{"redirect to steam", http.StatusFound, openid, false},
		{"not a redirect", http.StatusBadRequest, "", true},
		{"other host", http.StatusFound, "https://evil.example/openid/login", true},
		{"plain http", http.StatusFound, "http://steamcommunity.com/openid/login", true},
		{"other path", http.StatusFound, "https://steamcommunity.com/market/", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/auth/steam/login" || r.URL.Query().Get("client_port") != "54321" {
					t.Errorf("unexpected %s", r.URL)
				}
				if tc.location != "" {
					w.Header().Set("Location", tc.location)
				}
				w.WriteHeader(tc.status)
			})
			got, err := c.SteamOpenIDURL(context.Background(), c.SteamLoginURL(54321, "abc_DEF-123"))
			if tc.wantErr {
				if err == nil {
					t.Errorf("want error, got %s", got)
				}
				return
			}
			if err != nil || got != openid {
				t.Errorf("got %q, %v", got, err)
			}
		})
	}
}

func TestBaseURLsFromEnv(t *testing.T) {
	cases := []struct {
		api, link         string
		wantAcct, wantLnk string
	}{
		{"", "", DefaultBaseURL, DefaultLinkBaseURL},
		{"http://127.0.0.1:8080/api/v1/", "", "http://127.0.0.1:8080/api/v1", "http://127.0.0.1:8080/api/v1"},
		{"http://a/api/v1", "http://l/api/v1/", "http://a/api/v1", "http://l/api/v1"},
		{"", "http://l/api/v1", DefaultBaseURL, "http://l/api/v1"},
	}
	for _, tc := range cases {
		t.Setenv("ARKFGF_API", tc.api)
		t.Setenv("ARKFGF_LINK_API", tc.link)
		acct, lnk := BaseURLsFromEnv()
		if acct != tc.wantAcct || lnk != tc.wantLnk {
			t.Errorf("ARKFGF_API=%q ARKFGF_LINK_API=%q: got %q %q", tc.api, tc.link, acct, lnk)
		}
	}
}

// 账号接口和联机大厅接口分别发到各自的地址
func TestSplitBases(t *testing.T) {
	var accountHits, linkHits []string
	acct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accountHits = append(accountHits, r.URL.Path)
		w.Write([]byte(`{"steam_id":"1","name":"a"}`))
	}))
	t.Cleanup(acct.Close)
	link := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		linkHits = append(linkHits, r.URL.Path)
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(link.Close)

	c := NewSplit(acct.URL+"/api/v1", link.URL+"/api/v1/", "x")
	ctx := context.Background()
	if _, err := c.Me(ctx, "tok"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListRooms(ctx, "", "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetRoom(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Join(ctx, "tok", 7); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ClientReleases(ctx); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(accountHits, ","); got != "/api/v1/me" {
		t.Errorf("账号接口收到 %s", got)
	}
	if got := strings.Join(linkHits, ","); got != "/api/v1/link/rooms,/api/v1/link/rooms/7,/api/v1/link/rooms/7/join,/api/v1/link/client/releases" {
		t.Errorf("联机大厅接口收到 %s", got)
	}
	if !strings.HasPrefix(c.SteamLoginURL(1, "s"), acct.URL+"/api/v1/auth/steam/login?") {
		t.Errorf("SteamLoginURL = %s", c.SteamLoginURL(1, "s"))
	}
	if c.BaseURL() != acct.URL+"/api/v1" || c.LinkBaseURL() != link.URL+"/api/v1" {
		t.Errorf("BaseURL = %s, LinkBaseURL = %s", c.BaseURL(), c.LinkBaseURL())
	}
}
