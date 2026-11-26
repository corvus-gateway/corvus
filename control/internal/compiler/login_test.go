package compiler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

// A host with a login page carries its settings into rules.json (never the PIN hash) and
// gets the login, logout and credential-check locations; every proxy location of every
// host replaces a client-sent X-Corvus-User header.
func TestRenderLoginPage(t *testing.T) {
	c, st := renderPagesEnv(t)
	ctx := context.Background()
	save := func(h *store.ProxyHost) int64 {
		t.Helper()
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
		return h.ID
	}
	plain := save(&store.ProxyHost{Domains: []string{"open.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true})
	pin := save(&store.ProxyHost{Domains: []string{"pin.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true,
		Login: store.HostLogin{Enabled: true, Mode: store.LoginPIN, PINHash: "$2a$10$secret-hash", PINVersion: 3, ExcludePaths: []string{"/api/"}}})
	users := save(&store.ProxyHost{Domains: []string{"users.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true,
		ExtraTargets: []store.Target{{ForwardHost: "app2", ForwardPort: 8080}},
		Locations:    []store.Location{{Path: "/media/", ForwardScheme: "http", ForwardHost: "media", ForwardPort: 9000}},
		Login:        store.HostLogin{Enabled: true, Mode: store.LoginUsers, GroupIDs: []int64{1}, SessionHours: 24, PassUser: true}})
	off := save(&store.ProxyHost{Domains: []string{"off.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true,
		Login: store.HostLogin{Enabled: false, Mode: store.LoginPIN, PINHash: "$2a$10$other"}})
	page := save(&store.ProxyHost{Domains: []string{"page.example.com"}, Content: store.ContentPage, Enabled: true,
		Page:  store.StaticPage{Template: store.PageMaintenance, Status: 503},
		Login: store.HostLogin{Enabled: true, Mode: store.LoginPIN, PINHash: "h"}})

	dir := filepath.Join(t.TempDir(), "v1")
	if err := c.Render(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
	raw := readFile(t, filepath.Join(dir, "rules.json"))
	if strings.Contains(raw, "secret-hash") || strings.Contains(raw, "pin_hash") || strings.Contains(raw, "pin_version") {
		t.Fatalf("rules.json carries PIN secrets: %s", raw)
	}
	var rules rulesFile
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		t.Fatal(err)
	}
	if rules.LoginPage != "/opt/corvus/html/login.html" {
		t.Errorf("login page: %q", rules.LoginPage)
	}
	key := func(id int64) string { return strconv.FormatInt(id, 10) }
	if l := rules.Hosts[key(plain)].Login; l != nil {
		t.Errorf("host without login page: %+v", l)
	}
	if l := rules.Hosts[key(off)].Login; l != nil {
		t.Errorf("disabled login page: %+v", l)
	}
	if l := rules.Hosts[key(pin)].Login; l == nil || l.Mode != "pin" || l.Hours != 168 || strings.Join(l.Exclude, ",") != "/api/" || l.PassUser {
		t.Errorf("pin host: %+v", l)
	}
	if l := rules.Hosts[key(users)].Login; l == nil || l.Mode != "users" || l.Hours != 24 || len(l.Exclude) != 0 || l.Exclude == nil || !l.PassUser {
		t.Errorf("users host: %+v", l)
	}
	if l := rules.Hosts[key(page)].Login; l == nil || l.Mode != "pin" {
		t.Errorf("static page host: %+v", l)
	}

	conf := readFile(t, filepath.Join(dir, "nginx.conf"))
	for _, w := range []string{
		"lua_shared_dict corvus_signin 4m;",
		"limit_req_zone $corvus_login_key zone=corvus_login_client:10m rate=2r/s;",
		"POST    $corvus_client;",
		"location = /signin-sessions {",
		`require("corvus.rules").set_signin_sessions()`,
	} {
		if !strings.Contains(conf, w) {
			t.Errorf("nginx.conf: missing %q", w)
		}
	}

	hostConf := func(id int64) string { return readFile(t, filepath.Join(dir, "hosts", key(id)+".conf")) }
	loginLocs := []string{
		"location = /__corvus/login {",
		"limit_req zone=corvus_login_client burst=10 nodelay;",
		`content_by_lua_block { require("corvus.rules").login_page(` + key(users) + `) }`,
		"location = /__corvus/logout {",
		"location = /__corvus/signin-check {",
		"internal;",
		"proxy_pass_request_headers off;",
		"proxy_pass http://unix:/data/run/corvus-signin.sock:/v1/signin;",
	}
	u := hostConf(users)
	for _, w := range loginLocs {
		if !strings.Contains(u, w) {
			t.Errorf("users host: missing %q", w)
		}
	}
	// the internal location must not run the rules (and the login check) again
	if i := strings.Index(u, "location = /__corvus/signin-check {"); i < 0 || !strings.Contains(u[i:i+200], "rewrite_by_lua_block { return }") {
		t.Errorf("credential check location runs the rewrite phase")
	}
	// X-Corvus-User in every proxy location: the root (backend pool) and the custom location
	if n := strings.Count(u, "proxy_set_header X-Corvus-User $corvus_user;"); n != 2 {
		t.Errorf("users host: X-Corvus-User set in %d locations, want 2", n)
	}
	if !strings.Contains(u, `set $corvus_user "";`) {
		t.Error("users host: $corvus_user not initialised")
	}
	for _, id := range []int64{plain, off} {
		h := hostConf(id)
		if strings.Contains(h, "/__corvus/login") || strings.Contains(h, "corvus-signin.sock") {
			t.Errorf("host %d without login page has login locations", id)
		}
		if !strings.Contains(h, "proxy_set_header X-Corvus-User $corvus_user;") || !strings.Contains(h, `set $corvus_user "";`) {
			t.Errorf("host %d: client X-Corvus-User not replaced", id)
		}
	}
	if p := hostConf(page); !strings.Contains(p, "location = /__corvus/login {") {
		t.Error("static page host without login page locations")
	}
}
