// Package geoip maintains the IP → country database used by security rules.
//
// Source: DB-IP "IP to Country Lite" (CC BY 4.0, monthly, no account needed). The CSV
// (start,end,country) is converted into a compact sorted range file that the gateway's
// Lua code binary-searches in memory (loaded once in the nginx master, shared by workers):
//
//	"CVGEO1\x00\x00" | n4 uint32 | n6 uint32                       (16-byte header, little endian)
//	v4 starts  n4 × uint32 (host order, little endian)
//	v4 codes   n4 × 2 bytes ASCII ("XX" = unknown / gap)
//	v6 starts  n6 × 16 bytes (network order)
//	v6 codes   n6 × 2 bytes
//
// Lookup: the last range whose start ≤ address.
package geoip

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	Attribution = `IP geolocation by DB-IP (https://db-ip.com), CC BY 4.0`
	urlPattern  = "https://download.db-ip.com/free/dbip-country-lite-%s.csv.gz"
	maxGzip     = 64 << 20  // compressed download cap
	maxCSV      = 512 << 20 // decompressed cap
	refreshAge  = 32 * 24 * time.Hour
	magic       = "CVGEO1\x00\x00"
)

// Status is reported to the admin UI.
type Status struct {
	Available   bool       `json:"available"`
	Source      string     `json:"source"`
	UpdatedAt   *time.Time `json:"updated_at"`
	Ranges      int        `json:"ranges"`
	Error       string     `json:"error"`
	Attribution string     `json:"attribution"`
	Disabled    bool       `json:"disabled"`
}

type Service struct {
	Dir       string // e.g. /data/geoip
	URL       string // optional fixed URL (air-gapped mirror); default = monthly DB-IP URL
	Disabled  bool
	OnUpdated func(ctx context.Context)
	Client    *http.Client

	mu      sync.Mutex
	lastErr string
}

func (s *Service) File() string     { return filepath.Join(s.Dir, "country.bin") }
func (s *Service) metaFile() string { return filepath.Join(s.Dir, "country.json") }

// Available reports whether a database file exists (the gateway loads it on reload).
func (s *Service) Available() bool {
	st, err := os.Stat(s.File())
	return err == nil && st.Size() > 16
}

func (s *Service) Status() Status {
	st := Status{Attribution: Attribution, Disabled: s.Disabled}
	var meta struct {
		Source    string    `json:"source"`
		UpdatedAt time.Time `json:"updated_at"`
		Ranges    int       `json:"ranges"`
	}
	if b, err := os.ReadFile(s.metaFile()); err == nil && json.Unmarshal(b, &meta) == nil && s.Available() {
		st.Available, st.Source, st.Ranges = true, meta.Source, meta.Ranges
		t := meta.UpdatedAt
		st.UpdatedAt = &t
	}
	s.mu.Lock()
	st.Error = s.lastErr
	s.mu.Unlock()
	return st
}

// Run refreshes at start when the database is missing or older than a month, then daily.
func (s *Service) Run(ctx context.Context) {
	if s.Disabled {
		return
	}
	check := func() {
		if st, err := os.Stat(s.File()); err == nil && time.Since(st.ModTime()) < refreshAge {
			return
		}
		if _, err := s.Refresh(ctx); err != nil {
			slog.Warn("country database update failed", "err", err)
		}
	}
	check()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			check()
		}
	}
}

// Refresh downloads, converts and installs the database now.
func (s *Service) Refresh(ctx context.Context) (Status, error) {
	err := s.refresh(ctx)
	s.mu.Lock()
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.lastErr = ""
	}
	s.mu.Unlock()
	if err == nil && s.OnUpdated != nil {
		s.OnUpdated(ctx)
	}
	return s.Status(), err
}

func (s *Service) refresh(ctx context.Context) error {
	if s.Disabled {
		return errors.New("country database updates are disabled (CORVUS_GEOIP=off)")
	}
	var urls []string
	if s.URL != "" {
		urls = []string{s.URL}
	} else {
		now := time.Now().UTC()
		urls = []string{fmt.Sprintf(urlPattern, now.Format("2006-01")), fmt.Sprintf(urlPattern, now.AddDate(0, -1, 0).Format("2006-01"))}
	}
	var lastErr error
	for _, u := range urls {
		db, err := s.download(ctx, u)
		if err != nil {
			lastErr = err
			continue
		}
		if err := db.sanity(); err != nil {
			return fmt.Errorf("%s: %w", u, err)
		}
		source := "DB-IP Lite " + strings.TrimSuffix(strings.TrimPrefix(filepath.Base(u), "dbip-country-lite-"), ".csv.gz")
		return s.install(db, source)
	}
	return lastErr
}

func (s *Service) download(ctx context.Context, url string) (*DB, error) {
	c := s.Client
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Minute}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "corvus-awpg")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	zr, err := gzip.NewReader(io.LimitReader(resp.Body, maxGzip))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	defer zr.Close()
	return ParseCSV(io.LimitReader(zr, maxCSV))
}

// DB is the in-memory range table.
type DB struct {
	v4 []r4
	v6 []r6
}

type r4 struct {
	start uint32
	cc    [2]byte
}

type r6 struct {
	start [16]byte
	cc    [2]byte
}

var unknown = [2]byte{'X', 'X'}

// maxRows caps the ranges read from a CSV (DB-IP country lite has about one
// million), so a hostile or broken download cannot exhaust memory.
var maxRows = 4_000_000

// ParseCSV reads "start,end,CC" lines (IPv4 and IPv6 mixed, sorted or not).
func ParseCSV(r io.Reader) (*DB, error) {
	type span4 struct {
		a, b uint32
		cc   [2]byte
	}
	type span6 struct {
		a, b [16]byte
		cc   [2]byte
	}
	var s4 []span4
	var s6 []span6
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 64<<10)
	line, rows := 0, 0
	for sc.Scan() {
		line++
		f := strings.Split(strings.ReplaceAll(sc.Text(), `"`, ""), ",")
		if len(f) < 3 {
			continue
		}
		a, b := net.ParseIP(strings.TrimSpace(f[0])), net.ParseIP(strings.TrimSpace(f[1]))
		code := strings.ToUpper(strings.TrimSpace(f[2]))
		if a == nil || b == nil {
			continue
		}
		if rows++; rows > maxRows {
			return nil, fmt.Errorf("line %d: more than %d ranges", line, maxRows)
		}
		cc := unknown
		if len(code) == 2 && code[0] >= 'A' && code[0] <= 'Z' && code[1] >= 'A' && code[1] <= 'Z' && code != "ZZ" {
			cc = [2]byte{code[0], code[1]}
		}
		a4, b4 := a.To4(), b.To4()
		if (a4 == nil) != (b4 == nil) {
			return nil, fmt.Errorf("line %d: range mixes IPv4 and IPv6", line)
		}
		if a4 != nil {
			x, y := binary.BigEndian.Uint32(a4), binary.BigEndian.Uint32(b4)
			if y < x {
				return nil, fmt.Errorf("line %d: range ends before it starts", line)
			}
			s4 = append(s4, span4{x, y, cc})
			continue
		}
		var x, y [16]byte
		copy(x[:], a.To16())
		copy(y[:], b.To16())
		if bytes.Compare(y[:], x[:]) < 0 {
			return nil, fmt.Errorf("line %d: range ends before it starts", line)
		}
		s6 = append(s6, span6{x, y, cc})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("line %d: %w", line, err)
	}
	sort.Slice(s4, func(i, j int) bool { return s4[i].a < s4[j].a })
	sort.Slice(s6, func(i, j int) bool { return bytes.Compare(s6[i].a[:], s6[j].a[:]) < 0 })

	db := &DB{}
	add4 := func(start uint32, cc [2]byte) {
		if n := len(db.v4); n > 0 && db.v4[n-1].cc == cc {
			return
		}
		db.v4 = append(db.v4, r4{start, cc})
	}
	add4(0, unknown)
	var next uint64 // first address not yet covered
	for _, sp := range s4 {
		if uint64(sp.a) > next {
			add4(uint32(next), unknown)
		}
		if uint64(sp.b)+1 <= next {
			continue // fully overlapped
		}
		start := sp.a
		if uint64(start) < next {
			start = uint32(next)
		}
		add4(start, sp.cc)
		next = uint64(sp.b) + 1
	}
	if next <= 0xFFFFFFFF {
		add4(uint32(next), unknown)
	}

	add6 := func(start [16]byte, cc [2]byte) {
		if n := len(db.v6); n > 0 && db.v6[n-1].cc == cc {
			return
		}
		db.v6 = append(db.v6, r6{start, cc})
	}
	add6([16]byte{}, unknown)
	var next6 [16]byte
	done := false // next6 overflowed past ::ffff…
	for _, sp := range s6 {
		if done {
			break
		}
		if bytes.Compare(sp.b[:], next6[:]) < 0 {
			continue
		}
		start := sp.a
		if bytes.Compare(sp.a[:], next6[:]) > 0 {
			add6(next6, unknown)
		} else {
			start = next6
		}
		add6(start, sp.cc)
		next6, done = inc(sp.b)
	}
	if !done {
		add6(next6, unknown)
	}
	return db, nil
}

func inc(a [16]byte) ([16]byte, bool) {
	for i := 15; i >= 0; i-- {
		a[i]++
		if a[i] != 0 {
			return a, false
		}
	}
	return a, true
}

// Lookup returns the country code of ip ("XX" when unknown).
func (db *DB) Lookup(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		x := binary.BigEndian.Uint32(v4)
		i := sort.Search(len(db.v4), func(i int) bool { return db.v4[i].start > x }) - 1
		if i < 0 {
			return "XX"
		}
		return string(db.v4[i].cc[:])
	}
	var x [16]byte
	copy(x[:], ip.To16())
	i := sort.Search(len(db.v6), func(i int) bool { return bytes.Compare(db.v6[i].start[:], x[:]) > 0 }) - 1
	if i < 0 {
		return "XX"
	}
	return string(db.v6[i].cc[:])
}

func (db *DB) Ranges() int { return len(db.v4) + len(db.v6) }

// sanity rejects truncated or nonsensical downloads before they replace a good database.
func (db *DB) sanity() error {
	if len(db.v4) < 10000 {
		return fmt.Errorf("only %d IPv4 ranges — refusing a truncated database", len(db.v4))
	}
	if got := db.Lookup(net.ParseIP("8.8.8.8")); got != "US" {
		return fmt.Errorf("sanity check failed (8.8.8.8 → %s)", got)
	}
	return nil
}

// Encode writes the binary range file.
func (db *DB) Encode(w io.Writer) error {
	bw := bufio.NewWriter(w)
	hdr := make([]byte, 16)
	copy(hdr, magic)
	binary.LittleEndian.PutUint32(hdr[8:], uint32(len(db.v4)))
	binary.LittleEndian.PutUint32(hdr[12:], uint32(len(db.v6)))
	bw.Write(hdr)
	b4 := make([]byte, 4)
	for _, r := range db.v4 {
		binary.LittleEndian.PutUint32(b4, r.start)
		bw.Write(b4)
	}
	for _, r := range db.v4 {
		bw.Write(r.cc[:])
	}
	for _, r := range db.v6 {
		bw.Write(r.start[:])
	}
	for _, r := range db.v6 {
		bw.Write(r.cc[:])
	}
	return bw.Flush()
}

func (s *Service) install(db *DB, source string) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	tmp := s.File() + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if err := db.Encode(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.File()); err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]any{"source": source, "updated_at": time.Now().UTC().Truncate(time.Second), "ranges": db.Ranges()})
	if err := os.WriteFile(s.metaFile(), meta, 0o644); err != nil {
		return err
	}
	slog.Info("country database installed", "source", source, "ranges", db.Ranges())
	return nil
}
