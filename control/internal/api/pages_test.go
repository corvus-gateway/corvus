package api

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/compiler"
	"corvus/control/internal/deploy"
	"corvus/control/internal/store"
)

type okRunner struct{}

func (okRunner) Test(context.Context, string) (string, error) { return "ok", nil }
func (okRunner) Reload() error                                { return nil }

func (e *testEnv) session(email, role string) string {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.s.Store.CreateUser(ctx, email, role, "x", role)
	if err != nil {
		e.t.Fatal(err)
	}
	raw := auth.RandomToken(32)
	if err := e.s.Store.CreateSession(ctx, auth.TokenHash(raw), u.ID, time.Hour, "", ""); err != nil {
		e.t.Fatal(err)
	}
	return raw
}

func TestCustomPagesPermissionsAndValidation(t *testing.T) {
	e := newTestEnv(t)
	viewer := e.session("viewer@example.com", "viewer")
	operator := e.session("op@example.com", "operator")
	body := map[string]string{"html": "<p>{{STATUS}}</p>"}

	if w, _ := e.do("GET", "/api/settings/pages", nil); w.Code != 401 {
		t.Errorf("anonymous GET: %d", w.Code)
	}
	w, m := e.do("GET", "/api/settings/pages", nil, withCookie(viewer))
	if w.Code != 200 || m["max_bytes"] != float64(64*1024) {
		t.Errorf("viewer GET: %d %v", w.Code, m)
	}
	if p, _ := m["pages"].(map[string]any); p == nil || p["block"] == nil || p["error"] == nil {
		t.Errorf("GET pages: %v", m)
	}
	for _, rq := range []struct{ method, path string }{{"PUT", "/api/settings/pages/block"}, {"POST", "/api/settings/pages/error/reset"}} {
		if w, _ := e.do(rq.method, rq.path, body, withCookie(viewer), withCSRF()); w.Code != 403 {
			t.Errorf("viewer %s %s: %d, want 403", rq.method, rq.path, w.Code)
		}
		if w, _ := e.do(rq.method, rq.path, body); w.Code != 403 && w.Code != 401 {
			t.Errorf("anonymous %s %s: %d", rq.method, rq.path, w.Code)
		}
	}
	// Validation runs before anything is deployed.
	for _, c := range []struct {
		html string
		code int
	}{{"", 422}, {strings.Repeat("a", 64*1024+1), 422}} {
		if w, _ := e.do("PUT", "/api/settings/pages/error", map[string]string{"html": c.html}, withCookie(operator), withCSRF()); w.Code != c.code {
			t.Errorf("operator PUT %d bytes: %d, want %d", len(c.html), w.Code, c.code)
		}
	}
	if w, _ := e.do("PUT", "/api/settings/pages/other", body, withCookie(operator), withCSRF()); w.Code != 404 {
		t.Errorf("unknown page: %d", w.Code)
	}
}

func TestCustomPagesSaveAndReset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("deployment uses POSIX symlink swaps; run on Linux")
	}
	e := newTestEnv(t)
	dir := t.TempDir()
	html := filepath.Join(dir, "html")
	_ = os.MkdirAll(html, 0o755)
	_ = os.WriteFile(filepath.Join(html, "error.html"), []byte("<p>built-in {{STATUS}}</p>"), 0o644)
	p := compiler.Paths{DataDir: dir, LuaDir: "/opt/corvus/lua", HTMLDir: filepath.ToSlash(html), MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181}
	e.s.Paths = p
	e.s.Deployer = &deploy.Deployer{Store: e.s.Store, Compiler: &compiler.Compiler{P: p}, Runner: okRunner{}, Dir: filepath.Join(dir, "nginx")}
	operator := e.session("op@example.com", "operator")

	custom := "<!doctype html><h1>Down {{STATUS}} {{RID}}</h1>"
	w, m := e.do("PUT", "/api/settings/pages/error", map[string]string{"html": custom}, withCookie(operator), withCSRF())
	if w.Code != 200 || m["version"] == nil {
		t.Fatalf("PUT: %d %s", w.Code, w.Body.String())
	}
	if pg := m["page"].(map[string]any); pg["custom"] != true || pg["html"] != custom {
		t.Errorf("PUT result: %v", pg)
	}
	got, err := os.ReadFile(filepath.Join(dir, "nginx", "current", "pages", "error.html"))
	if err != nil || string(got) != custom {
		t.Errorf("deployed page: %q %v", got, err)
	}

	w, m = e.do("POST", "/api/settings/pages/error/reset", nil, withCookie(operator), withCSRF())
	if w.Code != 200 {
		t.Fatalf("reset: %d %s", w.Code, w.Body.String())
	}
	if pg := m["page"].(map[string]any); pg["custom"] != false || pg["html"] != "<p>built-in {{STATUS}}</p>" {
		t.Errorf("reset result: %v", pg)
	}
	if pages, _ := e.s.Store.CustomPages(context.Background()); pages != (store.CustomPages{}) {
		t.Errorf("stored pages after reset: %+v", pages)
	}
}

func TestUnknownHostsPermissionsAndValidation(t *testing.T) {
	e := newTestEnv(t)
	viewer := e.session("viewer@example.com", "viewer")
	operator := e.session("op@example.com", "operator")
	w, m := e.do("GET", "/api/settings/pages", nil, withCookie(viewer))
	if u, _ := m["unknown_hosts"].(map[string]any); w.Code != 200 || u["mode"] != "not_found" {
		t.Errorf("GET unknown hosts: %d %v", w.Code, m["unknown_hosts"])
	}
	body := map[string]string{"mode": "close"}
	if w, _ := e.do("PUT", "/api/settings/unknown-hosts", body, withCookie(viewer), withCSRF()); w.Code != 403 {
		t.Errorf("viewer PUT: %d, want 403", w.Code)
	}
	for _, b := range []map[string]string{{"mode": "drop"}, {"mode": "redirect"}, {"mode": "redirect", "redirect_url": "https://example.com/$host"}} {
		if w, _ := e.do("PUT", "/api/settings/unknown-hosts", b, withCookie(operator), withCSRF()); w.Code != 422 {
			t.Errorf("operator PUT %v: %d, want 422", b, w.Code)
		}
	}
	if u, _ := e.s.Store.UnknownHosts(context.Background()); u.Mode != store.UnknownNotFound {
		t.Errorf("invalid input stored: %+v", u)
	}
}

func TestUnknownHostsSave(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("deployment uses POSIX symlink swaps; run on Linux")
	}
	e := newTestEnv(t)
	dir := t.TempDir()
	p := compiler.Paths{DataDir: dir, LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181}
	e.s.Paths = p
	e.s.Deployer = &deploy.Deployer{Store: e.s.Store, Compiler: &compiler.Compiler{P: p}, Runner: okRunner{}, Dir: filepath.Join(dir, "nginx")}
	operator := e.session("op@example.com", "operator")

	w, m := e.do("PUT", "/api/settings/unknown-hosts", map[string]string{"mode": "redirect", "redirect_url": "https://Example.com/start"}, withCookie(operator), withCSRF())
	if w.Code != 200 || m["version"] == nil {
		t.Fatalf("PUT: %d %s", w.Code, w.Body.String())
	}
	conf, err := os.ReadFile(filepath.Join(dir, "nginx", "current", "nginx.conf"))
	if err != nil || !strings.Contains(string(conf), "return 302 https://example.com/start;") {
		t.Errorf("deployed config: %v\n%s", err, conf)
	}
	if u, _ := e.s.Store.UnknownHosts(context.Background()); u != (store.UnknownHosts{Mode: "redirect", RedirectURL: "https://example.com/start"}) {
		t.Errorf("stored: %+v", u)
	}
}
