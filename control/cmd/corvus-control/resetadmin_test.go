package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
)

func TestResetAdminCommand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CORVUS_DATA_DIR", dir)
	t.Setenv("CORVUS_DB_URL", "")
	var out, errOut bytes.Buffer

	// no database yet
	if code := resetAdmin(nil, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "no database") {
		t.Fatalf("missing database: %d %q", code, errOut.String())
	}
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	h, _ := auth.HashPassword("old-password-123")
	if _, err := st.CreateUser(context.Background(), "admin@example.com", "", h, "admin"); err != nil {
		t.Fatal(err)
	}
	st.Close()

	out.Reset()
	errOut.Reset()
	if code := resetAdmin([]string{"--disable-mfa"}, &out, &errOut); code != 0 {
		t.Fatalf("reset: %d %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Account:      admin@example.com") || !strings.Contains(out.String(), "New password: ") {
		t.Fatalf("output: %q", out.String())
	}
	pw := strings.TrimSpace(strings.SplitN(strings.SplitN(out.String(), "New password: ", 2)[1], "\n", 2)[0])
	st, _ = store.Open(filepath.Join(dir, "corvus.db"))
	defer st.Close()
	u, _ := st.UserByEmail(context.Background(), "admin@example.com")
	if !auth.CheckPassword(u.PasswordHash(), pw) {
		t.Fatal("printed password does not sign in")
	}

	errOut.Reset()
	if code := resetAdmin([]string{"--email", "nobody@example.com"}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "no account") {
		t.Fatalf("unknown account: %d %q", code, errOut.String())
	}
	errOut.Reset()
	if code := resetAdmin([]string{"extra"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "usage:") {
		t.Fatalf("stray argument: %d %q", code, errOut.String())
	}
	if code := resetAdmin([]string{"--bogus"}, &out, &errOut); code != 2 {
		t.Fatalf("unknown flag: %d", code)
	}
	if !isPostgres("postgres://x") || !isPostgres("postgresql://x") || isPostgres("/data/corvus.db") {
		t.Fatal("isPostgres")
	}
}
