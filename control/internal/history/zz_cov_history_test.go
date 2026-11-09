package history

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
)

func TestCovValidate(t *testing.T) {
	ok := DefaultSettings()
	if err := Validate(&ok); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	mut := func(f func(*Settings)) Settings { s := DefaultSettings(); f(&s); return s }
	bad := map[string]Settings{
		"log mode":   mut(func(s *Settings) { s.LogMode = "verbose" }),
		"log days 0": mut(func(s *Settings) { s.LogDays = 0 }),
		"log days":   mut(func(s *Settings) { s.LogDays = 366 }),
		"size small": mut(func(s *Settings) { s.LogMaxMB = 49 }),
		"size large": mut(func(s *Settings) { s.LogMaxMB = 1024*1024 + 1 }),
		"stats":      mut(func(s *Settings) { s.StatsDays = 401 }),
		"events":     mut(func(s *Settings) { s.EventsDays = 0 }),
	}
	for name, s := range bad {
		if err := Validate(&s); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	for _, m := range []string{"off", "notable", "all"} {
		s := mut(func(s *Settings) { s.LogMode = m })
		if err := Validate(&s); err != nil {
			t.Errorf("mode %s: %v", m, err)
		}
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestCovLoadCorruptSettings(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	_ = st.SetMeta(ctx, "retention", `{"log_mode":"all","log_days":3}`)
	if s := Load(ctx, st); s.LogMode != "all" || s.LogDays != 3 || s.StatsDays != 30 {
		t.Fatalf("partial settings keep defaults for the rest: %+v", s)
	}
	_ = st.SetMeta(ctx, "retention", `{bad`)
	if s := Load(ctx, st); s != DefaultSettings() {
		t.Fatalf("corrupt settings → defaults: %+v", s)
	}
}

func TestCovApplyCompressesOldLogs(t *testing.T) {
	st := openStore(t)
	dir := t.TempDir()
	s := newService(t, dir, st, traffic.New())
	defer s.db.Close()
	old := "access-" + time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02") + ".jsonl"
	if err := os.WriteFile(filepath.Join(s.logDir(), old), []byte(`{"h":"x"}`+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	set := DefaultSettings()
	set.LogMode = "off"
	s.Apply(context.Background(), set)
	if m, _ := s.mode.Load().(string); m != "off" {
		t.Fatalf("mode = %q", m)
	}
	gz := filepath.Join(s.logDir(), old+".gz")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(gz); err == nil {
			if _, err := os.Stat(filepath.Join(s.logDir(), old)); os.IsNotExist(err) {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("the previous day's log was not compressed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	f, _ := os.Open(gz)
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(zr)
	if string(b) != `{"h":"x"}`+"\n" {
		t.Fatalf("compressed content = %q", b)
	}
	if p := s.ArchivePath(old + ".gz"); p != gz {
		t.Fatalf("ArchivePath = %q", p)
	}
	for _, bad := range []string{"../x", "access-2020-01-01.jsonl", "notalog.txt"} {
		if p := s.ArchivePath(bad); p != "" {
			t.Errorf("ArchivePath(%q) = %q", bad, p)
		}
	}
	// compressing a file that is gone is a no-op
	compress(filepath.Join(s.logDir(), old))
	compress(filepath.Join(t.TempDir(), "missing.jsonl"))
}

func TestCovSinkModes(t *testing.T) {
	s := &Service{lines: make(chan []byte, 1)}
	r := &traffic.Record{H: "h", S: 200}
	s.mode.Store("off")
	s.Sink(r, true)
	if len(s.lines) != 0 {
		t.Fatal("off logs nothing")
	}
	s.mode.Store("notable")
	s.Sink(r, false)
	if len(s.lines) != 0 {
		t.Fatal("notable skips ordinary requests")
	}
	s.Sink(r, true)
	if len(s.lines) != 1 {
		t.Fatal("notable logs notable requests")
	}
	if line := <-s.lines; !strings.Contains(string(line), `"h":"h"`) {
		t.Fatalf("line = %s", line)
	}
	s.mode.Store("all")
	s.Sink(r, false)
	s.Sink(r, false) // channel full: dropped
	if len(s.lines) != 1 || s.dropped.Load() != 1 {
		t.Fatalf("queued=%d dropped=%d", len(s.lines), s.dropped.Load())
	}
	if u := (&Service{Dir: t.TempDir()}).Usage(); u.LogFiles != 0 || u.DBBytes != 0 {
		t.Fatalf("empty usage: %+v", u)
	}
}

func TestCovOpenErrors(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(f, nil, 0o600)
	s := &Service{Dir: f, Store: openStore(t), Collector: traffic.New()}
	if err := s.Open(context.Background()); err == nil {
		t.Fatal("open below a file must fail")
	}
	// the database path is a directory: creating the tables fails
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "traffic.db"), 0o700)
	s = &Service{Dir: dir, Store: openStore(t), Collector: traffic.New()}
	if err := s.Open(context.Background()); err == nil {
		t.Fatal("open with a directory as database must fail")
	}
}

func TestCovFlushAndSnapshotErrors(t *testing.T) {
	st := openStore(t)
	tc := traffic.New()
	tc.Add(&traffic.Record{T: float64(time.Now().Add(-time.Hour).Unix()), H: "h", S: 500, IP: "1.1.1.1"})
	dir := t.TempDir()
	s := newService(t, dir, st, tc)
	s.db.Close()
	s.flush(context.Background(), time.Now()) // closed database: logged, nothing recorded
	if s.coarseAt != 0 || s.evSeq != 0 {
		t.Fatalf("watermarks must not move on failure: %d %d", s.coarseAt, s.evSeq)
	}

	// snapshot into a directory that does not exist: no file, no panic
	s.Dir = filepath.Join(dir, "missing")
	s.saveSnapshot(time.Now())
	if _, err := os.Stat(s.snapshotPath()); err == nil {
		t.Fatal("snapshot must not exist")
	}
	// a corrupt snapshot is removed after the failed load
	s.Dir = dir
	_ = os.WriteFile(s.snapshotPath(), []byte("garbage"), 0o600)
	s.loadSnapshot(time.Now())
	if _, err := os.Stat(s.snapshotPath()); !os.IsNotExist(err) {
		t.Fatal("snapshot must be removed after loading")
	}
}

func TestCovWriteLogsRun(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	_ = st.SetMeta(ctx, "retention", `{"log_mode":"all","log_days":14,"log_max_mb":1024,"stats_days":30,"events_days":30}`)
	tc := traffic.New()
	dir := t.TempDir()
	s := newService(t, dir, st, tc)
	tc.Sink = s.Sink
	rctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { s.Run(rctx); close(done) }()
	for i := 0; i < 5; i++ {
		tc.Add(&traffic.Record{H: "run.example", U: "/", S: 200, IP: "1.1.1.1"})
	}
	// written by the log writer while running
	deadline := time.Now().Add(5 * time.Second)
	for {
		u := s.Usage()
		if u.LogFiles == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no log file")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	arch := s.Archives()
	if len(arch) != 1 {
		t.Fatalf("archives: %+v", arch)
	}
	b, _ := os.ReadFile(filepath.Join(s.logDir(), arch[0].Name))
	if n := strings.Count(string(b), "run.example"); n != 5 {
		t.Fatalf("logged lines = %d", n)
	}
	if _, err := os.Stat(s.snapshotPath()); err != nil {
		t.Fatalf("snapshot saved on stop: %v", err)
	}
}
