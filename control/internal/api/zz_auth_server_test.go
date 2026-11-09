package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
)

// newSetupEnv is a server without any user (first start) and the given setup token.
func newSetupEnv(t *testing.T, token string, maxFails int) (*testEnv, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "setup.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	tok, err := auth.PresetSetupToken(token)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Traffic: traffic.New(), Version: "9.9.9-test", Setup: tok,
		Limiter: auth.NewLimiter(maxFails, time.Minute), AcctLimiter: auth.NewLimiter(100, time.Minute), MFA: auth.NewMFAChallenges()}
	return &testEnv{t: t, s: s, h: s.Handler()}, path
}

func TestZZSetupFlow(t *testing.T) {
	const tok = "setup-token-0123456789"
	e, _ := newSetupEnv(t, tok, 2)
	ctx := context.Background()

	w, m := e.do("GET", "/api/status", nil)
	if w.Code != 200 || m["setup_required"] != true || m["version"] != "9.9.9-test" {
		t.Fatalf("status before setup: %d %v", w.Code, m)
	}
	// malformed body
	if w, m := e.do("POST", "/api/setup", nil, withCSRF(), zaRawBody("{")); w.Code != 400 || m["error"] != "bad_request" {
		t.Fatalf("malformed setup: %d %v", w.Code, m)
	}
	// unknown fields are rejected too
	if w, _ := e.do("POST", "/api/setup", map[string]any{"token": tok, "role": "admin"}, withCSRF()); w.Code != 400 {
		t.Fatalf("unknown field: %d", w.Code)
	}
	// invalid email and weak password: both reported, the token is not consumed
	w, m = e.do("POST", "/api/setup", map[string]string{"token": tok, "email": "nobody", "password": "short"}, withCSRF())
	if w.Code != 422 || m["error"] != "validation" {
		t.Fatalf("invalid setup: %d %v", w.Code, m)
	}
	fields := map[string]bool{}
	for _, f := range m["fields"].([]any) {
		fields[f.(map[string]any)["field"].(string)] = true
	}
	if !fields["email"] || !fields["password"] {
		t.Fatalf("fields: %v", m["fields"])
	}
	if e.s.Setup.Value() != tok {
		t.Fatal("setup token consumed by an invalid request")
	}

	// without the right token the body is not looked at (no password hashing): 403
	if w, m := e.do("POST", "/api/setup", map[string]string{"token": "nope", "email": "nobody", "password": "short"}, withCSRF()); w.Code != 403 || m["error"] != "invalid_token" {
		t.Fatalf("wrong token, invalid body: %d %v", w.Code, m)
	}
	e.s.Limiter = auth.NewLimiter(2, time.Minute)
	e.h = e.s.Handler()

	// wrong token: 403, counted against the address; the limit then answers 429
	good := map[string]string{"token": tok, "email": "  Boss@Example.com ", "password": zaPassword}
	bad := map[string]string{"token": "wrong-token-value", "email": "boss@example.com", "password": zaPassword}
	for i := 0; i < 2; i++ {
		if w, m := e.do("POST", "/api/setup", bad, withCSRF()); w.Code != 403 || m["error"] != "invalid_token" {
			t.Fatalf("wrong token %d: %d %v", i, w.Code, m)
		}
	}
	w, m = e.do("POST", "/api/setup", good, withCSRF())
	if w.Code != 429 || m["error"] != "rate_limited" || w.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limited setup: %d %v %v", w.Code, m, w.Header())
	}
	// another address still works; the token may be surrounded by spaces
	good["token"] = "  " + tok + " "
	w, m = e.do("POST", "/api/setup", good, withCSRF(), zaRemote("198.51.100.7:4000"))
	if w.Code != 201 || m["email"] != "boss@example.com" || m["role"] != "admin" || m["name"] != "Boss" {
		t.Fatalf("setup: %d %v", w.Code, m)
	}
	c := zaCookie(w, auth.SessionCookie)
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.MaxAge != int(auth.SessionTTL.Seconds()) {
		t.Fatalf("session cookie: %+v", c)
	}
	if w, me := e.do("GET", "/api/auth/me", nil, withCookie(c.Value)); w.Code != 200 || me["email"] != "boss@example.com" {
		t.Fatalf("me after setup: %d %v", w.Code, me)
	}
	if e.s.Setup.Value() != "" {
		t.Fatal("setup token still valid after use")
	}
	if zaAudited(t, e.s, "setup.admin_created") != 1 {
		t.Fatal("setup not audited")
	}
	u, err := e.s.Store.UserByEmail(ctx, "boss@example.com")
	if err != nil || !auth.CheckPassword(u.PasswordHash(), zaPassword) {
		t.Fatalf("stored admin: %v", err)
	}

	// once an administrator exists, setup is closed
	w, m = e.do("POST", "/api/setup", good, withCSRF(), zaRemote("198.51.100.8:4000"))
	if w.Code != 409 || m["error"] != "already_setup" {
		t.Fatalf("second setup: %d %v", w.Code, m)
	}
	if w, m := e.do("GET", "/api/status", nil); w.Code != 200 || m["setup_required"] != false {
		t.Fatalf("status after setup: %d %v", w.Code, m)
	}
}

// A broken database answers 500 (not "setup required" or a crash).
func TestZZStatusAndSetupDatabaseError(t *testing.T) {
	e, path := newSetupEnv(t, "setup-token-0123456789", 5)
	zaExec(t, path, "DROP TABLE users")
	if w, m := e.do("GET", "/api/status", nil); w.Code != 500 || m["error"] != "internal" {
		t.Fatalf("status: %d %v", w.Code, m)
	}
	w, m := e.do("POST", "/api/setup", map[string]string{"token": "setup-token-0123456789", "email": "a@example.com", "password": zaPassword}, withCSRF())
	if w.Code != 500 || m["error"] != "internal" {
		t.Fatalf("setup: %d %v", w.Code, m)
	}
	if e.s.Setup.Value() == "" {
		t.Fatal("token consumed although nothing was created")
	}
}

func TestZZLoginLogoutAndRateLimit(t *testing.T) {
	e, _ := newAuthEnv(t)
	e.s.Limiter = auth.NewLimiter(2, time.Minute)
	e.h = e.s.Handler()
	ctx := context.Background()
	if _, err := e.s.Store.CreateUser(ctx, "gone@example.com", "Gone", zaHash(t), "viewer"); err != nil {
		t.Fatal(err)
	}
	gone, _ := e.s.Store.UserByEmail(ctx, "gone@example.com")
	if err := e.s.Store.UpdateUser(ctx, gone.ID, gone.Name, gone.Role, true); err != nil {
		t.Fatal(err)
	}

	if w, m := e.do("POST", "/api/auth/login", nil, withCSRF(), zaRawBody("not json")); w.Code != 400 || m["error"] != "bad_request" {
		t.Fatalf("malformed login: %d %v", w.Code, m)
	}
	// the right password of a disabled account is refused like a wrong one
	w, m := e.do("POST", "/api/auth/login", map[string]string{"email": "gone@example.com", "password": zaPassword}, withCSRF())
	if w.Code != 401 || m["error"] != "invalid_credentials" || zaCookie(w, auth.SessionCookie) != nil {
		t.Fatalf("disabled login: %d %v", w.Code, m)
	}
	// successful sign-in: session + known-device cookie, last login recorded
	w, m = e.do("POST", "/api/auth/login", map[string]string{"email": " Admin@Example.com ", "password": zaPassword}, withCSRF(), zaRemote("203.0.113.5:1"))
	if w.Code != 200 || m["email"] != "admin@example.com" {
		t.Fatalf("login: %d %v", w.Code, m)
	}
	sess := zaCookie(w, auth.SessionCookie)
	if sess == nil || zaCookie(w, deviceCookie) == nil {
		t.Fatalf("cookies: %v", w.Result().Cookies())
	}
	if u, _ := e.s.Store.UserByID(ctx, e.admin.ID); u.LastLoginAt == nil {
		t.Fatal("last login not recorded")
	}
	if zaAudited(t, e.s, "auth.login") != 1 || zaAudited(t, e.s, "auth.login_failed") != 1 {
		t.Fatal("login audit entries missing")
	}

	// one more failure from 192.0.2.1 reaches the limit of 2: even the right password is refused
	if w, _ := e.do("POST", "/api/auth/login", map[string]string{"email": "nobody@example.com", "password": "whatever-pass"}, withCSRF()); w.Code != 401 {
		t.Fatalf("unknown user: %d", w.Code)
	}
	w, m = e.do("POST", "/api/auth/login", map[string]string{"email": "admin@example.com", "password": zaPassword}, withCSRF())
	if w.Code != 429 || m["error"] != "rate_limited" || w.Header().Get("Retry-After") == "" {
		t.Fatalf("blocked address: %d %v", w.Code, m)
	}

	// logout ends the session and clears the cookie
	w, m = e.do("POST", "/api/auth/logout", nil, withCookie(sess.Value), withCSRF())
	if w.Code != 200 || m["status"] != "ok" {
		t.Fatalf("logout: %d %v", w.Code, m)
	}
	if c := zaCookie(w, auth.SessionCookie); c == nil || c.MaxAge != -1 || c.Value != "" {
		t.Fatalf("logout cookie: %+v", c)
	}
	if w, _ := e.do("GET", "/api/auth/me", nil, withCookie(sess.Value)); w.Code != 401 {
		t.Fatalf("session survived logout: %d", w.Code)
	}
	if zaAudited(t, e.s, "auth.logout") != 1 {
		t.Fatal("logout not audited")
	}
	// logout requires a session
	if w, _ := e.do("POST", "/api/auth/logout", nil, withCSRF()); w.Code != 401 {
		t.Fatalf("anonymous logout: %d", w.Code)
	}
}

func TestZZPasswordChange(t *testing.T) {
	e, _ := newAuthEnv(t)
	ctx := context.Background()
	u, sess := zaUser(t, e.s, "op@example.com", "operator")
	other := auth.RandomToken(32)
	if err := e.s.Store.CreateSession(ctx, auth.TokenHash(other), u.ID, time.Hour, "", ""); err != nil {
		t.Fatal(err)
	}
	call := func(body any, opts ...reqOpt) (int, map[string]any, *http.Cookie) {
		w, m := e.do("POST", "/api/auth/password", body, append([]reqOpt{withCookie(sess), withCSRF()}, opts...)...)
		return w.Code, m, zaCookie(w, auth.SessionCookie)
	}
	if code, m, _ := call(nil, zaRawBody("[")); code != 400 || m["error"] != "bad_request" {
		t.Fatalf("malformed: %d %v", code, m)
	}
	if code, m, _ := call(map[string]string{"current": "wrong-password", "new": "Another-long-passw0rd"}); code != 403 || m["error"] != "invalid_credentials" {
		t.Fatalf("wrong current: %d %v", code, m)
	}
	if zaAudited(t, e.s, "auth.password_change_failed") != 1 {
		t.Fatal("failed change not audited")
	}
	code, m, _ := call(map[string]string{"current": zaPassword, "new": "short"})
	if code != 422 || !strings.Contains(w2s(m["fields"]), `"new"`) {
		t.Fatalf("weak new: %d %v", code, m)
	}
	code, m, c := call(map[string]string{"current": zaPassword, "new": "Another-long-passw0rd"})
	if code != 200 || m["status"] != "ok" || c == nil || c.Value == "" {
		t.Fatalf("change: %d %v %+v", code, m, c)
	}
	got, _ := e.s.Store.UserByID(ctx, u.ID)
	if !auth.CheckPassword(got.PasswordHash(), "Another-long-passw0rd") {
		t.Fatal("new password not stored")
	}
	// every other session ends, the new cookie works
	for _, old := range []string{sess, other} {
		if w, _ := e.do("GET", "/api/auth/me", nil, withCookie(old)); w.Code != 401 {
			t.Fatalf("old session still valid: %d", w.Code)
		}
	}
	if w, me := e.do("GET", "/api/auth/me", nil, withCookie(c.Value)); w.Code != 200 || me["email"] != "op@example.com" {
		t.Fatalf("new session: %d %v", w.Code, me)
	}
	if zaAudited(t, e.s, "auth.password_changed") != 1 {
		t.Fatal("password change not audited")
	}
}

func w2s(v any) string { return string(mustJSON(v)) }

func TestZZCSRFAndHeaders(t *testing.T) {
	e := newFullEnv(t)
	// state-changing request without the header
	w, m := e.do("POST", "/api/ip-lists", map[string]any{}, withCookie(e.cookie))
	if w.Code != 403 || m["error"] != "csrf" || !strings.Contains(m["message"].(string), "X-Corvus-Request") {
		t.Fatalf("missing header: %d %v", w.Code, m)
	}
	// foreign or malformed Origin
	for _, o := range []string{"https://evil.example", "http://[::1"} {
		w, m = e.do("DELETE", "/api/users/999", nil, withCookie(e.cookie), withCSRF(), zaHeader("Origin", o))
		if w.Code != 403 || m["error"] != "csrf" || !strings.Contains(m["message"].(string), "cross-origin") {
			t.Fatalf("origin %q: %d %v", o, w.Code, m)
		}
	}
	// same origin passes the check (and reaches the handler: user 999 does not exist)
	w, m = e.do("DELETE", "/api/users/999", nil, withCookie(e.cookie), withCSRF(), zaHeader("Origin", "http://example.com"))
	if w.Code != 404 || m["error"] != "not_found" {
		t.Fatalf("same origin: %d %v", w.Code, m)
	}
	// GET is not CSRF-checked
	if w, _ := e.do("GET", "/api/users", nil, withCookie(e.cookie)); w.Code != 200 {
		t.Fatalf("GET: %d", w.Code)
	}
	// Bearer requests are exempt (no cookie)
	tok, _ := e.createToken("ci", "operator", 1)
	if w, _ := e.do("POST", "/api/ip-lists", map[string]any{}, withBearer(tok)); w.Code == 403 {
		t.Fatalf("bearer request blocked by CSRF: %d %s", w.Code, w.Body)
	}

	// security headers on API responses (with no-store) ...
	w, _ = e.do("GET", "/api/status", nil)
	h := w.Header()
	if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("X-Frame-Options") != "DENY" || h.Get("Referrer-Policy") != "same-origin" ||
		!strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") || h.Get("Cache-Control") != "no-store" {
		t.Fatalf("API headers: %v", h)
	}
	// ... and on UI responses (cacheable)
	e.s.UI = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("console")) })
	e.h = e.s.Handler()
	w, _ = e.do("GET", "/dashboard", nil)
	if w.Code != 200 || w.Body.String() != "console" || w.Header().Get("X-Frame-Options") != "DENY" || w.Header().Get("Cache-Control") != "" {
		t.Fatalf("UI: %d %q %v", w.Code, w.Body, w.Header())
	}
	// unknown API endpoints answer JSON 404, not the UI
	for _, p := range []string{"/api/nope", "/api/proxy-hosts/1/nothing"} {
		w, m = e.do("GET", p, nil, withCookie(e.cookie))
		if w.Code != 404 || m["error"] != "not_found" || m["message"] != "unknown API endpoint" {
			t.Fatalf("%s: %d %v", p, w.Code, m)
		}
	}
}

func TestZZRolesAndBadInput(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")
	op := e.as("operator")
	// no session / unknown session
	if w, m := e.do("GET", "/api/users", nil); w.Code != 401 || m["message"] != "login required" {
		t.Fatalf("anonymous: %d %v", w.Code, m)
	}
	if w, m := e.do("GET", "/api/users", nil, withCookie("nope")); w.Code != 401 || m["message"] != "session expired" {
		t.Fatalf("unknown session: %d %v", w.Code, m)
	}
	// viewer → operator endpoint, operator → admin endpoint
	if w, m := e.do("POST", "/api/ip-lists", map[string]any{}, withCookie(viewer), withCSRF()); w.Code != 403 || m["error"] != "forbidden" {
		t.Fatalf("viewer: %d %v", w.Code, m)
	}
	if w, m := e.do("GET", "/api/users", nil, withCookie(op)); w.Code != 403 || m["error"] != "forbidden" {
		t.Fatalf("operator: %d %v", w.Code, m)
	}
	if n := zaAudited(t, e.s, "auth.forbidden"); n != 2 {
		t.Fatalf("auth.forbidden audited %d times", n)
	}
	// viewer may read
	if w, _ := e.do("GET", "/api/proxy-hosts", nil, withCookie(viewer)); w.Code != 200 {
		t.Fatalf("viewer read: %d", w.Code)
	}
	// invalid ids and bodies
	for _, p := range []string{"/api/users/abc", "/api/users/0", "/api/users/-3"} {
		if code, m := e.call("DELETE", p, nil); code != 400 || m["message"] != "invalid id" {
			t.Fatalf("%s: %d %v", p, code, m)
		}
	}
	w, m := e.do("POST", "/api/users", nil, withCookie(e.cookie), withCSRF(), zaRawBody(`{"email": "a@b.c", "extra": 1}`))
	if w.Code != 400 || !strings.Contains(m["message"].(string), "invalid JSON body") {
		t.Fatalf("unknown field: %d %v", w.Code, m)
	}
}

func TestZZAdminAllowList(t *testing.T) {
	e := newFullEnv(t)
	ctx := context.Background()
	if err := e.s.Store.SetNetwork(ctx, store.NetworkSettings{TrustedProxies: []string{}, RealIPHeader: "X-Forwarded-For",
		AdminAllow: []string{"10.0.0.0/8", "not-a-cidr"}}); err != nil {
		t.Fatal(err)
	}
	e.s.invalidateAllow()
	// outside the list: plain-text 403 for every path (API and UI)
	for _, p := range []string{"/api/status", "/", "/metrics"} {
		w, _ := e.do("GET", p, nil)
		if w.Code != 403 || !strings.Contains(w.Body.String(), "may not access the Corvus admin console") ||
			!strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
			t.Fatalf("%s from outside: %d %q", p, w.Code, w.Body)
		}
	}
	// inside the list, loopback, and an address without a port
	for _, addr := range []string{"10.1.2.3:5555", "127.0.0.1:80", "[::1]:80", "10.9.9.9"} {
		if w, _ := e.do("GET", "/api/status", nil, zaRemote(addr)); w.Code != 200 {
			t.Fatalf("from %s: %d", addr, w.Code)
		}
	}
	// an unparsable client address is not allowed
	if w, _ := e.do("GET", "/api/status", nil, zaRemote("garbage")); w.Code != 403 {
		t.Fatalf("garbage address: %d", w.Code)
	}
	// lock-out recovery switch
	t.Setenv("CORVUS_ADMIN_ALLOW_ANY", "1")
	e.h = e.s.Handler()
	if w, _ := e.do("GET", "/api/status", nil); w.Code != 200 {
		t.Fatalf("bypass: %d", w.Code)
	}
}

// A failing audit write does not fail the request; an internal error is a JSON 500.
func TestZZAuditFailureAndInternalError(t *testing.T) {
	e, path := newAuthEnv(t)
	zaExec(t, path, "DROP TABLE api_tokens")
	if code, m := e.call("GET", "/api/tokens", nil); code != 500 || m["error"] != "internal" || m["message"] != "internal error" {
		t.Fatalf("list tokens: %d %v", code, m)
	}
	zaExec(t, path, "DROP TABLE audit_log")
	if code, m := e.call("POST", "/api/auth/logout", nil); code != 200 || m["status"] != "ok" {
		t.Fatalf("logout without audit table: %d %v", code, m)
	}
}

func TestZZPreviewWaitPage(t *testing.T) {
	e := newFullEnv(t)
	html := t.TempDir()
	e.s.Paths.HTMLDir = html
	e.h = e.s.Handler()
	get := func(p string, opts ...reqOpt) *httpResult {
		w, _ := e.do("GET", p, nil, append([]reqOpt{withCookie(e.cookie)}, opts...)...)
		return &httpResult{code: w.Code, body: w.Body.String(), h: w.Header()}
	}
	if r := get("/preview/wait/shop", func(r *http.Request) { r.Header.Del("Cookie") }); r.code != 401 {
		t.Fatalf("anonymous preview: %d", r.code)
	}
	for _, room := range []string{"Shop", "-shop", "a_b", strings.Repeat("a", 33)} {
		if r := get("/preview/wait/" + room); r.code != 404 {
			t.Fatalf("room %q: %d", room, r.code)
		}
	}
	r := get("/preview/wait/shop?lang=de")
	if r.code != 302 || r.h.Get("Location") != "/preview/wait/shop?lang=de&preview=1" {
		t.Fatalf("redirect: %d %v", r.code, r.h.Get("Location"))
	}
	if r := get("/preview/wait/shop?preview=1"); r.code != 404 || !strings.Contains(r.body, "waiting page not found") {
		t.Fatalf("missing page: %d %q", r.code, r.body)
	}
	if err := os.WriteFile(filepath.Join(html, "wait.html"), []byte("<html>queue</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	r = get("/preview/wait/shop-2?preview=1")
	if r.code != 200 || r.body != "<html>queue</html>" || r.h.Get("X-Frame-Options") != "SAMEORIGIN" ||
		!strings.Contains(r.h.Get("Content-Security-Policy"), "script-src 'unsafe-inline'") || r.h.Get("Cache-Control") != "no-store" ||
		!strings.HasPrefix(r.h.Get("Content-Type"), "text/html") {
		t.Fatalf("page: %d %q %v", r.code, r.body, r.h)
	}
}

type httpResult struct {
	code int
	body string
	h    http.Header
}

func TestZZPurgeLoopStops(t *testing.T) {
	e := newTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { e.s.PurgeLoop(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("PurgeLoop did not return after cancel")
	}
}

func TestZZClientIP(t *testing.T) {
	for addr, want := range map[string]string{"192.0.2.1:1234": "192.0.2.1", "[2001:db8::1]:443": "2001:db8::1", "unix-socket": "unix-socket"} {
		r, _ := http.NewRequest("GET", "/", nil)
		r.RemoteAddr = addr
		if got := clientIP(r); got != want {
			t.Errorf("clientIP(%q) = %q, want %q", addr, got, want)
		}
	}
}
