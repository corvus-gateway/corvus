package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"corvus/control/internal/npmimport"
	"corvus/control/internal/store"
)

func TestCheckStream(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	st := e.s.Store
	e.s.Paths.HTTPPort, e.s.Paths.HTTPSPort, e.s.Paths.AdminPort = 80, 443, 9443
	r := httptest.NewRequest("POST", "/api/streams", nil)

	if err := checkStream(r, st, e.s.Paths, &store.Stream{IncomingPort: 443, TCP: true, ForwardHost: "demo", ForwardPort: 80, Enabled: true}); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("reserved port accepted: %v", err)
	}
	if err := checkStream(r, st, e.s.Paths, &store.Stream{IncomingPort: 9443, UDP: true, ForwardHost: "demo", ForwardPort: 80}); err == nil || !strings.Contains(err.Error(), "admin") {
		t.Fatalf("admin port accepted: %v", err)
	}
	s1 := &store.Stream{IncomingPort: 5432, TCP: true, ForwardHost: "db", ForwardPort: 5432, Enabled: true}
	if err := checkStream(r, st, e.s.Paths, s1); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveStream(ctx, s1); err != nil {
		t.Fatal(err)
	}
	if err := checkStream(r, st, e.s.Paths, s1); err != nil {
		t.Fatalf("self conflict: %v", err)
	}
	dup := &store.Stream{IncomingPort: 5432, TCP: true, UDP: true, ForwardHost: "db2", ForwardPort: 5432, Enabled: true}
	if err := checkStream(r, st, e.s.Paths, dup); err == nil || !strings.Contains(err.Error(), "stream #1") {
		t.Fatalf("duplicate port accepted: %v", err)
	}
	udp := &store.Stream{IncomingPort: 5432, UDP: true, ForwardHost: "db2", ForwardPort: 5432, Enabled: true}
	if err := checkStream(r, st, e.s.Paths, udp); err != nil {
		t.Fatalf("same port, other protocol: %v", err)
	}
	missing := int64(42)
	if err := checkStream(r, st, e.s.Paths, &store.Stream{IncomingPort: 6000, TCP: true, ForwardHost: "db", ForwardPort: 1, CertificateID: &missing}); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("missing certificate accepted: %v", err)
	}
}

func TestStreamRoutes(t *testing.T) {
	e := newTestEnv(t)
	if w, _ := e.do("GET", "/api/streams", nil, withCookie(e.cookie)); w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	if w, _ := e.do("GET", "/api/streams/1", nil, withCookie(e.cookie)); w.Code != 404 {
		t.Fatalf("missing stream: %d %s", w.Code, w.Body)
	}
	if w, _ := e.do("GET", "/api/streams", nil); w.Code != 401 {
		t.Fatalf("anonymous list: %d", w.Code)
	}
}

func TestNPMStreamMapping(t *testing.T) {
	h := npmimport.Stream{NPMID: 3, IncomingPort: 5432, ForwardHost: "db", ForwardPort: 5433, TCP: true, UDP: true, CertificateID: 4, Enabled: true}
	x := npmStream(h, map[int64]int64{4: 9})
	if x.IncomingPort != 5432 || x.ForwardHost != "db" || x.ForwardPort != 5433 || !x.TCP || !x.UDP || !x.Enabled || x.CertificateID == nil || *x.CertificateID != 9 {
		t.Fatalf("mapping: %+v", x)
	}
	if x := npmStream(h, nil); x.CertificateID != nil {
		t.Fatalf("unmapped certificate: %+v", x)
	}
}
