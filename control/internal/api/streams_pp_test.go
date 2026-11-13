package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Per-stream PROXY protocol: TCP only, accepting needs trusted PROXY protocol sources
// (not the HTTP listeners switch), omitted fields keep their value on update, and the
// sources can't be emptied while a stream accepts it.
func TestStreamProxyProtocol(t *testing.T) {
	e := newFullEnv(t)
	net := func(want int, trusted []string) map[string]any {
		t.Helper()
		return e.must(want, "PUT", "/api/settings/network", map[string]any{"trusted_proxies": []string{},
			"proxy_protocol": map[string]any{"trusted": trusted}})
	}

	// UDP only: refused (nginx accepts and sends the PROXY protocol on TCP only)
	for _, f := range []string{"proxy_protocol_in", "proxy_protocol_out"} {
		m := e.must(422, "POST", "/api/streams", map[string]any{"incoming_port": 5353, "tcp_forwarding": false, "udp_forwarding": true,
			"forward_host": "dns", "forward_port": 53, f: true})
		if zsField(m) != f || zsCode(m) != "needs_tcp" {
			t.Fatalf("UDP-only %s: %v", f, m)
		}
	}
	// accepting without trusted PROXY protocol sources: refused
	m := e.must(422, "POST", "/api/streams", map[string]any{"incoming_port": 2525, "forward_host": "mail", "forward_port": 25, "proxy_protocol_in": true})
	if zsField(m) != "proxy_protocol_in" || zsCode(m) != "trusted_required" {
		t.Fatalf("no trusted sources: %v", m)
	}
	// sending needs nothing
	m = e.must(201, "POST", "/api/streams", map[string]any{"incoming_port": 2526, "forward_host": "mail", "forward_port": 25, "proxy_protocol_out": true})
	out := zsObj(t, m, "stream")
	if out["proxy_protocol_out"] != true || out["proxy_protocol_in"] != false {
		t.Fatalf("send: %v", out)
	}

	// with trusted sources (HTTP PROXY protocol listeners still off)
	net(200, []string{"10.8.0.2"})
	m = e.must(201, "POST", "/api/streams", map[string]any{"incoming_port": 2525, "tcp_forwarding": true, "udp_forwarding": true,
		"forward_host": "mail", "forward_port": 25, "proxy_protocol_in": true})
	s := zsObj(t, m, "stream")
	if s["proxy_protocol_in"] != true || s["proxy_protocol_out"] != false {
		t.Fatalf("accept: %v", s)
	}
	sid := itoa(id(s))
	conf, err := os.ReadFile(filepath.Join(e.dir, "nginx", "current", "streams", sid+".conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"listen 2525 proxy_protocol;", "set_real_ip_from 10.8.0.2/32;", "listen 2525 udp;"} {
		if !strings.Contains(string(conf), want) {
			t.Errorf("missing %q\n%s", want, conf)
		}
	}

	// update: omitted = unchanged, false turns it off
	m = e.must(200, "PUT", "/api/streams/"+sid, map[string]any{"incoming_port": 2525, "udp_forwarding": true, "forward_host": "mail",
		"forward_port": 25, "proxy_protocol_out": true})
	if s := zsObj(t, m, "stream"); s["proxy_protocol_in"] != true || s["proxy_protocol_out"] != true {
		t.Fatalf("omitted on update: %v", s)
	}
	// turning TCP off with the PROXY protocol on: refused
	m = e.must(422, "PUT", "/api/streams/"+sid, map[string]any{"incoming_port": 2525, "tcp_forwarding": false, "udp_forwarding": true,
		"forward_host": "mail", "forward_port": 25})
	if zsCode(m) != "needs_tcp" {
		t.Fatalf("UDP only with PROXY protocol: %v", m)
	}

	// the sources can't be emptied while a stream accepts the PROXY protocol
	m = net(422, []string{})
	if zsField(m) != "proxy_protocol.trusted" || !strings.Contains(m["message"].(string), "stream #"+sid) {
		t.Fatalf("emptied sources: %v", m)
	}
	m = e.must(200, "PUT", "/api/streams/"+sid, map[string]any{"incoming_port": 2525, "udp_forwarding": true, "forward_host": "mail",
		"forward_port": 25, "proxy_protocol_in": false})
	if s := zsObj(t, m, "stream"); s["proxy_protocol_in"] != false || s["proxy_protocol_out"] != true {
		t.Fatalf("turn off: %v", s)
	}
	net(200, []string{})
	m = e.must(200, "GET", "/api/streams/"+sid, nil)
	if m["proxy_protocol_in"] != false || m["proxy_protocol_out"] != true {
		t.Fatalf("get: %v", m)
	}
}
