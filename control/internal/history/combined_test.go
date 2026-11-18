package history

import (
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Request log files convert to nginx "combined" lines, plain and compressed, including
// lines written before the traffic-source fields existed; broken and over-long lines
// are skipped without stopping the conversion.
func TestWriteCombined(t *testing.T) {
	dir := t.TempDir()
	lines := strings.Join([]string{
		`{"t":1762243516.123,"h":"a.example","m":"GET","u":"/p","s":200,"b":5,"rt":0.01,"ip":"203.0.113.9","wr":"-","ua":"UA \"x\"","rf":"https://www.google.com","us":"ig","uc":"fall"}`,
		`{"t":1762243517,"h":"a.example","m":"POST","u":"/login","s":403,"b":0,"rt":0,"ip":"198.51.100.4","wr":"-","x":"sqli","ua":""}`, // before 1.6.0
		`{"t":1762243518,"h":"a.example","m":"GET","u":"/` + strings.Repeat("x", maxLogLine) + `","s":200}`,                             // too long
		`not json`,
		``,
		`{"t":1762243519,"h":"a.example","m":"GET","u":"/last","s":304,"ip":"203.0.113.10"}`,
		`{"t":1762243520,"h":"a.exa`, // half-written last line
	}, "\n")
	want := `203.0.113.9 - - [04/Nov/2025:08:05:16 +0000] "GET /p?utm_source=ig&utm_campaign=fall HTTP/1.1" 200 5 "https://www.google.com" "UA \"x\""` + "\n" +
		`198.51.100.4 - - [04/Nov/2025:08:05:17 +0000] "POST /login HTTP/1.1" 403 0 "-" "-"` + "\n" +
		`203.0.113.10 - - [04/Nov/2025:08:05:19 +0000] "GET /last HTTP/1.1" 304 0 "-" "-"` + "\n"

	plain := filepath.Join(dir, "access-2025-11-04.jsonl")
	if err := os.WriteFile(plain, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	var zb bytes.Buffer
	zw := gzip.NewWriter(&zb)
	zw.Write([]byte(lines))
	zw.Close()
	gz := filepath.Join(dir, "access-2025-11-03.jsonl.gz")
	if err := os.WriteFile(gz, zb.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{plain, gz} {
		var out bytes.Buffer
		if err := WriteCombined(&out, p); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if out.String() != want {
			t.Errorf("%s:\n got %s\nwant %s", filepath.Base(p), out.String(), want)
		}
	}

	if err := WriteCombined(&bytes.Buffer{}, filepath.Join(dir, "missing.jsonl")); err == nil {
		t.Error("missing file: no error")
	}
	bad := filepath.Join(dir, "bad.jsonl.gz")
	os.WriteFile(bad, []byte("not gzip"), 0o600)
	if err := WriteCombined(&bytes.Buffer{}, bad); err == nil {
		t.Error("broken gzip: no error")
	}
	if err := WriteCombined(failWriter{}, plain); err == nil {
		t.Error("write error not returned")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }
