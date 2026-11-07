package underattack

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"corvus/control/internal/store"
)

// fakeTraffic returns fixed per-host samples (requests, blocked over 30 s).
type fakeTraffic map[string][2]int64

func (f fakeTraffic) Window(host string, d time.Duration, now time.Time) (int64, int64, int64) {
	s := f[host]
	return s[0], s[1], int64(d.Seconds())
}

func openStore(t *testing.T, path string) *store.Store {
	t.Helper()
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func addHost(t *testing.T, st *store.Store, domain, mode string, cfg store.UnderAttackConfig) *store.ProxyHost {
	t.Helper()
	h := &store.ProxyHost{Domains: []string{domain}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, WaitroomPath: "/",
		ClientIP: "direct", BodyInspection: "log", BodyMaxKB: 1024, Enabled: true, UnderAttack: mode, UnderAttackConfig: cfg}
	if err := st.SaveProxyHost(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestAutoActivationHysteresis(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corvus.db")
	st := openStore(t, path)
	h := addHost(t, st, "shop.example.com", "auto", store.UnderAttackConfig{}) // defaults: 200 r/s over 30 s, 50 %, 15 min
	addHost(t, st, "manual.example.com", "on", store.UnderAttackConfig{})
	addHost(t, st, "off.example.com", "off", store.UnderAttackConfig{})
	tr := fakeTraffic{"shop.example.com": {3000, 0}, "manual.example.com": {90000, 0}, "off.example.com": {90000, 0}} // 100 r/s
	s := &Service{Store: st, Traffic: tr}
	t0 := time.Unix(1_800_000_000, 0)

	if got := s.Evaluate(ctx, t0); len(got) != 0 {
		t.Fatalf("below threshold: %+v", got)
	}
	tr["shop.example.com"] = [2]int64{9000, 0} // 300 r/s
	got := s.Evaluate(ctx, t0.Add(10*time.Second))
	if len(got) != 1 || !got[0].On || got[0].HostID != h.ID || got[0].Host != "shop.example.com" {
		t.Fatalf("activation: %+v", got)
	}
	if got := s.Evaluate(ctx, t0.Add(20*time.Second)); len(got) != 0 {
		t.Fatalf("still over: no new transition, got %+v", got)
	}
	// still over the threshold at +10 min: the period is extended from there
	if got := s.Evaluate(ctx, t0.Add(10*time.Minute)); len(got) != 0 {
		t.Fatal(got)
	}
	tr["shop.example.com"] = [2]int64{300, 0}
	// 15 min after the activation, but only 14 min after the last hot round: still active
	if got := s.Evaluate(ctx, t0.Add(15*time.Minute+10*time.Second)); len(got) != 0 {
		t.Fatalf("hysteresis: deactivated too early: %+v", got)
	}
	sts, _ := s.Statuses(ctx, t0.Add(15*time.Minute+10*time.Second))
	if len(sts) != 2 || !sts[0].Active || sts[0].Mode != "auto" || sts[0].Until == nil || !sts[1].Active || sts[1].Mode != "on" {
		t.Fatalf("statuses: %+v", sts)
	}
	got = s.Evaluate(ctx, t0.Add(25*time.Minute))
	if len(got) != 1 || got[0].On || got[0].Manual {
		t.Fatalf("deactivation: %+v", got)
	}
	if d := s.Drain(); len(d) != 2 || !d[0].On || d[1].On {
		t.Fatalf("drain: %+v", d)
	}
	if d := s.Drain(); len(d) != 0 {
		t.Fatalf("drained twice: %+v", d)
	}
}

func TestBlockedRatio(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, filepath.Join(t.TempDir(), "corvus.db"))
	zero := 0.0
	addHost(t, st, "a.example.com", "auto", store.UnderAttackConfig{})
	addHost(t, st, "b.example.com", "auto", store.UnderAttackConfig{BlockedPct: &zero})
	addHost(t, st, "c.example.com", "auto", store.UnderAttackConfig{})
	tr := fakeTraffic{
		"a.example.com": {600, 400}, // 20 r/s, 67 % blocked
		"b.example.com": {600, 400}, // ratio disabled
		"c.example.com": {40, 40},   // too few requests to judge
	}
	s := &Service{Store: st, Traffic: tr}
	got := s.Evaluate(ctx, time.Unix(1_800_000_000, 0))
	if len(got) != 1 || got[0].Host != "a.example.com" {
		t.Fatalf("ratio: %+v", got)
	}
}

func TestPersistenceAndManualDeactivation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corvus.db")
	st := openStore(t, path)
	h := addHost(t, st, "shop.example.com", "auto", store.UnderAttackConfig{RPS: 10, DurationS: 600})
	tr := fakeTraffic{"shop.example.com": {3000, 0}}
	t0 := time.Unix(1_800_000_000, 0)
	if got := (&Service{Store: st, Traffic: tr}).Evaluate(ctx, t0); len(got) != 1 {
		t.Fatal(got)
	}

	// a restarted control plane (new service, same store) knows the activation
	s := &Service{Store: st, Traffic: fakeTraffic{}}
	sts, err := s.Statuses(ctx, t0.Add(time.Minute))
	if err != nil || len(sts) != 1 || !sts[0].Active || !sts[0].Until.Equal(t0.Add(10*time.Minute)) {
		t.Fatalf("after restart: %+v %v", sts, err)
	}
	if got := s.Evaluate(ctx, t0.Add(time.Minute)); len(got) != 0 {
		t.Fatalf("after restart: re-activated or ended: %+v", got)
	}

	// manual deactivation, then no automatic re-activation for one period
	s.Traffic = tr
	if err := s.Deactivate(ctx, h.ID, t0.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.Deactivate(ctx, h.ID, t0.Add(2*time.Minute)); err != ErrNotActive {
		t.Fatalf("second deactivate: %v", err)
	}
	if d := s.Drain(); len(d) != 1 || d[0].On || !d[0].Manual {
		t.Fatalf("manual transition: %+v", d)
	}
	if got := s.Evaluate(ctx, t0.Add(5*time.Minute)); len(got) != 0 {
		t.Fatalf("re-activated during hold: %+v", got)
	}
	if got := s.Evaluate(ctx, t0.Add(13*time.Minute)); len(got) != 1 || !got[0].On {
		t.Fatalf("after hold: %+v", got)
	}

	// switching the host away from auto ends the activation silently
	h.UnderAttack = "off"
	if err := st.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	if got := s.Evaluate(ctx, t0.Add(14*time.Minute)); len(got) != 0 {
		t.Fatal(got)
	}
	if p := s.Payload(ctx, t0.Add(14*time.Minute)); len(p["hosts"].(map[string]int64)) != 0 {
		t.Fatalf("payload after mode off: %+v", p)
	}
}

func TestPush(t *testing.T) {
	ctx := context.Background()
	st := openStore(t, filepath.Join(t.TempDir(), "corvus.db"))
	if err := st.SetMeta(ctx, "autoban", `{"enabled":false,"exempt":["10.1.0.0/16"]}`); err != nil {
		t.Fatal(err)
	}
	h := addHost(t, st, "shop.example.com", "auto", store.UnderAttackConfig{})
	var body []byte
	var method string
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		body, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte("1\n"))
	}))
	defer gw.Close()
	s := &Service{Store: st, Traffic: fakeTraffic{"shop.example.com": {9000, 0}}, PushURL: gw.URL}
	t0 := time.Unix(1_800_000_000, 0)
	s.Evaluate(ctx, t0)
	if err := s.Push(ctx, t0); err != nil {
		t.Fatal(err)
	}
	var p struct {
		Hosts  map[string]int64 `json:"hosts"`
		Exempt []string         `json:"exempt"`
		Now    int64            `json:"now"`
	}
	if err := json.Unmarshal(body, &p); err != nil || method != http.MethodPut {
		t.Fatalf("%s %s %v", method, body, err)
	}
	want := t0.Add(15 * time.Minute).Unix()
	if p.Hosts[strconv.FormatInt(h.ID, 10)] != want || len(p.Hosts) != 1 || p.Now != t0.Unix() {
		t.Fatalf("hosts: %s", body)
	}
	if len(p.Exempt) != 3 || p.Exempt[0] != "7f000000/8" || p.Exempt[2] != "0a010000/16" {
		t.Fatalf("exempt: %v", p.Exempt)
	}
	// after the period the host is no longer pushed (the gateway entry also expires)
	if p := s.Payload(ctx, t0.Add(16*time.Minute)); len(p["hosts"].(map[string]int64)) != 0 {
		t.Fatalf("expired activation pushed: %+v", p)
	}
}
