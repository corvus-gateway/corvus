package traffic

import (
	"fmt"
	"testing"
	"time"
)

// Past the host limit, a new host replaces one idle for over an hour instead of being
// counted as "(other)".
func TestHostCapEvictsIdle(t *testing.T) {
	c := New()
	now := time.Now()
	for i := 0; i < maxHosts; i++ {
		at := now.Add(-2 * time.Hour)
		if i > 0 {
			at = now.Add(-time.Minute)
		}
		c.Add(&Record{T: float64(at.Unix()), H: fmt.Sprintf("h%d.example.com", i), U: "/", S: 200, IP: "198.51.100.1"})
	}
	c.Add(&Record{T: float64(now.Unix()), H: "new.example.com", U: "/", S: 200, IP: "198.51.100.1"})
	if c.hosts["new.example.com"] == nil || c.hosts["h0.example.com"] != nil || c.hosts["(other)"] != nil {
		t.Fatal("idle host not replaced by the new one")
	}
	c.Add(&Record{T: float64(now.Unix()), H: "newer.example.com", U: "/", S: 200, IP: "198.51.100.1"})
	if c.hosts["(other)"] == nil {
		t.Fatal("with no idle host left, new names go to (other)")
	}
}
