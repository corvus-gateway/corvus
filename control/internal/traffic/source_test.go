package traffic

import (
	"fmt"
	"testing"
	"time"
)

// Per-IP counts for automatic bans survive a flood of other events that overflows the
// display ring (eventCap entries).
func TestEventsByIPSurvivesFlood(t *testing.T) {
	c := New()
	now := time.Now().Truncate(10 * time.Second).Add(-5 * time.Minute)
	for i := 0; i < 30; i++ {
		c.Add(&Record{T: float64(now.Unix()) + float64(i), H: "shop.example.com", U: fmt.Sprintf("/a%d", i), S: 403, IP: "203.0.113.9", X: "sqli"})
	}
	for i := 0; i < 3*eventCap; i++ {
		c.Add(&Record{T: float64(now.Unix()) + 40 + float64(i%60), H: "shop.example.com", U: fmt.Sprintf("/n%d", i), S: 403, IP: fmt.Sprintf("198.51.100.%d", i%250), X: "sqli"})
	}
	got := c.EventsByIP(now.Add(-time.Second), "blocked_exploit")["203.0.113.9"]
	if got == nil || got.Count != 30 || !got.Hosts["shop.example.com"] {
		t.Fatalf("attacker activity = %+v, want 30", got)
	}
	if after := c.EventsByIP(now.Add(35*time.Second), "blocked_exploit")["203.0.113.9"]; after != nil {
		t.Fatalf("events from before since were counted: %+v", after)
	}
}

// Only the detections bans and alerts act on are kept per address, and the number of
// addresses in all slots together is bounded.
func TestSourceSlotsBounded(t *testing.T) {
	c := &Collector{}
	now := time.Now().Unix()
	c.countSource(now, "banned", "a.example", "2001:db8::1", 1)
	c.countSource(now, "rate_limited", "a.example", "2001:db8::2", 1)
	if c.srcN != 0 {
		t.Fatalf("kinds nobody reads are tracked: %d", c.srcN)
	}
	// an old, full hour that nothing has overwritten
	old := now - 2*srcSlots*srcStep
	for i := 0; c.srcN < srcTotalCap; i++ {
		c.countSource(old+int64(i%srcSlots)*srcStep, "blocked_rule", "a.example", fmt.Sprint("198.51.100.", i), 1)
	}
	c.countSource(old+5*srcStep, "blocked_rule", "a.example", "203.0.113.1", 1)
	if c.srcN != srcTotalCap {
		t.Fatalf("over the cap: %d", c.srcN)
	}
	// new events are still counted: the stale slots make room
	c.countSource(now, "blocked_rule", "a.example", "203.0.113.9", 3)
	got := c.EventsByIP(time.Unix(now-srcStep, 0), SourceKinds...)["203.0.113.9"]
	if got == nil || got.Count != 3 {
		t.Fatalf("event after a full hour: %+v (tracked %d)", got, c.srcN)
	}
	n := 0
	for i := range c.src {
		n += len(c.src[i].ips)
	}
	if n != c.srcN {
		t.Fatalf("srcN %d, slots hold %d", c.srcN, n)
	}
}
