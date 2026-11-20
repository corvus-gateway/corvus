package store

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// Backend pools (0033): existing hosts get no extra targets; the list round-trips.
func TestProxyHostExtraTargets(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	var n int
	if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = '0033_proxy_extra_targets'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("migration not applied: %d %v", n, err)
	}
	// a row written without the new column (as before the migration)
	t0 := time.Now()
	if _, err := s.q.ExecContext(ctx, `INSERT INTO proxy_hosts(domains, forward_scheme, forward_host, forward_port, enabled, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?)`, `["old.example.com"]`, "http", "app", 8080, 1, t0, t0); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListProxyHosts(ctx)
	if err != nil || len(list) != 1 || list[0].ExtraTargets == nil || len(list[0].ExtraTargets) != 0 {
		t.Fatalf("defaults: %+v %v", list, err)
	}
	h := &list[0]
	if got := h.Targets(); !reflect.DeepEqual(got, []Target{{"app", 8080}}) {
		t.Fatalf("targets of a single-target host: %+v", got)
	}
	h.ExtraTargets = []Target{{"app2", 8080}, {"10.0.0.3", 8081}}
	if err := s.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	got, err := s.ProxyHostByID(ctx, h.ID)
	if err != nil || !reflect.DeepEqual(got.ExtraTargets, h.ExtraTargets) {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if all := got.Targets(); len(all) != 3 || all[0].ForwardHost != "app" || all[2].ForwardPort != 8081 {
		t.Fatalf("targets: %+v", all)
	}
	// a new host without extra targets stores and returns an empty list
	nh := &ProxyHost{Domains: []string{"new.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80}
	if err := s.SaveProxyHost(ctx, nh); err != nil || nh.ExtraTargets == nil {
		t.Fatalf("new host: %+v %v", nh.ExtraTargets, err)
	}
	if got, err := s.ProxyHostByID(ctx, nh.ID); err != nil || got.ExtraTargets == nil || len(got.ExtraTargets) != 0 {
		t.Fatalf("new host read back: %+v %v", got, err)
	}
}
