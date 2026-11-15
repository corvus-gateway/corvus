package gateway

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Redis supervises the embedded, Redis-compatible store (Valkey) that holds the access
// queue state when no external Redis is configured. It listens on loopback only, loads
// the generated ACL file (default user disabled, H12) and keeps nothing on disk: queue
// state is short-lived by design. If the process dies it is restarted.
type Redis struct {
	Bin       string // e.g. /usr/bin/valkey-server
	Port      string // loopback port, e.g. 6379
	ACLFile   string
	Dir       string // working directory (writable; nothing is persisted)
	MaxMemory string // e.g. 256mb

	mu          sync.Mutex
	cmd         *exec.Cmd
	stopped     bool
	supervising bool // the supervise goroutine runs (restarts the process when it exits)
	exited      chan struct{}
	started     time.Time // start of the current process
	exitErr     error     // how the last process ended (set before exited is closed)
}

// redisHealthUser may only PING and needs no password (generated ACL file, see
// live.ACLFile): a server on the port that accepts it is ours.
const redisHealthUser = "health"

// redisStableUptime: a server that ran this long is healthy again, so the restart
// backoff starts over.
const redisStableUptime = 5 * time.Minute

// redisSettle: after the first successful PING the server must still be running this
// long. A server that cannot bind its port exits right away, while another server on
// the port may have answered the PING.
var redisSettle = 200 * time.Millisecond

// redisReadyTimeout bounds how long Start waits for the server to answer.
var redisReadyTimeout = 10 * time.Second

func (r *Redis) args() []string {
	return []string{
		"--bind", "127.0.0.1", "--port", r.Port, "--protected-mode", "yes",
		"--aclfile", r.ACLFile, "--dir", r.Dir,
		"--save", "", "--appendonly", "no",
		"--maxmemory", r.MaxMemory, "--maxmemory-policy", "noeviction",
		"--daemonize", "no", "--logfile", "", "--loglevel", "warning",
	}
}

// Start launches the server, waits until it answers an authenticated PING and keeps it
// running until Stop is called or ctx ends. It fails when the server exits during
// startup, does not answer in time or another server holds the port; the process it
// started is then ended and nothing supervises it, so Start may be called again.
// Start does nothing while a supervised server runs.
func (r *Redis) Start(ctx context.Context) error {
	if r.Supervised() {
		return nil
	}
	if err := os.MkdirAll(r.Dir, 0o700); err != nil {
		return err
	}
	if err := r.spawn(); err != nil {
		return err
	}
	if err := r.waitReady(ctx, redisReadyTimeout); err != nil {
		r.kill(2 * time.Second) // not Stop: a later Start must still be able to spawn
		return err
	}
	r.mu.Lock()
	if r.stopped { // Stop was called meanwhile
		r.mu.Unlock()
		r.kill(2 * time.Second)
		return errors.New("redis stopped")
	}
	r.supervising = true
	r.mu.Unlock()
	go r.supervise(ctx)
	return nil
}

// Supervised reports whether a server started by Start is kept running (its supervisor
// runs and Stop was not called).
func (r *Redis) Supervised() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.supervising && !r.stopped
}

func (r *Redis) addr() string { return net.JoinHostPort("127.0.0.1", r.Port) }

// waitReady waits until the started process answers an authenticated PING and is
// still running.
func (r *Redis) waitReady(ctx context.Context, timeout time.Duration) error {
	r.mu.Lock()
	exited := r.exited
	r.mu.Unlock()
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-exited:
			return r.startupError()
		default:
		}
		err := pingRedis(r.addr(), 500*time.Millisecond)
		if err == nil {
			select {
			case <-exited:
				return r.startupError()
			case <-time.After(redisSettle):
				return nil
			}
		}
		if time.Now().After(deadline) {
			var fe *foreignRedisError
			if errors.As(err, &fe) {
				return fmt.Errorf("%w; %s", err, r.portHint())
			}
			return fmt.Errorf("embedded redis on %s not ready: %w", r.addr(), err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-exited:
			return r.startupError()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (r *Redis) portHint() string {
	return fmt.Sprintf("set CORVUS_REDIS_PORT to a free port (the embedded server only listens on 127.0.0.1:%s; "+
		"with host networking a Redis on the host may already use it)", r.Port)
}

// startupError explains why the process ended during startup.
func (r *Redis) startupError() error {
	r.mu.Lock()
	exitErr := r.exitErr
	r.mu.Unlock()
	if exitErr == nil {
		exitErr = errors.New("exit status 0")
	}
	if portAnswers(r.addr()) {
		return fmt.Errorf("embedded redis exited during startup (%v): port %s is already in use by another server; %s",
			exitErr, r.Port, r.portHint())
	}
	return fmt.Errorf("embedded redis exited during startup: %v", exitErr)
}

func (r *Redis) spawn() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return errors.New("redis stopped")
	}
	cmd := exec.Command(r.Bin, r.args()...)
	cmd.Dir = r.Dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	r.cmd = cmd
	r.exited = make(chan struct{})
	r.started = time.Now()
	r.exitErr = nil
	go func(c *exec.Cmd, done chan struct{}) {
		err := c.Wait()
		r.mu.Lock()
		if r.exited == done {
			r.exitErr = err
		}
		r.mu.Unlock()
		close(done)
	}(cmd, r.exited)
	slog.Info("embedded redis started", "pid", cmd.Process.Pid, "port", r.Port)
	return nil
}

// nextRedisBackoff returns the delay before the next restart: doubled up to 32 s, and
// back to 1 s after a process that ran for redisStableUptime.
func nextRedisBackoff(cur, uptime time.Duration) time.Duration {
	if uptime >= redisStableUptime || cur <= 0 {
		return time.Second
	}
	if cur < 30*time.Second {
		return cur * 2
	}
	return cur
}

func (r *Redis) supervise(ctx context.Context) {
	defer func() {
		r.mu.Lock()
		r.supervising = false
		r.mu.Unlock()
	}()
	var backoff time.Duration
	for {
		r.mu.Lock()
		exited := r.exited
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-exited:
		}
		r.mu.Lock()
		stopped, started, exitErr := r.stopped, r.started, r.exitErr
		r.mu.Unlock()
		if stopped {
			return
		}
		backoff = nextRedisBackoff(backoff, time.Since(started))
		if time.Since(started) < 10*time.Second && portAnswers(r.addr()) {
			slog.Error("embedded redis cannot start: port "+r.Port+" is in use by another server; "+r.portHint(), "err", exitErr)
		}
		slog.Error("embedded redis exited; restarting", "in", backoff, "err", exitErr)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if err := r.spawn(); err != nil {
			slog.Error("restart embedded redis", "err", err)
		}
	}
}

// Stop terminates the server (SIGTERM, then SIGKILL after timeout) for good: it is not
// restarted afterwards.
func (r *Redis) Stop(timeout time.Duration) {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
	r.kill(timeout)
}

// kill ends the current process (SIGTERM, then SIGKILL after timeout) and waits until
// it has exited, so its port is free for the next start.
func (r *Redis) kill(timeout time.Duration) {
	r.mu.Lock()
	cmd, exited := r.cmd, r.exited
	r.mu.Unlock()
	if cmd == nil || cmd.Process == nil || exited == nil {
		return
	}
	select {
	case <-exited:
		return
	default:
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = cmd.Process.Kill() // e.g. Windows: no SIGTERM
	}
	select {
	case <-exited:
		return
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
	}
	select {
	case <-exited:
	case <-time.After(timeout):
	}
}

// foreignRedisError: the server on the port refused the health user, so it is not the
// embedded server (another Redis).
type foreignRedisError struct{ addr, reply string }

func (e *foreignRedisError) Error() string {
	return fmt.Sprintf("the server on %s is not the embedded redis (it refused the %q user: %s)", e.addr, redisHealthUser, e.reply)
}

// pingRedis authenticates as the health user and sends PING.
func pingRedis(addr string, timeout time.Duration) error {
	c, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(timeout))
	u := redisHealthUser
	cmd := fmt.Sprintf("*3\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n$1\r\n-\r\n*1\r\n$4\r\nPING\r\n", len(u), u)
	if _, err := io.WriteString(c, cmd); err != nil {
		return err
	}
	br := bufio.NewReader(c)
	auth, err := br.ReadString('\n')
	if err != nil {
		return err
	}
	if auth = strings.TrimSpace(auth); auth != "+OK" {
		return &foreignRedisError{addr, auth}
	}
	pong, err := br.ReadString('\n')
	if err != nil {
		return err
	}
	if pong = strings.TrimSpace(pong); pong != "+PONG" {
		return fmt.Errorf("unexpected PING reply from %s: %q", addr, pong)
	}
	return nil
}

// portAnswers reports whether something accepts TCP connections on addr.
func portAnswers(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// WaitTCP waits until addr accepts TCP connections.
func WaitTCP(ctx context.Context, addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			c.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
