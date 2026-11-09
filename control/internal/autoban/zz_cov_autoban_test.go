package autoban

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"corvus/control/internal/traffic"
)

func TestCovValidateErrors(t *testing.T) {
	mut := func(f func(*Settings)) Settings { s := DefaultSettings(); f(&s); return s }
	bad := map[string]Settings{
		"threshold":    mut(func(s *Settings) { s.Threshold = 2 }),
		"window":       mut(func(s *Settings) { s.WindowS = 59 }),
		"duration":     mut(func(s *Settings) { s.DurationS = 59 }),
		"max < first":  mut(func(s *Settings) { s.MaxDurationS = 60 }),
		"permanent":    mut(func(s *Settings) { s.PermanentAfter = 1 }),
		"scan":         mut(func(s *Settings) { s.ScanThreshold = 19 }),
		"code":         mut(func(s *Settings) { s.BadBehaviourCodes = []int{200} }),
		"bb threshold": mut(func(s *Settings) { s.BadBehaviourThreshold = 2 }),
		"bb window":    mut(func(s *Settings) { s.BadBehaviourWindowS = 5 }),
		"exempt":       mut(func(s *Settings) { s.Exempt = []string{"not-a-cidr"} }),
		"too many code": mut(func(s *Settings) {
			s.BadBehaviourCodes = nil
			for c := 400; c < 431; c++ {
				s.BadBehaviourCodes = append(s.BadBehaviourCodes, c)
			}
		}),
	}
	for name, s := range bad {
		if err := Validate(&s); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	s := DefaultSettings()
	s.MaxDurationS, s.ScanThreshold, s.BadBehaviourCodes, s.BadBehaviourThreshold, s.BadBehaviourWindowS = 0, 0, []int{404, 403, 404}, 0, 0
	if err := Validate(&s); err != nil {
		t.Fatal(err)
	}
	if s.MaxDurationS != 7*86400 || s.ScanThreshold != 200 || len(s.BadBehaviourCodes) != 2 || s.BadBehaviourCodes[0] != 403 || s.BadBehaviourThreshold != 10 || s.BadBehaviourWindowS != 60 {
		t.Fatalf("defaults filled in: %+v", s)
	}
	s.BadBehaviourCodes = nil
	if err := Validate(&s); err != nil || len(s.BadBehaviourCodes) != len(DefaultBadBehaviourCodes) {
		t.Fatalf("empty codes → defaults: %v %v", s.BadBehaviourCodes, err)
	}
}

func TestCovScannerBan(t *testing.T) {
	s, tc, st := setup(t)
	ctx := context.Background()
	set := DefaultSettings()
	set.IncludeScans = true
	set.ScanThreshold = 20
	saveSettings(t, st, set)
	now := time.Now()
	for i := 0; i < 30; i++ {
		tc.Add(&traffic.Record{T: float64(now.Unix() - int64(i)), H: "h", U: "/wp-login.php", S: 404, IP: "203.0.113.50"})
		tc.Add(&traffic.Record{T: float64(now.Unix() - int64(i)), H: "h", U: "/", S: 404, IP: "2001:db8:1:2::" + string(rune('a'+i%6))})
	}
	// a busy but healthy client is not a scanner
	for i := 0; i < 100; i++ {
		tc.Add(&traffic.Record{T: float64(now.Unix() - int64(i%50)), H: "h", U: "/", S: 200, IP: "198.51.100.1"})
	}
	for i := 0; i < 25; i++ {
		tc.Add(&traffic.Record{T: float64(now.Unix() - int64(i)), H: "h", U: "/x", S: 404, IP: "198.51.100.1"})
	}
	got := s.Evaluate(ctx, now)
	joined := strings.Join(got, " ")
	if len(got) != 2 || !strings.Contains(joined, "203.0.113.50") || !strings.Contains(joined, "2001:db8:1:2::/64") {
		t.Fatalf("scanner bans: %v", got)
	}
	b, err := st.BanByIP(ctx, "203.0.113.50")
	if err != nil || !strings.HasPrefix(b.Reason, "possible scanner: 30 of 30") {
		t.Fatalf("ban: %+v %v", b, err)
	}
	// without IncludeScans nothing would have been banned for 404s alone
	set.IncludeScans = false
	saveSettings(t, st, set)
	s2, tc2, _ := setup(t)
	for i := 0; i < 30; i++ {
		tc2.Add(&traffic.Record{T: float64(now.Unix() - int64(i)), H: "h", U: "/x", S: 404, IP: "203.0.113.51"})
	}
	if got := s2.Evaluate(ctx, now); len(got) != 0 {
		t.Fatalf("scans ignored by default: %v", got)
	}
}

func TestCovRunPullsAndPushes(t *testing.T) {
	s, tc, st := setup(t)
	set := DefaultSettings()
	saveSettings(t, st, set)
	attacks(tc, "203.0.113.9", 25, time.Now())

	var mu sync.Mutex
	var pushes []map[string]any
	pulls := 0
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/bad-behaviour":
			pulls++
			_, _ = io.WriteString(w, `{"hits":[]}`)
		case "/bans":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			pushes = append(pushes, body)
		}
	}))
	defer gw.Close()
	s.PushURL, s.HitsURL, s.Client = gw.URL+"/bans", gw.URL+"/bad-behaviour", gw.Client()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(pushes)
		mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Run did not push")
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.Trigger() // a second round on demand
	s.Trigger() // coalesced
	deadline = time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(pushes)
		mu.Unlock()
		if n >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Trigger did not cause a round")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	ips, _ := pushes[0]["ips"].(map[string]any)
	if _, ok := ips[GatewayKey("203.0.113.9")]; !ok || pulls < 2 {
		t.Fatalf("first push %v, pulls %d", pushes[0], pulls)
	}
	if b, err := st.BanByIP(context.Background(), "203.0.113.9"); err != nil || b.Source != "auto" {
		t.Fatalf("ban stored by Run: %+v %v", b, err)
	}
}

func TestCovPullErrors(t *testing.T) {
	s, _, _ := setup(t)
	ctx := context.Background()
	if hits, err := s.Pull(ctx); hits != nil || err != nil {
		t.Fatal("no hits URL: nothing to pull")
	}
	if err := s.Push(ctx, time.Now()); err != nil {
		t.Fatal("no push URL: nothing to push")
	}
	body, code := "", 200
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}))
	defer gw.Close()
	s.HitsURL, s.PushURL = gw.URL, gw.URL
	code = 503
	if _, err := s.Pull(ctx); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("status: %v", err)
	}
	code, body = 200, "{"
	if _, err := s.Pull(ctx); err == nil {
		t.Fatal("bad JSON must fail")
	}
	s.HitsURL, s.PushURL = "://bad", "://bad"
	if _, err := s.Pull(ctx); err == nil {
		t.Fatal("bad URL must fail")
	}
	if err := s.Push(ctx, time.Now()); err == nil {
		t.Fatal("bad push URL must fail")
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	s.HitsURL, s.PushURL = dead.URL, dead.URL
	if _, err := s.Pull(ctx); err == nil {
		t.Fatal("unreachable gateway must fail")
	}
	if err := s.Push(ctx, time.Now()); err == nil {
		t.Fatal("unreachable gateway must fail")
	}
	if c := (&Service{}).client(); c.Timeout != 3*time.Second {
		t.Fatalf("default client timeout %v", c.Timeout)
	}
}
