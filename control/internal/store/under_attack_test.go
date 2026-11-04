package store

import (
	"context"
	"testing"
)

// Hosts from before the under-attack columns read back as "off" with an empty
// configuration; mode and parameters round-trip.
func TestUnderAttackColumns(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	t0 := now()
	if _, err := s.q.ExecContext(ctx, `INSERT INTO proxy_hosts(domains, forward_host, forward_port, created_at, updated_at) VALUES (?,?,?,?,?)`,
		`["legacy.example.com"]`, "app", 80, t0, t0); err != nil {
		t.Fatal(err)
	}
	hosts, err := s.ListProxyHosts(ctx)
	if err != nil || len(hosts) != 1 || hosts[0].UnderAttack != "off" || hosts[0].UnderAttackConfig != (UnderAttackConfig{}) {
		t.Fatalf("legacy host: %+v %v", hosts, err)
	}
	h := &hosts[0]
	pct := 0.0
	h.UnderAttack = "auto"
	h.UnderAttackConfig = UnderAttackConfig{Mode: "cookie", Difficulty: 16, TTL: 600, Bind: "ip_ua", RPS: 50, BlockedPct: &pct, WindowS: 60, DurationS: 300}
	if err := s.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	got, err := s.ProxyHostByID(ctx, h.ID)
	c := got.UnderAttackConfig
	if err != nil || got.UnderAttack != "auto" || c.Mode != "cookie" || c.TTL != 600 || c.Bind != "ip_ua" || c.RPS != 50 ||
		c.BlockedPct == nil || *c.BlockedPct != 0 || c.WindowS != 60 || c.DurationS != 300 {
		t.Fatalf("round trip: %+v %v", got, err)
	}
}
