// Package gateway runs and controls the OpenResty data plane as a child process.
package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Runner tests configurations and applies them to the running gateway.
type Runner interface {
	Test(ctx context.Context, conf string) (output string, err error)
	Reload() error
}

// OpenResty supervises an openresty master process started with -c <CurrentConf>.
// CurrentConf is a path through the "current" symlink, so a reload picks up
// whichever version the symlink points to.
type OpenResty struct {
	Bin         string
	CurrentConf string
	Env         []string // extra environment (secrets, Redis address)
	StatusURL   string   // e.g. http://127.0.0.1:9180/version

	mu         sync.Mutex
	cmd        *exec.Cmd
	done       chan error    // receives the exit status (Done)
	exited     chan struct{} // closed when the process has exited
	exitErr    error
	running    bool
	started    time.Time
	version    string    // version directory "current" pointed to at the last start
	errs       *errLog   // recent serious lines of the gateway's stderr
	reloadAt   time.Time // when the last reload was requested
	supervised bool      // Supervise restarts the gateway when it exits
	stopping   bool
	kick       chan struct{} // restart now (a new configuration was deployed while down)
	problem    string        // why the gateway last went down
}

// Status reports whether the gateway process is running and since when.
func (o *OpenResty) Status() (bool, time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.running, o.started
}

// Problem says why the gateway is down (for example the [emerg] lines of a failed
// start); empty while it runs.
func (o *OpenResty) Problem() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.running {
		return ""
	}
	return o.problem
}

func (o *OpenResty) env() []string { return append(os.Environ(), o.Env...) }

func (o *OpenResty) Test(ctx context.Context, conf string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, o.Bin, "-t", "-q", "-e", "stderr", "-c", conf)
	cmd.Env = o.env()
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	out := strings.TrimSpace(buf.String())
	if err != nil {
		if out == "" {
			out = err.Error()
		}
		return out, fmt.Errorf("configuration test failed")
	}
	if out == "" {
		out = "configuration test ok"
	}
	return out, nil
}

// Start launches openresty in the foreground (daemon off) and returns immediately.
// It fails while the gateway is running; after it exited it starts a new process.
func (o *OpenResty) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.running {
		return errors.New("gateway already started")
	}
	if o.stopping {
		return errors.New("gateway stopped")
	}
	cmd := exec.Command(o.Bin, "-e", "stderr", "-c", o.CurrentConf, "-g", "daemon off;") // -e: no write to the read-only prefix
	cmd.Env = o.env()
	if o.errs == nil {
		o.errs = newErrLog(os.Stderr)
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, o.errs
	if err := cmd.Start(); err != nil {
		return err
	}
	o.cmd = cmd
	o.started = time.Now()
	o.version = confVersion(o.CurrentConf)
	o.running = true
	done, exited := make(chan error, 1), make(chan struct{})
	o.done, o.exited = done, exited
	go func() {
		err := cmd.Wait()
		o.mu.Lock()
		o.running, o.exitErr = false, err
		o.mu.Unlock()
		close(exited)
		done <- err
	}()
	slog.Info("gateway started", "pid", cmd.Process.Pid, "conf", o.CurrentConf)
	return nil
}

// confVersion returns the version directory the symlinked directory of conf points to
// ("" when it is not a symlink).
func confVersion(conf string) string {
	target, err := os.Readlink(filepath.Dir(filepath.FromSlash(conf)))
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

// Restart policy of Supervise (variables for tests).
var (
	// a gateway that exits sooner after starting failed to start (bind() on a port in
	// use, a certificate that cannot be loaded): counted for OnBootFailure
	stableAfter = 30 * time.Second
	minBackoff  = 5 * time.Second
	maxBackoff  = 2 * time.Minute
)

// Supervise starts the gateway and keeps it running until ctx ends or Stop is called.
// When it cannot start or exits, the reason is logged and kept for Problem, and it is
// started again after a growing delay (5 s up to 2 min), at once when a configuration
// is deployed meanwhile (Reload). The control plane and its console keep running.
//
// onBootFailure (may be nil) is called with the number of consecutive early exits (the
// process started, then exited within stableAfter) and the version directory the
// process was started with ("" if unknown); it returns true when it switched the
// configuration, which is then tried at once.
func (o *OpenResty) Supervise(ctx context.Context, onBootFailure func(failures int, version string) bool) {
	o.mu.Lock()
	o.supervised = true
	if o.kick == nil {
		o.kick = make(chan struct{}, 1)
	}
	kick := o.kick
	o.mu.Unlock()
	backoff, failures := minBackoff, 0
	for {
		if ctx.Err() != nil || o.isStopping() {
			return
		}
		var reason, version string
		early := false
		if err := o.Start(); err != nil {
			reason = "cannot start the gateway: " + err.Error()
		} else {
			o.mu.Lock()
			exited, started := o.exited, o.started
			version = o.version
			o.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-exited:
			}
			if o.isStopping() {
				return
			}
			o.mu.Lock()
			exitErr := o.exitErr
			o.mu.Unlock()
			reason = o.exitReason(started, exitErr)
			if early = time.Since(started) < stableAfter; !early {
				backoff, failures = minBackoff, 0 // it ran fine for a while: a crash
			}
		}
		if early {
			failures++
		}
		o.mu.Lock()
		o.problem = reason
		o.mu.Unlock()
		slog.Error("GATEWAY DOWN: the admin console stays available; retrying", "reason", reason, "failed_starts", failures, "retry_in", backoff)
		if early && onBootFailure != nil && onBootFailure(failures, version) {
			// The switch reloaded (kicked) the stopped gateway: that request is served by
			// the next start. A deployment after this point kicks again and resets the count.
			select {
			case <-kick:
			default:
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-kick:
			backoff, failures = minBackoff, 0 // a new configuration: fresh attempts
		case <-time.After(backoff):
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

func (o *OpenResty) isStopping() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.stopping
}

// exitReason describes why the process that started at started exited: its serious
// log lines since then (once each), else the exit status.
func (o *OpenResty) exitReason(started time.Time, exitErr error) string {
	var msgs []string
	seen := map[string]bool{}
	o.mu.Lock()
	errs := o.errs
	o.mu.Unlock()
	if errs != nil {
		for _, l := range errs.since(started) {
			if m := logMessage(l); !seen[m] {
				seen[m] = true
				msgs = append(msgs, m)
			}
		}
	}
	if len(msgs) > 0 {
		return "the gateway exited: " + strings.Join(msgs, "; ")
	}
	if exitErr != nil {
		return "the gateway exited: " + exitErr.Error()
	}
	return "the gateway exited"
}

// Done is closed with the exit status when the gateway process exits.
func (o *OpenResty) Done() <-chan error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.done
}

// Reload asks the master to re-read the configuration (graceful, no dropped connections).
func (o *OpenResty) Reload() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cmd == nil {
		return nil // not started yet: it will load the current configuration when it starts
	}
	if !o.running {
		if o.supervised && !o.stopping {
			// down and waiting for a restart: start it now with the new configuration
			select {
			case o.kick <- struct{}{}:
			default:
			}
			return nil
		}
		return errors.New("gateway is not running")
	}
	o.reloadAt = time.Now()
	return o.cmd.Process.Signal(syscall.SIGHUP)
}

// Stop performs a graceful shutdown (SIGQUIT) and waits up to timeout. The gateway is
// not restarted afterwards.
func (o *OpenResty) Stop(timeout time.Duration) {
	o.mu.Lock()
	cmd, exited := o.cmd, o.exited
	if cmd != nil {
		o.stopping = true
	}
	o.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGQUIT)
	select {
	case <-exited:
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
	}
}

// WaitActive blocks until the gateway reports that version directory name is
// serving (new workers started after a reload), or the timeout expires.
func (o *OpenResty) WaitActive(ctx context.Context, name string, timeout time.Duration) error {
	if o.StatusURL == "" {
		return nil
	}
	o.mu.Lock()
	running := o.running
	o.mu.Unlock()
	if !running {
		return nil // not started, or down: it loads the current configuration when it starts
	}
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	deadline := time.Now().Add(timeout)
	serving := ""
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, o.StatusURL, nil)
		if resp, err := client.Do(req); err == nil {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
			resp.Body.Close()
			serving = strings.TrimSpace(string(b))
			if serving == name {
				o.clearUpstreamFailures() // they were made with the previous configuration
				return nil
			}
		}
		if time.Now().After(deadline) {
			if serving != "" {
				return &NotAcceptedError{Want: name, Serving: serving, Log: o.reloadErrors()}
			}
			return fmt.Errorf("gateway did not report version %s within %s", name, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// UpstreamFailures returns the latest upstream failure of each server block seen in the
// gateway's log since corvus-control started (why a host answers 502/504).
func (o *OpenResty) UpstreamFailures() []UpstreamFailure {
	o.mu.Lock()
	errs := o.errs
	o.mu.Unlock()
	if errs == nil {
		return nil
	}
	return errs.upstreamFailures()
}

// clearUpstreamFailures forgets the kept upstream failures: a new configuration serves,
// so a failure of the previous one (perhaps just fixed) is not shown any more. A
// target that still fails is reported again with the next request.
func (o *OpenResty) clearUpstreamFailures() {
	o.mu.Lock()
	errs := o.errs
	o.mu.Unlock()
	if errs != nil {
		errs.mu.Lock()
		errs.fails = nil
		errs.mu.Unlock()
	}
}

// reloadErrors returns the gateway's serious log lines since the last reload request.
func (o *OpenResty) reloadErrors() []string {
	o.mu.Lock()
	errs, at := o.errs, o.reloadAt
	o.mu.Unlock()
	if errs == nil {
		return nil
	}
	return errs.since(at)
}

// NotAcceptedError: after a reload the gateway kept serving another version, i.e. it
// rejected the new configuration at run time (for example a port that is already in
// use; `nginx -t` does not open sockets). Log holds the gateway's error lines since
// the reload, when there were any.
type NotAcceptedError struct {
	Want, Serving string
	Log           []string
}

func (e *NotAcceptedError) Error() string {
	if len(e.Log) == 0 {
		return fmt.Sprintf("the gateway kept the previous configuration (%s) instead of %s; see the gateway log (for example a port that is already in use)", e.Serving, e.Want)
	}
	var msgs []string // nginx repeats a failed bind() a few times: once each
	seen := map[string]bool{}
	for _, l := range e.Log {
		if m := logMessage(l); !seen[m] {
			seen[m] = true
			msgs = append(msgs, m)
		}
	}
	return fmt.Sprintf("the gateway kept the previous configuration (%s) instead of %s: %s", e.Serving, e.Want, strings.Join(msgs, "; "))
}

// logMessage trims an nginx error log line to its message:
// "2026/11/12 01:55:15 [emerg] 63#63: bind() to ... failed" → "bind() to ... failed".
func logMessage(line string) string {
	for _, lvl := range []string{"[emerg]", "[alert]", "[crit]"} {
		if i := strings.Index(line, lvl); i >= 0 {
			rest := strings.TrimSpace(line[i+len(lvl):])
			if j := strings.Index(rest, ": "); j > 0 && strings.Contains(rest[:j], "#") {
				rest = rest[j+2:]
			}
			return rest
		}
	}
	return line
}

// Noop is used when running the control plane without a gateway (development).
type Noop struct{}

func (Noop) Test(context.Context, string) (string, error) { return "test skipped (no gateway)", nil }
func (Noop) Reload() error                                { return nil }
