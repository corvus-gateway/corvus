package gateway

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMain lets the test binary stand in for valkey-server (CORVUS_FAKE_REDIS):
// "hang" runs without answering, "serve" answers on the --port it is given.
func TestMain(m *testing.M) {
	if mode := os.Getenv("CORVUS_FAKE_REDIS"); mode != "" {
		fakeRedisProcess(mode, os.Args[1:])
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeRedisProcess(mode string, args []string) {
	if mode == "serve" {
		port := ""
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "--port" {
				port = args[i+1]
			}
		}
		l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
		if err != nil {
			os.Exit(1)
		}
		go serveFakeRedis(l, false)
	}
	time.Sleep(time.Hour)
}

func fakeRedisServer(t *testing.T, bin string) *Redis {
	return &Redis{Bin: bin, Port: freePort(t), Dir: filepath.Join(t.TempDir(), "redis"), MaxMemory: "1mb"}
}

// A start that fails after the process was started (it does not answer in time) ends
// that process without stopping the Redis for good: the next Start spawns it again and
// supervises it.
func TestRedisStartAgainAfterNotReady(t *testing.T) {
	old := redisReadyTimeout
	redisReadyTimeout = 300 * time.Millisecond
	t.Cleanup(func() { redisReadyTimeout = old })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := fakeRedisServer(t, os.Args[0])
	t.Cleanup(func() { r.Stop(2 * time.Second) })
	t.Setenv("CORVUS_FAKE_REDIS", "hang")
	if err := r.Start(ctx); err == nil {
		t.Fatal("a server that never answers must fail to start")
	}
	r.mu.Lock()
	exited, stopped := r.exited, r.stopped
	r.mu.Unlock()
	select {
	case <-exited:
	default:
		t.Fatal("the process that did not answer is still running")
	}
	if stopped || r.Supervised() {
		t.Fatalf("after a failed start: stopped=%v supervised=%v", stopped, r.Supervised())
	}

	t.Setenv("CORVUS_FAKE_REDIS", "serve")
	if err := r.Start(ctx); err != nil {
		t.Fatalf("second start: %v", err)
	}
	if !r.Supervised() {
		t.Fatal("a started server must be supervised")
	}
	if err := r.Start(ctx); err != nil { // already running: nothing to do
		t.Fatal(err)
	}
	r.Stop(2 * time.Second)
	if r.Supervised() {
		t.Fatal("still supervised after Stop")
	}
}

// A start that could not even spawn the process can be repeated too.
func TestRedisStartAgainAfterSpawnError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := fakeRedisServer(t, filepath.Join(t.TempDir(), "missing-valkey-server"))
	t.Cleanup(func() { r.Stop(2 * time.Second) })
	if err := r.Start(ctx); err == nil {
		t.Fatal("a missing binary must fail")
	}
	t.Setenv("CORVUS_FAKE_REDIS", "serve")
	r.Bin = os.Args[0]
	if err := r.Start(ctx); err != nil {
		t.Fatalf("start after the binary appeared: %v", err)
	}
	if !r.Supervised() {
		t.Fatal("not supervised")
	}
}
