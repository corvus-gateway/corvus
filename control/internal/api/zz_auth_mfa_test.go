package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/seal"
)

// mfaLogin signs in with the password and returns the MFA challenge token.
func zaMFAChallenge(t *testing.T, e *fullEnv, email string, opts ...reqOpt) string {
	t.Helper()
	w, m := e.do("POST", "/api/auth/login", map[string]string{"email": email, "password": zaPassword}, append([]reqOpt{withCSRF()}, opts...)...)
	if w.Code != 200 || m["mfa_required"] != true {
		t.Fatalf("password step: %d %v", w.Code, m)
	}
	if zaCookie(w, "corvus_session") != nil {
		t.Fatal("session issued before the second factor")
	}
	tok, _ := m["mfa_token"].(string)
	if tok == "" {
		t.Fatalf("no mfa_token: %v", m)
	}
	return tok
}

func TestZZMFALifecycle(t *testing.T) {
	e, _ := newAuthEnv(t)
	ctx := context.Background()
	u, sess := zaUser(t, e.s, "mfa@example.com", "operator")
	call := func(method, path string, body any, opts ...reqOpt) (int, map[string]any) {
		w, m := e.do(method, path, body, append([]reqOpt{withCookie(sess), withCSRF()}, opts...)...)
		return w.Code, m
	}

	// nothing to enable or disable yet
	if code, m := call("POST", "/api/auth/mfa/enable", map[string]string{"code": "123456"}); code != 400 || m["error"] != "mfa_not_started" {
		t.Fatalf("enable before setup: %d %v", code, m)
	}
	if code, m := call("POST", "/api/auth/mfa/disable", map[string]string{"password": zaPassword, "code": "123456"}); code != 409 || m["error"] != "mfa_disabled" {
		t.Fatalf("disable while off: %d %v", code, m)
	}
	for _, p := range []string{"/api/auth/mfa/enable", "/api/auth/mfa/disable"} {
		if code, _ := call("POST", p, nil, zaRawBody("{")); code != 400 {
			t.Fatalf("malformed %s: %d", p, code)
		}
	}
	if w, _ := e.do("POST", "/api/auth/mfa/setup", nil, withCSRF()); w.Code != 401 {
		t.Fatalf("anonymous setup: %d", w.Code)
	}

	// setup: secret stored sealed, QR code and otpauth URL returned
	code, m := call("POST", "/api/auth/mfa/setup", nil)
	if code != 200 {
		t.Fatalf("setup: %d %v", code, m)
	}
	secret, _ := m["secret"].(string)
	if len(secret) != 32 || !strings.HasPrefix(m["qr_png"].(string), "data:image/png;base64,") ||
		!strings.HasPrefix(m["otpauth_url"].(string), "otpauth://totp/Corvus:mfa@example.com?") || !strings.Contains(m["otpauth_url"].(string), "secret="+secret) {
		t.Fatalf("setup response: %v", m)
	}
	db, _ := e.s.Store.UserByID(ctx, u.ID)
	if db.MFAEnabled || db.TOTPSecret() == "" || strings.Contains(db.TOTPSecret(), secret) {
		t.Fatalf("stored secret: enabled=%v %q", db.MFAEnabled, db.TOTPSecret())
	}
	if plain, err := seal.Open(e.s.DataKey, db.TOTPSecret()); err != nil || plain != secret {
		t.Fatalf("sealed secret: %q %v", plain, err)
	}
	// password sign-in still works without a second factor until it is enabled
	if w, m := e.do("POST", "/api/auth/login", map[string]string{"email": "mfa@example.com", "password": zaPassword}, withCSRF()); w.Code != 200 || m["mfa_required"] != nil {
		t.Fatalf("login before enable: %d %v", w.Code, m)
	}

	// wrong or malformed codes do not enable it
	for _, c := range []string{zaWrongTOTP(t, secret), "12345", "abcdef-ghjkm"} {
		if code, m := call("POST", "/api/auth/mfa/enable", map[string]string{"code": c}); code != 422 || m["error"] != "invalid_code" {
			t.Fatalf("enable with %q: %d %v", c, code, m)
		}
	}
	now := zaTOTP(t, secret, time.Now())
	code, m = call("POST", "/api/auth/mfa/enable", map[string]string{"code": " " + now + " "})
	if code != 200 {
		t.Fatalf("enable: %d %v", code, m)
	}
	recovery := m["recovery_codes"].([]any)
	if len(recovery) != 10 {
		t.Fatalf("recovery codes: %v", recovery)
	}
	if code, m := call("POST", "/api/auth/mfa/setup", nil); code != 409 || m["error"] != "mfa_enabled" {
		t.Fatalf("setup while on: %d %v", code, m)
	}
	if code, m := call("POST", "/api/auth/mfa/enable", map[string]string{"code": now}); code != 409 || m["error"] != "mfa_enabled" {
		t.Fatalf("enable while on: %d %v", code, m)
	}
	for _, a := range []string{"auth.mfa_setup_started", "auth.mfa_enabled"} {
		if zaAudited(t, e.s, a) != 1 {
			t.Fatalf("%s not audited", a)
		}
	}

	// sign-in: password first, then the code
	tok := zaMFAChallenge(t, e, "mfa@example.com")
	if w, m := e.do("POST", "/api/auth/login/mfa", nil, withCSRF(), zaRawBody("x")); w.Code != 400 || m["error"] != "bad_request" {
		t.Fatalf("malformed: %d %v", w.Code, m)
	}
	if w, m := e.do("POST", "/api/auth/login/mfa", map[string]string{"mfa_token": "forged", "code": now}, withCSRF()); w.Code != 401 || m["error"] != "mfa_expired" {
		t.Fatalf("forged challenge: %d %v", w.Code, m)
	}
	if w, m := e.do("POST", "/api/auth/login/mfa", map[string]string{"mfa_token": tok, "code": zaWrongTOTP(t, secret)}, withCSRF()); w.Code != 401 || m["error"] != "invalid_code" {
		t.Fatalf("wrong code: %d %v", w.Code, m)
	}
	// the code used to enable cannot be replayed
	if w, m := e.do("POST", "/api/auth/login/mfa", map[string]string{"mfa_token": tok, "code": now}, withCSRF()); w.Code != 401 || m["error"] != "invalid_code" {
		t.Fatalf("replayed code: %d %v", w.Code, m)
	}
	if zaAudited(t, e.s, "auth.mfa_failed") != 2 {
		t.Fatal("failed codes not audited")
	}
	next := zaTOTP(t, secret, time.Now().Add(30*time.Second))
	w, m := e.do("POST", "/api/auth/login/mfa", map[string]string{"mfa_token": tok, "code": next}, withCSRF())
	if w.Code != 200 || m["email"] != "mfa@example.com" || m["mfa_enabled"] != true {
		t.Fatalf("second factor: %d %v", w.Code, m)
	}
	s := zaCookie(w, "corvus_session")
	if s == nil || zaCookie(w, deviceCookie) == nil {
		t.Fatalf("cookies: %v", w.Result().Cookies())
	}
	if w, _ := e.do("GET", "/api/auth/me", nil, withCookie(s.Value)); w.Code != 200 {
		t.Fatalf("session after MFA: %d", w.Code)
	}
	// the challenge is single-use
	if w, m := e.do("POST", "/api/auth/login/mfa", map[string]string{"mfa_token": tok, "code": next}, withCSRF()); w.Code != 401 || m["error"] != "mfa_expired" {
		t.Fatalf("reused challenge: %d %v", w.Code, m)
	}

	// a recovery code works once
	rc := strings.ToUpper(recovery[0].(string))
	tok = zaMFAChallenge(t, e, "mfa@example.com")
	if w, _ := e.do("POST", "/api/auth/login/mfa", map[string]string{"mfa_token": tok, "code": rc}, withCSRF()); w.Code != 200 {
		t.Fatalf("recovery code: %d %s", w.Code, w.Body)
	}
	tok = zaMFAChallenge(t, e, "mfa@example.com")
	if w, m := e.do("POST", "/api/auth/login/mfa", map[string]string{"mfa_token": tok, "code": rc}, withCSRF()); w.Code != 401 || m["error"] != "invalid_code" {
		t.Fatalf("reused recovery code: %d %v", w.Code, m)
	}

	// disabling needs the password and a valid second factor
	if code, m := call("POST", "/api/auth/mfa/disable", map[string]string{"password": "wrong-password", "code": recovery[1].(string)}); code != 403 || m["error"] != "invalid_credentials" {
		t.Fatalf("disable with wrong password: %d %v", code, m)
	}
	if code, m := call("POST", "/api/auth/mfa/disable", map[string]string{"password": zaPassword, "code": "nope-nope"}); code != 403 || m["error"] != "invalid_credentials" {
		t.Fatalf("disable with wrong code: %d %v", code, m)
	}
	if zaAudited(t, e.s, "auth.mfa_disable_failed") != 2 {
		t.Fatal("failed disable not audited")
	}
	if code, m := call("POST", "/api/auth/mfa/disable", map[string]string{"password": zaPassword, "code": recovery[2].(string)}); code != 200 || m["status"] != "ok" {
		t.Fatalf("disable: %d %v", code, m)
	}
	db, _ = e.s.Store.UserByID(ctx, u.ID)
	if db.MFAEnabled || db.TOTPSecret() != "" {
		t.Fatalf("MFA still on after disable: %+v", db)
	}
	if zaAudited(t, e.s, "auth.mfa_disabled") != 1 {
		t.Fatal("disable not audited")
	}
}

// MFA challenges are limited per address, per account and per challenge, and end when
// the account is disabled.
func TestZZMFALimits(t *testing.T) {
	e, _ := newAuthEnv(t)
	ctx := context.Background()
	u, _ := zaUser(t, e.s, "lim@example.com", "viewer")
	secret := auth.NewTOTPSecret()
	sealed, _ := seal.Seal(e.s.DataKey, secret)
	if err := e.s.Store.SetTOTPSecret(ctx, u.ID, sealed); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Store.SetMFA(ctx, u.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	wrong := zaWrongTOTP(t, secret)
	try := func(tok, code string, opts ...reqOpt) (int, string) {
		w, m := e.do("POST", "/api/auth/login/mfa", map[string]string{"mfa_token": tok, "code": code}, append([]reqOpt{withCSRF()}, opts...)...)
		s, _ := m["error"].(string)
		return w.Code, s
	}

	// a challenge allows five attempts
	tok := zaMFAChallenge(t, e, "lim@example.com")
	for i := 0; i < 5; i++ {
		if code, kind := try(tok, wrong, zaRemote("198.51.100.1:1")); code != 401 || kind != "invalid_code" {
			t.Fatalf("attempt %d: %d %s", i, code, kind)
		}
	}
	if code, kind := try(tok, zaTOTP(t, secret, time.Now()), zaRemote("198.51.100.2:1")); code != 401 || kind != "mfa_expired" {
		t.Fatalf("sixth attempt: %d %s", code, kind)
	}

	// per-account limit (counted across challenges and addresses)
	e.s.AcctLimiter = auth.NewLimiter(1, time.Minute)
	tok = zaMFAChallenge(t, e, "lim@example.com")
	if code, _ := try(tok, wrong, zaRemote("198.51.100.3:1")); code != 401 {
		t.Fatalf("account failure: %d", code)
	}
	if code, kind := try(tok, zaTOTP(t, secret, time.Now()), zaRemote("198.51.100.4:1")); code != 429 || kind != "rate_limited" {
		t.Fatalf("account limit: %d %s", code, kind)
	}
	e.s.AcctLimiter = auth.NewLimiter(1000, time.Minute)

	// per-address limit
	e.s.Limiter = auth.NewLimiter(1, time.Minute)
	e.s.Limiter.Fail("198.51.100.9")
	w, m := e.do("POST", "/api/auth/login/mfa", map[string]string{"mfa_token": tok, "code": "000000"}, withCSRF(), zaRemote("198.51.100.9:1"))
	if w.Code != 429 || m["error"] != "rate_limited" || w.Header().Get("Retry-After") == "" {
		t.Fatalf("address limit: %d %v", w.Code, m)
	}
	e.s.Limiter = auth.NewLimiter(1000, time.Minute)

	// the account is disabled while the challenge is pending
	tok = zaMFAChallenge(t, e, "lim@example.com")
	if err := e.s.Store.UpdateUser(ctx, u.ID, u.Name, u.Role, true); err != nil {
		t.Fatal(err)
	}
	if code, kind := try(tok, zaTOTP(t, secret, time.Now())); code != 401 || kind != "mfa_expired" {
		t.Fatalf("disabled during MFA: %d %s", code, kind)
	}
}

// A TOTP code cannot be checked when the stored secret cannot be decrypted.
func TestZZMFAUndecryptableSecret(t *testing.T) {
	e, _ := newAuthEnv(t)
	ctx := context.Background()
	u, _ := zaUser(t, e.s, "key@example.com", "viewer")
	secret := auth.NewTOTPSecret()
	other := make([]byte, 32)
	other[0] = 1
	sealed, _ := seal.Seal(other, secret) // sealed with another data key
	_ = e.s.Store.SetTOTPSecret(ctx, u.ID, sealed)
	_ = e.s.Store.SetMFA(ctx, u.ID, true, nil)
	tok := zaMFAChallenge(t, e, "key@example.com")
	w, m := e.do("POST", "/api/auth/login/mfa", map[string]string{"mfa_token": tok, "code": zaTOTP(t, secret, time.Now())}, withCSRF())
	if w.Code != 401 || m["error"] != "invalid_code" {
		t.Fatalf("undecryptable secret: %d %v", w.Code, m)
	}
}
