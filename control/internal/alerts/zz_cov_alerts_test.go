package alerts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/live"
	"corvus/control/internal/store"
)

func TestCovValidators(t *testing.T) {
	for _, k := range Kinds {
		if !ValidKind(k) {
			t.Errorf("%s invalid", k)
		}
	}
	if ValidKind("email") || ValidKind("") {
		t.Fatal("unknown kinds")
	}
	for _, s := range []string{"info", "warning", "critical"} {
		if !ValidSeverity(s) {
			t.Errorf("%s invalid", s)
		}
	}
	if ValidSeverity("fatal") {
		t.Fatal("unknown severity")
	}

	ok := map[string]Target{
		"slack":    {URL: "https://hooks.slack.com/services/x"},
		"webhook":  {URL: "http://10.0.0.5:8080/hook"},
		"telegram": {BotToken: "123:abc", ChatID: "-100"},
	}
	for k, tg := range ok {
		if err := CheckTarget(k, tg); err != nil {
			t.Errorf("%s: %v", k, err)
		}
	}
	bad := []struct {
		kind string
		t    Target
		want string
	}{
		{"telegram", Target{BotToken: "1:a"}, "required"},
		{"telegram", Target{BotToken: "1:a/../x", ChatID: "1"}, "invalid bot token"},
		{"slack", Target{URL: "ftp://x/y"}, "webhook URL"},
		{"slack", Target{URL: "not a url"}, "webhook URL"},
		{"slack", Target{URL: "https://user:pw@example.com/h"}, "credentials"},
		{"slack", Target{URL: "http://127.0.0.1/h"}, "loopback"},
		{"slack", Target{URL: "http://LOCALHOST:80/h"}, "loopback"},
		{"discord", Target{URL: "http://169.254.169.254/x"}, "loopback"},
		{"teams", Target{URL: "http://[::1]/x"}, "loopback"},
	}
	for _, b := range bad {
		if err := CheckTarget(b.kind, b.t); err == nil || !strings.Contains(err.Error(), b.want) {
			t.Errorf("%s %+v: %v (want %q)", b.kind, b.t, err, b.want)
		}
	}

	if h := Hint("telegram", Target{ChatID: "-100", BotToken: "secret"}); h != "chat -100" {
		t.Fatalf("telegram hint %q", h)
	}
	if h := Hint("slack", Target{URL: "https://hooks.slack.com/services/T000/B000/XXXXsecret"}); h != "hooks.slack.com/…/cret" || strings.Contains(h, "XXXX") {
		t.Fatalf("slack hint %q", h)
	}
	if h := Hint("slack", Target{URL: "https://h.example/a"}); h != "h.example/…/a" {
		t.Fatalf("short path hint %q", h)
	}
	if h := Hint("slack", Target{URL: "%zz"}); h != "" {
		t.Fatalf("invalid url hint %q", h)
	}
}

func TestCovSettingsValidate(t *testing.T) {
	s := DefaultSettings()
	s.PublicURL = "  https://corvus.example.com/  "
	delete(s.Rules, "exploit")
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if s.PublicURL != "https://corvus.example.com" {
		t.Fatalf("public url normalized to %q", s.PublicURL)
	}
	if _, ok := s.Rules["exploit"]; !ok {
		t.Fatal("omitted rules get defaults")
	}
	mut := []struct {
		name string
		f    func(*Settings)
		want string
	}{
		{"cooldown low", func(s *Settings) { s.CooldownS = 10 }, "cooldown"},
		{"cooldown high", func(s *Settings) { s.CooldownS = 90000 }, "cooldown"},
		{"language", func(s *Settings) { s.Language = "de" }, "language"},
		{"url scheme", func(s *Settings) { s.PublicURL = "ftp://x" }, "public_url"},
		{"url host", func(s *Settings) { s.PublicURL = "https://" }, "public_url"},
		{"unknown rule", func(s *Settings) { s.Rules["nope"] = Rule{Severity: "info"} }, "unknown rule"},
		{"severity", func(s *Settings) { s.Rules["exploit"] = Rule{Severity: "loud"} }, "invalid severity"},
		{"range", func(s *Settings) { s.Rules["exploit"] = Rule{Severity: "info", Threshold: -1} }, "out of range"},
		{"window", func(s *Settings) { s.Rules["exploit"] = Rule{Severity: "info", WindowS: 30} }, "at least 60"},
	}
	for _, m := range mut {
		s := DefaultSettings()
		m.f(&s)
		if err := s.Validate(); err == nil || !strings.Contains(err.Error(), m.want) {
			t.Errorf("%s: %v", m.name, err)
		}
	}
}

func TestCovHelpers(t *testing.T) {
	for rule, want := range map[string]string{
		"cert_expiry": "/certificates", "acme_failed": "/certificates", "deploy_failed": "/deployments",
		"login_failed": "/audit", "waitroom_queue": "/waitrooms", "redis_down": "/waitrooms",
		"under_attack": "/proxy-hosts", "error_rate": "/traffic",
	} {
		if got := link("https://c", rule); got != "https://c"+want {
			t.Errorf("link(%s) = %q", rule, got)
		}
	}
	if link("", "error_rate") != "" {
		t.Fatal("no base, no link")
	}
	if trim("abcdef", 3) != "abc…" || trim("ab", 3) != "ab" {
		t.Fatal("trim")
	}
	for _, c := range []struct {
		d    time.Duration
		lang string
		want string
	}{{10 * time.Second, "en", "under a minute"}, {5 * time.Minute, "en", "5 min"}, {10 * time.Second, "ko", "1분 미만"}, {3 * time.Minute, "ko", "3분"}} {
		if got := humanDuration(c.d, c.lang); got != c.want {
			t.Errorf("humanDuration(%v,%s) = %q", c.d, c.lang, got)
		}
	}
	if _, _, err := payload("pager", Target{}, Message{}); err == nil {
		t.Fatal("unsupported kind")
	}
	for _, m := range []Message{{Status: "resolved"}, {Status: "test"}, {Severity: "critical"}, {Severity: "warning"}, {Severity: "info"}} {
		h, n := color(m)
		if h == "" || n == 0 || icon(m) == "" {
			t.Errorf("color/icon for %+v", m)
		}
	}
	if !contains([]string{"a", "b"}, "b") || contains(nil, "a") {
		t.Fatal("contains")
	}
}

func TestCovSendErrors(t *testing.T) {
	allowLoopback = true
	t.Cleanup(func() { allowLoopback = false })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "  channel_not_found  ", http.StatusNotFound)
	}))
	defer srv.Close()
	m := Message{Title: "t", Time: time.Now()}
	code, err := Send(context.Background(), "discord", Target{URL: srv.URL}, m)
	if code != 404 || err == nil || err.Error() != "HTTP 404" {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if _, err := Send(context.Background(), "slack", Target{URL: "http://\x7f"}, m); err == nil {
		t.Fatal("invalid url")
	}
	if _, err := Send(context.Background(), "pager", Target{}, m); err == nil {
		t.Fatal("unsupported kind")
	}
	// transport errors never contain the Telegram bot token
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Send(ctx, "telegram", Target{BotToken: "999:SECRETTOKEN", ChatID: "1"}, m)
	if err == nil || strings.Contains(err.Error(), "SECRETTOKEN") || !strings.Contains(err.Error(), "api.telegram.org/…") {
		t.Fatalf("token leak: %v", err)
	}
}

func TestCovEngineTestAndCorruptTarget(t *testing.T) {
	e, _, h, _ := setup(t)
	ctx := context.Background()
	chans, _ := e.Store.ListAlertChannels(ctx)
	res := e.Test(ctx, chans[0])
	if !res.OK || res.Channel != "slack" || h.n() != 1 {
		t.Fatalf("test delivery: %+v n=%d", res, h.n())
	}
	hist, _ := e.Store.ListAlerts(ctx, 5)
	if len(hist) != 1 || hist[0].Status != "test" || len(hist[0].Results) != 1 || !hist[0].Results[0].OK {
		t.Fatalf("history: %+v", hist)
	}
	bad := chans[0]
	bad.Target = "not-sealed"
	res = e.Test(ctx, bad)
	if res.OK || res.Error == "" || h.n() != 1 {
		t.Fatalf("corrupt target: %+v", res)
	}
}

func TestCovRoomsRedisCertAndResolved(t *testing.T) {
	e, _, h, _ := setup(t)
	ctx := context.Background()
	set := DefaultSettings()
	set.Language = "ko"
	set.PublicURL = "https://corvus.example"
	set.CooldownS = 60
	r := set.Rules["waitroom_queue"]
	r.Severity = "warning"
	set.Rules["waitroom_queue"] = r
	b, _ := json.Marshal(set)
	_ = e.Store.SetMeta(ctx, "alert_settings", string(b))

	waiting := int64(5)
	redisUp := false
	e.Src.Rooms = func(context.Context) []live.Stats {
		return []live.Stats{{Name: "sale", Available: true, Waiting: waiting, Active: 10, Capacity: 10}, {Name: "off", Available: false, Waiting: 99}}
	}
	e.Src.RedisUp = func(context.Context) bool { return redisUp }
	// a custom certificate expiring in 3 days
	_ = e.Store.CreateCertificate(ctx, &store.Certificate{Name: "c", Kind: "custom", Domains: []string{"a.example"}, NotAfter: time.Now().Add(72 * time.Hour)})

	now := time.Now()
	evalWait(e, ctx, now)
	act := e.Active()
	rules := map[string]bool{}
	for _, a := range act {
		rules[a.Rule] = true
	}
	if !rules["waitroom_queue"] || !rules["redis_down"] || !rules["cert_expiry"] || len(act) != 3 {
		t.Fatalf("active: %+v", act)
	}
	if h.n() != 3 { // queue now warning, redis critical, cert warning
		t.Fatalf("notifications %d", h.n())
	}
	h.mu.Lock()
	var sawLink bool
	for _, m := range h.msgs {
		att := m["attachments"].([]any)[0].(map[string]any)
		if att["title_link"] == "https://corvus.example/waitrooms" {
			sawLink = true
		}
	}
	h.mu.Unlock()
	if !sawLink {
		t.Fatal("public URL link missing")
	}

	// conditions clear: resolved after two rounds, in Korean
	waiting, redisUp = 0, true
	evalWait(e, ctx, now.Add(10*time.Second))
	evalWait(e, ctx, now.Add(20*time.Second))
	h.mu.Lock()
	n := len(h.msgs)
	last := h.msgs[n-1]["attachments"].([]any)[0].(map[string]any)
	h.mu.Unlock()
	if n != 5 || last["color"] != "#16a34a" {
		t.Fatalf("resolved notifications: n=%d last=%v", n, last)
	}
	if left := e.Active(); len(left) != 1 || left[0].Rule != "cert_expiry" {
		t.Fatalf("still active: %+v", left)
	}
	// after the cooldown the remaining alert is repeated as ongoing
	evalWait(e, ctx, now.Add(2*time.Minute))
	if h.n() != 6 {
		t.Fatalf("ongoing reminder: %d", h.n())
	}
}

func TestCovRun(t *testing.T) {
	e, _, _, _ := setup(t)
	e.Interval = 10 * time.Millisecond
	ctx := context.Background()
	v := &store.ConfigVersion{Message: "old failure", Status: "failed"}
	_ = e.Store.CreateVersion(ctx, v)
	cctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { e.Run(cctx); close(done) }()
	time.Sleep(60 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop")
	}
	e.mu.Lock()
	lv := e.lastVersion
	e.mu.Unlock()
	if lv != v.ID {
		t.Fatalf("failures before start-up must be skipped: lastVersion=%d", lv)
	}
	if hist, _ := e.Store.ListAlerts(ctx, 10); len(hist) != 0 {
		t.Fatalf("old failure alerted: %+v", hist)
	}
}
