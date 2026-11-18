package conncheck

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func listenPort(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return ln, ln.Addr().(*net.TCPAddr).Port
}

func TestResolverAddr(t *testing.T) {
	for in, want := range map[string]string{
		"127.0.0.11":                     "127.0.0.11:53",
		"[fd00::1]":                      "[fd00::1]:53",
		"[fd00::1]:5353 1.1.1.1":         "[fd00::1]:5353",
		"1.1.1.1:5353":                   "1.1.1.1:5353",
		"valid=30s 8.8.8.8":              "8.8.8.8:53",
		"dns.local":                      "dns.local:53",
		"":                               "",
		"ipv6=off":                       "",
		"fd00::2":                        "[fd00::2]:53",
		"  127.0.0.53  ipv6=off  extra ": "127.0.0.53:53",
	} {
		if got := ResolverAddr(in); got != want {
			t.Errorf("ResolverAddr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckOK(t *testing.T) {
	_, port := listenPort(t)
	r := (&Checker{}).Check(context.Background(), "http", "127.0.0.1", port)
	if !r.OK || r.Kind != "" || r.Address != "127.0.0.1:"+strconv.Itoa(port) || r.Target != "127.0.0.1" {
		t.Fatalf("got %+v", r)
	}
}

func TestCheckRefused(t *testing.T) {
	ln, port := listenPort(t)
	ln.Close()
	r := (&Checker{}).Check(context.Background(), "http", "127.0.0.1", port)
	if r.OK || r.Kind != KindRefused {
		t.Fatalf("got %+v", r)
	}
}

// A dialer that never answers (a firewall dropping packets) gives "timeout" after the
// checker's timeout.
func TestCheckTimeout(t *testing.T) {
	c := &Checker{Timeout: 50 * time.Millisecond, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: ctx.Err()}
	}}
	start := time.Now()
	r := c.Check(context.Background(), "http", "10.255.255.1", 80)
	if r.OK || r.Kind != KindTimeout {
		t.Fatalf("got %+v", r)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("timeout not applied")
	}
}

func TestCheckDialKinds(t *testing.T) {
	for err, want := range map[error]string{
		syscall.EHOSTUNREACH:                   KindUnreachable,
		&net.OpError{Err: syscall.ENETUNREACH}: KindUnreachable,
		syscall.ECONNREFUSED:                   KindRefused,
		errors.New("something else"):           KindOther,
	} {
		c := &Checker{Dial: func(context.Context, string, string) (net.Conn, error) { return nil, err }}
		if r := c.Check(context.Background(), "http", "192.0.2.1", 80); r.Kind != want {
			t.Errorf("%v: kind %q, want %q", err, r.Kind, want)
		}
	}
}

// Names are looked up fully qualified (no search domains), IPv4 only unless IPv6 is on.
func TestCheckDNS(t *testing.T) {
	_, port := listenPort(t)
	var gotNet, gotHost string
	c := &Checker{Lookup: func(_ context.Context, network, host string) ([]net.IP, error) {
		gotNet, gotHost = network, host
		switch host {
		case "app.lan.":
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		case "empty.":
			return nil, nil
		case "slow.":
			return nil, &net.DNSError{Err: "timeout", Name: host, IsTimeout: true}
		case "odd.":
			return nil, errors.New("server misbehaving")
		}
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}}
	r := c.Check(context.Background(), "http", "app.lan", port)
	if !r.OK || gotNet != "ip4" || gotHost != "app.lan." || r.Address != fmt.Sprintf("127.0.0.1:%d", port) {
		t.Fatalf("got %+v (%s %s)", r, gotNet, gotHost)
	}
	for host, msg := range map[string]string{"nas": "no such name", "empty": "no address", "slow": "did not answer", "odd": "misbehaving"} {
		r = c.Check(context.Background(), "http", host, port)
		if r.OK || r.Kind != KindDNS || !strings.Contains(r.Message, msg) || gotHost != host+"." {
			t.Errorf("%s: got %+v", host, r)
		}
	}
	c.IPv6 = true
	c.Check(context.Background(), "http", "app.lan", port)
	if gotNet != "ip" {
		t.Errorf("IPv6 on: network %q", gotNet)
	}
}

// The real resolver path: a resolver that does not answer is a DNS failure.
func TestCheckResolverDown(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := pc.LocalAddr().String()
	pc.Close() // nothing listens: queries are refused or time out
	c := &Checker{Resolver: addr, Timeout: 2 * time.Second}
	r := c.Check(context.Background(), "http", "corvus-conncheck.invalid", 80)
	if r.OK || r.Kind != KindDNS {
		t.Fatalf("got %+v", r)
	}
}

func TestCheckHTTPS(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.StartTLS()
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	c := &Checker{Lookup: func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}}
	// self-signed certificate, not verified (like the gateway)
	if r := c.Check(context.Background(), "https", "127.0.0.1", port); !r.OK {
		t.Fatalf("https by IP: %+v", r)
	}
	// SNI is the forward host
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	hello := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			hello <- ""
			return
		}
		buf := make([]byte, 4096)
		n, _ := conn.Read(buf)
		hello <- string(buf[:n])
		conn.Close() // not TLS: the handshake fails
	}()
	r := c.Check(context.Background(), "https", "app.lan", ln.Addr().(*net.TCPAddr).Port)
	if r.OK || r.Kind != KindTLS {
		t.Fatalf("plain TCP port with https: %+v", r)
	}
	if h := <-hello; !strings.Contains(h, "app.lan") {
		t.Error("SNI app.lan not sent")
	}
}

// A name with several addresses: nginx round-robins over all of them, so each is tried;
// one failing address fails the check and is named.
func TestCheckAllAddresses(t *testing.T) {
	_, open := listenPort(t)
	addrs := map[string][]net.IP{
		"two.":  {net.ParseIP("127.0.0.1"), net.ParseIP("127.0.0.2")},
		"half.": {net.ParseIP("127.0.0.1"), net.ParseIP("192.0.2.9")},
		"many.": nil,
	}
	for i := 0; i < 12; i++ {
		addrs["many."] = append(addrs["many."], net.ParseIP(fmt.Sprintf("127.0.0.%d", i+1)))
	}
	var mu sync.Mutex
	dialed := map[string]bool{}
	c := &Checker{
		Timeout: 300 * time.Millisecond,
		Lookup: func(_ context.Context, _, host string) ([]net.IP, error) {
			return addrs[host], nil
		},
		Dial: func(ctx context.Context, _, addr string) (net.Conn, error) {
			mu.Lock()
			dialed[addr] = true
			mu.Unlock()
			if strings.HasPrefix(addr, "192.0.2.9:") { // blackhole
				<-ctx.Done()
				return nil, &net.OpError{Op: "dial", Net: "tcp", Err: ctx.Err()}
			}
			var d net.Dialer
			return d.DialContext(ctx, "tcp", "127.0.0.1:"+strconv.Itoa(open)) // all reach the listener
		},
	}
	p := strconv.Itoa(open)
	r := c.Check(context.Background(), "http", "two", open)
	if !r.OK || r.Address != "127.0.0.1:"+p || len(r.Addresses) != 2 || r.Addresses[1] != "127.0.0.2:"+p {
		t.Fatalf("two addresses: %+v", r)
	}
	r = c.Check(context.Background(), "http", "half", open)
	if r.OK || r.Kind != KindTimeout || r.Address != "192.0.2.9:"+p || len(r.Addresses) != 2 ||
		!strings.Contains(r.Message, "1 of 2 addresses failed: 192.0.2.9:"+p) || strings.Contains(r.Message, "127.0.0.1:") {
		t.Fatalf("one failing: %+v", r)
	}
	dialed = map[string]bool{}
	r = c.Check(context.Background(), "http", "many", open)
	if !r.OK || len(r.Addresses) != MaxAddresses || len(dialed) != MaxAddresses {
		t.Fatalf("capped: %+v, dialed %d", r, len(dialed))
	}
	// a single address keeps the plain message and no list
	c.Dial = nil
	c.Lookup = func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	ln, refused := listenPort(t)
	ln.Close()
	r = c.Check(context.Background(), "http", "one", refused)
	if r.OK || r.Kind != KindRefused || r.Addresses != nil || strings.Contains(r.Message, "addresses failed") {
		t.Fatalf("single: %+v", r)
	}
}

// Refuse applies to every resolved address of a name: nothing is dialed.
func TestCheckRefuse(t *testing.T) {
	var dialed atomic.Bool
	c := &Checker{
		Lookup: func(context.Context, string, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("10.0.0.1"), net.ParseIP("127.0.0.1")}, nil
		},
		Dial: func(context.Context, string, string) (net.Conn, error) {
			dialed.Store(true)
			return nil, errors.New("dialed")
		},
		Refuse: func(ip string, port int) string {
			if ip == "127.0.0.1" && port == 9180 {
				return "internal"
			}
			return ""
		},
	}
	if r := c.Check(context.Background(), "http", "sneaky.example", 9180); r.OK || r.Refused != "internal" || dialed.Load() {
		t.Fatalf("refused: %+v (dialed %v)", r, dialed.Load())
	}
	if r := c.Check(context.Background(), "http", "sneaky.example", 80); r.Refused != "" || !dialed.Load() {
		t.Fatalf("other port: %+v", r)
	}
}

// https to a port that accepts but never answers the handshake: TCP works, so it is a
// TLS failure (not a firewall timeout).
func TestCheckHTTPSTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	held := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			held <- c
		}
	}()
	c := &Checker{Timeout: 100 * time.Millisecond}
	r := c.Check(context.Background(), "https", "127.0.0.1", ln.Addr().(*net.TCPAddr).Port)
	if r.OK || r.Kind != KindTLS || r.Message != "TCP connected, but no TLS response" {
		t.Fatalf("got %+v", r)
	}
	select {
	case c := <-held:
		c.Close()
	default:
	}
}
