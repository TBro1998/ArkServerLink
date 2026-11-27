package deeplink

import "testing"

func TestParseJoin(t *testing.T) {
	ok := map[string]int64{
		"arkfgf-link://join/12":    12,
		"arkfgf-link://join/12/":   12,
		"ARKFGF-LINK://Join/7":     7,
		`"arkfgf-link://join/99"`:  99,
		"arkfgf-link:join/5":       5,
		"arkfgf-link://join/3?x=1": 3,
	}
	for in, want := range ok {
		if got, valid := ParseJoin(in); !valid || got != want {
			t.Errorf("%q: got %d %v", in, got, valid)
		}
	}
	for _, in := range []string{
		"", "arkfgf-link://", "arkfgf-link://join/", "arkfgf-link://join/abc", "arkfgf-link://join/-1",
		"arkfgf-link://join/0", "arkfgf-link://leave/12", "arkfgf-link://join/12/extra", "https://arkfgf.com/rooms/12",
		`C:\Program Files\app.exe`, "arkfgf-link://join/99999999999999999999",
	} {
		if id, valid := ParseJoin(in); valid {
			t.Errorf("%q 不应合法，得到 %d", in, id)
		}
	}
}

func TestFindJoin(t *testing.T) {
	if id, ok := FindJoin([]string{`C:\app.exe`, "arkfgf-link://join/42"}); !ok || id != 42 {
		t.Errorf("got %d %v", id, ok)
	}
	if _, ok := FindJoin([]string{`C:\app.exe`}); ok {
		t.Error("没有链接时应返回 false")
	}
}

func TestParsePortal(t *testing.T) {
	ok := map[string]string{
		"arkfgf-link://portal?url=https%3A%2F%2Fark.example.com":       "https://ark.example.com",
		"arkfgf-link://portal/?url=http%3A%2F%2F1.2.3.4%3A8090":        "http://1.2.3.4:8090",
		`"arkfgf-link://portal?url=https%3A%2F%2Fark.example.com%2Fa"`: "https://ark.example.com/a",
	}
	for in, want := range ok {
		if got, valid := ParsePortal(in); !valid || got != want {
			t.Errorf("ParsePortal(%q) = %q, %v", in, got, valid)
		}
	}
	for _, bad := range []string{
		"arkfgf-link://portal?url=javascript%3Aalert(1)",
		"arkfgf-link://portal?url=file%3A%2F%2F%2Fc%3A%2F",
		"arkfgf-link://portal",
		"arkfgf-link://join/12",
		"https://portal?url=https%3A%2F%2Fx.com",
	} {
		if _, valid := ParsePortal(bad); valid {
			t.Errorf("ParsePortal(%q) 应当无效", bad)
		}
	}
	if u, ok := FindPortal([]string{"--x", "arkfgf-link://portal?url=https%3A%2F%2Fa.com"}); !ok || u != "https://a.com" {
		t.Errorf("FindPortal = %q %v", u, ok)
	}
}
