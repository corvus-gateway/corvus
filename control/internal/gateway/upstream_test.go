package gateway

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Lines as OpenResty 1.31 writes them (error_log stderr warn).
const (
	lnRefused = `2026/11/16 05:20:31 [error] 10#10: *3 connect() failed (111: Connection refused) while connecting to upstream, client: 192.0.2.7, server: a.test, request: "GET /ref?q=secret HTTP/1.1", upstream: "http://127.0.0.1:9/ref?q=secret", host: "alias.test"`
	lnDNS     = `2026/11/16 05:20:31 [error] 10#10: *2 no live upstreams while connecting to upstream, client: 127.0.0.1, server: b.test, request: "GET /dns HTTP/1.1", upstream: "http://corvus_up_4/dns", host: "b.test"`
	lnTimeout = `2026/11/16 05:20:33 [error] 10#10: *5 upstream timed out (110: Operation timed out) while connecting to upstream, client: 127.0.0.1, server: C.Test, request: "GET /to HTTP/1.1", upstream: "http://10.255.255.1:80/to", host: "c.test"`
	lnTLS     = `2026/11/16 05:20:33 [error] 10#10: *7 SSL_do_handshake() failed (SSL: error:0A00010B:SSL routines::wrong version number) while SSL handshaking to upstream, client: 127.0.0.1, server: d.test, request: "GET /tls HTTP/1.1", upstream: "https://127.0.0.1:8081/tls", host: "d.test"`
	lnRoute   = `2026/11/16 05:20:34 [error] 10#10: *9 connect() failed (113: No route to host) while connecting to upstream, client: 127.0.0.1, server: e.test, request: "GET / HTTP/1.1", upstream: "http://[fd00::5]:8080/", host: "e.test"`
	lnClosed  = `2026/11/16 05:20:35 [error] 10#10: *11 upstream prematurely closed connection while reading response header from upstream, client: 127.0.0.1, server: f.test, request: "GET / HTTP/1.1", upstream: "http://10.0.0.5:8080/", host: "f.test"`
)

func TestParseUpstreamFailure(t *testing.T) {
	for _, c := range []struct {
		line, server, kind, upstream, msg string
	}{
		{lnRefused, "a.test", "refused", "127.0.0.1:9", "connect() failed (111: Connection refused) while connecting to upstream"},
		{lnDNS, "b.test", "dns", "", "no live upstreams while connecting to upstream"},
		{lnTimeout, "c.test", "timeout", "10.255.255.1:80", "upstream timed out (110: Operation timed out) while connecting to upstream"},
		{lnTLS, "d.test", "tls", "127.0.0.1:8081", "SSL_do_handshake() failed (SSL: error:0A00010B:SSL routines::wrong version number) while SSL handshaking to upstream"},
		{lnRoute, "e.test", "unreachable", "[fd00::5]:8080", "connect() failed (113: No route to host) while connecting to upstream"},
		{lnClosed, "f.test", "other", "10.0.0.5:8080", "upstream prematurely closed connection while reading response header from upstream"},
	} {
		f, ok := parseUpstreamFailure(c.line)
		if !ok || f.Server != c.server || f.Kind != c.kind || f.Upstream != c.upstream || f.Message != c.msg {
			t.Errorf("%s: got %+v %v", c.server, f, ok)
		}
		if strings.Contains(f.Message+f.Upstream, "secret") || strings.Contains(f.Message, "192.0.2.7") {
			t.Errorf("request or client kept: %+v", f)
		}
	}
	for _, line := range []string{
		// client side, rate limiting, access rules, Lua, other levels
		`2026/11/16 05:20:35 [error] 10#10: *12 writev() failed (32: Broken pipe) while sending to client, client: 127.0.0.1, server: a.test, request: "GET / HTTP/1.1", upstream: "http://10.0.0.5:8080/", host: "a.test"`,
		`2026/11/16 05:20:35 [error] 10#10: *13 client prematurely closed connection, client: 127.0.0.1, server: a.test, request: "GET / HTTP/1.1", upstream: "http://10.0.0.5:8080/", host: "a.test"`,
		`2026/11/16 05:20:35 [error] 10#10: *14 limiting requests, excess: 5.000 by zone "corvus_rl1", client: 127.0.0.1, server: a.test, request: "GET / HTTP/1.1", host: "a.test"`,
		`2026/11/16 05:20:35 [error] 10#10: *15 access forbidden by rule, client: 127.0.0.1, server: a.test, request: "GET / HTTP/1.1", host: "a.test"`,
		`2026/11/16 05:20:35 [warn] 10#10: *16 upstream server temporarily disabled while connecting to upstream, client: 127.0.0.1, server: a.test, request: "GET / HTTP/1.1", upstream: "http://10.0.0.5:8080/", host: "a.test"`,
		`2026/11/16 05:20:29 [error] 10#10: recv() failed (111: Connection refused) while resolving, resolver: 127.0.0.11:53`,
		`2026/11/16 05:20:35 [error] 10#10: *17 connect() failed (111: Connection refused) while connecting to upstream, client: 127.0.0.1, server: , request: "GET / HTTP/1.1", upstream: "http://10.0.0.5:8080/", host: "a.test"`,
	} {
		if f, ok := parseUpstreamFailure(line); ok {
			t.Errorf("not an upstream failure: %q → %+v", line, f)
		}
	}
	// the visitor's text (referrer, user name, request, Lua errors) cannot make a line an
	// upstream failure
	forged := `, upstream: "http://6.6.6.6:80/"`
	for _, line := range []string{
		`2026/11/16 05:20:36 [error] 10#10: *20 client intended to send too large body: 2000000 bytes, client: 127.0.0.1, server: a.test, request: "POST / HTTP/1.1", host: "a.test", referrer: "http://x/` + forged + `"`,
		`2026/11/16 05:20:36 [error] 10#10: *21 limiting requests, excess: 5.000 by zone "corvus_rl1", client: 127.0.0.1, server: a.test, request: "GET / HTTP/1.1", host: "a.test", referrer: "while connecting to upstream` + forged + `"`,
		`2026/11/16 05:20:36 [error] 10#10: *22 user "x while connecting to upstream, client: 1.2.3.4, server: b.test, request: "GET / HTTP/1.1"` + forged + `, host: "b.test" was not found in "/data/htpasswd/1", client: 127.0.0.1, server: a.test, request: "GET / HTTP/1.1", host: "a.test"`,
		`2026/11/16 05:20:36 [error] 10#10: *23 failed to run log_by_lua*: /usr/local/corvus/lua/log.lua:12: attempt to index a nil value while connecting to upstream, client: 127.0.0.1, server: a.test, request: "GET / HTTP/1.1", upstream: "http://10.0.0.5:8080/", host: "a.test"`,
		`2026/11/16 05:20:36 [error] 10#10: *24 lua entry thread aborted: runtime error: x while reading response header from upstream, client: 127.0.0.1, server: a.test, request: "GET / HTTP/1.1", upstream: "http://10.0.0.5:8080/", host: "a.test"`,
		`2026/11/16 05:20:36 [error] 10#10: *25 access forbidden by rule, client: 127.0.0.1, server: a.test, request: "GET /` + forged + ` HTTP/1.1", host: "a.test"`,
	} {
		if f, ok := parseUpstreamFailure(line); ok {
			t.Errorf("forged upstream failure: %q → %+v", line, f)
		}
	}
	// a real failure keeps its own upstream, not one from the host or referrer
	withRef := strings.Replace(lnRefused, `host: "alias.test"`, `host: "alias.test`+forged+`", referrer: "x`+forged+`"`, 1)
	if f, ok := parseUpstreamFailure(withRef); !ok || f.Upstream != "127.0.0.1:9" || f.Server != "a.test" {
		t.Errorf("referrer after a real failure: %+v %v", f, ok)
	}

	long := strings.Replace(lnRefused, "connect() failed", strings.Repeat("x", 400), 1)
	if f, _ := parseUpstreamFailure(long); len(f.Message) != 300 {
		t.Errorf("message not shortened: %d", len(f.Message))
	}
}

// The latest failure per server is kept (not as a serious line); the table is bounded.
func TestErrLogUpstreamFailures(t *testing.T) {
	var out bytes.Buffer
	l := newErrLog(&out)
	start := time.Now()
	_, _ = l.Write([]byte(lnRefused + "\n" + lnDNS + "\n"))
	_, _ = l.Write([]byte(strings.Replace(lnRefused, "(111: Connection refused)", "(113: No route to host)", 1) + "\n"))
	if l.since(start) != nil {
		t.Fatal("upstream failures are not serious lines")
	}
	got := map[string]UpstreamFailure{}
	for _, f := range l.upstreamFailures() {
		got[f.Server] = f
	}
	if len(got) != 2 || got["a.test"].Kind != "unreachable" || got["b.test"].Kind != "dns" || got["a.test"].At.Before(start) {
		t.Fatalf("failures: %+v", got)
	}

	for i := 0; i < upstreamFailuresMax+5; i++ {
		l.upstreamFailure(UpstreamFailure{Server: fmt.Sprintf("h%d.test", i), At: start.Add(time.Duration(i) * time.Millisecond)})
	}
	all := l.upstreamFailures()
	if len(all) != upstreamFailuresMax {
		t.Fatalf("kept %d", len(all))
	}
	for _, f := range all {
		if f.Server == "h0.test" || f.Server == "a.test" {
			t.Fatalf("oldest not dropped: %s", f.Server)
		}
	}

	o := &OpenResty{}
	if o.UpstreamFailures() != nil {
		t.Fatal("no log yet")
	}
	o.errs = l
	if len(o.UpstreamFailures()) != upstreamFailuresMax {
		t.Fatal("OpenResty.UpstreamFailures")
	}
}
