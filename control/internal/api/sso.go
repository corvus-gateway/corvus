package api

// Single sign-on for the admin console via OpenID Connect (authorization code
// flow with PKCE S256, state and nonce).
//
// Decisions:
//   - SSO sign-ins skip the local TOTP step: the identity provider is responsible
//     for multi-factor authentication.
//   - When a role claim is configured and sync_role is on (default), the user's
//     role follows the IdP mapping on every sign-in (upgrades and downgrades), except
//     that the last active administrator is never demoted (that would lock everyone
//     out of user management). With default_role "deny", users without a matching
//     group cannot sign in via SSO at all. Without a role claim, existing users keep
//     their role and new users get the default role.
//   - Users are bound to the IdP account (issuer + subject), not to the email: an
//     existing user is linked on a first SSO sign-in only when the IdP marks the
//     email as verified (see ssoUser). Administrators can remove a link.

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"corvus/control/internal/auth"
	"corvus/control/internal/seal"
	"corvus/control/internal/sso"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

const (
	oidcCookie     = "corvus_oidc"
	oidcCookiePath = "/api/auth/oidc/"
	oidcFlowTTL    = 10 * time.Minute
	oidcMaxFlows   = 5000
	// pending flows per client address (several tabs or retries; more evicts the oldest)
	oidcMaxFlowsPerIP = 20
	providerTTL       = time.Hour
	callbackPath      = "/api/auth/oidc/callback"
)

// ssoRuntime is the in-memory part of SSO: pending sign-in flows and the cached
// provider (discovery + JWKS). The zero value is ready to use.
type ssoRuntime struct {
	mu    sync.Mutex
	flows map[string]*oidcFlow // by state
	prov  *cachedProvider
}

type oidcFlow struct {
	nonce, verifier, redirectURI string
	issuer, clientID             string
	bindHash                     string // hash of the browser-binding cookie
	ip                           string // client address that started the flow
	exp                          time.Time
}

type cachedProvider struct {
	issuer string
	p      *oidc.Provider
	client *http.Client
	at     time.Time
}

func (rt *ssoRuntime) putFlow(state string, f *oidcFlow) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.flows == nil {
		rt.flows = map[string]*oidcFlow{}
	}
	now := time.Now()
	if len(rt.flows) >= oidcMaxFlows/2 {
		for k, v := range rt.flows {
			if now.After(v.exp) {
				delete(rt.flows, k)
			}
		}
	}
	if f.ip != "" {
		// one client address holds at most oidcMaxFlowsPerIP pending flows: its oldest
		// one makes room, so a single address cannot fill the table
		var oldest string
		n := 0
		for k, v := range rt.flows {
			if v.ip != f.ip {
				continue
			}
			n++
			if oldest == "" || v.exp.Before(rt.flows[oldest].exp) {
				oldest = k
			}
		}
		if n >= oidcMaxFlowsPerIP {
			delete(rt.flows, oldest)
		}
	}
	if len(rt.flows) >= oidcMaxFlows {
		return false
	}
	rt.flows[state] = f
	return true
}

// flowClientKey groups clients for the per-address flow limit: IPv6 clients by /64
// (one host usually holds a whole /64).
func flowClientKey(ip string) string {
	if a := net.ParseIP(ip); a != nil && a.To4() == nil {
		return a.Mask(net.CIDRMask(64, 128)).String() + "/64"
	}
	return ip
}

// takeFlow removes and returns the flow (one use only).
func (rt *ssoRuntime) takeFlow(state string) *oidcFlow {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	f := rt.flows[state]
	delete(rt.flows, state)
	return f
}

func (rt *ssoRuntime) invalidate() {
	rt.mu.Lock()
	rt.prov = nil
	rt.mu.Unlock()
}

func (s *Server) ssoProvider(ctx context.Context, issuer string) (*oidc.Provider, *http.Client, error) {
	rt := &s.ssoRT
	rt.mu.Lock()
	if c := rt.prov; c != nil && c.issuer == issuer && time.Since(c.at) < providerTTL {
		rt.mu.Unlock()
		return c.p, c.client, nil
	}
	rt.mu.Unlock()
	client := sso.NewHTTPClient(issuer)
	p, _, err := sso.Discover(ctx, client, issuer)
	if err != nil {
		return nil, nil, err
	}
	rt.mu.Lock()
	rt.prov = &cachedProvider{issuer: issuer, p: p, client: client, at: time.Now()}
	rt.mu.Unlock()
	return p, client, nil
}

func (s *Server) loadSSO(ctx context.Context) (sso.Settings, error) {
	raw, err := s.Store.GetMeta(ctx, sso.MetaKey)
	if err != nil {
		return sso.Defaults(), err
	}
	cfg, err := sso.Parse(raw)
	if err != nil {
		slog.Error("sso settings are unreadable; SSO disabled", "err", err)
	}
	return cfg, nil
}

// ssoRedirectURI is the callback URL to register at the IdP.
func (s *Server) ssoRedirectURI(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || s.SecureCookie {
		scheme = "https"
	}
	return scheme + "://" + r.Host + callbackPath
}

// ------------------------------------------------------------------ settings API

type ssoView struct {
	Enabled         bool     `json:"enabled"`
	DisplayName     string   `json:"display_name"`
	Issuer          string   `json:"issuer"`
	ClientID        string   `json:"client_id"`
	ClientSecretSet bool     `json:"client_secret_set"`
	Scopes          []string `json:"scopes"`
	AllowedDomains  []string `json:"allowed_domains"`
	RoleClaim       string   `json:"role_claim"`
	AdminValues     []string `json:"admin_values"`
	OperatorValues  []string `json:"operator_values"`
	DefaultRole     string   `json:"default_role"`
	AutoProvision   bool     `json:"auto_provision"`
	SyncRole        bool     `json:"sync_role"`
	TrustEmail      bool     `json:"trust_email"`
	RedirectURI     string   `json:"redirect_uri"`
}

func (s *Server) ssoViewOf(r *http.Request, c sso.Settings) ssoView {
	return ssoView{Enabled: c.Enabled, DisplayName: c.Label(), Issuer: c.Issuer, ClientID: c.ClientID, ClientSecretSet: c.ClientSecret != "",
		Scopes: c.Scopes, AllowedDomains: c.AllowedDomains, RoleClaim: c.RoleClaim, AdminValues: c.AdminValues, OperatorValues: c.OperatorValues,
		DefaultRole: c.DefaultRole, AutoProvision: c.AutoProvision, SyncRole: c.SyncRole, TrustEmail: c.TrustEmail, RedirectURI: s.ssoRedirectURI(r)}
}

// publicSSO tells the sign-in page whether to show the SSO button.
func (s *Server) publicSSO(w http.ResponseWriter, r *http.Request) {
	c, err := s.loadSSO(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if !c.Ready() {
		writeJSON(w, 200, map[string]any{"enabled": false, "display_name": ""})
		return
	}
	writeJSON(w, 200, map[string]any{"enabled": true, "display_name": c.Label()})
}

func (s *Server) getSSOSettings(w http.ResponseWriter, r *http.Request) {
	c, err := s.loadSSO(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, s.ssoViewOf(r, c))
}

type ssoInput struct {
	Enabled           bool     `json:"enabled"`
	DisplayName       string   `json:"display_name"`
	Issuer            string   `json:"issuer"`
	ClientID          string   `json:"client_id"`
	ClientSecret      string   `json:"client_secret"`       // empty = keep
	ClearClientSecret bool     `json:"clear_client_secret"` // public client (PKCE only)
	Scopes            []string `json:"scopes"`
	AllowedDomains    []string `json:"allowed_domains"`
	RoleClaim         string   `json:"role_claim"`
	AdminValues       []string `json:"admin_values"`
	OperatorValues    []string `json:"operator_values"`
	DefaultRole       string   `json:"default_role"`
	AutoProvision     bool     `json:"auto_provision"`
	SyncRole          *bool    `json:"sync_role"`
	TrustEmail        *bool    `json:"trust_email"` // absent = keep (reset when the issuer changes)
	// read-only fields echoed back by clients are ignored
	ClientSecretSet *bool   `json:"client_secret_set,omitempty"`
	RedirectURI     *string `json:"redirect_uri,omitempty"`
}

func (s *Server) putSSOSettings(w http.ResponseWriter, r *http.Request) {
	var in ssoInput
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	prev, err := s.loadSSO(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	c := sso.Settings{Enabled: in.Enabled, DisplayName: in.DisplayName, Issuer: in.Issuer, ClientID: in.ClientID, ClientSecret: prev.ClientSecret,
		Scopes: in.Scopes, AllowedDomains: in.AllowedDomains, RoleClaim: in.RoleClaim, AdminValues: in.AdminValues, OperatorValues: in.OperatorValues,
		DefaultRole: in.DefaultRole, AutoProvision: in.AutoProvision, SyncRole: prev.SyncRole || prev.Issuer == ""}
	if strings.TrimRight(in.Issuer, "/") != strings.TrimRight(prev.Issuer, "/") {
		c.ClientSecret = "" // never send the old provider's secret to a new issuer
	} else {
		c.TrustEmail = prev.TrustEmail // a statement about this provider only
	}
	if in.TrustEmail != nil {
		c.TrustEmail = *in.TrustEmail
	}
	if in.SyncRole != nil {
		c.SyncRole = *in.SyncRole
	}
	if err := c.Normalize(); err != nil {
		fail(w, err)
		return
	}
	secretChanged := false
	switch {
	case strings.TrimSpace(in.ClientSecret) != "":
		if len(in.ClientSecret) > 1024 {
			fail(w, validate.Errors{{Field: "client_secret", Code: "invalid", Message: "client secret is too long"}})
			return
		}
		sealed, err := seal.Seal(s.DataKey, strings.TrimSpace(in.ClientSecret))
		if err != nil {
			fail(w, err)
			return
		}
		c.ClientSecret, secretChanged = sealed, true
	case in.ClearClientSecret:
		c.ClientSecret, secretChanged = "", prev.ClientSecret != ""
	}
	if c.Enabled {
		if _, _, err := sso.Discover(ctx, nil, c.Issuer); err != nil {
			fail(w, validate.Errors{{Field: "issuer", Code: "discovery", Message: trimMsg(err.Error())}})
			return
		}
	}
	if err := s.Store.SetMeta(ctx, sso.MetaKey, string(mustJSON(c))); err != nil {
		fail(w, err)
		return
	}
	s.ssoRT.invalidate()
	s.audit(r, "sso.update", c.Issuer, map[string]any{"enabled": c.Enabled, "client_id": c.ClientID, "client_secret_changed": secretChanged,
		"role_claim": c.RoleClaim, "default_role": c.DefaultRole, "auto_provision": c.AutoProvision, "sync_role": c.SyncRole, "trust_email": c.TrustEmail, "allowed_domains": c.AllowedDomains})
	writeJSON(w, 200, s.ssoViewOf(r, c))
}

// testSSODiscovery fetches the discovery document of an issuer without saving.
func (s *Server) testSSODiscovery(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Issuer string `json:"issuer"`
	}
	if !decode(w, r, &in) {
		return
	}
	_, d, err := sso.Discover(r.Context(), nil, strings.TrimSpace(in.Issuer))
	if err != nil {
		fail(w, validate.Errors{{Field: "issuer", Code: "discovery", Message: trimMsg(err.Error())}})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "discovery": d})
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func trimMsg(s string) string {
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

// ------------------------------------------------------------------ sign-in flow

func (s *Server) ssoOAuthConfig(c sso.Settings, p *oidc.Provider, redirectURI string) (*oauth2.Config, error) {
	secret, err := seal.Open(s.DataKey, c.ClientSecret)
	if err != nil {
		return nil, err
	}
	return &oauth2.Config{ClientID: c.ClientID, ClientSecret: secret, Endpoint: p.Endpoint(), RedirectURL: redirectURI, Scopes: c.Scopes}, nil
}

func (s *Server) ssoFail(w http.ResponseWriter, r *http.Request, code, email string, detail map[string]any) {
	if detail == nil {
		detail = map[string]any{}
	}
	detail["reason"] = code
	s.audit(r, "auth.sso_failed", email, detail)
	s.clearOIDCCookie(w)
	http.Redirect(w, r, "/login?sso_error="+url.QueryEscape(code), http.StatusFound)
}

func (s *Server) clearOIDCCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Value: "", Path: oidcCookiePath, HttpOnly: true, Secure: s.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

// oidcStart redirects the browser to the identity provider.
func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if blocked, _ := s.Limiter.Blocked(clientIP(r)); blocked {
		http.Redirect(w, r, "/login?sso_error=rate_limited", http.StatusFound)
		return
	}
	c, err := s.loadSSO(ctx)
	if err != nil || !c.Ready() {
		http.Redirect(w, r, "/login?sso_error=disabled", http.StatusFound)
		return
	}
	p, _, err := s.ssoProvider(ctx, c.Issuer)
	if err != nil {
		s.ssoFail(w, r, "provider_unavailable", "", map[string]any{"error": trimMsg(err.Error())})
		return
	}
	redirectURI := s.ssoRedirectURI(r)
	oc, err := s.ssoOAuthConfig(c, p, redirectURI)
	if err != nil {
		s.ssoFail(w, r, "config", "", map[string]any{"error": "client secret cannot be decrypted"})
		return
	}
	state, nonce, bind, verifier := auth.RandomToken(24), auth.RandomToken(24), auth.RandomToken(32), oauth2.GenerateVerifier()
	if !s.ssoRT.putFlow(state, &oidcFlow{nonce: nonce, verifier: verifier, redirectURI: redirectURI, issuer: c.Issuer, clientID: c.ClientID,
		bindHash: auth.TokenHash(bind), ip: flowClientKey(clientIP(r)), exp: time.Now().Add(oidcFlowTTL)}) {
		http.Redirect(w, r, "/login?sso_error=busy", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Value: bind, Path: oidcCookiePath, HttpOnly: true, Secure: s.SecureCookie,
		SameSite: http.SameSiteLaxMode, MaxAge: int(oidcFlowTTL.Seconds())})
	http.Redirect(w, r, oc.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

// oidcCallback completes the sign-in and issues the normal console session.
func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	ip := clientIP(r)
	if blocked, _ := s.Limiter.Blocked(ip); blocked {
		s.clearOIDCCookie(w)
		http.Redirect(w, r, "/login?sso_error=rate_limited", http.StatusFound)
		return
	}
	if e := q.Get("error"); e != "" {
		s.ssoFail(w, r, "idp_error", "", map[string]any{"error": trimMsg(e), "error_description": trimMsg(q.Get("error_description"))})
		return
	}
	f := s.ssoRT.takeFlow(q.Get("state"))
	ck, cerr := r.Cookie(oidcCookie)
	if f == nil || cerr != nil || subtle.ConstantTimeCompare([]byte(auth.TokenHash(ck.Value)), []byte(f.bindHash)) != 1 {
		s.Limiter.Fail(ip) // forged or replayed callbacks count like failed sign-ins
		s.ssoFail(w, r, "state", "", nil)
		return
	}
	if time.Now().After(f.exp) {
		s.ssoFail(w, r, "expired", "", nil)
		return
	}
	c, err := s.loadSSO(ctx)
	if err != nil || !c.Ready() {
		s.ssoFail(w, r, "disabled", "", nil)
		return
	}
	if c.Issuer != f.issuer || c.ClientID != f.clientID {
		s.ssoFail(w, r, "expired", "", map[string]any{"error": "SSO settings changed during sign-in"})
		return
	}
	p, client, err := s.ssoProvider(ctx, c.Issuer)
	if err != nil {
		s.ssoFail(w, r, "provider_unavailable", "", map[string]any{"error": trimMsg(err.Error())})
		return
	}
	oc, err := s.ssoOAuthConfig(c, p, f.redirectURI)
	if err != nil {
		s.ssoFail(w, r, "config", "", map[string]any{"error": "client secret cannot be decrypted"})
		return
	}
	cctx := sso.ClientContext(ctx, client)
	tok, err := oc.Exchange(cctx, q.Get("code"), oauth2.VerifierOption(f.verifier))
	if err != nil {
		s.ssoFail(w, r, "exchange", "", map[string]any{"error": trimMsg(err.Error())})
		return
	}
	rawID, _ := tok.Extra("id_token").(string)
	if rawID == "" {
		s.ssoFail(w, r, "token_invalid", "", map[string]any{"error": "no id_token in the token response"})
		return
	}
	idt, err := p.Verifier(&oidc.Config{ClientID: c.ClientID}).Verify(cctx, rawID) // signature (JWKS), iss, aud, exp
	if err != nil {
		s.ssoFail(w, r, "token_invalid", "", map[string]any{"error": trimMsg(err.Error())})
		return
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(f.nonce)) != 1 {
		s.ssoFail(w, r, "token_invalid", "", map[string]any{"error": "nonce mismatch"})
		return
	}
	raw := map[string]any{}
	if err := idt.Claims(&raw); err != nil {
		s.ssoFail(w, r, "token_invalid", "", map[string]any{"error": "unreadable claims"})
		return
	}
	cl := sso.ParseClaims(raw)
	// Email, email_verified or the role claim may only be available from userinfo.
	if (cl.Email == "" || cl.EmailVerified == nil || (c.RoleClaim != "" && cl.Values(c.RoleClaim) == nil)) && p.UserInfoEndpoint() != "" {
		if ui, err := p.UserInfo(cctx, oauth2.StaticTokenSource(tok)); err == nil && ui.Subject == idt.Subject {
			m := map[string]any{}
			if ui.Claims(&m) == nil {
				cl.Merge(m)
			}
		}
	}
	detail := map[string]any{"issuer": c.Issuer, "sub": idt.Subject}
	if idt.Subject == "" || len(idt.Subject) > store.MaxSubject {
		s.ssoFail(w, r, "token_invalid", "", map[string]any{"error": "missing or oversized subject"})
		return
	}
	if cl.Email == "" || !strings.Contains(cl.Email, "@") {
		s.ssoFail(w, r, "no_email", "", detail)
		return
	}
	if cl.EmailVerified != nil && !*cl.EmailVerified {
		s.ssoFail(w, r, "email_unverified", cl.Email, detail)
		return
	}
	if !c.DomainAllowed(cl.Email) {
		s.ssoFail(w, r, "domain", cl.Email, detail)
		return
	}
	role, mapped := c.MapRole(cl)
	if role == "" {
		s.ssoFail(w, r, "no_role", cl.Email, detail)
		return
	}
	u, provisioned, code := s.ssoUser(ctx, c, cl, idt.Subject, role, detail)
	if code != "" {
		s.ssoFail(w, r, code, cl.Email, detail)
		return
	}
	if u.Disabled {
		s.ssoFail(w, r, "user_disabled", u.Email, detail)
		return
	}
	if !provisioned && mapped && c.SyncRole && u.Role != role {
		keep := false
		if u.Role == "admin" {
			if n, err := s.Store.CountActiveAdmins(ctx, u.ID); err != nil || n == 0 {
				keep = true // never demote the last active administrator
				detail["role_sync_skipped"] = "last_admin"
			}
		}
		if !keep {
			err := s.Store.KeepingAdmin(ctx, func(tx *store.Store) error {
				return tx.UpdateUser(ctx, u.ID, u.Name, role, false)
			})
			switch {
			case errors.Is(err, store.ErrLastAdmin):
				detail["role_sync_skipped"] = "last_admin" // another change got there first
			case err != nil:
				fail(w, err)
				return
			default:
				_ = s.Store.DeleteUserSessions(ctx, u.ID) // the old role's sessions end, as on an admin role change
				detail["role_changed"] = map[string]string{"before": u.Role, "after": role}
				u.Role = role
			}
		}
	}
	if err := s.setSession(w, r, u); err != nil {
		fail(w, err)
		return
	}
	_ = s.Store.TouchLogin(ctx, u.ID)
	s.clearOIDCCookie(w)
	r = r.WithContext(context.WithValue(ctx, ctxKey{}, u))
	detail["role"] = u.Role
	detail["provisioned"] = provisioned
	s.audit(r, "auth.sso_login", u.Email, detail)
	http.Redirect(w, r, "/", http.StatusFound)
}

// ssoUser finds (or creates) the console user for a verified ID token. Users are
// matched by the identity provider account (issuer + subject) they are linked to:
//   - linked account → that user, whatever email the IdP reports now;
//   - otherwise a user with the same email is linked on this sign-in, but only when
//     the IdP states the email is verified (email_verified true, or no claim with
//     "trust_email" set for IdPs that never send it), and only if that
//     user is not linked to another account at the same issuer;
//   - otherwise, with auto-provisioning, a new user is created and linked.
//
// On failure it returns the sso_error code.
func (s *Server) ssoUser(ctx context.Context, c sso.Settings, cl sso.Claims, sub, role string, detail map[string]any) (u *store.User, provisioned bool, code string) {
	u, err := s.Store.UserByIdentity(ctx, c.Issuer, sub)
	if err == nil {
		return u, false, ""
	}
	if !errors.Is(err, store.ErrNotFound) {
		slog.Error("sso user lookup failed", "err", err)
		return nil, false, "internal"
	}
	u, err = s.Store.UserByEmail(ctx, cl.Email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		if !c.AutoProvision {
			return nil, false, "not_provisioned"
		}
		u, err = s.provisionSSOUser(ctx, c.Issuer, sub, cl, role)
		if errors.Is(err, errSSOLinkConflict) {
			return nil, false, "link_conflict"
		}
		if err != nil {
			slog.Error("sso provisioning failed", "err", err)
			return nil, false, "internal"
		}
		return u, true, ""
	case err != nil:
		slog.Error("sso user lookup failed", "err", err)
		return nil, false, "internal"
	}
	// An existing user who has not signed in with this IdP account before.
	if _, err := s.Store.IdentitySubject(ctx, u.ID, c.Issuer); err == nil {
		return nil, false, "link_conflict" // linked to a different account at this issuer
	} else if !errors.Is(err, store.ErrNotFound) {
		slog.Error("sso link lookup failed", "err", err)
		return nil, false, "internal"
	}
	if !c.EmailVerified(cl) {
		return nil, false, "link_unverified"
	}
	if err := s.Store.LinkIdentity(ctx, u.ID, c.Issuer, sub); err != nil {
		if store.IsUnique(err) { // a concurrent sign-in linked the user or the account
			return nil, false, "link_conflict"
		}
		slog.Error("sso link failed", "err", err)
		return nil, false, "internal"
	}
	detail["linked"] = true
	return u, false, ""
}

var errSSOLinkConflict = errors.New("sso: email already used by another identity")

// provisionSSOUser creates a console user for a first SSO sign-in and links it to
// the IdP account. The account gets a random (unknown) password, so it can only sign
// in via SSO unless an administrator sets a password.
func (s *Server) provisionSSOUser(ctx context.Context, issuer, sub string, cl sso.Claims, role string) (*store.User, error) {
	name := cl.Name
	if name == "" {
		name = strings.SplitN(cl.Email, "@", 2)[0]
	}
	if r := []rune(name); len(r) > 64 {
		name = string(r[:64])
	}
	hash, err := auth.HashPassword(auth.RandomToken(32))
	if err != nil {
		return nil, err
	}
	var u *store.User
	err = s.Store.WithTx(ctx, func(tx *store.Store) error {
		var err error
		if u, err = tx.CreateUser(ctx, cl.Email, name, hash, role); err != nil {
			return err
		}
		return tx.LinkIdentity(ctx, u.ID, issuer, sub)
	})
	if err != nil && store.IsUnique(err) {
		// concurrent first sign-in: the other request created and linked the user
		if u, lerr := s.Store.UserByIdentity(ctx, issuer, sub); lerr == nil {
			return u, nil
		}
		return nil, errSSOLinkConflict
	}
	return u, err
}
