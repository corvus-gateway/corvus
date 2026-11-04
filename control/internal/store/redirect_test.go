package store

import (
	"context"
	"testing"
	"time"
)

func TestRedirectionHostCRUD(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := &Certificate{Name: "c", Kind: "custom", Domains: []string{"old.example.com"}, NotAfter: time.Now().Add(time.Hour)}
	if err := s.CreateCertificate(ctx, c); err != nil {
		t.Fatal(err)
	}
	h := &RedirectionHost{Domains: []string{"old.example.com"}, ForwardScheme: "https", ForwardDomain: "new.example.com", HTTPCode: 308,
		PreservePath: true, CertificateID: &c.ID, SSLForced: true, HTTP2: true, BlockExploits: true, AdvancedConfig: "add_header X-A b;", Enabled: true}
	if err := s.SaveRedirectionHost(ctx, h); err != nil || h.ID == 0 {
		t.Fatalf("create: %v", err)
	}
	got, err := s.RedirectionHostByID(ctx, h.ID)
	if err != nil || got.ForwardDomain != "new.example.com" || got.HTTPCode != 308 || !got.PreservePath || *got.CertificateID != c.ID ||
		!got.SSLForced || !got.HTTP2 || !got.BlockExploits || !got.Enabled || got.AdvancedConfig != "add_header X-A b;" || got.Domains[0] != "old.example.com" {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if err := s.DeleteCertificate(ctx, c.ID); err == nil {
		t.Fatal("deleting a certificate used by a redirection host must fail")
	}
	got.PreservePath, got.Enabled, got.CertificateID, got.SSLForced, got.HTTPCode = false, false, nil, false, 302
	if err := s.SaveRedirectionHost(ctx, got); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListRedirectionHosts(ctx); len(list) != 1 || list[0].PreservePath || list[0].Enabled || list[0].CertificateID != nil || list[0].HTTPCode != 302 {
		t.Fatalf("update: %+v", list)
	}
	if err := s.DeleteRedirectionHost(ctx, h.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RedirectionHostByID(ctx, h.ID); err != ErrNotFound {
		t.Fatalf("after delete: %v", err)
	}
	if err := s.DeleteRedirectionHost(ctx, h.ID); err != ErrNotFound {
		t.Fatalf("delete missing: %v", err)
	}
	if err := s.SaveRedirectionHost(ctx, &RedirectionHost{ID: 999, Domains: []string{"x.example.com"}}); err != ErrNotFound {
		t.Fatalf("update missing: %v", err)
	}
}
