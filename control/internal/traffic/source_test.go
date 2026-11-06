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
