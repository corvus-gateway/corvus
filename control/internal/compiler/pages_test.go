package compiler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

func renderPagesEnv(t *testing.T) (*Compiler, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	p := Paths{DataDir: "/data", LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181, DefaultCert: "/data/certs/default"}
	return &Compiler{P: p}, st
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRenderErrorPagesAndCustomPages(t *testing.T) {
	c, st := renderPagesEnv(t)
	ctx := context.Background()
	rl := &store.RateLimit{Name: "api", RequestsPerSecond: 10, StatusCode: 503, Enabled: true, ExemptCIDRs: []string{}}
	if err := st.SaveRateLimit(ctx, rl); err != nil {
		t.Fatal(err)
	}
	plain := &store.ProxyHost{Domains: []string{"a.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true}
	limited := &store.ProxyHost{Domains: []string{"b.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true, RateLimitID: &rl.ID}
	own := &store.ProxyHost{Domains: []string{"c.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true,
		AdvancedConfig: "error_page 502 /oops.html;"}
	for _, h := range []*store.ProxyHost{plain, limited, own} {
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
	}

	// Built-in pages: referenced from the HTML directory, nothing written to pages/.
	dir := filepath.Join(t.TempDir(), "v1")
	if err := c.Render(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
	var rules rulesFile
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "rules.json"))), &rules); err != nil {
		t.Fatal(err)
	}
	if rules.BlockPage != "/opt/corvus/html/blocked.html" || rules.ErrorPage != "/opt/corvus/html/error.html" {
		t.Errorf("default pages: %q %q", rules.BlockPage, rules.ErrorPage)
	}
	if _, err := os.Stat(filepath.Join(dir, "pages")); !os.IsNotExist(err) {
		t.Error("pages/ written without custom pages")
	}
	conf := readFile(t, filepath.Join(dir, "hosts", "1.conf"))
	for _, want := range []string{
		"error_page 502 @corvus_e502;", "error_page 503 @corvus_e503;", "error_page 504 @corvus_e504;",
		"location @corvus_e502 {", `content_by_lua_block { require("corvus.rules").upstream_error(504) }`,
		"rewrite_by_lua_block { return }",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("host config missing %q:\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "proxy_intercept_errors") {
		t.Error("upstream responses must not be intercepted")
	}
	// The ACME location keeps nginx's own error handling.
	acme := conf[strings.Index(conf, "location ^~ /.well-known/acme-challenge/"):]
	if acme = acme[:strings.Index(acme, "}")]; strings.Contains(acme, "error_page") {
		t.Errorf("ACME location has an error_page: %s", acme)
	}
	// A rate limit answering 503 keeps its own error page; 503 is not taken over.
	lim := readFile(t, filepath.Join(dir, "hosts", "2.conf"))
	if strings.Contains(lim, "@corvus_e503") || !strings.Contains(lim, "error_page 502 @corvus_e502;") {
		t.Errorf("rate-limited host error pages:\n%s", lim)
	}
	if strings.Count(lim, "error_page 503 = @corvus_limited;") != 2 { // server + location /
		t.Errorf("rate limit error_page not repeated in the proxy location:\n%s", lim)
	}

	// Advanced configuration with its own error_page keeps it (no location-level override).
	if o := readFile(t, filepath.Join(dir, "hosts", "3.conf")); strings.Contains(o, "@corvus_e") {
		t.Errorf("host with its own error_page got Corvus error pages:\n%s", o)
	}

	// Custom pages: written verbatim into the version directory, placeholders preserved.
	block := "<!doctype html><h1>Nope {{STATUS}}</h1><p>{{RID}} {{HOST}}</p>"
	errPage := "<!doctype html><h1>Down {{STATUS}}</h1>{{RID}}"
	if err := st.SetCustomPages(ctx, store.CustomPages{Block: block, Error: errPage}); err != nil {
		t.Fatal(err)
	}
	dir2 := filepath.Join(t.TempDir(), "v2")
	if err := c.Render(ctx, st, dir2); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir2, "pages", "blocked.html")); got != block {
		t.Errorf("block page = %q", got)
	}
	if got := readFile(t, filepath.Join(dir2, "pages", "error.html")); got != errPage {
		t.Errorf("error page = %q", got)
	}
	rules = rulesFile{}
	_ = json.Unmarshal([]byte(readFile(t, filepath.Join(dir2, "rules.json"))), &rules)
	if rules.BlockPage != filepath.ToSlash(filepath.Join(dir2, "pages", "blocked.html")) || rules.ErrorPage != filepath.ToSlash(filepath.Join(dir2, "pages", "error.html")) {
		t.Errorf("custom page paths: %q %q", rules.BlockPage, rules.ErrorPage)
	}
	for _, f := range []string{"nginx.conf", filepath.Join("hosts", "1.conf")} {
		if strings.Contains(readFile(t, filepath.Join(dir2, f)), "Nope") {
			t.Errorf("page content leaked into %s", f)
		}
	}

	// An invalid stored page (e.g. written by an older build) never reaches the gateway.
	if err := st.SetCustomPages(ctx, store.CustomPages{Error: strings.Repeat("x", 64*1024+1)}); err != nil {
		t.Fatal(err)
	}
	if err := c.Render(ctx, st, filepath.Join(t.TempDir(), "v3")); err == nil {
		t.Error("oversized page rendered")
	}
}
