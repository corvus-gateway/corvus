package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStreamCRUD(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	c := &Certificate{Name: "c", Kind: "custom", Domains: []string{"db.example.com"}, NotAfter: time.Now().Add(time.Hour)}
	if err := s.CreateCertificate(ctx, c); err != nil {
		t.Fatal(err)
	}
	st := &Stream{IncomingPort: 5432, TCP: true, ForwardHost: "db", ForwardPort: 5432, CertificateID: &c.ID, Enabled: true}
	if err := s.SaveStream(ctx, st); err != nil || st.ID == 0 {
		t.Fatalf("create: %v", err)
	}
	got, err := s.StreamByID(ctx, st.ID)
	if err != nil || got.IncomingPort != 5432 || !got.TCP || got.UDP || got.ForwardHost != "db" || got.ForwardPort != 5432 ||
		got.CertificateID == nil || *got.CertificateID != c.ID || !got.Enabled {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if err := s.DeleteCertificate(ctx, c.ID); err == nil || !strings.Contains(err.Error(), "stream") {
		t.Fatalf("certificate used by a stream deleted: %v", err)
	}
	got.UDP, got.TCP, got.CertificateID, got.Enabled = true, false, nil, false
	if err := s.SaveStream(ctx, got); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListStreams(ctx)
	if err != nil || len(list) != 1 || list[0].TCP || !list[0].UDP || list[0].CertificateID != nil || list[0].Enabled {
		t.Fatalf("list after update: %+v %v", list, err)
	}
	if err := s.DeleteStream(ctx, st.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StreamByID(ctx, st.ID); err != ErrNotFound {
		t.Fatalf("deleted stream still found: %v", err)
	}
	if err := s.DeleteStream(ctx, st.ID); err != ErrNotFound {
		t.Fatalf("delete missing: %v", err)
	}
}
