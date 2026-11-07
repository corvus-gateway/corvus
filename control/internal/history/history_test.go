package history

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
)

func newService(t *testing.T, dir string, st *store.Store, tc *traffic.Collector) *Service {
	t.Helper()
	s := &Service{Dir: dir, Store: st, Collector: tc}
	if err := s.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPersistAndRestore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now()
	tc := traffic.New()
	base := float64(now.Add(-2 * time.Hour).Unix())
	for i := 0; i < 300; i++ {
		r := &traffic.Record{T: base + float64(i*10), H: "shop.example.com", U: "/p", S: 200, RT: 0.02, IP: "198.51.100.1"}
		if i%10 == 0 {
			r.S, r.X = 403, "sqli"
		}
		if i%25 == 0 {
			r.R = "7:block"
		}
		tc.Add(r)
	}
	before := tc.SummaryFor(24*time.Hour, "", now)
	hitsBefore := tc.RuleHits(now)[7].Hits24h

	s := newService(t, filepath.Join(dir, "history"), st, tc)
	s.flush(ctx, now)
	s.db.Close()

	// a fresh collector (restart) gets the same 24h picture back
	tc2 := traffic.New()
	s2 := newService(t, filepath.Join(dir, "history"), st, tc2)
	defer s2.db.Close()
	s2.Restore(ctx, now)
	after := tc2.SummaryFor(24*time.Hour, "", now)
	if after.Requests != before.Requests || after.S4xx != before.S4xx || after.P95 != before.P95 {
		t.Fatalf("summary mismatch: before %+v after %+v", before, after)
	}
	if n := len(tc2.Events("", 500)); n == 0 {
		t.Fatal("events not restored")
	}
	if got := tc2.RuleHits(now)[7].Hits24h; got != hitsBefore {
		t.Fatalf("rule hits %d, want %d", got, hitsBefore)
	}
	// flushing again does not duplicate
	s2.flush(ctx, now)
	var n int
	_ = s2.db.QueryRow(`SELECT COUNT(*) FROM rule_events`).Scan(&n)
	if int64(n) != hitsBefore {
		t.Fatalf("rule events duplicated: %d", n)
	}
}

func TestLogRetention(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	set := Settings{LogMode: "notable", LogDays: 3, LogMaxMB: 50, StatsDays: 30, EventsDays: 30}
	raw, _ := json.Marshal(set)
	_ = st.SetMeta(ctx, "retention", string(raw))
	s := newService(t, filepath.Join(dir, "history"), st, traffic.New())
	defer s.db.Close()

	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	write := func(day string, gz bool, size int) {
		name := "access-" + day + ".jsonl"
		if gz {
			name += ".gz"
		}
		_ = os.WriteFile(filepath.Join(s.logDir(), name), []byte(strings.Repeat("x", size)), 0o640)
	}
	write("2026-11-01", true, 10)      // older than 3 days → removed
	write("2026-11-08", true, 30<<20)  // within days but pushes over the 50 MB cap → removed (oldest)
	write("2026-11-09", false, 30<<20) // uncompressed earlier day → compressed (tiny after gzip)
	write("2026-11-10", false, 1<<10)  // today → kept
	_ = os.WriteFile(filepath.Join(s.logDir(), "notes.txt"), []byte("x"), 0o640)
	s.cleanup(ctx, now)

	names := map[string]bool{}
	for _, a := range s.Archives() {
		names[a.Name] = true
	}
	if names["access-2026-11-01.jsonl.gz"] || names["access-2026-11-09.jsonl"] || !names["access-2026-11-09.jsonl.gz"] || !names["access-2026-11-10.jsonl"] {
		t.Fatalf("after cleanup: %v", names)
	}
	if u := s.Usage(); u.LogBytes > 50<<20 {
		t.Fatalf("size cap not enforced: %d", u.LogBytes)
	}
	if s.ArchivePath("../corvus.db") != "" || s.ArchivePath("notes.txt") != "" || s.ArchivePath("access-2026-11-10.jsonl") == "" {
		t.Fatal("archive path validation")
	}

	// sink honours the mode
	s.mode.Store("notable")
	s.Sink(&traffic.Record{S: 200}, false)
	s.Sink(&traffic.Record{S: 403}, true)
	if len(s.lines) != 1 {
		t.Fatalf("queued %d lines", len(s.lines))
	}
}

// Stopping Run saves the unfinished bucket and the queued log lines; after a restart
// the bucket keeps counting and is saved again with the combined total.
func TestFinalSaveOnStop(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now()
	tc := traffic.New()
	s := newService(t, filepath.Join(dir, "history"), st, tc)
	s.mode.Store("all")
	tc.Sink = s.Sink
	for i := 0; i < 5; i++ {
		tc.Add(&traffic.Record{T: float64(now.Unix()), H: "shop.example.com", U: "/p", S: 200, RT: 0.01, IP: "198.51.100.1"})
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond)
	stop()
	<-done

	logs, _ := filepath.Glob(filepath.Join(dir, "history", "logs", "access-*.jsonl"))
	if len(logs) != 1 {
		t.Fatalf("log files: %v", logs)
	}
	if b, _ := os.ReadFile(logs[0]); strings.Count(string(b), "\n") != 5 {
		t.Fatalf("log lines: %q", b)
	}

	// restart: the unfinished bucket comes back and is saved again with new requests
	tc2 := traffic.New()
	s2 := newService(t, filepath.Join(dir, "history"), st, tc2)
	defer s2.db.Close()
	s2.Restore(context.Background(), now)
	if got := tc2.SummaryFor(24*time.Hour, "", now).Requests; got != 5 {
		t.Fatalf("restored requests = %d, want 5", got)
	}
	for i := 0; i < 3; i++ {
		tc2.Add(&traffic.Record{T: float64(now.Unix()), H: "shop.example.com", U: "/p", S: 200, RT: 0.01, IP: "198.51.100.1"})
	}
	s2.flush(context.Background(), now.Add(traffic.CoarseStep*time.Second))
	var n int64
	if err := s2.db.QueryRow(`SELECT count FROM stats WHERE host = '' ORDER BY ts DESC LIMIT 1`).Scan(&n); err != nil || n != 8 {
		t.Fatalf("saved count = %d (%v), want 8", n, err)
	}
}
