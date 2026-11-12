package gateway

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"time"
)

// errLog passes the gateway's stderr through unchanged and keeps its recent serious
// lines ([emerg], [alert], [crit]), so a rejected reload can say why (for example
// "bind() to 0.0.0.0:8090 failed (98: Address already in use)").
type errLog struct {
	out io.Writer

	mu      sync.Mutex
	partial []byte
	lines   []errLine
}

type errLine struct {
	at   time.Time
	text string
}

const errLogKeep = 50

func newErrLog(out io.Writer) *errLog { return &errLog{out: out} }

func (l *errLog) Write(p []byte) (int, error) {
	n, err := l.out.Write(p)
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	buf := append(l.partial, p...)
	for {
		i := bytes.IndexByte(buf, '\n')
		if i < 0 {
			break
		}
		l.keep(now, string(buf[:i]))
		buf = buf[i+1:]
	}
	if len(buf) > 4096 { // a runaway line: keep the start, drop the rest
		l.keep(now, string(buf[:4096]))
		buf = nil
	}
	l.partial = append([]byte(nil), buf...)
	return n, err
}

func (l *errLog) keep(at time.Time, line string) {
	if !strings.Contains(line, "[emerg]") && !strings.Contains(line, "[alert]") && !strings.Contains(line, "[crit]") {
		return
	}
	l.lines = append(l.lines, errLine{at, strings.TrimSpace(line)})
	if len(l.lines) > errLogKeep {
		l.lines = l.lines[len(l.lines)-errLogKeep:]
	}
}

// since returns the serious lines written at or after t, oldest first (at most 5).
func (l *errLog) since(t time.Time) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, e := range l.lines {
		if !e.at.Before(t) {
			out = append(out, e.text)
		}
	}
	if len(out) > 5 {
		out = out[len(out)-5:]
	}
	return out
}
