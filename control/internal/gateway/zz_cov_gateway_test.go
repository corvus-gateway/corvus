//go:build linux

package gateway

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// writeScript writes an executable shell script that stands in for a real binary.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// fakeOpenResty: "-t ... -c conf" fails when the configuration contains "broken",
// prints a message when it contains "chatty", and is silent otherwise. Without -t it
// runs as a master: SIGHUP is recorded in $dir/hups, SIGQUIT exits.
const fakeOpenResty = `
dir=$(dirname "$0")
if [ "$1" = "-t" ]; then
	conf="$6"
	if grep -q broken "$conf"; then echo "nginx: [emerg] unknown directive \"broken\"" >&2; exit 1; fi
	if grep -q silentfail "$conf"; then exit 3; fi
	if grep -q chatty "$conf"; then echo "nginx: the configuration file $conf syntax is ok"; fi
	exit 0
fi
echo "$@" > "$dir/args"
trap 'echo hup >> "$dir/hups"' HUP
trap 'exit 0' QUIT
while true; do sleep 0.02; done
`

func TestCovOpenRestyTest(t *testing.T) {
	dir := t.TempDir()
	o := &OpenResty{Bin: writeScript(t, dir, "openresty", fakeOpenResty), Env: []string{"CORVUS_X=1"}}
	conf := func(body string) string {
		p := filepath.Join(t.TempDir(), "nginx.conf")
		_ = os.WriteFile(p, []byte(body), 0o644)
		return p
	}
	out, err := o.Test(context.Background(), conf("events {}"))
	if err != nil || out != "configuration test ok" {
		t.Fatalf("silent success: %q %v", out, err)
	}
	out, err = o.Test(context.Background(), conf("# chatty"))
	if err != nil || !strings.Contains(out, "syntax is ok") {
		t.Fatalf("chatty success: %q %v", out, err)
	}
	out, err = o.Test(context.Background(), conf("broken;"))
	if err == nil || !strings.Contains(out, `unknown directive "broken"`) {
		t.Fatalf("broken conf: %q %v", out, err)
	}
	out, err = o.Test(context.Background(), conf("silentfail"))
	if err == nil || !strings.Contains(out, "exit status 3") {
		t.Fatalf("silent failure must report the exit status: %q %v", out, err)
	}
	missing := &OpenResty{Bin: filepath.Join(dir, "does-not-exist")}
	if out, err := missing.Test(context.Background(), conf("")); err == nil || out == "" {
		t.Fatalf("missing binary: %q %v", out, err)
	}
	if env := o.env(); env[len(env)-1] != "CORVUS_X=1" {
		t.Fatalf("extra env must be appended: %v", env[len(env)-1])
	}
}

func TestCovOpenRestyLifecycle(t *testing.T) {
	dir := t.TempDir()
	o := &OpenResty{Bin: writeScript(t, dir, "openresty", fakeOpenResty), CurrentConf: "/etc/corvus/current/nginx.conf"}

	if running, _ := o.Status(); running {
		t.Fatal("not started yet")
	}
	if err := o.Reload(); err != nil {
		t.Fatalf("reload before start is a no-op: %v", err)
	}
	o.Stop(time.Second) // no process: returns at once

	before := time.Now()
	if err := o.Start(); err != nil {
		t.Fatal(err)
	}
	if err := o.Start(); err == nil {
		t.Fatal("second start must fail")
	}
	running, since := o.Status()
	if !running || since.Before(before) {
		t.Fatalf("status = %v %v", running, since)
	}
	// the master is started in the foreground with the current configuration
	waitFor(t, func() bool { b, _ := os.ReadFile(filepath.Join(dir, "args")); return len(b) > 0 })
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if got := strings.TrimSpace(string(args)); got != "-e stderr -c /etc/corvus/current/nginx.conf -g daemon off;" {
		t.Fatalf("args = %q", got)
	}
	if err := o.Reload(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		b, _ := os.ReadFile(filepath.Join(dir, "hups"))
		return strings.Count(string(b), "hup") == 1
	})

	stopStart := time.Now()
	o.Stop(5 * time.Second) // consumes the exit status itself
	if time.Since(stopStart) > 4*time.Second {
		t.Fatal("gateway did not exit on SIGQUIT")
	}
	if running, _ := o.Status(); running {
		t.Fatal("stopped gateway must not report running")
	}
	if err := o.Reload(); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("reload after exit = %v", err)
	}
}

func TestCovOpenRestyStopKillsAfterTimeout(t *testing.T) {
	dir := t.TempDir()
	// ignores SIGQUIT: Stop must fall back to SIGKILL
	bin := writeScript(t, dir, "openresty", "trap '' QUIT\nwhile true; do sleep 0.02; done\n")
	o := &OpenResty{Bin: bin, CurrentConf: "x"}
	if err := o.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // let the shell install its trap
	start := time.Now()
	o.Stop(200 * time.Millisecond)
	select {
	case err := <-o.Done():
		if err == nil {
			t.Fatal("a killed process exits with an error status")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("process was not killed")
	}
	if time.Since(start) < 200*time.Millisecond {
		t.Fatal("Stop must wait for the graceful timeout first")
	}
}

func TestCovOpenRestyStartMissingBinary(t *testing.T) {
	o := &OpenResty{Bin: filepath.Join(t.TempDir(), "nope")}
	if err := o.Start(); err == nil {
		t.Fatal("starting a missing binary must fail")
	}
	if running, _ := o.Status(); running {
		t.Fatal("not running")
	}
	if o.Done() != nil {
		t.Fatal("no done channel without a process")
	}
}

func TestCovWaitActive(t *testing.T) {
	ctx := context.Background()
	// no status URL / not started: nothing to wait for
	if err := (&OpenResty{}).WaitActive(ctx, "v1", time.Second); err != nil {
		t.Fatal(err)
	}
	var serving atomic.Value
	serving.Store("v1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(serving.Load().(string) + "\n"))
	}))
	defer srv.Close()
	o := &OpenResty{StatusURL: srv.URL}
	if err := o.WaitActive(ctx, "v2", time.Second); err != nil {
		t.Fatalf("not started: %v", err)
	}

	dir := t.TempDir()
	o.Bin = writeScript(t, dir, "openresty", fakeOpenResty)
	o.CurrentConf = "x"
	if err := o.Start(); err != nil {
		t.Fatal(err)
	}
	defer o.Stop(time.Second)

	if err := o.WaitActive(ctx, "v1", time.Second); err != nil {
		t.Fatalf("already serving: %v", err)
	}
	go func() { time.Sleep(120 * time.Millisecond); serving.Store("v2") }()
	if err := o.WaitActive(ctx, "v2", 3*time.Second); err != nil {
		t.Fatalf("switches after a while: %v", err)
	}
	err := o.WaitActive(ctx, "v3", 150*time.Millisecond)
	var na *NotAcceptedError
	if !errors.As(err, &na) || na.Serving != "v2" || na.Want != "v3" {
		t.Fatalf("kept old version: %v", err)
	}
	if !strings.Contains(na.Error(), "kept the previous configuration (v2) instead of v3") {
		t.Fatalf("message: %s", na.Error())
	}

	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := o.WaitActive(cctx, "v9", time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait = %v", err)
	}

	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	o.StatusURL = dead.URL
	if err := o.WaitActive(ctx, "v1", 100*time.Millisecond); err == nil || !strings.Contains(err.Error(), "did not report version v1") {
		t.Fatalf("unreachable status = %v", err)
	}
}

func TestCovNoop(t *testing.T) {
	var r Runner = Noop{}
	if out, err := r.Test(context.Background(), "x"); err != nil || !strings.Contains(out, "skipped") {
		t.Fatalf("noop test: %q %v", out, err)
	}
	if err := r.Reload(); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// listen reserves a loopback port that stands in for the server's socket (the fake
// binaries do not open sockets themselves).
func listen(t *testing.T) (net.Listener, string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return l, port
}

func TestCovRedisArgsAndLifecycle(t *testing.T) {
	bin := t.TempDir()
	work := filepath.Join(t.TempDir(), "redis-work")
	_, port := listen(t)
	// records its arguments and every start in its working directory
	script := writeScript(t, bin, "valkey-server", `echo "$@" > args
echo start >> starts
trap 'exit 0' TERM
while true; do sleep 0.02; done
`)
	r := &Redis{Bin: script, Port: port, ACLFile: "/etc/corvus/users.acl", Dir: work, MaxMemory: "64mb"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(work); err != nil || !st.IsDir() || st.Mode().Perm() != 0o700 {
		t.Fatalf("work dir: %v %v", st, err)
	}
	waitFor(t, func() bool { b, _ := os.ReadFile(filepath.Join(work, "args")); return len(b) > 0 })
	args, _ := os.ReadFile(filepath.Join(work, "args"))
	for _, want := range []string{"--bind 127.0.0.1", "--port " + port, "--aclfile /etc/corvus/users.acl", "--dir " + work, "--maxmemory 64mb", "--appendonly no", "--daemonize no"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("args %q lack %q", args, want)
		}
	}
	r.Stop(5 * time.Second)
	r.mu.Lock()
	exited := r.exited
	r.mu.Unlock()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("redis did not stop")
	}
	// after Stop nothing is restarted and spawn refuses
	if err := r.spawn(); err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("spawn after stop = %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	b, _ := os.ReadFile(filepath.Join(work, "starts"))
	if strings.Count(string(b), "start") != 1 {
		t.Fatalf("starts = %q", b)
	}
}

func TestCovRedisRestartsAfterCrash(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the 1 s restart backoff")
	}
	bin := t.TempDir()
	work := t.TempDir()
	_, port := listen(t)
	// first run crashes at once, later runs stay up
	script := writeScript(t, bin, "valkey-server", `echo start >> starts
if [ "$(wc -l < starts)" -lt 2 ]; then exit 1; fi
trap 'exit 0' TERM
while true; do sleep 0.02; done
`)
	r := &Redis{Bin: script, Port: port, ACLFile: "a", Dir: work, MaxMemory: "1mb"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		b, _ := os.ReadFile(filepath.Join(work, "starts"))
		if strings.Count(string(b), "start") >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("redis was not restarted: %q", b)
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.Stop(5 * time.Second)
}

func TestCovRedisSupervisorStopsWithContext(t *testing.T) {
	bin := t.TempDir()
	work := t.TempDir()
	_, port := listen(t)
	script := writeScript(t, bin, "valkey-server", "exit 1\n")
	r := &Redis{Bin: script, Port: port, ACLFile: "a", Dir: work, MaxMemory: "1mb"}
	ctx, cancel := context.WithCancel(context.Background())
	if err := r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	exited := r.exited
	r.mu.Unlock()
	<-exited // crashed; the supervisor now waits for the backoff
	cancel() // and must give up instead of restarting
	time.Sleep(1200 * time.Millisecond)
	r.mu.Lock()
	same := r.exited == exited
	r.mu.Unlock()
	if !same {
		t.Fatal("supervisor restarted redis after the context ended")
	}
}

func TestCovRedisStartErrors(t *testing.T) {
	// the working directory cannot be created below a file
	f := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(f, nil, 0o600)
	r := &Redis{Bin: "/bin/true", Port: "1", Dir: filepath.Join(f, "sub")}
	if err := r.Start(context.Background()); err == nil {
		t.Fatal("start with an unusable dir must fail")
	}
	r = &Redis{Bin: filepath.Join(t.TempDir(), "missing"), Port: "1", Dir: t.TempDir()}
	if err := r.Start(context.Background()); err == nil {
		t.Fatal("start with a missing binary must fail")
	}
	(&Redis{}).Stop(time.Second) // never started: no-op
}

func TestCovRedisStopKillsAfterTimeout(t *testing.T) {
	bin := t.TempDir()
	_, port := listen(t)
	script := writeScript(t, bin, "valkey-server", "trap '' TERM\nwhile true; do sleep 0.02; done\n")
	r := &Redis{Bin: script, Port: port, ACLFile: "a", Dir: t.TempDir(), MaxMemory: "1mb"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	r.Stop(150 * time.Millisecond)
	r.mu.Lock()
	exited := r.exited
	r.mu.Unlock()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("redis ignoring SIGTERM was not killed")
	}
}

func TestCovWaitTCP(t *testing.T) {
	_, port := listen(t)
	if err := WaitTCP(context.Background(), "127.0.0.1:"+port, time.Second); err != nil {
		t.Fatal(err)
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	if err := WaitTCP(context.Background(), addr, 150*time.Millisecond); err == nil {
		t.Fatal("closed port must time out")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := WaitTCP(ctx, addr, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled = %v", err)
	}
}
