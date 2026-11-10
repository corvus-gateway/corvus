package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/store"
)

// IPv6 listeners are rendered for proxy, redirection and default servers only when
// enabled; the resolver returns AAAA answers only with ResolverIPv6.
func TestRenderIPv6(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := Paths{DataDir: filepath.ToSlash(dir), LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181}

	cert := &store.Certificate{Name: "c", Kind: "custom", Domains: []string{"tls.example.com"}, NotAfter: time.Now().Add(90 * 24 * time.Hour)}
	if err := st.CreateCertificate(ctx, cert); err != nil {
		t.Fatal(err)
	}
	certDir := filepath.FromSlash(p.CertDir(cert.ID))
	os.MkdirAll(certDir, 0o755)
	os.WriteFile(filepath.Join(certDir, "fullchain.pem"), []byte("CERT"), 0o644)
	os.WriteFile(filepath.Join(certDir, "privkey.pem"), []byte("KEY"), 0o600)

	for _, h := range []*store.ProxyHost{
		{Domains: []string{"tls.example.com"}, ForwardHost: "app", ForwardPort: 8080, Enabled: true, CertificateID: &cert.ID},
		{Domains: []string{"plain.example.com"}, ForwardHost: "app", ForwardPort: 8080, Enabled: true},
	} {
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SaveRedirectionHost(ctx, &store.RedirectionHost{Domains: []string{"old.example.com"}, ForwardScheme: "https",
		ForwardDomain: "new.example.com", HTTPCode: 301, CertificateID: &cert.ID, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	render := func(name string, p Paths) func(string) string {
		out := filepath.Join(dir, name)
		if err := (&Compiler{P: p}).Render(ctx, st, out); err != nil {
			t.Fatal(err)
		}
		return func(f string) string {
			b, err := os.ReadFile(filepath.Join(out, f))
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	check := func(conf string, want, deny []string) {
		t.Helper()
		for _, w := range want {
			if !strings.Contains(conf, w) {
				t.Errorf("missing %q\n%s", w, conf)
			}
		}
		for _, d := range deny {
			if strings.Contains(conf, d) {
				t.Errorf("unexpected %q\n%s", d, conf)
			}
		}
	}

	// IPv4 only (default): the configuration is unchanged.
	read := render("v4", p)
	check(read("nginx.conf"), []string{"ipv6=off"}, []string{"[::]"})
	check(read("hosts/1.conf"), []string{"listen 80;", "listen 443 ssl;"}, []string{"[::]"})
	check(read("hosts/2.conf"), []string{"listen 80;"}, []string{"[::]"})
	check(read("hosts/redirect-1.conf"), []string{"listen 443 ssl;"}, []string{"[::]"})

	// Listening on IPv6; IPv4-only resolver answers.
	p6 := p
	p6.IPv6 = true
	read = render("v6", p6)
	check(read("nginx.conf"), []string{"listen [::]:80 default_server;", "listen [::]:443 ssl default_server;", "ssl_reject_handshake on;", "ipv6=off"}, nil)
	check(read("hosts/1.conf"), []string{"listen 80;", "listen [::]:80;", "listen 443 ssl;", "listen [::]:443 ssl;"}, nil)
	check(read("hosts/2.conf"), []string{"listen [::]:80;"}, []string{"listen [::]:443"})
	check(read("hosts/redirect-1.conf"), []string{"listen [::]:80;", "listen [::]:443 ssl;"}, nil)
	// The status listener stays on IPv4 loopback.
	check(read("nginx.conf"), []string{"listen 127.0.0.1:9180;"}, []string{"[::]:9180"})

	p6.ResolverIPv6 = true
	read = render("v6r", p6)
	check(read("nginx.conf"), []string{"resolver 127.0.0.11 ipv6=on valid=30s;"}, []string{"ipv6=off"})
}

// Per-client limits key IPv6 clients by their /64 (IPv4 by address), like bans and the
// access queue: the global zones and every rate limit policy use $corvus_client.
func TestRenderClientKeyIPv6Prefix(t *testing.T) {
	ctx := context.Background()
	c, st, _, dir := streamEnv(t)
	rl := &store.RateLimit{Name: "api", RequestsPerSecond: 10, StatusCode: 429, MaxConnections: 5, Enabled: true, ExemptCIDRs: []string{}}
	if err := st.SaveRateLimit(ctx, rl); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "v1")
	if err := c.Render(ctx, st, out); err != nil {
		t.Fatal(err)
	}
	conf := readFile(t, filepath.Join(out, "nginx.conf"))
	for _, want := range []string{
		"map $binary_remote_addr $corvus_client {",
		`"~(?s)^\x00{10}\xff\xff"  $binary_remote_addr;`,
		`"~(?s)^(.{8}).{8}$"      $1;`,
		"default                  $binary_remote_addr;",
		"limit_conn_zone $corvus_client zone=corvus_conn_client:10m;",
		"limit_req_zone $corvus_client zone=corvus_wr_client:10m",
		"limit_req_zone $corvus_client zone=corvus_ch_client:10m",
		`"0"     "$corvus_host $corvus_client";`,
		"limit_conn_zone $corvus_rl1_key zone=corvus_cl1:10m;",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("nginx.conf lacks %q", want)
		}
	}
	if strings.Contains(conf, "zone $binary_remote_addr") || strings.Contains(conf, "$binary_remote_addr$corvus_host") {
		t.Error("a limit zone still keys on the full client address")
	}
	// zones once keyed by $binary_remote_addr must not reappear with another key
	// (nginx refuses such a reload when upgrading or rolling back)
	for _, old := range []string{"corvus_conn_ip", "corvus_wr_api", "corvus_ch_api"} {
		if strings.Contains(conf, old) {
			t.Errorf("nginx.conf reuses the zone name %s", old)
		}
	}
}
