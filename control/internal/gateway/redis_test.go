package gateway

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeRedis answers like a Redis server: the embedded one (the health user may PING)
// or, with foreign set, another server that refuses the health user.
func fakeRedis(t *testing.T, foreign bool) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go serveFakeRedis(l, foreign)
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return port
}

func serveFakeRedis(l net.Listener, foreign bool) {
	func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(5 * time.Second))
				br := bufio.NewReader(c)
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					if strings.TrimSpace(line) == "PING" {
						break
					}
				}
				if foreign {
					_, _ = io.WriteString(c, "-WRONGPASS invalid username-password pair or user is disabled.\r\n-NOAUTH Authentication required.\r\n")
				} else {
					_, _ = io.WriteString(c, "+OK\r\n+PONG\r\n")
				}
			}(c)
		}
	}()
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	l.Close()
	return port
}

func TestPingRedis(t *testing.T) {
	if err := pingRedis("127.0.0.1:"+fakeRedis(t, false), time.Second); err != nil {
		t.Fatalf("own server: %v", err)
	}
	err := pingRedis("127.0.0.1:"+fakeRedis(t, true), time.Second)
	var fe *foreignRedisError
	if !errors.As(err, &fe) || !strings.Contains(err.Error(), "WRONGPASS") {
		t.Fatalf("foreign server: %v", err)
	}
	if err := pingRedis("127.0.0.1:"+freePort(t), 200*time.Millisecond); err == nil {
		t.Fatal("closed port answered")
	}
}

// redisWithoutProcess stands in for a started server: exited is closed to simulate
// the process ending.
func redisWithoutProcess(port string) *Redis {
	return &Redis{Port: port, exited: make(chan struct{}), started: time.Now()}
}

func TestRedisWaitReady(t *testing.T) {
	ctx := context.Background()
	// own server answers and the process keeps running
	r := redisWithoutProcess(fakeRedis(t, false))
	if err := r.waitReady(ctx, 2*time.Second); err != nil {
		t.Fatal(err)
	}

	// another Redis holds the port; the process could not bind and exited
	r = redisWithoutProcess(fakeRedis(t, true))
	r.exitErr = errors.New("exit status 1")
	close(r.exited)
	err := r.waitReady(ctx, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "already in use") || !strings.Contains(err.Error(), "CORVUS_REDIS_PORT") ||
		!strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("foreign server, exited process: %v", err)
	}

	// another Redis answers and the process keeps running (never ready)
	r = redisWithoutProcess(fakeRedis(t, true))
	if err := r.waitReady(ctx, 300*time.Millisecond); err == nil || !strings.Contains(err.Error(), "CORVUS_REDIS_PORT") {
		t.Fatalf("foreign server: %v", err)
	}

	// the process exits after the first PING (another server answered it)
	r = redisWithoutProcess(fakeRedis(t, false))
	go func() { time.Sleep(redisSettle / 4); close(r.exited) }()
	if err := r.waitReady(ctx, 2*time.Second); err == nil || !strings.Contains(err.Error(), "exited during startup") {
		t.Fatalf("exit while settling: %v", err)
	}

	// nothing listens and the process exited
	r = redisWithoutProcess(freePort(t))
	close(r.exited)
	if err := r.waitReady(ctx, time.Second); err == nil || strings.Contains(err.Error(), "already in use") {
		t.Fatalf("exited without a port: %v", err)
	}

	// nothing listens, the process runs: timeout, or the context ends first
	r = redisWithoutProcess(freePort(t))
	if err := r.waitReady(ctx, 200*time.Millisecond); err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("timeout: %v", err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := r.waitReady(cctx, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

func TestNextRedisBackoff(t *testing.T) {
	steps := []time.Duration{}
	var b time.Duration
	for i := 0; i < 8; i++ {
		b = nextRedisBackoff(b, time.Second)
		steps = append(steps, b)
	}
	want := []time.Duration{1, 2, 4, 8, 16, 32, 32, 32}
	for i, w := range want {
		if steps[i] != w*time.Second {
			t.Fatalf("backoff %v, want %v s", steps, want)
		}
	}
	// a server that ran for the stable uptime starts over
	if got := nextRedisBackoff(32*time.Second, redisStableUptime); got != time.Second {
		t.Fatalf("after a stable run: %v", got)
	}
}
