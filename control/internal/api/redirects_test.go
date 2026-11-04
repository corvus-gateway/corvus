package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/npmimport"
	"corvus/control/internal/store"
)

func TestRedirectionHostDomainConflicts(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	st := e.s.Store
	if err := st.SaveProxyHost(ctx, &store.ProxyHost{Domains: []string{"app.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/redirection-hosts", nil)
	rh := &store.RedirectionHost{Domains: []string{"App.Example.com"}, ForwardDomain: "example.com"}
	if err := checkRedirectionHost(r, st, rh); err == nil || !strings.Contains(err.Error(), "proxy host #1") {
		t.Fatalf("conflict with a proxy host not reported: %v", err)
	}
	rh = &store.RedirectionHost{Domains: []string{"old.example.com"}, ForwardDomain: "example.com"}
	if err := checkRedirectionHost(r, st, rh); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveRedirectionHost(ctx, rh); err != nil {
		t.Fatal(err)
	}
	// editing the host itself is not a conflict
	if err := checkRedirectionHost(r, st, rh); err != nil {
		t.Fatalf("self conflict: %v", err)
	}
	other := &store.RedirectionHost{Domains: []string{"old.example.com"}, ForwardDomain: "example.org"}
	if err := checkRedirectionHost(r, st, other); err == nil || !strings.Contains(err.Error(), "redirection host #1") {
		t.Fatalf("conflict with a redirection host not reported: %v", err)
	}
	ph := &store.ProxyHost{Domains: []string{"old.example.com"}, ForwardHost: "app", ForwardPort: 80}
	if err := checkProxyHost(r, st, ph); err == nil || !strings.Contains(err.Error(), "redirection host #1") {
		t.Fatalf("proxy host may not take a redirection host's domain: %v", err)
	}
	missing := int64(42)
	bad := &store.RedirectionHost{Domains: []string{"x.example.com"}, ForwardDomain: "example.com", CertificateID: &missing}
	if err := checkRedirectionHost(r, st, bad); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("missing certificate accepted: %v", err)
	}
}

func TestRedirectionHostRoutes(t *testing.T) {
	e := newTestEnv(t)
	if w, _ := e.do("GET", "/api/redirection-hosts", nil, withCookie(e.cookie)); w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	// A viewer session (not an API token: token "last used" bookkeeping is process-global).
	ctx := context.Background()
	v, err := e.s.Store.CreateUser(ctx, "viewer@example.com", "Viewer", "x", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	raw := auth.RandomToken(32)
	if err := e.s.Store.CreateSession(ctx, auth.TokenHash(raw), v.ID, time.Hour, "127.0.0.1", "test"); err != nil {
		t.Fatal(err)
	}
	if w, _ := e.do("GET", "/api/redirection-hosts", nil, withCookie(raw)); w.Code != 200 {
		t.Fatalf("viewer list: %d", w.Code)
	}
	body := map[string]any{"domains": []string{"a.example.com"}, "forward_domain": "example.com"}
	if w, _ := e.do("POST", "/api/redirection-hosts", body, withCookie(raw), withCSRF()); w.Code != 403 {
		t.Fatalf("viewer create: %d %s", w.Code, w.Body)
	}
	if w, _ := e.do("DELETE", "/api/redirection-hosts/1", nil, withCookie(raw), withCSRF()); w.Code != 403 {
		t.Fatalf("viewer delete: %d", w.Code)
	}
}

func TestNPMRedirectMapping(t *testing.T) {
	h := npmimport.Redirect{NPMID: 7, Domains: []string{"old.example.com"}, ForwardScheme: "https", ForwardDomain: "new.example.com", HTTPCode: 308,
		PreservePath: true, CertificateID: 3, SSLForced: true, HTTP2: true, BlockExploits: true, Enabled: true, AdvancedConfig: "add_header X-A b;"}
	x := npmRedirect(h, map[int64]int64{3: 11})
	if x.CertificateID == nil || *x.CertificateID != 11 || !x.SSLForced || x.HTTPCode != 308 || !x.PreservePath || x.ForwardScheme != "https" ||
		x.ForwardDomain != "new.example.com" || !x.HTTP2 || !x.BlockExploits || !x.Enabled || x.AdvancedConfig != "add_header X-A b;" {
		t.Fatalf("mapping: %+v", x)
	}
	// certificate not imported: plain HTTP, no forced HTTPS (would fail validation)
	if x := npmRedirect(h, map[int64]int64{}); x.CertificateID != nil || x.SSLForced {
		t.Fatalf("unmapped certificate: %+v", x)
	}
}
