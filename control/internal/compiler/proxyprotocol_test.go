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

// PROXY protocol listeners are added to every public server (proxy, redirection and
// default servers, IPv6 when enabled) only while enabled; ports 80/443 stay unchanged.
// The client address is taken from the PROXY header of trusted proxies at http level;
// hosts that read it from a header keep their own realip settings.
func TestRenderProxyProtocol(t *testing.T) {
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
		{Domains: []string{"tls.example.com"}, ForwardHost: "app", ForwardPort: 8080, Enabled: true, CertificateID: &cert.ID, HTTP2: true},
		{Domains: []string{"plain.example.com"}, ForwardHost: "app", ForwardPort: 8080, Enabled: true},
		{Domains: []string{"cf.example.com"}, ForwardHost: "app", ForwardPort: 8080, Enabled: true, ClientIP: "cloudflare"},
	} {
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SaveRedirectionHost(ctx, &store.RedirectionHost{Domains: []string{"old.example.com"}, ForwardScheme: "https",
		ForwardDomain: "new.example.com", HTTPCode: 301, CertificateID: &cert.ID, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	render := func(name string, p Paths) (func(string) string, error) {
		out := filepath.Join(dir, name)
		if err := (&Compiler{P: p, Cloudflare: func(context.Context, *store.Store) []string { return []string{"173.245.48.0/20"} }}).Render(ctx, st, out); err != nil {
			return nil, err
		}
		return func(f string) string {
			b, err := os.ReadFile(filepath.Join(out, f))
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}, nil
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

	// Off (default): nothing changes.
	read, err := render("off", p)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"nginx.conf", "hosts/1.conf", "hosts/2.conf", "hosts/redirect-1.conf"} {
		check(read(f), nil, []string{"proxy_protocol", "8443", "listen 8080"})
	}

	// Enabled without trusted proxies: refused (the header would never be used).
	pp := store.ProxyProtocol{Enabled: true, HTTPPort: 8080, HTTPSPort: 8443}
	if err := st.SetNetwork(ctx, store.NetworkSettings{RealIPHeader: "X-Forwarded-For", ProxyProtocol: pp}); err != nil {
		t.Fatal(err)
	}
	if _, err := render("untrusted", p); err == nil || !strings.Contains(err.Error(), "trusted proxies") {
		t.Fatalf("no trusted proxies: %v", err)
	}

	if err := st.SetNetwork(ctx, store.NetworkSettings{TrustedProxies: []string{"10.0.0.0/8", "2001:db8::/32"},
		RealIPHeader: "X-Forwarded-For", ProxyProtocol: pp}); err != nil {
		t.Fatal(err)
	}
	read, err = render("on", p)
	if err != nil {
		t.Fatal(err)
	}
	main := read("nginx.conf")
	check(main, []string{
		"listen 80 default_server;", "listen 443 ssl default_server;",
		"listen 8080 proxy_protocol default_server;", "listen 8443 ssl proxy_protocol default_server;",
		"    set_real_ip_from 10.0.0.0/8;\n    set_real_ip_from 2001:db8::/32;\n    real_ip_header proxy_protocol;",
	}, []string{"[::]"})
	// http-level realip, before the servers (inherited by those that don't set their own)
	if strings.Index(main, "real_ip_header proxy_protocol;") > strings.Index(main, "server_name _;") {
		t.Error("realip is not at http level")
	}
	check(read("hosts/1.conf"), []string{"listen 80;", "listen 8080 proxy_protocol;", "listen 443 ssl;", "listen 8443 ssl proxy_protocol;", "http2 on;"},
		[]string{"[::]", "real_ip_header"})
	// no certificate: no TLS listener at all
	check(read("hosts/2.conf"), []string{"listen 80;", "listen 8080 proxy_protocol;"}, []string{"8443", "real_ip_header"})
	// Cloudflare host: own realip (replaces the inherited one)
	check(read("hosts/3.conf"), []string{"listen 8080 proxy_protocol;", "real_ip_header CF-Connecting-IP;"}, []string{"real_ip_header proxy_protocol"})
	check(read("hosts/redirect-1.conf"), []string{"listen 80;", "listen 8080 proxy_protocol;", "listen 443 ssl;", "listen 8443 ssl proxy_protocol;"}, nil)

	// IPv6 listeners follow Paths.IPv6.
	p6 := p
	p6.IPv6 = true
	read, err = render("on6", p6)
	if err != nil {
		t.Fatal(err)
	}
	check(read("nginx.conf"), []string{"listen [::]:8080 proxy_protocol default_server;", "listen [::]:8443 ssl proxy_protocol default_server;"}, nil)
	check(read("hosts/1.conf"), []string{"listen [::]:8080 proxy_protocol;", "listen [::]:8443 ssl proxy_protocol;"}, nil)
	check(read("hosts/redirect-1.conf"), []string{"listen [::]:8080 proxy_protocol;", "listen [::]:8443 ssl proxy_protocol;"}, nil)

	// A stream may not use the PROXY protocol ports while they are enabled.
	pw := p.WithNetwork(store.NetworkSettings{ProxyProtocol: pp})
	if why := StreamConflict(pw, nil, &store.Stream{IncomingPort: 8443, TCP: true, Enabled: true}); !strings.Contains(why, "PROXY protocol HTTPS") {
		t.Errorf("stream on 8443: %q", why)
	}
	if why := StreamConflict(p.WithNetwork(store.NetworkSettings{}), nil, &store.Stream{IncomingPort: 8443, TCP: true, Enabled: true}); why != "" {
		t.Errorf("disabled: %q", why)
	}
}

func TestCheckProxyProtocol(t *testing.T) {
	p := Paths{HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181, AdminPort: 9443, RedisPort: 6379}
	trusted := []string{"10.0.0.0/8"}
	streams := []store.Stream{{ID: 4, IncomingPort: 8080, TCP: true}}
	for _, c := range []struct {
		name    string
		pp      store.ProxyProtocol
		trusted []string
		field   string
		msg     string
	}{
		{"ok", store.ProxyProtocol{Enabled: true, HTTPPort: 8081, HTTPSPort: 8443}, trusted, "", ""},
		{"off ignores conflicts", store.ProxyProtocol{HTTPPort: 80, HTTPSPort: 80}, nil, "", ""},
		{"off checks the range", store.ProxyProtocol{HTTPPort: 0, HTTPSPort: 8443}, nil, "proxy_protocol.http_port", "between 1 and 65535"},
		{"range", store.ProxyProtocol{Enabled: true, HTTPPort: 8081, HTTPSPort: 70000}, trusted, "proxy_protocol.https_port", "between 1 and 65535"},
		{"same", store.ProxyProtocol{Enabled: true, HTTPPort: 8443, HTTPSPort: 8443}, trusted, "proxy_protocol.https_port", "must differ"},
		{"http", store.ProxyProtocol{Enabled: true, HTTPPort: 80, HTTPSPort: 8443}, trusted, "proxy_protocol.http_port", "used by Corvus (HTTP)"},
		{"admin", store.ProxyProtocol{Enabled: true, HTTPPort: 8081, HTTPSPort: 9443}, trusted, "proxy_protocol.https_port", "admin console"},
		{"redis", store.ProxyProtocol{Enabled: true, HTTPPort: 6379, HTTPSPort: 8443}, trusted, "proxy_protocol.http_port", "Valkey"},
		{"stream", store.ProxyProtocol{Enabled: true, HTTPPort: 8080, HTTPSPort: 8443}, trusted, "proxy_protocol.http_port", "stream #4"},
		{"trusted", store.ProxyProtocol{Enabled: true, HTTPPort: 8081, HTTPSPort: 8443}, nil, "trusted_proxies", "trusted proxies"},
	} {
		errs := CheckProxyProtocol(p, c.pp, c.trusted, streams)
		if c.field == "" {
			if len(errs) != 0 {
				t.Errorf("%s: %v", c.name, errs)
			}
			continue
		}
		if len(errs) != 1 || errs[0].Field != c.field || !strings.Contains(errs[0].Message, c.msg) {
			t.Errorf("%s: %v", c.name, errs)
		}
	}
}
