package autoban

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"corvus/control/internal/store"
)

// captureLog redirects the default logger (all levels) for the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

type fakeGateway struct {
	mu     sync.Mutex
	status int
	reply  string
	bodies []map[string]any
	epoch  string
}

func (g *fakeGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if r.URL.Path == "/bad-behaviour" {
		_, _ = io.WriteString(w, `{"hits":[],"epoch":"`+g.epoch+`"}`)
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	g.bodies = append(g.bodies, body)
	if g.status != 0 {
		w.WriteHeader(g.status)
	}
	_, _ = io.WriteString(w, g.reply)
}

func (g *fakeGateway) pushes() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.bodies)
}

func (g *fakeGateway) last() map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.bodies[len(g.bodies)-1]
}

func TestSyncSkipsUnchangedAndSendsChanges(t *testing.T) {
	s, _, st := setup(t)
	ctx := context.Background()
	g := &fakeGateway{epoch: "e1"}
	gw := httptest.NewServer(g)
	defer gw.Close()
	s.PushURL, s.HitsURL = gw.URL+"/bans", gw.URL+"/bad-behaviour"
	now := time.Now()
	ban := func(ip string) {
		if err := st.UpsertBan(ctx, &store.IPBan{IP: ip, Reason: "t", Source: "manual", Offenses: 1}); err != nil {
			t.Fatal(err)
		}
	}
	ban("203.0.113.1")
	ban("203.0.113.2")

	// first push: full list, the gateway sweeps its store (no "removed")
	if ok, err := s.sync(ctx, now, false); !ok || err != nil {
		t.Fatalf("first sync: %v %v", ok, err)
	}
	if _, has := g.last()["removed"]; has {
		t.Fatalf("first push must be a full sweep: %v", g.last())
	}
	// unchanged (also at a later "now"): nothing is sent
	if ok, err := s.sync(ctx, now.Add(15*time.Second), false); ok || err != nil || g.pushes() != 1 {
		t.Fatalf("unchanged list pushed: %v %v %d", ok, err, g.pushes())
	}
	// a lifted ban: pushed with the removed entry, no sweep
	if b, err := st.BanByIP(ctx, "203.0.113.2"); err != nil {
		t.Fatal(err)
	} else if _, err := st.LiftBan(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.sync(ctx, now.Add(30*time.Second), false); !ok || err != nil {
		t.Fatalf("changed list not pushed: %v %v", ok, err)
	}
	rem, _ := g.last()["removed"].([]any)
	ips, _ := g.last()["ips"].(map[string]any)
	if len(rem) != 1 || rem[0] != GatewayKey("203.0.113.2") || len(ips) != 1 {
		t.Fatalf("delta push: %v", g.last())
	}
	// Trigger (manual change) forces a push even when unchanged
	s.Trigger()
	if ok, _ := s.sync(ctx, now.Add(40*time.Second), false); !ok {
		t.Fatal("Trigger did not force a push")
	}
	if rem, ok := g.last()["removed"].([]any); !ok || len(rem) != 0 {
		t.Fatalf("forced push without changes: %v", g.last())
	}
	if ok, _ := s.sync(ctx, now.Add(50*time.Second), false); ok {
		t.Fatal("force not reset")
	}
	// after FullSyncEvery the list is pushed again with a full sweep
	if ok, _ := s.sync(ctx, now.Add(FullSyncEvery), false); !ok {
		t.Fatal("no push after FullSyncEvery")
	}
	if _, has := g.last()["removed"]; has {
		t.Fatalf("periodic push must sweep: %v", g.last())
	}
	n := g.pushes()

	// a restarted gateway (new epoch) gets the full list in the next round
	if _, err := s.Pull(ctx); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.sync(ctx, now.Add(FullSyncEvery+15*time.Second), false); ok {
		t.Fatal("same epoch must not force a push")
	}
	restarts := 0
	s.OnGatewayRestart = func() { restarts++ }
	g.mu.Lock()
	g.epoch = "e2"
	g.mu.Unlock()
	if _, err := s.Pull(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pull(ctx); err != nil {
		t.Fatal(err)
	}
	if restarts != 1 {
		t.Fatalf("OnGatewayRestart called %d times, want once", restarts)
	}
	if ok, _ := s.sync(ctx, now.Add(FullSyncEvery+30*time.Second), false); !ok || g.pushes() != n+1 {
		t.Fatal("new gateway epoch did not force a push")
	}
	if _, has := g.last()["removed"]; has {
		t.Fatalf("push after a gateway restart must sweep: %v", g.last())
	}
	if st := s.SyncStatus(); st.LastSuccess == nil || st.Entries != 1 || st.ConsecutiveFailures != 0 || st.Bytes == 0 {
		t.Fatalf("status: %+v", st)
	}
}

func TestSyncFailuresWarnAndRecover(t *testing.T) {
	s, _, _ := setup(t)
	ctx := context.Background()
	logs := captureLog(t)
	g := &fakeGateway{status: 503, reply: "no ban store\n"}
	gw := httptest.NewServer(g)
	defer gw.Close()
	s.PushURL = gw.URL + "/bans"
	now := time.Now()
	for i := 0; i < FailureWarnAfter-1; i++ {
		if _, err := s.sync(ctx, now, false); err == nil || !strings.Contains(err.Error(), "503: no ban store") {
			t.Fatalf("error = %v", err)
		}
	}
	if strings.Contains(logs.String(), "level=WARN") {
		t.Fatalf("warned too early:\n%s", logs)
	}
	// a failed push is retried every round (also when the list did not change)
	if ok, _ := s.sync(ctx, now, false); !ok {
		t.Fatal("failed push not retried")
	}
	if strings.Count(logs.String(), "level=WARN") != 1 || !strings.Contains(logs.String(), "no ban store") {
		t.Fatalf("no warning after %d failures:\n%s", FailureWarnAfter, logs)
	}
	_, _ = s.sync(ctx, now.Add(time.Minute), false)
	if strings.Count(logs.String(), "level=WARN") != 1 {
		t.Fatalf("warning repeated within the hour:\n%s", logs)
	}
	if st := s.SyncStatus(); st.ConsecutiveFailures != FailureWarnAfter+1 || st.LastSuccess != nil || !strings.Contains(st.LastError, "no ban store") {
		t.Fatalf("status: %+v", st)
	}
	_, _ = s.sync(ctx, now.Add(failureRewarn+time.Minute), false)
	if strings.Count(logs.String(), "level=WARN") != 2 {
		t.Fatalf("warning not repeated after an hour:\n%s", logs)
	}

	g.mu.Lock()
	g.status, g.reply = 0, "0\n"
	g.mu.Unlock()
	if _, err := s.sync(ctx, now.Add(2*time.Hour), false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "works again") {
		t.Fatalf("recovery not logged:\n%s", logs)
	}
	if st := s.SyncStatus(); st.ConsecutiveFailures != 0 || st.LastError != "" || st.LastSuccess == nil {
		t.Fatalf("status after recovery: %+v", st)
	}
	// the push after a failure is a full sweep (the gateway may hold part of it)
	if _, has := g.last()["removed"]; has {
		t.Fatalf("push after a failure must sweep: %v", g.last())
	}
}

func TestSyncTooLarge(t *testing.T) {
	s := &Service{PushURL: "http://127.0.0.1:1/bans"}
	err := s.send(context.Background(), make([]byte, MaxPushBytes+1), 400000)
	if err == nil || !strings.Contains(err.Error(), "too large") || !strings.Contains(err.Error(), "400000 bans") {
		t.Fatalf("err = %v", err)
	}
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = io.WriteString(w, "<html>413 Request Entity Too Large</html>")
	}))
	defer gw.Close()
	s.PushURL = gw.URL
	if err := s.send(context.Background(), []byte(`{"ips":{}}`), 0); err == nil || !strings.Contains(err.Error(), "body limit") {
		t.Fatalf("413: %v", err)
	}
	gw2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "<html>bad</html>")
	}))
	defer gw2.Close()
	s.PushURL = gw2.URL
	if err := s.send(context.Background(), []byte(`{}`), 0); err == nil || err.Error() != "gateway answered 502" {
		t.Fatalf("html error page: %v", err)
	}
	var tr PushTracker
	tr.Record(time.Now(), 1, 1, errors.New("x"))
	if tr.Status().ConsecutiveFailures != 1 {
		t.Fatal("tracker")
	}
}
