package store

import (
	"context"
	"testing"
)

// Notes are stored and updated on proxy hosts, redirection hosts and streams.
func TestNotesRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	h := &ProxyHost{Domains: []string{"a.example.com"}, ForwardScheme: "http", ForwardHost: "web", ForwardPort: 80, Enabled: true, Notes: "shop front"}
	if err := s.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	h.Notes = "shop front\nowner: web team"
	if err := s.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ProxyHostByID(ctx, h.ID); err != nil || got.Notes != h.Notes {
		t.Fatalf("proxy host notes: %+v %v", got, err)
	}

	r := &RedirectionHost{Domains: []string{"old.example.com"}, ForwardScheme: "auto", ForwardDomain: "a.example.com", HTTPCode: 301, Enabled: true, Notes: "old name"}
	if err := s.SaveRedirectionHost(ctx, r); err != nil {
		t.Fatal(err)
	}
	if got, err := s.RedirectionHostByID(ctx, r.ID); err != nil || got.Notes != "old name" {
		t.Fatalf("redirection notes: %+v %v", got, err)
	}

	st := &Stream{IncomingPort: 5432, TCP: true, ForwardHost: "db", ForwardPort: 5432, Enabled: true, Notes: "postgres"}
	if err := s.SaveStream(ctx, st); err != nil {
		t.Fatal(err)
	}
	st.Notes = ""
	if err := s.SaveStream(ctx, st); err != nil {
		t.Fatal(err)
	}
	if got, err := s.StreamByID(ctx, st.ID); err != nil || got.Notes != "" {
		t.Fatalf("stream notes: %+v %v", got, err)
	}
}
