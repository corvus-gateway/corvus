package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/compiler"
	"corvus/control/internal/deploy"
	"corvus/control/internal/traffic"
)

// fakeGW accepts every configuration, except one containing "REJECT" (so tests can
// provoke config_rejected through advanced configuration).
type fakeGW struct{ reloads int }

func (f *fakeGW) Test(_ context.Context, conf string) (string, error) {
	b, err := os.ReadFile(conf)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(conf)
	hosts, _ := filepath.Glob(filepath.Join(dir, "hosts", "*.conf"))
	for _, h := range hosts {
		hb, _ := os.ReadFile(h)
		b = append(b, hb...)
	}
	if strings.Contains(string(b), "REJECT") {
		return "nginx: [emerg] unknown directive \"REJECT\"\nnginx: configuration file test failed", errRejected
	}
	return "", nil
}
func (f *fakeGW) Reload() error { f.reloads++; return nil }

type rejectErr struct{}

func (rejectErr) Error() string { return "exit status 1" }

var errRejected = rejectErr{}

// fullEnv is a testEnv whose server can deploy: a real store, compiler and deployer with
// a runner that accepts the configuration. Requests go through the whole handler chain
// (authentication, CSRF, role checks).
type fullEnv struct {
	*testEnv
	runner *fakeGW
	dir    string
}

func newFullEnv(t *testing.T) *fullEnv {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("deployments need POSIX symlinks; run on Linux")
	}
	e := newTestEnv(t)
	dir := t.TempDir()
	p := compiler.Paths{DataDir: filepath.ToSlash(dir), LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181, DefaultCert: "/data/certs/default"}
	r := &fakeGW{}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	e.s.Deployer = &deploy.Deployer{Store: e.s.Store, Compiler: &compiler.Compiler{P: p}, Runner: r, Dir: filepath.Join(dir, "nginx")}
	e.s.Paths = p
	e.s.DataKey = key
	e.s.Limiter = auth.NewLimiter(1000, time.Minute)
	e.s.AcctLimiter = auth.NewLimiter(1000, time.Minute)
	e.s.MFA = auth.NewMFAChallenges()
	if e.s.Traffic == nil {
		e.s.Traffic = traffic.New()
	}
	e.h = e.s.Handler()
	return &fullEnv{testEnv: e, runner: r, dir: dir}
}

// call sends an authenticated API request as the admin and returns the status code and
// the decoded JSON body (nil when the body is not a JSON object).
func (e *fullEnv) call(method, path string, body any) (int, map[string]any) {
	e.t.Helper()
	w, m := e.do(method, path, body, withCookie(e.cookie), withCSRF())
	return w.Code, m
}

// callList is call for endpoints that answer with a JSON array.
func (e *fullEnv) callList(method, path string) (int, []any) {
	e.t.Helper()
	w, _ := e.do(method, path, nil, withCookie(e.cookie), withCSRF())
	var l []any
	_ = json.Unmarshal(w.Body.Bytes(), &l)
	return w.Code, l
}

// must fails the test unless the request returns want.
func (e *fullEnv) must(want int, method, path string, body any) map[string]any {
	e.t.Helper()
	w, m := e.do(method, path, body, withCookie(e.cookie), withCSRF())
	if w.Code != want {
		e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	return m
}

// as returns a session cookie for a new user with the given role.
func (e *fullEnv) as(role string) string {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.s.Store.CreateUser(ctx, role+"@example.com", role, "x", role)
	if err != nil {
		e.t.Fatal(err)
	}
	raw := auth.RandomToken(32)
	if err := e.s.Store.CreateSession(ctx, auth.TokenHash(raw), u.ID, time.Hour, "127.0.0.1", "test"); err != nil {
		e.t.Fatal(err)
	}
	return raw
}

// id reads a numeric "id" from a decoded object. Mutations answer with the object under
// its kind (e.g. {"proxy_host": {...}, "version": {...}}).
func id(m map[string]any) int64 {
	if v, ok := m["id"].(float64); ok {
		return int64(v)
	}
	return 0
}

func TestHarnessDeploys(t *testing.T) {
	e := newFullEnv(t)
	m := e.must(201, "POST", "/api/proxy-hosts", map[string]any{
		"domains": []string{"app.example.com"}, "forward_scheme": "http", "forward_host": "app", "forward_port": 8080,
	})
	ph, _ := m["proxy_host"].(map[string]any)
	if id(ph) == 0 {
		t.Fatalf("no id in %v", m)
	}
	if e.runner.reloads == 0 {
		t.Fatal("no reload")
	}
}
