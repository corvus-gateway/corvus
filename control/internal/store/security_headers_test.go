package store

import (
	"context"
	"testing"
)

// Hosts created before the security_headers column existed (or written without it)
// read back as "off": the migration must not change any existing host's responses.
func TestSecurityHeadersDefaultsOff(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	t0 := now()
	if _, err := s.q.ExecContext(ctx, `INSERT INTO proxy_hosts(domains, forward_host, forward_port, created_at, updated_at) VALUES (?,?,?,?,?)`,
		`["legacy.example.com"]`, "app", 80, t0, t0); err != nil {
		t.Fatal(err)
	}
	hosts, err := s.ListProxyHosts(ctx)
	if err != nil || len(hosts) != 1 || hosts[0].SecurityHeaders != "off" {
		t.Fatalf("legacy host: %+v %v", hosts, err)
	}

	h := &ProxyHost{Domains: []string{"new.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, WaitroomPath: "/", ClientIP: "direct", BodyInspection: "log", BodyMaxKB: 1024, Enabled: true}
	if err := s.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ProxyHostByID(ctx, h.ID); got.SecurityHeaders != "off" {
		t.Fatalf("unset preset stored as %q, want off", got.SecurityHeaders)
	}
	h.SecurityHeaders = "strict"
	if err := s.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ProxyHostByID(ctx, h.ID); got.SecurityHeaders != "strict" {
		t.Fatalf("round trip: %q", got.SecurityHeaders)
	}
}
