package store

import (
	"context"
	"testing"
)

// Hosts created before migration 0026 keep the global upload size and timeouts and get
// gzip; the new fields round-trip, including 0 (unlimited upload) and NULL.
func TestHostLimitsAndGzip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	t0 := now()
	if _, err := s.q.ExecContext(ctx, `INSERT INTO proxy_hosts(domains, forward_host, forward_port, created_at, updated_at) VALUES (?,?,?,?,?)`,
		`["legacy.example.com"]`, "app", 80, t0, t0); err != nil {
		t.Fatal(err)
	}
	hosts, err := s.ListProxyHosts(ctx)
	if err != nil || len(hosts) != 1 {
		t.Fatalf("%+v %v", hosts, err)
	}
	if h := hosts[0]; h.MaxUploadMB != nil || h.UpstreamTimeoutS != nil || !h.Gzip {
		t.Fatalf("legacy host: upload=%v timeout=%v gzip=%v", h.MaxUploadMB, h.UpstreamTimeoutS, h.Gzip)
	}

	zero, timeout := 0, 600
	h := &ProxyHost{Domains: []string{"new.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, WaitroomPath: "/", ClientIP: "direct",
		BodyInspection: "log", BodyMaxKB: 1024, Enabled: true, MaxUploadMB: &zero, UpstreamTimeoutS: &timeout}
	if err := s.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	got, err := s.ProxyHostByID(ctx, h.ID)
	if err != nil || got.MaxUploadMB == nil || *got.MaxUploadMB != 0 || got.UpstreamTimeoutS == nil || *got.UpstreamTimeoutS != 600 || got.Gzip {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	h.MaxUploadMB, h.UpstreamTimeoutS, h.Gzip = nil, nil, true
	if err := s.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ProxyHostByID(ctx, h.ID); got.MaxUploadMB != nil || got.UpstreamTimeoutS != nil || !got.Gzip {
		t.Fatalf("reset: %+v", got)
	}
}
