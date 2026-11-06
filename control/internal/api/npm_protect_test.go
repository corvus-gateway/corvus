package api

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/compiler"
	"corvus/control/internal/deploy"
)

const npmProtectSchema = `
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
INSERT INTO user VALUES (1, 'ops@example.com', 0);
INSERT INTO access_list VALUES (1, 'Admins (VPN)', 1, 1, 0);
INSERT INTO access_list_auth VALUES (1, 1, 'admin', 'secret-pass');
INSERT INTO access_list VALUES (2, 'Bad users', 1, 1, 0);
INSERT INTO access_list_auth VALUES (2, 2, 'name with space', 'pw');
INSERT INTO proxy_host VALUES (1, '["admin.example.com"]', 'http', 'app', 80, 1, 0, 0, 1, 0, 1, 1, '', '[]', 0);
INSERT INTO proxy_host VALUES (2, '["private.example.com"]', 'http', 'app', 80, 2, 0, 0, 1, 0, 1, 1, '', '[]', 0);
INSERT INTO proxy_host VALUES (3, '["public.example.com"]', 'http', 'app', 80, 0, 7, 0, 1, 0, 1, 1, '', '[]', 0);
`

// A host protected by an NPM access list is imported with that protection or not at
// all; names NPM allows (parentheses) are adapted instead of dropping the list.
func TestNPMImportKeepsProtection(t *testing.T) {
	e, _ := newCredEnv(t)
	ctx := context.Background()
	root := t.TempDir()
	data := filepath.Join(root, "npm", "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(data, "database.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(npmProtectSchema); err != nil {
		t.Fatal(err)
	}
	db.Close()
	dir := t.TempDir()
	p := compiler.Paths{DataDir: filepath.ToSlash(dir), LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181}
	e.s.Paths = p
	e.s.NPMImportDir = root
	e.s.Deployer = &deploy.Deployer{Store: e.s.Store, Compiler: &compiler.Compiler{P: p}, Runner: &countRunner{}, Dir: filepath.Join(dir, "nginx")}

	w, _ := e.do("POST", "/api/import/npm/apply", map[string]any{}, withCookie(e.cookie), withCSRF())
	if w.Code != 200 {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	body := w.Body.String()
	hosts, _ := e.s.Store.ListProxyHosts(ctx)
	byDomain := map[string]bool{}
	for _, h := range hosts {
		byDomain[h.Domains[0]] = true
		if h.Domains[0] == "admin.example.com" && h.AccessListID == nil {
			t.Fatal("protected host imported without its password protection")
		}
	}
	if !byDomain["admin.example.com"] || !byDomain["public.example.com"] {
		t.Fatalf("hosts: %v (%s)", byDomain, body)
	}
	if byDomain["private.example.com"] {
		t.Fatalf("host whose access list failed was imported unprotected (%s)", body)
	}
	if !strings.Contains(body, "its access list (NPM #2) was not imported") || !strings.Contains(body, "imported without HTTPS") {
		t.Fatalf("skip report: %s", body)
	}
	lists, _ := e.s.Store.ListAccessLists(ctx)
	if len(lists) != 1 || lists[0].Name != "Admins -VPN-" {
		t.Fatalf("access lists: %+v", lists)
	}
}
