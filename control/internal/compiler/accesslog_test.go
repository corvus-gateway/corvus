package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

// nginx's per-request syslog access log stays off unless the advanced configuration sets
// its own server-level access_log: one inside a location or a comment would otherwise log
// every other request twice (Lua batch + syslog) and double the traffic numbers.
func TestOwnAccessLogOnlyAtServerLevel(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := Paths{DataDir: filepath.ToSlash(dir), LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181}
	c := &Compiler{P: p, Hosts: func() map[string]string { return nil }, HostGateway: func() string { return "172.18.0.1" }}

	cases := []struct {
		adv     string
		offWant bool
	}{
		{"location /health { access_log off; return 200; }", true},
		{"# access_log /var/log/x.log;\nadd_header X-A 1;", true},
		{"access_log /data/logs/own.log;", false},
	}
	for i, tc := range cases {
		h := &store.ProxyHost{Domains: []string{"al" + string(rune('a'+i)) + ".example.com"}, ForwardScheme: "http",
			ForwardHost: "app", ForwardPort: 80, Enabled: true, AdvancedConfig: tc.adv}
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "v1")
	if err := c.Render(ctx, st, out); err != nil {
		t.Fatal(err)
	}
	for i, tc := range cases {
		b, err := os.ReadFile(filepath.Join(out, "hosts", string(rune('1'+i))+".conf"))
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(b), "\n    access_log off;"); got != tc.offWant {
			t.Errorf("advanced %q: server-level access_log off = %v, want %v", tc.adv, got, tc.offWant)
		}
	}
}
