package autoban

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
)

func setup(t *testing.T) (*Service, *traffic.Collector, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	tc := traffic.New()
	return &Service{Store: st, Traffic: tc}, tc, st
}

func attacks(tc *traffic.Collector, ip string, n int, at time.Time) {
	for i := 0; i < n; i++ {
		tc.Add(&traffic.Record{T: float64(at.Unix()) - float64(i), H: "shop.example.com", U: "/x", S: 403, IP: ip, X: "sqli"})
	}
}

func TestAutoBanThresholdExemptAndEscalation(t *testing.T) {
	s, tc, st := setup(t)
	ctx := context.Background()
	set := DefaultSettings()
	set.Exempt = []string{"10.0.0.0/8"}
	if err := Validate(&set); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(set)
	_ = st.SetMeta(ctx, "autoban", string(raw))

	now := time.Now()
	attacks(tc, "203.0.113.5", 25, now) // over threshold
	attacks(tc, "198.51.100.7", 5, now) // under threshold
	attacks(tc, "10.1.2.3", 50, now)    // exempt network
	got := s.Evaluate(ctx, now)
	if len(got) != 1 || got[0] != "203.0.113.5" {
		t.Fatalf("banned %v", got)
	}
	b, err := st.BanByIP(ctx, "203.0.113.5")
	if err != nil || b.Source != "auto" || b.ExpiresAt == nil || b.ExpiresAt.Sub(now) < 14*time.Minute || b.ExpiresAt.Sub(now) > 16*time.Minute {
		t.Fatalf("ban: %+v %v", b, err)
	}
	if again := s.Evaluate(ctx, now); len(again) != 0 {
		t.Fatalf("active ban re-created: %v", again)
	}
	// after the ban expires the same offender gets twice the duration
	later := now.Add(20 * time.Minute)
	attacks(tc, "203.0.113.5", 25, later)
	if got := s.Evaluate(ctx, later); len(got) != 1 {
		t.Fatalf("repeat offender not banned: %v", got)
	}
	b, _ = st.BanByIP(ctx, "203.0.113.5")
	if b.Offenses != 2 || b.ExpiresAt.Sub(later) < 29*time.Minute {
		t.Fatalf("escalation: %+v", b)
	}

	// an operator lifts the ban: the IP is not re-banned for detections from before
	b, _ = st.BanByIP(ctx, "203.0.113.5")
	liftedAt := later.UTC().Truncate(time.Second)
	if err := st.UpsertBan(ctx, &store.IPBan{IP: b.IP, Source: "auto", Offenses: 2, ExpiresAt: &liftedAt}); err != nil {
		t.Fatal(err)
	}
	if got := s.Evaluate(ctx, later.Add(time.Second)); len(got) != 0 {
		t.Fatalf("re-banned right after unban: %v", got)
	}
	_ = st.UpsertBan(ctx, &store.IPBan{IP: "203.0.113.5", Source: "auto", Offenses: 2, ExpiresAt: b.ExpiresAt})

	// push: the gateway receives the active list
	var body map[string]map[string]int64
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(200)
	}))
	defer gw.Close()
	s.PushURL = gw.URL
	if err := s.Push(ctx, later); err != nil {
		t.Fatal(err)
	}
	if exp, ok := body["ips"]["203.0.113.5"]; !ok || exp <= later.Unix() {
		t.Fatalf("push body %v", body)
	}

	// disabled → nothing new
	set.Enabled = false
	raw, _ = json.Marshal(set)
	_ = st.SetMeta(ctx, "autoban", string(raw))
	attacks(tc, "192.0.2.9", 40, later)
	if got := s.Evaluate(ctx, later); len(got) != 0 {
		t.Fatalf("banned while disabled: %v", got)
	}
}

func TestBanUntil(t *testing.T) {
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	set := DefaultSettings() // 15 min, doubling, max 7 days
	got := func(off int) time.Duration {
		u := banUntil(set, off, now)
		if u == nil {
			return -1
		}
		return u.Sub(now)
	}
	if got(1) != 15*time.Minute || got(2) != 30*time.Minute || got(5) != 4*time.Hour {
		t.Fatalf("escalation: %v %v %v", got(1), got(2), got(5))
	}
	if got(20) != 7*24*time.Hour {
		t.Fatalf("cap: %v", got(20))
	}
	set.MaxDurationS = 30 * 86400
	if got(20) != 30*24*time.Hour {
		t.Fatalf("30-day cap: %v", got(20))
	}
	set.PermanentAfter = 3
	if got(2) == -1 || got(3) != -1 || got(4) != -1 {
		t.Fatalf("permanent from offense 3: %v %v %v", got(2), got(3), got(4))
	}
}

func TestValidateDurations(t *testing.T) {
	s := DefaultSettings()
	s.MaxDurationS = 0
	if err := Validate(&s); err != nil || s.MaxDurationS != 7*86400 {
		t.Fatalf("default max: %v %d", err, s.MaxDurationS)
	}
	for _, bad := range []func(*Settings){
		func(s *Settings) { s.MaxDurationS = 31 * 86400 },
		func(s *Settings) { s.MaxDurationS = 60 }, // shorter than the first ban
		func(s *Settings) { s.PermanentAfter = 1 },
	} {
		s := DefaultSettings()
		bad(&s)
		if err := Validate(&s); err == nil {
			t.Fatalf("accepted invalid settings: %+v", s)
		}
	}
}
