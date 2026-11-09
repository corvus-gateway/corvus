// Package traffic receives structured access logs from the gateway over a Unix
// datagram socket (nginx syslog output) and keeps in-memory aggregates for the
// dashboard: time series, per-host and per-path stats, a searchable recent log
// buffer and notable events. Collection is lossy by design: if the collector is
// slow or down, the gateway drops log datagrams instead of slowing requests.
package traffic

import (
	"log/slog"
	"math"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Record is one access log entry as emitted by the gateway (log_format corvus_log, parse.go).
type Record struct {
	T  float64 `json:"t"`  // $msec
	H  string  `json:"h"`  // configured host name ($corvus_host)
	M  string  `json:"m"`  // method
	U  string  `json:"u"`  // $uri (no query string)
	S  int     `json:"s"`  // status
	B  int64   `json:"b"`  // body bytes sent
	RT float64 `json:"rt"` // request time (s)
	IP string  `json:"ip"` // client address (after real-IP processing)
	WR string  `json:"wr"` // access queue decision
	X  string  `json:"x"`  // exploit rule matched (detect-only unless the host blocks)
	RL string  `json:"rl"` // $limit_req_status
	UA string  `json:"ua"`
	R  string  `json:"r"`  // security rule match "<id>:<action>" ($corvus_rule; challenge, challenge_pass, challenge_fail for challenges)
	CC string  `json:"cc"` // client country used by rules ($corvus_cc)
	BD string  `json:"bd"` // body inspection "<signature>:<log|block>" ($corvus_body)
	BN string  `json:"bn"` // "1" when the client address is banned ($corvus_ban)
}

const (
	fineStep    = 10 // seconds
	fineSlots   = 360
	coarseStep  = 300
	coarseSlots = 288
	maxHosts    = 100
	logCap      = 5000
	eventCap    = 500
	pathMinutes = 60
	pathCap     = 2000
	ipCap       = 20000
	srcStep     = 10    // seconds
	srcSlots    = 360   // one hour
	srcIPCap    = 20000 // client IPs tracked per slot
)

type srcSlot struct {
	start int64
	ips   map[string]*srcAct
}

type srcAct struct {
	kinds map[string]int64
	hosts map[string]bool
}

// latency histogram upper bounds in milliseconds (last bucket = +Inf)
var latBounds = []float64{1, 2, 3, 5, 8, 12, 18, 27, 40, 60, 90, 135, 200, 300, 450, 700, 1000, 1500, 2200, 3300, 5000, 7500, 11000, 16000, 24000, 36000, 60000}

const nLat = 28

type bucket struct {
	start                                  int64
	count, s2, s3, s4, s5, blocked, queued int64
	bytes                                  int64
	banned                                 int64 // refused because the client is banned (part of blocked)
	hist                                   [nLat]uint32
}

func (b *bucket) add(r *Record, blocked bool) {
	b.count++
	switch {
	case r.S >= 500:
		b.s5++
	case r.S >= 400:
		b.s4++
	case r.S >= 300:
		b.s3++
	default:
		b.s2++
	}
	if blocked {
		b.blocked++
	}
	if r.BN != "" {
		b.banned++
	}
	if r.WR == "queue" {
		b.queued++
	}
	b.bytes += r.B
	ms := r.RT * 1000
	i := sort.SearchFloat64s(latBounds, ms)
	b.hist[i]++
}

func (b *bucket) merge(o *bucket) {
	b.count += o.count
	b.s2 += o.s2
	b.s3 += o.s3
	b.s4 += o.s4
	b.s5 += o.s5
	b.blocked += o.blocked
	b.banned += o.banned
	b.queued += o.queued
	b.bytes += o.bytes
	for i := range b.hist {
		b.hist[i] += o.hist[i]
	}
}

// quantile returns the upper bound (ms) of the bucket containing quantile q.
func (b *bucket) quantile(q float64) float64 {
	var total uint64
	for _, c := range b.hist {
		total += uint64(c)
	}
	if total == 0 {
		return 0
	}
	target := uint64(math.Ceil(q * float64(total)))
	var acc uint64
	for i, c := range b.hist {
		acc += uint64(c)
		if acc >= target {
			if i < len(latBounds) {
				return latBounds[i]
			}
			return 60000
		}
	}
	return 60000
}

type series struct {
	fine   [fineSlots]bucket
	coarse [coarseSlots]bucket
	last   int64 // unix time of the latest request
}

func (s *series) add(ts int64, r *Record, blocked bool) {
	if ts > s.last {
		s.last = ts
	}
	f := &s.fine[(ts/fineStep)%fineSlots]
	if start := ts - ts%fineStep; f.start != start {
		*f = bucket{start: start}
	}
	f.add(r, blocked)
	c := &s.coarse[(ts/coarseStep)%coarseSlots]
	if start := ts - ts%coarseStep; c.start != start {
		*c = bucket{start: start}
	}
	c.add(r, blocked)
}

type pathStat struct {
	requests, errors int64
	lat              bucket
}

type minute struct {
	start int64
	paths map[string]*pathStat // key: host + "\x00" + path
	ips   map[string]struct{}
	hips  map[string]map[string]struct{}
	ipst  map[string]*ipStat // per client IP (scanner detection)
}

type ipStat struct {
	n, s4 int64
	host  string
}

// LogLine is a recent request as returned by the API.
type LogLine struct {
	TS     float64 `json:"ts"`
	Host   string  `json:"host"`
	Method string  `json:"method"`
	Path   string  `json:"path"`
	Status int     `json:"status"`
	RTms   float64 `json:"rt_ms"`
	Bytes  int64   `json:"bytes"`
	IP     string  `json:"ip"`
	WR     string  `json:"wr,omitempty"`
	UA     string  `json:"ua,omitempty"`
	Rule   string  `json:"rule,omitempty"`
}

// Event is an aggregated notable occurrence (merged per kind/host/path/ip per minute).
type Event struct {
	TS    float64 `json:"ts"`
	Kind  string  `json:"kind"`
	Host  string  `json:"host"`
	Path  string  `json:"path"`
	IP    string  `json:"ip"`
	Rule  string  `json:"rule,omitempty"`
	Count int64   `json:"count"`
	seq   int64   // persistence watermark (internal/history)
}

type Collector struct {
	MaskIPs func() bool

	mu      sync.Mutex
	global  series
	hosts   map[string]*series
	minutes [pathMinutes]minute
	logs    []LogLine
	logPos  int
	events  []Event
	evIndex map[string]int // key → index in events for the current minute
	evMin   int64
	// event counts per client IP in 10-second slots, for automatic bans and alerts:
	// unlike the display ring above (eventCap entries), it keeps the full hour during a
	// wide attack
	src     [srcSlots]srcSlot
	started time.Time
	dropped int64

	// uniq caches unique-IP counts per host and window (see Summary): counting means
	// merging up to an hour of per-minute IP sets, too costly for every 4 s dashboard poll.
	uniq map[string]uniqCount

	ruleStats  map[int64]*ruleStat
	ruleEvents []RuleEvent
	rulePos    int

	counters map[string]*hostCounters // monotonic, for /metrics (counters.go)
	evSeq    int64
	ruleSeq  int64

	// Sink, when set, receives every record (called under the collector lock: it must
	// not block). notable = blocked, a security/queue event, or a 4xx/5xx response.
	Sink func(r *Record, notable bool)
}

func New() *Collector {
	return &Collector{hosts: map[string]*series{}, evIndex: map[string]int{}, started: time.Now()}
}

// Listen receives datagrams on a Unix socket until stop is closed.
func (c *Collector) Listen(path string, stop <-chan struct{}) error {
	_ = os.Remove(path)
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		return err
	}
	// Only the gateway may write access logs here (F13). Non-root image (H9): nginx
	// workers run as our own user, so owner-only. Legacy root mode: root + group nobody.
	if os.Getuid() != 0 {
		_ = os.Chmod(path, 0o600)
	} else if err := os.Chown(path, 0, 65534); err == nil {
		_ = os.Chmod(path, 0o660)
	}
	_ = conn.SetReadBuffer(4 << 20)
	go func() {
		<-stop
		conn.Close()
	}()
	go func() {
		buf := make([]byte, 64<<10)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				select {
				case <-stop:
					return
				default:
				}
				slog.Warn("traffic collector read", "err", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			c.Ingest(buf[:n])
		}
	}()
	return nil
}

// Ingest parses one datagram (see parseDatagram) and records its entries.
func (c *Collector) Ingest(msg []byte) {
	if bad := parseDatagram(msg, c.Add); bad > 0 {
		c.mu.Lock()
		c.dropped += int64(bad)
		c.mu.Unlock()
	}
}

// classify returns whether the gateway blocked the request and every notable
// event kind it represents (a request can, e.g., match an exploit rule and a
// dry-run rate limit at the same time).
func classify(r *Record) (blocked bool, kinds []string) {
	switch {
	case r.RL == "REJECTED" || (r.S == 429 && !waitroomHeld(r.WR)):
		blocked = true
		kinds = append(kinds, "rate_limited")
	case r.RL == "REJECTED_DRY_RUN":
		kinds = append(kinds, "rate_limit_dry_run")
	}
	if r.BN != "" {
		// refused before any other check: count it only as a ban hit, so banned
		// clients do not keep re-triggering detections and alerts
		return true, []string{"banned"}
	}
	if _, action, ok := parseBody(r.BD); ok && (action == "block" || action == "log") {
		if action == "block" {
			blocked = true
			kinds = append(kinds, "blocked_body")
		} else {
			kinds = append(kinds, "body_detected")
		}
	}
	if action, ok := ruleAction(r.R); ok {
		switch action {
		case "block":
			blocked = true
			kinds = append(kinds, "blocked_rule")
		case "log":
			kinds = append(kinds, "rule_logged")
		// Log-mode rule matches and challenges are neither blocks nor attacks: they never
		// count toward automatic bans or security alerts (see autoban.Kinds, alerts.securityKinds).
		case "challenge":
			kinds = append(kinds, "challenge_issued")
		case "challenge_pass":
			kinds = append(kinds, "challenge_passed")
		case "challenge_fail":
			kinds = append(kinds, "challenge_failed")
		}
	}
	if r.X != "" {
		if r.S == 403 {
			blocked = true
			kinds = append(kinds, "blocked_exploit")
		} else {
			kinds = append(kinds, "exploit_detected")
		}
	}
	switch {
	case strings.HasPrefix(r.WR, "fail-"):
		kinds = append(kinds, "waitroom_fail")
	case r.WR == "queue":
		kinds = append(kinds, "waitroom_queue")
	}
	if r.S >= 500 {
		kinds = append(kinds, "server_error")
	}
	return blocked, kinds
}

// waitroomHeld reports whether the access queue answered the request itself (cookie
// check, joined the line, still waiting, queue full; 429 + Retry-After): visitors told
// to wait, not rate limited.
func waitroomHeld(wr string) bool {
	return wr == "cookie-check" || wr == "queue" || wr == "wait" || wr == "full"
}

// isChallenge reports whether the gateway answered with a challenge page (issued, or
// re-issued after a failed answer) instead of the site's response.
func isChallenge(r *Record) bool {
	action, ok := ruleAction(r.R)
	return ok && (action == "challenge" || action == "challenge_fail")
}

func (c *Collector) Add(r *Record) {
	if r.T == 0 {
		r.T = float64(time.Now().UnixMilli()) / 1000
	}
	if r.H == "" {
		r.H = "_"
	}
	if len(r.U) > 200 {
		r.U = r.U[:200]
	}
	if len(r.UA) > 300 {
		r.UA = r.UA[:300]
	}
	ts := int64(r.T)
	blocked, kinds := classify(r)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Sink != nil {
		c.Sink(r, blocked || len(kinds) > 0 || r.S >= 400)
	}
	if r.R != "" {
		c.addRule(r)
	}
	if r.BD != "" {
		c.addBody(r)
	}
	c.global.add(ts, r, blocked)
	hs := c.hosts[r.H]
	if hs == nil {
		if len(c.hosts) >= maxHosts && !c.evictIdleHost(ts) {
			r.H = "(other)"
			hs = c.hosts[r.H]
		}
		if hs == nil {
			hs = &series{}
			c.hosts[r.H] = hs
		}
	}
	hs.add(ts, r, blocked)
	c.count(r, kinds)

	// per-minute paths & unique IPs
	m := &c.minutes[(ts/60)%pathMinutes]
	if start := ts - ts%60; m.start != start {
		*m = minute{start: start, paths: map[string]*pathStat{}, ips: map[string]struct{}{}, hips: map[string]map[string]struct{}{}, ipst: map[string]*ipStat{}}
	}
	key := r.H + "\x00" + r.U
	ps := m.paths[key]
	if ps == nil {
		if len(m.paths) >= pathCap {
			key = r.H + "\x00(other)"
			ps = m.paths[key]
		}
		if ps == nil {
			ps = &pathStat{}
			m.paths[key] = ps
		}
	}
	ps.requests++
	if r.S >= 400 {
		ps.errors++
	}
	ps.lat.add(r, false)
	if len(m.ips) < ipCap {
		m.ips[r.IP] = struct{}{}
		hm := m.hips[r.H]
		if hm == nil {
			hm = map[string]struct{}{}
			m.hips[r.H] = hm
		}
		hm[r.IP] = struct{}{}
		st := m.ipst[r.IP]
		if st == nil && len(m.ipst) < ipCap {
			st = &ipStat{}
			m.ipst[r.IP] = st
		}
		if st != nil {
			st.n++
			if r.S >= 400 && r.S < 500 && !isChallenge(r) {
				st.s4++ // scanner detection: challenge pages are not counted
			}
			st.host = r.H
		}
	}

	// recent log ring
	line := LogLine{TS: r.T, Host: r.H, Method: r.M, Path: r.U, Status: r.S, RTms: math.Round(r.RT*10000) / 10, Bytes: r.B, IP: r.IP, WR: r.WR, UA: r.UA, Rule: r.X}
	if r.WR == "-" {
		line.WR = ""
	}
	if len(c.logs) < logCap {
		c.logs = append(c.logs, line)
	} else {
		c.logs[c.logPos] = line
		c.logPos = (c.logPos + 1) % logCap
	}

	// events, merged per minute
	for _, kind := range kinds {
		min := ts / 60
		if min != c.evMin {
			c.evMin = min
			c.evIndex = map[string]int{}
		}
		ek := kind + "\x00" + r.H + "\x00" + r.U + "\x00" + r.IP
		if idx, ok := c.evIndex[ek]; ok && idx < len(c.events) {
			c.events[idx].Count++
			c.events[idx].TS = r.T
		} else {
			if len(c.events) >= eventCap {
				c.events = append(c.events[:0:0], c.events[eventCap/5:]...) // drop oldest 20%
				c.evIndex = map[string]int{}
			}
			c.evSeq++
			c.events = append(c.events, Event{TS: r.T, Kind: kind, Host: r.H, Path: r.U, IP: r.IP, Rule: r.X, Count: 1, seq: c.evSeq})
			c.evIndex[ek] = len(c.events) - 1
		}
		c.countSource(ts, kind, r.H, r.IP, 1)
	}
}

// ------------------------------------------------------------------ queries

// Range parses 5m|15m|1h|6h|24h (default 15m).
func Range(s string) time.Duration {
	switch s {
	case "5m":
		return 5 * time.Minute
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "24h":
		return 24 * time.Hour
	default:
		return 15 * time.Minute
	}
}

func (c *Collector) seriesFor(host string) *series {
	if host == "" {
		return &c.global
	}
	return c.hosts[host]
}

// buckets returns the buckets covering [now-d, now] at the appropriate resolution.
func (s *series) buckets(now int64, d time.Duration) (step int64, out []*bucket) {
	secs := int64(d.Seconds())
	if secs <= fineStep*fineSlots {
		step = fineStep
		first := now - secs
		for i := 0; i < fineSlots; i++ {
			b := &s.fine[i]
			if b.count > 0 && b.start > first-step && b.start <= now {
				out = append(out, b)
			}
		}
	} else {
		step = coarseStep
		first := now - secs
		for i := 0; i < coarseSlots; i++ {
			b := &s.coarse[i]
			if b.count > 0 && b.start > first-step && b.start <= now {
				out = append(out, b)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].start < out[j].start })
	return step, out
}

type Summary struct {
	Range     string  `json:"range"`
	Requests  int64   `json:"requests"`
	RPS       float64 `json:"rps"`     // current: last complete 10 s window
	AvgRPS    float64 `json:"avg_rps"` // average over the range
	S2xx      int64   `json:"s2xx"`
	S3xx      int64   `json:"s3xx"`
	S4xx      int64   `json:"s4xx"`
	S5xx      int64   `json:"s5xx"`
	ErrorRate float64 `json:"error_rate"` // 5xx share (0..1)
	P50       float64 `json:"p50"`
	P95       float64 `json:"p95"`
	P99       float64 `json:"p99"`
	BytesOut  int64   `json:"bytes_out"`
	Blocked   int64   `json:"blocked"`
	Queued    int64   `json:"queued"`
	UniqueIPs int     `json:"unique_ips"` // over min(range, 1h)
}

type uniqCount struct {
	at time.Time
	n  int
}

// uniqTTL is how long a unique-IP count is reused.
const uniqTTL = 5 * time.Second

// Summary is SummaryFor plus the number of unique client IPs (over min(range, 1h)),
// for the API. The count is cached for uniqTTL.
func (c *Collector) Summary(rangeName, host string, now time.Time) Summary {
	d := Range(rangeName)
	out := c.SummaryFor(d, host, now)
	c.mu.Lock()
	key := host + "|" + d.String()
	if u, ok := c.uniq[key]; ok && now.Sub(u.at) >= 0 && now.Sub(u.at) < uniqTTL {
		out.UniqueIPs = u.n
	} else {
		out.UniqueIPs = c.uniqueIPs(host, now.Unix(), d)
		if c.uniq == nil || len(c.uniq) > 1000 {
			c.uniq = map[string]uniqCount{}
		}
		c.uniq[key] = uniqCount{at: now, n: out.UniqueIPs}
	}
	c.mu.Unlock()
	out.Range = rangeName
	if out.Range == "" {
		out.Range = "15m"
	}
	return out
}

// SummaryFor aggregates an arbitrary window (rounded to the bucket size). It leaves
// UniqueIPs at 0 (see Summary): the alert engine calls it for every host and rule.
func (c *Collector) SummaryFor(d time.Duration, host string, now time.Time) Summary {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := Summary{}
	s := c.seriesFor(host)
	if s == nil {
		return out
	}
	ts := now.Unix()
	_, bs := s.buckets(ts, d)
	var agg bucket
	for _, b := range bs {
		agg.merge(b)
	}
	out.Requests, out.S2xx, out.S3xx, out.S4xx, out.S5xx = agg.count, agg.s2, agg.s3, agg.s4, agg.s5
	out.BytesOut, out.Blocked, out.Queued = agg.bytes, agg.blocked, agg.queued
	out.P50, out.P95, out.P99 = agg.quantile(.5), agg.quantile(.95), agg.quantile(.99)
	if agg.count > 0 {
		out.ErrorRate = float64(agg.s5) / float64(agg.count)
	}
	out.AvgRPS = round2(float64(agg.count) / d.Seconds())
	last := ts - ts%fineStep - fineStep
	if b := &s.fine[(last/fineStep)%fineSlots]; b.start == last {
		out.RPS = round2(float64(b.count) / fineStep)
	}
	return out
}

func (c *Collector) uniqueIPs(host string, now int64, d time.Duration) int {
	if d > time.Hour {
		d = time.Hour
	}
	first := now - int64(d.Seconds())
	set := map[string]struct{}{}
	for i := range c.minutes {
		m := &c.minutes[i]
		if m.start == 0 || m.start <= first-60 || m.start > now {
			continue
		}
		src := m.ips
		if host != "" {
			src = m.hips[host]
		}
		for ip := range src {
			set[ip] = struct{}{}
		}
	}
	return len(set)
}

type Point struct {
	T        int64   `json:"t"`
	Requests int64   `json:"requests"`
	S2xx     int64   `json:"s2xx"`
	S3xx     int64   `json:"s3xx"`
	S4xx     int64   `json:"s4xx"`
	S5xx     int64   `json:"s5xx"`
	Blocked  int64   `json:"blocked"`
	Queued   int64   `json:"queued"`
	P50      float64 `json:"p50"`
	P95      float64 `json:"p95"`
	P99      float64 `json:"p99"`
	BytesOut int64   `json:"bytes_out"`
}

// Timeseries returns a gap-filled series (zero points for idle intervals).
func (c *Collector) Timeseries(rangeName, host string, now time.Time) (int64, []Point) {
	d := Range(rangeName)
	c.mu.Lock()
	defer c.mu.Unlock()
	ts := now.Unix()
	step := int64(fineStep)
	if int64(d.Seconds()) > fineStep*fineSlots {
		step = coarseStep
	}
	byStart := map[int64]*bucket{}
	if s := c.seriesFor(host); s != nil {
		_, bs := s.buckets(ts, d)
		for _, b := range bs {
			byStart[b.start] = b
		}
	}
	end := ts - ts%step
	first := end - int64(d.Seconds()) + step
	pts := make([]Point, 0, (end-first)/step+1)
	for t := first; t <= end; t += step {
		p := Point{T: t}
		if b := byStart[t]; b != nil {
			p = Point{T: t, Requests: b.count, S2xx: b.s2, S3xx: b.s3, S4xx: b.s4, S5xx: b.s5, Blocked: b.blocked, Queued: b.queued,
				P50: b.quantile(.5), P95: b.quantile(.95), P99: b.quantile(.99), BytesOut: b.bytes}
		}
		pts = append(pts, p)
	}
	return step, pts
}

type HostStat struct {
	Host      string  `json:"host"`
	Requests  int64   `json:"requests"`
	RPS       float64 `json:"rps"`
	S4xx      int64   `json:"s4xx"`
	S5xx      int64   `json:"s5xx"`
	ErrorRate float64 `json:"error_rate"`
	P95       float64 `json:"p95"`
	BytesOut  int64   `json:"bytes_out"`
	Blocked   int64   `json:"blocked"`
}

func (c *Collector) Hosts(rangeName string, now time.Time) []HostStat {
	d := Range(rangeName)
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []HostStat{}
	for name, s := range c.hosts {
		_, bs := s.buckets(now.Unix(), d)
		var agg bucket
		for _, b := range bs {
			agg.merge(b)
		}
		if agg.count == 0 {
			continue
		}
		out = append(out, HostStat{Host: name, Requests: agg.count, RPS: round2(float64(agg.count) / d.Seconds()), S4xx: agg.s4, S5xx: agg.s5,
			ErrorRate: float64(agg.s5) / float64(agg.count), P95: agg.quantile(.95), BytesOut: agg.bytes, Blocked: agg.blocked})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Requests > out[j].Requests })
	return out
}

type PathStat struct {
	Host     string  `json:"host"`
	Path     string  `json:"path"`
	Requests int64   `json:"requests"`
	Errors   int64   `json:"errors"`
	P95      float64 `json:"p95"`
}

// TopPaths aggregates per-minute path stats over min(range, 1h).
func (c *Collector) TopPaths(rangeName, host string, limit int, now time.Time) []PathStat {
	d := Range(rangeName)
	if d > time.Hour {
		d = time.Hour
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ts := now.Unix()
	first := ts - int64(d.Seconds())
	agg := map[string]*pathStat{}
	for i := range c.minutes {
		m := &c.minutes[i]
		if m.start == 0 || m.start <= first-60 || m.start > ts {
			continue
		}
		for k, ps := range m.paths {
			if host != "" && !strings.HasPrefix(k, host+"\x00") {
				continue
			}
			a := agg[k]
			if a == nil {
				a = &pathStat{}
				agg[k] = a
			}
			a.requests += ps.requests
			a.errors += ps.errors
			a.lat.merge(&ps.lat)
		}
	}
	out := make([]PathStat, 0, len(agg))
	for k, a := range agg {
		h, p, _ := strings.Cut(k, "\x00")
		out = append(out, PathStat{Host: h, Path: p, Requests: a.requests, Errors: a.errors, P95: a.lat.quantile(.95)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Requests > out[j].Requests })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// LogQuery filters the recent log buffer (newest first).
type LogQuery struct {
	Host   string
	Status string // "2xx".."5xx" or an exact code
	Q      string // substring of path or IP
	Limit  int
}

func (c *Collector) Logs(q LogQuery) []LogLine {
	if q.Limit <= 0 || q.Limit > 1000 {
		q.Limit = 200
	}
	mask := c.MaskIPs != nil && c.MaskIPs()
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]LogLine, 0, q.Limit)
	n := len(c.logs)
	for i := 0; i < n && len(out) < q.Limit; i++ {
		// newest first: walk backwards from the write position
		idx := (c.logPos - 1 - i + n) % n
		if n < logCap {
			idx = n - 1 - i
		}
		l := c.logs[idx]
		if q.Host != "" && l.Host != q.Host {
			continue
		}
		if q.Status != "" && !statusMatch(l.Status, q.Status) {
			continue
		}
		if q.Q != "" && !strings.Contains(l.Path, q.Q) && !strings.Contains(l.IP, q.Q) {
			continue
		}
		if mask {
			l.IP = MaskIP(l.IP)
		}
		out = append(out, l)
	}
	return out
}

func statusMatch(code int, f string) bool {
	if len(f) == 3 && strings.HasSuffix(f, "xx") {
		return code/100 == int(f[0]-'0')
	}
	n, err := strconv.Atoi(f)
	return err == nil && n == code
}

// Events returns the newest events first, optionally filtered by kind.
func (c *Collector) Events(kind string, limit int) []Event {
	if limit <= 0 || limit > eventCap {
		limit = 100
	}
	mask := c.MaskIPs != nil && c.MaskIPs()
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []Event{}
	for i := len(c.events) - 1; i >= 0 && len(out) < limit; i-- {
		e := c.events[i]
		if kind != "" && e.Kind != kind {
			continue
		}
		if mask {
			e.IP = MaskIP(e.IP)
		}
		out = append(out, e)
	}
	return out
}

// MaskIP hides the host part: IPv4 → a.b.c.0, IPv6 → /48.
func MaskIP(s string) string {
	ip := net.ParseIP(s)
	if ip == nil {
		return s
	}
	if v4 := ip.To4(); v4 != nil {
		return net.IPv4(v4[0], v4[1], v4[2], 0).String()
	}
	return ip.Mask(net.CIDRMask(48, 128)).String() + "/48"
}

// Uptime of the collector (≈ how much history exists).
func (c *Collector) Since() time.Time { return c.started }

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// HostNames lists hosts with recorded traffic.
func (c *Collector) HostNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.hosts))
	for h := range c.hosts {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// EventCounts sums event counts per host for the given kinds since t.
func (c *Collector) EventCounts(since time.Time, kinds ...string) map[string]int64 {
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]int64{}
	ts := float64(since.UnixMilli()) / 1000
	for _, e := range c.events {
		if want[e.Kind] && e.TS >= ts {
			out[e.Host] += e.Count
		}
	}
	return out
}

// IPActivity is one client's recent request profile.
type IPActivity struct {
	Requests, S4xx int64
	Host           string // most recent host
}

// IPCounts returns per-IP request and 4xx counts over the last d (minute resolution).
func (c *Collector) IPCounts(d time.Duration, now time.Time) map[string]IPActivity {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]IPActivity{}
	cut := now.Unix() - int64(d.Seconds())
	for i := range c.minutes {
		m := &c.minutes[i]
		if m.ipst == nil || m.start+60 <= cut || m.start > now.Unix() {
			continue
		}
		for ip, st := range m.ipst {
			a := out[ip]
			a.Requests += st.n
			a.S4xx += st.s4
			a.Host = st.host
			out[ip] = a
		}
	}
	return out
}

// SourceActivity aggregates security events per client IP.
type SourceActivity struct {
	Count int64
	Hosts map[string]bool
	Kinds map[string]bool
}

// EventsByIP sums event counts per client IP for the given kinds since t.
func (c *Collector) EventsByIP(since time.Time, kinds ...string) map[string]*SourceActivity {
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]*SourceActivity{}
	// 10-second slots that start at or after since: the partly covered first slot is
	// left out (an undercount of at most 10 s, never counting events from before since)
	from := float64(since.UnixMilli()) / 1000
	for i := range c.src {
		b := &c.src[i]
		if b.ips == nil || float64(b.start) < from {
			continue
		}
		for ip, s := range b.ips {
			for k, n := range s.kinds {
				if !want[k] {
					continue
				}
				a := out[ip]
				if a == nil {
					a = &SourceActivity{Hosts: map[string]bool{}, Kinds: map[string]bool{}}
					out[ip] = a
				}
				a.Count += n
				a.Kinds[k] = true
				for h := range s.hosts {
					a.Hosts[h] = true
				}
			}
		}
	}
	return out
}

// Window sums the requests and blocked requests (blocks and rate limits) of host over
// the complete 10-second buckets of the last d (rounded up to 10 s, at most 1 h); the
// bucket still being filled is left out. secs is the time span covered.
func (c *Collector) Window(host string, d time.Duration, now time.Time) (requests, blocked int64, secs int64) {
	n := (int64(d.Seconds()) + fineStep - 1) / fineStep
	if n < 1 {
		n = 1
	}
	if n > fineSlots-1 {
		n = fineSlots - 1
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.hosts[host]
	if s == nil {
		return 0, 0, n * fineStep
	}
	ts := now.Unix()
	end := ts - ts%fineStep // start of the current, incomplete bucket
	for start := end - n*fineStep; start < end; start += fineStep {
		if b := &s.fine[(start/fineStep)%fineSlots]; b.start == start {
			// banned clients are already refused at no cost: they must not keep a host
			// in "under attack" mode for everyone else
			requests += b.count - b.banned
			blocked += b.blocked - b.banned
		}
	}
	return requests, blocked, n * fineStep
}

// evictIdleHost makes room for a new host name (hosts renamed or removed keep their
// series otherwise, and later hosts would only be counted as "(other)", invisible to
// "under attack" auto mode): it drops the series idle for the longest time, if that is
// over an hour. Caller holds c.mu.
func (c *Collector) evictIdleHost(now int64) bool {
	oldest, at := "", now
	for h, s := range c.hosts {
		if h != "(other)" && s.last < at {
			oldest, at = h, s.last
		}
	}
	if oldest == "" || now-at < 3600 {
		return false
	}
	delete(c.hosts, oldest)
	return true
}

// countSource adds n events of kind from ip at unix time ts to the per-IP slots (caller
// holds c.mu). Slots older than an hour are overwritten.
func (c *Collector) countSource(ts int64, kind, host, ip string, n int64) {
	if ip == "" || n <= 0 {
		return
	}
	start := ts - ts%srcStep
	b := &c.src[(start/srcStep)%srcSlots]
	if b.start != start || b.ips == nil {
		if b.start > start && b.ips != nil {
			return // a newer slot already uses this place (restored event older than an hour)
		}
		b.start, b.ips = start, map[string]*srcAct{}
	}
	a := b.ips[ip]
	if a == nil {
		if len(b.ips) >= srcIPCap {
			return
		}
		a = &srcAct{kinds: map[string]int64{}, hosts: map[string]bool{}}
		b.ips[ip] = a
	}
	a.kinds[kind] += n
	a.hosts[host] = true
}
