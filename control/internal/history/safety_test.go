package history

import (
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var errDiskFull = errors.New("no space left on device")

// failingFile wraps a real file; writes fail while full is set, Close/Sync fail on demand.
type failingFile struct {
	*os.File
	full      *atomic.Bool
	failClose bool
	failSync  bool
}

func (f *failingFile) Write(b []byte) (int, error) {
	if f.full != nil && f.full.Load() {
		return 0, errDiskFull
	}
	return f.File.Write(b)
}

func (f *failingFile) Close() error {
	err := f.File.Close()
	if f.failClose {
		return errDiskFull
	}
	return err
}

func (f *failingFile) Sync() error {
	if f.failSync {
		return errDiskFull
	}
	return f.File.Sync()
}

// A failed .gz (close or sync error, e.g. a full disk) never replaces the day's log.
func TestCompressKeepsOriginalOnFailure(t *testing.T) {
	defer func(old func(string) (logFile, error)) { createTmp = old }(createTmp)
	for name, mk := range map[string]func(*os.File) logFile{
		"close": func(f *os.File) logFile { return &failingFile{File: f, failClose: true} },
		"sync":  func(f *os.File) logFile { return &failingFile{File: f, failSync: true} },
		"write": func(f *os.File) logFile {
			full := &atomic.Bool{}
			full.Store(true)
			return &failingFile{File: f, full: full}
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "access-2026-01-01.jsonl")
			content := strings.Repeat(`{"h":"x"}`+"\n", 100)
			if err := os.WriteFile(p, []byte(content), 0o640); err != nil {
				t.Fatal(err)
			}
			createTmp = func(path string) (logFile, error) {
				f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
				if err != nil {
					return nil, err
				}
				return mk(f), nil
			}
			compress(p)
			if b, err := os.ReadFile(p); err != nil || string(b) != content {
				t.Fatalf("original must be kept intact: %v", err)
			}
			for _, n := range []string{p + ".gz", p + ".gz.tmp"} {
				if _, err := os.Stat(n); !os.IsNotExist(err) {
					t.Fatalf("%s must not exist: %v", filepath.Base(n), err)
				}
			}
		})
	}
	// working again: compressed and the original removed
	createTmp = func(path string) (logFile, error) {
		return os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "access-2026-01-02.jsonl")
	_ = os.WriteFile(p, []byte("a\n"), 0o640)
	compress(p)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("original must be removed after a successful compression")
	}
	f, err := os.Open(p + ".gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(zr); string(b) != "a\n" {
		t.Fatalf("content %q", b)
	}
}

// A write error does not lose the rest of the day: the lines that fail count as
// dropped, and once the disk recovers the log is reopened and continues on a new line.
func TestDayLogRecoversAfterWriteError(t *testing.T) {
	defer func(old func(string) (logFile, error)) { openLogFile = old }(openLogFile)
	full := &atomic.Bool{}
	opens := 0
	openLogFile = func(path string) (logFile, error) {
		opens++
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			return nil, err
		}
		return &failingFile{File: f, full: full}, nil
	}
	dir := t.TempDir()
	var dropped atomic.Int64
	var openLog atomic.Value
	d := &dayLog{dir: dir, dropped: &dropped, openLog: &openLog}
	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	d.write(now, []byte(`{"n":1}`))
	d.write(now, []byte(`{"n":2}`))
	d.flush(now)
	if dropped.Load() != 0 {
		t.Fatalf("dropped = %d", dropped.Load())
	}
	// leave half a line in the file, as a partial write on a full disk would
	p := filepath.Join(dir, "access-2026-03-01.jsonl")
	af, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	af.WriteString(`{"n":`)
	af.Close()

	full.Store(true)
	d.write(now, []byte(`{"n":3}`))
	d.write(now, []byte(`{"n":4}`))
	d.flush(now) // fails: both buffered lines are lost
	if got := dropped.Load(); got != 2 {
		t.Fatalf("dropped after failed flush = %d", got)
	}
	if d.f != nil {
		t.Fatal("the failed writer must be discarded")
	}
	// before the retry delay: no reopen, counted as dropped
	d.write(now.Add(time.Second), []byte(`{"n":5}`))
	if got := dropped.Load(); got != 3 || opens != 1 {
		t.Fatalf("dropped=%d opens=%d", got, opens)
	}
	full.Store(false)
	later := now.Add(logRetry + time.Second)
	d.write(later, []byte(`{"n":6}`))
	d.close(later)
	if opens != 2 {
		t.Fatalf("opens = %d", opens)
	}
	b, _ := os.ReadFile(p)
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	want := []string{`{"n":1}`, `{"n":2}`, `{"n":`, `{"n":6}`}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("file lines %q", lines)
	}
	if dropped.Load() != 3 {
		t.Fatalf("dropped = %d", dropped.Load())
	}
}

// A directory that cannot be written: every line counts as dropped, no panic.
func TestDayLogUnwritableDir(t *testing.T) {
	var dropped atomic.Int64
	d := &dayLog{dir: filepath.Join(t.TempDir(), "missing", "logs"), dropped: &dropped}
	now := time.Now()
	for i := 0; i < 3; i++ {
		d.write(now, []byte("x"))
	}
	d.flush(now)
	d.close(now)
	if dropped.Load() != 3 {
		t.Fatalf("dropped = %d", dropped.Load())
	}
}

// Midnight rotation hands the finished file over for compression.
func TestDayLogRotates(t *testing.T) {
	var dropped atomic.Int64
	var rotated []string
	dir := t.TempDir()
	d := &dayLog{dir: dir, dropped: &dropped, rotated: func(p string) { rotated = append(rotated, filepath.Base(p)) }}
	day1 := time.Date(2026, 3, 1, 23, 59, 0, 0, time.UTC)
	d.write(day1, []byte("a"))
	d.write(day1.Add(2*time.Minute), []byte("b"))
	d.close(day1)
	if len(rotated) != 1 || rotated[0] != "access-2026-03-01.jsonl" {
		t.Fatalf("rotated %v", rotated)
	}
	for name, want := range map[string]string{"access-2026-03-01.jsonl": "a\n", "access-2026-03-02.jsonl": "b\n"} {
		if b, _ := os.ReadFile(filepath.Join(dir, name)); string(b) != want {
			t.Fatalf("%s = %q", name, b)
		}
	}
}
