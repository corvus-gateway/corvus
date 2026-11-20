package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

// A host with several forward targets gets one upstream block with all of them:
// round-robin, passive failover (max_fails/fail_timeout) and a shared zone; a host with
// one target renders exactly as before (max_fails=0, zone only for names).
func TestRenderBackendPool(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := Paths{DataDir: filepath.ToSlash(dir), LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181, AdminPort: 81}
	c := &Compiler{P: p, HostGateway: func() string { return "172.18.0.1" }, Self: func(string) bool { return false }}

	save := func(h *store.ProxyHost) {
		t.Helper()
		h.Enabled = true
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	save(&store.ProxyHost{Domains: []string{"single.example.com"}, ForwardScheme: "http", ForwardHost: "10.0.0.1", ForwardPort: 80})
	save(&store.ProxyHost{Domains: []string{"pool.example.com"}, ForwardScheme: "http", ForwardHost: "app1", ForwardPort: 8080,
		ExtraTargets: []store.Target{{ForwardHost: "app2", ForwardPort: 8080}, {ForwardHost: "10.0.0.7", ForwardPort: 9000}, {ForwardHost: "127.0.0.1", ForwardPort: 3000}}})
	save(&store.ProxyHost{Domains: []string{"ips.example.com"}, ForwardScheme: "https", ForwardHost: "10.0.0.1", ForwardPort: 443,
		ExtraTargets: []store.Target{{ForwardHost: "10.0.0.2", ForwardPort: 443}}})
	// the admin console in a pool with another backend: the gateway key is not sent
	save(&store.ProxyHost{Domains: []string{"console.example.com"}, ForwardScheme: "http", ForwardHost: "127.0.0.1", ForwardPort: 81,
		ExtraTargets: []store.Target{{ForwardHost: "other", ForwardPort: 81}}})
	save(&store.ProxyHost{Domains: []string{"console1.example.com"}, ForwardScheme: "http", ForwardHost: "127.0.0.1", ForwardPort: 81})
	// a pool with a custom location: the location keeps its single upstream
	save(&store.ProxyHost{Domains: []string{"loc.example.com"}, ForwardScheme: "http", ForwardHost: "a", ForwardPort: 80,
		ExtraTargets: []store.Target{{ForwardHost: "b", ForwardPort: 80}},
		Locations:    []store.Location{{Path: "/api", ForwardScheme: "http", ForwardHost: "api", ForwardPort: 80}}})

	out := filepath.Join(dir, "v1")
	if err := c.Render(ctx, st, out); err != nil {
		t.Fatal(err)
	}
	read := func(id string) string {
		b, err := os.ReadFile(filepath.Join(out, "hosts", id+".conf"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	single := read("1")
	if !strings.Contains(single, "upstream corvus_up_1 {\n    server 10.0.0.1:80 max_fails=0;\n    keepalive 64;") ||
		strings.Contains(single, "zone ") || strings.Contains(single, "fail_timeout") || strings.Contains(single, "round-robin") {
		t.Errorf("single target changed:\n%s", single)
	}
	pool := read("2")
	want := "upstream corvus_up_2 {\n    zone corvus_up_2 64k;\n" +
		"    server app1:8080 max_fails=3 fail_timeout=30s resolve;\n" +
		"    server app2:8080 max_fails=3 fail_timeout=30s resolve;\n" +
		"    server 10.0.0.7:9000 max_fails=3 fail_timeout=30s;\n" +
		"    server 172.18.0.1:3000 max_fails=3 fail_timeout=30s;\n" +
		"    keepalive 64;"
	if !strings.Contains(pool, want) || !strings.Contains(pool, "proxy_pass http://corvus_up_2;") || !strings.Contains(pool, `set $corvus_upstream "app1";`) {
		t.Errorf("pool:\n%s", pool)
	}
	ips := read("3")
	if !strings.Contains(ips, "zone corvus_up_3 64k;\n    server 10.0.0.1:443 max_fails=3 fail_timeout=30s;\n    server 10.0.0.2:443 max_fails=3 fail_timeout=30s;") ||
		!strings.Contains(ips, "proxy_ssl_name 10.0.0.1;") {
		t.Errorf("IP pool (zone for the shared failure state):\n%s", ips)
	}
	if conf := read("4"); strings.Contains(conf, "CORVUS_CONSOLE_KEY") {
		t.Errorf("console key sent to a pool with another backend:\n%s", conf)
	}
	if conf := read("5"); !strings.Contains(conf, "CORVUS_CONSOLE_KEY") {
		t.Errorf("console key missing for the console itself")
	}
	loc := read("6")
	if !strings.Contains(loc, "server b:80 max_fails=3 fail_timeout=30s resolve;") || !strings.Contains(loc, "server api:80 max_fails=0 resolve;") {
		t.Errorf("pool with a custom location:\n%s", loc)
	}

	// Corvus's own services are refused for every backend, not just the first
	c.HostGateway = func() string { return "" }
	h, _ := st.ProxyHostByID(ctx, 1)
	h.ExtraTargets = []store.Target{{ForwardHost: "10.0.0.2", ForwardPort: 80}, {ForwardHost: "127.0.0.1", ForwardPort: 9180}}
	if err := st.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := c.Render(ctx, st, filepath.Join(dir, "v2")); err == nil || !strings.Contains(err.Error(), "used internally") {
		t.Fatalf("internal backend in a pool rendered: %v", err)
	}
}
