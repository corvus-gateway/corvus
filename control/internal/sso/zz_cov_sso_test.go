package sso

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCovParseAndDefaults(t *testing.T) {
	d := Defaults()
	if d.DisplayName != "SSO" || len(d.Scopes) != 3 || d.DefaultRole != "viewer" || !d.SyncRole || d.Enabled {
		t.Fatalf("defaults: %+v", d)
	}
	s, err := Parse("  ")
	if err != nil || s.DisplayName != "SSO" {
		t.Fatalf("empty: %+v %v", s, err)
	}
	s, err = Parse(`{"enabled":true,"issuer":"https://idp","client_id":"c","scopes":null,"allowed_domains":null,"admin_values":null,"default_role":""}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.Scopes, " ") != DefaultScopes || s.AllowedDomains == nil || s.AdminValues == nil || s.OperatorValues == nil || s.DefaultRole != "viewer" {
		t.Fatalf("nil fields must be defaulted: %+v", s)
	}
	if !s.Ready() {
		t.Fatal("enabled with issuer and client is ready")
	}
	if (Settings{Enabled: true, Issuer: "x"}).Ready() || (Settings{Issuer: "x", ClientID: "c"}).Ready() {
		t.Fatal("incomplete or disabled settings are not ready")
	}
	if s, err := Parse(`{bad`); err == nil || s.DisplayName != "SSO" {
		t.Fatalf("corrupt settings fall back to defaults with an error: %+v %v", s, err)
	}
	if (Settings{DisplayName: "  "}).Label() != "SSO" || (Settings{DisplayName: "Okta"}).Label() != "Okta" {
		t.Fatal("label")
	}
}

func fieldErrs(t *testing.T, err error) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err == nil {
		return out
	}
	b, _ := json.Marshal(err)
	var list []struct{ Field, Code string }
	if json.Unmarshal(b, &list) != nil || len(list) == 0 {
		// fall back to the message
		out["?"] = err.Error()
		return out
	}
	for _, e := range list {
		out[strings.ToLower(e.Field)] = e.Code
	}
	return out
}

func TestCovNormalize(t *testing.T) {
	s := Settings{
		Enabled:        true,
		DisplayName:    "  Corp  ",
		Issuer:         " https://idp.example.com/realms/corp ",
		ClientID:       " corvus ",
		Scopes:         []string{"email,profile", "groups email"},
		AllowedDomains: []string{"@Example.COM", "example.com", " ", "corp.example.org"},
		RoleClaim:      "realm_access.roles",
		AdminValues:    []string{" admins ", "admins", ""},
		OperatorValues: []string{"ops"},
	}
	if err := s.Normalize(); err != nil {
		t.Fatal(err)
	}
	if s.DisplayName != "Corp" || s.Issuer != "https://idp.example.com/realms/corp" || s.ClientID != "corvus" {
		t.Fatalf("trimmed: %+v", s)
	}
	if strings.Join(s.Scopes, " ") != "openid email profile groups" {
		t.Fatalf("scopes: %v", s.Scopes)
	}
	if strings.Join(s.AllowedDomains, ",") != "example.com,corp.example.org" {
		t.Fatalf("domains: %v", s.AllowedDomains)
	}
	if strings.Join(s.AdminValues, ",") != "admins" || s.DefaultRole != "viewer" {
		t.Fatalf("values: %+v", s)
	}

	// empty → defaults, disabled needs nothing
	e := Settings{}
	if err := e.Normalize(); err != nil || e.DisplayName != "SSO" || strings.Join(e.Scopes, " ") != DefaultScopes {
		t.Fatalf("empty: %+v %v", e, err)
	}

	bad := Settings{
		Enabled:        true,
		DisplayName:    strings.Repeat("x", 65),
		ClientID:       "has space",
		Scopes:         []string{"bad\"scope"},
		AllowedDomains: []string{"*.example.com"},
		RoleClaim:      "roles with space",
		AdminValues:    []string{strings.Repeat("a", 257)},
		DefaultRole:    "admin",
	}
	err := bad.Normalize()
	if err == nil {
		t.Fatal("invalid settings must fail")
	}
	msg := err.Error()
	for _, want := range []string{"display name", "issuer URL is required", "client ID must be", "invalid scope", "invalid domain", "claim name", "256 characters", "default role"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q lacks %q", msg, want)
		}
	}
	if fe := fieldErrs(t, err); len(fe) == 0 {
		t.Fatal("expected field errors")
	}

	missing := Settings{Enabled: true, Issuer: "ftp://idp.example.com"}
	err = missing.Normalize()
	if err == nil || !strings.Contains(err.Error(), "client ID is required") || !strings.Contains(err.Error(), "https") {
		t.Fatalf("missing client and bad issuer: %v", err)
	}

	many := Settings{}
	for i := 0; i < 21; i++ {
		many.Scopes = append(many.Scopes, "s"+strings.Repeat("x", i))
	}
	for i := 0; i < 51; i++ {
		many.AllowedDomains = append(many.AllowedDomains, "d"+strings.Repeat("x", i)+".example.com")
	}
	for i := 0; i < 101; i++ {
		many.OperatorValues = append(many.OperatorValues, "v"+strings.Repeat("x", i))
	}
	err = many.Normalize()
	if err == nil || !strings.Contains(err.Error(), "at most 20 scopes") || !strings.Contains(err.Error(), "at most 50 domains") || !strings.Contains(err.Error(), "at most 100 values") {
		t.Fatalf("limits: %v", err)
	}
}

func TestCovCheckIssuer(t *testing.T) {
	ok := []string{"https://idp.example.com", "http://localhost:8080/realms/x", "http://127.0.0.1:9000", "http://app.localhost", "http://[::1]:80"}
	for _, u := range ok {
		if err := CheckIssuer(u); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
	bad := map[string]string{
		"http://idp.example.com":              "https",
		"https://u:p@idp.example.com":         "credentials",
		"https://idp.example.com/?a=1":        "query",
		"https://idp.example.com/#frag":       "fragment",
		"/relative":                           "absolute",
		"ftp://idp.example.com":               "https",
		"https://" + strings.Repeat("a", 520): "too long",
	}
	for u, want := range bad {
		if err := CheckIssuer(u); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v (want %q)", u, err, want)
		}
	}
	if stripQuery("https://a/b?c#d") != "https://a/b" || stripQuery("https://a/b#x") != "https://a/b" || stripQuery("https://a") != "https://a" {
		t.Fatal("stripQuery")
	}
}

func TestCovBlockedIP(t *testing.T) {
	cases := []struct {
		ip    string
		local bool
		want  bool
	}{
		{"127.0.0.1", false, true},
		{"127.0.0.1", true, false},
		{"169.254.169.254", true, true},
		{"0.0.0.0", false, true},
		{"224.0.0.1", false, true},
		{"10.1.2.3", false, false},
		{"8.8.8.8", false, false},
		{"fe80::1", false, true},
	}
	for _, c := range cases {
		if got := blockedIP(net.ParseIP(c.ip), c.local); got != c.want {
			t.Errorf("blockedIP(%s, %v) = %v", c.ip, c.local, got)
		}
	}
}

type fakeRT struct {
	body string
	err  error
	n    int
}

func (f *fakeRT) RoundTrip(r *http.Request) (*http.Response, error) {
	f.n++
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(f.body)), Request: r}, nil
}

func TestCovGuardTransport(t *testing.T) {
	get := func(g *guardTransport, u string) (*http.Response, error) {
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		return g.RoundTrip(req)
	}
	base := &fakeRT{body: "hello"}
	g := &guardTransport{base: base}
	if _, err := get(g, "http://idp.example.com/x"); err == nil || !strings.Contains(err.Error(), "non-https") {
		t.Fatalf("plain http: %v", err)
	}
	if _, err := get(g, "https://169.254.169.254/latest"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("metadata address: %v", err)
	}
	if _, err := get(g, "https://127.0.0.1/x"); err == nil {
		t.Fatal("loopback is blocked for a remote issuer")
	}
	if base.n != 0 {
		t.Fatal("refused requests must not reach the network")
	}
	resp, err := get(g, "https://idp.example.com/x")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "hello" {
		t.Fatalf("body %q", b)
	}
	local := &guardTransport{base: base, local: true}
	if _, err := get(local, "http://localhost:8080/x"); err != nil {
		t.Fatalf("local http: %v", err)
	}
	if _, err := get(local, "http://idp.example.com/x"); err == nil {
		t.Fatal("http to a remote host stays forbidden for a local issuer")
	}
	boom := errors.New("dial failed")
	if _, err := get(&guardTransport{base: &fakeRT{err: boom}}, "https://idp.example.com"); !errors.Is(err, boom) {
		t.Fatalf("transport error = %v", err)
	}
}

func TestCovLimitedBody(t *testing.T) {
	read := func(body string, n int64) (string, error) {
		l := &limitedBody{rc: io.NopCloser(strings.NewReader(body)), n: n}
		b, err := io.ReadAll(l)
		_ = l.Close()
		return string(b), err
	}
	if s, err := read("12345", 5); err != nil || s != "12345" {
		t.Fatalf("exactly at the limit: %q %v", s, err)
	}
	if s, err := read("123", 5); err != nil || s != "123" {
		t.Fatalf("below the limit: %q %v", s, err)
	}
	if _, err := read("123456", 5); !errors.Is(err, errTooLarge) {
		t.Fatalf("over the limit: %v", err)
	}
}

func TestCovNewHTTPClientAndDiscover(t *testing.T) {
	mode := "ok"
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	// httptest listens on 127.0.0.1; use "localhost" so the issuer may use http
	issuer := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)

	c := NewHTTPClient(issuer)
	if c.CheckRedirect == nil || c.Timeout == 0 {
		t.Fatal("client must not follow redirects and must time out")
	}
	ctx := context.Background()
	if ClientContext(ctx, c) == ctx {
		t.Fatal("client context must carry the client")
	}

	// the discovery document advertises the 127.0.0.1 URL; go-oidc requires the issuer to match
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := map[string]any{
			"issuer":                 issuer,
			"authorization_endpoint": issuer + "/auth?x=1",
			"token_endpoint":         issuer + "/token",
			"jwks_uri":               issuer + "/jwks",
		}
		switch mode {
		case "missing":
			delete(d, "jwks_uri")
		case "remote-http":
			d["token_endpoint"] = "http://evil.example.com/token"
		case "redirect":
			http.Redirect(w, r, "https://elsewhere.example.com/", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(d)
	})
	p, d, err := Discover(ctx, nil, issuer)
	if err != nil || p == nil || d.TokenEndpoint != issuer+"/token" || d.Issuer != issuer {
		t.Fatalf("discover: %+v %v", d, err)
	}
	mode = "missing"
	if _, _, err := Discover(ctx, c, issuer); err == nil || !strings.Contains(err.Error(), "lacks") {
		t.Fatalf("incomplete document: %v", err)
	}
	mode = "remote-http"
	if _, _, err := Discover(ctx, c, issuer); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("plain-http endpoint: %v", err)
	}
	mode = "redirect"
	if _, _, err := Discover(ctx, c, issuer); err == nil || !strings.Contains(err.Error(), "discovery failed") {
		t.Fatalf("redirected discovery: %v", err)
	}
	if _, _, err := Discover(ctx, c, "http://idp.example.com"); err == nil {
		t.Fatal("invalid issuer is rejected before any request")
	}
	// a remote issuer's client refuses to dial loopback addresses
	remote := NewHTTPClient("https://idp.example.com")
	if _, err := remote.Get("https://127.0.0.1:1/"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("loopback from a remote client: %v", err)
	}
}

func TestCovClaims(t *testing.T) {
	c := ParseClaims(map[string]any{
		"sub":                       "u1",
		"email":                     "  Alice@Example.COM ",
		"email_verified":            "TRUE",
		"preferred_username":        "alice",
		"groups":                    []any{"admins", "staff", 7.0, true, map[string]any{}},
		"scope":                     "a b,c",
		"level":                     3.0,
		"https://example.com/roles": []any{"ops"},
		"realm_access":              map[string]any{"roles": []any{"r1"}},
	})
	if c.Subject != "u1" || c.Email != "alice@example.com" || c.EmailVerified == nil || !*c.EmailVerified || c.Name != "alice" {
		t.Fatalf("claims: %+v", c)
	}
	if v := c.Values("groups"); strings.Join(v, ",") != "admins,staff,7,true" {
		t.Fatalf("array values: %v", v)
	}
	if v := c.Values("scope"); strings.Join(v, ",") != "a,b,c" {
		t.Fatalf("string values: %v", v)
	}
	if v := c.Values("level"); len(v) != 1 || v[0] != "3" {
		t.Fatalf("number value: %v", v)
	}
	if v := c.Values("realm_access.roles"); len(v) != 1 || v[0] != "r1" {
		t.Fatalf("nested: %v", v)
	}
	if v := c.Values("https://example.com/roles"); len(v) != 1 || v[0] != "ops" {
		t.Fatalf("literal key with dots: %v", v)
	}
	for _, missing := range []string{"", "nope", "realm_access.nope", "scope.sub", "realm_access"} {
		if v := c.Values(missing); v != nil {
			t.Errorf("Values(%q) = %v", missing, v)
		}
	}

	// ID token without email: userinfo fills it in
	c2 := ParseClaims(map[string]any{"sub": "u2", "email_verified": false})
	if c2.EmailVerified == nil || *c2.EmailVerified {
		t.Fatal("bool email_verified")
	}
	c2.Merge(map[string]any{"email": "bob@example.com", "email_verified": true, "given_name": " Bob ", "groups": []any{"x"}, "sub": "other"})
	if c2.Email != "bob@example.com" || !*c2.EmailVerified || c2.Name != "Bob" {
		t.Fatalf("merged: %+v", c2)
	}
	if c2.Subject != "u2" || c2.raw["sub"] != "u2" {
		t.Fatal("existing claims are not overwritten")
	}
	if v := c2.Values("groups"); len(v) != 1 {
		t.Fatalf("merged raw claims: %v", v)
	}
	// email_verified missing from the ID token: taken from userinfo for the same email only
	c3 := ParseClaims(map[string]any{"sub": "u3", "email": "carol@example.com"})
	c3.Merge(map[string]any{"email": "other@example.com", "email_verified": true})
	if c3.EmailVerified != nil {
		t.Fatal("email_verified of another address")
	}
	c3.Merge(map[string]any{"email": "Carol@Example.com", "email_verified": true})
	if c3.EmailVerified == nil || !*c3.EmailVerified {
		t.Fatal("email_verified from userinfo")
	}
	c3.Merge(map[string]any{"email": "carol@example.com", "email_verified": false})
	if !*c3.EmailVerified {
		t.Fatal("a known email_verified is not overwritten")
	}
	f, tr := false, true
	for _, tc := range []struct {
		v     *bool
		trust bool
		want  bool
	}{{nil, false, false}, {nil, true, true}, {&f, true, false}, {&tr, false, true}} {
		if got := (Settings{TrustEmail: tc.trust}).EmailVerified(Claims{EmailVerified: tc.v}); got != tc.want {
			t.Errorf("EmailVerified(%v, trust %v) = %v", tc.v, tc.trust, got)
		}
	}
	// already set: Merge keeps the ID token values
	c.Merge(map[string]any{"email": "x@y", "name": "X"})
	if c.Email != "alice@example.com" || c.Name != "alice" {
		t.Fatalf("merge must not override: %+v", c)
	}
}

func TestCovDomainAndRole(t *testing.T) {
	s := Settings{AllowedDomains: []string{"example.com"}, DefaultRole: "viewer", RoleClaim: "groups", AdminValues: []string{"admins"}, OperatorValues: []string{"ops"}}
	if !s.DomainAllowed("a@EXAMPLE.com") || s.DomainAllowed("a@other.com") || s.DomainAllowed("no-at") {
		t.Fatal("domain allow-list")
	}
	if !(Settings{}).DomainAllowed("anything") {
		t.Fatal("empty list allows any domain")
	}
	claims := func(groups ...any) Claims { return ParseClaims(map[string]any{"groups": groups}) }
	if r, m := s.MapRole(claims("ops", "admins")); r != "admin" || !m {
		t.Fatalf("admin wins: %s %v", r, m)
	}
	if r, m := s.MapRole(claims("ops")); r != "operator" || !m {
		t.Fatalf("operator: %s %v", r, m)
	}
	if r, m := s.MapRole(claims("staff")); r != "viewer" || !m {
		t.Fatalf("default: %s %v", r, m)
	}
	s.DefaultRole = "deny"
	if r, m := s.MapRole(claims("staff")); r != "" || !m {
		t.Fatalf("deny: %q %v", r, m)
	}
	s.RoleClaim = ""
	if r, m := s.MapRole(claims("admins")); r != "" || m {
		t.Fatalf("no role claim: %q %v", r, m)
	}
}
