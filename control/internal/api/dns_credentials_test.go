package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/acme"
	"corvus/control/internal/compiler"
	"corvus/control/internal/deploy"

	_ "modernc.org/sqlite"
)

type startRecorder struct{ started []int64 }

func (f *startRecorder) Start(id int64) { f.started = append(f.started, id) }

func newCredEnv(t *testing.T) (*testEnv, *startRecorder) {
	t.Helper()
	e := newTestEnv(t)
	e.s.DataKey = make([]byte, 32)
	_, _ = rand.Read(e.s.DataKey)
	rec := &startRecorder{}
	e.s.ACME = rec
	return e, rec
}

func TestDNSCredentialsAPI(t *testing.T) {
	e, rec := newCredEnv(t)
	ctx := context.Background()
	do := func(method, path string, body any) (int, string, map[string]any) {
		w, m := e.do(method, path, body, withCookie(e.cookie), withCSRF())
		return w.Code, w.Body.String(), m
	}
	const secret = "cf-secret-token-1"

	// create: the secret is never returned
	code, body, m := do("POST", "/api/dns-credentials", map[string]any{"name": "Main zone", "provider": "cloudflare", "credentials": "dns_cloudflare_api_token=" + secret})
	if code != 201 || strings.Contains(body, secret) || strings.Contains(body, "secret") || m["name"] != "Main zone" || m["provider"] != "cloudflare" {
		t.Fatalf("create: %d %s", code, body)
	}
	id := int64(m["id"].(float64))
	if code, body, _ := do("POST", "/api/dns-credentials", map[string]any{"name": "main zone", "credentials": "dns_cloudflare_api_token=x"}); code != 422 || !strings.Contains(body, "name") {
		t.Fatalf("duplicate name: %d %s", code, body)
	}
	if code, _, _ := do("POST", "/api/dns-credentials", map[string]any{"credentials": acme.Providers[0].Template}); code != 422 {
		t.Fatalf("template placeholder accepted: %d", code)
	}

	// a certificate that picks the saved credential
	code, body, m = do("POST", "/api/certificates/acme", map[string]any{"domains": []string{"*.example.com"}, "email": "ops@example.com", "challenge": "dns-01", "dns_credential_id": id})
	if code != 202 || m["dns_credential_id"] == nil || int64(m["dns_credential_id"].(float64)) != id {
		t.Fatalf("request with saved credential: %d %s", code, body)
	}
	// a certificate with new credentials saves them under an automatic name
	code, body, m = do("POST", "/api/certificates/acme", map[string]any{"domains": []string{"*.other.com"}, "email": "ops@example.com", "challenge": "dns-01",
		"dns_provider": "cloudflare", "dns_credentials_ini": "dns_cloudflare_api_token=second"})
	if code != 202 || m["dns_credential_id"] == nil {
		t.Fatalf("request with new credentials: %d %s", code, body)
	}
	if code, _, _ := do("POST", "/api/certificates/acme", map[string]any{"domains": []string{"*.x.com"}, "email": "ops@example.com", "challenge": "dns-01", "dns_credential_id": 999}); code != 422 {
		t.Fatalf("unknown credential: %d", code)
	}
	if len(rec.started) != 2 {
		t.Fatalf("issuance started %d times", len(rec.started))
	}

	code, body, _ = do("GET", "/api/dns-credentials", nil)
	if code != 200 || strings.Contains(body, secret) || strings.Contains(body, "second") || !strings.Contains(body, `"name":"Cloudflare"`) || !strings.Contains(body, `"used_by":1`) {
		t.Fatalf("list: %d %s", code, body)
	}

	// replace the secret and rename: stored for renewals, no re-issuance
	code, body, _ = do("PUT", fmt.Sprintf("/api/dns-credentials/%d", id), map[string]any{"name": "Zone A", "credentials": "dns_cloudflare_api_token=rotated"})
	if code != 200 || strings.Contains(body, "rotated") || !strings.Contains(body, "Zone A") {
		t.Fatalf("replace: %d %s", code, body)
	}
	saved, _ := e.s.Store.DNSCredentialByID(ctx, id)
	if cred, err := acme.OpenCredential(e.s.DataKey, saved); err != nil || cred.APIToken != "rotated" || saved.Name != "Zone A" {
		t.Fatalf("stored after replace: %+v %v", cred, err)
	}
	if len(rec.started) != 2 {
		t.Fatal("replacing a secret started issuance")
	}
	// rename only keeps the secret
	if code, _, _ := do("PUT", fmt.Sprintf("/api/dns-credentials/%d", id), map[string]any{"name": "Zone B"}); code != 200 {
		t.Fatalf("rename: %d", code)
	}
	saved, _ = e.s.Store.DNSCredentialByID(ctx, id)
	if cred, _ := acme.OpenCredential(e.s.DataKey, saved); cred.APIToken != "rotated" {
		t.Fatal("rename changed the secret")
	}

	// in use: refused with the number of certificates
	code, body, _ = do("DELETE", fmt.Sprintf("/api/dns-credentials/%d", id), nil)
	if code != 422 || !strings.Contains(body, "in_use") || !strings.Contains(body, "1 certificate") {
		t.Fatalf("delete in use: %d %s", code, body)
	}
	if err := e.s.Store.DeleteCertificate(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if code, body, _ := do("DELETE", fmt.Sprintf("/api/dns-credentials/%d", id), nil); code != 200 {
		t.Fatalf("delete unused: %d %s", code, body)
	}
	if code, _, _ := do("DELETE", fmt.Sprintf("/api/dns-credentials/%d", id), nil); code != 404 {
		t.Fatalf("delete missing: %d", code)
	}

	n := 0
	for _, a := range auditActions(t, e) {
		if strings.HasPrefix(a, "dns_credential.") {
			n++
		}
	}
	if n != 5 { // create, create (with certificate), update, update, delete
		t.Fatalf("audit entries: %d", n)
	}
}

func auditActions(t *testing.T, e *testEnv) []string {
	t.Helper()
	list, err := e.s.Store.ListAudit(context.Background(), 200)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, a := range list {
		out = append(out, a.Action)
	}
	return out
}

const npmCredSchema = `
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
INSERT INTO certificate VALUES (1, 1, 'letsencrypt', 'a', '["a.example.com"]', '{"dns_challenge":true,"dns_provider":"cloudflare","dns_provider_credentials":"dns_cloudflare_api_token = shared-token"}', 0);
INSERT INTO certificate VALUES (2, 1, 'letsencrypt', 'b', '["b.example.com"]', '{"dns_challenge":true,"dns_provider":"cloudflare","dns_provider_credentials":"# same token\ndns_cloudflare_api_token=shared-token"}', 0);
INSERT INTO certificate VALUES (3, 1, 'letsencrypt', 'c', '["c.example.com"]', '{"dns_challenge":true,"dns_provider":"cloudflare","dns_provider_credentials":"dns_cloudflare_api_token = other-token"}', 0);
`

// Imported DNS certificates become saved credentials, one per provider and secret.
func TestNPMImportDNSCredentials(t *testing.T) {
	e, _ := newCredEnv(t)
	ctx := context.Background()
	root := t.TempDir()
	data := filepath.Join(root, "npm", "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, d := range []string{"a.example.com", "b.example.com", "c.example.com"} {
		dir := filepath.Join(root, "npm", "letsencrypt", "live", fmt.Sprintf("npm-%d", i+1))
		os.MkdirAll(dir, 0o755)
		cert, key := selfSigned(t, int64(i+1), time.Now().Add(30*24*time.Hour), d)
		os.WriteFile(filepath.Join(dir, "fullchain.pem"), []byte(cert), 0o644)
		os.WriteFile(filepath.Join(dir, "privkey.pem"), []byte(key), 0o600)
	}
	db, err := sql.Open("sqlite", filepath.Join(data, "database.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(npmCredSchema); err != nil {
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
	if w.Code != 200 || strings.Contains(w.Body.String(), "shared-token") {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	creds, _ := e.s.Store.ListDNSCredentials(ctx)
	if len(creds) != 2 || creds[0].Name != "Cloudflare" || creds[0].UsedBy != 2 || creds[1].Name != "Cloudflare 2" || creds[1].UsedBy != 1 {
		t.Fatalf("credentials: %+v", creds)
	}
	certs, _ := e.s.Store.ListCertificates(ctx)
	for _, c := range certs {
		if c.DNSCredentialID == nil || c.ACME.Credentials != "" || !c.AutoRenew || c.ACME.Challenge != "dns-01" {
			t.Fatalf("imported certificate: %+v", c)
		}
	}
	got, _ := acme.OpenCredential(e.s.DataKey, &creds[0])
	if got.APIToken != "shared-token" {
		t.Fatalf("secret: %+v", got)
	}
}
