package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/conncheck"
	"corvus/control/internal/gateway"
)

// "Check connection": the forward host as the gateway uses it, failures as kinds, only
// for those who may edit hosts, rate-limited per user.
func TestCheckConnection(t *testing.T) {
	e := newFullEnv(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	open := ln.Addr().(*net.TCPAddr).Port
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	refused := closed.Addr().(*net.TCPAddr).Port
	closed.Close()
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer tlsSrv.Close()
	tlsPort := tlsSrv.Listener.Addr().(*net.TCPAddr).Port

	var looked string
	e.s.ConnCheck = &conncheck.Checker{Timeout: 200 * time.Millisecond,
		Lookup: func(_ context.Context, _, host string) ([]net.IP, error) {
			looked = host
			if host == "app.lan." {
				return []net.IP{net.ParseIP("127.0.0.1")}, nil
			}
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		},
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr == "192.0.2.1:80" { // blackhole
				<-ctx.Done()
				return nil, ctx.Err()
			}
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		}}
	// the compiler's mapping: a name from /etc/hosts (extra_hosts) becomes its address
	e.s.Deployer.Compiler.Hosts = func() map[string]string { return map[string]string{"nas.extra": "127.0.0.1"} }

	check := func(scheme, host string, port int) map[string]any {
		t.Helper()
		return e.must(200, "POST", "/api/proxy-hosts/check", map[string]any{"forward_scheme": scheme, "forward_host": host, "forward_port": port})
	}
	if m := check("http", "app.lan", open); m["ok"] != true || m["address"] != fmt.Sprintf("127.0.0.1:%d", open) || looked != "app.lan." {
		t.Fatalf("ok: %v (looked up %q)", m, looked)
	}
	if m := check("http", "NAS.extra", open); m["ok"] != true || m["target"] != "127.0.0.1" {
		t.Fatalf("static host: %v", m)
	}
	if m := check("http", "127.0.0.1", refused); m["ok"] != false || m["kind"] != "refused" {
		t.Fatalf("refused: %v", m)
	}
	if m := check("http", "192.0.2.1", 80); m["ok"] != false || m["kind"] != "timeout" {
		t.Fatalf("timeout: %v", m)
	}
	if m := check("http", "nas", 80); m["ok"] != false || m["kind"] != "dns" || looked != "nas." {
		t.Fatalf("dns: %v", m)
	}
	if m := check("https", "127.0.0.1", tlsPort); m["ok"] != true {
		t.Fatalf("https: %v", m)
	}
	if m := check("https", "127.0.0.1", open); m["ok"] != false || m["kind"] != "tls" {
		t.Fatalf("tls: %v", m)
	}

	// invalid input and Corvus's own services
	e.must(422, "POST", "/api/proxy-hosts/check", map[string]any{"forward_scheme": "ftp", "forward_host": "app", "forward_port": 80})
	e.must(422, "POST", "/api/proxy-hosts/check", map[string]any{"forward_scheme": "http", "forward_host": "a b", "forward_port": 80})
	e.must(422, "POST", "/api/proxy-hosts/check", map[string]any{"forward_scheme": "http", "forward_host": "127.0.0.1", "forward_port": 9180})
	e.must(400, "POST", "/api/proxy-hosts/check", map[string]any{"forward_host": "app", "port": 80})

	// viewers can't; without the CSRF header nobody can
	viewer := e.as("viewer")
	if w, _ := e.zsAs(viewer, "POST", "/api/proxy-hosts/check", map[string]any{"forward_host": "app.lan", "forward_port": open}); w.Code != 403 {
		t.Fatalf("viewer: %d", w.Code)
	}
	if w, _ := e.do("POST", "/api/proxy-hosts/check", map[string]any{"forward_host": "app.lan", "forward_port": open}, withCookie(e.cookie)); w.Code != 403 {
		t.Fatalf("no CSRF header: %d", w.Code)
	}

	// per-user limit
	e.s.connLimit = auth.NewLimiter(2, time.Minute)
	op := e.as("operator")
	for i, want := range []int{200, 200, 429} {
		if w, _ := e.zsAs(op, "POST", "/api/proxy-hosts/check", map[string]any{"forward_host": "app.lan", "forward_port": open}); w.Code != want {
			t.Fatalf("operator check %d: %d, want %d", i, w.Code, want)
		}
	}
	e.must(200, "POST", "/api/proxy-hosts/check", map[string]any{"forward_host": "app.lan", "forward_port": open}) // another user

	// default checker: the gateway's resolver from Paths
	e.s.ConnCheck = nil
	if m := check("http", "127.0.0.1", refused); m["kind"] != "refused" {
		t.Fatalf("default checker: %v", m)
	}
}

// Concurrent checks of one user cannot get past the per-minute limit (the limit is
// checked and recorded in one step), and only a few checks run at once overall.
func TestCheckConnectionLimits(t *testing.T) {
	e := newFullEnv(t)
	release := make(chan struct{})
	var dialed atomic.Int32
	started := make(chan struct{}, 64)
	e.s.ConnCheck = &conncheck.Checker{Timeout: 5 * time.Second,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialed.Add(1)
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil, syscall.ECONNREFUSED
		}}
	body := map[string]any{"forward_host": "192.0.2.10", "forward_port": 80}

	// the global cap: connCheckConcurrent checks run, the next one is turned away
	op := e.as("operator")
	codes := make(chan int, connCheckConcurrent)
	for i := 0; i < connCheckConcurrent; i++ {
		go func() {
			w, _ := e.zsAs(op, "POST", "/api/proxy-hosts/check", body)
			codes <- w.Code
		}()
	}
	for i := 0; i < connCheckConcurrent; i++ {
		<-started
	}
	if w, _ := e.zsAs(e.cookie, "POST", "/api/proxy-hosts/check", body); w.Code != 429 {
		t.Fatalf("over the concurrency cap: %d", w.Code)
	}
	close(release)
	for i := 0; i < connCheckConcurrent; i++ {
		if c := <-codes; c != 200 {
			t.Fatalf("running check: %d", c)
		}
	}

	// the per-user limit under concurrency: exactly Max checks get through
	e.s.connLimit = auth.NewLimiter(5, time.Minute)
	e.s.connSem = make(chan struct{}, 100)
	dialed.Store(0)
	var wg sync.WaitGroup
	var ok, limited atomic.Int32
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch w, _ := e.zsAs(e.cookie, "POST", "/api/proxy-hosts/check", body); w.Code {
			case 200:
				ok.Add(1)
			case 429:
				limited.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 5 || limited.Load() != 25 || dialed.Load() != 5 {
		t.Fatalf("ok %d, limited %d, dialed %d", ok.Load(), limited.Load(), dialed.Load())
	}
}

// A name that resolves to loopback cannot be used to probe Corvus's own services.
func TestCheckConnectionInternalByName(t *testing.T) {
	e := newFullEnv(t)
	e.s.ConnCheck = &conncheck.Checker{Timeout: 200 * time.Millisecond,
		Lookup: func(context.Context, string, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("10.0.0.9"), net.ParseIP("127.0.0.1")}, nil
		},
		Dial: func(context.Context, string, string) (net.Conn, error) { return nil, syscall.ECONNREFUSED }}
	m := e.must(422, "POST", "/api/proxy-hosts/check", map[string]any{"forward_scheme": "http", "forward_host": "sneaky.example.com", "forward_port": 9180})
	if !strings.Contains(fmt.Sprint(m["message"]), "used internally") {
		t.Fatalf("internal by name: %v", m)
	}
	if m := e.must(200, "POST", "/api/proxy-hosts/check", map[string]any{"forward_scheme": "http", "forward_host": "sneaky.example.com", "forward_port": 8080}); m["ok"] != false {
		t.Fatalf("other port: %v", m)
	}
}

// The latest upstream failure of a host, from the gateway log, while recent and newer
// than the host's last change.
func TestUpstreamErrors(t *testing.T) {
	e := newFullEnv(t)
	if l := e.zsList("/api/upstream-errors"); len(l) != 0 {
		t.Fatalf("no gateway: %v", l)
	}
	mk := func(domain string) int64 {
		return id(zsObj(t, e.must(201, "POST", "/api/proxy-hosts", map[string]any{
			"domains": []string{domain, "alias." + domain}, "forward_scheme": "http", "forward_host": "app", "forward_port": 8080,
		}), "proxy_host"))
	}
	a, b := mk("a.example.com"), mk("b.example.com")
	_ = mk("c.example.com")
	off := mk("off.example.com")
	e.must(200, "PATCH", fmt.Sprintf("/api/proxy-hosts/%d", off), map[string]any{"enabled": false})
	now := time.Now()
	var fails []gateway.UpstreamFailure
	e.s.UpstreamFailures = func() []gateway.UpstreamFailure { return fails }
	if l := e.zsList("/api/upstream-errors"); len(l) != 0 {
		t.Fatalf("no failures: %v", l)
	}
	fails = []gateway.UpstreamFailure{
		{Server: "a.example.com", At: now.Add(2 * time.Second), Kind: "refused", Upstream: "10.0.0.5:8080", Message: "connect() failed (111: Connection refused)"},
		{Server: "b.example.com", At: now.Add(-20 * time.Minute), Kind: "timeout"}, // too old
		{Server: "c.example.com", At: now.Add(-time.Hour), Kind: "dns"},            // before the last change
		{Server: "off.example.com", At: now.Add(2 * time.Second), Kind: "dns"},     // disabled
		{Server: "unknown.example.com", At: now.Add(time.Second), Kind: "refused"}, // no such host
	}
	l := e.zsList("/api/upstream-errors")
	if len(l) != 1 || l[0]["host_id"] != float64(a) || l[0]["kind"] != "refused" || l[0]["upstream"] != "10.0.0.5:8080" {
		t.Fatalf("errors: %v", l)
	}
	// saving the host (a fix) clears the warning until the next failure
	h := e.must(200, "PUT", fmt.Sprintf("/api/proxy-hosts/%d", a), map[string]any{
		"domains": []string{"a.example.com"}, "forward_scheme": "http", "forward_host": "app", "forward_port": 8081,
	})
	saved, err := time.Parse(time.RFC3339, zsObj(t, h, "proxy_host")["updated_at"].(string))
	if err != nil {
		t.Fatal(err)
	}
	fails[0].At = saved.Add(500 * time.Millisecond) // within the second of the save
	if l := e.zsList("/api/upstream-errors"); len(l) != 0 {
		t.Fatalf("after saving: %v", l)
	}
	fails[0].At = saved.Add(1500 * time.Millisecond)
	if l := e.zsList("/api/upstream-errors"); len(l) != 1 {
		t.Fatalf("failing again: %v", l)
	}
	_ = b
	// viewers see it too
	if w, _ := e.zsAs(e.as("viewer"), "GET", "/api/upstream-errors", nil); w.Code != 200 {
		t.Fatalf("viewer: %d", w.Code)
	}
}
