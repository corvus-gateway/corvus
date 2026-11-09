package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
)

func zaFields(m map[string]any) map[string]string {
	out := map[string]string{}
	l, _ := m["fields"].([]any)
	for _, f := range l {
		fm := f.(map[string]any)
		out[fm["field"].(string)] = fm["code"].(string)
	}
	return out
}

func TestZZUsersCreateAndList(t *testing.T) {
	e := newFullEnv(t)
	ctx := context.Background()

	// every problem is reported at once
	code, m := e.call("POST", "/api/users", map[string]any{"email": "not-an-email", "role": "root", "name": strings.Repeat("n", 65), "password": "short"})
	f := zaFields(m)
	if code != 422 || f["email"] != "invalid" || f["role"] != "invalid" || f["name"] != "invalid" || f["password"] != "weak" {
		t.Fatalf("invalid user: %d %v", code, m)
	}
	if code, _ := e.call("POST", "/api/users", nil); code != 400 {
		t.Fatalf("empty body: %d", code)
	}
	// valid: email normalised, name derived from it
	m = e.must(201, "POST", "/api/users", map[string]any{"email": "  New.Op@Example.COM ", "role": "operator", "password": zaPassword})
	if m["email"] != "new.op@example.com" || m["name"] != "new.op" || m["role"] != "operator" || m["password_hash"] != nil {
		t.Fatalf("created: %v", m)
	}
	u, err := e.s.Store.UserByEmail(ctx, "new.op@example.com")
	if err != nil || !auth.CheckPassword(u.PasswordHash(), zaPassword) {
		t.Fatalf("stored user: %v", err)
	}
	// duplicate email
	code, m = e.call("POST", "/api/users", map[string]any{"email": "NEW.OP@example.com", "name": "Again", "role": "viewer", "password": zaPassword})
	if f := zaFields(m); code != 422 || f["email"] != "duplicate" {
		t.Fatalf("duplicate: %d %v", code, m)
	}
	if zaAudited(t, e.s, "user.create") != 1 {
		t.Fatal("user.create not audited once")
	}

	code, list := e.callList("GET", "/api/users")
	if code != 200 || len(list) != 2 {
		t.Fatalf("list: %d %v", code, list)
	}
	for _, it := range list {
		if strings.Contains(w2s(it), "password") || strings.Contains(w2s(it), "totp") {
			t.Fatalf("secrets in user list: %v", it)
		}
	}
	// only administrators manage users
	op := e.as("operator")
	for _, r := range []struct{ method, path string }{{"GET", "/api/users"}, {"POST", "/api/users"}, {"PUT", "/api/users/1"}, {"DELETE", "/api/users/1"}, {"POST", "/api/users/1/reset-mfa"}} {
		if w, _ := e.do(r.method, r.path, map[string]any{}, withCookie(op), withCSRF()); w.Code != 403 {
			t.Fatalf("operator %s %s: %d", r.method, r.path, w.Code)
		}
	}
}

func TestZZUsersUpdate(t *testing.T) {
	e := newFullEnv(t)
	ctx := context.Background()
	op, opSess := zaUser(t, e.s, "op@example.com", "operator")
	path := fmt.Sprintf("/api/users/%d", op.ID)
	alive := func(sess string) bool {
		w, _ := e.do("GET", "/api/auth/me", nil, withCookie(sess))
		return w.Code == 200
	}

	if code, _ := e.call("PUT", path, nil); code != 400 {
		t.Fatalf("empty body: %d", code)
	}
	if code, m := e.call("PUT", "/api/users/4242", map[string]any{"name": "x"}); code != 404 || m["error"] != "not_found" {
		t.Fatalf("unknown user: %d %v", code, m)
	}
	if code, m := e.call("PUT", path, map[string]any{"role": "superuser"}); code != 422 || zaFields(m)["role"] != "invalid" {
		t.Fatalf("bad role: %d %v", code, m)
	}
	if code, m := e.call("PUT", path, map[string]any{"password": "short"}); code != 422 || zaFields(m)["password"] != "weak" {
		t.Fatalf("weak password: %d %v", code, m)
	}

	// a rename keeps the user's sessions
	m := e.must(200, "PUT", path, map[string]any{"name": "  Operator One "})
	if m["name"] != "Operator One" || m["role"] != "operator" || !alive(opSess) {
		t.Fatalf("rename: %v alive=%v", m, alive(opSess))
	}
	// a role change ends them
	m = e.must(200, "PUT", path, map[string]any{"role": "viewer"})
	if m["role"] != "viewer" || m["name"] != "Operator One" || alive(opSess) {
		t.Fatalf("role change: %v alive=%v", m, alive(opSess))
	}
	// disabling ends them too (and blocks new sessions)
	sess2 := auth.RandomToken(32)
	_ = e.s.Store.CreateSession(ctx, auth.TokenHash(sess2), op.ID, time.Hour, "", "")
	m = e.must(200, "PUT", path, map[string]any{"disabled": true})
	if m["disabled"] != true {
		t.Fatalf("disable: %v", m)
	}
	m = e.must(200, "PUT", path, map[string]any{"disabled": false})
	if m["disabled"] != false || alive(sess2) {
		t.Fatalf("enable: %v (session of the disabled account came back: %v)", m, alive(sess2))
	}
	// an administrator setting someone's password ends that user's sessions
	sess3 := auth.RandomToken(32)
	_ = e.s.Store.CreateSession(ctx, auth.TokenHash(sess3), op.ID, time.Hour, "", "")
	e.must(200, "PUT", path, map[string]any{"password": "Brand-new-passw0rd!"})
	if alive(sess3) {
		t.Fatal("session survived an administrator's password reset")
	}
	got, _ := e.s.Store.UserByID(ctx, op.ID)
	if !auth.CheckPassword(got.PasswordHash(), "Brand-new-passw0rd!") {
		t.Fatal("password not changed")
	}
	if zaAudited(t, e.s, "user.password_reset") != 1 || zaAudited(t, e.s, "user.update") != 5 {
		t.Fatalf("audit: reset=%d update=%d", zaAudited(t, e.s, "user.password_reset"), zaAudited(t, e.s, "user.update"))
	}

	// own account: no role change, no disabling; a password or name change is fine and keeps the session
	self := fmt.Sprintf("/api/users/%d", e.admin.ID)
	for _, body := range []map[string]any{{"role": "viewer"}, {"disabled": true}} {
		if code, m := e.call("PUT", self, body); code != 409 || m["error"] != "conflict" || !strings.Contains(m["message"].(string), "your own") {
			t.Fatalf("self %v: %d %v", body, code, m)
		}
	}
	m = e.must(200, "PUT", self, map[string]any{"name": "Chief", "role": "admin", "password": "Self-chosen-passw0rd"})
	if m["name"] != "Chief" || !alive(e.cookie) {
		t.Fatalf("self update: %v alive=%v", m, alive(e.cookie))
	}

	// another administrator can be demoted while one remains
	other, _ := zaUser(t, e.s, "admin2@example.com", "admin")
	m = e.must(200, "PUT", fmt.Sprintf("/api/users/%d", other.ID), map[string]any{"role": "operator"})
	if m["role"] != "operator" {
		t.Fatalf("demote: %v", m)
	}
}

func TestZZUsersDeleteAndResetMFA(t *testing.T) {
	e := newFullEnv(t)
	ctx := context.Background()
	viewer, vSess := zaUser(t, e.s, "v@example.com", "viewer")
	other, _ := zaUser(t, e.s, "admin2@example.com", "admin")

	if code, m := e.call("DELETE", fmt.Sprintf("/api/users/%d", e.admin.ID), nil); code != 409 || !strings.Contains(m["message"].(string), "your own account") {
		t.Fatalf("delete self: %d %v", code, m)
	}
	if code, _ := e.call("DELETE", "/api/users/4242", nil); code != 404 {
		t.Fatalf("delete unknown: %d", code)
	}
	if code, _ := e.call("POST", "/api/users/4242/reset-mfa", nil); code != 404 {
		t.Fatalf("reset unknown: %d", code)
	}
	if code, _ := e.call("POST", "/api/users/x/reset-mfa", nil); code != 400 {
		t.Fatalf("reset bad id: %d", code)
	}

	// reset MFA: off, secret gone, sessions ended
	_ = e.s.Store.SetTOTPSecret(ctx, viewer.ID, "v1:whatever")
	_ = e.s.Store.SetMFA(ctx, viewer.ID, true, []string{"h1"})
	if m := e.must(200, "POST", fmt.Sprintf("/api/users/%d/reset-mfa", viewer.ID), nil); m["status"] != "ok" {
		t.Fatalf("reset: %v", m)
	}
	got, _ := e.s.Store.UserByID(ctx, viewer.ID)
	if got.MFAEnabled || got.TOTPSecret() != "" || got.RecoveryCodes() != "[]" {
		t.Fatalf("after reset: enabled=%v secret=%q codes=%q", got.MFAEnabled, got.TOTPSecret(), got.RecoveryCodes())
	}
	if w, _ := e.do("GET", "/api/auth/me", nil, withCookie(vSess)); w.Code != 401 {
		t.Fatalf("session survived MFA reset: %d", w.Code)
	}
	if zaAudited(t, e.s, "user.mfa_reset") != 1 {
		t.Fatal("reset not audited")
	}

	// delete: a viewer and another administrator (one remains)
	for _, id := range []int64{viewer.ID, other.ID} {
		if m := e.must(200, "DELETE", fmt.Sprintf("/api/users/%d", id), nil); m["status"] != "ok" {
			t.Fatalf("delete %d: %v", id, m)
		}
		if _, err := e.s.Store.UserByID(ctx, id); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("user %d still exists: %v", id, err)
		}
	}
	if zaAudited(t, e.s, "user.delete") != 2 {
		t.Fatal("delete not audited")
	}
}

func TestZZLastAdminErr(t *testing.T) {
	err := lastAdminErr(store.ErrLastAdmin)
	var c conflictError
	if !errors.As(err, &c) || c.Error() != store.ErrLastAdmin.Error() {
		t.Fatalf("lastAdminErr(ErrLastAdmin) = %#v", err)
	}
	other := errors.New("boom")
	if lastAdminErr(other) != other {
		t.Fatal("other errors must pass through")
	}
}
