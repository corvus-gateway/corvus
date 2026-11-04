package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

// Every proxy host gets its own upstream block with a keep-alive pool; names are
// resolved at run time (zone + resolve), IP literals and mapped loopback addresses are not.
func TestRenderUpstreams(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := Paths{DataDir: filepath.ToSlash(dir), LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181}
	c := &Compiler{P: p,
		Hosts:       func() map[string]string { return map[string]string{"host.docker.internal": "172.17.0.1"} },
		HostGateway: func() string { return "172.18.0.1" }}

	room := &store.Waitroom{Name: "event", Enabled: true, Capacity: 500, ActiveTTL: 120, GrantTTL: 30, QueueTTL: 120, Refresh: 15, FailMode: "open"}
	if err := st.SaveWaitroom(ctx, room); err != nil {
		t.Fatal(err)
	}
	n := 0
	mk := func(scheme, host string, port int, ws bool, roomID *int64) {
		n++
		h := &store.ProxyHost{Domains: []string{"h" + string(rune('a'+n)) + ".example.com"}, ForwardScheme: scheme,
			ForwardHost: host, ForwardPort: port, Websocket: ws, WaitroomID: roomID, WaitroomPath: "/event", Enabled: true}
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	mk("http", "app", 8080, false, nil)                // 1: container / DNS name
	mk("http", "10.3.2.39", 80, false, nil)            // 2: IP literal
	mk("http", "127.0.0.1", 3000, false, nil)          // 3: loopback = Docker host
	mk("https", "api.internal", 8443, false, nil)      // 4: HTTPS upstream (SNI)
	mk("https", "10.0.0.5", 443, false, nil)           // 5: HTTPS to an IP (no SNI)
	mk("http", "chat", 8080, true, nil)                // 6: websocket
	mk("http", "localhost", 8080, false, nil)          // 7: loopback name = Docker host
	mk("http", "host.docker.internal", 81, false, nil) // 8: /etc/hosts entry
	mk("http", "shop", 8080, false, &room.ID)          // 9: access queue location

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
	keepalive := []string{"keepalive 64;", "keepalive_requests 10000;", "keepalive_timeout 4s;", "proxy_set_header Connection", "proxy_http_version 1.1;"}
	cases := []struct {
		id         string
		want, deny []string
	}{
		{"1", []string{"upstream corvus_up_1 {", "zone corvus_up_1 64k;", "server app:8080 max_fails=0 resolve;", "proxy_pass http://corvus_up_1;", `proxy_set_header Connection "";`},
			[]string{"$corvus_upstream:", "proxy_ssl"}},
		{"2", []string{"server 10.3.2.39:80 max_fails=0;", "proxy_pass http://corvus_up_2;"}, []string{"resolve;", "zone "}},
		{"3", []string{"server 172.18.0.1:3000 max_fails=0;"}, []string{"127.0.0.1:3000", "resolve;"}},
		{"4", []string{"server api.internal:8443 max_fails=0 resolve;", "proxy_pass https://corvus_up_4;", "proxy_ssl_server_name on;", "proxy_ssl_name api.internal;"}, nil},
		{"5", []string{"server 10.0.0.5:443 max_fails=0;", "proxy_pass https://corvus_up_5;", "proxy_ssl_name 10.0.0.5;"}, []string{"resolve;"}},
		{"6", []string{"server chat:8080 max_fails=0 resolve;", "proxy_set_header Upgrade $corvus_upgrade;", "proxy_set_header Connection $corvus_connection_upgrade;"}, nil},
		{"7", []string{"server 172.18.0.1:8080 max_fails=0;"}, []string{"resolve;", "localhost"}},
		{"8", []string{"server 172.17.0.1:81 max_fails=0;"}, []string{"resolve;"}},
		{"9", []string{"server shop:8080 max_fails=0 resolve;"}, nil},
	}
	for _, tc := range cases {
		conf := read(tc.id)
		for _, w := range append(tc.want, keepalive...) {
			if !strings.Contains(conf, w) {
				t.Errorf("host %s: missing %q", tc.id, w)
			}
		}
		for _, d := range tc.deny {
			if strings.Contains(conf, d) {
				t.Errorf("host %s: unexpected %q", tc.id, d)
			}
		}
	}
	// access log from Lua (batched), nginx's per-request syslog output off; the queue host
	// passes its decision
	if c := read("1"); !strings.Contains(c, "access_log off;") || !strings.Contains(c, `require("corvus.rules").log(1) }`) {
		t.Errorf("host 1: access log not rendered as expected:\n%s", c)
	}
	if c := read("9"); !strings.Contains(c, `require("corvus.rules").log(9, true) }`) {
		t.Errorf("host 9: log phase without the queue flag")
	}
	// the access queue location and the root location both use the pool
	if n := strings.Count(read("9"), "proxy_pass http://corvus_up_9;"); n != 2 {
		t.Errorf("host 9: %d proxy_pass to the pool, want 2", n)
	}
	main, err := os.ReadFile(filepath.Join(out, "nginx.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"upstream corvus_acme {", "server 127.0.0.1:9181;", "resolver 127.0.0.11"} {
		if !strings.Contains(string(main), w) {
			t.Errorf("nginx.conf: missing %q", w)
		}
	}
}
