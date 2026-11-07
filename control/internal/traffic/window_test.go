package traffic

import (
	"testing"
	"time"
)

// Window counts only complete 10-second buckets of the requested span.
func TestWindow(t *testing.T) {
	c := New()
	now := time.Unix(1_000_005, 0) // 5 s into the bucket starting at 1_000_000
	add := func(ts int64, n int, status int) {
		for i := 0; i < n; i++ {
			c.Add(&Record{T: float64(ts), H: "a.example.com", S: status, IP: "10.0.0.1"})
		}
	}
	add(999_960, 7, 200)    // outside the 30 s window
	add(999_970, 10, 200)   // first bucket of the window
	add(999_985, 5, 429)    // rate limited = blocked
	add(999_999, 15, 200)   // last complete bucket
	add(1_000_002, 99, 200) // current bucket: not counted yet
	req, blocked, secs := c.Window("a.example.com", 30*time.Second, now)
	if req != 30 || blocked != 5 || secs != 30 {
		t.Fatalf("window: %d requests, %d blocked over %d s", req, blocked, secs)
	}
	if req, _, secs := c.Window("a.example.com", 25*time.Second, now); req != 30 || secs != 30 {
		t.Fatalf("rounded window: %d over %d s", req, secs)
	}
	if req, _, _ := c.Window("other.example.com", 30*time.Second, now); req != 0 {
		t.Fatalf("unknown host: %d", req)
	}
}

// Under-attack challenges count as challenge events, not as rule matches or blocks.
func TestUnderAttackRuleEvents(t *testing.T) {
	for v, want := range map[string]string{"ua:challenge": "challenge_issued", "ua:challenge_pass": "challenge_passed", "ua:challenge_fail": "challenge_failed"} {
		blocked, kinds := classify(&Record{S: 403, R: v})
		if blocked || len(kinds) != 1 || kinds[0] != want {
			t.Errorf("%s: blocked=%v kinds=%v", v, blocked, kinds)
		}
	}
	if !isChallenge(&Record{S: 403, R: "ua:challenge"}) || isChallenge(&Record{S: 303, R: "ua:challenge_pass"}) {
		t.Error("isChallenge")
	}
	c := New()
	c.Add(&Record{S: 403, R: "ua:challenge", H: "a", IP: "1.1.1.1"})
	if len(c.ruleStats) != 0 {
		t.Errorf("under-attack challenge recorded as a rule: %+v", c.ruleStats)
	}
}

// Access queue answers (429 + Retry-After) are visitors told to wait: not rate limited,
// not blocked, not server errors. The per-IP entry limit stays a rate limit.
func TestWaitroomHeldNotRateLimited(t *testing.T) {
	for _, wr := range []string{"cookie-check", "queue", "full"} {
		blocked, kinds := classify(&Record{S: 429, WR: wr})
		for _, k := range kinds {
			if k == "rate_limited" || k == "server_error" {
				t.Errorf("%s: kind %s", wr, k)
			}
		}
		if blocked {
			t.Errorf("%s: blocked", wr)
		}
	}
	if blocked, _ := classify(&Record{S: 429, WR: "ip-limit"}); !blocked {
		t.Error("ip-limit 429 must stay a rate limit")
	}
}
