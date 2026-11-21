package compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/store"
)

// Hosts that serve a page: no upstream block for the host, location / serves the page
// file of the version directory (built-in templates rendered and escaped, custom HTML
// byte for byte) with the chosen status, and the host's protections stay in place:
// security rules and the access log (server phases), Basic auth, rate limit, Force HTTPS
// and HSTS, the access queue. Custom locations keep forwarding.
func TestRenderStaticPages(t *testing.T) {
	c, st := renderPagesEnv(t)
	c.P.DataDir = filepath.ToSlash(t.TempDir())
	ctx := context.Background()

	cert := &store.Certificate{Name: "c", Kind: "custom", Domains: []string{"maint.example.com"}, NotAfter: time.Now().Add(90 * 24 * time.Hour), Status: "valid"}
	if err := st.CreateCertificate(ctx, cert); err != nil {
		t.Fatal(err)
	}
	certDir := filepath.FromSlash(c.P.CertDir(cert.ID))
	_ = os.MkdirAll(certDir, 0o755)
	_ = os.WriteFile(filepath.Join(certDir, "fullchain.pem"), []byte("CERT"), 0o644)
	_ = os.WriteFile(filepath.Join(certDir, "privkey.pem"), []byte("KEY"), 0o600)
	rl := &store.RateLimit{Name: "rl", RequestsPerSecond: 10, StatusCode: 429, Enabled: true, ExemptCIDRs: []string{}}
	if err := st.SaveRateLimit(ctx, rl); err != nil {
		t.Fatal(err)
	}
	acl := &store.AccessList{Name: "office", Users: []store.AccessListUser{{Username: "bob", PasswordHash: "$2y$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01234"}}}
	if err := st.SaveAccessList(ctx, acl); err != nil {
		t.Fatal(err)
	}
	room := &store.Waitroom{Name: "event", Enabled: true, Capacity: 500, ActiveTTL: 120, GrantTTL: 30, QueueTTL: 120, Refresh: 15, FailMode: "open"}
	if err := st.SaveWaitroom(ctx, room); err != nil {
		t.Fatal(err)
	}

	custom := "<!doctype html>\r\n<title>{{HOST}} ' \" $host ; }</title>\n<p>Ünïcode ✓</p>\n"
	hosts := []*store.ProxyHost{
		// 1: parked, defaults
		{Domains: []string{"parked.example.com"}, Content: store.ContentPage, Page: store.StaticPage{Template: store.PageParked}},
		// 2: maintenance with the operator's text (escaped), HTTPS, Basic auth, rate limit,
		// a location that still forwards, and a forward target kept for switching back
		{Domains: []string{"maint.example.com"}, Content: store.ContentPage, ForwardScheme: "http", ForwardHost: "old-app", ForwardPort: 8080,
			Page:          store.StaticPage{Template: store.PageMaintenance, Title: `Back <b>soon</b>`, Message: "We & you\n<script>alert(1)</script>"},
			CertificateID: &cert.ID, SSLForced: true, RateLimitID: &rl.ID, AccessListID: &acl.ID,
			Locations: []store.Location{{Path: "/api", ForwardScheme: "http", ForwardHost: "api", ForwardPort: 9000}}},
		// 3: custom HTML, 410
		{Domains: []string{"gone.example.com"}, Content: store.ContentPage, Page: store.StaticPage{Template: store.PageCustom, Status: 410, HTML: custom}},
		// 4: coming soon behind the access queue on a sub path
		{Domains: []string{"soon.example.com"}, Content: store.ContentPage, Page: store.StaticPage{Template: store.PageComingSoon, Status: 404},
			WaitroomID: &room.ID, WaitroomPath: "/shop"},
		// 5: a host that forwards, with a page kept from earlier: unchanged
		{Domains: []string{"fwd.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80,
			Page: store.StaticPage{Template: store.PageCustom, HTML: "<p>kept</p>"}},
	}
	for _, h := range hosts {
		h.Enabled = true
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(t.TempDir(), "v1")
	if err := c.Render(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
	conf := func(id int64) string { return readFile(t, filepath.Join(dir, "hosts", fmt.Sprintf("%d.conf", id))) }
	page := func(id int64) string {
		return readFile(t, filepath.Join(dir, "pages", fmt.Sprintf("host-%d.html", id)))
	}
	file := func(id int64) string {
		return filepath.ToSlash(filepath.Join(dir, "pages", fmt.Sprintf("host-%d.html", id)))
	}
	serve := func(id int64, status int, host bool) string {
		s := fmt.Sprintf(`content_by_lua_block { require("corvus.page").serve("%s", %d`, file(id), status)
		if host {
			s += ", true"
		}
		return s + ") }"
	}
	has := func(name, s string, want ...string) {
		t.Helper()
		for _, w := range want {
			if !strings.Contains(s, w) {
				t.Errorf("%s: missing %q\n%s", name, w, s)
			}
		}
	}
	hasNot := func(name, s string, deny ...string) {
		t.Helper()
		for _, d := range deny {
			if strings.Contains(s, d) {
				t.Errorf("%s: unexpected %q", name, d)
			}
		}
	}
	common := []string{`rewrite_by_lua_block { require("corvus.rules").rewrite(`, `log_by_lua_block { require("corvus.rules").log(`, "access_log off;", "location ^~ /.well-known/acme-challenge/ {"}

	c1 := conf(hosts[0].ID)
	has("parked", c1, append(common, serve(hosts[0].ID, 200, true), "# The host serves a static page (parked, status 200)", `set $corvus_upstream "";`)...)
	hasNot("parked", c1, "upstream corvus_up_", "proxy_pass http://corvus_up_", "# Forward host with")
	p1 := page(hosts[0].ID)
	has("parked page", p1, `<html lang="en">`, "<h1 id=\"h\">This domain is parked</h1>", PageHostMarker, `<meta name="robots" content="noindex">`, `"ko":["주차된 도메인"`, "own = [false,false]")

	c2 := conf(hosts[1].ID)
	has("maintenance", c2, append(common, serve(hosts[1].ID, 503, true),
		"auth_basic $corvus_auth;", "limit_req zone=corvus_rl", "if ($corvus_https_redirect) { return 301 https://$host$request_uri; }",
		`add_header Strict-Transport-Security "max-age=31536000" always;`,
		"upstream corvus_up_2_1 {", "location ^~ /api {", "proxy_pass http://corvus_up_2_1;")...)
	hasNot("maintenance", c2, "upstream corvus_up_2 {", "old-app")
	// the page location and the /api location both carry the guard
	if n := strings.Count(c2, "auth_basic $corvus_auth;"); n != 2 {
		t.Errorf("maintenance: Basic auth in %d locations, want 2 (/ and /api)", n)
	}
	p2 := page(hosts[1].ID)
	has("maintenance page", p2, "<title>Back &lt;b&gt;soon&lt;/b&gt;</title>", "We &amp; you\n&lt;script&gt;alert(1)&lt;/script&gt;", `<body class="maintenance">`, "<html>", "own = [true,true]")
	hasNot("maintenance page", p2, "<script>alert(1)", "<b>soon", `lang="en"`)

	c3 := conf(hosts[2].ID)
	has("custom", c3, serve(hosts[2].ID, 410, false))
	if got := page(hosts[2].ID); got != custom {
		t.Errorf("custom page not byte for byte:\n%q\n%q", got, custom)
	}

	c4 := conf(hosts[3].ID)
	has("queue", c4, `location ~* "^/shop(?:[/;]|$)" {`, serve(hosts[3].ID, 404, true), `require("corvus.waitroom").access()`)
	if n := strings.Count(c4, `require("corvus.page").serve(`); n != 2 {
		t.Errorf("queue: page served by %d locations, want 2 (/shop and /)", n)
	}

	c5 := conf(hosts[4].ID)
	has("forward", c5, "upstream corvus_up_5 {", "proxy_pass http://corvus_up_5;", "# Forward host with")
	hasNot("forward", c5, "corvus.page", "static page")
	if _, err := os.Stat(filepath.Join(dir, "pages", fmt.Sprintf("host-%d.html", hosts[4].ID))); !os.IsNotExist(err) {
		t.Errorf("a forwarding host got a page file: %v", err)
	}

	// a disabled page host renders nothing
	hosts[0].Enabled = false
	if err := st.SaveProxyHost(ctx, hosts[0]); err != nil {
		t.Fatal(err)
	}
	dir2 := filepath.Join(t.TempDir(), "v2")
	if err := c.Render(ctx, st, dir2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir2, "pages", fmt.Sprintf("host-%d.html", hosts[0].ID))); !os.IsNotExist(err) {
		t.Errorf("disabled page host: %v", err)
	}

	// a stored page host that fails validation is never rendered
	bad := &store.ProxyHost{Domains: []string{"bad.example.com"}, Content: store.ContentPage, Enabled: true, Page: store.StaticPage{Template: "nope"}}
	if err := st.SaveProxyHost(ctx, bad); err != nil {
		t.Fatal(err)
	}
	if err := c.Render(ctx, st, filepath.Join(t.TempDir(), "v3")); err == nil || !strings.Contains(err.Error(), "page.template") {
		t.Errorf("invalid page rendered: %v", err)
	}
}

// Built-in templates in each language; the preview fills in the host name (escaped).
func TestRenderStaticPageTemplates(t *testing.T) {
	for _, tpl := range []string{store.PageParked, store.PageComingSoon, store.PageMaintenance} {
		for lang, texts := range pageTexts[tpl] {
			b, err := RenderStaticPage(store.StaticPage{Template: tpl, Status: 200}, lang, "")
			if err != nil {
				t.Fatal(err)
			}
			s := string(b)
			if !strings.Contains(s, fmt.Sprintf(`<html lang="%s">`, lang)) || !strings.Contains(s, ">"+texts[1]+"</h1>") || !strings.Contains(s, PageHostMarker) {
				t.Errorf("%s/%s:\n%s", tpl, lang, s)
			}
		}
		if len(pageTexts[tpl]) != 8 {
			t.Errorf("%s: %d languages, want 8", tpl, len(pageTexts[tpl]))
		}
	}
	b, err := PreviewStaticPage(store.StaticPage{Template: store.PageComingSoon}, "xx", `*.a"<b>.example.com`)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(b); !strings.Contains(s, `a&#34;&lt;b&gt;.example.com`) || strings.Contains(s, PageHostMarker) || !strings.Contains(s, ">Coming soon</h1>") {
		t.Errorf("preview:\n%s", s)
	}
	if b, _ := PreviewStaticPage(store.StaticPage{Template: store.PageParked}, "ko", ""); !strings.Contains(string(b), "example.com") || !strings.Contains(string(b), "이 도메인은 주차되어 있습니다") {
		t.Errorf("preview without host:\n%s", b)
	}
	if b, _ := PreviewStaticPage(store.StaticPage{Template: store.PageCustom, HTML: "<p>{{HOST}}</p>"}, "ko", "x.example.com"); string(b) != "<p>{{HOST}}</p>" {
		t.Errorf("custom preview changed: %s", b)
	}
	if _, err := RenderStaticPage(store.StaticPage{Template: "nope"}, "en", ""); err == nil {
		t.Error("unknown template rendered")
	}
}
