package steam

import "testing"

func TestConnectURL(t *testing.T) {
	got := ConnectURL("127.0.0.1:7777")
	want := "steam://run/346110//+connect%20127.0.0.1:7777"
	if got != want {
		t.Fatalf("ConnectURL = %q, want %q", got, want)
	}
}
