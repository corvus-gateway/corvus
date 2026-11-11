package alerts

import (
	"context"
	"testing"
	"time"

	"corvus/control/internal/traffic"
)

// Log-only detections and scanners must raise alerts without any blocking configured.
func TestSecurityAlertsWithoutBlocking(t *testing.T) {
	e, tc, _, _ := setup(t)
	ctx := context.Background()
	now := time.Now()
	ts := float64(now.Unix())
	// 12 exploit-pattern requests (detect only, not blocked) from one IP
	for i := 0; i < 12; i++ {
		tc.Add(&traffic.Record{T: ts - float64(i), H: "shop.example.com", U: "/x", S: 200, IP: "203.0.113.9", X: "sqli"})
	}
	// 25 log-only security-rule hits on one host
	for i := 0; i < 25; i++ {
		tc.Add(&traffic.Record{T: ts - float64(i), H: "api.example.com", U: "/login", S: 200, IP: "198.51.100.1", R: "7:log"})
	}
	// a scanner: 150 404s
	for i := 0; i < 150; i++ {
		tc.Add(&traffic.Record{T: ts - float64(i%120), H: "shop.example.com", U: "/wp-admin", S: 404, IP: "192.0.2.77"})
	}
	evalWait(e, ctx, now)
	got := map[string]string{}
	for _, a := range e.Active() {
		got[a.Rule+"|"+a.Key] = a.Title
	}
	for _, want := range []string{"attack_source|203.0.113.9", "security_events|api.example.com", "scan_suspect|192.0.2.77"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing alert %s; active: %v", want, got)
		}
	}
	if _, ok := got["scan_suspect|198.51.100.1"]; ok {
		t.Error("normal client flagged as scanner")
	}
}
