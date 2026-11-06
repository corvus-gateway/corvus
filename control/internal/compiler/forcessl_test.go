package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/store"
)

// "Force HTTPS" still applies when the advanced configuration defines its own location /
// (common in NPM setups): the redirect moves to server level.
func TestForceSSLWithOwnRootLocation(t *testing.T) {
	c, st := renderPagesEnv(t)
	c.P.DataDir = filepath.ToSlash(t.TempDir()) // certificate files are written below it
	ctx := context.Background()
	cert := &store.Certificate{Name: "c", Kind: "custom", Domains: []string{"a.example.com"}, NotAfter: time.Now().Add(90 * 24 * time.Hour), Status: "valid"}
	if err := st.CreateCertificate(ctx, cert); err != nil {
		t.Fatal(err)
	}
	certDir := filepath.FromSlash(c.P.CertDir(cert.ID))
	os.MkdirAll(certDir, 0o755)
	os.WriteFile(filepath.Join(certDir, "fullchain.pem"), []byte("CERT"), 0o644)
	os.WriteFile(filepath.Join(certDir, "privkey.pem"), []byte("KEY"), 0o600)
	mk := func(d, adv string) int64 {
		h := &store.ProxyHost{Domains: []string{d}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, Enabled: true,
			CertificateID: &cert.ID, SSLForced: true, AdvancedConfig: adv}
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
		return h.ID
	}
	own := mk("a.example.com", "location / { proxy_pass http://app:80; }")
	plain := mk("b.example.com", "")
	dir := filepath.Join(t.TempDir(), "v1")
	if err := c.Render(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
	read := func(id int64) string {
		b, err := os.ReadFile(filepath.Join(dir, "hosts", strconv.FormatInt(id, 10)+".conf"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	server := "\n    if ($corvus_https_redirect) { return 301 https://$host$request_uri; }\n"
	if conf := read(own); !strings.Contains(conf, server) || !strings.Contains(conf, `acme-challenge/") { set $corvus_https_redirect ""; }`) {
		t.Fatalf("own root location: no server-level redirect\n%s", conf)
	}
	if conf := read(plain); strings.Contains(conf, server) || !strings.Contains(conf, "        if ($corvus_https_redirect)") {
		t.Fatalf("plain host: redirect should stay in the locations\n%s", conf)
	}
}
