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
