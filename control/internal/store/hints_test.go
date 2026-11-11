package store

import (
	"context"
	"testing"
	"time"
)

// Error hints are derived from the stored messages when they are read.
func TestErrorHintsOnRead(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	c := &Certificate{Name: "c", Kind: "acme", Domains: []string{"a.example.com"}, NotAfter: time.Now(), Status: "error",
		LastError: "acme: error: 429 :: urn:ietf:params:acme:error:rateLimited :: too many certificates"}
	ok := &Certificate{Name: "ok", Kind: "custom", Domains: []string{"b.example.com"}, NotAfter: time.Now()}
	for _, x := range []*Certificate{c, ok} {
		if err := s.CreateCertificate(ctx, x); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := s.CertificateByID(ctx, c.ID); err != nil || got.LastErrorHint != "acme.rate_limited" {
		t.Errorf("certificate hint = %+v, %v", got, err)
	}
	if got, _ := s.CertificateByID(ctx, ok.ID); got.LastErrorHint != "" {
		t.Errorf("hint without error: %q", got.LastErrorHint)
	}

	l := &IPList{Name: "feed", Kind: ListKindIP, SourceURL: "https://lists.example/x.txt", Items: []string{}}
	if err := s.SaveIPList(ctx, l); err != nil {
		t.Fatal(err)
	}
	if err := s.SetIPListFetch(ctx, l.ID, ListFetch{CheckedAt: time.Now(), Error: "dial tcp: lookup lists.example: no such host"}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.IPListByID(ctx, l.ID); err != nil || got.FetchErrorHint != "net.dns" {
		t.Errorf("list hint = %+v, %v", got, err)
	}

	if err := s.AddAlert(ctx, &AlertRecord{Rule: "test", Status: "test", Results: []DeliveryResult{
		{ChannelID: 1, Error: "HTTP 401"}, {ChannelID: 2, OK: true}}}); err != nil {
		t.Fatal(err)
	}
	al, err := s.ListAlerts(ctx, 10)
	if err != nil || len(al) != 1 || al[0].Results[0].Hint != "net.auth" || al[0].Results[1].Hint != "" {
		t.Errorf("alert hints = %+v, %v", al, err)
	}
}
