package autoban

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/store"
)

func saveSettings(t *testing.T, st *store.Store, set Settings) {
	t.Helper()
	if err := Validate(&set); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(set)
	if err := st.SetMeta(context.Background(), "autoban", string(raw)); err != nil {
		t.Fatal(err)
	}
}

func TestBadBehaviourDefaultsAndValidation(t *testing.T) {
	d := DefaultSettings()
	if d.BadBehaviour || d.BadBehaviourThreshold != 10 || d.BadBehaviourWindowS != 60 ||
		!reflect.DeepEqual(d.BadBehaviourCodes, []int{400, 401, 403, 404, 405, 429, 444}) {
		t.Fatalf("defaults: %+v", d)
	}
	// settings saved before the feature existed (zero values) get the defaults
	s := DefaultSettings()
	s.BadBehaviourCodes, s.BadBehaviourThreshold, s.BadBehaviourWindowS = nil, 0, 0
	if err := Validate(&s); err != nil || s.BadBehaviourThreshold != 10 || s.BadBehaviourWindowS != 60 || len(s.BadBehaviourCodes) != 7 {
		t.Fatalf("zero values: %v %+v", err, s)
	}
	// codes are de-duplicated and sorted
	s = DefaultSettings()
	s.BadBehaviourCodes = []int{404, 403, 404}
	if err := Validate(&s); err != nil || !reflect.DeepEqual(s.BadBehaviourCodes, []int{403, 404}) {
		t.Fatalf("codes: %v %v", err, s.BadBehaviourCodes)
	}
	for _, bad := range []func(*Settings){
		func(s *Settings) { s.BadBehaviourCodes = []int{200} },
		func(s *Settings) { s.BadBehaviourCodes = []int{600} },
		func(s *Settings) { s.BadBehaviourThreshold = 2 },
		func(s *Settings) { s.BadBehaviourWindowS = 5 },
		func(s *Settings) { s.BadBehaviourWindowS = 7200 },
	} {
		s := DefaultSettings()
		bad(&s)
		if err := Validate(&s); err == nil {
			t.Fatalf("accepted invalid settings: %+v", s)
		}
	}
}

func TestHitReason(t *testing.T) {
	h := Hit{Host: "example.com", WindowS: 60, Counts: map[string]int64{"404": 12}}
	if got := h.Reason(); got != "bad behaviour: 12×404 on example.com in 60s" {
		t.Fatalf("reason %q", got)
	}
	h.Counts = map[string]int64{"403": 3, "404": 7, "401": 3}
	if got := h.Reason(); got != "bad behaviour: 7×404, 3×401, 3×403 on example.com in 60s" {
		t.Fatalf("reason %q", got)
	}
}

func TestRecordHitsEscalationExemptAndAck(t *testing.T) {
	s, _, st := setup(t)
	ctx := context.Background()
	now := time.Now()
	set := DefaultSettings()
	set.BadBehaviour = true
	set.Exempt = []string{"10.0.0.0/8"}
	saveSettings(t, st, set)

	hits := []Hit{
		{ID: "a1", IP: "203.0.113.5", Host: "example.com", WindowS: 60, Counts: map[string]int64{"404": 12}},
		{ID: "b1", IP: "10.1.2.3", Host: "example.com", WindowS: 60, Counts: map[string]int64{"404": 10}}, // exempt
		{ID: "c1", IP: "not-an-ip", Host: "example.com"},
	}
	got := s.RecordHits(ctx, now, hits)
	if len(got) != 1 || got[0] != "203.0.113.5" {
		t.Fatalf("banned %v", got)
	}
	b, err := st.BanByIP(ctx, "203.0.113.5")
	if err != nil || b.Source != "auto" || b.Offenses != 1 || b.Reason != "bad behaviour: 12×404 on example.com in 60s" ||
		b.ExpiresAt == nil || b.ExpiresAt.Sub(now) < 14*time.Minute || b.ExpiresAt.Sub(now) > 16*time.Minute {
		t.Fatalf("ban: %+v %v", b, err)
	}
	if _, err := st.BanByIP(ctx, "10.1.2.3"); err == nil {
		t.Fatal("exempt IP banned")
	}
	// every processed hit is acknowledged (recorded or skipped)
	want := map[string]string{"203.0.113.5": "a1", "10.1.2.3": "b1", "not-an-ip": "c1"}
	if !reflect.DeepEqual(s.acks, want) {
		t.Fatalf("acks %v", s.acks)
	}

	// the same hit reported again (push failed) is not recorded twice
	if got := s.RecordHits(ctx, now, hits[:1]); len(got) != 0 {
		t.Fatalf("recorded twice: %v", got)
	}
	if b2, _ := st.BanByIP(ctx, "203.0.113.5"); b2.Offenses != 1 {
		t.Fatalf("offenses changed: %+v", b2)
	}

	// repeat offender after the ban expired: doubled duration
	later := now.Add(20 * time.Minute)
	if got := s.RecordHits(ctx, later, []Hit{{ID: "a2", IP: "203.0.113.5", Host: "example.com", WindowS: 60, Counts: map[string]int64{"403": 10}}}); len(got) != 1 {
		t.Fatalf("repeat offender not banned: %v", got)
	}
	b, _ = st.BanByIP(ctx, "203.0.113.5")
	if b.Offenses != 2 || b.ExpiresAt.Sub(later) < 29*time.Minute {
		t.Fatalf("escalation: %+v", b)
	}

	// bad behaviour switched off: hits are acknowledged but not recorded
	set.BadBehaviour = false
	saveSettings(t, st, set)
	if got := s.RecordHits(ctx, later, []Hit{{ID: "d1", IP: "198.51.100.9", Host: "example.com", WindowS: 60}}); len(got) != 0 {
		t.Fatalf("banned while disabled: %v", got)
	}
	if s.acks["198.51.100.9"] != "d1" {
		t.Fatalf("disabled hit not acknowledged: %v", s.acks)
	}
}

func TestPullAndPushAcknowledge(t *testing.T) {
	s, _, st := setup(t)
	ctx := context.Background()
	now := time.Now()
	set := DefaultSettings()
	set.BadBehaviour = true
	set.BadBehaviourThreshold = 5
	set.BadBehaviourCodes = []int{404}
	set.Exempt = []string{"192.0.2.0/24"}
	saveSettings(t, st, set)

	var pushed struct {
		IPs map[string]int64  `json:"ips"`
		Ack map[string]string `json:"ack"`
		BB  struct {
			Enabled   bool     `json:"enabled"`
			Codes     []int    `json:"codes"`
			Threshold int      `json:"threshold"`
			Window    int      `json:"window"`
			Duration  int      `json:"duration"`
			Exempt    []string `json:"exempt"`
		} `json:"bb"`
	}
	pushStatus := 500
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad-behaviour":
			_, _ = io.WriteString(w, `{"hits":[{"id":"x1","ip":"203.0.113.7","host":"shop.example.com","counts":{"404":5},"window":60,"t":1}]}`)
		case "/bans":
			raw, _ := io.ReadAll(r.Body)
			pushed.Ack = nil
			_ = json.Unmarshal(raw, &pushed)
			w.WriteHeader(pushStatus)
		}
	}))
	defer gw.Close()
	s.PushURL, s.HitsURL = gw.URL+"/bans", gw.URL+"/bad-behaviour"

	hits, err := s.Pull(ctx)
	if err != nil || len(hits) != 1 || hits[0].Counts["404"] != 5 {
		t.Fatalf("pull: %v %+v", err, hits)
	}
	s.RecordHits(ctx, now, hits)

	// push fails: the acknowledgement is kept for the next push
	if err := s.Push(ctx, now); err == nil {
		t.Fatal("push error not reported")
	}
	if s.acks["203.0.113.7"] != "x1" {
		t.Fatalf("ack dropped after failed push: %v", s.acks)
	}
	pushStatus = 200
	if err := s.Push(ctx, now); err != nil {
		t.Fatal(err)
	}
	if pushed.Ack["203.0.113.7"] != "x1" || pushed.IPs["203.0.113.7"] <= now.Unix() {
		t.Fatalf("push body: %+v", pushed)
	}
	if !pushed.BB.Enabled || pushed.BB.Threshold != 5 || pushed.BB.Window != 60 || pushed.BB.Duration != 900 ||
		!reflect.DeepEqual(pushed.BB.Codes, []int{404}) {
		t.Fatalf("bb config: %+v", pushed.BB)
	}
	if strings.Join(pushed.BB.Exempt, " ") != "7f000000/8 00000000000000000000000000000001/128 c0000200/24" {
		t.Fatalf("exempt networks: %v", pushed.BB.Exempt)
	}
	if len(s.acks) != 0 {
		t.Fatalf("acks not cleared after push: %v", s.acks)
	}
	if err := s.Push(ctx, now); err != nil || len(pushed.Ack) != 0 {
		t.Fatalf("ack sent twice: %v %v", err, pushed.Ack)
	}

	// automatic bans disabled: the gateway is told to stop counting
	set.Enabled = false
	saveSettings(t, st, set)
	if err := s.Push(ctx, now); err != nil || pushed.BB.Enabled {
		t.Fatalf("bb still enabled: %v %+v", err, pushed.BB)
	}
}
