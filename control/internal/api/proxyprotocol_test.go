package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PROXY protocol settings: defaults, validation (ports, its own trusted sources,
// streams), merge semantics, rendering, audit, and stream ports reserved while enabled.
func TestNetworkProxyProtocol(t *testing.T) {
	e := newFullEnv(t)
	op := e.as("operator")

	m := e.must(200, "GET", "/api/settings/network", nil)
	pp := zsObj(t, m, "proxy_protocol")
	if pp["enabled"] != false || pp["http_port"] != float64(8080) || pp["https_port"] != float64(8443) || len(pp["trusted"].([]any)) != 0 {
		t.Fatalf("defaults: %v", pp)
	}
	put := func(want int, body map[string]any) map[string]any {
		t.Helper()
		if _, ok := body["trusted_proxies"]; !ok {
			body["trusted_proxies"] = []string{"192.168.0.0/16"}
		}
		return e.must(want, "PUT", "/api/settings/network", body)
	}

	on := map[string]any{"enabled": true, "http_port": 8080, "https_port": 8443, "trusted": []string{"10.0.0.5"}}
	if w, _ := e.zsAs(op, "PUT", "/api/settings/network", map[string]any{"proxy_protocol": on}); w.Code != 403 {
		t.Fatalf("operator put: %d", w.Code)
	}
	// no PROXY protocol sources: refused, even with general trusted proxies
	m = put(422, map[string]any{"proxy_protocol": map[string]any{"enabled": true}})
	if zsField(m) != "proxy_protocol.trusted" || zsCode(m) != "required" {
		t.Fatalf("no sources: %v", m)
	}
	// invalid / too broad sources, Corvus's own ports, equal ports, out of range
	for _, c := range []struct {
		pp    map[string]any
		field string
	}{
		{map[string]any{"enabled": true, "trusted": []string{"nope"}}, "proxy_protocol.trusted"},
		{map[string]any{"enabled": true, "trusted": []string{"0.0.0.0/0"}}, "proxy_protocol.trusted"},
		{map[string]any{"enabled": true, "http_port": 80, "trusted": []string{"10.0.0.5"}}, "proxy_protocol.http_port"},
		{map[string]any{"enabled": true, "https_port": 9181, "trusted": []string{"10.0.0.5"}}, "proxy_protocol.https_port"},
		{map[string]any{"enabled": true, "http_port": 8443, "trusted": []string{"10.0.0.5"}}, "proxy_protocol.https_port"},
		{map[string]any{"enabled": false, "http_port": 70000}, "proxy_protocol.http_port"},
	} {
		m = put(422, map[string]any{"proxy_protocol": c.pp})
		if zsField(m) != c.field {
			t.Fatalf("%v: %v", c.pp, m)
		}
	}
	// a stream on the port (even a disabled one)
	s := e.must(201, "POST", "/api/streams", map[string]any{"incoming_port": 8443, "forward_host": "db", "forward_port": 5432, "enabled": false})
	m = put(422, map[string]any{"proxy_protocol": on})
	if zsField(m) != "proxy_protocol.https_port" || !strings.Contains(m["message"].(string), "stream #") {
		t.Fatalf("stream conflict: %v", m)
	}
	e.must(200, "DELETE", "/api/streams/"+itoa(id(zsObj(t, s, "stream"))), nil)

	m = put(200, map[string]any{"proxy_protocol": on})
	if pp := zsObj(t, zsObj(t, m, "network"), "proxy_protocol"); pp["enabled"] != true || pp["trusted"].([]any)[0] != "10.0.0.5/32" || m["version"] == nil {
		t.Fatalf("enable: %v", m)
	}
	conf, err := os.ReadFile(filepath.Join(e.dir, "nginx", "current", "nginx.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"listen 8080 proxy_protocol default_server;", "listen 8443 ssl proxy_protocol default_server;",
		"set_real_ip_from 10.0.0.5/32;", "real_ip_header proxy_protocol;"} {
		if !strings.Contains(string(conf), want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(string(conf), "192.168.0.0/16") {
		t.Error("the general trusted proxies are trusted for the PROXY header")
	}
	// the general trusted proxies are independent: clearing them is fine
	m = put(200, map[string]any{"trusted_proxies": []string{}, "mask_ips": true})
	if pp := zsObj(t, zsObj(t, m, "network"), "proxy_protocol"); pp["enabled"] != true {
		t.Fatalf("omitted: %v", m)
	}
	// merge: omitted fields and zero ports keep the stored values
	put(200, map[string]any{"proxy_protocol": map[string]any{"http_port": 8081, "https_port": 8444}})
	m = put(200, map[string]any{"proxy_protocol": map[string]any{"http_port": 0}})
	if pp := zsObj(t, zsObj(t, m, "network"), "proxy_protocol"); pp["enabled"] != true || pp["http_port"] != float64(8081) ||
		pp["https_port"] != float64(8444) || len(pp["trusted"].([]any)) != 1 {
		t.Fatalf("merge: %v", pp)
	}
	// emptying the sources while enabled is refused
	m = put(422, map[string]any{"proxy_protocol": map[string]any{"trusted": []string{}}})
	if zsField(m) != "proxy_protocol.trusted" {
		t.Fatalf("clear sources: %v", m)
	}
	// streams may not use the ports while enabled
	m = e.must(422, "POST", "/api/streams", map[string]any{"incoming_port": 8081, "forward_host": "db", "forward_port": 5432})
	if zsField(m) != "incoming_port" || !strings.Contains(m["message"].(string), "PROXY protocol HTTP") {
		t.Fatalf("stream on the PROXY protocol port: %v", m)
	}

	// disabling keeps ports and sources, and frees the ports
	put(200, map[string]any{"proxy_protocol": map[string]any{"enabled": false}})
	m = e.must(200, "GET", "/api/settings/network", nil)
	if pp := zsObj(t, m, "proxy_protocol"); pp["enabled"] != false || pp["http_port"] != float64(8081) || len(pp["trusted"].([]any)) != 1 {
		t.Fatalf("disabled: %v", pp)
	}
	e.must(201, "POST", "/api/streams", map[string]any{"incoming_port": 8081, "forward_host": "db", "forward_port": 5432})
	if zsCount(zsAudit(t, e), "network.update") != 5 {
		t.Fatalf("audit: %v", zsAudit(t, e))
	}
}
