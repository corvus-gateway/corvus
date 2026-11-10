package compiler

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

// Only upstreams that are the admin console itself receive the gateway key; any other
// upstream on the same port number never does.
func TestRenderConsoleKeyOnlyForTheConsole(t *testing.T) {
	ctx := context.Background()
	c, st, _, dir := streamEnv(t)
	c.Hosts = func() map[string]string { return map[string]string{"host.docker.internal": "172.17.0.1"} }
	c.Self = func(h string) bool { return h == "corvus" || h == "10.9.0.2" }
	hosts := []struct {
		host    string
		port    int
		console bool
	}{
		{"corvus", 9443, true},               // 1: container name (documented setup)
		{"127.0.0.1", 9443, true},            // 2: loopback (mapped to the Docker host)
		{"localhost", 9443, true},            // 3
		{"host.docker.internal", 9443, true}, // 4: the Docker host
		{"172.17.0.1", 9443, true},           // 5: the Docker host's address
		{"10.9.0.2", 9443, true},             // 6: this container's address
		{"10.0.0.5", 9443, false},            // 7: another machine on the console port
		{"app", 9443, false},                 // 8: another container on the console port
		{"corvus", 8080, false},              // 9: another port
	}
	for i, h := range hosts {
		ph := &store.ProxyHost{Domains: []string{"c" + string(rune('a'+i)) + ".example.com"}, ForwardScheme: "https",
			ForwardHost: h.host, ForwardPort: h.port, Enabled: true}
		if err := st.SaveProxyHost(ctx, ph); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "v1")
	if err := c.Render(ctx, st, out); err != nil {
		t.Fatal(err)
	}
	for i, h := range hosts {
		b, err := os.ReadFile(filepath.Join(out, "hosts", string(rune('1'+i))+".conf"))
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Contains(string(b), "proxy_set_header X-Corvus-Gateway $corvus_gateway_key;")
		if got != h.console {
			t.Errorf("%s:%d: gateway key sent=%v, want %v", h.host, h.port, got, h.console)
		}
		if !got && !strings.Contains(string(b), `proxy_set_header X-Corvus-Gateway "";`) {
			t.Errorf("%s:%d: client-supplied gateway header not cleared", h.host, h.port)
		}
	}
}

func TestStreamTarget(t *testing.T) {
	p := Paths{StatusPort: 9180, ACMEPort: 9181, RedisPort: 6379, AdminPort: 9443}
	for _, c := range []struct {
		addr string
		port int
		self bool
		bad  bool
	}{
		{"127.0.0.1", 9443, false, true}, {"[::1]", 9443, false, true}, {"0.0.0.0", 9443, false, true},
		{"corvus", 9443, true, true}, {"10.9.0.2", 9443, true, true},
		{"127.0.0.1", 9180, false, true}, // the other internal ports stay refused
		{"172.17.0.1", 9443, false, false}, {"10.0.0.5", 9443, false, false}, {"corvus", 8080, true, false},
	} {
		if got := p.StreamTarget(c.addr, c.port, c.self) != ""; got != c.bad {
			t.Errorf("%s:%d (self=%v) refused=%v, want %v", c.addr, c.port, c.self, got, c.bad)
		}
	}
	// proxy hosts may still publish the console on loopback (the gateway adds its key)
	if why := p.InternalTarget("127.0.0.1", 9443); why != "" {
		t.Errorf("proxy host to the console refused: %s", why)
	}
}

// A stream may not forward to the console on this container.
func TestRenderRefusesStreamToConsole(t *testing.T) {
	ctx := context.Background()
	c, st, _, dir := streamEnv(t)
	c.Self = func(h string) bool { return h == "corvus" }
	s := &store.Stream{IncomingPort: 8091, TCP: true, ForwardHost: "corvus", ForwardPort: 9443, Enabled: true}
	if err := st.SaveStream(ctx, s); err != nil {
		t.Fatal(err)
	}
	// stored before the check existed: left out, the rest of the configuration applies
	if err := c.Render(ctx, st, filepath.Join(dir, "v1")); err != nil {
		t.Fatalf("render with a stream to the console failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "v1", "streams", fmt.Sprintf("%d.conf", s.ID))); !os.IsNotExist(err) {
		t.Fatalf("stream to the console rendered: %v", err)
	}
	c.Self = func(string) bool { return false }
	if err := c.Render(ctx, st, filepath.Join(dir, "v2")); err != nil {
		t.Fatalf("stream to another machine's port 9443 refused: %v", err)
	}
}

func TestSelfHost(t *testing.T) {
	if SelfHost("") {
		t.Error("empty host is self")
	}
	if name, err := os.Hostname(); err == nil && !SelfHost(strings.ToUpper(name)) {
		t.Errorf("own hostname %q is not self", name)
	}
	if !SelfHost("127.0.0.1") {
		t.Error("loopback address is not self")
	}
	if !SelfHost("[::1]") {
		t.Error("IPv6 loopback is not self")
	}
	if SelfHost("192.0.2.123") {
		t.Error("TEST-NET address is self")
	}
	if SelfHost("corvus-nonexistent.invalid") {
		t.Error("unresolvable name is self")
	}
	// an address of one of the interfaces
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				if !SelfHost(n.IP.String()) {
					t.Errorf("interface address %s is not self", n.IP)
				}
				break
			}
		}
	}
}
