package traffic

import (
	"fmt"
	"testing"
	"time"
)

func TestCollectorAggregates(t *testing.T) {
	c := New()
	now := time.Now()
	base := float64(now.Unix() - 40)
	for i := 0; i < 100; i++ {
		st := 200
		switch {
		case i%10 == 0:
			st = 500
		case i%10 == 1:
			st = 429
		}
		line := fmt.Sprintf(`<190>Nov  3 03:12:42 corvus: {"t":%f,"h":"shop.example.com","m":"GET","u":"/p%d","s":%d,"b":100,"rt":0.%03d,"ip":"10.0.0.%d","wr":"-","x":"","rl":"","ua":"t"}`,
			base+float64(i%30), i%3, st, 5+i%20, i%7)
		c.Ingest([]byte(line))
	}
	c.Ingest([]byte(`<190>corvus: {"t":` + fmt.Sprint(base) + `,"h":"shop.example.com","m":"GET","u":"/.env","s":403,"b":0,"rt":0,"ip":"1.2.3.4","wr":"-","x":"dotfile","rl":"","ua":"scan"}`))
	c.Ingest([]byte("garbage"))

	s := c.Summary("5m", "", now)
	if s.Requests != 101 || s.S5xx != 10 || s.S4xx != 11 || s.Blocked != 11 || s.UniqueIPs != 8 {
		t.Fatalf("summary: %+v", s)
	}
	if s.P50 <= 0 || s.P95 < s.P50 || s.P99 < s.P95 {
		t.Fatalf("percentiles: %+v", s)
	}
	if hs := c.Hosts("5m", now); len(hs) != 1 || hs[0].Requests != 101 {
		t.Fatalf("hosts: %+v", hs)
	}
	if tp := c.TopPaths("5m", "shop.example.com", 3, now); len(tp) != 3 || tp[0].Requests < tp[2].Requests {
		t.Fatalf("top paths: %+v", tp)
	}
	step, pts := c.Timeseries("5m", "", now)
	var total int64
	for _, p := range pts {
		total += p.Requests
	}
	if step != 10 || total != 101 || len(pts) != 30 {
		t.Fatalf("timeseries step=%d points=%d total=%d", step, len(pts), total)
	}
	if logs := c.Logs(LogQuery{Status: "5xx"}); len(logs) != 10 {
		t.Fatalf("5xx logs: %d", len(logs))
	}
	if logs := c.Logs(LogQuery{Q: ".env", Limit: 5}); len(logs) != 1 || logs[0].Rule != "dotfile" {
		t.Fatalf("search: %+v", logs)
	}
	ev := c.Events("", 100)
	kinds := map[string]int64{}
	for _, e := range ev {
		kinds[e.Kind] += e.Count
	}
	if kinds["server_error"] != 10 || kinds["rate_limited"] != 10 || kinds["blocked_exploit"] != 1 {
		t.Fatalf("events: %v", kinds)
	}
	c.MaskIPs = func() bool { return true }
	if l := c.Logs(LogQuery{Limit: 1}); l[0].IP != "1.2.3.0" {
		t.Fatalf("masking: %s", l[0].IP)
	}
}

func TestLogRingNewestFirst(t *testing.T) {
	c := New()
	for i := 0; i < logCap+10; i++ {
		c.Add(&Record{T: float64(time.Now().Unix()), H: "h", U: fmt.Sprintf("/%d", i), S: 200, IP: "1.1.1.1"})
	}
	l := c.Logs(LogQuery{Limit: 2})
	if l[0].Path != fmt.Sprintf("/%d", logCap+9) || l[1].Path != fmt.Sprintf("/%d", logCap+8) {
		t.Fatalf("order: %s %s", l[0].Path, l[1].Path)
	}
}

func TestMultipleEventKinds(t *testing.T) {
	c := New()
	c.Add(&Record{T: float64(time.Now().Unix()), H: "h", U: "/.env", S: 200, IP: "1.1.1.1", X: "dotfile", RL: "REJECTED_DRY_RUN"})
	kinds := map[string]bool{}
	for _, e := range c.Events("", 10) {
		kinds[e.Kind] = true
	}
	if !kinds["exploit_detected"] || !kinds["rate_limit_dry_run"] {
		t.Fatalf("kinds: %v", kinds)
	}
}

// Challenge pages are events of their own: never "blocked", and their 403s do not
// count as 4xx for scanner detection.
func TestChallengeEvents(t *testing.T) {
	c := New()
	now := time.Now()
	ts := float64(now.Unix())
	c.Add(&Record{T: ts, H: "h", U: "/shop", S: 403, IP: "2.2.2.2", R: "7:challenge"})
	c.Add(&Record{T: ts, H: "h", U: "/__corvus/challenge/verify", S: 403, IP: "2.2.2.2", R: "7:challenge_fail"})
	c.Add(&Record{T: ts, H: "h", U: "/__corvus/challenge/verify", S: 303, IP: "2.2.2.2", R: "7:challenge_pass"})
	kinds := map[string]bool{}
	for _, e := range c.Events("", 10) {
		kinds[e.Kind] = true
	}
	if len(kinds) != 3 || !kinds["challenge_issued"] || !kinds["challenge_failed"] || !kinds["challenge_passed"] {
		t.Fatalf("kinds: %v", kinds)
	}
	if s := c.SummaryFor(time.Minute, "", now); s.Blocked != 0 || s.Requests != 3 {
		t.Errorf("summary: %+v", s)
	}
	if a := c.IPCounts(time.Minute, now)["2.2.2.2"]; a.Requests != 3 || a.S4xx != 0 {
		t.Errorf("IP activity: %+v", a)
	}
	ev := c.RuleEvents(7, 10)
	if len(ev) != 3 || ev[0].Action != "challenge_pass" {
		t.Errorf("rule events: %+v", ev)
	}
}

// Unique IPs are counted for the API (Summary), cached for uniqTTL, and skipped by
// SummaryFor, which the alert engine calls for every host and rule.
func TestUniqueIPsCached(t *testing.T) {
	c := New()
	now := time.Now()
	add := func(ip string) {
		c.Ingest([]byte(fmt.Sprintf(`<190>corvus: {"t":%d,"h":"a.example","m":"GET","u":"/","s":200,"b":0,"rt":0,"ip":"%s","wr":"-","x":"","rl":"","ua":"t"}`, now.Unix()-10, ip)))
	}
	add("10.0.0.1")
	add("10.0.0.2")
	if f := c.SummaryFor(5*time.Minute, "", now); f.Requests != 2 || f.UniqueIPs != 0 {
		t.Fatalf("SummaryFor: %+v", f)
	}
	if s := c.Summary("5m", "", now); s.UniqueIPs != 2 {
		t.Fatalf("unique IPs: %d", s.UniqueIPs)
	}
	add("10.0.0.3")
	if s := c.Summary("5m", "", now.Add(time.Second)); s.UniqueIPs != 2 {
		t.Fatalf("cached unique IPs: %d", s.UniqueIPs)
	}
	if s := c.Summary("5m", "", now.Add(uniqTTL)); s.UniqueIPs != 3 {
		t.Fatalf("recomputed unique IPs: %d", s.UniqueIPs)
	}
}
