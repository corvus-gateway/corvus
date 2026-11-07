package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDNSCredentialCRUD(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	d := &DNSCredential{Name: "Cloudflare", Provider: "cloudflare", Secret: "sealed-1"}
	if err := s.CreateDNSCredential(ctx, d); err != nil || d.ID == 0 {
		t.Fatalf("create: %v", err)
	}
	if err := s.CreateDNSCredential(ctx, &DNSCredential{Name: "Cloudflare", Provider: "cloudflare", Secret: "x"}); !IsUnique(err) {
		t.Fatalf("duplicate name: %v", err)
	}
	c := &Certificate{Name: "w", Kind: "acme", Domains: []string{"*.example.com"}, NotAfter: time.Now().Add(time.Hour),
		ACME: ACMEInfo{Email: "a@example.com", Challenge: "dns-01", DNSProvider: "cloudflare"}, DNSCredentialID: &d.ID}
	if err := s.CreateCertificate(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, err := s.CertificateByID(ctx, c.ID)
	if err != nil || got.DNSCredentialID == nil || *got.DNSCredentialID != d.ID {
		t.Fatalf("certificate reference: %+v %v", got, err)
	}
	cd, err := s.DNSCredentialByID(ctx, d.ID)
	if err != nil || cd.UsedBy != 1 || cd.Secret != "sealed-1" {
		t.Fatalf("used_by: %+v %v", cd, err)
	}
	cd.Name, cd.Secret = "Main zone", "sealed-2"
	if err := s.UpdateDNSCredential(ctx, cd); err != nil {
		t.Fatal(err)
	}
	if cd, _ = s.DNSCredentialByID(ctx, d.ID); cd.Name != "Main zone" || cd.Secret != "sealed-2" {
		t.Fatalf("update: %+v", cd)
	}
	if err := s.DeleteDNSCredential(ctx, d.ID); err == nil || !strings.Contains(err.Error(), "1 certificate") {
		t.Fatalf("in-use credential deleted: %v", err)
	}
	if err := s.DeleteCertificate(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDNSCredential(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDNSCredential(ctx, d.ID); err != ErrNotFound {
		t.Fatalf("delete missing: %v", err)
	}
}

// SetCertificateDNSCredential moves a legacy certificate onto a saved credential.
func TestSetCertificateDNSCredential(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := &Certificate{Name: "old", Kind: "acme", Domains: []string{"*.example.com"}, NotAfter: time.Now().Add(time.Hour),
		ACME: ACMEInfo{Email: "a@example.com", Challenge: "dns-01", DNSProvider: "cloudflare", Credentials: "legacy-sealed"}}
	if err := s.CreateCertificate(ctx, c); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.CertificateByID(ctx, c.ID); got.DNSCredentialID != nil || got.ACME.Credentials != "legacy-sealed" {
		t.Fatalf("legacy certificate: %+v", got)
	}
	d := &DNSCredential{Name: "Cloudflare", Provider: "cloudflare", Secret: "legacy-sealed"}
	if err := s.CreateDNSCredential(ctx, d); err != nil {
		t.Fatal(err)
	}
	info := c.ACME
	info.Credentials = ""
	if err := s.SetCertificateDNSCredential(ctx, c.ID, d.ID, info); err != nil {
		t.Fatal(err)
	}
	got, _ := s.CertificateByID(ctx, c.ID)
	if got.DNSCredentialID == nil || *got.DNSCredentialID != d.ID || got.ACME.Credentials != "" || got.ACME.Email != "a@example.com" {
		t.Fatalf("after move: %+v", got)
	}
}
