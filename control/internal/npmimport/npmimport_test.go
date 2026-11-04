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
CREATE TABLE stream (id INTEGER PRIMARY KEY, incoming_port INTEGER, forwarding_host TEXT, forwarding_port INTEGER, tcp_forwarding INTEGER,
  udp_forwarding INTEGER, certificate_id INTEGER DEFAULT 0, enabled INTEGER, meta TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE redirection_host (id INTEGER PRIMARY KEY, domain_names TEXT, forward_scheme TEXT DEFAULT 'auto', forward_domain_name TEXT,
  forward_http_code INTEGER DEFAULT 302, preserve_path INTEGER, certificate_id INTEGER DEFAULT 0, ssl_forced INTEGER, http2_support INTEGER,
  block_exploits INTEGER, enabled INTEGER, advanced_config TEXT, is_deleted INTEGER DEFAULT 0);
INSERT INTO redirection_host VALUES (1, '["old.example.com","www.old.example.com"]', 'https', 'new.example.com', 301, 1, 2, 1, 1, 1, 1, '', 0);
INSERT INTO redirection_host VALUES (2, '["legacy.example.com"]', '$scheme', 'https://example.com/landing', 300, 0, 0, 0, 0, 0, 0, 'return 410 $server;', 0);
INSERT INTO redirection_host VALUES (3, '["deleted-redirect.example.com"]', 'http', 'example.com', 302, 0, 0, 0, 0, 0, 1, '', 1);
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
INSERT INTO stream VALUES (1, 5432, ' db ', 5432, 1, 0, 2, 1, '{}', 0);
INSERT INTO stream VALUES (2, 53, '10.0.0.53', 53, 1, 1, 0, 0, '{}', 0);
INSERT INTO stream VALUES (3, 1194, 'vpn', 1194, 0, 1, 2, 1, '{}', 0);
INSERT INTO stream VALUES (4, 25, 'mail', 25, 1, 0, 0, 1, '{}', 1);
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
	if le.DNSCredentials == nil || le.DNSCredentials.APIToken != "cf-token-123" || le.Email != "ops@example.com" || !le.FilesFound {
		t.Fatalf("LE cert: %+v", le)
	}
	a := snap.AccessLists[0]
	if a.SatisfyAll || len(a.Clients) != 1 || a.Users[0].Password != "plain-secret" {
		t.Fatalf("acl: %+v", a)
	}
	if len(snap.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", snap.Warnings)
	}
	if len(snap.Streams) != 3 {
		t.Fatalf("streams (soft-deleted must be filtered): %+v", snap.Streams)
	}
	s1, s2, s3 := snap.Streams[0], snap.Streams[1], snap.Streams[2]
	if s1.NPMID != 1 || s1.IncomingPort != 5432 || s1.ForwardHost != "db" || s1.ForwardPort != 5432 || !s1.TCP || s1.UDP || s1.CertificateID != 2 || !s1.Enabled {
		t.Fatalf("stream 1 mapping: %+v", s1)
	}
	if !s2.TCP || !s2.UDP || s2.Enabled || s2.CertificateID != 0 || s2.ForwardHost != "10.0.0.53" {
		t.Fatalf("stream 2 mapping: %+v", s2)
	}
	// UDP only: the certificate cannot be used
	if s3.TCP || !s3.UDP || s3.CertificateID != 0 || len(s3.Warnings) != 1 {
		t.Fatalf("stream 3 mapping: %+v", s3)
	}
	if len(snap.Redirects) != 2 {
		t.Fatalf("redirection hosts: %+v", snap.Redirects)
	}
	r1, r2 := snap.Redirects[0], snap.Redirects[1]
	if r1.NPMID != 1 || len(r1.Domains) != 2 || r1.ForwardScheme != "https" || r1.ForwardDomain != "new.example.com" || r1.HTTPCode != 301 ||
		!r1.PreservePath || r1.CertificateID != 2 || !r1.SSLForced || !r1.HTTP2 || !r1.BlockExploits || !r1.Enabled || len(r1.Warnings) != 0 ||
		r1.Forward != "301 https://new.example.com$request_uri" {
		t.Fatalf("redirect 1 mapping: %+v", r1)
	}
	// "$scheme" (older NPM) is auto, a scheme typed into the target is kept, 300 becomes 302,
	// NPM-specific advanced config is dropped.
	if r2.ForwardScheme != "https" || r2.ForwardDomain != "example.com/landing" || r2.HTTPCode != 302 || r2.PreservePath || r2.Enabled ||
		r2.AdvancedConfig != "" || len(r2.Warnings) != 2 {
		t.Fatalf("redirect 2 mapping: %+v", r2)
	}
}

func TestReadLegacyRedirects(t *testing.T) {
	data := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(data, "database.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	// NPM before 2.8: no forward_scheme / forward_http_code columns.
	if _, err := db.Exec(`
CREATE TABLE proxy_host (id INTEGER PRIMARY KEY, domain_names TEXT, forward_scheme TEXT, forward_host TEXT, forward_port INTEGER,
  access_list_id INTEGER DEFAULT 0, certificate_id INTEGER DEFAULT 0, ssl_forced INTEGER, block_exploits INTEGER, allow_websocket_upgrade INTEGER,
  http2_support INTEGER, enabled INTEGER, advanced_config TEXT, locations TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE certificate (id INTEGER PRIMARY KEY, owner_user_id INTEGER, provider TEXT, nice_name TEXT, domain_names TEXT, meta TEXT, is_deleted INTEGER DEFAULT 0);
CREATE TABLE access_list (id INTEGER PRIMARY KEY, name TEXT, satisfy_any INTEGER, is_deleted INTEGER DEFAULT 0);
CREATE TABLE redirection_host (id INTEGER PRIMARY KEY, domain_names TEXT, forward_domain_name TEXT, preserve_path INTEGER, certificate_id INTEGER DEFAULT 0,
  ssl_forced INTEGER, http2_support INTEGER, block_exploits INTEGER, enabled INTEGER, advanced_config TEXT, is_deleted INTEGER DEFAULT 0);
INSERT INTO redirection_host VALUES (1, '["a.example.com"]', 'b.example.com', 1, 0, 0, 0, 1, 1, NULL, 0);
CREATE TABLE stream (id INTEGER PRIMARY KEY, incoming_port INTEGER, forward_ip TEXT, forwarding_port INTEGER, tcp_forwarding INTEGER,
  udp_forwarding INTEGER, enabled INTEGER, is_deleted INTEGER DEFAULT 0);
INSERT INTO stream VALUES (1, 2222, '10.0.0.2', 22, 1, 0, 1, 0);
`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	snap, err := Read(context.Background(), Source{DataDir: data})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Redirects) != 1 || snap.Redirects[0].ForwardScheme != "auto" || snap.Redirects[0].HTTPCode != 302 || snap.Redirects[0].ForwardDomain != "b.example.com" {
		t.Fatalf("legacy redirect: %+v", snap.Redirects)
	}
	// NPM before 2.9: forward_ip, no certificate_id.
	if len(snap.Streams) != 1 || snap.Streams[0].ForwardHost != "10.0.0.2" || snap.Streams[0].IncomingPort != 2222 || snap.Streams[0].ForwardPort != 22 || !snap.Streams[0].TCP {
		t.Fatalf("legacy stream: %+v", snap.Streams)
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
