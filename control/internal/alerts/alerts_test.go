package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"corvus/control/internal/seal"
	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
)

type hook struct {
	mu   sync.Mutex
	msgs []map[string]any
	hdr  []http.Header
}

func (h *hook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	h.mu.Lock()
	h.msgs = append(h.msgs, m)
	h.hdr = append(h.hdr, r.Header.Clone())
	h.mu.Unlock()
}

func (h *hook) n() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.msgs) }

func setup(t *testing.T) (*Engine, *traffic.Collector, *hook, *httptest.Server) {
	t.Helper()
	allowLoopback = true
	t.Cleanup(func() { allowLoopback = false })
	st, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	key := make([]byte, 32)
	h := &hook{}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	tgt, _ := json.Marshal(Target{URL: srv.URL + "/hooks/abc"})
	sealed, _ := seal.Seal(key, string(tgt))
	if err := st.SaveAlertChannel(context.Background(), &store.AlertChannel{Name: "slack", Kind: "slack", Target: sealed, Enabled: true, MinSeverity: "warning"}); err != nil {
		t.Fatal(err)
	}
	tc := traffic.New()
	return &Engine{Store: st, Key: key, Src: Sources{Traffic: tc}}, tc, h, srv
}

func feed(tc *traffic.Collector, at time.Time, n int, status int) {
	for i := 0; i < n; i++ {
		tc.Add(&traffic.Record{T: float64(at.Unix()) - float64(i%60), H: "shop.example.com", U: "/", S: status, RT: 0.01, IP: "1.2.3.4"})
	}
}

func TestErrorRateFireCooldownResolve(t *testing.T) {
	e, tc, h, _ := setup(t)
	ctx := context.Background()
	now := time.Now()
	feed(tc, now, 80, 200)
	feed(tc, now, 20, 502) // 20% 5xx
	e.Evaluate(ctx, now)
	if h.n() != 1 {
		t.Fatalf("expected 1 firing notification, got %d", h.n())
	}
	att := h.msgs[0]["attachments"].([]any)[0].(map[string]any)
	if !strings.Contains(att["title"].(string), "5xx") || att["color"] != "#dc2626" {
		t.Fatalf("slack payload: %v", att)
	}
	if len(e.Active()) != 1 {
		t.Fatal("alert not active")
	}
	e.Evaluate(ctx, now.Add(time.Minute)) // within cooldown → silent
	if h.n() != 1 {
		t.Fatalf("cooldown violated: %d messages", h.n())
	}
	e.Evaluate(ctx, now.Add(20*time.Minute)) // window passed, traffic gone: one quiet round is not enough
	if h.n() != 1 {
		t.Fatalf("resolved after a single quiet round: %d messages", h.n())
	}
	e.Evaluate(ctx, now.Add(20*time.Minute+30*time.Second)) // second quiet round → resolved
	if h.n() != 2 || !strings.Contains(h.msgs[1]["attachments"].([]any)[0].(map[string]any)["title"].(string), "Resolved") {
		t.Fatalf("expected resolved message, got %d", h.n())
	}
	hist, _ := e.Store.ListAlerts(ctx, 10)
	if len(hist) != 2 || hist[0].Status != "resolved" || !hist[1].Results[0].OK {
		t.Fatalf("history: %+v", hist)
	}
}

func TestSeverityAndEventFilters(t *testing.T) {
	e, tc, h, _ := setup(t)
	ctx := context.Background()
	chans, _ := e.Store.ListAlertChannels(ctx)
	c := chans[0]
	c.Events = []string{"latency_p95"} // only latency
	_ = e.Store.SaveAlertChannel(ctx, &c)
	now := time.Now()
	feed(tc, now, 60, 500)
	e.Evaluate(ctx, now)
	if h.n() != 0 {
		t.Fatal("event filter ignored")
	}
}

func TestDeployFailedOneShotAndLogin(t *testing.T) {
	e, _, h, _ := setup(t)
	ctx := context.Background()
	e.init()
	v := &store.ConfigVersion{Message: "bad", Status: "failed", TestOutput: "nginx: [emerg] unknown directive"}
	_ = e.Store.CreateVersion(ctx, v)
	for i := 0; i < 12; i++ {
		_ = e.Store.Audit(ctx, nil, "auth.login_failed", "x@y.z", nil, "9.9.9.9")
	}
	now := time.Now()
	e.Evaluate(ctx, now)
	if h.n() != 2 {
		t.Fatalf("want deploy_failed + login_failed, got %d", h.n())
	}
	e.Evaluate(ctx, now.Add(30*time.Second))
	if h.n() != 2 {
		t.Fatalf("one-shot deploy alert repeated: %d", h.n())
	}
}

func TestPayloadsPerKind(t *testing.T) {
	m := Message{Rule: "exploit", Severity: "warning", Status: "firing", Title: "T <b>", Text: "x & y", Fields: [][2]string{{"Host", "a"}}, Time: time.Now(), Link: "https://corvus/traffic"}
	for _, k := range Kinds {
		dest, body, err := payload(k, Target{URL: "https://example.com/h", BotToken: "123:abc", ChatID: "-100"}, m)
		if err != nil || len(body) == 0 || dest == "" {
			t.Fatalf("%s: %v", k, err)
		}
		var v map[string]any
		if json.Unmarshal(body, &v) != nil {
			t.Fatalf("%s: invalid JSON", k)
		}
	}
	_, body, _ := payload("telegram", Target{BotToken: "1:a", ChatID: "5"}, m)
	var tg map[string]any
	_ = json.Unmarshal(body, &tg)
	if txt := tg["text"].(string); !strings.Contains(txt, "T &lt;b&gt;") || !strings.Contains(txt, "x &amp; y") {
		t.Fatalf("telegram HTML not escaped: %s", txt)
	}
}

func TestWebhookSignatureAndSSRF(t *testing.T) {
	_, _, h, srv := setup(t)
	m := Message{Rule: "test", Severity: "info", Status: "test", Title: "t", Time: time.Now()}
	if _, err := Send(context.Background(), "webhook", Target{URL: srv.URL, Secret: "s3cret"}, m); err != nil {
		t.Fatal(err)
	}
	if sig := h.hdr[0].Get("X-Corvus-Signature"); !strings.HasPrefix(sig, "sha256=") || h.hdr[0].Get("X-Corvus-Timestamp") == "" {
		t.Fatalf("signature headers: %v", h.hdr[0])
	}
	allowLoopback = false
	client.CloseIdleConnections() // the check runs when a connection is dialed
	if _, err := Send(context.Background(), "slack", Target{URL: srv.URL}, m); err == nil {
		t.Fatal("loopback destination must be refused")
	}
	if _, err := Send(context.Background(), "slack", Target{URL: "http://169.254.169.254/latest/meta-data"}, m); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("metadata address must be refused: %v", err)
	}
	_ = fmt.Sprint
}

// A condition that drops out for one round and comes back is not resolved and fired again.
func TestErrorRateFlapping(t *testing.T) {
	e, tc, h, _ := setup(t)
	ctx := context.Background()
	now := time.Now()
	feed(tc, now, 80, 200)
	feed(tc, now, 20, 502)
	e.Evaluate(ctx, now)
	e.Evaluate(ctx, now.Add(20*time.Minute)) // gone for one round
	feed(tc, now.Add(20*time.Minute+30*time.Second), 80, 200)
	feed(tc, now.Add(20*time.Minute+30*time.Second), 20, 502)
	e.Evaluate(ctx, now.Add(20*time.Minute+30*time.Second)) // back
	for i := 1; i < h.n(); i++ {                            // an "ongoing" reminder after the cooldown is fine
		title := h.msgs[i]["attachments"].([]any)[0].(map[string]any)["title"].(string)
		if strings.Contains(title, "Resolved") || !strings.Contains(title, "Still firing") {
			t.Fatalf("flapping condition sent %q", title)
		}
	}
}
