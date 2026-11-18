// Package history persists traffic statistics, security events and request logs with
// bounded retention, so dashboards survive restarts and disk use stays predictable.
//
// Layout (under <data>/history):
//
//	traffic.db                    SQLite: 5-minute stats per host, security events, rule/body events
//	logs/access-YYYY-MM-DD.jsonl  request log of the current day (UTC)
//	logs/access-YYYY-MM-DD.jsonl.gz  earlier days, compressed
//
// Retention (meta "retention"): log mode off | notable | all, log days, total log size
// cap, stats days, events days. Cleanup runs hourly; the oldest log files are removed
// first when the size cap is exceeded.
package history

import (
	"bufio"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"

	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
)

// Settings are stored in meta "retention".
type Settings struct {
	LogMode    string `json:"log_mode"` // off | notable | all
	LogDays    int    `json:"log_days"`
	LogMaxMB   int    `json:"log_max_mb"`
	StatsDays  int    `json:"stats_days"`
	EventsDays int    `json:"events_days"`
}

func DefaultSettings() Settings {
	return Settings{LogMode: "notable", LogDays: 14, LogMaxMB: 1024, StatsDays: 30, EventsDays: 30}
}

func Load(ctx context.Context, s *store.Store) Settings {
	set := DefaultSettings()
	if v, _ := s.GetMeta(ctx, "retention"); v != "" {
		_ = json.Unmarshal([]byte(v), &set)
	}
	return set
}

func Validate(s *Settings) error {
	switch s.LogMode {
	case "off", "notable", "all":
	default:
		return fmt.Errorf("log mode must be off, notable or all")
	}
	if s.LogDays < 1 || s.LogDays > 365 {
		return fmt.Errorf("log retention must be 1-365 days")
	}
	if s.LogMaxMB < 50 || s.LogMaxMB > 1024*1024 {
		return fmt.Errorf("log size limit must be 50 MB - 1 TB")
	}
	if s.StatsDays < 1 || s.StatsDays > 400 || s.EventsDays < 1 || s.EventsDays > 400 {
		return fmt.Errorf("statistics and event retention must be 1-400 days")
	}
	return nil
}

type Service struct {
	Dir       string
	Store     *store.Store
	Collector *traffic.Collector

	db      *sql.DB
	lines   chan []byte
	dropped atomic.Int64
	mode    atomic.Value // string: current log mode (read on the hot path)
	openLog atomic.Value // string: path of the day file writeLogs is appending to

	mu       sync.Mutex
	coarseAt int64 // last persisted bucket start
	evSeq    int64
	ruleSeq  int64
}

// Usage summarises storage for the UI.
type Usage struct {
	DBBytes   int64     `json:"db_bytes"`
	LogBytes  int64     `json:"log_bytes"`
	LogFiles  int       `json:"log_files"`
	Oldest    string    `json:"oldest_log,omitempty"`
	Dropped   int64     `json:"dropped_lines"`
	CheckedAt time.Time `json:"checked_at"`
}

// Archive is one downloadable log file.
type Archive struct {
	Name  string    `json:"name"`
	Bytes int64     `json:"bytes"`
	Mod   time.Time `json:"modified"`
}

func (s *Service) logDir() string { return filepath.Join(s.Dir, "logs") }

// Open creates the database and starts accepting log lines. Call before Run.
func (s *Service) Open(ctx context.Context) error {
	if err := os.MkdirAll(s.logDir(), 0o750); err != nil {
		return err
	}
	dsn := "file:" + filepath.Join(s.Dir, "traffic.db") + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=auto_vacuum(INCREMENTAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS stats (host TEXT NOT NULL, ts INTEGER NOT NULL, count INTEGER, s2 INTEGER, s3 INTEGER, s4 INTEGER, s5 INTEGER,
			blocked INTEGER, queued INTEGER, bytes INTEGER, hist BLOB, PRIMARY KEY (host, ts))`,
		`CREATE INDEX IF NOT EXISTS stats_ts ON stats(ts)`,
		`CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY AUTOINCREMENT, ts REAL NOT NULL, kind TEXT, host TEXT, path TEXT, ip TEXT, rule TEXT, count INTEGER)`,
		`CREATE INDEX IF NOT EXISTS events_ts ON events(ts)`,
		`CREATE TABLE IF NOT EXISTS rule_events (id INTEGER PRIMARY KEY AUTOINCREMENT, ts REAL NOT NULL, rule_id INTEGER, rule TEXT, action TEXT, host TEXT,
			ip TEXT, country TEXT, method TEXT, path TEXT, ua TEXT, status INTEGER)`,
		`CREATE INDEX IF NOT EXISTS rule_events_ts ON rule_events(ts)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			db.Close()
			return err
		}
	}
	s.db = db
	s.lines = make(chan []byte, 20000)
	s.mode.Store(Load(ctx, s.Store).LogMode)
	return nil
}

// Restore loads the last 24 hours into the collector (call once, before Run).
func (s *Service) Restore(ctx context.Context, now time.Time) {
	since := now.Add(-24 * time.Hour)
	var rows []traffic.CoarseRow
	if rs, err := s.db.QueryContext(ctx, `SELECT host, ts, count, s2, s3, s4, s5, blocked, queued, bytes, hist FROM stats WHERE ts > ?`, since.Unix()); err == nil {
		for rs.Next() {
			var r traffic.CoarseRow
			var hist []byte
			if rs.Scan(&r.Host, &r.Start, &r.Count, &r.S2, &r.S3, &r.S4, &r.S5, &r.Blocked, &r.Queued, &r.Bytes, &hist) == nil {
				decodeHist(hist, &r)
				rows = append(rows, r)
			}
		}
		rs.Close()
	}
	s.Collector.RestoreCoarse(rows, now)
	var evs []traffic.Event
	if rs, err := s.db.QueryContext(ctx, `SELECT ts, kind, host, path, ip, rule, count FROM events WHERE ts > ? ORDER BY ts DESC LIMIT 500`, float64(since.Unix())); err == nil {
		for rs.Next() {
			var e traffic.Event
			if rs.Scan(&e.TS, &e.Kind, &e.Host, &e.Path, &e.IP, &e.Rule, &e.Count) == nil {
				evs = append(evs, e)
			}
		}
		rs.Close()
	}
	reverse(evs)
	var rules []traffic.RuleEvent
	if rs, err := s.db.QueryContext(ctx, `SELECT ts, rule_id, rule, action, host, ip, country, method, path, ua, status FROM rule_events WHERE ts > ? ORDER BY ts DESC LIMIT 1000`, float64(since.Unix())); err == nil {
		for rs.Next() {
			var e traffic.RuleEvent
			if rs.Scan(&e.TS, &e.RuleID, &e.Rule, &e.Action, &e.Host, &e.IP, &e.Country, &e.Method, &e.Path, &e.UA, &e.Status) == nil {
				rules = append(rules, e)
			}
		}
		rs.Close()
	}
	reverse(rules)
	s.mu.Lock()
	s.evSeq = s.Collector.RestoreEvents(evs)
	s.ruleSeq = s.Collector.RestoreRuleEvents(rules)
	if len(rows) > 0 {
		for _, r := range rows {
			// a bucket saved unfinished at shutdown is flushed again once it is complete
			if r.Start > s.coarseAt && r.Start+traffic.CoarseStep <= now.Unix() {
				s.coarseAt = r.Start
			}
		}
	}
	s.mu.Unlock()
	slog.Info("traffic history restored", "buckets", len(rows), "events", len(evs), "rule_events", len(rules))
	s.loadSnapshot(now)
}

func (s *Service) snapshotPath() string { return filepath.Join(s.Dir, "live.snapshot") }

// loadSnapshot restores the live state saved at the last clean shutdown (10-second
// buckets, per-minute stats, recent request log). The file is removed afterwards so an
// unclean stop later never loads it twice.
func (s *Service) loadSnapshot(now time.Time) {
	p := s.snapshotPath()
	f, err := os.Open(p)
	if err != nil {
		return
	}
	err = s.Collector.ReadSnapshot(f, now)
	f.Close()
	os.Remove(p)
	switch {
	case err == nil:
		slog.Info("live traffic restored")
	case errors.Is(err, traffic.ErrSnapshotStale):
		// stopped for more than an hour: nothing in it is shown any more
	default:
		slog.Warn("live traffic snapshot", "err", err)
	}
}

// saveSnapshot writes the live state for the next start (see loadSnapshot).
func (s *Service) saveSnapshot(now time.Time) {
	p := s.snapshotPath()
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		slog.Warn("live traffic snapshot", "err", err)
		return
	}
	err = s.Collector.WriteSnapshot(f, now)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, p)
	}
	if err != nil {
		os.Remove(tmp)
		slog.Warn("live traffic snapshot", "err", err)
	}
}

func reverse[T any](a []T) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

func encodeHist(r traffic.CoarseRow) []byte {
	b := make([]byte, 4*len(r.Hist))
	for i, v := range r.Hist {
		binary.LittleEndian.PutUint32(b[i*4:], v)
	}
	return b
}

func decodeHist(b []byte, r *traffic.CoarseRow) {
	for i := range r.Hist {
		if (i+1)*4 <= len(b) {
			r.Hist[i] = binary.LittleEndian.Uint32(b[i*4:])
		}
	}
}

// Sink is installed as traffic.Collector.Sink: queues log lines without blocking.
func (s *Service) Sink(r *traffic.Record, notable bool) {
	mode, _ := s.mode.Load().(string)
	if mode == "off" || (mode != "all" && !notable) {
		return
	}
	line, err := json.Marshal(r)
	if err != nil {
		return
	}
	select {
	case s.lines <- line:
	default:
		s.dropped.Add(1) // disk slower than traffic: drop rather than slow requests
	}
}

// Run flushes statistics/events every minute, writes logs and enforces retention.
//
// When ctx ends it writes the request lines still queued, saves the statistics of the
// current (unfinished) bucket and the live snapshot, closes the database and returns;
// callers wait for it so a restart or upgrade does not lose the last minutes.
func (s *Service) Run(ctx context.Context) {
	logsDone := make(chan struct{})
	go func() { s.writeLogs(ctx); close(logsDone) }()
	flush := time.NewTicker(time.Minute)
	defer flush.Stop()
	clean := time.NewTicker(time.Hour)
	defer clean.Stop()
	s.cleanup(ctx, time.Now())
	for {
		select {
		case <-ctx.Done():
			<-logsDone
			// including the current bucket: Restore loads it back and keeps adding to it
			s.flush(context.Background(), time.Now().Add(traffic.CoarseStep*time.Second))
			s.db.Close()
			s.saveSnapshot(time.Now())
			return
		case now := <-flush.C:
			s.mode.Store(Load(ctx, s.Store).LogMode)
			s.flush(ctx, now)
		case now := <-clean.C:
			s.cleanup(ctx, now)
		}
	}
}

func (s *Service) flush(ctx context.Context, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Warn("history flush", "err", err)
		return
	}
	rows := s.Collector.CompletedCoarse(s.coarseAt, now)
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO stats(host, ts, count, s2, s3, s4, s5, blocked, queued, bytes, hist) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			r.Host, r.Start, r.Count, r.S2, r.S3, r.S4, r.S5, r.Blocked, r.Queued, r.Bytes, encodeHist(r)); err != nil {
			tx.Rollback()
			slog.Warn("history flush stats", "err", err)
			return
		}
	}
	evs, evSeq := s.Collector.EventsAfter(s.evSeq, now)
	for _, e := range evs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO events(ts, kind, host, path, ip, rule, count) VALUES (?,?,?,?,?,?,?)`, e.TS, e.Kind, e.Host, e.Path, e.IP, e.Rule, e.Count); err != nil {
			tx.Rollback()
			return
		}
	}
	res, ruleSeq := s.Collector.RuleEventsAfter(s.ruleSeq)
	sort.Slice(res, func(i, j int) bool { return res[i].TS < res[j].TS })
	for _, e := range res {
		if _, err := tx.ExecContext(ctx, `INSERT INTO rule_events(ts, rule_id, rule, action, host, ip, country, method, path, ua, status) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			e.TS, e.RuleID, e.Rule, e.Action, e.Host, e.IP, e.Country, e.Method, e.Path, e.UA, e.Status); err != nil {
			tx.Rollback()
			return
		}
	}
	if err := tx.Commit(); err != nil {
		slog.Warn("history commit", "err", err)
		return
	}
	for _, r := range rows {
		if r.Start > s.coarseAt {
			s.coarseAt = r.Start
		}
	}
	s.evSeq, s.ruleSeq = evSeq, ruleSeq
}

// ------------------------------------------------------------------ request logs

var logName = regexp.MustCompile(`^access-(\d{4}-\d{2}-\d{2})\.jsonl(\.gz)?$`)

// logFile is what the request log writes to (an *os.File; replaced in tests).
type logFile interface {
	io.Writer
	Sync() error
	Close() error
	Name() string
}

// openLogFile opens (appends to) a day file. A variable so tests can make it fail.
var openLogFile = func(path string) (logFile, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
}

// dayLog appends request lines to the file of the current UTC day.
//
// A write error (full disk, I/O error) leaves a bufio.Writer failed for good, so after
// one the writer and file are discarded and reopened (at most every logRetry) instead
// of silently losing the rest of the day. The lines lost with the failed buffer and
// every line that cannot be written count as dropped; warnings are rate-limited.
type dayLog struct {
	dir     string
	dropped *atomic.Int64
	openLog *atomic.Value
	rotated func(prev string) // called with the finished day file (to compress it)

	f        logFile
	w        *bufio.Writer
	day      string
	pending  int64     // lines buffered since the last successful flush
	retryAt  time.Time // after a failure: do not reopen before this
	broken   bool      // the last file ended in a failed write (may end mid-line)
	lastWarn time.Time
}

const logRetry = 5 * time.Second

func (d *dayLog) warn(now time.Time, msg string, err error) {
	if !d.lastWarn.IsZero() && now.Sub(d.lastWarn) < time.Minute {
		return
	}
	d.lastWarn = now
	slog.Warn(msg, "err", err, "dropped_lines", d.dropped.Load())
}

// fail discards the failed writer: the buffered lines are lost and count as dropped
// (an upper bound: bufio may already have written some of them).
func (d *dayLog) fail(now time.Time, err error) {
	d.dropped.Add(d.pending)
	d.pending = 0
	if d.f != nil {
		d.f.Close()
	}
	d.f, d.w = nil, nil
	d.broken = true
	d.retryAt = now.Add(logRetry)
	d.warn(now, "request log write failed; lines are dropped until it recovers", err)
}

// open makes sure the file of now's day is open. False when it cannot be.
func (d *dayLog) open(now time.Time) bool {
	day := now.UTC().Format("2006-01-02")
	if d.f != nil && day == d.day {
		return true
	}
	if d.f != nil { // midnight: finish the previous day
		prev := d.f.Name()
		d.close(now)
		if d.rotated != nil {
			d.rotated(prev)
		}
	}
	if now.Before(d.retryAt) {
		return false
	}
	path := filepath.Join(d.dir, "access-"+day+".jsonl")
	f, err := openLogFile(path)
	if err != nil {
		d.retryAt = now.Add(logRetry)
		d.warn(now, "request log", err)
		return false
	}
	d.f, d.w, d.day, d.pending = f, bufio.NewWriterSize(f, 256<<10), day, 0
	if d.broken {
		// the failed write may have left half a line: start on a fresh line
		if st, err := os.Stat(path); err == nil && st.Size() > 0 && !endsWithNewline(path, st.Size()) {
			d.w.WriteByte('\n')
		}
		d.broken = false
	}
	if d.openLog != nil {
		d.openLog.Store(f.Name())
	}
	return true
}

func endsWithNewline(path string, size int64) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	b := make([]byte, 1)
	if _, err := f.ReadAt(b, size-1); err != nil {
		return true
	}
	return b[0] == '\n'
}

// write appends one line.
func (d *dayLog) write(now time.Time, line []byte) {
	if !d.open(now) {
		d.dropped.Add(1)
		return
	}
	_, err := d.w.Write(line)
	if err == nil {
		err = d.w.WriteByte('\n')
	}
	if err != nil {
		d.dropped.Add(1)
		d.fail(now, err)
		return
	}
	d.pending++
}

// flush writes the buffered lines to the file.
func (d *dayLog) flush(now time.Time) {
	if d.f == nil {
		return
	}
	if err := d.w.Flush(); err != nil {
		d.fail(now, err)
		return
	}
	d.pending = 0
}

// close flushes and closes the current file.
func (d *dayLog) close(now time.Time) {
	d.flush(now)
	if d.f == nil {
		return
	}
	if err := d.f.Close(); err != nil {
		d.warn(now, "request log close", err)
	}
	d.f, d.w = nil, nil
}

func (s *Service) writeLogs(ctx context.Context) {
	d := &dayLog{dir: s.logDir(), dropped: &s.dropped, openLog: &s.openLog, rotated: func(prev string) { go compress(prev) }}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			for drained := false; !drained; {
				select {
				case line := <-s.lines:
					d.write(time.Now(), line)
				default:
					drained = true
				}
			}
			d.close(time.Now())
			return
		case line := <-s.lines:
			d.write(time.Now(), line)
		case now := <-tick.C:
			d.flush(now)
			if d.f != nil && now.UTC().Format("2006-01-02") != d.day {
				d.open(now) // rotate at midnight even when idle
			}
		}
	}
}

// compressMu serialises compression: the midnight rotation, the hourly cleanup and a
// retention change may pick the same file, and two writers of one .gz.tmp corrupt it.
var compressMu sync.Mutex

// createTmp creates the temporary .gz file. A variable so tests can make it fail.
var createTmp = func(path string) (logFile, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
}

// compress gzips a finished day file and removes the original. The original is only
// removed once the .gz is completely written, synced and renamed into place: on any
// error (e.g. a full disk) the partial .gz.tmp is removed and the original kept, so a
// later cleanup compresses it again.
func compress(path string) {
	compressMu.Lock()
	defer compressMu.Unlock()
	if _, err := os.Stat(path); err != nil {
		return // already compressed by an earlier call
	}
	if err := compressFile(path); err != nil {
		os.Remove(path + ".gz.tmp")
		slog.Warn("request log compression failed; the uncompressed file is kept", "file", filepath.Base(path), "err", err)
		return
	}
	os.Remove(path)
}

func compressFile(path string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := createTmp(path + ".gz.tmp")
	if err != nil {
		return err
	}
	zw, _ := gzip.NewWriterLevel(out, gzip.BestCompression)
	_, err = io.Copy(zw, in)
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	in.Close() // close before the original is removed (required on Windows)
	if err != nil {
		return err
	}
	return os.Rename(path+".gz.tmp", path+".gz")
}

// Archives lists log files, newest first.
func (s *Service) Archives() []Archive {
	ents, _ := os.ReadDir(s.logDir())
	var out []Archive
	for _, e := range ents {
		if !logName.MatchString(e.Name()) {
			continue
		}
		if info, err := e.Info(); err == nil {
			out = append(out, Archive{Name: e.Name(), Bytes: info.Size(), Mod: info.ModTime().UTC()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out
}

// ArchivePath returns the path of a log file by name (validated) or "".
func (s *Service) ArchivePath(name string) string {
	if !logName.MatchString(name) || strings.ContainsAny(name, `/\`) {
		return ""
	}
	p := filepath.Join(s.logDir(), name)
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// maxLogLine bounds one request log line when converting (longer lines are skipped).
const maxLogLine = 64 << 10

// WriteCombined streams a request log file (by path, .jsonl or .jsonl.gz) to w in nginx's
// "combined" log format (traffic.AppendCombined), one line at a time: memory use does
// not depend on the file size. Lines that cannot be parsed (e.g. the half-written last
// line of today's file) are skipped; lines written before 1.6.0 have no referrer or
// UTM values and are converted all the same.
func WriteCombined(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var in io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer zr.Close()
		in = zr
	}
	br := bufio.NewReaderSize(in, maxLogLine)
	bw := bufio.NewWriterSize(w, 64<<10)
	var out []byte
	long := false // inside a line longer than the buffer: skipped up to its end
	for {
		line, err := br.ReadSlice('\n')
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			long = true
			continue
		case long:
			long = false
		case len(line) > 1:
			var r traffic.Record
			if json.Unmarshal(line, &r) == nil && r.T > 0 {
				l := r.Line()
				out = traffic.AppendCombined(out[:0], &l)
				if _, werr := bw.Write(out); werr != nil {
					return werr
				}
			}
		}
		if err == io.EOF {
			return bw.Flush()
		}
		if err != nil {
			return err
		}
	}
}

// Usage reports storage use.
func (s *Service) Usage() Usage {
	u := Usage{CheckedAt: time.Now().UTC(), Dropped: s.dropped.Load()}
	for _, n := range []string{"traffic.db", "traffic.db-wal"} {
		if st, err := os.Stat(filepath.Join(s.Dir, n)); err == nil {
			u.DBBytes += st.Size()
		}
	}
	arch := s.Archives()
	for _, a := range arch {
		u.LogBytes += a.Bytes
	}
	u.LogFiles = len(arch)
	if len(arch) > 0 {
		u.Oldest = arch[len(arch)-1].Name
	}
	return u
}

// ------------------------------------------------------------------ retention

func (s *Service) cleanup(ctx context.Context, now time.Time) {
	set := Load(ctx, s.Store)
	s.mode.Store(set.LogMode)
	s.mu.Lock()
	statsCut := now.AddDate(0, 0, -set.StatsDays).Unix()
	evCut := float64(now.AddDate(0, 0, -set.EventsDays).Unix())
	_, _ = s.db.ExecContext(ctx, `DELETE FROM stats WHERE ts < ?`, statsCut)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM events WHERE ts < ?`, evCut)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM rule_events WHERE ts < ?`, evCut)
	// hard caps so a flood cannot grow the database without bound
	_, _ = s.db.ExecContext(ctx, `DELETE FROM events WHERE id <= (SELECT id FROM events ORDER BY id DESC LIMIT 1 OFFSET 1000000)`)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM rule_events WHERE id <= (SELECT id FROM rule_events ORDER BY id DESC LIMIT 1 OFFSET 1000000)`)
	_, _ = s.db.ExecContext(ctx, `PRAGMA incremental_vacuum`)
	_, _ = s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	s.mu.Unlock()

	// logs: compress leftovers, drop old days, then enforce the size cap (oldest first)
	today := now.UTC().Format("2006-01-02")
	cutDay := now.UTC().AddDate(0, 0, -set.LogDays).Format("2006-01-02")
	for _, a := range s.Archives() {
		m := logName.FindStringSubmatch(a.Name)
		if m[1] < cutDay {
			os.Remove(filepath.Join(s.logDir(), a.Name))
		} else if p := filepath.Join(s.logDir(), a.Name); m[2] == "" && m[1] < today && p != s.openLog.Load() {
			// not the file still being written (just after midnight, before rotation)
			compress(p)
		}
	}
	arch := s.Archives()
	var total int64
	for _, a := range arch {
		total += a.Bytes
	}
	limit := int64(set.LogMaxMB) << 20
	for i := len(arch) - 1; i >= 0 && total > limit; i-- {
		if logName.FindStringSubmatch(arch[i].Name)[1] == today {
			break // never delete the file being written
		}
		if os.Remove(filepath.Join(s.logDir(), arch[i].Name)) == nil {
			total -= arch[i].Bytes
		}
	}
}

// Apply stores new settings and applies them immediately.
func (s *Service) Apply(ctx context.Context, set Settings) {
	s.mode.Store(set.LogMode)
	go s.cleanup(context.Background(), time.Now())
}
