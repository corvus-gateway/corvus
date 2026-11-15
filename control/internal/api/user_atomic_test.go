package api

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
)

// atomicEnv is a test server whose database file is reachable directly, so a test can
// install SQLite triggers that make one statement of a request fail.
type atomicEnv struct {
	*testEnv
	raw *sql.DB
}

func newAtomicEnv(t *testing.T, adminPassword string) *atomicEnv {
	t.Helper()
	path := filepath.Join(t.TempDir(), "corvus.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	hash := "x"
	if adminPassword != "" {
		if hash, err = auth.HashPassword(adminPassword); err != nil {
			t.Fatal(err)
		}
	}
	admin, err := st.CreateUser(ctx, "admin@example.com", "Admin", hash, "admin")
	if err != nil {
		t.Fatal(err)
	}
	cookie := auth.RandomToken(32)
	if err := st.CreateSession(ctx, auth.TokenHash(cookie), admin.ID, time.Hour, "127.0.0.1", "test"); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Traffic: traffic.New(), Version: "1.2.3-test",
		Limiter: auth.NewLimiter(10, time.Minute), AcctLimiter: auth.NewLimiter(10, time.Minute)}
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	return &atomicEnv{testEnv: &testEnv{t: t, s: s, h: s.Handler(), admin: admin, cookie: cookie}, raw: raw}
}

func (e *atomicEnv) exec(q string) {
	e.t.Helper()
	if _, err := e.raw.Exec(q); err != nil {
		e.t.Fatal(err)
	}
}

func (e *atomicEnv) sessions(userID int64) int {
	var n int
	if err := e.raw.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, userID).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// A password reset whose later step fails changes nothing: the password, the API
// tokens and the sessions stay as they were (they used to be changed before the
// failing step, while the request reported an error).
func TestUserUpdateIsAtomic(t *testing.T) {
	e := newAtomicEnv(t, "")
	ctx := context.Background()
	oldHash, _ := auth.HashPassword("Old-long-passw0rd")
	op, err := e.s.Store.CreateUser(ctx, "op@example.com", "Op", oldHash, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Store.CreateAPIToken(ctx, "op-ci", "cvs_opop", auth.TokenHash("cvs_op-token"), "operator", op.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Store.CreateSession(ctx, auth.TokenHash("op-session"), op.ID, time.Hour, "127.0.0.1", "test"); err != nil {
		t.Fatal(err)
	}
	unchanged := func(step string) {
		t.Helper()
		u, err := e.s.Store.UserByID(ctx, op.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !auth.CheckPassword(u.PasswordHash(), "Old-long-passw0rd") || u.Name != "Op" || u.Role != "operator" {
			t.Fatalf("%s: user changed: %+v", step, u)
		}
		if _, err := e.s.Store.APITokenByHash(ctx, auth.TokenHash("cvs_op-token")); err != nil {
			t.Fatalf("%s: token revoked: %v", step, err)
		}
		if n := e.sessions(op.ID); n != 1 {
			t.Fatalf("%s: sessions = %d", step, n)
		}
	}
	body := map[string]any{"password": "New-long-passw0rd", "name": "Renamed", "role": "viewer"}
	path := fmt.Sprintf("/api/users/%d", op.ID)

	// the user update (after the password change) fails
	e.exec(`CREATE TRIGGER fail_update BEFORE UPDATE OF name ON users BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	if w, _ := e.do("PUT", path, body, withCookie(e.cookie), withCSRF()); w.Code < 400 {
		t.Fatalf("update with a failing step: %d %s", w.Code, w.Body)
	}
	unchanged("failing update")
	e.exec(`DROP TRIGGER fail_update`)

	// the session revocation fails: the request fails and nothing is applied
	e.exec(`CREATE TRIGGER fail_sessions BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	if w, _ := e.do("PUT", path, body, withCookie(e.cookie), withCSRF()); w.Code < 400 {
		t.Fatalf("update with failing session revocation: %d %s", w.Code, w.Body)
	}
	unchanged("failing session revocation")
	// MFA reset: same
	if w, _ := e.do("POST", path+"/reset-mfa", nil, withCookie(e.cookie), withCSRF()); w.Code < 400 {
		t.Fatalf("MFA reset with failing session revocation: %d %s", w.Code, w.Body)
	}
	unchanged("failing MFA reset")
	e.exec(`DROP TRIGGER fail_sessions`)

	// working again: everything applied together
	if w, _ := e.do("PUT", path, body, withCookie(e.cookie), withCSRF()); w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body)
	}
	u, _ := e.s.Store.UserByID(ctx, op.ID)
	if !auth.CheckPassword(u.PasswordHash(), "New-long-passw0rd") || u.Name != "Renamed" || u.Role != "viewer" {
		t.Fatalf("not applied: %+v", u)
	}
	if _, err := e.s.Store.APITokenByHash(ctx, auth.TokenHash("cvs_op-token")); err == nil {
		t.Fatal("token survived the password reset")
	}
	if n := e.sessions(op.ID); n != 0 {
		t.Fatalf("sessions after reset = %d", n)
	}
}

// Changing your own password fails as a whole when the other sessions cannot be ended.
func TestOwnPasswordChangeIsAtomic(t *testing.T) {
	e := newAtomicEnv(t, "Admin-long-passw0rd")
	ctx := context.Background()
	if err := e.s.Store.CreateSession(ctx, auth.TokenHash("other-device"), e.admin.ID, time.Hour, "127.0.0.1", "test"); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"current": "Admin-long-passw0rd", "new": "Changed-long-passw0rd"}
	e.exec(`CREATE TRIGGER fail_sessions BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	if w, _ := e.do("POST", "/api/auth/password", body, withCookie(e.cookie), withCSRF()); w.Code < 400 {
		t.Fatalf("change with failing session revocation: %d %s", w.Code, w.Body)
	}
	u, _ := e.s.Store.UserByID(ctx, e.admin.ID)
	if !auth.CheckPassword(u.PasswordHash(), "Admin-long-passw0rd") {
		t.Fatal("password changed although the other sessions survived")
	}
	e.exec(`DROP TRIGGER fail_sessions`)
	if w, _ := e.do("POST", "/api/auth/password", body, withCookie(e.cookie), withCSRF()); w.Code != 200 {
		t.Fatalf("change: %d %s", w.Code, w.Body)
	}
	if _, err := e.s.Store.SessionUser(ctx, auth.TokenHash("other-device")); err == nil {
		t.Fatal("the other session survived the password change")
	}
}
