package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/seal"
)

// newLimitEnv is a testEnv with the sign-in machinery (limiters, MFA, data key); it
// needs no gateway, so it also runs where deployments cannot.
func newLimitEnv(t *testing.T, ipMax, acctMax int) *testEnv {
	t.Helper()
	e := newTestEnv(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	e.s.DataKey = key
	e.s.Limiter = auth.NewLimiter(ipMax, time.Minute)
	e.s.AcctLimiter = auth.NewLimiter(acctMax, time.Minute)
	e.s.MFA = auth.NewMFAChallenges()
	e.h = e.s.Handler()
	return e
}

func TestZZSignInInputLimits(t *testing.T) {
	e := newLimitEnv(t, 1000, 1000)
	long := strings.Repeat("a", maxEmailLen) + "@example.com"
	for _, body := range []map[string]string{
		{"email": long, "password": "whatever-pass"},
		{"email": "a@example.com", "password": strings.Repeat("p", maxPasswordInput+1)},
	} {
		if w, m := e.do("POST", "/api/auth/login", body, withCSRF()); w.Code != 400 || m["error"] != "bad_request" {
			t.Fatalf("oversized login: %d %v", w.Code, m)
		}
	}
	for _, body := range []map[string]string{
		{"mfa_token": strings.Repeat("t", maxCodeInput+1), "code": "123456"},
		{"mfa_token": "t", "code": strings.Repeat("1", maxCodeInput+1)},
	} {
		if w, m := e.do("POST", "/api/auth/login/mfa", body, withCSRF()); w.Code != 400 || m["error"] != "bad_request" {
			t.Fatalf("oversized MFA login: %d %v", w.Code, m)
		}
	}
	if n := zaAudited(t, e.s, "auth.login_failed"); n != 0 {
		t.Fatalf("oversized input audited %d times", n)
	}
}

// Attempts on a locked account count against the client address, so they stop (and
// stop being audited) once the address is blocked.
func TestZZLockedAccountCountsAgainstAddress(t *testing.T) {
	e := newLimitEnv(t, 3, 1)
	zaUser(t, e.s, "locked@example.com", "viewer")
	login := func(remote string) (int, map[string]any) {
		w, m := e.do("POST", "/api/auth/login", map[string]string{"email": "locked@example.com", "password": "wrong-password"}, withCSRF(), zaRemote(remote))
		return w.Code, m
	}
	if code, _ := login("198.51.100.1:1"); code != 401 {
		t.Fatalf("first failure: %d", code)
	}
	for i := 0; i < 3; i++ {
		if code, m := login("198.51.100.2:1"); code != 429 || !strings.Contains(m["message"].(string), "account") {
			t.Fatalf("locked attempt %d: %d %v", i, code, m)
		}
	}
	for i := 0; i < 5; i++ {
		if code, m := login("198.51.100.2:1"); code != 429 || strings.Contains(m["message"].(string), "account") {
			t.Fatalf("blocked address %d: %d %v", i, code, m)
		}
	}
	if n := zaAudited(t, e.s, "auth.login_locked"); n != 3 {
		t.Fatalf("auth.login_locked audited %d times", n)
	}
}

func TestZZPasswordChangeLimitsAndTokens(t *testing.T) {
	e := newLimitEnv(t, 1000, 2)
	ctx := context.Background()
	u, sess := zaUser(t, e.s, "pw@example.com", "admin")
	if _, err := e.s.Store.CreateAPIToken(ctx, "ci", "cvs_abcd", auth.TokenHash("t1"), "viewer", u.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Store.CreateAPIToken(ctx, "other", "cvs_efgh", auth.TokenHash("t2"), "viewer", e.admin.ID, nil); err != nil {
		t.Fatal(err)
	}
	change := func(current string) (int, map[string]any) {
		w, m := e.do("POST", "/api/auth/password", map[string]string{"current": current, "new": "Another-long-passw0rd"}, withCookie(sess), withCSRF())
		return w.Code, m
	}
	if code, _ := change("wrong-password"); code != 403 {
		t.Fatalf("wrong: %d", code)
	}
	if code, _ := change(strings.Repeat("x", maxPasswordInput+1)); code != 403 {
		t.Fatalf("oversized: %d", code)
	}
	// the account limit is reached: even the right password waits
	if code, m := change(zaPassword); code != 429 || m["error"] != "rate_limited" {
		t.Fatalf("limited: %d %v", code, m)
	}
	e.s.AcctLimiter = auth.NewLimiter(2, time.Minute)
	w, m := e.do("POST", "/api/auth/password", map[string]string{"current": zaPassword, "new": "Another-long-passw0rd"}, withCookie(sess), withCSRF())
	if w.Code != 200 || m["status"] != "ok" || zaCookie(w, auth.SessionCookie) == nil {
		t.Fatalf("change: %d %v", w.Code, m)
	}
	toks, err := e.s.Store.ListAPITokens(ctx)
	if err != nil || len(toks) != 1 || toks[0].CreatedBy != e.admin.ID {
		t.Fatalf("tokens after the change: %v %+v", err, toks)
	}
	list, _ := e.s.Store.ListAudit(ctx, 1)
	var d map[string]any
	_ = json.Unmarshal(list[0].Detail, &d)
	if list[0].Action != "auth.password_changed" || d["tokens_revoked"] != float64(1) {
		t.Fatalf("audit: %s %s", list[0].Action, list[0].Detail)
	}

	// the address limit applies as well
	e.s.Limiter = auth.NewLimiter(1, time.Minute)
	e.s.Limiter.Fail("192.0.2.1")
	if w, m := e.do("POST", "/api/auth/password", map[string]string{"current": "x", "new": "y"}, withCookie(zaCookie(w, auth.SessionCookie).Value), withCSRF()); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("address limit: %d %v", w.Code, m)
	}
}

// Turning MFA off checks the password and the code together and is rate limited; a
// recovery code is not used up by an attempt with a wrong password.
func TestZZMFADisableChecksBoth(t *testing.T) {
	e := newLimitEnv(t, 1000, 3)
	ctx := context.Background()
	u, sess := zaUser(t, e.s, "mfa-off@example.com", "viewer")
	secret := auth.NewTOTPSecret()
	sealed, _ := seal.Seal(e.s.DataKey, secret)
	if err := e.s.Store.SetTOTPSecret(ctx, u.ID, sealed); err != nil {
		t.Fatal(err)
	}
	codes, hashes := auth.NewRecoveryCodes(2)
	if err := e.s.Store.SetMFA(ctx, u.ID, true, hashes); err != nil {
		t.Fatal(err)
	}
	disable := func(pw, code string) (int, map[string]any) {
		w, m := e.do("POST", "/api/auth/mfa/disable", map[string]string{"password": pw, "code": code}, withCookie(sess), withCSRF())
		return w.Code, m
	}
	msgs := map[string]bool{}
	for _, c := range [][2]string{{"wrong-password", codes[0]}, {zaPassword, zaWrongTOTP(t, secret)}} {
		code, m := disable(c[0], c[1])
		if code != 403 || m["error"] != "invalid_credentials" {
			t.Fatalf("disable %v: %d %v", c, code, m)
		}
		msgs[m["message"].(string)] = true
	}
	if len(msgs) != 1 {
		t.Fatalf("answers differ: %v", msgs)
	}
	if got, _ := e.s.Store.UserByID(ctx, u.ID); !strings.Contains(got.RecoveryCodes(), auth.RecoveryHash(codes[0])) {
		t.Fatal("recovery code used up by a failed attempt")
	}
	if code, _ := disable("wrong-password", "000000"); code != 403 {
		t.Fatalf("third failure: %d", code)
	}
	if code, m := disable(zaPassword, codes[0]); code != 429 || m["error"] != "rate_limited" {
		t.Fatalf("limited: %d %v", code, m)
	}
	e.s.AcctLimiter = auth.NewLimiter(3, time.Minute)
	if code, m := disable(zaPassword, codes[0]); code != 200 {
		t.Fatalf("disable: %d %v", code, m)
	}
	if got, _ := e.s.Store.UserByID(ctx, u.ID); got.MFAEnabled {
		t.Fatal("MFA still on")
	}
	if zaAudited(t, e.s, "auth.mfa_disable_failed") != 3 {
		t.Fatal("failures not audited")
	}
}

func TestZZPurgeRemovesOldAudit(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	if err := e.s.Store.Audit(ctx, nil, "recent", "", nil, ""); err != nil {
		t.Fatal(err)
	}
	e.s.purge(ctx)
	if n := zaAudited(t, e.s, "recent"); n != 1 {
		t.Fatalf("recent entry removed: %d", n)
	}
}
