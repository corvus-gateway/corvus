package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Static pages (0034): existing hosts forward and have no page; content and page
// round-trip, also a large custom page.
func TestProxyHostStaticPage(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	var n int
	if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = '0034_proxy_static_page'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("migration not applied: %d %v", n, err)
	}
	// a row written without the new columns (as before the migration)
	t0 := time.Now()
	if _, err := s.q.ExecContext(ctx, `INSERT INTO proxy_hosts(domains, forward_scheme, forward_host, forward_port, enabled, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?)`, `["old.example.com"]`, "http", "app", 8080, 1, t0, t0); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListProxyHosts(ctx)
	if err != nil || len(list) != 1 || list[0].Content != ContentForward || !list[0].Page.IsZero() || list[0].IsPage() {
		t.Fatalf("defaults: %+v %v", list, err)
	}
	h := &list[0]
	html := "<!doctype html><p>" + strings.Repeat("é", 100*1024) + "</p>"
	h.Content, h.Page = ContentPage, StaticPage{Template: PageCustom, Status: 503, HTML: html}
	if err := s.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	got, err := s.ProxyHostByID(ctx, h.ID)
	if err != nil || !got.IsPage() || got.Page != h.Page || got.Page.HTMLBytes != len(html) {
		t.Fatalf("round trip: %v %v %+v", err, got.Content, got.Page.Status)
	}
	// a new host without a content mode is stored as forward
	nh := &ProxyHost{Domains: []string{"new.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80}
	if err := s.SaveProxyHost(ctx, nh); err != nil || nh.Content != ContentForward {
		t.Fatalf("new host: %q %v", nh.Content, err)
	}
	if got, err := s.ProxyHostByID(ctx, nh.ID); err != nil || got.Content != ContentForward || !got.Page.IsZero() {
		t.Fatalf("new host read back: %+v %v", got, err)
	}
}
