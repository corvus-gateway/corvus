package api

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
)

// NPM v2 schema subset (with the default-site setting).
const zaNPMSchema = `
CREATE TABLE user (id INTEGER PRIMARY KEY, email TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE certificate (id INTEGER PRIMARY KEY, owner_user_id INTEGER, provider TEXT, nice_name TEXT, domain_names TEXT, meta TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE access_list (id INTEGER PRIMARY KEY, name TEXT, satisfy_any INTEGER, pass_auth INTEGER DEFAULT 1, is_deleted INTEGER DEFAULT 0);
CREATE TABLE access_list_client (id INTEGER PRIMARY KEY, access_list_id INTEGER, address TEXT, directive TEXT);
CREATE TABLE access_list_auth (id INTEGER PRIMARY KEY, access_list_id INTEGER, username TEXT, password TEXT);
CREATE TABLE proxy_host (id INTEGER PRIMARY KEY, domain_names TEXT, forward_scheme TEXT, forward_host TEXT, forward_port INTEGER,
  access_list_id INTEGER DEFAULT 0, certificate_id INTEGER DEFAULT 0, ssl_forced INTEGER, block_exploits INTEGER, allow_websocket_upgrade INTEGER,
  http2_support INTEGER, enabled INTEGER, advanced_config TEXT, locations TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE stream (id INTEGER PRIMARY KEY, incoming_port INTEGER, forwarding_host TEXT, forwarding_port INTEGER, tcp_forwarding INTEGER,
  udp_forwarding INTEGER, certificate_id INTEGER DEFAULT 0, enabled INTEGER, meta TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE redirection_host (id INTEGER PRIMARY KEY, domain_names TEXT, forward_scheme TEXT DEFAULT 'auto', forward_domain_name TEXT,
  forward_http_code INTEGER DEFAULT 302, preserve_path INTEGER, certificate_id INTEGER DEFAULT 0, ssl_forced INTEGER, http2_support INTEGER,
  block_exploits INTEGER, enabled INTEGER, advanced_config TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE setting (id TEXT PRIMARY KEY, name TEXT, description TEXT, value TEXT, meta TEXT);
`

const zaNPMRows = `
INSERT INTO setting VALUES ('default-site', 'Default Site', '', '444', '{}');
INSERT INTO user VALUES (1, 'ops@example.com', 0);
INSERT INTO certificate VALUES (1, 1, 'letsencrypt', 'shop', '["shop.example.com"]', '{"dns_challenge":true,"dns_provider":"cloudflare","dns_provider_credentials":"dns_cloudflare_api_token = cf-token-123"}', 0);
INSERT INTO certificate VALUES (2, 1, 'other', 'custom', '["a.example.com"]', '{}', 0);
INSERT INTO certificate VALUES (3, 1, 'other', 'no files', '["c.example.com"]', '{}', 0);
INSERT INTO certificate VALUES (4, 1, 'other', 'broken', '["d.example.com"]', '{}', 0);
INSERT INTO certificate VALUES (5, 1, 'letsencrypt', '', '["five.example.com"]', '{"dns_challenge":true,"dns_provider":"unknown-dns","dns_provider_credentials":"x"}', 0);
INSERT INTO certificate VALUES (6, 1, 'letsencrypt', 'six', '["six.example.com"]', '{}', 0);
INSERT INTO access_list VALUES (1, 'office', 1, 1, 0);
INSERT INTO access_list_client VALUES (1, 1, '192.168.0.0/24', 'allow');
INSERT INTO access_list_client VALUES (2, 1, '10.0.0.1', 'deny');
INSERT INTO access_list_auth VALUES (1, 1, 'alice', 'plain-secret');
INSERT INTO access_list VALUES (2, 'Office IPs', 0, 1, 0);
INSERT INTO access_list_client VALUES (3, 2, '203.0.113.0/24', 'allow');
INSERT INTO access_list VALUES (3, 'broken allow', 1, 1, 0);
INSERT INTO access_list_client VALUES (4, 3, 'not-an-ip', 'allow');
INSERT INTO access_list VALUES (4, 'broken deny', 1, 1, 0);
INSERT INTO access_list_client VALUES (5, 4, '300.1.1.1', 'deny');
INSERT INTO access_list_auth VALUES (2, 4, 'carol', 'pw');
INSERT INTO access_list VALUES (5, '***', 1, 1, 0);
INSERT INTO access_list_auth VALUES (3, 5, 'bob', 'bob-secret');
INSERT INTO proxy_host VALUES (1, '["shop.example.com"]', 'http', 'shop', 8080, 1, 1, 1, 1, 1, 1, 1, '', '[]', 0);
INSERT INTO proxy_host VALUES (2, '["a.example.com"]', 'http', 'a', 80, 2, 2, 1, 0, 0, 0, 1, '', '[]', 0);
INSERT INTO proxy_host VALUES (3, '["Taken.example.com"]', 'http', 't', 80, 0, 0, 0, 0, 0, 0, 1, '', '[]', 0);
INSERT INTO proxy_host VALUES (4, '["nocert.example.com"]', 'http', 'n', 80, 0, 99, 1, 0, 0, 0, 1, 'client_max_body_size 10m;', '[]', 0);
INSERT INTO proxy_host VALUES (5, '["broken-acl.example.com"]', 'http', 'b', 80, 3, 0, 0, 0, 0, 0, 1, '', '[]', 0);
INSERT INTO proxy_host VALUES (6, '["bad-port.example.com"]', 'http', 'p', 0, 0, 0, 0, 0, 0, 0, 1, '', '[]', 0);
INSERT INTO proxy_host VALUES (7, '["five.example.com"]', 'http', 'f', 80, 5, 5, 0, 0, 0, 0, 1, '', '[]', 0);
INSERT INTO redirection_host VALUES (1, '["old.example.com"]', 'https', 'new.example.com', 301, 1, 2, 1, 1, 1, 1, '', 0);
INSERT INTO redirection_host VALUES (2, '["moved.example.com"]', 'http', 'example.org', 302, 0, 0, 0, 0, 0, 1, '', 0);
INSERT INTO redirection_host VALUES (3, '["bad-redirect.example.com"]', 'http', '', 302, 0, 0, 0, 0, 0, 1, '', 0);
INSERT INTO stream VALUES (1, 5432, 'db', 5432, 1, 0, 2, 1, '{}', 0);
INSERT INTO stream VALUES (2, 443, 'web', 443, 1, 0, 0, 1, '{}', 0);
INSERT INTO stream VALUES (3, 6000, 'x', 0, 1, 0, 0, 1, '{}', 0);
`

// zaNPMData writes an NPM installation (database + certificate files) below root.
func zaNPMData(t *testing.T, root, rows string) {
	t.Helper()
	data := filepath.Join(root, "npm", "data")
	le := filepath.Join(root, "npm", "letsencrypt")
	write := func(dir, cert, key string) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(dir, "fullchain.pem"), []byte(cert), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "privkey.pem"), []byte(key), 0o600)
	}
	c, k := selfSigned(t, 1, time.Now().Add(60*24*time.Hour), "shop.example.com")
	write(filepath.Join(le, "live", "npm-1"), c, k)
	c, k = selfSigned(t, 2, time.Now().Add(60*24*time.Hour), "a.example.com")
	write(filepath.Join(data, "custom_ssl", "npm-2"), c, k)
	write(filepath.Join(data, "custom_ssl", "npm-4"), "CERT", "KEY")
	c, k = selfSigned(t, 5, time.Now().Add(60*24*time.Hour), "five.example.com")
	write(filepath.Join(le, "live", "npm-5"), c, k)
	c, k = selfSigned(t, 6, time.Now().Add(60*24*time.Hour), "six.example.com")
	write(filepath.Join(le, "live", "npm-6"), c, k)
	db, err := sql.Open("sqlite", filepath.Join(data, "database.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(zaNPMSchema + rows); err != nil {
		t.Fatal(err)
	}
}

func zaSkips(m map[string]any) map[string]string {
	out := map[string]string{}
	l, _ := m["skipped"].([]any)
	for _, s := range l {
		sm := s.(map[string]any)
		out[fmt.Sprintf("%s:%v", sm["kind"], sm["npm_id"])] = sm["reason"].(string)
	}
	return out
}

func TestZZNPMPreviewAndApply(t *testing.T) {
	e := newFullEnv(t)
	ctx := context.Background()
	root := t.TempDir()
	zaNPMData(t, root, zaNPMRows)
	e.s.NPMImportDir = root

	// Corvus already has a host, a redirect and a password list with names the import uses
	e.must(201, "POST", "/api/proxy-hosts", map[string]any{"domains": []string{"taken.example.com"}, "forward_scheme": "http", "forward_host": "app", "forward_port": 8080})
	if err := e.s.Store.SaveRedirectionHost(ctx, &store.RedirectionHost{Domains: []string{"moved.example.com"}, ForwardScheme: "https", ForwardDomain: "example.net", HTTPCode: 301, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	h, _ := auth.HtpasswdHash("pw-12345")
	if err := e.s.Store.SaveAccessList(ctx, &store.AccessList{Name: "office", Satisfy: "any", Allow: []string{}, Deny: []string{}, Users: []store.AccessListUser{{Username: "u", PasswordHash: h}}}); err != nil {
		t.Fatal(err)
	}

	// preview marks conflicts with what Corvus already has
	m := e.must(200, "POST", "/api/import/npm/preview", map[string]any{})
	hosts := m["proxy_hosts"].([]any)
	if len(hosts) != 7 || len(m["certificates"].([]any)) != 6 || len(m["access_lists"].([]any)) != 5 {
		t.Fatalf("preview counts: %d hosts, %v", len(hosts), m)
	}
	conflicts := map[string]string{}
	for _, kind := range []string{"proxy_hosts", "redirection_hosts", "streams"} {
		for _, it := range m[kind].([]any) {
			im := it.(map[string]any)
			if c, _ := im["conflict"].(string); c != "" {
				conflicts[fmt.Sprintf("%s:%v", kind, im["npm_id"])] = c
			}
		}
	}
	if !strings.Contains(conflicts["proxy_hosts:3"], "already used by Corvus proxy host #") ||
		!strings.Contains(conflicts["redirection_hosts:2"], "already used by Corvus redirection host #") ||
		!strings.Contains(conflicts["streams:2"], "port 443 is used by Corvus") || len(conflicts) != 3 {
		t.Fatalf("conflicts: %v", conflicts)
	}

	// selective import: only host 2 with its certificate and IP list, unknown hosts not taken
	m = e.must(200, "POST", "/api/import/npm/apply", map[string]any{"proxy_host_ids": []int64{2}, "redirection_host_ids": []int64{},
		"stream_ids": []int64{}, "certificate_ids": []int64{2}, "access_list_ids": []int64{2}, "unknown_hosts": false})
	imp := m["imported"].(map[string]any)
	if imp["proxy_hosts"] != 1.0 || imp["certificates"] != 1.0 || imp["access_lists"] != 0.0 || imp["security_rules"] == nil || imp["unknown_hosts"] != nil {
		t.Fatalf("selective import: %v", m)
	}
	if u, _ := e.s.Store.UnknownHosts(ctx); u.Mode == store.UnknownClose {
		t.Fatal("unknown host setting imported although deselected")
	}
	if hosts, _ := e.s.Store.ListProxyHosts(ctx); len(hosts) != 2 || w2s(hosts[1].Domains) != `["a.example.com"]` {
		t.Fatalf("hosts after selective import: %+v", hosts)
	}

	// everything
	m = e.must(200, "POST", "/api/import/npm/apply", map[string]any{})
	imp = m["imported"].(map[string]any)
	skips := zaSkips(m)
	want := map[string]float64{"certificates": 4, "access_lists": 2, "proxy_hosts": 3, "redirection_hosts": 1, "streams": 1, "unknown_hosts": 1}
	for k, v := range want {
		if imp[k] != v {
			t.Fatalf("imported %s = %v, want %v (%v, skipped %v)", k, imp[k], v, imp, skips)
		}
	}
	for key, part := range map[string]string{
		"certificate:3":      "files not found",
		"certificate:4":      "",
		"access_list:3":      "",
		"access_list:4":      "",
		"proxy_host:2":       "already used",
		"proxy_host:3":       "already used",
		"proxy_host:4":       "imported without HTTPS",
		"proxy_host:5":       "its access list (NPM #3) was not imported",
		"proxy_host:6":       "",
		"redirection_host:2": "already used",
		"redirection_host:3": "",
		"stream:2":           "port 443",
		"stream:3":           "",
	} {
		reason, ok := skips[key]
		if !ok || !strings.Contains(reason, part) {
			t.Errorf("skip %s = %q (present %v), want it to mention %q", key, reason, ok, part)
		}
	}
	if t.Failed() {
		t.Fatalf("skipped: %v", skips)
	}

	// what was created
	certs, _ := e.s.Store.ListCertificates(ctx)
	byName := map[string]store.Certificate{}
	for _, c := range certs {
		byName[c.Name] = c
	}
	if c := byName["shop"]; c.Kind != "acme" || !c.AutoRenew || c.ACME.Challenge != "dns-01" || c.ACME.DNSProvider != "cloudflare" || c.DNSCredentialID == nil || c.ACME.Email != "ops@example.com" {
		t.Fatalf("Let's Encrypt DNS certificate: %+v", c)
	}
	if c := byName["five.example.com"]; c.Kind != "acme" || c.AutoRenew || c.ACME.Challenge != "dns-01" || c.DNSCredentialID != nil {
		t.Fatalf("unsupported DNS provider certificate: %+v", c)
	}
	if c := byName["six"]; c.Kind != "acme" || !c.AutoRenew || c.ACME.Challenge != "http-01" {
		t.Fatalf("HTTP-01 certificate: %+v", c)
	}
	if c := byName["custom"]; c.Kind != "custom" {
		t.Fatalf("custom certificate: %+v", c)
	}
	if _, err := os.Stat(filepath.Join(filepath.FromSlash(e.s.Paths.CertDir(byName["shop"].ID)), "fullchain.pem")); err != nil {
		t.Fatalf("certificate files not written: %v", err)
	}
	lists, _ := e.s.Store.ListAccessLists(ctx)
	names := map[string]bool{}
	for _, l := range lists {
		names[l.Name] = true
	}
	if !names["office"] || !names["office-npm-2"] || !names["npm-list-5"] || len(lists) != 3 {
		t.Fatalf("access lists: %v", names)
	}
	if u, _ := e.s.Store.UnknownHosts(ctx); u.Mode != store.UnknownClose {
		t.Fatalf("unknown hosts: %+v", u)
	}
	// NPM's global 2000 MB upload limit, unless the advanced configuration sets its own
	all, _ := e.s.Store.ListProxyHosts(ctx)
	uploads := map[string]string{}
	for _, h := range all {
		uploads[h.Domains[0]] = "nil"
		if h.MaxUploadMB != nil {
			uploads[h.Domains[0]] = fmt.Sprint(*h.MaxUploadMB)
		}
	}
	if uploads["shop.example.com"] != "2000" || uploads["nocert.example.com"] != "nil" || uploads["taken.example.com"] != "nil" {
		t.Fatalf("upload limits: %v", uploads)
	}
	if zaAudited(t, e.s, "import.npm") != 2 || zaAudited(t, e.s, "dns_credential.create") != 1 {
		t.Fatalf("audit: import=%d cred=%d", zaAudited(t, e.s, "import.npm"), zaAudited(t, e.s, "dns_credential.create"))
	}
}

func TestZZNPMRequestErrors(t *testing.T) {
	e, path := newAuthEnv(t)
	root := t.TempDir()

	// not configured
	if code, m := e.call("POST", "/api/import/npm/preview", map[string]any{}); code != 422 || !strings.Contains(w2s(m), "not configured") {
		t.Fatalf("unconfigured: %d %v", code, m)
	}
	e.s.NPMImportDir = root
	if code, _ := e.call("POST", "/api/import/npm/preview", map[string]any{"bogus": 1}); code != 400 {
		t.Fatalf("unknown field: %d", code)
	}
	for _, body := range []map[string]any{{"path": "/etc"}, {"path": filepath.Join(root, "npm", "data"), "letsencrypt_path": "/etc/letsencrypt"}, {"path": root + "/../x"}} {
		if code, m := e.call("POST", "/api/import/npm/apply", body); code != 422 || zaFields(m)["path"] != "invalid" {
			t.Fatalf("outside %v: %d %v", body, code, m)
		}
	}
	// nothing there
	if code, m := e.call("POST", "/api/import/npm/preview", map[string]any{}); code != 422 || zaFields(m)["path"] != "unreadable" {
		t.Fatalf("no database: %d %v", code, m)
	}
	// only administrators (and only with a session)
	if w, _ := e.do("POST", "/api/import/npm/preview", map[string]any{}, withCookie(e.as("operator")), withCSRF()); w.Code != 403 {
		t.Fatalf("operator: %d", w.Code)
	}

	// database errors while marking conflicts
	zaNPMData(t, root, "")
	if code, m := e.call("POST", "/api/import/npm/preview", map[string]any{}); code != 200 || len(m["streams"].([]any)) != 0 || len(m["redirection_hosts"].([]any)) != 0 {
		t.Fatalf("empty installation: %d %v", code, m)
	}
	for _, table := range []string{"streams", "redirection_hosts", "proxy_hosts"} {
		zaExec(t, path, "DROP TABLE "+table)
		if code, m := e.call("POST", "/api/import/npm/preview", map[string]any{}); code != 500 || m["error"] != "internal" {
			t.Fatalf("without %s: %d %v", table, code, m)
		}
	}
}

// When the gateway rejects the imported configuration nothing is kept, including the
// certificate files written for the import.
func TestZZNPMApplyRejected(t *testing.T) {
	e := newFullEnv(t)
	root := t.TempDir()
	zaNPMData(t, root, `
INSERT INTO certificate VALUES (2, 1, 'other', 'custom', '["a.example.com"]', '{}', 0);
INSERT INTO proxy_host VALUES (1, '["a.example.com"]', 'http', 'a', 80, 0, 2, 1, 0, 0, 0, 1, 'REJECT;', '[]', 0);
`)
	e.s.NPMImportDir = root
	code, m := e.call("POST", "/api/import/npm/apply", map[string]any{})
	if code != 422 || m["error"] != "config_rejected" {
		t.Fatalf("apply: %d %v", code, m)
	}
	hosts, _ := e.s.Store.ListProxyHosts(context.Background())
	certs, _ := e.s.Store.ListCertificates(context.Background())
	if len(hosts) != 0 || len(certs) != 0 {
		t.Fatalf("rolled back import left %d hosts, %d certificates", len(hosts), len(certs))
	}
	entries, _ := os.ReadDir(filepath.Join(filepath.FromSlash(e.s.Paths.DataDir), "certs"))
	for _, en := range entries {
		if en.Name() != "default" {
			t.Fatalf("certificate files left behind: %s", en.Name())
		}
	}
}

func TestZZNPMNames(t *testing.T) {
	if got := npmPolicyName("  ...", 7); got != "npm-list-7" {
		t.Errorf("empty name: %q", got)
	}
	if got := npmPolicyName("Ünïcode (VPN)", 1); got != "n-code -VPN-" {
		t.Errorf("non-ASCII name: %q", got)
	}
	long := strings.Repeat("x", 80)
	if got := suffixName(long, "-npm-2"); len(got) != 64 || !strings.HasSuffix(got, "-npm-2") {
		t.Errorf("suffixName: %q (%d)", got, len(got))
	}
	if got := suffixName(strings.Repeat("y", 57)+" abcdef", "-npm-3"); strings.Contains(got, " -npm") {
		t.Errorf("trailing space kept: %q", got)
	}
	if !selected(nil, 5) || selected([]int64{}, 5) || !selected([]int64{1, 5}, 5) || selected([]int64{1}, 5) {
		t.Error("selected")
	}
	if contains([]string{"a"}, "b") || !contains([]string{"a", "all"}, "all") {
		t.Error("contains")
	}
}
