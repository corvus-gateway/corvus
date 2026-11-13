package store

import (
	"context"
	"testing"
	"time"
)

// PROXY protocol flags (0030): existing streams default to off; both round-trip.
func TestStreamProxyProtocol(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	var n int
	if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = '0030_stream_proxy_protocol'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("migration not applied: %d %v", n, err)
	}
	// a row written without the new columns (as before the migration)
	t0 := time.Now()
	if _, err := s.q.ExecContext(ctx, `INSERT INTO streams(incoming_port, tcp_forwarding, udp_forwarding, forward_host, forward_port, enabled, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)`, 25, 1, 0, "mail", 25, 1, t0, t0); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListStreams(ctx)
	if err != nil || len(list) != 1 || list[0].ProxyProtocolIn || list[0].ProxyProtocolOut {
		t.Fatalf("defaults: %+v %v", list, err)
	}
	st := &list[0]
	st.ProxyProtocolIn = true
	if err := s.SaveStream(ctx, st); err != nil {
		t.Fatal(err)
	}
	if got, err := s.StreamByID(ctx, st.ID); err != nil || !got.ProxyProtocolIn || got.ProxyProtocolOut {
		t.Fatalf("in: %+v %v", got, err)
	}
	st2 := &Stream{IncomingPort: 26, TCP: true, ForwardHost: "mail", ForwardPort: 25, Enabled: true, ProxyProtocolOut: true}
	if err := s.SaveStream(ctx, st2); err != nil {
		t.Fatal(err)
	}
	if got, err := s.StreamByID(ctx, st2.ID); err != nil || got.ProxyProtocolIn || !got.ProxyProtocolOut {
		t.Fatalf("out: %+v %v", got, err)
	}
}
