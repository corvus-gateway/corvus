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
	for _, w := range []string{"client_max_body_size 100m;", "gzip on;", "gzip_proxied any;", "gzip_vary on;", "application/json"} {
		if !strings.Contains(main, w) {
			t.Errorf("nginx.conf: missing %q", w)
		}
	}
	if strings.Contains(main, "text/event-stream") {
		t.Error("event streams must not be compressed (buffering)")
	}
	cases := []struct {
		id         string
		want, deny []string
	}{
		{"1", nil, []string{"\n    client_max_body_size", "gzip off", "proxy_read_timeout", "proxy_send_timeout"}},
		{"2", []string{"proxy_read_timeout 1h;"}, []string{"proxy_send_timeout", "gzip off"}},
		{"3", []string{"client_max_body_size 2048m;", "gzip off;", "proxy_read_timeout 300s;", "proxy_send_timeout 300s;"}, nil},
		{"4", []string{"client_max_body_size 0;", "proxy_read_timeout 30s;", "proxy_send_timeout 30s;"}, []string{"1h", "gzip off"}},
		{"5", []string{"client_max_body_size 20m;", "gzip on;"}, []string{"client_max_body_size 5m", "gzip off"}},
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
