package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConsoleVisitorAddress(t *testing.T) {
	p := NewConsoleProxies([]string{"10.9.0.0/16"})
	p.local = func(ip net.IP) bool { return ip.IsLoopback() || ip.Equal(net.ParseIP("172.20.0.5")) }
	s := &Server{Proxies: p}
	var seen string
	h := s.realIP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = clientIP(r) }))

	cases := []struct {
		name, peer, xff, want string
	}{
		{"direct, header ignored", "192.168.1.50:5000", "1.2.3.4", "192.168.1.50"},
		{"own gateway over loopback", "127.0.0.1:4000", "203.0.113.7", "203.0.113.7"},
		{"own gateway via container address", "172.20.0.5:4000", "203.0.113.8", "203.0.113.8"},
		{"trusted proxy, spoofed left entry skipped", "10.9.1.1:80", "6.6.6.6, 203.0.113.9", "203.0.113.9"},
		{"chain of trusted proxies", "10.9.1.1:80", "203.0.113.10, 10.9.2.2", "203.0.113.10"},
		{"trusted peer without header", "10.9.1.1:80", "", "10.9.1.1"},
		{"malformed entry", "127.0.0.1:4000", "junk", "127.0.0.1"},
		{"ipv6 visitor", "[::1]:4000", "2001:db8::1", "2001:db8::1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/api/status", nil)
		r.RemoteAddr = c.peer
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		h.ServeHTTP(httptest.NewRecorder(), r)
		if seen != c.want {
			t.Errorf("%s: got %q, want %q", c.name, seen, c.want)
		}
	}
}

// Behind the gateway, the admin allowlist checks the visitor, not the loopback peer.
func TestAdminAllowSeesVisitorBehindGateway(t *testing.T) {
	e := newTestEnv(t)
	e.s.Proxies = NewConsoleProxies(nil)
	ctx := context.Background()
	n, _ := e.s.Store.Network(ctx)
	n.AdminAllow = []string{"192.0.2.0/24"}
	if err := e.s.Store.SetNetwork(ctx, n); err != nil {
		t.Fatal(err)
	}
	h := e.s.Handler()
	for _, c := range []struct {
		xff  string
		want int
	}{{"198.51.100.1", 403}, {"192.0.2.10", 200}} {
		r := httptest.NewRequest("GET", "/api/status", nil)
		r.RemoteAddr = "127.0.0.1:4000"
		r.Header.Set("X-Forwarded-For", c.xff)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("visitor %s: %d, want %d", c.xff, w.Code, c.want)
		}
	}
}
