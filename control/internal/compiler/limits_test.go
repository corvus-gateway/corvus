package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

// Upload size, upstream timeout and gzip per proxy host: unset values keep the global
// defaults (and the WebSocket read timeout); advanced configuration that sets the same
// server-level directive wins.
func TestRenderHostLimitsAndGzip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := Paths{DataDir: filepath.ToSlash(dir), LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181}
	c := &Compiler{P: p}
	n := func(v int) *int { return &v }

	for _, h := range []*store.ProxyHost{
		{Domains: []string{"default.example.com"}, Gzip: true},                                                                // 1
		{Domains: []string{"ws.example.com"}, Websocket: true, Gzip: true},                                                    // 2
		{Domains: []string{"big.example.com"}, MaxUploadMB: n(2048), UpstreamTimeoutS: n(300)},                                // 3
		{Domains: []string{"unlimited.example.com"}, MaxUploadMB: n(0), Websocket: true, UpstreamTimeoutS: n(30), Gzip: true}, // 4
		{Domains: []string{"adv.example.com"}, MaxUploadMB: n(5), AdvancedConfig: "client_max_body_size 20m;\ngzip on;"},      // 5
		{Domains: []string{"wslong.example.com"}, Websocket: true, UpstreamTimeoutS: n(7200)},                                 // 6
		// location advanced configuration with its own timeouts: no duplicate directive
		{Domains: []string{"wsloc.example.com"}, Websocket: true, Locations: []store.Location{
			{Path: "/api", ForwardScheme: "http", ForwardHost: "api", ForwardPort: 81, AdvancedConfig: "proxy_read_timeout 600s;"},
			{Path: "/", ForwardScheme: "http", ForwardHost: "web", ForwardPort: 82, AdvancedConfig: "proxy_send_timeout 5s;\nproxy_read_timeout 7s;"},
			{Path: "/x", ForwardScheme: "http", ForwardHost: "x", ForwardPort: 83, AdvancedConfig: "# proxy_read_timeout 9s;"},
		}}, // 7
	} {
		h.ForwardHost, h.ForwardPort, h.Enabled = "app", 8080, true
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "v1")
	if err := c.Render(ctx, st, out); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	main := read("nginx.conf")
	for _, w := range []string{"client_max_body_size 100m;", "gzip on;", "gzip_proxied any;", "gzip_vary on;", "application/json",
		// downloads are not spooled to /tmp, uploads stream to the upstream
		"proxy_max_temp_file_size 0;", "proxy_request_buffering off;", "client_body_buffer_size 128k;",
		"proxy_connect_timeout   10s;", "client_body_timeout   60s;",
		"proxy_ignore_headers X-Accel-Redirect X-Accel-Expires X-Accel-Limit-Rate X-Accel-Charset;"} {
		if !strings.Contains(main, w) {
			t.Errorf("nginx.conf: missing %q", w)
		}
	}
	if strings.Contains(main, "X-Accel-Buffering X-") {
		t.Error("X-Accel-Buffering must reach nginx (streaming upstreams turn buffering off)")
	}
	if strings.Contains(main, "text/event-stream") {
		t.Error("event streams must not be compressed (buffering)")
	}
	cases := []struct {
		id         string
		want, deny []string
	}{
		{"1", nil, []string{"\n    client_max_body_size", "gzip off", "proxy_read_timeout", "proxy_send_timeout"}},
		{"2", []string{"proxy_read_timeout 3600s;", "proxy_send_timeout 3600s;"}, []string{"gzip off"}},
		{"3", []string{"client_max_body_size 2048m;", "gzip off;", "proxy_read_timeout 300s;", "proxy_send_timeout 300s;"}, nil},
		// an explicit timeout applies as set, also with WebSocket
		{"4", []string{"client_max_body_size 0;", "proxy_read_timeout 30s;", "proxy_send_timeout 30s;"}, []string{"3600s", "gzip off"}},
		{"5", []string{"client_max_body_size 20m;", "gzip on;"}, []string{"client_max_body_size 5m", "gzip off"}},
		{"6", []string{"proxy_read_timeout 7200s;", "proxy_send_timeout 7200s;"}, []string{"3600s"}},
		{"7", []string{"proxy_read_timeout 600s;", "proxy_send_timeout 5s;", "proxy_read_timeout 7s;", "proxy_read_timeout 9s;"}, nil},
	}
	// one timeout of each kind per location: /api keeps its read timeout, "/" both, and a
	// commented-out directive does not count
	if c := read("hosts/7.conf"); strings.Count(c, "proxy_read_timeout") != 4 || strings.Count(c, "proxy_send_timeout") != 3 ||
		strings.Count(c, "proxy_read_timeout 3600s;") != 1 || strings.Count(c, "proxy_send_timeout 3600s;") != 2 {
		t.Errorf("host 7: generated timeouts must give way to the location's own\n%s", c)
	}
	for _, tc := range cases {
		conf := read("hosts/" + tc.id + ".conf")
		for _, w := range tc.want {
			if !strings.Contains(conf, w) {
				t.Errorf("host %s: missing %q\n%s", tc.id, w, conf)
			}
		}
		for _, d := range tc.deny {
			if strings.Contains(conf, d) {
				t.Errorf("host %s: unexpected %q\n%s", tc.id, d, conf)
			}
		}
	}
}
