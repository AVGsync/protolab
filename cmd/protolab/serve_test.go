package main

import (
	"net"
	"testing"
)

// The announced address (log, TUI, --open) must be the one the server listens
// on: a server bound to [::1] or 127.0.0.2 is not reachable as 127.0.0.1.
func TestServerURL(t *testing.T) {
	for _, tc := range []struct {
		ip   string
		want string
	}{
		{"127.0.0.1", "http://127.0.0.1:7420"},
		{"::1", "http://[::1]:7420"},
		{"127.0.0.2", "http://127.0.0.2:7420"},
		{"0.0.0.0", "http://127.0.0.1:7420"},
		{"::", "http://127.0.0.1:7420"},
		{"192.168.1.5", "http://192.168.1.5:7420"},
	} {
		if got := serverURL(&net.TCPAddr{IP: net.ParseIP(tc.ip), Port: 7420}); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.ip, got, tc.want)
		}
	}
}
