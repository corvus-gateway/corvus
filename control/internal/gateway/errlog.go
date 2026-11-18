package gateway

import (
	"bytes"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"corvus/control/internal/conncheck"
)

// errLog passes the gateway's stderr through unchanged and keeps its recent serious
// lines ([emerg], [alert], [crit]), so a rejected reload can say why (for example
// "bind() to 0.0.0.0:8090 failed (98: Address already in use)"). It also keeps the
// latest upstream failure per server (why a host answers 502/504, see UpstreamFailure).
type errLog struct {
	out io.Writer

	mu      sync.Mutex
	partial []byte
	skip    bool // discard up to the next newline (the rest of a truncated line)
	lines   []errLine
	fails   map[string]UpstreamFailure // by server name
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
	if l.skip { // the rest of a runaway line kept truncated before
		i := bytes.IndexByte(buf, '\n')
		if i < 0 {
			l.partial = nil
			return n, err
		}
		buf, l.skip = buf[i+1:], false
	}
	for {
		i := bytes.IndexByte(buf, '\n')
		if i < 0 {
			break
		}
		l.keep(now, string(buf[:i]))
		buf = buf[i+1:]
	}
	if len(buf) > 4096 { // a runaway line: keep the start, drop the rest up to its end
		l.keep(now, string(buf[:4096]))
		buf, l.skip = nil, true
	}
	l.partial = append([]byte(nil), buf...)
	return n, err
}

func (l *errLog) keep(at time.Time, line string) {
	if f, ok := parseUpstreamFailure(line); ok {
		f.At = at
		l.upstreamFailure(f)
		return
	}
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

// UpstreamFailure is the latest failed attempt of a server block to reach its upstream,
// from the gateway's [error] lines (error_log level warn), e.g.
//
//	[error] 10#10: *3 connect() failed (111: Connection refused) while connecting to
//	upstream, client: 192.0.2.1, server: shop.example.com, request: "GET / HTTP/1.1",
//	upstream: "http://10.0.0.5:8080/", host: "shop.example.com"
//
// Server is the first server_name of the block (from the configuration, not from the
// request), so it identifies the proxy host; client address and request are not kept.
type UpstreamFailure struct {
	Server   string
	At       time.Time
	Kind     string // conncheck kinds: dns, refused, timeout, unreachable, tls, other
	Upstream string // host:port tried; "" when nginx names the upstream group (no address)
	Message  string // nginx's reason, e.g. "connect() failed (111: Connection refused) while connecting to upstream"
}

// upstreamFailuresMax bounds the table. Server names come from the configuration, so
// this is only reached with that many hosts failing.
const upstreamFailuresMax = 1000

func (l *errLog) upstreamFailure(f UpstreamFailure) {
	if l.fails == nil {
		l.fails = map[string]UpstreamFailure{}
	}
	if _, ok := l.fails[f.Server]; !ok && len(l.fails) >= upstreamFailuresMax {
		oldest := ""
		for k, v := range l.fails {
			if oldest == "" || v.At.Before(l.fails[oldest].At) {
				oldest = k
			}
		}
		delete(l.fails, oldest)
	}
	l.fails[f.Server] = f
}

// upstreamFailures returns the kept failures (a copy).
func (l *errLog) upstreamFailures() []UpstreamFailure {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]UpstreamFailure, 0, len(l.fails))
	for _, f := range l.fails {
		out = append(out, f)
	}
	return out
}

// field returns the text after key up to end.
func field(line, key, end string) (string, bool) {
	_, rest, ok := strings.Cut(line, key)
	if !ok {
		return "", false
	}
	v, _, _ := strings.Cut(rest, end)
	return v, true
}

// upstreamMessage reports whether the message part of an [error] line (before
// ", client: ") is nginx's own report of a failure on the way to an upstream.
func upstreamMessage(msg string) bool {
	m := strings.ToLower(msg)
	if strings.ContainsAny(m, `"'`) || strings.Contains(m, "lua") || strings.Contains(m, "to client") ||
		strings.Contains(m, "client prematurely closed") || strings.Contains(m, "client closed") {
		return false
	}
	i := strings.LastIndex(m, " while ")
	if i < 0 {
		return false
	}
	action := m[i+len(" while "):]
	if !strings.HasSuffix(action, "upstream") {
		return false
	}
	for _, r := range action {
		if r != ' ' && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

// parseUpstreamFailure recognises an [error] line about reaching an upstream:
//
//	<msg> while <action> upstream, client: ..., server: ..., request: "...", upstream: "...", host: "..."[, referrer: "..."]
//
// Request, host and referrer (and, in some messages, a user name) are the visitor's
// text, so only nginx's own parts decide: the message before ", client: " has to end in
// an upstream action ("while connecting to upstream", "while SSL handshaking to
// upstream", "while reading response header from upstream", ...) and contain no quoted
// or Lua text, and the upstream field is read only before the host and referrer fields.
// Rate limiting, denied requests, Lua errors and failures on the client side ("while
// sending to client", "client prematurely closed") are not upstream failures.
func parseUpstreamFailure(line string) (UpstreamFailure, bool) {
	var f UpstreamFailure
	_, rest, ok := strings.Cut(line, " [error] ")
	if !ok {
		return f, false
	}
	msg, fields, ok := strings.Cut(rest, ", client: ")
	if !ok {
		return f, false
	}
	if _, m, ok := strings.Cut(msg, ": "); ok { // "10#10: "
		msg = m
	}
	if strings.HasPrefix(msg, "*") { // connection number
		if _, m, ok := strings.Cut(msg, " "); ok {
			msg = m
		}
	}
	if !upstreamMessage(msg) {
		return f, false
	}
	if f.Server, ok = field(fields, ", server: ", ", "); !ok || f.Server == "" {
		return f, false
	}
	f.Server = strings.ToLower(f.Server)
	// the visitor's host and referrer come after the upstream field
	head := fields
	for _, k := range []string{`, host: "`, `, referrer: "`} {
		if i := strings.Index(head, k); i >= 0 {
			head = head[:i]
		}
	}
	i := strings.LastIndex(head, `, upstream: "`)
	if i < 0 {
		return f, false
	}
	up, _, _ := strings.Cut(head[i+len(`, upstream: "`):], `"`)
	if len(msg) > 300 {
		msg = msg[:300]
	}
	f.Message = msg
	// "http://10.0.0.5:8080/path" → "10.0.0.5:8080"; an upstream group name (no port) → ""
	if _, after, ok := strings.Cut(up, "://"); ok {
		up = after
	}
	up, _, _ = strings.Cut(up, "/")
	if _, port, err := net.SplitHostPort(up); err == nil && port != "" {
		f.Upstream = up
	}
	switch m := strings.ToLower(msg); {
	case strings.Contains(m, "no live upstreams"), strings.Contains(m, "could not be resolved"), strings.Contains(m, "no resolver"):
		f.Kind = conncheck.KindDNS
	case strings.Contains(m, "connection refused"):
		f.Kind = conncheck.KindRefused
	case strings.Contains(m, "timed out"):
		f.Kind = conncheck.KindTimeout
	case strings.Contains(m, "no route to host"), strings.Contains(m, "unreachable"):
		f.Kind = conncheck.KindUnreachable
	case strings.Contains(m, "ssl"):
		f.Kind = conncheck.KindTLS
	default:
		f.Kind = conncheck.KindOther
	}
	return f, true
}
