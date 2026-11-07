package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// openTest opens SQLite in a temp dir, or PostgreSQL when CORVUS_TEST_PG_URL is set
// (the database must be empty; it is reset by dropping the public schema).
func openTest(t *testing.T) *Store {
	t.Helper()
	if url := os.Getenv("CORVUS_TEST_PG_URL"); url != "" {
		s, err := Open(url)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
			t.Fatal(err)
		}
		s.Close()
		s, err = Open(url)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	s, err := Open(filepath.Join(t.TempDir(), "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	t.Logf("dialect: %s", s.Dialect())

	u, err := s.CreateUser(ctx, "Admin@Example.com", "Admin", "hash", "admin")
	if err != nil || u.ID == 0 || u.Email != "admin@example.com" {
		t.Fatalf("user: %+v %v", u, err)
	}
	if err := s.CreateSession(ctx, "tok", u.ID, time.Hour, "1.2.3.4", "ua"); err != nil {
		t.Fatal(err)
	}
	if su, err := s.SessionUser(ctx, "tok"); err != nil || su.ID != u.ID {
		t.Fatalf("session: %v", err)
	}
	if err := s.SetMFA(ctx, u.ID, true, []string{"h1"}); err != nil {
		t.Fatal(err)
	}
	if u2, _ := s.UserByID(ctx, u.ID); !u2.MFAEnabled || u2.RecoveryCodes() != `["h1"]` {
		t.Fatalf("mfa: %+v", u2)
	}
	// TOTP replay protection is persisted and monotonic (H11)
	for i, c := range []struct {
		step int64
		want bool
	}{{100, true}, {100, false}, {99, false}, {101, true}} {
		if ok, err := s.ClaimTOTPStep(ctx, u.ID, c.step); err != nil || ok != c.want {
			t.Fatalf("claim %d (step %d): ok=%v err=%v", i, c.step, ok, err)
		}
	}

	c := &Certificate{Name: "c", Kind: "acme", Domains: []string{"a.example.com"}, NotAfter: time.Now().Add(48 * time.Hour), Status: "pending", AutoRenew: true,
		ACME: ACMEInfo{Email: "a@b.c", Challenge: "dns-01", DNSProvider: "cloudflare"}}
	if err := s.CreateCertificate(ctx, c); err != nil || c.ID == 0 {
		t.Fatalf("cert: %v", err)
	}
	if got, _ := s.CertificateByID(ctx, c.ID); got.ACME.DNSProvider != "cloudflare" || got.Status != "pending" || !got.AutoRenew {
		t.Fatalf("cert round trip: %+v", got)
	}

	w := &Waitroom{Name: "event", Enabled: true, Capacity: 500, ActiveTTL: 120, GrantTTL: 30, QueueTTL: 120, Refresh: 15, FailMode: "open"}
	rl := &RateLimit{Name: "api", RequestsPerSecond: 10, Burst: 5, StatusCode: 429, ExemptCIDRs: []string{"10.0.0.0/8"}, Enabled: true}
	acl := &AccessList{Name: "office", Satisfy: "any", Allow: []string{"10.0.0.0/8"}, Deny: []string{"all"}, Users: []AccessListUser{{Username: "bob", PasswordHash: "$apr1$x"}}}
	err = s.WithTx(ctx, func(tx *Store) error {
		if err := tx.SaveWaitroom(ctx, w); err != nil {
			return err
		}
		if err := tx.SaveRateLimit(ctx, rl); err != nil {
			return err
		}
		if err := tx.SaveAccessList(ctx, acl); err != nil {
			return err
		}
		h := &ProxyHost{Domains: []string{"a.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, CertificateID: &c.ID,
			WaitroomID: &w.ID, WaitroomPath: "/", RateLimitID: &rl.ID, AccessListID: &acl.ID, ClientIP: "cloudflare", BlockExploits: true, Enabled: true}
		return tx.SaveProxyHost(ctx, h)
	})
	if err != nil {
		t.Fatal(err)
	}
	hosts, _ := s.ListProxyHosts(ctx)
	if len(hosts) != 1 || *hosts[0].RateLimitID != rl.ID || *hosts[0].AccessListID != acl.ID || hosts[0].ClientIP != "cloudflare" || !hosts[0].BlockExploits {
		t.Fatalf("host: %+v", hosts)
	}
	if list, _ := s.ListRateLimits(ctx); len(list) != 1 || list[0].Hosts != 1 || list[0].ExemptCIDRs[0] != "10.0.0.0/8" {
		t.Fatalf("rate limits: %+v", list)
	}
	if list, _ := s.ListAccessLists(ctx); len(list) != 1 || len(list[0].Users) != 1 || list[0].Hosts != 1 {
		t.Fatalf("acls: %+v", list)
	}
	if err := s.DeleteRateLimit(ctx, rl.ID); err == nil {
		t.Fatal("deleting an in-use rate limit must fail")
	}
	if err := s.SaveRateLimit(ctx, &RateLimit{Name: "api", RequestsPerSecond: 1, StatusCode: 429, ExemptCIDRs: []string{}}); !IsUnique(err) {
		t.Fatalf("duplicate name should be a unique violation: %v", err)
	}

	// rolled-back transaction leaves nothing behind
	_ = s.WithTx(ctx, func(tx *Store) error {
		_ = tx.SaveWaitroom(ctx, &Waitroom{Name: "tmp", Capacity: 1, ActiveTTL: 10, GrantTTL: 5, QueueTTL: 10, Refresh: 1, FailMode: "open"})
		return os.ErrInvalid
	})
	if rooms, _ := s.ListWaitrooms(ctx); len(rooms) != 1 {
		t.Fatalf("rollback leaked: %d rooms", len(rooms))
	}

	rev, _ := s.BumpConfigRev(ctx)
	v := &ConfigVersion{Message: "m", ConfigRev: rev}
	if err := s.CreateVersion(ctx, v); err != nil || v.ID == 0 {
		t.Fatal(err)
	}
	if err := s.FinishVersion(ctx, v.ID, true, "ok"); err != nil {
		t.Fatal(err)
	}
	if a, err := s.ActiveVersion(ctx); err != nil || a.ID != v.ID {
		t.Fatalf("active: %v", err)
	}
	if err := s.Audit(ctx, &u.ID, "x.y", "t", map[string]int{"a": 1}, "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListAudit(ctx, 5); len(list) != 1 || list[0].Email != "admin@example.com" {
		t.Fatalf("audit: %+v", list)
	}
	n := NetworkSettings{TrustedProxies: []string{"10.0.0.0/8"}, RealIPHeader: "X-Real-IP", MaskIPs: true}
	if err := s.SetNetwork(ctx, n); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Network(ctx); got.RealIPHeader != "X-Real-IP" || !got.MaskIPs {
		t.Fatalf("network: %+v", got)
	}
	if err := s.SaveACMEAccount(ctx, &ACMEAccount{Email: "a@b.c", KeyPEM: "k1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveACMEAccount(ctx, &ACMEAccount{Email: "a@b.c", KeyPEM: "k2"}); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.ACMEAccount(ctx, "a@b.c", false); a.KeyPEM != "k2" {
		t.Fatal("acme account upsert")
	}
}

// A recovery code can be used once even when two requests use codes at the same time.
func TestReplaceRecoveryCodesCAS(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u, err := s.CreateUser(ctx, "a@example.com", "A", "x", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRecoveryCodes(ctx, u.ID, []string{"h1", "h2"}); err != nil {
		t.Fatal(err)
	}
	cur, _ := s.UserByID(ctx, u.ID)
	old := cur.RecoveryCodes()
	if ok, err := s.ReplaceRecoveryCodes(ctx, u.ID, old, []string{"h2"}); !ok || err != nil {
		t.Fatalf("first use: %v %v", ok, err)
	}
	if ok, _ := s.ReplaceRecoveryCodes(ctx, u.ID, old, []string{"h1"}); ok {
		t.Fatal("a second request with the stale list succeeded")
	}
	if cur, _ = s.UserByID(ctx, u.ID); cur.RecoveryCodes() != `["h2"]` {
		t.Fatalf("codes = %s", cur.RecoveryCodes())
	}
}
