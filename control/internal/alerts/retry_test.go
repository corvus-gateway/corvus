package alerts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"corvus/control/internal/seal"
	"corvus/control/internal/store"
)

// evalWait runs one evaluation round and waits for its deliveries.
func evalWait(e *Engine, ctx context.Context, now time.Time) {
	e.Evaluate(ctx, now)
	e.wait()
}

// sendR is sendRetry without a concurrency bound, as (code, error).
func sendR(ctx context.Context, kind string, t Target, m Message) (int, error) {
	d := sendRetry(ctx, kind, t, m, nil)
	return d.code, d.err
}

// fastRetries shortens the delivery retry delays for a test.
func fastRetries(t *testing.T) {
	t.Helper()
	d, m := retryDelays, maxRetryAfter
	retryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryDelays, maxRetryAfter = d, m })
}

// flaky answers with the given status codes in turn, then 200.
func flaky(t *testing.T, header http.Header, codes ...int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	allowLoopback.Store(true)
	t.Cleanup(func() { allowLoopback.Store(false) })
	n := new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(n.Add(1)) - 1
		if i < len(codes) {
			for k, v := range header {
				w.Header()[k] = v
			}
			w.WriteHeader(codes[i])
		}
	}))
	t.Cleanup(srv.Close)
	return srv, n
}

func TestRetryFailsTwiceThenSucceeds(t *testing.T) {
	fastRetries(t)
	srv, n := flaky(t, nil, 502, 503)
	code, err := sendR(context.Background(), "slack", Target{URL: srv.URL}, Message{Title: "t", Time: time.Now()})
	if err != nil || code != 200 || n.Load() != 3 {
		t.Fatalf("code=%d err=%v attempts=%d", code, err, n.Load())
	}
	// three failures: the last error is reported
	srv, n = flaky(t, nil, 500, 500, 500, 500)
	if code, err := sendR(context.Background(), "slack", Target{URL: srv.URL}, Message{Time: time.Now()}); err == nil || code != 500 || n.Load() != 3 {
		t.Fatalf("code=%d err=%v attempts=%d", code, err, n.Load())
	}
}

func TestRetryNotForClientErrors(t *testing.T) {
	fastRetries(t)
	for _, c := range []int{400, 401, 404} {
		srv, n := flaky(t, nil, c)
		if code, err := sendR(context.Background(), "discord", Target{URL: srv.URL}, Message{Time: time.Now()}); err == nil || code != c || n.Load() != 1 {
			t.Fatalf("%d: code=%d err=%v attempts=%d", c, code, err, n.Load())
		}
	}
	srv, n := flaky(t, nil, 408)
	if _, err := sendR(context.Background(), "discord", Target{URL: srv.URL}, Message{Time: time.Now()}); err != nil || n.Load() != 2 {
		t.Fatalf("408 must be retried: err=%v attempts=%d", err, n.Load())
	}
}

func TestRetryHonoursRetryAfter(t *testing.T) {
	fastRetries(t)
	retryDelays = []time.Duration{time.Hour, time.Hour} // only Retry-After may set the wait
	maxRetryAfter = 50 * time.Millisecond               // and it is capped
	srv, n := flaky(t, http.Header{"Retry-After": {"3600"}}, 429)
	start := time.Now()
	if _, err := sendR(context.Background(), "slack", Target{URL: srv.URL}, Message{Time: time.Now()}); err != nil || n.Load() != 2 {
		t.Fatalf("err=%v attempts=%d", err, n.Load())
	}
	if d := time.Since(start); d < 40*time.Millisecond || d > 10*time.Second {
		t.Fatalf("waited %v", d)
	}
	// a cancelled context ends the wait
	ctx, cancel := context.WithCancel(context.Background())
	srv, n = flaky(t, nil, 503, 503, 503)
	retryDelays = []time.Duration{time.Hour}
	time.AfterFunc(20*time.Millisecond, cancel)
	if _, err := sendR(ctx, "slack", Target{URL: srv.URL}, Message{Time: time.Now()}); err == nil || n.Load() != 1 {
		t.Fatalf("cancelled: err=%v attempts=%d", err, n.Load())
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for v, want := range map[string]time.Duration{
		"": 0, "7": 7 * time.Second, "-1": 0, "0": 0, "99999": time.Hour, "soon": 0,
		now.Add(30 * time.Second).Format(http.TimeFormat): 30 * time.Second,
		now.Add(-time.Minute).Format(http.TimeFormat):     0,
	} {
		if got := parseRetryAfter(v, now); got != want {
			t.Errorf("%q: %v, want %v", v, got, want)
		}
	}
}

// A firing alert whose notification failed is not marked as sent: it is tried again
// after a short pause instead of waiting for the whole cooldown.
func TestFailedNotificationRetriedSoon(t *testing.T) {
	fastRetries(t)
	e, tc, _, _ := setup(t)
	ctx := context.Background()
	codes := []int{500, 500, 500} // every attempt of the first round fails
	var hits atomic.Int32
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if i := int(hits.Add(1)) - 1; i < len(codes) {
			w.WriteHeader(codes[i])
		}
	}))
	t.Cleanup(down.Close)
	chans, _ := e.Store.ListAlertChannels(ctx)
	c := chans[0]
	tgt, _ := json.Marshal(Target{URL: down.URL})
	c.Target, _ = seal.Seal(e.Key, string(tgt))
	if err := e.Store.SaveAlertChannel(ctx, &c); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	feed(tc, now, 80, 200)
	feed(tc, now, 20, 502)
	evalWait(e, ctx, now)
	if hits.Load() != 3 {
		t.Fatalf("attempts = %d, want 3", hits.Load())
	}
	hist, _ := e.Store.ListAlerts(ctx, 10)
	if len(hist) != 1 || len(hist[0].Results) != 1 || hist[0].Results[0].OK || hist[0].Results[0].Error != "HTTP 500" {
		t.Fatalf("history: %+v", hist)
	}
	evalWait(e, ctx, now.Add(30*time.Second)) // within the short pause: no new attempt
	if hits.Load() != 3 {
		t.Fatalf("retried within the pause: %d", hits.Load())
	}
	evalWait(e, ctx, now.Add(failedRetry+time.Second)) // after the pause (from the end of the delivery), within the cooldown
	hist, _ = e.Store.ListAlerts(ctx, 10)
	if hits.Load() != 4 || len(hist) != 2 || !hist[0].Results[0].OK || hist[0].Status != "firing" {
		t.Fatalf("attempts=%d history=%+v", hits.Load(), hist)
	}
	if hist[0].Title != hist[1].Title {
		t.Fatalf("a never-delivered alert is sent as new, not as ongoing: %q", hist[0].Title)
	}
	evalWait(e, ctx, now.Add(failedRetry+time.Minute)) // delivered now: the cooldown applies
	if hits.Load() != 4 {
		t.Fatalf("cooldown after delivery ignored: %d", hits.Load())
	}
}

// Slow receivers do not hold up the evaluation round.
func TestEvaluateDoesNotWaitForDelivery(t *testing.T) {
	e, tc, _, _ := setup(t)
	ctx := context.Background()
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	t.Cleanup(slow.Close)
	tgt, _ := json.Marshal(Target{URL: slow.URL})
	sealed, _ := seal.Seal(e.Key, string(tgt))
	_ = e.Store.SaveAlertChannel(ctx, &store.AlertChannel{Name: "slow", Kind: "webhook", Target: sealed, Enabled: true, MinSeverity: "info"})
	now := time.Now()
	feed(tc, now, 80, 200)
	feed(tc, now, 20, 502)
	done := make(chan struct{})
	go func() { e.Evaluate(ctx, now); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Evaluate blocked on a slow receiver")
	}
	if a := e.Active(); len(a) != 1 {
		t.Fatalf("active: %+v", a)
	}
	e.Evaluate(ctx, now.Add(time.Second)) // delivery in flight: not sent twice
	close(release)
	e.wait()
	hist, _ := e.Store.ListAlerts(ctx, 10)
	if len(hist) != 1 || len(hist[0].Results) != 2 {
		t.Fatalf("history: %+v", hist)
	}
}
