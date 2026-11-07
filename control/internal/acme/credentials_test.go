package acme

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"corvus/control/internal/seal"
	"corvus/control/internal/store"
)

func testService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	return &Service{Store: st, Key: key}
}

// legacyCert creates a DNS-01 certificate the way older versions stored it: with its own sealed secret.
func legacyCert(t *testing.T, s *Service, provider string, cred Credentials) int64 {
	t.Helper()
	raw, _ := json.Marshal(cred)
	sealed, err := seal.Seal(s.Key, string(raw))
	if err != nil {
		t.Fatal(err)
	}
	c := &store.Certificate{Name: "c", Kind: "acme", Domains: []string{"*.example.com"}, NotAfter: time.Now().Add(time.Hour), AutoRenew: true,
		ACME: store.ACMEInfo{Email: "a@example.com", Challenge: "dns-01", DNSProvider: provider, Credentials: sealed}}
	if err := s.Store.CreateCertificate(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return c.ID
}

func TestAutoName(t *testing.T) {
	list := []store.DNSCredential{{Name: "Cloudflare"}, {Name: "Cloudflare 2"}, {Name: "DigitalOcean"}}
	if got := AutoName(list, "cloudflare"); got != "Cloudflare 3" {
		t.Fatalf("got %q", got)
	}
	if got := AutoName(list, "duckdns"); got != "DuckDNS" {
		t.Fatalf("got %q", got)
	}
}

// Older certificates keep renewing from their own secret, and the startup move turns
// them into saved credentials: one per distinct provider+secret.
func TestAdoptLegacyCredentials(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	a := legacyCert(t, s, "cloudflare", Credentials{APIToken: "tok-a"})
	b := legacyCert(t, s, "cloudflare", Credentials{APIToken: "tok-a"})
	c := legacyCert(t, s, "cloudflare", Credentials{APIToken: "tok-c"})
	d := legacyCert(t, s, "duckdns", Credentials{Values: map[string]string{"dns_duckdns_token": "duck"}})

	cert, _ := s.Store.CertificateByID(ctx, a)
	if p, cred, err := s.dnsCredentials(ctx, cert); err != nil || p != "cloudflare" || cred.APIToken != "tok-a" {
		t.Fatalf("legacy renewal credentials: %s %+v %v", p, cred, err)
	}

	n, err := s.AdoptLegacyCredentials(ctx)
	if err != nil || n != 4 {
		t.Fatalf("adopt: %d %v", n, err)
	}
	list, _ := s.Store.ListDNSCredentials(ctx)
	if len(list) != 3 {
		t.Fatalf("want 3 de-duplicated credentials, got %+v", list)
	}
	names := map[string]int{}
	for _, x := range list {
		names[x.Name] = x.UsedBy
	}
	if names["Cloudflare"] != 2 || names["Cloudflare 2"] != 1 || names["DuckDNS"] != 1 {
		t.Fatalf("names/usage: %v", names)
	}
	ca, _ := s.Store.CertificateByID(ctx, a)
	cb, _ := s.Store.CertificateByID(ctx, b)
	cc, _ := s.Store.CertificateByID(ctx, c)
	cd, _ := s.Store.CertificateByID(ctx, d)
	if ca.DNSCredentialID == nil || cb.DNSCredentialID == nil || *ca.DNSCredentialID != *cb.DNSCredentialID || *cc.DNSCredentialID == *ca.DNSCredentialID {
		t.Fatalf("references: %v %v %v", ca.DNSCredentialID, cb.DNSCredentialID, cc.DNSCredentialID)
	}
	if ca.ACME.Credentials != "" || ca.ACME.Email != "a@example.com" || ca.ACME.Challenge != "dns-01" {
		t.Fatalf("legacy secret not dropped / info lost: %+v", ca.ACME)
	}
	if p, cred, err := s.dnsCredentials(ctx, cd); err != nil || p != "duckdns" || cred.Values["dns_duckdns_token"] != "duck" {
		t.Fatalf("moved credentials: %s %+v %v", p, cred, err)
	}
	if n, err := s.AdoptLegacyCredentials(ctx); err != nil || n != 0 {
		t.Fatalf("second run: %d %v", n, err)
	}
}

// Renewals read the referenced credential, so replacing its secret applies to every
// certificate that uses it.
func TestRenewalUsesSavedCredential(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	saved, err := SaveCredential(ctx, s.Store, s.Key, "", "cloudflare", Credentials{APIToken: "old"})
	if err != nil || saved.Name != "Cloudflare" {
		t.Fatalf("save: %+v %v", saved, err)
	}
	var ids []int64
	for i := 0; i < 2; i++ {
		c := &store.Certificate{Name: "c", Kind: "acme", Domains: []string{"*.example.com"}, NotAfter: time.Now().Add(time.Hour),
			ACME: store.ACMEInfo{Email: "a@example.com", Challenge: "dns-01", DNSProvider: "cloudflare"}, DNSCredentialID: &saved.ID}
		if err := s.Store.CreateCertificate(ctx, c); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	saved.Secret, _ = SealCredential(s.Key, Credentials{APIToken: "new"})
	if err := s.Store.UpdateDNSCredential(ctx, saved); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		c, _ := s.Store.CertificateByID(ctx, id)
		p, cred, err := s.dnsCredentials(ctx, c)
		if err != nil || p != "cloudflare" || cred.APIToken != "new" {
			t.Fatalf("certificate %d: %s %+v %v", id, p, cred, err)
		}
		if _, err := dnsProvider(p, cred); err != nil {
			t.Fatal(err)
		}
	}
}

// Imported certificates share one saved credential per provider and secret.
func TestReuseOrSaveCredential(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	a, created, err := ReuseOrSaveCredential(ctx, s.Store, s.Key, "cloudflare", Credentials{APIToken: "t"})
	if err != nil || !created {
		t.Fatalf("first: %v %v", created, err)
	}
	b, created, err := ReuseOrSaveCredential(ctx, s.Store, s.Key, "cloudflare", Credentials{APIToken: "t"})
	if err != nil || created || b.ID != a.ID {
		t.Fatalf("same secret not reused: %+v %v %v", b, created, err)
	}
	c, created, err := ReuseOrSaveCredential(ctx, s.Store, s.Key, "cloudflare", Credentials{APIToken: "other"})
	if err != nil || !created || c.ID == a.ID || c.Name != "Cloudflare 2" {
		t.Fatalf("different secret: %+v %v %v", c, created, err)
	}
}
