package alerts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"corvus/control/internal/seal"
	"corvus/control/internal/store"
)

// fakeClock replaces the delivery clock: a wait advances the time at once.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }

func (c *fakeClock) advance(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func useFakeClock(t *testing.T, start time.Time) *fakeClock {
	t.Helper()
	c := &fakeClock{t: start}
	n, s := clockNow, sleep
	clockNow = c.now
	sleep = func(_ context.Context, d time.Duration) bool { c.advance(d); return true }
	t.Cleanup(func() { clockNow, sleep = n, s })
	return c
}

// setChannel points the setup's channel at url.
func setChannel(t *testing.T, e *Engine, url string) {
	t.Helper()
	ctx := context.Background()
	chans, _ := e.Store.ListAlertChannels(ctx)
	c := chans[0]
	tgt, _ := json.Marshal(Target{URL: url})
	c.Target, _ = seal.Seal(e.Key, string(tgt))
	if err := e.Store.SaveAlertChannel(ctx, &c); err != nil {
		t.Fatal(err)
	}
}

// A receiver answering 429 with Retry-After for 20 minutes: each round's retries wait
// as asked, the next round starts no earlier than Retry-After after the end of the
// last delivery, and the history has one failure and one success, not a row per round.
func TestRetryAfterBoundsAttempts(t *testing.T) {
	e, tc, _, _ := setup(t)
	ctx := context.Background()
	start := time.Now().Truncate(time.Second)
	clk := useFakeClock(t, start)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if clk.now().Before(start.Add(20 * time.Minute)) {
			w.Header().Set("Retry-After", "300")
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	t.Cleanup(srv.Close)
	setChannel(t, e, srv.URL)

	// evaluation every 30 s for 30 simulated minutes
	for clk.now().Before(start.Add(30 * time.Minute)) {
		now := clk.now()
		feed(tc, now, 80, 200)
		feed(tc, now, 20, 502)
		evalWait(e, ctx, now)
		clk.advance(30 * time.Second)
	}
	// rounds at 0, 9 and 18 min (3 + 3 + 2 attempts, waits of 2 min within a round,
	// then 5 min from its end); a 2 min pause from each round's start would be more
	if n := hits.Load(); n < 3 || n > 8 {
		t.Fatalf("attempts = %d, want at most 8", n)
	}
	hist, _ := e.Store.ListAlerts(ctx, 20)
	if len(hist) != 2 || !hist[0].Results[0].OK || hist[1].Results[0].OK || hist[1].Results[0].Error != "HTTP 429" {
		t.Fatalf("history (newest first): %+v", hist)
	}
}

// A failing receiver: the history records the episode's first failed round only.
func TestFailedRoundsRecordedOnce(t *testing.T) {
	e, tc, _, _ := setup(t)
	ctx := context.Background()
	start := time.Now().Truncate(time.Second)
	clk := useFakeClock(t, start)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	setChannel(t, e, srv.URL)
	for i := 0; i < 20; i++ { // 10 simulated minutes
		now := clk.now()
		feed(tc, now, 80, 200)
		feed(tc, now, 20, 502)
		evalWait(e, ctx, now)
		clk.advance(30 * time.Second)
	}
	if hits.Load() < 6 {
		t.Fatalf("attempts = %d: the alert must be tried in several rounds", hits.Load())
	}
	if hist, _ := e.Store.ListAlerts(ctx, 20); len(hist) != 1 {
		t.Fatalf("history rows = %d, want 1", len(hist))
	}
}

// A channel waiting to retry does not hold a delivery slot: with one slot, another
// notification is delivered while the first channel backs off.
func TestBackoffReleasesSlot(t *testing.T) {
	e, _, h, _ := setup(t)
	ctx := context.Background()
	e.init()
	e.sem = make(chan struct{}, 1)
	limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(limited.Close)
	tgt, _ := json.Marshal(Target{URL: limited.URL})
	sealed, _ := seal.Seal(e.Key, string(tgt))
	if err := e.Store.SaveAlertChannel(ctx, &store.AlertChannel{Name: "limited", Kind: "webhook", Target: sealed, Enabled: true,
		MinSeverity: "info", Events: []string{"busy"}}); err != nil {
		t.Fatal(err)
	}
	chans, _ := e.Store.ListAlertChannels(ctx)
	for _, c := range chans {
		if c.Name == "slack" {
			c.Events = []string{"other"}
			if err := e.Store.SaveAlertChannel(ctx, &c); err != nil {
				t.Fatal(err)
			}
		}
	}

	backingOff, release := make(chan struct{}), make(chan struct{})
	s := sleep
	sleep = func(context.Context, time.Duration) bool { // the only wait: it ends the retries
		close(backingOff)
		<-release
		return false
	}
	t.Cleanup(func() { sleep = s })

	done := make(chan struct{})
	go func() {
		defer close(done)
		e.dispatch(ctx, Message{Rule: "busy", Severity: "critical", Status: "firing", Title: "busy", Time: time.Now()}, true)
	}()
	<-backingOff
	ok := make(chan bool, 1)
	go func() {
		d, _ := e.dispatch(ctx, Message{Rule: "other", Severity: "critical", Status: "firing", Title: "other", Time: time.Now()}, true)
		ok <- d
	}()
	select {
	case d := <-ok:
		if !d || h.n() != 1 {
			t.Fatalf("delivered=%v hook=%d", d, h.n())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("delivery blocked by a channel waiting to retry")
	}
	close(release)
	<-done
}
