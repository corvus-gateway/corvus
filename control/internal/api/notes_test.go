package api

import (
	"testing"

	"corvus/control/internal/store"
)

// Omitting "notes" in an update keeps the current notes; sending it replaces them.
func TestNotesInput(t *testing.T) {
	s := ""
	note := "new"

	h := &store.ProxyHost{Notes: "keep"}
	proxyHostInput{}.apply(h)
	if h.Notes != "keep" {
		t.Fatalf("proxy host: omitted notes changed to %q", h.Notes)
	}
	proxyHostInput{Notes: &note}.apply(h)
	if h.Notes != "new" {
		t.Fatalf("proxy host: notes = %q", h.Notes)
	}

	r := &store.RedirectionHost{Notes: "keep"}
	redirectionHostInput{}.apply(r)
	if r.Notes != "keep" {
		t.Fatalf("redirection: omitted notes changed to %q", r.Notes)
	}
	redirectionHostInput{Notes: &s}.apply(r)
	if r.Notes != "" {
		t.Fatalf("redirection: notes = %q", r.Notes)
	}

	st := &store.Stream{Notes: "keep"}
	streamInput{}.apply(st)
	if st.Notes != "keep" {
		t.Fatalf("stream: omitted notes changed to %q", st.Notes)
	}
	streamInput{Notes: &note}.apply(st)
	if st.Notes != "new" {
		t.Fatalf("stream: notes = %q", st.Notes)
	}
}
