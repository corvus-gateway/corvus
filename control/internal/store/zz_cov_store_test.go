package store

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// pgOverSQLite returns a Store that goes through the PostgreSQL query rewriting layer
// ($n placeholders, RETURNING id) while still being backed by SQLite, which accepts
// both. It exercises the dialect code paths without a PostgreSQL server.
func pgOverSQLite(t *testing.T) *Store {
	t.Helper()
	s := openTest(t)
	if s.dialect != SQLite {
		t.Skip("needs the SQLite backend")
	}
	return &Store{db: s.db, q: pgQuerier{s.db}, dialect: Postgres}
}

func TestCovRebind(t *testing.T) {
	cases := map[string]string{
		`SELECT 1`:                                  `SELECT 1`,
		`SELECT * FROM t WHERE a = ? AND b = ?`:     `SELECT * FROM t WHERE a = $1 AND b = $2`,
		`UPDATE t SET v = '?' WHERE id = ?`:         `UPDATE t SET v = '?' WHERE id = $1`,
		`INSERT INTO t(a,b) VALUES (?, 'x?y', ?)`:   `INSERT INTO t(a,b) VALUES ($1, 'x?y', $2)`,
		`SELECT '' , ? FROM t`:                      `SELECT '' , $1 FROM t`,
		`?,?,?,?,?,?,?,?,?,?,?`:                     `$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11`,
		`UPDATE x SET allow = '[]', deny = ? WHERE`: `UPDATE x SET allow = '[]', deny = $1 WHERE`,
	}
	for in, want := range cases {
		if got := rebind(in); got != want {
			t.Errorf("rebind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCovInsertTable(t *testing.T) {
	cases := map[string]string{
		`INSERT INTO users(email) VALUES (?)`:       "users",
		"  insert into proxy_hosts (a) VALUES (?)":  "proxy_hosts",
		`INSERT INTO meta(key, value) VALUES (?,?)`: "meta",
		`UPDATE users SET a = 1`:                    "",
		`INSERT`:                                    "",
		`INSERT INTO lonelytable`:                   "",
		`DELETE FROM users WHERE id = ?`:            "",
	}
	for in, want := range cases {
		if got := insertTable(in); got != want {
			t.Errorf("insertTable(%q) = %q, want %q", in, got, want)
		}
	}
	r := idResult{id: 7, affected: 1}
	if id, err := r.LastInsertId(); id != 7 || err != nil {
		t.Fatalf("LastInsertId = %d, %v", id, err)
	}
	if n, err := r.RowsAffected(); n != 1 || err != nil {
		t.Fatalf("RowsAffected = %d, %v", n, err)
	}
}

func TestCovMigrationSQLAndIsUnique(t *testing.T) {
	body := "CREATE TABLE x (id INTEGER PRIMARY KEY AUTOINCREMENT, v TEXT)"
	if got := migrationSQL(SQLite, body); got != body {
		t.Fatalf("sqlite migration must be untouched: %q", got)
	}
	if got := migrationSQL(Postgres, body); got != "CREATE TABLE x (id BIGSERIAL PRIMARY KEY, v TEXT)" {
		t.Fatalf("postgres migration: %q", got)
	}
	if IsUnique(nil) {
		t.Fatal("nil is not a unique violation")
	}
	for _, m := range []string{"UNIQUE constraint failed: users.email", `duplicate key value violates unique constraint "x"`, "ERROR (SQLSTATE 23505)"} {
		if !IsUnique(errors.New(m)) {
			t.Errorf("IsUnique(%q) = false", m)
		}
	}
	if IsUnique(errors.New("syntax error")) {
		t.Fatal("other errors are not unique violations")
	}
	if (&Store{dialect: Postgres}).wrap(nil) == nil {
		t.Fatal("postgres wrap returns a querier")
	}
	if _, ok := (&Store{dialect: Postgres}).wrap(nil).(pgQuerier); !ok {
		t.Fatal("postgres wrap must rewrite queries")
	}
}

func TestCovRolesAndTokenExpiry(t *testing.T) {
	for _, r := range []string{"viewer", "operator", "admin"} {
		if !ValidRole(r) {
			t.Errorf("%s should be valid", r)
		}
	}
	if ValidRole("root") || ValidRole("") {
		t.Fatal("unknown roles are invalid")
	}
	if !RoleAtLeast("admin", "operator") || !RoleAtLeast("operator", "operator") || RoleAtLeast("viewer", "operator") {
		t.Fatal("role ranking")
	}
	if RoleAtLeast("admin", "bogus") {
		t.Fatal("an unknown minimum role never matches")
	}
	now := time.Now()
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	if (&APIToken{}).Expired(now) {
		t.Fatal("no expiry never expires")
	}
	if !(&APIToken{ExpiresAt: &past}).Expired(now) || (&APIToken{ExpiresAt: &future}).Expired(now) {
		t.Fatal("expiry comparison")
	}
	if !(&APIToken{ExpiresAt: &now}).Expired(now) {
		t.Fatal("a token is expired at its expiry instant")
	}
}

func TestCovOpenErrors(t *testing.T) {
	// A directory that does not exist cannot hold the database file.
	if _, err := Open(filepath.Join(t.TempDir(), "missing", "dir", "corvus.db")); err == nil {
		t.Fatal("open in a missing directory must fail")
	}
	// Reopening an existing database skips applied migrations and keeps data.
	p := filepath.Join(t.TempDir(), "c.db")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.SetMeta(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, _ := s.GetMeta(ctx, "k"); v != "v" {
		t.Fatalf("meta after reopen = %q", v)
	}
	if s.Dialect() != SQLite {
		t.Fatal("dialect")
	}
}

func TestCovSnapshot(t *testing.T) {
	s := openTest(t)
	if s.dialect != SQLite {
		t.Skip("sqlite only")
	}
	ctx := context.Background()
	if err := s.SetMeta(ctx, "snap", "yes"); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "snap.db")
	if err := s.SnapshotTo(ctx, dst); err != nil {
		t.Fatal(err)
	}
	c, err := Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if v, _ := c.GetMeta(ctx, "snap"); v != "yes" {
		t.Fatalf("snapshot content = %q", v)
	}
	if err := s.SnapshotTo(ctx, dst); err == nil {
		t.Fatal("snapshot onto an existing file must fail")
	}
	if err := (&Store{dialect: Postgres}).SnapshotTo(ctx, dst); err == nil || !strings.Contains(err.Error(), "pg_dump") {
		t.Fatalf("postgres snapshot error = %v", err)
	}
}

func TestCovSuiteSQLite(t *testing.T)    { covSuite(t, openTest(t)) }
func TestCovSuitePgDialect(t *testing.T) { covSuite(t, pgOverSQLite(t)) }

// covSuite exercises the CRUD surface of the store that the older tests leave out.
func covSuite(t *testing.T, s *Store) {
	ctx := context.Background()

	// ---------------------------------------------------------------- users & sessions
	u, err := s.CreateUser(ctx, "Admin@Example.com", "Admin", "hash1", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "admin@example.com" || u.PasswordHash() != "hash1" || u.TOTPSecret() != "" || u.LastLoginAt != nil {
		t.Fatalf("created user: %+v", u)
	}
	if _, err := s.CreateUser(ctx, "admin@example.com", "dup", "h", "admin"); !IsUnique(err) {
		t.Fatalf("duplicate email err = %v", err)
	}
	if n, err := s.CountUsers(ctx); err != nil || n != 1 {
		t.Fatalf("CountUsers = %d, %v", n, err)
	}
	if got, err := s.UserByEmail(ctx, "ADMIN@example.com"); err != nil || got.ID != u.ID {
		t.Fatalf("UserByEmail: %v %v", got, err)
	}
	if _, err := s.UserByEmail(ctx, "nobody@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user err = %v", err)
	}
	if err := s.TouchLogin(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePassword(ctx, u.ID, "hash2"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTOTPSecret(ctx, u.ID, "enc-secret"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.UserByID(ctx, u.ID)
	if got.LastLoginAt == nil || got.PasswordHash() != "hash2" || got.TOTPSecret() != "enc-secret" {
		t.Fatalf("after updates: %+v hash=%s", got, got.PasswordHash())
	}
	if err := s.SetMFA(ctx, u.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.UserByID(ctx, u.ID); !got.MFAEnabled || got.RecoveryCodes() != "[]" || got.TOTPSecret() != "enc-secret" {
		t.Fatalf("mfa on: %+v codes=%q", got, got.RecoveryCodes())
	}
	if err := s.SetMFA(ctx, u.ID, false, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.UserByID(ctx, u.ID); got.MFAEnabled || got.TOTPSecret() != "" {
		t.Fatalf("disabling MFA must clear the secret: %+v", got)
	}
	if err := s.SetRecoveryCodes(ctx, u.ID, []string{"x", "y"}); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ClaimTOTPStep(ctx, u.ID, 100); !ok || err != nil {
		t.Fatalf("first claim: %v %v", ok, err)
	}
	if ok, _ := s.ClaimTOTPStep(ctx, u.ID, 100); ok {
		t.Fatal("replayed step must be rejected")
	}
	if ok, _ := s.ClaimTOTPStep(ctx, u.ID, 99); ok {
		t.Fatal("older step must be rejected")
	}

	v, err := s.CreateUser(ctx, "viewer@example.com", "V", "h", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	users, err := s.ListUsers(ctx)
	if err != nil || len(users) != 2 || users[0].ID != u.ID || users[1].Role != "viewer" {
		t.Fatalf("ListUsers: %+v %v", users, err)
	}
	if err := s.UpdateUser(ctx, v.ID, "Viewer", "operator", true); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.UserByID(ctx, v.ID); got.Name != "Viewer" || got.Role != "operator" || !got.Disabled {
		t.Fatalf("UpdateUser: %+v", got)
	}
	if err := s.UpdateUser(ctx, 9999, "x", "admin", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateUser missing = %v", err)
	}
	if n, _ := s.CountActiveAdmins(ctx, 0); n != 1 {
		t.Fatalf("CountActiveAdmins = %d", n)
	}
	if n, _ := s.CountActiveAdmins(ctx, u.ID); n != 0 {
		t.Fatalf("CountActiveAdmins excluding = %d", n)
	}
	if s.dialect == SQLite {
		err = s.KeepingAdmin(ctx, func(tx *Store) error { return tx.UpdateUser(ctx, u.ID, "A", "viewer", false) })
		if !errors.Is(err, ErrLastAdmin) {
			t.Fatalf("demoting the last admin = %v", err)
		}
		if got, _ := s.UserByID(ctx, u.ID); got.Role != "admin" {
			t.Fatal("the demotion must be rolled back")
		}
		boom := errors.New("boom")
		if err := s.KeepingAdmin(ctx, func(tx *Store) error { return boom }); !errors.Is(err, boom) {
			t.Fatalf("fn error = %v", err)
		}
		if err := s.KeepingAdmin(ctx, func(tx *Store) error { return tx.UpdateUser(ctx, v.ID, "Viewer", "viewer", false) }); err != nil {
			t.Fatal(err)
		}
	}

	// sessions
	if err := s.CreateSession(ctx, "tok-live", u.ID, time.Hour, "1.2.3.4", "ua"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, "tok-dead", u.ID, -time.Hour, "1.2.3.4", "ua"); err != nil {
		t.Fatal(err)
	}
	if su, err := s.SessionUser(ctx, "tok-live"); err != nil || su.ID != u.ID {
		t.Fatalf("SessionUser: %v %v", su, err)
	}
	if _, err := s.SessionUser(ctx, "tok-dead"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session = %v", err)
	}
	if err := s.PurgeExpiredSessions(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&n)
	if n != 1 {
		t.Fatalf("sessions after purge = %d", n)
	}
	if err := s.DeleteSession(ctx, "tok-live"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionUser(ctx, "tok-live"); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted session must be gone")
	}
	_ = s.CreateSession(ctx, "a", u.ID, time.Hour, "", "")
	_ = s.CreateSession(ctx, "b", u.ID, time.Hour, "", "")
	if err := s.DeleteUserSessions(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	_ = s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&n)
	if n != 0 {
		t.Fatalf("sessions after DeleteUserSessions = %d", n)
	}

	// ---------------------------------------------------------------- API tokens
	exp := time.Now().Add(48 * time.Hour)
	tok, err := s.CreateAPIToken(ctx, "ci", "cvs_abcd", "hash-a", "operator", u.ID, &exp)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Name != "ci" || tok.CreatorEmail != "admin@example.com" || tok.ExpiresAt == nil || tok.LastUsedAt != nil || tok.Role != "operator" {
		t.Fatalf("token: %+v", tok)
	}
	if !tok.ExpiresAt.Equal(exp.UTC().Truncate(time.Second)) {
		t.Fatalf("expiry %v vs %v", tok.ExpiresAt, exp)
	}
	tok2, err := s.CreateAPIToken(ctx, "prom", "cvs_efgh", "hash-b", "viewer", v.ID, nil)
	if err != nil || tok2.ExpiresAt != nil {
		t.Fatalf("token2: %+v %v", tok2, err)
	}
	if _, err := s.CreateAPIToken(ctx, "dup", "cvs_x", "hash-a", "viewer", u.ID, nil); !IsUnique(err) {
		t.Fatalf("duplicate hash = %v", err)
	}
	if got, err := s.APITokenByHash(ctx, "hash-b"); err != nil || got.ID != tok2.ID {
		t.Fatalf("by hash: %v %v", got, err)
	}
	if _, err := s.APITokenByHash(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing hash = %v", err)
	}
	if err := s.TouchAPIToken(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	got1, _ := s.APITokenByID(ctx, tok.ID)
	if got1.LastUsedAt == nil {
		t.Fatal("touch must record last use")
	}
	first := *got1.LastUsedAt
	_ = s.TouchAPIToken(ctx, tok.ID)
	if got1, _ = s.APITokenByID(ctx, tok.ID); !got1.LastUsedAt.Equal(first) {
		t.Fatal("touch is throttled to once a minute")
	}
	toks, err := s.ListAPITokens(ctx)
	if err != nil || len(toks) != 2 || toks[0].ID != tok.ID {
		t.Fatalf("ListAPITokens: %+v %v", toks, err)
	}
	if err := s.DeleteAPIToken(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAPIToken(ctx, tok.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
	if c, err := s.DeleteUserAPITokens(ctx, v.ID); err != nil || c != 1 {
		t.Fatalf("DeleteUserAPITokens = %d, %v", c, err)
	}
	if c, _ := s.DeleteUserAPITokens(ctx, v.ID); c != 0 {
		t.Fatalf("nothing left = %d", c)
	}

	// ---------------------------------------------------------------- bans
	t0 := time.Now()
	in1h := t0.Add(time.Hour)
	b := &IPBan{IP: "203.0.113.5", Reason: "scan", Source: "auto", Offenses: 1, ExpiresAt: &in1h}
	if err := s.UpsertBan(ctx, b); err != nil || b.ID == 0 {
		t.Fatalf("UpsertBan new: %v id=%d", err, b.ID)
	}
	firstID := b.ID
	b2 := &IPBan{IP: "203.0.113.5", Reason: "again", Source: "manual", Offenses: 2, ExpiresAt: &in1h, CreatedBy: &u.ID}
	if err := s.UpsertBan(ctx, b2); err != nil || b2.ID != firstID {
		t.Fatalf("UpsertBan renew: %v id=%d want %d", err, b2.ID, firstID)
	}
	perm := &IPBan{IP: "198.51.100.9", Reason: "manual", Source: "manual", Offenses: 1}
	if err := s.UpsertBan(ctx, perm); err != nil {
		t.Fatal(err)
	}
	bb, err := s.BanByIP(ctx, "203.0.113.5")
	if err != nil || bb.Reason != "again" || bb.Offenses != 2 || bb.CreatedBy == nil || *bb.CreatedBy != u.ID || bb.ExpiresAt == nil {
		t.Fatalf("BanByIP: %+v %v", bb, err)
	}
	if _, err := s.BanByIP(ctx, "192.0.2.1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing ban = %v", err)
	}
	active, err := s.ActiveBans(ctx, t0)
	if err != nil || len(active) != 2 {
		t.Fatalf("ActiveBans: %+v %v", active, err)
	}
	if lifted, err := s.LiftBan(ctx, firstID); err != nil || lifted.IP != "203.0.113.5" {
		t.Fatalf("LiftBan: %+v %v", lifted, err)
	}
	if _, err := s.LiftBan(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lift missing = %v", err)
	}
	active, _ = s.ActiveBans(ctx, time.Now())
	if len(active) != 1 || active[0].IP != "198.51.100.9" || active[0].ExpiresAt != nil {
		t.Fatalf("after lift: %+v", active)
	}
	if pb, err := s.MakeBanPermanent(ctx, firstID); err != nil || pb.ExpiresAt != nil {
		t.Fatalf("MakeBanPermanent: %+v %v", pb, err)
	}
	if _, err := s.MakeBanPermanent(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("permanent missing = %v", err)
	}
	if active, _ = s.ActiveBans(ctx, time.Now()); len(active) != 2 {
		t.Fatalf("permanent ban must be active: %+v", active)
	}
	old := t0.Add(-48 * time.Hour)
	oldBan := &IPBan{IP: "192.0.2.77", Source: "auto", Offenses: 1, ExpiresAt: &old}
	_ = s.UpsertBan(ctx, oldBan)
	if err := s.PruneBans(ctx, t0.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BanByIP(ctx, "192.0.2.77"); !errors.Is(err, ErrNotFound) {
		t.Fatal("old expired ban must be pruned")
	}
	if _, err := s.BanByIP(ctx, "198.51.100.9"); err != nil {
		t.Fatal("permanent ban must survive pruning")
	}

	// ---------------------------------------------------------------- alerts
	ch := &AlertChannel{Name: "ops", Kind: "webhook", Target: "sealed", TargetHint: "https://h…", Enabled: true, MinSeverity: "warning"}
	if err := s.SaveAlertChannel(ctx, ch); err != nil || ch.ID == 0 || ch.Events == nil {
		t.Fatalf("SaveAlertChannel: %+v %v", ch, err)
	}
	ch.Events = []string{"deploy_failed"}
	ch.Enabled = false
	if err := s.SaveAlertChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}
	chs, err := s.ListAlertChannels(ctx)
	if err != nil || len(chs) != 1 || chs[0].Enabled || len(chs[0].Events) != 1 || chs[0].Target != "sealed" {
		t.Fatalf("ListAlertChannels: %+v %v", chs, err)
	}
	if c, err := s.AlertChannelByID(ctx, ch.ID); err != nil || c.Name != "ops" {
		t.Fatalf("AlertChannelByID: %v %v", c, err)
	}
	if _, err := s.AlertChannelByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing channel = %v", err)
	}
	if err := s.SaveAlertChannel(ctx, &AlertChannel{ID: 999, Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing channel = %v", err)
	}
	// a channel saved with a corrupt events column still lists with an empty slice
	_, _ = s.q.ExecContext(ctx, `UPDATE alert_channels SET events = 'null' WHERE id = ?`, ch.ID)
	if chs, _ = s.ListAlertChannels(ctx); chs[0].Events == nil {
		t.Fatal("events must never be nil")
	}
	if err := s.DeleteAlertChannel(ctx, ch.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAlertChannel(ctx, ch.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice = %v", err)
	}
	a1 := &AlertRecord{Rule: "deploy", Severity: "critical", Status: "firing", Key: "k", Title: "T", Message: "M"}
	if err := s.AddAlert(ctx, a1); err != nil || a1.ID == 0 || a1.Results == nil {
		t.Fatalf("AddAlert: %+v %v", a1, err)
	}
	a2 := &AlertRecord{Rule: "cert", Severity: "warning", Status: "resolved", Results: []DeliveryResult{{ChannelID: 1, Channel: "ops", OK: false, Error: "timeout"}}}
	_ = s.AddAlert(ctx, a2)
	al, err := s.ListAlerts(ctx, 10)
	if err != nil || len(al) != 2 || al[0].ID != a2.ID || len(al[0].Results) != 1 || al[0].Results[0].Error != "timeout" {
		t.Fatalf("ListAlerts: %+v %v", al, err)
	}
	if al, _ = s.ListAlerts(ctx, 1); len(al) != 1 {
		t.Fatal("limit")
	}

	// audit counting
	_ = s.Audit(ctx, &u.ID, "login.failed", "x", nil, "1.1.1.1")
	_ = s.Audit(ctx, nil, "login.failed", "y", map[string]int{"n": 1}, "")
	_ = s.Audit(ctx, nil, "other", "z", nil, "")
	if c, err := s.CountAudit(ctx, "login.failed", time.Now().Add(-time.Minute)); err != nil || c != 2 {
		t.Fatalf("CountAudit = %d, %v", c, err)
	}
	if c, _ := s.CountAudit(ctx, "login.failed", time.Now().Add(time.Hour)); c != 0 {
		t.Fatalf("CountAudit future = %d", c)
	}

	// ---------------------------------------------------------------- versions
	if id, err := s.MaxVersionID(ctx); err != nil || id != 0 {
		t.Fatalf("MaxVersionID empty = %d, %v", id, err)
	}
	if c, _ := s.CountVersions(ctx); c != 0 {
		t.Fatal("no versions yet")
	}
	v1 := &ConfigVersion{CreatedBy: &u.ID, Message: "first", ConfigRev: 1}
	if err := s.CreateVersion(ctx, v1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetVersionDir(ctx, v1.ID, "/v/1"); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishVersion(ctx, v1.ID, true, "ok"); err != nil {
		t.Fatal(err)
	}
	v2 := &ConfigVersion{Message: "bad", Dir: "/v/2"}
	_ = s.CreateVersion(ctx, v2)
	_ = s.FinishVersion(ctx, v2.ID, false, "nginx: [emerg]")
	v3 := &ConfigVersion{Message: "rollback", RollbackOf: &v1.ID, Dir: "/v/3"}
	_ = s.CreateVersion(ctx, v3)
	_ = s.FinishVersion(ctx, v3.ID, true, "ok")
	if id, _ := s.MaxVersionID(ctx); id != v3.ID {
		t.Fatalf("MaxVersionID = %d", id)
	}
	if c, _ := s.CountVersions(ctx); c != 3 {
		t.Fatalf("CountVersions = %d", c)
	}
	got3, err := s.VersionByID(ctx, v3.ID)
	if err != nil || got3.Status != "active" || got3.RollbackOf == nil || *got3.RollbackOf != v1.ID || got3.CreatedBy != nil {
		t.Fatalf("VersionByID: %+v %v", got3, err)
	}
	got1v, _ := s.VersionByID(ctx, v1.ID)
	if got1v.Status != "superseded" || got1v.Email != "admin@example.com" || got1v.Dir != "/v/1" || got1v.CreatedBy == nil {
		t.Fatalf("v1: %+v", got1v)
	}
	if _, err := s.VersionByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing version = %v", err)
	}
	list, err := s.ListVersions(ctx, 2, 0)
	if err != nil || len(list) != 2 || list[0].ID != v3.ID || list[1].ID != v2.ID {
		t.Fatalf("ListVersions: %+v %v", list, err)
	}
	if list, _ = s.ListVersions(ctx, 10, 2); len(list) != 1 || list[0].ID != v1.ID {
		t.Fatalf("ListVersions offset: %+v", list)
	}
	failed, err := s.FailedVersionsSince(ctx, 0)
	if err != nil || len(failed) != 1 || failed[0].ID != v2.ID || failed[0].TestOutput != "nginx: [emerg]" {
		t.Fatalf("FailedVersionsSince: %+v %v", failed, err)
	}
	if failed, _ = s.FailedVersionsSince(ctx, v2.ID); len(failed) != 0 {
		t.Fatal("no failures after v2")
	}
	dirs, err := s.VersionDirsInUse(ctx, 1)
	if err != nil || !dirs["/v/3"] || dirs["/v/2"] {
		t.Fatalf("VersionDirsInUse(1): %v %v", dirs, err)
	}
	if dirs, _ = s.VersionDirsInUse(ctx, 5); !dirs["/v/1"] || !dirs["/v/3"] || dirs["/v/2"] {
		t.Fatalf("VersionDirsInUse(5): %v", dirs)
	}
	if av, err := s.ActiveVersion(ctx); err != nil || av.ID != v3.ID {
		t.Fatalf("ActiveVersion: %v %v", av, err)
	}

	// ---------------------------------------------------------------- waitrooms
	w := &Waitroom{Name: "sale", Enabled: true, Capacity: 100, ActiveTTL: 60, GrantTTL: 30, QueueTTL: 600, Refresh: 5, AdmitRate: 10, FailMode: "open", MaxQueue: 1000, MaxNewIP: 3, BindIP: true}
	if err := s.SaveWaitroom(ctx, w); err != nil {
		t.Fatal(err)
	}
	w.Capacity = 50
	if err := s.SaveWaitroom(ctx, w); err != nil {
		t.Fatal(err)
	}
	gw, err := s.WaitroomByID(ctx, w.ID)
	if err != nil || gw.Capacity != 50 || !gw.BindIP || !gw.Enabled {
		t.Fatalf("WaitroomByID: %+v %v", gw, err)
	}
	if _, err := s.WaitroomByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing waitroom = %v", err)
	}
	if err := s.SaveWaitroom(ctx, &Waitroom{ID: 999, Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing waitroom = %v", err)
	}

	// ---------------------------------------------------------------- rate limits, access lists, hosts
	rl := &RateLimit{Name: "api", RequestsPerSecond: 10, Burst: 20, StatusCode: 429, Enabled: true, ExemptCIDRs: []string{"10.0.0.0/8"}}
	if err := s.SaveRateLimit(ctx, rl); err != nil {
		t.Fatal(err)
	}
	rl.DryRun = true
	if err := s.SaveRateLimit(ctx, rl); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRateLimit(ctx, &RateLimit{ID: 999, Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing rate limit = %v", err)
	}
	grl, err := s.RateLimitByID(ctx, rl.ID)
	if err != nil || !grl.DryRun || grl.ExemptCIDRs[0] != "10.0.0.0/8" {
		t.Fatalf("RateLimitByID: %+v %v", grl, err)
	}
	if _, err := s.RateLimitByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing rate limit = %v", err)
	}
	acl := &AccessList{Name: "staff", Satisfy: "all", Users: []AccessListUser{{Username: "bob", PasswordHash: "h"}}}
	if err := s.SaveAccessList(ctx, acl); err != nil {
		t.Fatal(err)
	}
	acl.Users = []AccessListUser{{Username: "carol", PasswordHash: "h2"}, {Username: "alice", PasswordHash: "h1"}}
	if err := s.SaveAccessList(ctx, acl); err != nil {
		t.Fatal(err)
	}
	gacl, err := s.AccessListByID(ctx, acl.ID)
	if err != nil || len(gacl.Users) != 2 || gacl.Users[0].Username != "alice" || gacl.Allow == nil || gacl.Deny == nil {
		t.Fatalf("AccessListByID: %+v %v", gacl, err)
	}
	if _, err := s.AccessListByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing access list = %v", err)
	}
	if err := s.SaveAccessList(ctx, &AccessList{ID: 999, Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing access list = %v", err)
	}

	cert := &Certificate{Name: "c", Kind: "custom", Domains: []string{"a.example.com"}, NotAfter: time.Now().Add(time.Hour)}
	if err := s.CreateCertificate(ctx, cert); err != nil {
		t.Fatal(err)
	}
	acmeCert := &Certificate{Name: "le", Kind: "acme", Domains: []string{"b.example.com"}, Status: "pending", ACME: ACMEInfo{Challenge: "http-01", Email: "x@example.com"}}
	if err := s.CreateCertificate(ctx, acmeCert); err != nil {
		t.Fatal(err)
	}
	h := &ProxyHost{Domains: []string{"a.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, Enabled: true, ClientIP: "direct",
		CertificateID: &cert.ID, WaitroomID: &w.ID, RateLimitID: &rl.ID, AccessListID: &acl.ID}
	if err := s.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	gh, err := s.ProxyHostByID(ctx, h.ID)
	if err != nil || gh.WaitroomID == nil || gh.RateLimitID == nil || gh.AccessListID == nil || gh.CertificateID == nil {
		t.Fatalf("ProxyHostByID: %+v %v", gh, err)
	}
	if _, err := s.ProxyHostByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing proxy host = %v", err)
	}
	if err := s.SaveProxyHost(ctx, &ProxyHost{ID: 999}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing proxy host = %v", err)
	}
	if rls, _ := s.ListRateLimits(ctx); rls[0].Hosts != 1 {
		t.Fatalf("rate limit host count = %d", rls[0].Hosts)
	}
	for name, del := range map[string]func() error{
		"waitroom":    func() error { return s.DeleteWaitroom(ctx, w.ID) },
		"rate limit":  func() error { return s.DeleteRateLimit(ctx, rl.ID) },
		"access list": func() error { return s.DeleteAccessList(ctx, acl.ID) },
		"certificate": func() error { return s.DeleteCertificate(ctx, cert.ID) },
	} {
		if err := del(); err == nil || !strings.Contains(err.Error(), "used by 1 proxy host") {
			t.Errorf("deleting an in-use %s = %v", name, err)
		}
	}
	if err := s.DeleteProxyHost(ctx, h.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProxyHost(ctx, h.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete proxy host twice = %v", err)
	}
	for name, del := range map[string]func() error{
		"waitroom":    func() error { return s.DeleteWaitroom(ctx, w.ID) },
		"rate limit":  func() error { return s.DeleteRateLimit(ctx, rl.ID) },
		"access list": func() error { return s.DeleteAccessList(ctx, acl.ID) },
	} {
		if err := del(); err != nil {
			t.Errorf("delete %s: %v", name, err)
		}
		if err := del(); !errors.Is(err, ErrNotFound) {
			t.Errorf("delete %s twice = %v", name, err)
		}
	}

	// certificate state
	if err := s.UpdateCertificateState(ctx, acmeCert.ID, "valid", "", []string{"b.example.com", "c.example.com"}, time.Now().Add(90*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	gc, err := s.CertificateByID(ctx, acmeCert.ID)
	if err != nil || gc.Status != "valid" || len(gc.Domains) != 2 || gc.Challenge != "http-01" {
		t.Fatalf("after UpdateCertificateState: %+v %v", gc, err)
	}
	if err := s.SetCertificateStatus(ctx, acmeCert.ID, "error", "rate limited"); err != nil {
		t.Fatal(err)
	}
	if gc, _ = s.CertificateByID(ctx, acmeCert.ID); gc.Status != "error" || gc.LastError != "rate limited" {
		t.Fatalf("after SetCertificateStatus: %+v", gc)
	}
	if err := s.ReplaceCustomCertificate(ctx, acmeCert.ID, "x", nil, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replacing an ACME certificate = %v", err)
	}
	_ = s.SetCertificateStatus(ctx, cert.ID, "error", "expired")
	if err := s.ReplaceCustomCertificate(ctx, cert.ID, "renamed", []string{"z.example.com"}, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if gc, _ = s.CertificateByID(ctx, cert.ID); gc.Name != "renamed" || gc.Status != "valid" || gc.LastError != "" || gc.Domains[0] != "z.example.com" {
		t.Fatalf("after ReplaceCustomCertificate: %+v", gc)
	}
	if _, err := s.CertificateByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing cert = %v", err)
	}
	if err := s.DeleteCertificate(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing cert = %v", err)
	}
	rh := &RedirectionHost{Domains: []string{"r.example.com"}, ForwardScheme: "auto", ForwardDomain: "x.example.com", HTTPCode: 301, CertificateID: &cert.ID, Enabled: true}
	if err := s.SaveRedirectionHost(ctx, rh); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCertificate(ctx, cert.ID); err == nil || !strings.Contains(err.Error(), "redirection host") {
		t.Fatalf("cert used by redirection = %v", err)
	}
	_ = s.DeleteRedirectionHost(ctx, rh.ID)
	st := &Stream{IncomingPort: 9000, TCP: true, ForwardHost: "db", ForwardPort: 5432, CertificateID: &cert.ID, Enabled: true}
	if err := s.SaveStream(ctx, st); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCertificate(ctx, cert.ID); err == nil || !strings.Contains(err.Error(), "stream") {
		t.Fatalf("cert used by stream = %v", err)
	}
	_ = s.DeleteStream(ctx, st.ID)
	if err := s.DeleteCertificate(ctx, cert.ID); err != nil {
		t.Fatal(err)
	}

	// ACME accounts
	if _, err := s.ACMEAccount(ctx, "x@example.com", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account = %v", err)
	}
	if err := s.SaveACMEAccount(ctx, &ACMEAccount{Email: "x@example.com", Staging: true, KeyPEM: "k1", Registration: "{}"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveACMEAccount(ctx, &ACMEAccount{Email: "x@example.com", Staging: true, KeyPEM: "k2", Registration: `{"a":1}`}); err != nil {
		t.Fatal(err)
	}
	if acc, err := s.ACMEAccount(ctx, "x@example.com", true); err != nil || acc.KeyPEM != "k2" {
		t.Fatalf("ACMEAccount: %+v %v", acc, err)
	}
	if _, err := s.ACMEAccount(ctx, "x@example.com", false); !errors.Is(err, ErrNotFound) {
		t.Fatal("staging and production accounts are separate")
	}

	// ---------------------------------------------------------------- settings in meta
	if cf, err := s.CloudflareIPs(ctx); err != nil || cf != nil {
		t.Fatalf("no cloudflare snapshot yet: %v %v", cf, err)
	}
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := s.SetCloudflareIPs(ctx, CloudflareIPs{IPv4: []string{"173.245.48.0/20"}, IPv6: []string{"2400:cb00::/32"}, UpdatedAt: when, Source: "builtin"}); err != nil {
		t.Fatal(err)
	}
	if cf, err := s.CloudflareIPs(ctx); err != nil || cf.Source != "builtin" || !cf.UpdatedAt.Equal(when) || cf.IPv6[0] != "2400:cb00::/32" {
		t.Fatalf("CloudflareIPs: %+v %v", cf, err)
	}
	if nw, err := s.Network(ctx); err != nil || nw.RealIPHeader != "X-Forwarded-For" || nw.TrustedProxies == nil {
		t.Fatalf("default network: %+v %v", nw, err)
	}
	_ = s.SetMeta(ctx, "network", `{"real_ip_header":"CF-Connecting-IP","mask_ips":true}`)
	if nw, _ := s.Network(ctx); nw.RealIPHeader != "CF-Connecting-IP" || !nw.MaskIPs || nw.TrustedProxies == nil {
		t.Fatalf("network: %+v", nw)
	}
	if p, err := s.CustomPages(ctx); err != nil || p.Block != "" || p.Error != "" {
		t.Fatalf("default pages: %+v %v", p, err)
	}
	if err := s.SetCustomPages(ctx, CustomPages{Block: "<h1>no</h1>", Error: "<h1>down</h1>"}); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.CustomPages(ctx); p.Block != "<h1>no</h1>" || p.Error != "<h1>down</h1>" {
		t.Fatalf("pages: %+v", p)
	}
	if uh, err := s.UnknownHosts(ctx); err != nil || uh.Mode != UnknownNotFound {
		t.Fatalf("default unknown hosts: %+v %v", uh, err)
	}
	if err := s.SetUnknownHosts(ctx, UnknownHosts{Mode: UnknownRedirect, RedirectURL: "https://example.com"}); err != nil {
		t.Fatal(err)
	}
	if uh, _ := s.UnknownHosts(ctx); uh.Mode != UnknownRedirect || uh.RedirectURL != "https://example.com" {
		t.Fatalf("unknown hosts: %+v", uh)
	}
	_ = s.SetMeta(ctx, "unknown_hosts", `{"redirect_url":"x"}`)
	if uh, _ := s.UnknownHosts(ctx); uh.Mode != UnknownNotFound {
		t.Fatalf("empty mode defaults to not_found: %+v", uh)
	}
	_ = s.SetMeta(ctx, "unknown_hosts", `{bad json`)
	if _, err := s.UnknownHosts(ctx); err == nil {
		t.Fatal("corrupt unknown_hosts must report an error")
	}
	_ = s.SetMeta(ctx, "custom_pages", `{bad json`)
	if _, err := s.CustomPages(ctx); err == nil {
		t.Fatal("corrupt custom_pages must report an error")
	}
	_ = s.SetMeta(ctx, "cloudflare_ips", `{bad json`)
	if _, err := s.CloudflareIPs(ctx); err == nil {
		t.Fatal("corrupt cloudflare_ips must report an error")
	}
	if rev1, err := s.BumpConfigRev(ctx); err != nil {
		t.Fatal(err)
	} else if rev2, _ := s.BumpConfigRev(ctx); rev2 != rev1+1 {
		t.Fatalf("config rev %d -> %d", rev1, rev2)
	}

	// ---------------------------------------------------------------- security rules & IP lists
	lst := &IPList{Name: "bad-ips", Items: []string{"192.0.2.0/24"}}
	if err := s.SaveIPList(ctx, lst); err != nil || lst.Kind != ListKindIP {
		t.Fatalf("SaveIPList: %+v %v", lst, err)
	}
	ua := &IPList{Name: "bots", Kind: ListKindUserAgent, Items: []string{"curl"}, SourceURL: "https://feeds.example/ua.txt", RefreshS: 3600}
	_ = s.SaveIPList(ctx, ua)
	lst.Description = "edited"
	if err := s.SaveIPList(ctx, lst); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveIPList(ctx, &IPList{ID: 999, Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing list = %v", err)
	}
	rule := &SecurityRule{Name: "block bad", Enabled: true, Action: "block", StatusCode: 403, Scope: "all",
		Expression: Expression{Groups: [][]Condition{{{Field: "ip.src", Operator: "in", Value: []string{"list:" + strconv.FormatInt(lst.ID, 10)}}}}}}
	if err := s.SaveSecurityRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	if gr, err := s.SecurityRuleByID(ctx, rule.ID); err != nil || gr.ChallengeMode != DefaultChallengeMode || gr.ChallengeTTL != DefaultChallengeTTL {
		t.Fatalf("SecurityRuleByID: %+v %v", gr, err)
	}
	if _, err := s.SecurityRuleByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing rule = %v", err)
	}
	if err := s.SaveSecurityRule(ctx, &SecurityRule{ID: 999, Name: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing rule = %v", err)
	}
	gl, err := s.IPListByID(ctx, lst.ID)
	if err != nil || gl.Rules != 1 || gl.Description != "edited" || gl.FetchedAt != nil {
		t.Fatalf("IPListByID: %+v %v", gl, err)
	}
	if _, err := s.IPListByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing list = %v", err)
	}
	if err := s.DeleteIPList(ctx, lst.ID); err == nil || !strings.Contains(err.Error(), "used by 1 security rule") {
		t.Fatalf("delete used list = %v", err)
	}

	// feed fetch bookkeeping
	checked := time.Now().UTC().Truncate(time.Second)
	if err := s.SetIPListFetch(ctx, ua.ID, ListFetch{CheckedAt: checked, FetchedAt: &checked, Items: []string{"wget", "python"}, Warning: "2 lines skipped"}); err != nil {
		t.Fatal(err)
	}
	gu, _ := s.IPListByID(ctx, ua.ID)
	if len(gu.Items) != 2 || gu.FetchedAt == nil || gu.CheckedAt == nil || gu.FetchWarning != "2 lines skipped" {
		t.Fatalf("after fetch with items: %+v", gu)
	}
	later := checked.Add(time.Hour)
	if err := s.SetIPListFetch(ctx, ua.ID, ListFetch{CheckedAt: later, FetchedAt: &later}); err != nil {
		t.Fatal(err)
	}
	if gu, _ = s.IPListByID(ctx, ua.ID); len(gu.Items) != 2 || gu.FetchWarning != "" || !gu.FetchedAt.Equal(later) {
		t.Fatalf("after unchanged fetch: %+v", gu)
	}
	if err := s.SetIPListFetch(ctx, ua.ID, ListFetch{CheckedAt: later.Add(time.Hour), Error: "HTTP 500"}); err != nil {
		t.Fatal(err)
	}
	if gu, _ = s.IPListByID(ctx, ua.ID); gu.FetchError != "HTTP 500" || !gu.FetchedAt.Equal(later) || len(gu.Items) != 2 {
		t.Fatalf("failed fetch keeps entries and last success: %+v", gu)
	}
	if err := s.SetIPListFetch(ctx, 999, ListFetch{CheckedAt: later}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("fetch for missing list = %v", err)
	}
	if err := s.ResetIPListFetch(ctx, ua.ID); err != nil {
		t.Fatal(err)
	}
	if gu, _ = s.IPListByID(ctx, ua.ID); gu.FetchedAt != nil || gu.CheckedAt != nil || gu.FetchError != "" {
		t.Fatalf("after reset: %+v", gu)
	}
	if err := s.DeleteSecurityRule(ctx, rule.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSecurityRule(ctx, rule.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete rule twice = %v", err)
	}
	if err := s.DeleteIPList(ctx, lst.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteIPList(ctx, lst.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete list twice = %v", err)
	}
	// reorder validation
	r1 := &SecurityRule{Name: "r1", Action: "log", Scope: "all"}
	r2 := &SecurityRule{Name: "r2", Action: "log", Scope: "all"}
	_ = s.SaveSecurityRule(ctx, r1)
	_ = s.SaveSecurityRule(ctx, r2)
	if err := s.ReorderSecurityRules(ctx, []int64{r1.ID}); err == nil {
		t.Fatal("a partial order must be rejected")
	}
	if err := s.ReorderSecurityRules(ctx, []int64{r1.ID, r1.ID}); err == nil {
		t.Fatal("duplicates must be rejected")
	}
	if err := s.ReorderSecurityRules(ctx, []int64{r1.ID, 999}); err == nil || !strings.Contains(err.Error(), "unknown rule 999") {
		t.Fatalf("unknown id = %v", err)
	}
	if err := s.ReorderSecurityRules(ctx, []int64{r2.ID, r1.ID}); err != nil {
		t.Fatal(err)
	}
	if rs, _ := s.ListSecurityRules(ctx); rs[0].ID != r2.ID {
		t.Fatalf("order after reorder: %d first", rs[0].ID)
	}
}

// TestCovClosedDatabase checks that every accessor reports the database error instead
// of panicking or returning zero values silently.
func TestCovClosedDatabase(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	ctx := context.Background()
	now := time.Now()
	id := int64(1)
	calls := map[string]func() error{
		"BumpConfigRev":   func() error { _, err := s.BumpConfigRev(ctx); return err },
		"WithTx":          func() error { return s.WithTx(ctx, func(*Store) error { return nil }) },
		"CountUsers":      func() error { _, err := s.CountUsers(ctx); return err },
		"CreateUser":      func() error { _, err := s.CreateUser(ctx, "a", "b", "c", "admin"); return err },
		"UserByID":        func() error { _, err := s.UserByID(ctx, 1); return err },
		"TouchLogin":      func() error { return s.TouchLogin(ctx, 1) },
		"UpdatePassword":  func() error { return s.UpdatePassword(ctx, 1, "h") },
		"SessionUser":     func() error { _, err := s.SessionUser(ctx, "t"); return err },
		"CreateCert":      func() error { return s.CreateCertificate(ctx, &Certificate{}) },
		"ListCerts":       func() error { _, err := s.ListCertificates(ctx); return err },
		"CertByID":        func() error { _, err := s.CertificateByID(ctx, 1); return err },
		"DeleteCert":      func() error { return s.DeleteCertificate(ctx, 1) },
		"ListWaitrooms":   func() error { _, err := s.ListWaitrooms(ctx); return err },
		"WaitroomByID":    func() error { _, err := s.WaitroomByID(ctx, 1); return err },
		"SaveWaitroomNew": func() error { return s.SaveWaitroom(ctx, &Waitroom{}) },
		"SaveWaitroomUpd": func() error { return s.SaveWaitroom(ctx, &Waitroom{ID: 1}) },
		"DeleteWaitroom":  func() error { return s.DeleteWaitroom(ctx, 1) },
		"ListProxyHosts":  func() error { _, err := s.ListProxyHosts(ctx); return err },
		"SaveHostNew":     func() error { return s.SaveProxyHost(ctx, &ProxyHost{}) },
		"SaveHostUpd":     func() error { return s.SaveProxyHost(ctx, &ProxyHost{ID: 1}) },
		"DeleteProxyHost": func() error { return s.DeleteProxyHost(ctx, 1) },
		"CreateVersion":   func() error { return s.CreateVersion(ctx, &ConfigVersion{}) },
		"FinishVersion":   func() error { return s.FinishVersion(ctx, 1, true, "") },
		"ListVersions":    func() error { _, err := s.ListVersions(ctx, 1, 0); return err },
		"VersionDirs":     func() error { _, err := s.VersionDirsInUse(ctx, 1); return err },
		"ListAudit":       func() error { _, err := s.ListAudit(ctx, 1); return err },
		"ListChannels":    func() error { _, err := s.ListAlertChannels(ctx); return err },
		"ChannelByID":     func() error { _, err := s.AlertChannelByID(ctx, 1); return err },
		"SaveChannelNew":  func() error { return s.SaveAlertChannel(ctx, &AlertChannel{}) },
		"SaveChannelUpd":  func() error { return s.SaveAlertChannel(ctx, &AlertChannel{ID: 1}) },
		"DeleteChannel":   func() error { return s.DeleteAlertChannel(ctx, 1) },
		"AddAlert":        func() error { return s.AddAlert(ctx, &AlertRecord{}) },
		"ListAlerts":      func() error { _, err := s.ListAlerts(ctx, 1); return err },
		"FailedVersions":  func() error { _, err := s.FailedVersionsSince(ctx, 0); return err },
		"ActiveBans":      func() error { _, err := s.ActiveBans(ctx, now); return err },
		"UpsertBan":       func() error { return s.UpsertBan(ctx, &IPBan{IP: "x"}) },
		"LiftBan":         func() error { _, err := s.LiftBan(ctx, 1); return err },
		"MakePermanent":   func() error { _, err := s.MakeBanPermanent(ctx, 1); return err },
		"CreateToken":     func() error { _, err := s.CreateAPIToken(ctx, "a", "b", "c", "d", 1, &now); return err },
		"ListTokens":      func() error { _, err := s.ListAPITokens(ctx); return err },
		"DeleteToken":     func() error { return s.DeleteAPIToken(ctx, 1) },
		"DeleteUserToks":  func() error { _, err := s.DeleteUserAPITokens(ctx, 1); return err },
		"Network":         func() error { _, err := s.Network(ctx); return err },
		"CloudflareIPs":   func() error { _, err := s.CloudflareIPs(ctx); return err },
		"ListRateLimits":  func() error { _, err := s.ListRateLimits(ctx); return err },
		"RateLimitByID":   func() error { _, err := s.RateLimitByID(ctx, 1); return err },
		"SaveRLNew":       func() error { return s.SaveRateLimit(ctx, &RateLimit{}) },
		"SaveRLUpd":       func() error { return s.SaveRateLimit(ctx, &RateLimit{ID: 1}) },
		"DeleteRateLimit": func() error { return s.DeleteRateLimit(ctx, 1) },
		"ListACLs":        func() error { _, err := s.ListAccessLists(ctx); return err },
		"ACLByID":         func() error { _, err := s.AccessListByID(ctx, 1); return err },
		"SaveACL":         func() error { return s.SaveAccessList(ctx, &AccessList{}) },
		"DeleteACL":       func() error { return s.DeleteAccessList(ctx, 1) },
		"ReplaceCert":     func() error { return s.ReplaceCustomCertificate(ctx, 1, "", nil, now) },
		"ACMEAccount":     func() error { _, err := s.ACMEAccount(ctx, "a", false); return err },
		"ReplaceCodes":    func() error { _, err := s.ReplaceRecoveryCodes(ctx, 1, "", nil); return err },
		"ListUsers":       func() error { _, err := s.ListUsers(ctx); return err },
		"UpdateUser":      func() error { return s.UpdateUser(ctx, 1, "", "", false) },
		"DeleteUser":      func() error { return s.DeleteUser(ctx, 1) },
		"KeepingAdmin":    func() error { return s.KeepingAdmin(ctx, func(*Store) error { return nil }) },
		"ClaimTOTPStep":   func() error { _, err := s.ClaimTOTPStep(ctx, 1, 1); return err },
		"ListRules":       func() error { _, err := s.ListSecurityRules(ctx); return err },
		"RuleByID":        func() error { _, err := s.SecurityRuleByID(ctx, 1); return err },
		"SaveRule":        func() error { return s.SaveSecurityRule(ctx, &SecurityRule{}) },
		"DeleteRule":      func() error { return s.DeleteSecurityRule(ctx, 1) },
		"ReorderRules":    func() error { return s.ReorderSecurityRules(ctx, []int64{1}) },
		"ListIPLists":     func() error { _, err := s.ListIPLists(ctx); return err },
		"IPListByID":      func() error { _, err := s.IPListByID(ctx, 1); return err },
		"SaveIPListNew":   func() error { return s.SaveIPList(ctx, &IPList{}) },
		"SaveIPListUpd":   func() error { return s.SaveIPList(ctx, &IPList{ID: 1}) },
		"SetFetchItems":   func() error { return s.SetIPListFetch(ctx, 1, ListFetch{Items: []string{}}) },
		"SetFetchOK":      func() error { return s.SetIPListFetch(ctx, 1, ListFetch{FetchedAt: &now}) },
		"SetFetchFail":    func() error { return s.SetIPListFetch(ctx, 1, ListFetch{}) },
		"DeleteIPList":    func() error { return s.DeleteIPList(ctx, 1) },
		"MigrateACL":      func() error { _, err := s.MigrateAccessListIPRules(ctx); return err },
		"hostsUsingACL":   func() error { _, err := s.hostsUsingAccessList(ctx, id); return err },
		"accessListUsers": func() error { _, err := s.accessListUsers(ctx, id); return err },
		"ListDNSCreds":    func() error { _, err := s.ListDNSCredentials(ctx); return err },
		"DNSCredByID":     func() error { _, err := s.DNSCredentialByID(ctx, 1); return err },
		"UpdateDNSCred":   func() error { return s.UpdateDNSCredential(ctx, &DNSCredential{ID: 1}) },
		"DeleteDNSCred":   func() error { return s.DeleteDNSCredential(ctx, 1) },
		"ListRedirects":   func() error { _, err := s.ListRedirectionHosts(ctx); return err },
		"SaveRedirectNew": func() error { return s.SaveRedirectionHost(ctx, &RedirectionHost{}) },
		"SaveRedirectUpd": func() error { return s.SaveRedirectionHost(ctx, &RedirectionHost{ID: 1}) },
		"DeleteRedirect":  func() error { return s.DeleteRedirectionHost(ctx, 1) },
		"ListStreams":     func() error { _, err := s.ListStreams(ctx); return err },
		"SaveStreamNew":   func() error { return s.SaveStream(ctx, &Stream{}) },
		"SaveStreamUpd":   func() error { return s.SaveStream(ctx, &Stream{ID: 1}) },
		"DeleteStream":    func() error { return s.DeleteStream(ctx, 1) },
	}
	for name, call := range calls {
		if err := call(); err == nil {
			t.Errorf("%s on a closed database returned no error", name)
		}
	}
	if s.Dialect() != SQLite {
		t.Fatal("dialect")
	}
}
