package geoip

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// bigCSV returns a CSV with more than 10000 IPv4 ranges (alternating countries so they
// do not merge) and 8.8.8.0/24 in the US, which passes the sanity check.
func bigCSV(n int) []byte {
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		a := uint32(0x01000000 + i*256)
		cc := "DE"
		if i%2 == 1 {
			cc = "FR"
		}
		fmt.Fprintf(&b, "%d.%d.%d.0,%d.%d.%d.255,%s\n", a>>24, a>>16&0xff, a>>8&0xff, a>>24, a>>16&0xff, a>>8&0xff, cc)
	}
	b.WriteString("8.8.8.0,8.8.8.255,US\n2001:4860::,2001:4860:ffff:ffff:ffff:ffff:ffff:ffff,US\n")
	return b.Bytes()
}

func gz(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write(data)
	_ = w.Close()
	return b.Bytes()
}

func TestCovRefreshInstallsAndReportsStatus(t *testing.T) {
	body := gz(t, bigCSV(12000))
	var ua atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua.Store(r.Header.Get("User-Agent"))
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dir := filepath.Join(t.TempDir(), "geoip")
	updated := 0
	s := &Service{Dir: dir, URL: srv.URL + "/dbip-country-lite-2026-11.csv.gz", Client: srv.Client(), OnUpdated: func(context.Context) { updated++ }}

	if s.Available() {
		t.Fatal("no database yet")
	}
	st := s.Status()
	if st.Available || st.UpdatedAt != nil || st.Attribution != Attribution {
		t.Fatalf("empty status: %+v", st)
	}

	st, err := s.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Available || st.Source != "DB-IP Lite 2026-11" || st.Ranges < 10000 || st.UpdatedAt == nil || st.Error != "" {
		t.Fatalf("status after refresh: %+v", st)
	}
	if updated != 1 {
		t.Fatalf("OnUpdated called %d times", updated)
	}
	if ua.Load() != "corvus-awpg" {
		t.Fatalf("user agent %v", ua.Load())
	}
	b, err := os.ReadFile(s.File())
	if err != nil || !bytes.HasPrefix(b, []byte(magic)) {
		t.Fatalf("installed file: %v", err)
	}
	if _, err := os.Stat(s.File() + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary file left behind")
	}
}

func TestCovRefreshErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := (&Service{Disabled: true}).Refresh(ctx); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled = %v", err)
	}

	var mode atomic.Int32
	small := gz(t, []byte("1.0.0.0,1.0.0.255,AU\n8.8.8.0,8.8.8.255,US\n"))
	wrong := gz(t, bytes.ReplaceAll(bigCSV(12000), []byte("8.8.8.255,US"), []byte("8.8.8.255,CA")))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch mode.Load() {
		case 0:
			http.Error(w, "gone", http.StatusNotFound)
		case 1:
			_, _ = w.Write([]byte("not gzip"))
		case 2:
			_, _ = w.Write(small)
		case 3:
			_, _ = w.Write(wrong)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	s := &Service{Dir: dir, URL: srv.URL + "/x.csv.gz", Client: srv.Client()}

	for m, want := range map[int32]string{0: "HTTP 404", 1: "x.csv.gz: ", 2: "refusing a truncated database", 3: "8.8.8.8 → CA"} {
		mode.Store(m)
		st, err := s.Refresh(ctx)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("mode %d: err = %v, want %q", m, err, want)
			continue
		}
		if st.Error != err.Error() || st.Available {
			t.Errorf("mode %d: status %+v", m, st)
		}
	}
	if s.Available() {
		t.Fatal("a rejected download must not be installed")
	}

	// transport error
	bad := &Service{Dir: dir, URL: "http://127.0.0.1:1/x", Client: &http.Client{Timeout: time.Second}}
	if _, err := bad.Refresh(ctx); err == nil {
		t.Fatal("unreachable mirror must fail")
	}
	// invalid URL
	if _, err := (&Service{Dir: dir, URL: "://bad"}).Refresh(ctx); err == nil {
		t.Fatal("invalid URL must fail")
	}
}

func TestCovInstallErrors(t *testing.T) {
	db, _ := ParseCSV(strings.NewReader("1.0.0.0,1.0.0.255,AU\n"))
	f := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(f, nil, 0o644)
	if err := (&Service{Dir: filepath.Join(f, "sub")}).install(db, "x"); err == nil {
		t.Fatal("install below a file must fail")
	}
	dir := t.TempDir()
	// country.bin is a directory: the rename fails
	_ = os.MkdirAll(filepath.Join(dir, "country.bin", "x"), 0o755)
	if err := (&Service{Dir: dir}).install(db, "x"); err == nil {
		t.Fatal("rename onto a directory must fail")
	}
	dir2 := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir2, "country.bin.tmp", "x"), 0o755)
	if err := (&Service{Dir: dir2}).install(db, "x"); err == nil {
		t.Fatal("unwritable temp file must fail")
	}
	dir3 := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir3, "country.json", "x"), 0o755)
	if err := (&Service{Dir: dir3}).install(db, "x"); err == nil {
		t.Fatal("unwritable meta file must fail")
	}
	if db.Ranges() != len(db.v4)+len(db.v6) || db.Ranges() < 3 {
		t.Fatalf("ranges = %d", db.Ranges())
	}
}

func TestCovRun(t *testing.T) {
	// disabled: returns immediately
	done := make(chan struct{})
	go func() { (&Service{Disabled: true}).Run(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("disabled Run must return")
	}

	// fresh database: no download at start; returns when ctx ends
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	defer srv.Close()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "country.bin"), make([]byte, 64), 0o644)
	s := &Service{Dir: dir, URL: srv.URL, Client: srv.Client()}
	ctx, cancel := context.WithCancel(context.Background())
	done = make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	if hits.Load() != 0 {
		t.Fatal("a fresh database must not be downloaded again")
	}

	// stale database: downloaded at start (fails here, error recorded)
	old := time.Now().Add(-40 * 24 * time.Hour)
	_ = os.Chtimes(filepath.Join(dir, "country.bin"), old, old)
	ctx, cancel = context.WithCancel(context.Background())
	done = make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	for hits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if hits.Load() == 0 {
		t.Fatal("stale database must be refreshed")
	}
	if st := s.Status(); !strings.Contains(st.Error, "HTTP 500") {
		t.Fatalf("error not recorded: %+v", st)
	}
}

func TestCovDefaultURLs(t *testing.T) {
	// Without a fixed URL the monthly DB-IP URLs are tried in order; route them to a
	// local server through the client's transport.
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Host+r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", srv.Listener.Addr().String())
	}}
	s := &Service{Dir: t.TempDir(), Client: &http.Client{Transport: rewriteHTTPS{tr}}}
	_, err := s.Refresh(context.Background())
	if err == nil || len(paths) != 2 {
		t.Fatalf("err=%v paths=%v", err, paths)
	}
	now := time.Now().UTC()
	if !strings.Contains(paths[0], now.Format("2006-01")) || !strings.Contains(paths[1], now.AddDate(0, -1, 0).Format("2006-01")) {
		t.Fatalf("paths %v", paths)
	}
}

// rewriteHTTPS sends https requests as plain http to the test server.
type rewriteHTTPS struct{ rt http.RoundTripper }

func (r rewriteHTTPS) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	return r.rt.RoundTrip(req)
}
