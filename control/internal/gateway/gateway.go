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

	mu       sync.Mutex
	cmd      *exec.Cmd
	done     chan error
	started  time.Time
	errs     *errLog   // recent serious lines of the gateway's stderr
	reloadAt time.Time // when the last reload was requested
}

// Status reports whether the gateway process is running and since when.
func (o *OpenResty) Status() (bool, time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cmd != nil && o.cmd.ProcessState == nil, o.started
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
func (o *OpenResty) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cmd != nil {
		return errors.New("gateway already started")
	}
	cmd := exec.Command(o.Bin, "-e", "stderr", "-c", o.CurrentConf, "-g", "daemon off;") // -e: no write to the read-only prefix
	cmd.Env = o.env()
	o.errs = newErrLog(os.Stderr)
	cmd.Stdout, cmd.Stderr = os.Stdout, o.errs
	if err := cmd.Start(); err != nil {
		return err
	}
	o.cmd = cmd
	o.started = time.Now()
	o.done = make(chan error, 1)
	go func() { o.done <- cmd.Wait() }()
	slog.Info("gateway started", "pid", cmd.Process.Pid, "conf", o.CurrentConf)
	return nil
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
	if o.cmd.ProcessState != nil {
		return errors.New("gateway is not running")
	}
	o.reloadAt = time.Now()
	return o.cmd.Process.Signal(syscall.SIGHUP)
}

// Stop performs a graceful shutdown (SIGQUIT) and waits up to timeout.
func (o *OpenResty) Stop(timeout time.Duration) {
	o.mu.Lock()
	cmd, done := o.cmd, o.done
	o.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGQUIT)
	select {
	case <-done:
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
	started := o.cmd != nil
	o.mu.Unlock()
	if !started {
		return nil
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
