package conncheck

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// CheckAll checks every backend of a pool, results in order, a bounded number at once.
func TestCheckAll(t *testing.T) {
	_, open := listenPort(t)
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	refused := closed.Addr().(*net.TCPAddr).Port
	closed.Close()
	var running, peak atomic.Int32
	c := &Checker{Timeout: time.Second, Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		n := running.Add(1)
		defer running.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}}
	targets := []Target{{"127.0.0.1", open}, {"127.0.0.1", refused}}
	for i := 0; i < 20; i++ {
		targets = append(targets, Target{"127.0.0.1", open})
	}
	res := c.CheckAll(context.Background(), "http", targets)
	if len(res) != len(targets) || !res[0].OK || res[1].OK || res[1].Kind != KindRefused || !res[21].OK {
		t.Fatalf("results: %+v", res[:2])
	}
	if p := peak.Load(); p > MaxParallel || p < 2 {
		t.Fatalf("checked %d at once, want 2..%d", p, MaxParallel)
	}
	if res := c.CheckAll(context.Background(), "http", nil); len(res) != 0 {
		t.Fatalf("no targets: %+v", res)
	}
}
