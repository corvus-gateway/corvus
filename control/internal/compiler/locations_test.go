package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

// Custom locations get their own upstream block and a ^~ prefix location with the host's
// protections; a location "/" replaces the forward host; the access queue still applies
// to locations under the protected path.
func TestRenderLocations(t *testing.T) {
	c, st := renderPagesEnv(t)
	c.HostGateway = func() string { return "172.18.0.1" }
	ctx := context.Background()
	rl := &store.RateLimit{Name: "api", RequestsPerSecond: 10, StatusCode: 429, Enabled: true, ExemptCIDRs: []string{}}
	if err := st.SaveRateLimit(ctx, rl); err != nil {
		t.Fatal(err)
	}
	room := &store.Waitroom{Name: "event", Enabled: true, Capacity: 500, ActiveTTL: 120, GrantTTL: 30, QueueTTL: 120, Refresh: 15, FailMode: "open"}
	if err := st.SaveWaitroom(ctx, room); err != nil {
		t.Fatal(err)
	}
	mk := func(d, wp string, roomID *int64, locs ...store.Location) int64 {
		h := &store.ProxyHost{Domains: []string{d}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true,
			RateLimitID: &rl.ID, WaitroomID: roomID, WaitroomPath: wp, Locations: locs}
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
		return h.ID
	}
	plain := mk("a.example.com", "/", nil,
		store.Location{Path: "/api", ForwardScheme: "https", ForwardHost: "api.internal", ForwardPort: 8443, ForwardPath: "/v2/",
			AdvancedConfig: "client_max_body_size 50m;"},
		store.Location{Path: "/static/", ForwardHost: "127.0.0.1", ForwardPort: 9000})
	queued := mk("b.example.com", "/event", &room.ID,
		store.Location{Path: "/event/api", ForwardHost: "10.0.0.7", ForwardPort: 80})
	rootQ := mk("c.example.com", "/", &room.ID,
		store.Location{Path: "/", ForwardHost: "new-app", ForwardPort: 3000},
		store.Location{Path: "/x", ForwardHost: "x", ForwardPort: 80})

	timeout := 30
	th := &store.ProxyHost{Domains: []string{"t.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true,
		UpstreamTimeoutS: &timeout, Locations: []store.Location{{Path: "/api", ForwardHost: "api", ForwardPort: 80}}}
	if err := st.SaveProxyHost(ctx, th); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(t.TempDir(), "v1")
	if err := c.Render(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
	read := func(id int64) string {
		b, err := os.ReadFile(filepath.Join(dir, "hosts", strconv.FormatInt(id, 10)+".conf"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	check := func(id int64, want, deny []string) {
		t.Helper()
		conf := read(id)
		for _, w := range want {
			if !strings.Contains(conf, w) {
				t.Errorf("host %d: missing %q\n%s", id, w, conf)
			}
		}
		for _, d := range deny {
			if strings.Contains(conf, d) {
				t.Errorf("host %d: unexpected %q", id, d)
			}
		}
	}
	check(plain, []string{
		"upstream corvus_up_1 {", "upstream corvus_up_1_1 {", "server api.internal:8443 max_fails=0 resolve;",
		"upstream corvus_up_1_2 {", "server 172.18.0.1:9000 max_fails=0;",
		"location ^~ /api {", "proxy_pass https://corvus_up_1_1/v2/;", "proxy_ssl_name api.internal;", "client_max_body_size 50m;",
		"location ^~ /static/ {", "proxy_pass http://corvus_up_1_2;",
		"location / {", "proxy_pass http://corvus_up_1;",
	}, []string{"corvus_qroom", "$corvus_room"})
	// rate limit (guard) inside each custom location, plus the root location
	if n := strings.Count(read(plain), "limit_req zone=corvus_rl"); n != 3 {
		t.Errorf("host %d: %d rate-limited locations, want 3", plain, n)
	}
	check(queued, []string{
		"map $uri $corvus_qroom_2 {", `"~*^/event(?:[/;]|$)" "event";`,
		"location ^~ /event/api {", "set $corvus_room $corvus_qroom_2;",
		`access_by_lua_block { if ngx.var.corvus_room ~= "" then require("corvus.waitroom").access() end }`,
		"proxy_pass http://corvus_up_2_1;",
	}, nil)
	conf := read(rootQ)
	check(rootQ, []string{"upstream corvus_up_3_1 {", "server new-app:3000 max_fails=0 resolve;", "location ^~ /x {"}, []string{"corvus_qroom", "location ^~ / {"})
	// "/" replaces the forward host in the root location; the room applies to every location
	if !strings.Contains(conf, "location / {") || strings.Count(conf, "proxy_pass http://corvus_up_3_1;") != 1 || strings.Contains(conf, "proxy_pass http://corvus_up_3;") {
		t.Errorf("host %d: root location does not use the custom location:\n%s", rootQ, conf)
	}
	if n := strings.Count(conf, `set $corvus_room "event";`); n != 2 {
		t.Errorf("host %d: %d queued locations, want 2", rootQ, n)
	}
	// the host's upstream timeout applies to its custom locations too
	if n := strings.Count(read(th.ID), "proxy_read_timeout 30s;"); n != 2 {
		t.Errorf("host %d: upstream timeout in %d locations, want 2", th.ID, n)
	}
}
