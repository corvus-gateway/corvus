//go:build linux

package gateway

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A gateway that cannot start (a port in use) is retried, reports why, and starts at
// once when a new configuration is deployed (Reload); after Stop it stays down.
func TestSupervise(t *testing.T) {
	oldStable, oldMin, oldMax := stableAfter, minBackoff, maxBackoff
	stableAfter, minBackoff, maxBackoff = time.Hour, time.Hour, time.Hour // retries only when asked
	t.Cleanup(func() { stableAfter, minBackoff, maxBackoff = oldStable, oldMin, oldMax })

	dir := t.TempDir()
	bin := writeScript(t, dir, "openresty", `dir=$(dirname "$0")
echo start >> "$dir/starts"
if [ ! -e "$dir/ok" ]; then
	echo "2026/11/12 01:55:15 [emerg] 63#63: bind() to 0.0.0.0:8443 failed (98: Address already in use)" >&2
	exit 1
fi
trap 'exit 0' QUIT
while true; do sleep 0.02; done
`)
	// started through the "current" symlink: the callback learns the version directory
	if err := os.MkdirAll(filepath.Join(dir, "versions", "v000007"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("versions", "v000007"), filepath.Join(dir, "current")); err != nil {
		t.Fatal(err)
	}
	o := &OpenResty{Bin: bin, CurrentConf: filepath.Join(dir, "current", "nginx.conf")}
	var mu sync.Mutex
	var calls []int
	var versions []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	go func() {
		o.Supervise(ctx, func(n int, version string) bool {
			mu.Lock()
			calls = append(calls, n)
			versions = append(versions, version)
			mu.Unlock()
			return n < 3 // "switched the configuration": retry at once
		})
		close(stopped)
	}()
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(calls) == 3 })
	mu.Lock()
	if calls[0] != 1 || calls[1] != 2 || calls[2] != 3 {
		t.Fatalf("consecutive failures = %v", calls)
	}
	if versions[0] != "v000007" {
		t.Fatalf("failed with version %q, want v000007", versions[0])
	}
	mu.Unlock()
	if running, _ := o.Status(); running {
		t.Fatal("must not report running")
	}
	if p := o.Problem(); !strings.Contains(p, "bind() to 0.0.0.0:8443 failed (98: Address already in use)") || strings.Contains(p, "[emerg]") {
		t.Fatalf("problem = %q", p)
	}
	// WaitActive does not wait for a gateway that is down
	o.StatusURL = "http://127.0.0.1:1/version"
	if err := o.WaitActive(ctx, "v9", time.Minute); err != nil {
		t.Fatal(err)
	}

	// the operator fixes the configuration and deploys: the gateway starts now
	_ = os.WriteFile(filepath.Join(dir, "ok"), nil, 0o644)
	if err := o.Reload(); err != nil {
		t.Fatalf("reload while down: %v", err)
	}
	waitFor(t, func() bool { r, _ := o.Status(); return r })
	if p := o.Problem(); p != "" {
		t.Fatalf("problem while running = %q", p)
	}
	// the process runs; let the script record its start before it is stopped (under
	// load the QUIT could otherwise arrive before its first line)
	waitFor(t, func() bool {
		b, _ := os.ReadFile(filepath.Join(dir, "starts"))
		return strings.Count(string(b), "start") >= 4
	})

	o.Stop(5 * time.Second)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not end after Stop")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "starts"))
	if n := strings.Count(string(b), "start"); n != 4 {
		t.Fatalf("starts = %d, want 4", n)
	}
	if err := o.Reload(); err == nil {
		t.Fatal("reload after Stop must fail")
	}
}

// A process that cannot even be started is retried too (no boot-failure callback).
func TestSuperviseStartError(t *testing.T) {
	oldMin := minBackoff
	minBackoff = 10 * time.Millisecond
	t.Cleanup(func() { minBackoff = oldMin })
	o := &OpenResty{Bin: filepath.Join(t.TempDir(), "missing"), CurrentConf: "x"}
	ctx, cancel := context.WithCancel(context.Background())
	called := false
	done := make(chan struct{})
	go func() { o.Supervise(ctx, func(int, string) bool { called = true; return false }); close(done) }()
	waitFor(t, func() bool { return strings.Contains(o.Problem(), "cannot start the gateway") })
	cancel()
	<-done
	if called {
		t.Fatal("boot-failure callback for a process that never started")
	}
}
