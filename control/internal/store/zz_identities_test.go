package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestZZIdentityLinks(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	a, _ := s.CreateUser(ctx, "a@example.com", "A", "x", "admin")
	b, _ := s.CreateUser(ctx, "b@example.com", "B", "x", "viewer")
	const iss = "https://idp.example.com/"

	if _, err := s.UserByIdentity(ctx, iss, "sub-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlinked lookup: %v", err)
	}
	if _, err := s.IdentitySubject(ctx, a.ID, iss); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlinked subject: %v", err)
	}
	if err := s.LinkIdentity(ctx, a.ID, iss, "sub-a"); err != nil {
		t.Fatal(err)
	}
	// the issuer is compared without a trailing slash
	u, err := s.UserByIdentity(ctx, "https://idp.example.com", "sub-a")
	if err != nil || u.ID != a.ID {
		t.Fatalf("lookup: %v %+v", err, u)
	}
	if sub, err := s.IdentitySubject(ctx, a.ID, iss); err != nil || sub != "sub-a" {
		t.Fatalf("subject: %q %v", sub, err)
	}
	// a subject belongs to one user; a user has one subject per issuer
	if err := s.LinkIdentity(ctx, b.ID, iss, "sub-a"); !IsUnique(err) {
		t.Fatalf("subject linked twice: %v", err)
	}
	if err := s.LinkIdentity(ctx, a.ID, iss, "sub-other"); !IsUnique(err) {
		t.Fatalf("second subject at the same issuer: %v", err)
	}
	// another issuer is a separate link
	if err := s.LinkIdentity(ctx, a.ID, "https://other.example.com", "sub-a"); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListUsers(ctx)
	if err != nil || len(list) != 2 || !list[0].SSOLinked || list[1].SSOLinked {
		t.Fatalf("list: %v %+v", err, list)
	}
	if n, err := s.DeleteUserIdentities(ctx, a.ID); err != nil || n != 2 {
		t.Fatalf("delete: %d %v", n, err)
	}
	if _, err := s.UserByIdentity(ctx, iss, "sub-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("link survived: %v", err)
	}
	// links end with the user
	if err := s.LinkIdentity(ctx, b.ID, iss, "sub-b"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserByIdentity(ctx, iss, "sub-b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("link of a deleted user: %v", err)
	}
	if NormIssuer(" https://x.example.com// ") != "https://x.example.com" {
		t.Fatal("NormIssuer")
	}
}

// Users who signed in with SSO before identities were stored keep their link.
func TestZZBackfillIdentities(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	a, _ := s.CreateUser(ctx, "a@example.com", "A", "x", "admin")
	b, _ := s.CreateUser(ctx, "b@example.com", "B", "x", "viewer")
	gone, _ := s.CreateUser(ctx, "gone@example.com", "G", "x", "viewer")
	login := func(uid int64, detail any) {
		t.Helper()
		if err := s.Audit(ctx, &uid, "auth.sso_login", "", detail, ""); err != nil {
			t.Fatal(err)
		}
	}
	login(b.ID, map[string]any{"issuer": "https://idp.example.com", "sub": "new-a"}) // older: subject taken by a
	login(a.ID, map[string]any{"issuer": "https://idp.example.com/", "sub": "old-a"})
	login(a.ID, map[string]any{"issuer": "https://idp.example.com", "sub": "new-a"}) // newest wins
	login(b.ID, map[string]any{"issuer": "", "sub": "x"})
	login(b.ID, map[string]any{"issuer": "https://idp.example.com", "sub": strings.Repeat("s", MaxSubject+1)})
	login(b.ID, "not an object")
	login(gone.ID, map[string]any{"issuer": "https://idp.example.com", "sub": "gone"})
	if err := s.Audit(ctx, &b.ID, "auth.login", "", map[string]any{"issuer": "https://idp.example.com", "sub": "pw"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.WithTx(ctx, func(tx *Store) error { return backfillIdentities(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
	if sub, err := s.IdentitySubject(ctx, a.ID, "https://idp.example.com"); err != nil || sub != "new-a" {
		t.Fatalf("a: %q %v", sub, err)
	}
	if sub, err := s.IdentitySubject(ctx, b.ID, "https://idp.example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("b linked: %q %v", sub, err)
	}
	// running it again changes nothing
	if err := s.WithTx(ctx, func(tx *Store) error { return backfillIdentities(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
}

func TestZZAuditLimitsAndPrune(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	long := strings.Repeat("é", 400) // 800 bytes
	if err := s.Audit(ctx, nil, "auth.login_failed", long, map[string]string{"blob": strings.Repeat("\x01", 8000)}, ""); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListAudit(ctx, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	e := list[0]
	if len(e.Target) > AuditTargetMax || !strings.HasSuffix(e.Target, "…") || !strings.HasPrefix(e.Target, "éé") {
		t.Fatalf("target: %d bytes %q", len(e.Target), e.Target[:10])
	}
	var d map[string]any
	if len(e.Detail) > AuditDetailMax || json.Unmarshal(e.Detail, &d) != nil || d["truncated"] != true {
		t.Fatalf("detail: %d bytes %s", len(e.Detail), e.Detail[:40])
	}
	if truncUTF8("short", 10) != "short" || auditJSON(map[string]int{"a": 1}) != `{"a":1}` {
		t.Fatal("small values must be kept")
	}
	// a large host entry (about 20 KB) is kept whole
	host := map[string]string{"advanced_config": strings.Repeat("proxy_set_header X-A b;\n", 800)}
	if j := auditJSON(host); j != toJSON(host) || len(j) < 16<<10 {
		t.Fatalf("20 KB detail cut: %d bytes", len(j))
	}

	// retention by age
	old := now().Add(-400 * 24 * time.Hour)
	if _, err := s.q.ExecContext(ctx, `INSERT INTO audit_log(ts, action, target, detail, ip) VALUES (?, 'x', '', '{}', '')`, old); err != nil {
		t.Fatal(err)
	}
	if n, err := s.PruneAudit(ctx, time.Now().Add(-AuditRetention), AuditMaxRows); err != nil || n != 1 {
		t.Fatalf("age prune: %d %v", n, err)
	}
	// row cap: only the newest entries stay
	for i := 0; i < 5; i++ {
		_ = s.Audit(ctx, nil, "x", "", nil, "")
	}
	if n, err := s.PruneAudit(ctx, time.Now().Add(-AuditRetention), 2); err != nil || n != 4 {
		t.Fatalf("cap prune: %d %v", n, err)
	}
	if list, _ := s.ListAudit(ctx, 10); len(list) != 2 {
		t.Fatalf("kept %d", len(list))
	}
	if n, err := s.PruneAudit(ctx, time.Now().Add(-AuditRetention), 2); err != nil || n != 0 {
		t.Fatalf("nothing to prune: %d %v", n, err)
	}
}
