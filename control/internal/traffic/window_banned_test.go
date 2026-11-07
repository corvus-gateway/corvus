package traffic

import (
	"testing"
	"time"
)

// Requests refused because the client is banned do not count toward "under attack".
func TestWindowIgnoresBanned(t *testing.T) {
	c := New()
	now := time.Now().Truncate(10 * time.Second)
	for i := 0; i < 30; i++ {
		ts := float64(now.Add(-20 * time.Second).Unix())
		c.Add(&Record{T: ts, H: "shop.example.com", U: "/", S: 200, IP: "198.51.100.1"})
		for j := 0; j < 4; j++ {
			c.Add(&Record{T: ts, H: "shop.example.com", U: "/x", S: 403, IP: "203.0.113.9", BN: "1"})
		}
	}
	req, blocked, _ := c.Window("shop.example.com", 30*time.Second, now)
	if req != 30 || blocked != 0 {
		t.Fatalf("requests=%d blocked=%d, want 30 and 0", req, blocked)
	}
}
