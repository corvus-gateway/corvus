package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"corvus/control/internal/deploy"
	"corvus/control/internal/store"
)

func TestSecret(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "waitroom_hmac")

	// created when missing, then stable
	v, err := secret(p, true)
	if err != nil || len(v) < secretMinLen {
		t.Fatalf("new secret %q %v", v, err)
	}
	if again, _ := secret(p, true); again != v {
		t.Fatal("secret changed on second read")
	}
	if b, _ := os.ReadFile(p); strings.TrimSpace(string(b)) != v {
		t.Fatalf("file holds %q", b)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*tmp*")); len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}

	// damaged (truncated by a crash): replaceable secrets get a new value...
	_ = os.WriteFile(p, nil, 0o600)
	nv, err := secret(p, true)
	if err != nil || len(nv) < secretMinLen || nv == v {
		t.Fatalf("regenerated %q %v", nv, err)
	}

	// ...the data key does not: a new one would make every sealed setting unreadable
	key := filepath.Join(dir, "data_key")
	_ = os.WriteFile(key, []byte("short\n"), 0o600)
	if _, err := secret(key, false); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("damaged data key: %v", err)
	}
	if b, _ := os.ReadFile(key); string(b) != "short\n" {
		t.Fatal("a damaged data key must be left as it is")
	}
	_ = os.Remove(key)
	if k, err := secret(key, false); err != nil || len(k) < secretMinLen {
		t.Fatalf("missing data key is created: %q %v", k, err)
	}

	// unreadable (here: a directory) is an error, not a reason to overwrite
	if _, err := secret(dir, true); err == nil {
		t.Fatal("reading a directory must fail")
	}
}

type fakeFallback struct {
	calls []string
	err   error
}

func (f *fakeFallback) Fallback(_ context.Context, failed string, _ string) (*store.ConfigVersion, error) {
	f.calls = append(f.calls, fmt.Sprintf("fallback(%s)", failed))
	if f.err != nil {
		return nil, f.err
	}
	return &store.ConfigVersion{Message: "fallback to #1", ConfigRev: 7, Automatic: true}, nil
}

func (f *fakeFallback) SafeMode(_ context.Context, failed string, _ string) (*store.ConfigVersion, error) {
	f.calls = append(f.calls, fmt.Sprintf("safe(%s)", failed))
	if f.err != nil {
		return nil, f.err
	}
	return &store.ConfigVersion{Message: "safe mode: default server only", ConfigRev: -1, Automatic: true}, nil
}

func TestGatewayBootFailure(t *testing.T) {
	f := &fakeFallback{}
	policy := gatewayBootFailure(context.Background(), f)
	var switched []bool
	for n := 1; n <= 6; n++ {
		switched = append(switched, policy(n, fmt.Sprintf("v%06d", n)))
	}
	if fmt.Sprint(switched) != "[false false true true false false]" || strings.Join(f.calls, ",") != "fallback(v000003),safe(v000004)" {
		t.Fatalf("switched %v, calls %v", switched, f.calls)
	}

	// the fallback already ended in safe mode (recognised by its revision, not its
	// message): no second safe-mode version
	f = &fakeFallback{}
	policy = gatewayBootFailure(context.Background(), safeFallback{f})
	for n := 1; n <= 5; n++ {
		policy(n, "v000001")
	}
	if strings.Join(f.calls, ",") != "fallback(v000001)" {
		t.Fatalf("calls %v", f.calls)
	}

	// an operator deployed meanwhile: the fallback is abandoned, nothing switched
	f = &fakeFallback{err: fmt.Errorf("wrapped: %w", deploy.ErrLiveChanged)}
	policy = gatewayBootFailure(context.Background(), f)
	for n := 1; n <= 3; n++ {
		if policy(n, "v000001") {
			t.Fatalf("switched at %d although the live configuration changed", n)
		}
	}
}

// safeFallback: no earlier configuration was usable, Fallback ended in safe mode.
type safeFallback struct{ *fakeFallback }

func (s safeFallback) Fallback(ctx context.Context, failed string, reason string) (*store.ConfigVersion, error) {
	_, _ = s.fakeFallback.Fallback(ctx, failed, reason)
	return &store.ConfigVersion{Message: "fallback ended in: safe mode", ConfigRev: -1, Automatic: true}, nil
}

// fakeRedisStarter fails its first starts (with any error) and then succeeds.
type fakeRedisStarter struct {
	mu         sync.Mutex
	fails      []error
	starts     int
	supervised bool
}

func (f *fakeRedisStarter) Start(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
	if len(f.fails) > 0 {
		err := f.fails[0]
		f.fails = f.fails[1:]
		return err
	}
	f.supervised = true
	return nil
}

func (f *fakeRedisStarter) Supervised() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.supervised
}

// Whatever made Start fail (the binary missing, or the process started and did not
// answer in time: it was ended), starting is retried until Redis runs supervised.
func TestStartRedisRetries(t *testing.T) {
	oldFirst, oldMax := redisRetryFirst, redisRetryMax
	redisRetryFirst, redisRetryMax = time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { redisRetryFirst, redisRetryMax = oldFirst, oldMax })

	f := &fakeRedisStarter{fails: []error{
		&net.OpError{Op: "dial", Err: errors.New("connection refused")}, // not ready in time
		&os.PathError{Op: "fork/exec", Path: "/usr/bin/valkey-server", Err: os.ErrNotExist},
		context.DeadlineExceeded,
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startRedis(ctx, f)
	deadline := time.Now().Add(5 * time.Second)
	for !f.Supervised() {
		if time.Now().After(deadline) {
			t.Fatalf("redis not started after %d attempts", f.starts)
		}
		time.Sleep(time.Millisecond)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.starts != 4 {
		t.Fatalf("starts = %d, want 4", f.starts)
	}
}
