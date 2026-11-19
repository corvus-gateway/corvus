package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/history"
	"corvus/control/internal/traffic"
)

// Traffic sources (referrer origin, UTM values) appear in the request log, its search and
// every export; the combined export suits GoAccess and contains nothing else of the URL.
func TestTrafficSourcesInLogExports(t *testing.T) {
	e := newFullEnv(t)
	logViews.Delete(e.admin.ID)
	now := float64(time.Now().Unix())
	e.s.Traffic.Add(&traffic.Record{T: now, H: "a.example", M: "GET", U: "/landing", S: 200, B: 42, IP: "203.0.113.7",
		UA: `Bot "x"`, RF: "https://www.google.com/search?q=secret", US: "ig", UC: "fall"})
	e.s.Traffic.Add(&traffic.Record{T: now + 1, H: "a.example", M: "GET", U: "/other", S: 200, IP: "203.0.113.8"})

	m := e.must(200, "GET", "/api/traffic/logs?q=google", nil)
	items, _ := m["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("search by referrer: %v", m)
	}
	l := items[0].(map[string]any)
	if l["ref"] != "https://www.google.com" || l["utm_source"] != "ig" || l["utm_campaign"] != "fall" || l["utm_medium"] != nil {
		t.Fatalf("log line: %v", l)
	}

	w, _ := e.do("GET", "/api/traffic/logs?format=csv&q=ig", nil, withCookie(e.cookie))
	rows, err := csv.NewReader(bytes.NewReader(w.Body.Bytes())).ReadAll()
	if err != nil || len(rows) != 2 || strings.Join(rows[0][11:], ",") != "referrer,utm_source,utm_medium,utm_campaign" ||
		strings.Join(rows[1][11:], ",") != "https://www.google.com,ig,,fall" {
		t.Fatalf("csv: %v %v", rows, err)
	}

	w, _ = e.do("GET", "/api/traffic/logs?format=combined", nil, withCookie(e.cookie))
	body := w.Body.String()
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") ||
		!strings.Contains(w.Header().Get("Content-Disposition"), ".log") {
		t.Fatalf("combined: %d %v", w.Code, w.Header())
	}
	got := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	if len(got) != 2 || !strings.HasPrefix(got[0], "203.0.113.7 - - [") ||
		!strings.HasSuffix(got[0], `"GET /landing?utm_source=ig&utm_campaign=fall HTTP/1.1" 200 42 "https://www.google.com" "Bot \"x\""`) ||
		!strings.HasSuffix(got[1], `"GET /other HTTP/1.1" 200 0 "-" "-"`) || strings.Contains(body, "secret") {
		t.Fatalf("combined lines (oldest first):\n%s", body)
	}
}

func TestArchiveCombinedDownload(t *testing.T) {
	e := newFullEnv(t)
	op := e.as("operator")
	dir := t.TempDir()
	h := &history.Service{Dir: dir, Store: e.s.Store, Collector: e.s.Traffic}
	if err := h.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.s.History = h
	e.h = e.s.Handler()
	name := "access-2000-01-02.jsonl"
	line := `{"t":946771200,"h":"a.example","m":"GET","u":"/","s":200,"b":1,"ip":"203.0.113.9","ua":"UA","rf":"https://t.co","um":"social"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "logs", name), []byte(line), 0o640); err != nil {
		t.Fatal(err)
	}
	w, _ := e.zsAs(op, "GET", "/api/traffic/archives/"+name+"?format=combined", nil)
	want := `203.0.113.9 - - [02/Jan/2000:00:00:00 +0000] "GET /?utm_medium=social HTTP/1.1" 200 1 "https://t.co" "UA"` + "\n"
	if w.Code != 200 || w.Body.String() != want || !strings.Contains(w.Header().Get("Content-Disposition"), `filename="access-2000-01-02.log"`) {
		t.Fatalf("combined archive: %d %v %q", w.Code, w.Header(), w.Body.String())
	}
	if w, _ := e.zsAs(op, "GET", "/api/traffic/archives/access-1999-01-01.jsonl?format=combined", nil); w.Code != 404 {
		t.Fatalf("missing archive: %d", w.Code)
	}

	// not gzip data: an error answer, not an empty "access-2000-01-03.log"
	if err := os.WriteFile(filepath.Join(dir, "logs", "access-2000-01-03.jsonl.gz"), []byte("not gzip"), 0o640); err != nil {
		t.Fatal(err)
	}
	w, _ = e.zsAs(op, "GET", "/api/traffic/archives/access-2000-01-03.jsonl.gz?format=combined", nil)
	if w.Code != 500 || w.Header().Get("Content-Disposition") != "" {
		t.Fatalf("broken archive: %d %v %q", w.Code, w.Header(), w.Body.String())
	}

	// corrupt data after a valid start: part of the file is sent, then the connection is
	// aborted (http.ErrAbortHandler) so the download fails instead of looking complete
	var zb bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&zb, gzip.NoCompression)
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(zw, `{"t":946857600,"h":"a.example","m":"GET","u":"/page/%d","s":200,"ip":"203.0.113.9","ua":"UA"}`+"\n", i)
	}
	zw.Close()
	cut := filepath.Join(dir, "logs", "access-2000-01-04.jsonl.gz")
	if err := os.WriteFile(cut, zb.Bytes()[:zb.Len()*3/4], 0o640); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/traffic/archives/access-2000-01-04.jsonl.gz?format=combined", nil)
	withCookie(op)(req)
	withCSRF()(req)
	func() {
		defer func() {
			if p := recover(); p != http.ErrAbortHandler {
				t.Errorf("truncated archive: recovered %v, want http.ErrAbortHandler", p)
			}
		}()
		e.h.ServeHTTP(rec, req)
	}()
	if rec.Body.Len() == 0 || rec.Code != 200 {
		t.Errorf("truncated archive: %d, %d bytes before the abort", rec.Code, rec.Body.Len())
	}
}
