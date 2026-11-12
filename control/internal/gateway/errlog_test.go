package gateway

import (
	"bytes"
	"fmt"
	"strings"
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
