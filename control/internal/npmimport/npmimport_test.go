package npmimport

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// Schema subset of NPM v2.16.
const npmSchema = `
CREATE TABLE user (id INTEGER PRIMARY KEY, email TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE certificate (id INTEGER PRIMARY KEY, owner_user_id INTEGER, provider TEXT, nice_name TEXT, domain_names TEXT, meta TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE access_list (id INTEGER PRIMARY KEY, name TEXT, satisfy_any INTEGER, pass_auth INTEGER DEFAULT 1, is_deleted INTEGER DEFAULT 0);
CREATE TABLE access_list_client (id INTEGER PRIMARY KEY, access_list_id INTEGER, address TEXT, directive TEXT);
CREATE TABLE access_list_auth (id INTEGER PRIMARY KEY, access_list_id INTEGER, username TEXT, password TEXT);
CREATE TABLE proxy_host (id INTEGER PRIMARY KEY, domain_names TEXT, forward_scheme TEXT, forward_host TEXT, forward_port INTEGER,
  access_list_id INTEGER DEFAULT 0, certificate_id INTEGER DEFAULT 0, ssl_forced INTEGER, block_exploits INTEGER, allow_websocket_upgrade INTEGER,
  http2_support INTEGER, enabled INTEGER, advanced_config TEXT, locations TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE stream (id INTEGER PRIMARY KEY, is_deleted INTEGER DEFAULT 0);
CREATE TABLE redirection_host (id INTEGER PRIMARY KEY, is_deleted INTEGER DEFAULT 0);
INSERT INTO user VALUES (1, 'ops@example.com', 0);
INSERT INTO certificate VALUES (1, 1, 'letsencrypt', 'shop', '["shop.example.com"]', '{"dns_challenge":true,"dns_provider":"cloudflare","dns_provider_credentials":"# Cloudflare API token\ndns_cloudflare_api_token = cf-token-123"}', 0);
INSERT INTO certificate VALUES (2, 1, 'other', 'custom', '["a.example.com"]', '{}', 0);
INSERT INTO certificate VALUES (3, 1, 'other', 'gone', '["old.example.com"]', '{}', 1);
INSERT INTO access_list VALUES (1, 'office', 1, 1, 0);
INSERT INTO access_list_client VALUES (1, 1, '192.168.0.0/24', 'allow');
INSERT INTO access_list_auth VALUES (1, 1, 'alice', 'plain-secret');
INSERT INTO proxy_host VALUES (1, '["shop.example.com"]', 'http', 'app', 8080, 1, 1, 1, 1, 1, 1, 1, 'client_max_body_size 20m;', '[]', 0);
INSERT INTO proxy_host VALUES (2, '["b.example.com"]', 'http', 'b', 80, 0, 0, 0, 0, 0, 0, 1, 'proxy_pass $forward_scheme://$server:$port;', '[{"path":"/x"}]', 0);
INSERT INTO proxy_host VALUES (3, '["deleted.example.com"]', 'http', 'c', 80, 0, 0, 0, 0, 0, 0, 1, '', '[]', 1);
INSERT INTO stream VALUES (1, 0);
`

func TestRead(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	os.MkdirAll(filepath.Join(data, "custom_ssl", "npm-2"), 0o755)
	os.MkdirAll(filepath.Join(root, "letsencrypt", "live", "npm-1"), 0o755)
	for _, p := range []string{filepath.Join(data, "custom_ssl", "npm-2"), filepath.Join(root, "letsencrypt", "live", "npm-1")} {
		os.WriteFile(filepath.Join(p, "fullchain.pem"), []byte("CERT"), 0o644)
		os.WriteFile(filepath.Join(p, "privkey.pem"), []byte("KEY"), 0o600)
	}
	db, err := sql.Open("sqlite", filepath.Join(data, "database.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(npmSchema); err != nil {
		t.Fatal(err)
	}
	db.Close()

	snap, err := Read(context.Background(), Source{DataDir: data, LetsEncryptDir: filepath.Join(root, "letsencrypt")})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.ProxyHosts) != 2 || len(snap.Certificates) != 2 || len(snap.AccessLists) != 1 {
		t.Fatalf("soft-deleted rows not filtered: %d hosts %d certs %d acls", len(snap.ProxyHosts), len(snap.Certificates), len(snap.AccessLists))
	}
	h1, h2 := snap.ProxyHosts[0], snap.ProxyHosts[1]
	if !h1.SSLForced || !h1.Websocket || !h1.HTTP2 || h1.CertificateID != 1 || h1.AccessListID != 1 || h1.AdvancedConfig == "" {
		t.Fatalf("host 1 mapping: %+v", h1)
	}
	if h2.AdvancedConfig != "" || h2.Locations != 1 || len(h2.Warnings) != 2 {
		t.Fatalf("host 2 should drop NPM-specific config and warn about locations: %+v", h2)
	}
	le := snap.Certificates[0]
	if le.DNSToken != "cf-token-123" || le.Email != "ops@example.com" || !le.FilesFound {
		t.Fatalf("LE cert: %+v", le)
	}
	a := snap.AccessLists[0]
	if a.SatisfyAll || len(a.Clients) != 1 || a.Users[0].Password != "plain-secret" {
		t.Fatalf("acl: %+v", a)
	}
	if len(snap.Warnings) != 1 {
		t.Fatalf("expected stream warning, got %v", snap.Warnings)
	}
}

func TestReadExplainsMissingDatabase(t *testing.T) {
	dir := t.TempDir()
	_, err := Read(context.Background(), Source{DataDir: dir})
	if err == nil || !strings.Contains(err.Error(), "not found") || !strings.Contains(err.Error(), "MariaDB") {
		t.Fatalf("empty data dir: got %v", err)
	}
	_, err = Read(context.Background(), Source{DataDir: filepath.Join(dir, "missing")})
	if err == nil || !strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("missing data dir: got %v", err)
	}
}
