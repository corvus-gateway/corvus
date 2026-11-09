package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/sigs"
	"corvus/control/internal/store"
)

// ------------------------------------------------------------------ rule templates & managed rules (templates.go)

// zzhRules is a RulesService with the built-in bundle and scripted check/install results.
type zzhRules struct {
	b          *sigs.Bundle
	checkErr   error
	installErr error
	installed  []string
	checks     int
}

func (f *zzhRules) Bundle() *sigs.Bundle { return f.b }
func (f *zzhRules) Status(context.Context) sigs.Status {
	return sigs.Status{Serial: f.b.Serial, Version: f.b.Version, Source: "builtin", Templates: len(f.b.Templates)}
}
func (f *zzhRules) Check(ctx context.Context) (sigs.Status, error) {
	f.checks++
	return f.Status(ctx), f.checkErr
}
func (f *zzhRules) Install(ctx context.Context, data, sig []byte) (sigs.Status, error) {
	if f.installErr != nil {
		return f.Status(ctx), f.installErr
	}
	f.installed = append(f.installed, string(data)+"|"+string(sig))
	st := f.Status(ctx)
	st.Version, st.Source = "uploaded", "installed"
	return st, nil
}

func newZZHRules(t *testing.T, st *store.Store) *zzhRules {
	t.Helper()
	svc := &sigs.Service{Dir: t.TempDir(), Store: st}
	if err := svc.Init(); err != nil {
		t.Fatal(err)
	}
	return &zzhRules{b: svc.Bundle()}
}

func TestZZHRuleTemplates(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()
	viewer, operator := e.as("viewer"), e.as("operator")

	// without a bundle
	if code, l := e.callList("GET", "/api/rule-templates"); code != 200 || len(l) != 0 {
		t.Fatalf("templates without bundle: %d %v", code, l)
	}
	if code, m := e.call("POST", "/api/security-rules/from-template", map[string]any{"template_id": "wp2shell"}); code != 404 || m["error"] != "not_found" {
		t.Fatalf("from template without bundle: %d %v", code, m)
	}

	rules := newZZHRules(t, e.s.Store)
	e.s.Rules = rules
	code, l := e.zzhAs(viewer, "GET", "/api/rule-templates", nil)
	_ = l
	if code != 200 {
		t.Fatalf("viewer templates: %d", code)
	}
	_, list := e.callList("GET", "/api/rule-templates")
	if len(list) != len(rules.b.Templates) || len(list) == 0 {
		t.Fatalf("templates: %d of %d", len(list), len(rules.b.Templates))
	}
	needs := map[string]bool{}
	for _, x := range list {
		xm := x.(map[string]any)
		needs[xm["id"].(string)] = xm["needs_exempt"].(bool)
	}
	if !needs["wp2shell"] || needs["nextjs-middleware-bypass"] {
		t.Fatalf("needs_exempt: %v", needs)
	}

	// a rule from a template starts in log-only mode
	m := e.zzhExpect(201, "POST", "/api/security-rules/from-template", map[string]any{"template_id": "nextjs-middleware-bypass"})
	r := zzhObj(t, m, "rule")
	if r["action"] != "log" || r["scope"] != "all" || !strings.Contains(r["name"].(string), "CVE-2025-29927") || zzhObj(t, m, "version")["status"] != "active" {
		t.Fatalf("rule from template: %v", m)
	}
	if l, _ := e.s.Store.ListSecurityRules(ctx); len(l) != 1 {
		t.Fatalf("rule not stored: %d", len(l))
	}
	// an operator may create one too, scoped to a host and blocking
	hid := e.zzhCreateHost(zzhHost("wp.example.com"))
	code, m = e.zzhAs(operator, "POST", "/api/security-rules/from-template", map[string]any{"template_id": "wp2shell", "action": "block", "status_code": 404,
		"scope": "hosts", "host_ids": []int64{hid}})
	if code != 201 {
		t.Fatalf("operator from template: %d %v", code, m)
	}
	if r := zzhObj(t, m, "rule"); r["action"] != "block" || r["status_code"] != float64(404) {
		t.Fatalf("scoped rule: %v", r)
	}
	if code, _ := e.zzhAs(viewer, "POST", "/api/security-rules/from-template", map[string]any{"template_id": "wp2shell"}); code != 403 {
		t.Fatalf("viewer from template: %d", code)
	}

	m = e.zzhExpect(422, "POST", "/api/security-rules/from-template", map[string]any{"template_id": "nope"})
	zzhHasField(t, m, "template_id", "not_found")
	m = e.zzhExpect(422, "POST", "/api/security-rules/from-template", map[string]any{"template_id": "wp2shell", "scope": "hosts", "host_ids": []int64{999}})
	zzhHasField(t, m, "host_ids", "not_found")
	m = e.zzhExpect(422, "POST", "/api/security-rules/from-template", map[string]any{"template_id": "wp2shell", "action": "explode"})
	zzhHasField(t, m, "action", "invalid")
	// the exempt placeholder must name an existing IP list
	m = e.zzhExpect(422, "POST", "/api/security-rules/from-template", map[string]any{"template_id": "wp2shell", "exempt_list_id": 999})
	if m["error"] != "validation" || !strings.Contains(strings.Join(zzhFields(m), ","), "expression") {
		t.Fatalf("missing exempt list: %v", m)
	}
	e.zzhExpect(400, "POST", "/api/security-rules/from-template", map[string]any{"template": "wp2shell"})

	// the rule limit
	err := e.s.Store.WithTx(ctx, func(tx *store.Store) error {
		for i := 2; i < 200; i++ {
			rule := rules.b.Templates[0].Rule("log", 0, "all", nil, 0)
			if err := tx.SaveSecurityRule(ctx, &rule); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if code, m := e.call("POST", "/api/security-rules/from-template", map[string]any{"template_id": "react2shell"}); code != 409 || m["error"] != "conflict" {
		t.Fatalf("rule limit: %d %v", code, m)
	}
	if !e.zzhAudited("security_rule.create") {
		t.Fatal("rule creation not audited")
	}
}

func TestZZHManagedRules(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()
	viewer, operator := e.as("viewer"), e.as("operator")

	if code, m := e.call("GET", "/api/managed-rules", nil); code != 404 || m["error"] != "not_found" {
		t.Fatalf("status without bundle: %d %v", code, m)
	}
	rules := newZZHRules(t, e.s.Store)
	e.s.Rules = rules
	code, m := e.zzhAs(viewer, "GET", "/api/managed-rules", nil)
	if code != 200 || m["source"] != "builtin" || m["templates"] != float64(len(rules.b.Templates)) {
		t.Fatalf("status: %d %v", code, m)
	}

	// settings: admin only, https URL
	for _, c := range []struct{ method, path string }{
		{"PUT", "/api/settings/managed-rules"}, {"POST", "/api/managed-rules/check"}, {"POST", "/api/managed-rules/upload"},
	} {
		if code, _ := e.zzhAs(operator, c.method, c.path, map[string]any{}); code != 403 {
			t.Fatalf("operator %s %s: %d", c.method, c.path, code)
		}
	}
	for _, bad := range []string{"http://rules.example.com/b.json", "https://a b", "https://" + strings.Repeat("x", 500)} {
		m := e.zzhExpect(422, "PUT", "/api/settings/managed-rules", map[string]any{"url": bad})
		zzhHasField(t, m, "url", "invalid")
	}
	e.zzhExpect(400, "PUT", "/api/settings/managed-rules", map[string]any{"uri": "x"})
	e.zzhExpect(200, "PUT", "/api/settings/managed-rules", map[string]any{"url": "  https://rules.example.com/bundle.json  ", "auto_update": true})
	if v, _ := e.s.Store.GetMeta(ctx, "managed_rules"); !strings.Contains(v, `"url":"https://rules.example.com/bundle.json"`) || !strings.Contains(v, `"auto_update":true`) {
		t.Fatalf("settings stored: %q", v)
	}
	e.zzhExpect(200, "PUT", "/api/settings/managed-rules", map[string]any{"url": ""})

	// check now
	rules.checkErr = errors.New("download failed")
	m = e.zzhExpect(502, "POST", "/api/managed-rules/check", nil)
	if m["error"] != "update_failed" || m["message"] != "download failed" || m["status"] == nil {
		t.Fatalf("failed check: %v", m)
	}
	rules.checkErr = nil
	if m := e.zzhExpect(200, "POST", "/api/managed-rules/check", nil); m["source"] != "builtin" || rules.checks != 2 {
		t.Fatalf("check: %v", m)
	}

	// upload a bundle
	e.zzhExpect(400, "POST", "/api/managed-rules/upload", "not an object")
	rules.installErr = errors.New("bad signature")
	m = e.zzhExpect(422, "POST", "/api/managed-rules/upload", map[string]any{"bundle": "{}", "signature": "c2ln"})
	zzhHasField(t, m, "bundle", "rejected")
	rules.installErr = nil
	if m := e.zzhExpect(200, "POST", "/api/managed-rules/upload", map[string]any{"bundle": "{}", "signature": "c2ln"}); m["source"] != "installed" {
		t.Fatalf("upload: %v", m)
	}
	if len(rules.installed) != 1 || rules.installed[0] != "{}|c2ln" {
		t.Fatalf("installed: %v", rules.installed)
	}
	for _, a := range []string{"managed_rules.update", "managed_rules.check", "managed_rules.upload"} {
		if !e.zzhAudited(a) {
			t.Errorf("audit %s missing", a)
		}
	}
}

// ------------------------------------------------------------------ custom pages (pages.go)

func TestZZHPages(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()
	viewer := e.as("viewer")
	html := t.TempDir()
	if err := os.WriteFile(filepath.Join(html, "blocked.html"), []byte("<p>built-in block</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.s.Paths.HTMLDir = html // error.html is missing: its default is ""

	code, m := e.zzhAs(viewer, "GET", "/api/settings/pages", nil)
	if code != 200 {
		t.Fatalf("viewer pages: %d", code)
	}
	pages := zzhObj(t, m, "pages")
	block, errPage := pages["block"].(map[string]any), pages["error"].(map[string]any)
	if block["custom"] != false || block["html"] != "<p>built-in block</p>" || block["default_html"] != "<p>built-in block</p>" ||
		errPage["html"] != "" || m["max_bytes"] == nil || len(m["placeholders"].([]any)) == 0 || zzhObj(t, m, "unknown_hosts")["mode"] == nil {
		t.Fatalf("pages: %v", m)
	}

	if code, m := e.call("PUT", "/api/settings/pages/other", map[string]any{"html": "x"}); code != 404 || m["error"] != "not_found" {
		t.Fatalf("unknown page: %d %v", code, m)
	}
	e.zzhExpect(404, "POST", "/api/settings/pages/other/reset", nil)
	m = e.zzhExpect(422, "PUT", "/api/settings/pages/block", map[string]any{"html": "   "})
	zzhHasField(t, m, "html", "required")
	m = e.zzhExpect(422, "PUT", "/api/settings/pages/error", map[string]any{"html": strings.Repeat("x", 64*1024+1)})
	zzhHasField(t, m, "html", "too_large")
	e.zzhExpect(400, "PUT", "/api/settings/pages/block", map[string]any{"body": "x"})
	if code, _ := e.zzhAs(viewer, "PUT", "/api/settings/pages/block", map[string]any{"html": "x"}); code != 403 {
		t.Fatalf("viewer put page: %d", code)
	}

	m = e.zzhExpect(200, "PUT", "/api/settings/pages/block", map[string]any{"html": "<h1>{{STATUS}} blocked</h1>"})
	p := zzhObj(t, m, "page")
	if p["custom"] != true || p["html"] != "<h1>{{STATUS}} blocked</h1>" || p["default_html"] != "<p>built-in block</p>" || zzhObj(t, m, "version")["status"] != "active" {
		t.Fatalf("put page: %v", m)
	}
	m = e.zzhExpect(200, "PUT", "/api/settings/pages/error", map[string]any{"html": "<h1>oops</h1>"})
	if zzhObj(t, m, "page")["html"] != "<h1>oops</h1>" {
		t.Fatalf("put error page: %v", m)
	}
	if cp, _ := e.s.Store.CustomPages(ctx); cp.Block != "<h1>{{STATUS}} blocked</h1>" || cp.Error != "<h1>oops</h1>" {
		t.Fatalf("stored pages: %+v", cp)
	}
	m = e.zzhExpect(200, "POST", "/api/settings/pages/block/reset", nil)
	if p := zzhObj(t, m, "page"); p["custom"] != false || p["html"] != "<p>built-in block</p>" {
		t.Fatalf("reset: %v", p)
	}
	if cp, _ := e.s.Store.CustomPages(ctx); cp.Block != "" || cp.Error != "<h1>oops</h1>" {
		t.Fatalf("stored after reset: %+v", cp)
	}
	if !e.zzhAudited("page.update") || !e.zzhAudited("page.reset") {
		t.Fatal("page changes not audited")
	}

	// a rejected deployment keeps the stored page
	e.zzhBreakGateway()
	if m := e.zzhExpect(422, "PUT", "/api/settings/pages/error", map[string]any{"html": "<h1>new</h1>"}); m["error"] != "config_rejected" {
		t.Fatalf("rejected page: %v", m)
	}
	if cp, _ := e.s.Store.CustomPages(ctx); cp.Error != "<h1>oops</h1>" {
		t.Fatalf("rejected page stored: %+v", cp)
	}
	if m := e.zzhExpect(422, "PUT", "/api/settings/unknown-hosts", map[string]any{"mode": "close"}); m["error"] != "config_rejected" {
		t.Fatalf("rejected unknown hosts: %v", m)
	}
	if u, _ := e.s.Store.UnknownHosts(ctx); u.Mode == "close" {
		t.Fatalf("rejected unknown hosts stored: %+v", u)
	}
}

func TestZZHUnknownHosts(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()
	operator := e.as("operator")

	for _, c := range []struct {
		body  map[string]any
		field string
		code  string
	}{
		{map[string]any{"mode": "teapot"}, "mode", "invalid"},
		{map[string]any{"mode": "redirect"}, "redirect_url", "required"},
		{map[string]any{"mode": "redirect", "redirect_url": "ftp://x.example.com"}, "redirect_url", "invalid"},
		{map[string]any{"mode": "redirect", "redirect_url": "https://x.example.com:0/"}, "redirect_url", "invalid"},
		{map[string]any{"mode": "redirect", "redirect_url": "https://x.example.com/a b"}, "redirect_url", "invalid"},
	} {
		m := e.zzhExpect(422, "PUT", "/api/settings/unknown-hosts", c.body)
		zzhHasField(t, m, c.field, c.code)
	}
	e.zzhExpect(400, "PUT", "/api/settings/unknown-hosts", map[string]any{"url": "x"})

	code, m := e.zzhAs(operator, "PUT", "/api/settings/unknown-hosts", map[string]any{"mode": "redirect", "redirect_url": "HTTPS://WWW.Example.com/landing"})
	if code != 200 {
		t.Fatalf("operator redirect: %d %v", code, m)
	}
	if u := zzhObj(t, m, "unknown_hosts"); u["redirect_url"] != "https://www.example.com/landing" {
		t.Fatalf("normalized: %v", u)
	}
	if u, _ := e.s.Store.UnknownHosts(ctx); u.Mode != "redirect" || u.RedirectURL != "https://www.example.com/landing" {
		t.Fatalf("stored: %+v", u)
	}
	m = e.zzhExpect(200, "PUT", "/api/settings/unknown-hosts", map[string]any{"mode": "close", "redirect_url": "https://ignored.example.com"})
	if u := zzhObj(t, m, "unknown_hosts"); u["mode"] != "close" || u["redirect_url"] != "" {
		t.Fatalf("close: %v", u)
	}
	if !e.zzhAudited("unknown_hosts.update") {
		t.Fatal("not audited")
	}
}

// ------------------------------------------------------------------ DNS credentials (dns_credentials.go)

func TestZZHDNSCredentials(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()
	viewer := e.as("viewer")
	e.s.ACME = &startRecorder{}

	if code, l := e.callList("GET", "/api/dns-credentials"); code != 200 || len(l) != 0 {
		t.Fatalf("empty list: %d %v", code, l)
	}
	m := e.zzhExpect(201, "POST", "/api/dns-credentials", map[string]any{"name": " Zone A ", "credentials": "dns_cloudflare_api_token=a"})
	a := id(m)
	if m["name"] != "Zone A" || m["provider"] != "cloudflare" {
		t.Fatalf("create: %v", m)
	}
	// without a name one is generated
	m = e.zzhExpect(201, "POST", "/api/dns-credentials", map[string]any{"credentials": "dns_cloudflare_api_token=b"})
	b := id(m)
	if m["name"] == "" {
		t.Fatalf("automatic name: %v", m)
	}
	for _, c := range []struct {
		body  map[string]any
		field string
	}{
		{map[string]any{"name": "x"}, "credentials"},
		{map[string]any{"name": "x", "provider": "nosuch", "credentials": "a=b"}, "credentials"},
		{map[string]any{"name": "x", "credentials": "no equals sign"}, "credentials"},
		{map[string]any{"name": strings.Repeat("n", 101), "credentials": "dns_cloudflare_api_token=c"}, "name"},
		{map[string]any{"name": "zone a", "credentials": "dns_cloudflare_api_token=c"}, "name"},
	} {
		m := e.zzhExpect(422, "POST", "/api/dns-credentials", c.body)
		zzhHasField(t, m, c.field, "")
	}
	e.zzhExpect(400, "POST", "/api/dns-credentials", map[string]any{"secret": "x"})

	// update
	m = e.zzhExpect(200, "PUT", zzhPath("/api/dns-credentials", a), map[string]any{"name": "Zone A2"})
	if m["name"] != "Zone A2" {
		t.Fatalf("rename: %v", m)
	}
	before, _ := e.s.Store.DNSCredentialByID(ctx, a)
	m = e.zzhExpect(200, "PUT", zzhPath("/api/dns-credentials", a), map[string]any{"credentials": "dns_cloudflare_api_token=new"})
	after, _ := e.s.Store.DNSCredentialByID(ctx, a)
	if m["name"] != "Zone A2" || after.Secret == before.Secret || after.Secret == "" {
		t.Fatalf("secret not replaced: %v", m)
	}
	for _, c := range []struct {
		body  map[string]any
		field string
		code  string
	}{
		{map[string]any{"name": "   "}, "name", "required"},
		{map[string]any{"name": strings.Repeat("n", 101)}, "name", "invalid"},
		{map[string]any{"name": "ZONE A2 "}, "", ""}, // own name, other case: allowed
		{map[string]any{"credentials": "garbage"}, "credentials", "invalid"},
		{map[string]any{"provider": "route53"}, "credentials", "required"},
	} {
		if c.field == "" {
			e.zzhExpect(200, "PUT", zzhPath("/api/dns-credentials", a), c.body)
			continue
		}
		m := e.zzhExpect(422, "PUT", zzhPath("/api/dns-credentials", a), c.body)
		zzhHasField(t, m, c.field, c.code)
	}
	bName := ""
	if l, _ := e.s.Store.ListDNSCredentials(ctx); len(l) == 2 {
		bName = l[1].Name
	}
	m = e.zzhExpect(422, "PUT", zzhPath("/api/dns-credentials", a), map[string]any{"name": strings.ToUpper(bName)})
	zzhHasField(t, m, "name", "invalid")
	e.zzhExpect(404, "PUT", "/api/dns-credentials/999", map[string]any{"name": "x"})
	e.zzhExpect(400, "PUT", "/api/dns-credentials/x", map[string]any{"name": "x"})
	e.zzhExpect(400, "PUT", zzhPath("/api/dns-credentials", a), map[string]any{"bogus": 1})

	// roles
	if code, _ := e.zzhAs(viewer, "GET", "/api/dns-credentials", nil); code != 200 {
		t.Fatalf("viewer list: %d", code)
	}
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/dns-credentials"}, {"PUT", zzhPath("/api/dns-credentials", a)}, {"DELETE", zzhPath("/api/dns-credentials", a)},
	} {
		if code, _ := e.zzhAs(viewer, c.method, c.path, map[string]any{"name": "v"}); code != 403 {
			t.Fatalf("viewer %s %s: %d", c.method, c.path, code)
		}
	}

	// delete: in use by a certificate → 422
	e.zzhExpect(202, "POST", "/api/certificates/acme", map[string]any{"domains": []string{"*.a.example.com"}, "email": "ops@example.com",
		"challenge": "dns-01", "dns_credential_id": a})
	m = e.zzhExpect(422, "DELETE", zzhPath("/api/dns-credentials", a), nil)
	zzhHasField(t, m, "id", "in_use")
	if m := e.zzhExpect(200, "DELETE", zzhPath("/api/dns-credentials", b), nil); m["status"] != "ok" {
		t.Fatalf("delete: %v", m)
	}
	e.zzhExpect(404, "DELETE", zzhPath("/api/dns-credentials", b), nil)
	e.zzhExpect(400, "DELETE", "/api/dns-credentials/0", nil)
	for _, act := range []string{"dns_credential.create", "dns_credential.update", "dns_credential.delete"} {
		if !e.zzhAudited(act) {
			t.Errorf("audit %s missing", act)
		}
	}
}

// ------------------------------------------------------------------ ACME (acme.go)

func TestZZHACME(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()
	viewer := e.as("viewer")

	if code, l := e.callList("GET", "/api/acme/dns-providers"); code != 200 || len(l) == 0 {
		t.Fatalf("providers: %d %v", code, l)
	}
	if code, m := e.call("POST", "/api/certificates/acme", map[string]any{"domains": []string{"a.example.com"}, "email": "ops@example.com"}); code != 501 || m["error"] != "unavailable" {
		t.Fatalf("issue without ACME: %d %v", code, m)
	}
	certPEM, keyPEM := selfSigned(t, 30, time.Now().Add(30*24*time.Hour), "custom.example.com")
	custom := id(e.zzhExpect(201, "POST", "/api/certificates", map[string]any{"certificate": certPEM, "private_key": keyPEM}))
	if code, m := e.call("POST", zzhPath("/api/certificates", custom)+"/renew", nil); code != 409 || m["error"] != "not_acme" {
		t.Fatalf("renew without ACME: %d %v", code, m)
	}

	rec := &startRecorder{}
	e.s.ACME = rec
	e.zzhExpect(400, "POST", "/api/certificates/acme", map[string]any{"domain": "x"})
	m := e.zzhExpect(422, "POST", "/api/certificates/acme", map[string]any{"domains": []string{" ", ""}, "email": "a@b"})
	zzhHasField(t, m, "domains", "required")
	zzhHasField(t, m, "email", "invalid")
	many := []string{}
	for i := 0; i < 51; i++ {
		many = append(many, "d"+itoa(int64(i))+".example.com")
	}
	m = e.zzhExpect(422, "POST", "/api/certificates/acme", map[string]any{"domains": many, "email": "ops@example.com"})
	zzhHasField(t, m, "domains", "required")
	m = e.zzhExpect(422, "POST", "/api/certificates/acme", map[string]any{"domains": []string{"bad_domain!.com"}, "email": "ops@example.com"})
	zzhHasField(t, m, "domains", "invalid_domain")
	if len(rec.started) != 0 {
		t.Fatalf("started for rejected orders: %v", rec.started)
	}
	if code, _ := e.zzhAs(viewer, "POST", "/api/certificates/acme", map[string]any{"domains": []string{"a.example.com"}, "email": "ops@example.com"}); code != 403 {
		t.Fatalf("viewer issue: %d", code)
	}

	m = e.zzhExpect(202, "POST", "/api/certificates/acme", map[string]any{"domains": []string{"A.example.com", "a.example.com", "www.a.example.com"},
		"email": " ops@example.com ", "name": " Site A ", "staging": true})
	cid := id(m)
	if m["name"] != "Site A" || m["status"] != "pending" || m["challenge"] != "http-01" || len(m["domains"].([]any)) != 2 {
		t.Fatalf("issue: %v", m)
	}
	if c, _ := e.s.Store.CertificateByID(ctx, cid); !c.ACME.Staging || c.ACME.Email != "ops@example.com" {
		t.Fatalf("stored order: %+v", c.ACME)
	}
	if len(rec.started) != 1 || rec.started[0] != cid {
		t.Fatalf("not started: %v", rec.started)
	}
	m = e.zzhExpect(202, "POST", "/api/certificates/acme", map[string]any{"domains": []string{"*.b.example.com"}, "email": "ops@example.com",
		"challenge": "dns-01", "dns_provider": "cloudflare", "dns_credentials": "dns_cloudflare_api_token=tok", "dns_credential_name": "B zone"})
	if m["name"] != "*.b.example.com" || m["dns_credential_id"] == nil {
		t.Fatalf("dns issue: %v", m)
	}
	if l, _ := e.s.Store.ListDNSCredentials(ctx); len(l) != 1 || l[0].Name != "B zone" {
		t.Fatalf("credential: %+v", l)
	}

	// renew: pending/error orders go back to pending, valid ones stay valid until renewed
	if err := e.s.Store.SetCertificateStatus(ctx, cid, "error", "rate limited"); err != nil {
		t.Fatal(err)
	}
	m = e.zzhExpect(202, "POST", zzhPath("/api/certificates", cid)+"/renew", nil)
	if m["status"] != "pending" {
		t.Fatalf("renew failed order: %v", m)
	}
	if c, _ := e.s.Store.CertificateByID(ctx, cid); c.Status != "pending" || c.LastError != "" {
		t.Fatalf("stored after renew: %+v", c)
	}
	if err := e.s.Store.SetCertificateStatus(ctx, cid, "valid", ""); err != nil {
		t.Fatal(err)
	}
	if m := e.zzhExpect(202, "POST", zzhPath("/api/certificates", cid)+"/renew", nil); m["status"] != "valid" {
		t.Fatalf("renew valid: %v", m)
	}
	if n := len(rec.started); n != 4 || rec.started[n-1] != cid {
		t.Fatalf("renewals not started: %v", rec.started)
	}
	if code, m := e.call("POST", zzhPath("/api/certificates", custom)+"/renew", nil); code != 409 || m["error"] != "not_acme" {
		t.Fatalf("renew custom: %d %v", code, m)
	}
	e.zzhExpect(404, "POST", "/api/certificates/999/renew", nil)
	e.zzhExpect(400, "POST", "/api/certificates/x/renew", nil)
	if code, _ := e.zzhAs(viewer, "POST", zzhPath("/api/certificates", cid)+"/renew", nil); code != 403 {
		t.Fatalf("viewer renew: %d", code)
	}
	for _, a := range []string{"certificate.acme_requested", "certificate.acme_renew"} {
		if !e.zzhAudited(a) {
			t.Errorf("audit %s missing", a)
		}
	}
}
