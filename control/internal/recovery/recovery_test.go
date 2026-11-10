package recovery

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
)

func open(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func user(t *testing.T, st *store.Store, email, role string) *store.User {
	t.Helper()
	h, _ := auth.HashPassword("old-password-123")
	u, err := st.CreateUser(context.Background(), email, "", h, role)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestResetOnlyAdmin(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	admin := user(t, st, "admin@example.com", "admin")
	user(t, st, "op@example.com", "operator")
	if err := st.CreateSession(ctx, "sess", admin.ID, time.Hour, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAPIToken(ctx, "ci", "cvs_x", "tokhash", "admin", admin.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.SetTOTPSecret(ctx, admin.ID, "enc"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetMFA(ctx, admin.ID, true, []string{"r1"}); err != nil {
		t.Fatal(err)
	}

	res, err := ResetAdmin(ctx, st, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Email != "admin@example.com" || len(res.Password) < auth.MinPassword || !res.MFADisabled || res.TokensDeleted != 1 {
		t.Fatalf("result: %+v", res)
	}
	u, _ := st.UserByID(ctx, admin.ID)
	if !auth.CheckPassword(u.PasswordHash(), res.Password) || auth.CheckPassword(u.PasswordHash(), "old-password-123") {
		t.Fatal("password not replaced")
	}
	if u.MFAEnabled {
		t.Fatal("two-step verification still on")
	}
	if _, err := st.SessionUser(ctx, "sess"); err == nil {
		t.Fatal("session survived")
	}
	if toks, _ := st.ListAPITokens(ctx); len(toks) != 0 {
		t.Fatalf("tokens survived: %d", len(toks))
	}
	log, _ := st.ListAudit(ctx, 10)
	if len(log) == 0 || log[0].Action != "auth.reset_local" || log[0].Target != "admin@example.com" || log[0].IP != "local" {
		t.Fatalf("audit: %+v", log)
	}
}

func TestResetKeepsMFAWithoutFlag(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	admin := user(t, st, "admin@example.com", "admin")
	_ = st.SetMFA(ctx, admin.ID, true, nil)
	res, err := ResetAdmin(ctx, st, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := st.UserByID(ctx, admin.ID); !u.MFAEnabled || res.MFADisabled {
		t.Fatal("two-step verification changed without --disable-mfa")
	}
}

func TestResetChoosesAccount(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	if _, err := ResetAdmin(ctx, st, "", false); err == nil || !strings.Contains(err.Error(), "no active administrator") {
		t.Fatalf("empty database: %v", err)
	}
	user(t, st, "a@example.com", "admin")
	b := user(t, st, "b@example.com", "admin")
	if _, err := ResetAdmin(ctx, st, "", false); err == nil || !strings.Contains(err.Error(), "a@example.com, b@example.com") {
		t.Fatalf("several administrators: %v", err)
	}
	res, err := ResetAdmin(ctx, st, " B@Example.com ", false)
	if err != nil || res.Email != "b@example.com" {
		t.Fatalf("by e-mail: %+v %v", res, err)
	}
	if _, err := ResetAdmin(ctx, st, "nobody@example.com", false); err == nil || !strings.Contains(err.Error(), "no account") {
		t.Fatalf("unknown e-mail: %v", err)
	}
	if err := st.UpdateUser(ctx, b.ID, "", "admin", true); err != nil {
		t.Fatal(err)
	}
	if _, err := ResetAdmin(ctx, st, "b@example.com", false); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled account: %v", err)
	}
	// the disabled administrator does not count: the other one is chosen
	if res, err := ResetAdmin(ctx, st, "", false); err != nil || res.Email != "a@example.com" {
		t.Fatalf("only active administrator: %+v %v", res, err)
	}
}
