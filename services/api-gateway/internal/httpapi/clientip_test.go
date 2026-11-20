package httpapi

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIP(t *testing.T) {
	a := &API{opts: Options{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("10.0.0.0/8")}}}
	for _, c := range []struct {
		name, peer, xff, want string
	}{
		{"no proxy: the peer", "203.0.113.7:5000", "", "203.0.113.7"},
		{"an untrusted peer cannot claim an address", "203.0.113.7:5000", "198.51.100.1", "203.0.113.7"},
		{"behind nginx", "172.20.0.22:4000", "198.51.100.1", "198.51.100.1"},
		{"a forged entry on the left is ignored", "172.20.0.22:4000", "6.6.6.6, 198.51.100.1", "198.51.100.1"},
		{"two trusted hops", "172.20.0.22:4000", "198.51.100.1, 10.1.2.3", "198.51.100.1"},
		{"a trusted peer without the header", "172.20.0.22:4000", "", "172.20.0.22"},
		{"garbage stops the walk", "172.20.0.22:4000", "198.51.100.1, nonsense", "172.20.0.22"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.peer
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := a.clientIP(r); got != c.want {
			t.Errorf("%s: clientIP = %s, want %s", c.name, got, c.want)
		}
	}
}
