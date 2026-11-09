package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/sso"
	"corvus/control/internal/store"
)

// zaSSOSettings rewrites the stored SSO settings.
func zaSSOSettings(t *testing.T, e *ssoEnv, mutate func(*sso.Settings)) {
	t.Helper()
	ctx := context.Background()
	raw, _ := e.s.Store.GetMeta(ctx, sso.MetaKey)
	c, err := sso.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&c)
	if err := e.s.Store.SetMeta(ctx, sso.MetaKey, string(mustJSON(c))); err != nil {
		t.Fatal(err)
	}
	e.s.ssoRT.invalidate()
}

// zaStart begins a sign-in and returns the IdP's redirect back to the callback.
func zaStart(t *testing.T, e *ssoEnv, c *http.Client) string {
	t.Helper()
	resp := e.get(c, e.console.URL+"/api/auth/oidc/start")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("start: %d", resp.StatusCode)
	}
	return e.idp.authorize(t, resp.Header.Get("Location"))
}

// zaWrapIdP intercepts requests to the mock IdP.
func zaWrapIdP(e *ssoEnv, wrap func(next http.Handler) http.Handler) {
	e.idp.srv.Config.Handler = wrap(e.idp.srv.Config.Handler)
}

func TestZZSSOStartFailures(t *testing.T) {
	e := newSSOEnv(t, nil)
	c := e.browser()

	// too many failures from this address
	lim := e.s.Limiter
	e.s.Limiter = auth.NewLimiter(1, time.Minute)
	e.s.Limiter.Fail("127.0.0.1")
	wantErr(t, e.get(c, e.console.URL+"/api/auth/oidc/start").Header.Get("Location"), "rate_limited")
	resp := e.get(c, e.console.URL+"/api/auth/oidc/callback?state=x&code=y")
	wantErr(t, resp.Header.Get("Location"), "rate_limited")
	e.s.Limiter = lim

	// the stored client secret cannot be decrypted
	zaSSOSettings(t, e, func(s *sso.Settings) { s.ClientSecret = "garbage" })
	wantErr(t, e.get(c, e.console.URL+"/api/auth/oidc/start").Header.Get("Location"), "config")

	// the provider's discovery document cannot be fetched
	zaSSOSettings(t, e, func(s *sso.Settings) { s.Issuer = e.idp.srv.URL + "/missing" })
	wantErr(t, e.get(c, e.console.URL+"/api/auth/oidc/start").Header.Get("Location"), "provider_unavailable")

	// unreadable settings disable SSO (and are not an error for the public endpoint)
	if err := e.s.Store.SetMeta(context.Background(), sso.MetaKey, "{not json"); err != nil {
		t.Fatal(err)
	}
	wantErr(t, e.get(c, e.console.URL+"/api/auth/oidc/start").Header.Get("Location"), "disabled")
	r, _ := http.Get(e.console.URL + "/api/auth/sso")
	var pub map[string]any
	_ = json.NewDecoder(r.Body).Decode(&pub)
	r.Body.Close()
	if r.StatusCode != 200 || pub["enabled"] != false {
		t.Fatalf("public sso with broken settings: %d %v", r.StatusCode, pub)
	}
	if e.audited("auth.sso_failed") != 2 {
		t.Fatalf("auth.sso_failed audited %d times", e.audited("auth.sso_failed"))
	}
}

func TestZZSSOBusyAndFlowStore(t *testing.T) {
	e := newSSOEnv(t, nil)
	// a full table of live flows: new sign-ins are refused
	rt := &e.s.ssoRT
	rt.mu.Lock()
	rt.flows = map[string]*oidcFlow{}
	for i := 0; i < oidcMaxFlows; i++ {
		rt.flows[auth.RandomToken(12)] = &oidcFlow{exp: time.Now().Add(time.Hour)}
	}
	rt.mu.Unlock()
	wantErr(t, e.get(e.browser(), e.console.URL+"/api/auth/oidc/start").Header.Get("Location"), "busy")

	// expired flows are dropped to make room
	var fresh ssoRuntime
	if !fresh.putFlow("first", &oidcFlow{exp: time.Now().Add(-time.Minute)}) {
		t.Fatal("first flow refused")
	}
	for i := 0; i < oidcMaxFlows; i++ {
		fresh.flows[auth.RandomToken(12)] = &oidcFlow{exp: time.Now().Add(-time.Minute)}
	}
	if !fresh.putFlow("new", &oidcFlow{exp: time.Now().Add(time.Minute)}) || len(fresh.flows) != 1 {
		t.Fatalf("expired flows kept: %d", len(fresh.flows))
	}
	if fresh.takeFlow("new") == nil || fresh.takeFlow("new") != nil {
		t.Fatal("flows must be single-use")
	}
}

func TestZZSSOCallbackFailures(t *testing.T) {
	e := newSSOEnv(t, nil)

	// the IdP reports an error
	c := e.browser()
	resp := e.get(c, e.console.URL+"/api/auth/oidc/callback?error=access_denied&error_description="+url.QueryEscape(strings.Repeat("d", 400)))
	wantErr(t, resp.Header.Get("Location"), "idp_error")
	list, _ := e.s.Store.ListAudit(context.Background(), 10)
	if len(list) == 0 || list[0].Action != "auth.sso_failed" || !strings.Contains(string(list[0].Detail), "…") || !strings.Contains(string(list[0].Detail), "access_denied") {
		t.Fatalf("idp error audit (long description trimmed): %+v", list)
	}
	for _, ck := range resp.Cookies() {
		if ck.Name == oidcCookie && ck.MaxAge != -1 {
			t.Fatalf("flow cookie not cleared: %+v", ck)
		}
	}

	// the flow expired
	back := zaStart(t, e, c)
	e.s.ssoRT.mu.Lock()
	for _, f := range e.s.ssoRT.flows {
		f.exp = time.Now().Add(-time.Second)
	}
	e.s.ssoRT.mu.Unlock()
	wantErr(t, e.get(c, back).Header.Get("Location"), "expired")

	// SSO was turned off during the sign-in
	back = zaStart(t, e, c)
	zaSSOSettings(t, e, func(s *sso.Settings) { s.Enabled = false })
	wantErr(t, e.get(c, back).Header.Get("Location"), "disabled")
	zaSSOSettings(t, e, func(s *sso.Settings) { s.Enabled = true })

	// the client changed during the sign-in
	back = zaStart(t, e, c)
	zaSSOSettings(t, e, func(s *sso.Settings) { s.ClientID = "another-client" })
	wantErr(t, e.get(c, back).Header.Get("Location"), "expired")
	zaSSOSettings(t, e, func(s *sso.Settings) { s.ClientID = e.idp.clientID })

	// the secret became unreadable during the sign-in
	back = zaStart(t, e, c)
	raw, _ := e.s.Store.GetMeta(context.Background(), sso.MetaKey)
	good, _ := sso.Parse(raw)
	zaSSOSettings(t, e, func(s *sso.Settings) { s.ClientSecret = "v1:AAAA" })
	wantErr(t, e.get(c, back).Header.Get("Location"), "config")
	zaSSOSettings(t, e, func(s *sso.Settings) { s.ClientSecret = good.ClientSecret })

	// the provider went away during the sign-in
	back = zaStart(t, e, c)
	down := true
	zaWrapIdP(e, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if down && strings.HasSuffix(r.URL.Path, "/openid-configuration") {
				http.Error(w, "down", http.StatusServiceUnavailable)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	e.s.ssoRT.invalidate()
	wantErr(t, e.get(c, back).Header.Get("Location"), "provider_unavailable")
	down = false

	// the IdP refuses the code exchange (wrong client secret)
	back = zaStart(t, e, c)
	secret := e.idp.secret
	e.idp.secret = "rotated"
	wantErr(t, e.get(c, back).Header.Get("Location"), "exchange")
	e.idp.secret = secret

	// no ID token in the token response
	noID := true
	zaWrapIdP(e, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if noID && r.URL.Path == "/token" {
				writeJSON(w, 200, map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 60})
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	wantErr(t, e.login(e.browser()), "token_invalid")
	noID = false

	// no email anywhere (and no userinfo endpoint)
	e.idp.mutate = func(c map[string]any) { delete(c, "email") }
	wantErr(t, e.login(e.browser()), "no_email")
	e.idp.mutate = nil

	if n := e.audited("auth.sso_login"); n != 0 {
		t.Fatalf("%d successful sign-ins", n)
	}
	if n := e.audited("auth.sso_failed"); n != 9 {
		t.Fatalf("auth.sso_failed audited %d times", n)
	}
}

// Email and groups only available from the userinfo endpoint; a name longer than 64
// characters is shortened; a missing name falls back to the email's local part.
func TestZZSSOUserInfoAndNames(t *testing.T) {
	e := newSSOEnv(t, nil)
	zaWrapIdP(e, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/openid-configuration"):
				rec := httptest.NewRecorder()
				next.ServeHTTP(rec, r)
				d := map[string]any{}
				_ = json.Unmarshal(rec.Body.Bytes(), &d)
				d["userinfo_endpoint"] = e.idp.srv.URL + "/userinfo"
				writeJSON(w, 200, d)
			case r.URL.Path == "/userinfo":
				if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer at-") {
					http.Error(w, "no token", http.StatusUnauthorized)
					return
				}
				writeJSON(w, 200, map[string]any{"sub": "sub-dave", "email": "dave@example.com", "email_verified": true, "groups": []string{"corvus-ops"}})
			default:
				next.ServeHTTP(w, r)
			}
		})
	})
	e.idp.mu.Lock()
	e.idp.claims = map[string]any{"sub": "sub-dave", "name": strings.Repeat("Ð", 70)}
	e.idp.mu.Unlock()
	c := e.browser()
	if loc := e.login(c); loc != "/" {
		t.Fatalf("userinfo sign-in: %q", loc)
	}
	u := e.me(c)
	if u == nil || u.Email != "dave@example.com" || u.Role != "operator" || len([]rune(u.Name)) != 64 {
		t.Fatalf("me = %+v", u)
	}

	// a userinfo answer for another subject is ignored
	e.idp.mu.Lock()
	e.idp.claims = map[string]any{"sub": "sub-someone-else"}
	e.idp.mu.Unlock()
	wantErr(t, e.login(e.browser()), "no_email")

	// no name claim: the local part of the email
	e.identity("erin@example.com", "corvus-ops")
	e.idp.mutate = func(c map[string]any) { delete(c, "name") }
	c = e.browser()
	e.login(c)
	if u := e.me(c); u == nil || u.Name != "erin" {
		t.Fatalf("me = %+v", u)
	}
}

func TestZZSSOSettingsEdits(t *testing.T) {
	e := newSSOEnv(t, nil)
	ctx := context.Background()
	root, _ := e.s.Store.UserByEmail(ctx, "root@example.com")
	tok := auth.RandomToken(32)
	if err := e.s.Store.CreateSession(ctx, auth.TokenHash(tok), root.ID, time.Hour, "127.0.0.1", "test"); err != nil {
		t.Fatal(err)
	}
	call := func(method, path string, body any, raw ...string) (int, map[string]any) {
		var rd io.Reader = bytes.NewReader(mustJSON(body))
		if body == nil {
			rd = http.NoBody
		}
		if len(raw) > 0 {
			rd = strings.NewReader(raw[0])
		}
		req, _ := http.NewRequest(method, e.console.URL+path, rd)
		req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: tok})
		req.Header.Set("X-Corvus-Request", "1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out := map[string]any{}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	stored := func() sso.Settings {
		raw, _ := e.s.Store.GetMeta(ctx, sso.MetaKey)
		c, _ := sso.Parse(raw)
		return c
	}
	base := func() map[string]any {
		return map[string]any{"enabled": false, "issuer": e.idp.srv.URL, "client_id": e.idp.clientID, "default_role": "viewer",
			"client_secret_set": true, "redirect_uri": "ignored"}
	}

	if code, _ := call("PUT", "/api/settings/sso", nil, "{"); code != 400 {
		t.Fatalf("malformed PUT: %d", code)
	}
	if code, _ := call("POST", "/api/settings/sso/test", nil, "{"); code != 400 {
		t.Fatalf("malformed test: %d", code)
	}
	if code, m := call("POST", "/api/settings/sso/test", map[string]string{"issuer": e.idp.srv.URL + "/nope"}); code != 422 || zaFields(m)["issuer"] != "discovery" {
		t.Fatalf("test of a bad issuer: %d %v", code, m)
	}

	// a new secret is sealed; sync_role can be turned off
	in := base()
	in["client_secret"] = "  new-secret  "
	in["sync_role"] = false
	if code, m := call("PUT", "/api/settings/sso", in); code != 200 || m["client_secret_set"] != true || m["sync_role"] != false {
		t.Fatalf("new secret: %d %v", code, m)
	}
	c := stored()
	if !strings.HasPrefix(c.ClientSecret, "v1:") || strings.Contains(c.ClientSecret, "new-secret") || c.SyncRole {
		t.Fatalf("stored: %+v", c)
	}
	// too long
	in["client_secret"] = strings.Repeat("s", 1025)
	if code, m := call("PUT", "/api/settings/sso", in); code != 422 || zaFields(m)["client_secret"] != "invalid" {
		t.Fatalf("long secret: %d %v", code, m)
	}
	// sync_role is kept when not sent
	in = base()
	if code, m := call("PUT", "/api/settings/sso", in); code != 200 || m["sync_role"] != false || m["client_secret_set"] != true {
		t.Fatalf("keep: %d %v", code, m)
	}
	// clear the secret (public client)
	in["clear_client_secret"] = true
	if code, m := call("PUT", "/api/settings/sso", in); code != 200 || m["client_secret_set"] != false {
		t.Fatalf("clear: %d %v", code, m)
	}
	if stored().ClientSecret != "" {
		t.Fatal("secret not cleared")
	}
	if n := e.audited("sso.update"); n != 3 {
		t.Fatalf("sso.update audited %d times", n)
	}
	list, _ := e.s.Store.ListAudit(ctx, 1)
	if !strings.Contains(string(list[0].Detail), `"client_secret_changed":true`) {
		t.Fatalf("clearing not recorded as a secret change: %s", list[0].Detail)
	}
	// invalid settings
	in = base()
	in["default_role"] = "superuser"
	if code, _ := call("PUT", "/api/settings/sso", in); code != 422 {
		t.Fatalf("invalid default role: %d", code)
	}

	// behind TLS the callback URL is https
	e.s.SecureCookie = true
	if code, m := call("GET", "/api/settings/sso", nil); code != 200 || !strings.HasPrefix(m["redirect_uri"].(string), "https://") {
		t.Fatalf("redirect_uri with secure cookies: %d %v", code, m)
	}
}

// Without a readable meta table the SSO endpoints answer 500.
func TestZZSSOSettingsDatabaseError(t *testing.T) {
	e := newFullEnv(t)
	path := filepath.Join(t.TempDir(), "sso.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	u, err := st.CreateUser(context.Background(), "root@example.com", "Root", "x", "admin")
	if err != nil {
		t.Fatal(err)
	}
	raw := auth.RandomToken(32)
	_ = st.CreateSession(context.Background(), auth.TokenHash(raw), u.ID, time.Hour, "", "")
	e.s.Store, e.cookie = st, raw
	e.h = e.s.Handler()
	zaExec(t, path, "DROP TABLE meta")
	for _, r := range []struct{ method, path string }{{"GET", "/api/auth/sso"}, {"GET", "/api/settings/sso"}, {"PUT", "/api/settings/sso"}} {
		if code, m := e.call(r.method, r.path, map[string]any{"enabled": false}); code != 500 || m["error"] != "internal" {
			t.Fatalf("%s %s: %d %v", r.method, r.path, code, m)
		}
	}
	w, _ := e.do("GET", "/api/auth/oidc/start", nil)
	if w.Code != 302 || w.Header().Get("Location") != "/login?sso_error=disabled" {
		t.Fatalf("start: %d %v", w.Code, w.Header())
	}
}

func TestZZTrimMsg(t *testing.T) {
	if trimMsg("short") != "short" {
		t.Fatal("short message changed")
	}
	if got := trimMsg(strings.Repeat("a", 301)); len(got) != 300+len("…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("long message: %d", len(got))
	}
}
