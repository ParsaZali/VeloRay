package panel

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTrustedProxyIPCannotBeSpoofed(t *testing.T) {
	prefixes, err := parseTrustedProxies("127.0.0.1/32,::1/128,10.10.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	s := Server{Config: Config{TrustedProxies: prefixes}}
	for _, test := range []struct{ peer, forwarded, want string }{
		{"127.0.0.1:44100", "198.51.100.9", "198.51.100.9"},
		{"[::1]:44100", "2001:db8::9", "2001:db8::9"},
		{"127.0.0.1:44100", "192.0.2.66, 198.51.100.9", "198.51.100.9"},
		{"127.0.0.1:44100", "198.51.100.9, 10.10.0.2", "198.51.100.9"},
		{"198.51.100.9:44100", "192.0.2.66", "198.51.100.9"},
		{"127.0.0.1:44100", "unknown, 198.51.100.9", "127.0.0.1"},
		{"127.0.0.1:44100", "198.51.100.9:1234", "127.0.0.1"},
		{"127.0.0.1:44100", "[2001:db8::9]", "127.0.0.1"},
		{"127.0.0.1:44100", strings.Repeat("198.51.100.9,", 17), "127.0.0.1"},
	} {
		r := httptest.NewRequest("GET", "/api/health", nil)
		r.RemoteAddr = test.peer
		r.Header.Set("X-Forwarded-For", test.forwarded)
		if got := clientIP(s.withProxyIP(r)); got != test.want {
			t.Fatalf("%s %s: got %s want %s", test.peer, test.forwarded, got, test.want)
		}
		if r.RemoteAddr != test.peer {
			t.Fatal("mutated socket peer")
		}
	}
	for _, bad := range []string{"0.0.0.0/0", "::/0", "localhost", "127.0.0.1", "10.0.0.0/8,"} {
		if _, err = parseTrustedProxies(bad); err == nil {
			t.Fatal("accepted invalid proxy CIDR", bad)
		}
	}
	if p, err := parseTrustedProxies(""); err != nil || len(p) != 0 {
		t.Fatal("cannot disable trust")
	}
}
