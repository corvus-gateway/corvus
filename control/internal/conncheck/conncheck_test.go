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

// https to a port that accepts but never answers the handshake: timeout, not TLS.
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
	if r.OK || r.Kind != KindTimeout {
		t.Fatalf("got %+v", r)
	}
	select {
	case c := <-held:
		c.Close()
	default:
	}
}
