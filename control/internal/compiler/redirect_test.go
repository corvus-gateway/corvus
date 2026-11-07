package compiler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/store"
)

func TestRenderRedirectionHosts(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := Paths{DataDir: filepath.ToSlash(dir), LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181}
	c := &Compiler{P: p}

	issued := &store.Certificate{Name: "old", Kind: "acme", Domains: []string{"old.example.com"}, NotAfter: time.Now().Add(90 * 24 * time.Hour)}
	pending := &store.Certificate{Name: "new", Kind: "acme", Domains: []string{"pending.example.com"}, NotAfter: time.Now(), Status: "pending"}
	for _, cert := range []*store.Certificate{issued, pending} {
		if err := st.CreateCertificate(ctx, cert); err != nil {
			t.Fatal(err)
		}
	}
	certDir := filepath.FromSlash(p.CertDir(issued.ID))
	os.MkdirAll(certDir, 0o755)
	os.WriteFile(filepath.Join(certDir, "fullchain.pem"), []byte("CERT"), 0o644)
	os.WriteFile(filepath.Join(certDir, "privkey.pem"), []byte("KEY"), 0o600)

	hosts := []*store.RedirectionHost{
		{Domains: []string{"old.example.com", "www.old.example.com"}, ForwardScheme: "https", ForwardDomain: "new.example.com", HTTPCode: 301,
			PreservePath: true, CertificateID: &issued.ID, SSLForced: true, HTTP2: true, BlockExploits: true, Enabled: true},
		{Domains: []string{"pending.example.com"}, ForwardScheme: "auto", ForwardDomain: "example.com/landing", HTTPCode: 302,
			CertificateID: &pending.ID, SSLForced: true, Enabled: true},
		{Domains: []string{"off.example.com"}, ForwardScheme: "http", ForwardDomain: "example.com", HTTPCode: 307, Enabled: false},
	}
	for _, h := range hosts {
		if err := st.SaveRedirectionHost(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	all := &store.SecurityRule{Name: "block bots", Enabled: true, Action: "block", StatusCode: 403, Scope: "all",
		Expression: store.Expression{Groups: [][]store.Condition{{{Field: "http.user_agent", Operator: "contains", Value: []string{"badbot"}}}}}}
	if err := st.SaveSecurityRule(ctx, all); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "v1")
	if err := c.Render(ctx, st, out); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	tls := read("hosts/redirect-1.conf")
	for _, want := range []string{
		"server_name old.example.com www.old.example.com;",
		"listen 443 ssl;", "http2 on;", "ssl_certificate     " + p.CertDir(issued.ID) + "/fullchain.pem;",
		"location ^~ /.well-known/acme-challenge/ {\n        proxy_http_version 1.1;\n        proxy_set_header Connection \"\";\n        proxy_pass http://corvus_acme;",
		"if ($corvus_https_redirect) { return 301 https://$host$request_uri; }",
		`set $corvus_redirect "https://new.example.com$request_uri";`, "return ngx.redirect(ngx.var.corvus_redirect, 301)",
		`require("corvus.rules").rewrite("r1")`,
	} {
		if !strings.Contains(tls, want) {
			t.Errorf("redirect-1.conf: missing %q\n%s", want, tls)
		}
	}

	// Certificate not issued yet: HTTP only (ACME HTTP-01 must still reach the challenge),
	// no forced HTTPS; path is not preserved and "auto" keeps the request's scheme.
	plain := read("hosts/redirect-2.conf")
	for _, want := range []string{"listen 80;", "location ^~ /.well-known/acme-challenge/", `set $corvus_redirect "$scheme://example.com/landing";`, "return ngx.redirect(ngx.var.corvus_redirect, 302)"} {
		if !strings.Contains(plain, want) {
			t.Errorf("redirect-2.conf: missing %q\n%s", want, plain)
		}
	}
	for _, bad := range []string{"ssl", "$request_uri", "corvus_https_redirect"} {
		if strings.Contains(plain, bad) {
			t.Errorf("redirect-2.conf: unexpected %q\n%s", bad, plain)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "hosts", "redirect-3.conf")); err == nil {
		t.Error("disabled redirection host was rendered")
	}

	var rules struct {
		Hosts map[string]struct {
			Rules    []int `json:"rules"`
			Exploits bool  `json:"exploits"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal([]byte(read("rules.json")), &rules); err != nil {
		t.Fatal(err)
	}
	if r1, ok := rules.Hosts["r1"]; !ok || !r1.Exploits || len(r1.Rules) != 1 {
		t.Errorf("rules.json r1: %+v (all: %+v)", r1, rules.Hosts)
	}
	if _, ok := rules.Hosts["r3"]; ok {
		t.Error("disabled redirection host in rules.json")
	}
	if !strings.Contains(read("nginx.conf"), "2 redirection host(s)") {
		t.Error("nginx.conf does not count redirection hosts")
	}
}

func TestRedirectTarget(t *testing.T) {
	cases := []struct {
		h    store.RedirectionHost
		want string
	}{
		{store.RedirectionHost{ForwardScheme: "auto", ForwardDomain: "a.example.com", PreservePath: true}, "$scheme://a.example.com$request_uri"},
		{store.RedirectionHost{ForwardScheme: "https", ForwardDomain: "a.example.com/base/", PreservePath: true}, "https://a.example.com/base$request_uri"},
		{store.RedirectionHost{ForwardScheme: "http", ForwardDomain: "a.example.com:8080/x"}, "http://a.example.com:8080/x"},
	}
	for _, c := range cases {
		if got := redirectTarget(c.h); got != c.want {
			t.Errorf("%+v: got %q want %q", c.h, got, c.want)
		}
	}
}
