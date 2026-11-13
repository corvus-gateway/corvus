package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PROXY protocol settings: defaults, validation (ports, trusted proxies, streams),
// rendering, audit, and stream ports reserved while enabled.
func TestNetworkProxyProtocol(t *testing.T) {
	e := newFullEnv(t)
	op := e.as("operator")

	m := e.must(200, "GET", "/api/settings/network", nil)
	pp := zsObj(t, m, "proxy_protocol")
	if pp["enabled"] != false || pp["http_port"] != float64(8080) || pp["https_port"] != float64(8443) {
		t.Fatalf("defaults: %v", pp)
	}

	on := map[string]any{"enabled": true, "http_port": 8080, "https_port": 8443}
	if w, _ := e.zsAs(op, "PUT", "/api/settings/network", map[string]any{"trusted_proxies": []string{"10.0.0.0/8"}, "proxy_protocol": on}); w.Code != 403 {
		t.Fatalf("operator put: %d", w.Code)
	}
	// no trusted proxies: refused
	m = e.must(422, "PUT", "/api/settings/network", map[string]any{"trusted_proxies": []string{}, "proxy_protocol": on})
	if zsField(m) != "trusted_proxies" || zsCode(m) != "no_trusted_proxies" {
		t.Fatalf("no trusted proxies: %v", m)
	}
	// Corvus's own ports, equal ports, out of range
	for _, c := range []struct {
		pp    map[string]any
		field string
	}{
		{map[string]any{"enabled": true, "http_port": 80, "https_port": 8443}, "proxy_protocol.http_port"},
		{map[string]any{"enabled": true, "http_port": 8080, "https_port": 9181}, "proxy_protocol.https_port"},
		{map[string]any{"enabled": true, "http_port": 8443, "https_port": 8443}, "proxy_protocol.https_port"},
		{map[string]any{"enabled": false, "http_port": 70000, "https_port": 8443}, "proxy_protocol.http_port"},
	} {
		m = e.must(422, "PUT", "/api/settings/network", map[string]any{"trusted_proxies": []string{"10.0.0.0/8"}, "proxy_protocol": c.pp})
		if zsField(m) != c.field {
			t.Fatalf("%v: %v", c.pp, m)
		}
	}
	// a stream on the port (even a disabled one)
	s := e.must(201, "POST", "/api/streams", map[string]any{"incoming_port": 8443, "forward_host": "db", "forward_port": 5432, "enabled": false})
	m = e.must(422, "PUT", "/api/settings/network", map[string]any{"trusted_proxies": []string{"10.0.0.0/8"}, "proxy_protocol": on})
	if zsField(m) != "proxy_protocol.https_port" || !strings.Contains(m["message"].(string), "stream #") {
		t.Fatalf("stream conflict: %v", m)
	}
	e.must(200, "DELETE", "/api/streams/"+itoa(id(zsObj(t, s, "stream"))), nil)

	m = e.must(200, "PUT", "/api/settings/network", map[string]any{"trusted_proxies": []string{"10.0.0.0/8"}, "proxy_protocol": on})
	if pp := zsObj(t, zsObj(t, m, "network"), "proxy_protocol"); pp["enabled"] != true || m["version"] == nil {
		t.Fatalf("enable: %v", m)
	}
	conf, err := os.ReadFile(filepath.Join(e.dir, "nginx", "current", "nginx.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"listen 8080 proxy_protocol default_server;", "listen 8443 ssl proxy_protocol default_server;",
		"set_real_ip_from 10.0.0.0/8;", "real_ip_header proxy_protocol;"} {
		if !strings.Contains(string(conf), want) {
			t.Errorf("missing %q", want)
		}
	}
	// omitted = unchanged; clearing the trusted proxies is refused while enabled
	m = e.must(200, "PUT", "/api/settings/network", map[string]any{"trusted_proxies": []string{"10.0.0.0/8"}, "mask_ips": true})
	if pp := zsObj(t, zsObj(t, m, "network"), "proxy_protocol"); pp["enabled"] != true {
		t.Fatalf("omitted: %v", m)
	}
	m = e.must(422, "PUT", "/api/settings/network", map[string]any{"trusted_proxies": []string{}})
	if zsField(m) != "trusted_proxies" {
		t.Fatalf("clear trusted: %v", m)
	}
	// streams may not use the ports while enabled
	m = e.must(422, "POST", "/api/streams", map[string]any{"incoming_port": 8080, "forward_host": "db", "forward_port": 5432})
	if zsField(m) != "incoming_port" || !strings.Contains(m["message"].(string), "PROXY protocol HTTP") {
		t.Fatalf("stream on the PROXY protocol port: %v", m)
	}

	// disabling (missing ports default) frees them
	e.must(200, "PUT", "/api/settings/network", map[string]any{"trusted_proxies": []string{}, "proxy_protocol": map[string]any{"enabled": false}})
	m = e.must(200, "GET", "/api/settings/network", nil)
	if pp := zsObj(t, m, "proxy_protocol"); pp["enabled"] != false || pp["http_port"] != float64(8080) {
		t.Fatalf("disabled: %v", pp)
	}
	e.must(201, "POST", "/api/streams", map[string]any{"incoming_port": 8080, "forward_host": "db", "forward_port": 5432})
	if zsCount(zsAudit(t, e), "network.update") != 3 {
		t.Fatalf("audit: %v", zsAudit(t, e))
	}
}
