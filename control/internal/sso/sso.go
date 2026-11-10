// Package sso implements single sign-on for the admin console via OpenID Connect:
// the settings model (stored in the meta table, client secret sealed with the data
// key), an SSRF-guarded HTTP client for talking to the identity provider, and the
// claim helpers (email, domain allow-list, role mapping).
package sso

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"corvus/control/internal/validate"
)

// MetaKey is the meta table key holding the JSON settings.
const MetaKey = "sso"

// Default values.
const (
	DefaultScopes      = "openid email profile"
	DefaultDisplayName = "SSO"
	maxBody            = 1 << 20 // per IdP response
)

// Settings is the stored configuration. ClientSecret holds the sealed value
// (seal.Seal with the data key) and is never sent to the browser.
type Settings struct {
	Enabled        bool     `json:"enabled"`
	DisplayName    string   `json:"display_name"`
	Issuer         string   `json:"issuer"`
	ClientID       string   `json:"client_id"`
	ClientSecret   string   `json:"client_secret,omitempty"` // sealed
	Scopes         []string `json:"scopes"`
	AllowedDomains []string `json:"allowed_domains"`
	RoleClaim      string   `json:"role_claim"`
	AdminValues    []string `json:"admin_values"`
	OperatorValues []string `json:"operator_values"`
	DefaultRole    string   `json:"default_role"` // "viewer" | "deny"
	AutoProvision  bool     `json:"auto_provision"`
	SyncRole       bool     `json:"sync_role"` // update the user's role from the mapping on every login
	// TrustEmail: the IdP only issues verified email addresses, so a token without an
	// email_verified claim may link an existing user (an explicit false still refuses).
	TrustEmail bool `json:"trust_email"`
}

// Defaults returns the settings used before anything was saved.
func Defaults() Settings {
	return Settings{DisplayName: DefaultDisplayName, Scopes: strings.Fields(DefaultScopes), AllowedDomains: []string{},
		AdminValues: []string{}, OperatorValues: []string{}, DefaultRole: "viewer", SyncRole: true}
}

// Parse decodes stored settings (empty → defaults).
func Parse(raw string) (Settings, error) {
	s := Defaults()
	if strings.TrimSpace(raw) == "" {
		return s, nil
	}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return Defaults(), err
	}
	if s.Scopes == nil {
		s.Scopes = strings.Fields(DefaultScopes)
	}
	for _, p := range []*[]string{&s.AllowedDomains, &s.AdminValues, &s.OperatorValues} {
		if *p == nil {
			*p = []string{}
		}
	}
	if s.DefaultRole == "" {
		s.DefaultRole = "viewer"
	}
	return s, nil
}

// EmailVerified reports whether cl's email may link an existing user: the IdP says it
// is verified, or it does not say and the IdP is trusted to verify emails.
func (s Settings) EmailVerified(cl Claims) bool {
	if cl.EmailVerified != nil {
		return *cl.EmailVerified
	}
	return s.TrustEmail
}

// Ready reports whether the configuration is complete enough to offer the SSO button.
func (s Settings) Ready() bool { return s.Enabled && s.Issuer != "" && s.ClientID != "" }

// Label is the sign-in button text.
func (s Settings) Label() string {
	if strings.TrimSpace(s.DisplayName) == "" {
		return DefaultDisplayName
	}
	return s.DisplayName
}

var (
	scopeRe = regexp.MustCompile(`^[\x21\x23-\x5b\x5d-\x7e]{1,64}$`)
	claimRe = regexp.MustCompile(`^[A-Za-z0-9_:/.\-]{1,128}$`)
	// a lower-case DNS name (no wildcard)
	domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9-]{2,63}$|^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
)

// Normalize trims and validates s in place (the client secret is not touched).
func (s *Settings) Normalize() error {
	var errs validate.Errors
	add := func(field, code, msg string) {
		errs = append(errs, validate.FieldError{Field: field, Code: code, Message: msg})
	}
	s.DisplayName = strings.TrimSpace(s.DisplayName)
	if s.DisplayName == "" {
		s.DisplayName = DefaultDisplayName
	}
	if len([]rune(s.DisplayName)) > 64 {
		add("display_name", "invalid", "display name must be at most 64 characters")
	}
	s.Issuer = strings.TrimSpace(s.Issuer)
	if s.Issuer != "" {
		if err := CheckIssuer(s.Issuer); err != nil {
			add("issuer", "invalid", err.Error())
		}
	} else if s.Enabled {
		add("issuer", "required", "issuer URL is required")
	}
	s.ClientID = strings.TrimSpace(s.ClientID)
	if s.ClientID == "" && s.Enabled {
		add("client_id", "required", "client ID is required")
	}
	if len(s.ClientID) > 256 || strings.ContainsAny(s.ClientID, " \t\r\n") {
		add("client_id", "invalid", "client ID must be at most 256 characters without spaces")
	}
	// scopes: split on spaces/commas, dedupe, openid always first
	var scopes []string
	for _, sc := range s.Scopes {
		for _, f := range strings.FieldsFunc(sc, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' }) {
			if !slices.Contains(scopes, f) {
				scopes = append(scopes, f)
			}
		}
	}
	if len(scopes) == 0 {
		scopes = strings.Fields(DefaultScopes)
	}
	if !slices.Contains(scopes, oidc.ScopeOpenID) {
		scopes = append([]string{oidc.ScopeOpenID}, scopes...)
	}
	for _, sc := range scopes {
		if !scopeRe.MatchString(sc) {
			add("scopes", "invalid", fmt.Sprintf("invalid scope %q", sc))
			break
		}
	}
	if len(scopes) > 20 {
		add("scopes", "invalid", "at most 20 scopes")
	}
	s.Scopes = scopes

	domains := []string{}
	for _, d := range s.AllowedDomains {
		d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@"))
		if d == "" || slices.Contains(domains, d) {
			continue
		}
		if !domainRe.MatchString(d) || len(d) > 253 {
			add("allowed_domains", "invalid", fmt.Sprintf("invalid domain %q", d))
			continue
		}
		domains = append(domains, d)
	}
	if len(domains) > 50 {
		add("allowed_domains", "invalid", "at most 50 domains")
	}
	s.AllowedDomains = domains

	s.RoleClaim = strings.TrimSpace(s.RoleClaim)
	if s.RoleClaim != "" && !claimRe.MatchString(s.RoleClaim) {
		add("role_claim", "invalid", "claim name may contain letters, digits and _ : / . - (use dots for nested claims)")
	}
	clean := func(field string, in []string) []string {
		out := []string{}
		for _, v := range in {
			v = strings.TrimSpace(v)
			if v == "" || slices.Contains(out, v) {
				continue
			}
			if len(v) > 256 {
				add(field, "invalid", "values must be at most 256 characters")
				continue
			}
			out = append(out, v)
		}
		if len(out) > 100 {
			add(field, "invalid", "at most 100 values")
		}
		return out
	}
	s.AdminValues = clean("admin_values", s.AdminValues)
	s.OperatorValues = clean("operator_values", s.OperatorValues)
	if s.DefaultRole == "" {
		s.DefaultRole = "viewer"
	}
	if s.DefaultRole != "viewer" && s.DefaultRole != "deny" {
		add("default_role", "invalid", `default role must be "viewer" or "deny"`)
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

// CheckIssuer validates the issuer URL: https only (http is accepted for
// localhost so tests and local IdPs work), no credentials, query or fragment.
func CheckIssuer(issuer string) error {
	if len(issuer) > 512 {
		return errors.New("issuer URL is too long")
	}
	u, err := url.Parse(issuer)
	if err != nil || u.Host == "" {
		return errors.New("issuer must be an absolute URL such as https://idp.example.com/realms/corp")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("issuer must not contain credentials, a query or a fragment")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !isLocalHost(u.Hostname()) {
			return errors.New("issuer must use https")
		}
	default:
		return errors.New("issuer must use https")
	}
	return nil
}

func isLocalHost(h string) bool {
	h = strings.ToLower(strings.TrimSuffix(h, "."))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// ------------------------------------------------------------------ HTTP client

// blockedIP mirrors the alert webhook guard: never reach link-local (cloud
// metadata), unspecified or multicast addresses, nor loopback unless the issuer
// itself is on localhost. Private networks stay allowed for on-prem IdPs.
func blockedIP(ip net.IP, allowLoopback bool) bool {
	if ip.IsLoopback() {
		return !allowLoopback
	}
	return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// guardTransport enforces the scheme rule on every request (discovery may point the
// JWKS / token endpoints anywhere) and caps response sizes.
type guardTransport struct {
	base  http.RoundTripper
	local bool // issuer on localhost: http and loopback allowed
}

func (g *guardTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	switch {
	case r.URL.Scheme == "https":
	case r.URL.Scheme == "http" && g.local && isLocalHost(r.URL.Hostname()):
	default:
		return nil, fmt.Errorf("sso: refusing non-https request to %s", r.URL.Host)
	}
	if ip := net.ParseIP(r.URL.Hostname()); ip != nil && blockedIP(ip, g.local) {
		return nil, fmt.Errorf("sso: destination %s is not allowed", r.URL.Hostname())
	}
	resp, err := g.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	resp.Body = &limitedBody{rc: resp.Body, n: maxBody}
	return resp, nil
}

type limitedBody struct {
	rc io.ReadCloser
	n  int64
}

var errTooLarge = errors.New("sso: identity provider response too large")

func (l *limitedBody) Read(p []byte) (int, error) {
	if l.n <= 0 {
		// allow detecting EOF exactly at the limit
		var one [1]byte
		if n, _ := l.rc.Read(one[:]); n > 0 {
			return 0, errTooLarge
		}
		return 0, io.EOF
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.rc.Read(p)
	l.n -= int64(n)
	return n, err
}

func (l *limitedBody) Close() error { return l.rc.Close() }

// NewHTTPClient returns the client used for every request to the IdP of issuer.
func NewHTTPClient(issuer string) *http.Client {
	local := false
	if u, err := url.Parse(issuer); err == nil {
		local = isLocalHost(u.Hostname())
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: func(network, address string, _ syscall.RawConn) error {
		host, _, _ := net.SplitHostPort(address)
		if ip := net.ParseIP(host); ip != nil && blockedIP(ip, local) {
			return fmt.Errorf("destination %s is not allowed", host)
		}
		return nil
	}}
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
	}
	return &http.Client{
		Timeout:       15 * time.Second,
		Transport:     &guardTransport{base: tr, local: local},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// ClientContext attaches the guarded client for go-oidc and x/oauth2 calls.
func ClientContext(ctx context.Context, c *http.Client) context.Context {
	return oidc.ClientContext(ctx, c) // same key x/oauth2 uses (oauth2.HTTPClient)
}

// Discovery is the subset of the provider metadata shown to administrators.
type Discovery struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	UserinfoEndpoint      string   `json:"userinfo_endpoint,omitempty"`
	JWKSURI               string   `json:"jwks_uri"`
	ScopesSupported       []string `json:"scopes_supported,omitempty"`
	CodeChallengeMethods  []string `json:"code_challenge_methods_supported,omitempty"`
	SigningAlgs           []string `json:"id_token_signing_alg_values_supported,omitempty"`
}

// Discover fetches and checks the issuer's discovery document. c should come from
// NewHTTPClient(issuer); nil creates one.
func Discover(ctx context.Context, c *http.Client, issuer string) (*oidc.Provider, *Discovery, error) {
	if err := CheckIssuer(issuer); err != nil {
		return nil, nil, err
	}
	if c == nil {
		c = NewHTTPClient(issuer)
	}
	// The provider keeps only the client (not the context) for later JWKS fetches.
	pctx, cancel := context.WithTimeout(ClientContext(ctx, c), 15*time.Second)
	defer cancel()
	p, err := oidc.NewProvider(pctx, issuer)
	if err != nil {
		return nil, nil, fmt.Errorf("discovery failed: %w", err)
	}
	var d Discovery
	if err := p.Claims(&d); err != nil {
		return nil, nil, fmt.Errorf("discovery document is invalid: %w", err)
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" || d.JWKSURI == "" {
		return nil, nil, errors.New("discovery document lacks authorization_endpoint, token_endpoint or jwks_uri")
	}
	for _, ep := range []string{d.AuthorizationEndpoint, d.TokenEndpoint, d.JWKSURI} {
		if err := CheckIssuer(stripQuery(ep)); err != nil {
			return nil, nil, fmt.Errorf("discovered endpoint %s is not allowed: %v", ep, err)
		}
	}
	return p, &d, nil
}

func stripQuery(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		return u[:i]
	}
	return u
}

// ------------------------------------------------------------------ claims

// Claims are the identity attributes read from the ID token (and userinfo).
type Claims struct {
	Subject       string
	Email         string
	EmailVerified *bool
	Name          string
	raw           map[string]any
}

// ParseClaims extracts the standard fields from a raw claim set.
func ParseClaims(raw map[string]any) Claims {
	c := Claims{raw: raw}
	c.Subject, _ = raw["sub"].(string)
	c.Email, _ = raw["email"].(string)
	c.Email = strings.ToLower(strings.TrimSpace(c.Email))
	switch v := raw["email_verified"].(type) {
	case bool:
		c.EmailVerified = &v
	case string: // some IdPs send "true"/"false"
		b := strings.EqualFold(v, "true")
		c.EmailVerified = &b
	}
	for _, k := range []string{"name", "preferred_username", "given_name"} {
		if v, _ := raw[k].(string); strings.TrimSpace(v) != "" {
			c.Name = strings.TrimSpace(v)
			break
		}
	}
	return c
}

// Merge fills missing fields (and claims) from userinfo. The caller checks that the
// userinfo subject is the ID token's. email_verified is taken from userinfo when the
// ID token lacks it and userinfo reports the same email.
func (c *Claims) Merge(other map[string]any) {
	o := ParseClaims(other)
	if c.Email == "" && o.Email != "" {
		c.Email, c.EmailVerified = o.Email, o.EmailVerified
	} else if c.EmailVerified == nil && o.EmailVerified != nil && o.Email == c.Email {
		c.EmailVerified = o.EmailVerified
	}
	if c.Name == "" {
		c.Name = o.Name
	}
	for k, v := range other {
		if _, ok := c.raw[k]; !ok {
			c.raw[k] = v
		}
	}
}

// Values returns the string values of claim (dots address nested objects, e.g.
// "realm_access.roles"). Arrays, single strings and space/comma separated
// strings are accepted.
func (c Claims) Values(claim string) []string {
	if claim == "" {
		return nil
	}
	var cur any = map[string]any(c.raw)
	if v, ok := c.raw[claim]; ok { // a literal key containing dots/colons wins (URIs as claim names)
		cur = v
	} else {
		for _, part := range strings.Split(claim, ".") {
			m, ok := cur.(map[string]any)
			if !ok {
				return nil
			}
			if cur, ok = m[part]; !ok {
				return nil
			}
		}
	}
	switch v := cur.(type) {
	case string:
		return strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == ',' })
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			switch s := e.(type) {
			case string:
				out = append(out, s)
			case float64, bool:
				out = append(out, fmt.Sprint(s))
			}
		}
		return out
	case float64, bool:
		return []string{fmt.Sprint(v)}
	}
	return nil
}

// DomainAllowed applies the allowed-domain list (empty = any domain).
func (s Settings) DomainAllowed(email string) bool {
	if len(s.AllowedDomains) == 0 {
		return true
	}
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return false
	}
	return slices.Contains(s.AllowedDomains, strings.ToLower(email[at+1:]))
}

// MapRole returns the role for the claims. matched is true when the role claim
// is configured (the role then comes from the IdP); role "" means access denied.
func (s Settings) MapRole(c Claims) (role string, matched bool) {
	def := s.DefaultRole
	if def == "deny" {
		def = ""
	}
	if s.RoleClaim == "" {
		return def, false
	}
	vals := c.Values(s.RoleClaim)
	has := func(list []string) bool {
		for _, v := range vals {
			if slices.Contains(list, v) {
				return true
			}
		}
		return false
	}
	switch {
	case has(s.AdminValues):
		return "admin", true
	case has(s.OperatorValues):
		return "operator", true
	}
	return def, true
}
