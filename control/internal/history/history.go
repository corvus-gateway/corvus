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
// current (unfinished) bucket too, closes the database and returns; callers wait for it
// so a restart or upgrade does not lose the last minutes.
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

func (s *Service) writeLogs(ctx context.Context) {
	var f *os.File
	var w *bufio.Writer
	day := ""
	open := func(now time.Time) {
		d := now.UTC().Format("2006-01-02")
		if d == day && f != nil {
			return
		}
		if f != nil {
			w.Flush()
			f.Close()
			prev := f.Name()
			go compress(prev)
		}
		var err error
		f, err = os.OpenFile(filepath.Join(s.logDir(), "access-"+d+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			slog.Warn("request log", "err", err)
			f = nil
			return
		}
		w = bufio.NewWriterSize(f, 256<<10)
		day = d
		s.openLog.Store(f.Name())
	}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			for drained := false; !drained; {
				select {
				case line := <-s.lines:
					open(time.Now())
					if f != nil {
						w.Write(line)
						w.WriteByte('\n')
					}
				default:
					drained = true
				}
			}
			if f != nil {
				w.Flush()
				f.Close()
			}
			return
		case line := <-s.lines:
			open(time.Now())
			if f != nil {
				w.Write(line)
				w.WriteByte('\n')
			}
		case now := <-tick.C:
			if f != nil {
				w.Flush()
				if now.UTC().Format("2006-01-02") != day {
					open(now) // rotate at midnight even when idle
				}
			}
		}
	}
}

// compressMu serialises compression: the midnight rotation, the hourly cleanup and a
// retention change may pick the same file, and two writers of one .gz.tmp corrupt it.
var compressMu sync.Mutex

// compress gzips a finished day file and removes the original.
func compress(path string) {
	compressMu.Lock()
	defer compressMu.Unlock()
	if _, err := os.Stat(path); err != nil {
		return // already compressed by an earlier call
	}
	in, err := os.Open(path)
	if err != nil {
		return
	}
	out, err := os.OpenFile(path+".gz.tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		in.Close()
		return
	}
	zw, _ := gzip.NewWriterLevel(out, gzip.BestCompression)
	_, err = io.Copy(zw, in)
	in.Close() // close before removing (required on Windows)
	if err != nil {
		zw.Close()
		out.Close()
		os.Remove(path + ".gz.tmp")
		return
	}
	zw.Close()
	out.Close()
	if os.Rename(path+".gz.tmp", path+".gz") == nil {
		os.Remove(path)
	}
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
