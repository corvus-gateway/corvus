package traffic

import (
	"net"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"
)

func TestCovCountersAndLatency(t *testing.T) {
	c := New()
	now := time.Now()
	ts := float64(now.Unix() - 5)
	add := func(h string, s int, x, rl, r, bd string, rt float64) {
		c.Add(&Record{T: ts, H: h, M: "GET", U: "/", S: s, RT: rt, IP: "10.0.0.1", X: x, RL: rl, R: r, BD: bd})
	}
	add("b.example", 200, "", "", "", "", 0.010)
	add("b.example", 301, "", "", "", "", 0.020)
	add("b.example", 404, "", "", "", "", 0.030)
	add("b.example", 503, "", "", "", "", 0.040)
	add("a.example", 403, "dotfile", "", "", "", 0.001)
	add("a.example", 429, "", "REJECTED", "", "", 0.001)
	add("a.example", 403, "", "", "7:block", "", 0.001)
	add("a.example", 403, "", "", "", "sqli:block", 0.001)

	cs := c.Counters()
	if len(cs) != 2 || cs[0].Host != "a.example" || cs[1].Host != "b.example" {
		t.Fatalf("counters hosts: %+v", cs)
	}
	b := cs[1]
	for _, cl := range []string{"2xx", "3xx", "4xx", "5xx"} {
		if b.Class[cl] != 1 {
			t.Errorf("b %s = %d", cl, b.Class[cl])
		}
	}
	a := cs[0]
	if a.Class["4xx"] != 4 {
		t.Errorf("a 4xx = %d", a.Class["4xx"])
	}
	for _, k := range BlockKinds {
		if a.Blocked[k] != 1 {
			t.Errorf("a blocked[%s] = %d", k, a.Blocked[k])
		}
		if b.Blocked[k] != 0 {
			t.Errorf("b blocked[%s] = %d", k, b.Blocked[k])
		}
	}

	lat := c.Latency(time.Minute, now)
	if len(lat) != 2 || lat[1].Host != "b.example" || lat[1].Count != 4 {
		t.Fatalf("latency: %+v", lat)
	}
	if lat[1].P50 <= 0 || lat[1].P99 < lat[1].P50 || lat[1].P99 > 0.1 {
		t.Fatalf("latency quantiles (seconds): %+v", lat[1])
	}
	if l := c.Latency(time.Minute, now.Add(2*time.Hour)); len(l) != 0 {
		t.Fatalf("no requests in a later window: %+v", l)
	}

	// body inspection matches become rule events "body:<sig>"
	evs := c.RuleEvents(0, 10)
	if len(evs) != 2 || evs[0].Rule != "body:sqli" || evs[0].Action != "block" || evs[1].RuleID != 7 {
		t.Fatalf("rule events: %+v", evs)
	}
	hits := c.RuleHits(now)
	if len(hits) != 1 || hits[7].Hits24h != 1 || hits[7].Last == nil {
		t.Fatalf("rule hits: %+v", hits)
	}
	if h := c.RuleHits(now.Add(48 * time.Hour)); h[7].Hits24h != 0 || h[7].Last == nil {
		t.Fatalf("hits expire after 24h but the last match stays: %+v", h[7])
	}

	if hn := c.HostNames(); len(hn) != 2 || hn[0] != "a.example" {
		t.Fatalf("host names: %v", hn)
	}
	if s := c.Since(); s.After(time.Now()) || time.Since(s) > time.Minute {
		t.Fatalf("since: %v", s)
	}
	ec := c.EventCounts(now.Add(-time.Minute), "blocked_exploit", "rate_limited")
	if ec["a.example"] != 2 || len(ec) != 1 {
		t.Fatalf("event counts: %v", ec)
	}
	if ec := c.EventCounts(now.Add(time.Minute), "blocked_exploit"); len(ec) != 0 {
		t.Fatalf("future cut-off: %v", ec)
	}
}

func TestCovAddBodyIgnoresMalformed(t *testing.T) {
	c := New()
	for _, bd := range []string{"nocolon", ":block", "sig:allow"} {
		c.Add(&Record{H: "h", S: 200, IP: "1.1.1.1", BD: bd})
	}
	c.Add(&Record{H: "h", S: 200, IP: "1.1.1.1", BD: "xss:log"})
	evs := c.RuleEvents(0, 10)
	if len(evs) != 1 || evs[0].Rule != "body:xss" || evs[0].Action != "log" {
		t.Fatalf("body events: %+v", evs)
	}
	if sig, act, ok := parseBody("a:b:c"); !ok || sig != "a:b" || act != "c" {
		t.Fatalf("parseBody keeps colons in the signature: %q %q %v", sig, act, ok)
	}
}

func TestCovPersistCoarseRoundTrip(t *testing.T) {
	c := New()
	now := time.Now()
	// two completed 5-minute buckets and one current one
	old := now.Unix() - now.Unix()%CoarseStep - 2*CoarseStep
	for i, h := range []string{"a", "a", "b"} {
		c.Add(&Record{T: float64(old + int64(i)*CoarseStep + 1), H: h, S: 200 + 100*i, B: 10, RT: 0.01, IP: "1.1.1.1"})
	}
	c.Add(&Record{T: float64(now.Unix()), H: "a", S: 200, IP: "1.1.1.1"})

	rows := c.CompletedCoarse(0, now)
	byKey := map[string]CoarseRow{}
	for _, r := range rows {
		byKey[r.Host+"@"+time.Unix(r.Start, 0).UTC().Format(time.TimeOnly)] = r
		if r.Start+CoarseStep > now.Unix() {
			t.Fatalf("incomplete bucket exported: %+v", r)
		}
	}
	// the third record (host b) and the fourth fall into the still-open bucket
	var global, hostA int
	for _, r := range rows {
		switch r.Host {
		case "":
			global++
		case "a":
			hostA++
		}
	}
	if global != 2 || hostA != 2 {
		t.Fatalf("completed buckets: global=%d a=%d rows=%+v", global, hostA, rows)
	}
	if again := c.CompletedCoarse(old, now); len(again) != 2 {
		t.Fatalf("after watermark: %+v", again)
	}

	// restore into a fresh collector, plus rows that are too old, in the future, and a new host
	d := New()
	extra := []CoarseRow{
		{Host: "", Start: now.Unix() - 2*86400, Count: 99},
		{Host: "", Start: now.Unix() + 3600, Count: 99},
		{Host: "c", Start: old, Count: 3, S5: 3},
	}
	d.RestoreCoarse(append(rows, extra...), now)
	got := d.CompletedCoarse(0, now)
	var total int64
	hosts := map[string]bool{}
	for _, r := range got {
		if r.Host == "" {
			total += r.Count
		}
		hosts[r.Host] = true
	}
	if total != 2 {
		t.Fatalf("restored global requests = %d (rows %+v)", total, got)
	}
	if !hosts["a"] || !hosts["c"] {
		t.Fatalf("restored hosts: %v", hosts)
	}
	// restoring the same rows again merges into the existing buckets
	d.RestoreCoarse(rows, now)
	total = 0
	for _, r := range d.CompletedCoarse(0, now) {
		if r.Host == "" {
			total += r.Count
		}
	}
	if total != 4 {
		t.Fatalf("merged global requests = %d", total)
	}
}

func TestCovRestoreCoarseHostCap(t *testing.T) {
	c := New()
	now := time.Now()
	start := now.Unix() - now.Unix()%CoarseStep - CoarseStep
	var rows []CoarseRow
	for i := 0; i < maxHosts+5; i++ {
		rows = append(rows, CoarseRow{Host: "h" + string(rune('A'+i%26)) + string(rune('a'+i/26)), Start: start, Count: 1})
	}
	c.RestoreCoarse(rows, now)
	if n := len(c.HostNames()); n != maxHosts {
		t.Fatalf("hosts after restore = %d, want cap %d", n, maxHosts)
	}
}

func TestCovPersistEvents(t *testing.T) {
	c := New()
	now := time.Now()
	past := float64(now.Unix() - 600)
	c.Add(&Record{T: past, H: "h", U: "/.env", S: 403, IP: "1.2.3.4", X: "dotfile"})
	c.Add(&Record{T: past, H: "h", U: "/.env", S: 403, IP: "1.2.3.4", X: "dotfile"}) // merged
	c.Add(&Record{T: float64(now.Unix()), H: "h", U: "/x", S: 500, IP: "1.2.3.5"})   // current minute: not final

	evs, seq := c.EventsAfter(0, now)
	if len(evs) != 1 || evs[0].Count != 2 || evs[0].Kind != "blocked_exploit" || seq == 0 {
		t.Fatalf("EventsAfter: %+v seq=%d", evs, seq)
	}
	if more, s2 := c.EventsAfter(seq, now); len(more) != 0 || s2 != seq {
		t.Fatalf("nothing new after the watermark: %+v %d", more, s2)
	}
	if later, _ := c.EventsAfter(seq, now.Add(2*time.Minute)); len(later) != 1 || later[0].Kind != "server_error" {
		t.Fatalf("once the minute is over: %+v", later)
	}

	d := New()
	w := d.RestoreEvents(evs)
	if w != 1 {
		t.Fatalf("watermark = %d", w)
	}
	if got := d.Events("", 10); len(got) != 1 || got[0].Count != 2 || got[0].Path != "/.env" {
		t.Fatalf("restored events: %+v", got)
	}
	if by := d.EventsByIP(now.Add(-time.Hour), "blocked_exploit"); by["1.2.3.4"] == nil || by["1.2.3.4"].Count != 2 {
		t.Fatalf("restored source counts: %+v", by)
	}
	// restoring more than the cap keeps the newest events
	var many []Event
	for i := 0; i < eventCap+10; i++ {
		many = append(many, Event{TS: past, Kind: "server_error", Host: "h", IP: "9.9.9.9", Count: 1, Path: "/" + string(rune('a'+i%26))})
	}
	e := New()
	if w := e.RestoreEvents(many); w != int64(eventCap+10) {
		t.Fatalf("watermark = %d", w)
	}
	if n := len(e.Events("", eventCap)); n > eventCap || n < eventCap*4/5 {
		t.Fatalf("events kept = %d", n)
	}
}

func TestCovPersistRuleEvents(t *testing.T) {
	c := New()
	now := time.Now()
	c.Add(&Record{T: float64(now.Unix() - 30), H: "h", S: 403, IP: "1.1.1.1", R: "3:block"})
	c.Add(&Record{T: float64(now.Unix() - 20), H: "h", S: 200, IP: "1.1.1.1", BD: "x:log"})
	evs, seq := c.RuleEventsAfter(0)
	if len(evs) != 2 || seq != 2 {
		t.Fatalf("RuleEventsAfter: %+v %d", evs, seq)
	}
	if evs2, s := c.RuleEventsAfter(1); len(evs2) != 1 || s != 2 {
		t.Fatalf("after 1: %+v %d", evs2, s)
	}

	d := New()
	if w := d.RestoreRuleEvents(evs); w != 2 {
		t.Fatalf("watermark %d", w)
	}
	hits := d.RuleHits(now)
	if hits[3].Hits24h != 1 || hits[3].Last == nil || len(hits) != 1 {
		t.Fatalf("restored hits: %+v", hits)
	}
	if got := d.RuleEvents(0, 10); len(got) != 2 {
		t.Fatalf("restored events: %+v", got)
	}
	// overflow the ring: the oldest entries are overwritten
	var many []RuleEvent
	for i := 0; i < ruleEventCap+3; i++ {
		many = append(many, RuleEvent{TS: float64(now.Unix() - 10), RuleID: 5, Action: "log"})
	}
	e := New()
	e.RestoreRuleEvents(many)
	if got := e.RuleEvents(5, ruleEventCap); len(got) != ruleEventCap {
		t.Fatalf("ring size = %d", len(got))
	}
	if h := e.RuleHits(now); h[5].Hits24h != int64(ruleEventCap+3) {
		t.Fatalf("hit counter counts every restored event: %d", h[5].Hits24h)
	}
}

func TestCovListen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unixgram")
	}
	c := New()
	path := filepath.Join(t.TempDir(), "t.sock")
	stop := make(chan struct{})
	if err := c.Listen(path, stop); err != nil {
		t.Fatal(err)
	}
	defer close(stop)
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	msg := `<190>corvus: {"t":` + itoa(time.Now().Unix()) + `,"h":"sock.example","m":"GET","u":"/","s":200,"ip":"1.1.1.1"}`
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("garbage")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		names := c.HostNames()
		sort.Strings(names)
		if len(names) == 1 && names[0] == "sock.example" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("datagram not ingested: %v", names)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// a path in a missing directory cannot be bound
	if err := New().Listen(filepath.Join(t.TempDir(), "no", "such", "dir.sock"), stop); err == nil {
		t.Fatal("listen in a missing directory must fail")
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
