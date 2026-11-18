package gateway

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"corvus/control/internal/hints"
)

func TestErrLogKeepsSeriousLines(t *testing.T) {
	var out bytes.Buffer
	l := newErrLog(&out)
	start := time.Now()
	// written in pieces, as a pipe delivers it
	parts := []string{
		"2026/11/12 01:55:15 [warn] 63#63: *39 bad behaviour ban 10.0.0.1\n2026/11/12 01:55:16 [em",
		"erg] 1#1: bind() to 0.0.0.0:8090 failed (98: Address already in use)\n",
		"2026/11/12 01:55:16 [error] 63#63: *40 connect() failed (111: Connection refused)\n",
		"2026/11/12 01:55:17 [alert] 1#1: worker process 70 exited on signal 9\npartial [crit] line",
	}
	for _, p := range parts {
		if n, err := l.Write([]byte(p)); err != nil || n != len(p) {
			t.Fatalf("write: %d %v", n, err)
		}
	}
	if out.String() != strings.Join(parts, "") {
		t.Fatal("stderr is not passed through unchanged")
	}
	got := l.since(start)
	if len(got) != 2 || !strings.Contains(got[0], "[emerg]") || !strings.Contains(got[1], "[alert]") {
		t.Fatalf("serious lines: %q", got)
	}
	if l.since(time.Now().Add(time.Second)) != nil {
		t.Fatal("lines before the reload must not be reported")
	}
	// the unfinished line is kept once it ends
	_, _ = l.Write([]byte("\n"))
	if got := l.since(start); len(got) != 3 || got[2] != "partial [crit] line" {
		t.Fatalf("partial line: %q", got)
	}
}

func TestErrLogBounds(t *testing.T) {
	l := newErrLog(&bytes.Buffer{})
	start := time.Now()
	for i := 0; i < 80; i++ {
		_, _ = fmt.Fprintf(l, "x [emerg] 1#1: line %d\n", i)
	}
	if len(l.lines) != errLogKeep {
		t.Fatalf("kept %d lines", len(l.lines))
	}
	got := l.since(start)
	if len(got) != 5 || !strings.HasSuffix(got[4], "line 79") {
		t.Fatalf("since: %q", got)
	}
	_, _ = l.Write([]byte("[crit] " + strings.Repeat("a", 6000)))
	if l.partial != nil && len(l.partial) != 0 {
		t.Fatalf("runaway line kept in the buffer: %d bytes", len(l.partial))
	}
}

// The rest of a truncated runaway line, arriving in later writes, is not taken for new
// lines; the line after it is.
func TestErrLogRunawayTail(t *testing.T) {
	l := newErrLog(&bytes.Buffer{})
	start := time.Now()
	_, _ = l.Write([]byte("x [crit] head " + strings.Repeat("a", 5000)))
	_, _ = l.Write([]byte(strings.Repeat("b", 3000)))
	_, _ = l.Write([]byte("bbb [emerg] tail of the same line\nnext [alert] line\n"))
	got := l.since(start)
	if len(got) != 2 || !strings.HasPrefix(got[0], "x [crit] head") || got[1] != "next [alert] line" {
		t.Fatalf("lines: %q", got)
	}
	// and a forged upstream failure in the tail is ignored too
	_, _ = l.Write([]byte("y " + strings.Repeat("c", 5000)))
	_, _ = l.Write([]byte("\n" + lnRefused + "\n"))
	if fs := l.upstreamFailures(); len(fs) != 1 || fs[0].Server != "a.test" {
		t.Fatalf("after a runaway line: %+v", fs)
	}
	l2 := newErrLog(&bytes.Buffer{})
	_, _ = l2.Write([]byte("z " + strings.Repeat("d", 5000)))
	_, _ = l2.Write([]byte(lnRefused + "\n")) // the visitor-controlled end of the long line
	if fs := l2.upstreamFailures(); len(fs) != 0 {
		t.Fatalf("tail parsed as a line: %+v", fs)
	}
}

// Once the gateway serves a new configuration, failures of the previous one are dropped.
func TestWaitActiveClearsUpstreamFailures(t *testing.T) {
	serving := "v000001"
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write([]byte(serving))
	}))
	defer srv.Close()
	o := &OpenResty{StatusURL: srv.URL, running: true, errs: newErrLog(&bytes.Buffer{})}
	_, _ = o.errs.Write([]byte(lnRefused + "\n"))
	// not (yet) serving the new version: kept
	if err := o.WaitActive(context.Background(), "v000002", 100*time.Millisecond); err == nil {
		t.Fatal("v000002 is not serving")
	}
	if len(o.UpstreamFailures()) != 1 {
		t.Fatal("failure dropped while the previous configuration still serves")
	}
	mu.Lock()
	serving = "v000002"
	mu.Unlock()
	if err := o.WaitActive(context.Background(), "v000002", time.Second); err != nil {
		t.Fatal(err)
	}
	if fs := o.UpstreamFailures(); len(fs) != 0 {
		t.Fatalf("failures of the previous configuration: %+v", fs)
	}
	_, _ = o.errs.Write([]byte(lnDNS + "\n"))
	if len(o.UpstreamFailures()) != 1 {
		t.Fatal("new failures are kept")
	}
}

func TestNotAcceptedErrorMessage(t *testing.T) {
	plain := (&NotAcceptedError{Want: "v000009", Serving: "v000008"}).Error()
	if !strings.Contains(plain, "see the gateway log") || hints.Reload(plain) != "" {
		t.Fatalf("without log lines: %q", plain)
	}
	e := &NotAcceptedError{Want: "v000009", Serving: "v000008", Log: []string{
		"2026/11/12 01:55:16 [emerg] 1#1: bind() to 0.0.0.0:8090 failed (98: Address already in use)",
	}}
	msg := e.Error()
	if !strings.HasSuffix(msg, ": bind() to 0.0.0.0:8090 failed (98: Address already in use)") {
		t.Fatalf("message: %q", msg)
	}
	if hints.Reload(msg) != hints.DeployPortInUse {
		t.Fatalf("hint for %q", msg)
	}
	rep := &NotAcceptedError{Want: "b", Serving: "a", Log: []string{
		"t [emerg] 1#1: bind() to 0.0.0.0:8090 failed (98: Address in use)",
		"t [emerg] 1#1: bind() to [::]:8090 failed (98: Address in use)",
		"t [emerg] 1#1: bind() to 0.0.0.0:8090 failed (98: Address in use)",
		"t [emerg] 1#1: still could not bind()",
	}}
	if got := rep.Error(); strings.Count(got, "0.0.0.0:8090") != 1 || !strings.HasSuffix(got, "still could not bind()") {
		t.Fatalf("repeated lines: %q", got)
	}
	if got := logMessage("no level here"); got != "no level here" {
		t.Fatalf("logMessage: %q", got)
	}
}
