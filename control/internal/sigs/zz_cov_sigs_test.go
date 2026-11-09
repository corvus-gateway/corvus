package sigs

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"corvus/control/internal/store"
)

type covEnv struct {
	s    *Service
	st   *store.Store
	priv ed25519.PrivateKey
}

func newCovEnv(t *testing.T) *covEnv {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	old := PublicKey
	PublicKey = base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { PublicKey = old })
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Service{Dir: filepath.Join(dir, "rules"), Store: st}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	return &covEnv{s: s, st: st, priv: priv}
}

func (e *covEnv) bundle(t *testing.T, serialDelta int64, version string) ([]byte, []byte) {
	t.Helper()
	b, _ := parse(defaultBundle)
	b.Serial += serialDelta
	b.Version = version
	data, _ := json.Marshal(b)
	return data, []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(e.priv, data)))
}

func (e *covEnv) setSettings(t *testing.T, set Settings) {
	t.Helper()
	b, _ := json.Marshal(set)
	if err := e.st.SetMeta(context.Background(), "managed_rules", string(b)); err != nil {
		t.Fatal(err)
	}
}

func TestCovNeedsExempt(t *testing.T) {
	with := Template{Groups: [][]store.Condition{{{Field: "ip.src", Operator: "in", Value: []string{"1.2.3.4"}}}, {{Field: "ip.src", Operator: "not_in", Value: []string{"{exempt}"}}}}}
	without := Template{Groups: [][]store.Condition{{{Field: "http.path", Operator: "eq", Value: []string{"/x"}}}}}
	if !with.NeedsExempt() || without.NeedsExempt() || (Template{}).NeedsExempt() {
		t.Fatal("NeedsExempt")
	}
	// long names/descriptions are truncated; CVE prefixes the name
	long := Template{CVEs: []string{"CVE-2024-1"}, Title: map[string]string{"en": strings.Repeat("é", 150)}, Description: map[string]string{"en": strings.Repeat("x", 600)}, Groups: with.Groups}
	r := long.Rule("block", 403, "all", nil, 0)
	if len([]rune(r.Name)) != 100 || !strings.HasPrefix(r.Name, "[CVE-2024-1] ") || len(r.Description) != 480 {
		t.Fatalf("rule name/desc: %d %q %d", len([]rune(r.Name)), r.Name[:20], len(r.Description))
	}
	if len(r.Expression.Groups) != 1 {
		t.Fatalf("exempt-only group must be dropped without a list: %+v", r.Expression.Groups)
	}
}

func TestCovParseRejects(t *testing.T) {
	cases := map[string]string{
		"not json":          `{`,
		"unknown field":     `{"serial":1,"version":"v","extra":1}`,
		"no serial":         `{"serial":0,"version":"v"}`,
		"bad signature id":  `{"serial":1,"version":"v","signatures":[{"id":"BAD","pattern":"x"}]}`,
		"duplicate sig":     `{"serial":1,"version":"v","signatures":[{"id":"a","pattern":"x"},{"id":"a","pattern":"y"}]}`,
		"bad options":       `{"serial":1,"version":"v","signatures":[{"id":"a","pattern":"x","options":"zz"}]}`,
		"template no title": `{"serial":1,"version":"v","templates":[{"id":"t","groups":[[{"field":"a","operator":"eq","value":["b"]}]]}]}`,
		"template no group": `{"serial":1,"version":"v","templates":[{"id":"t","title":{"en":"x"}}]}`,
	}
	for name, in := range cases {
		if _, err := parse([]byte(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := parse([]byte(`{"serial":1,"version":"v","signatures":[{"id":"a","pattern":"x","options":"jo"}]}`)); err != nil {
		t.Fatal(err)
	}
}

func TestCovInitWithInvalidInstalledBundle(t *testing.T) {
	e := newCovEnv(t)
	data, sig := e.bundle(t, 1, "v-new")
	_ = os.MkdirAll(e.s.Dir, 0o750)
	_ = os.WriteFile(filepath.Join(e.s.Dir, "bundle.json"), data, 0o640)
	_ = os.WriteFile(filepath.Join(e.s.Dir, "bundle.json.sig"), []byte("bm9wZQ=="), 0o640)
	s := &Service{Dir: e.s.Dir, Store: e.st}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if st := s.Status(context.Background()); st.Source != "builtin" || st.InstalledAt != nil {
		t.Fatalf("badly signed bundle must be ignored: %+v", st)
	}
	_ = os.WriteFile(filepath.Join(e.s.Dir, "bundle.json.sig"), sig, 0o640)
	s = &Service{Dir: e.s.Dir, Store: e.st}
	_ = s.Init()
	if st := s.Status(context.Background()); st.Source != "installed" || st.Version != "v-new" || st.InstalledAt == nil {
		t.Fatalf("signed bundle: %+v", st)
	}
}

func TestCovSettingsEnvFallback(t *testing.T) {
	e := newCovEnv(t)
	t.Setenv("CORVUS_RULES_URL", "https://rules.example/bundle.json")
	if set := e.s.Settings(context.Background()); set.URL != "https://rules.example/bundle.json" || set.AutoUpdate {
		t.Fatalf("env fallback: %+v", set)
	}
	e.setSettings(t, Settings{URL: "https://mirror.example/b.json", AutoUpdate: true})
	if set := e.s.Settings(context.Background()); set.URL != "https://mirror.example/b.json" || !set.AutoUpdate {
		t.Fatalf("stored: %+v", set)
	}
}

func TestCovCheck(t *testing.T) {
	e := newCovEnv(t)
	ctx := context.Background()
	t.Setenv("CORVUS_RULES_URL", "")

	if _, err := e.s.Check(ctx); err == nil || !strings.Contains(err.Error(), "no update URL") {
		t.Fatalf("no url = %v", err)
	}
	e.setSettings(t, Settings{URL: "http://insecure.example/b.json"})
	if _, err := e.s.Check(ctx); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("http url = %v", err)
	}

	data, sig := e.bundle(t, 5, "v-remote")
	var sigStatus atomic.Int32
	sigStatus.Store(200)
	var ua atomic.Value
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua.Store(r.Header.Get("User-Agent"))
		switch r.URL.Path {
		case "/b.json":
			_, _ = w.Write(data)
		case "/b.json.sig":
			w.WriteHeader(int(sigStatus.Load()))
			_, _ = w.Write(sig)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	e.s.Client = srv.Client()
	updated := 0
	e.s.OnUpdated = func(context.Context) { updated++ }

	// missing bundle
	e.setSettings(t, Settings{URL: srv.URL + "/missing.json"})
	st, err := e.s.Check(ctx)
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") || st.Error == "" || st.CheckedAt == nil {
		t.Fatalf("missing bundle: %+v %v", st, err)
	}

	// signature download fails
	e.setSettings(t, Settings{URL: srv.URL + "/b.json"})
	sigStatus.Store(500)
	if _, err := e.s.Check(ctx); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("sig failure = %v", err)
	}

	// installs the newer bundle
	sigStatus.Store(200)
	st, err = e.s.Check(ctx)
	if err != nil || st.Version != "v-remote" || st.Source != "installed" || st.Error != "" || updated != 1 {
		t.Fatalf("check install: %+v %v updated=%d", st, err, updated)
	}
	if ua.Load() != "corvus-awpg" {
		t.Fatalf("UA %v", ua.Load())
	}
	// same bundle again: not newer is not an error
	st, err = e.s.Check(ctx)
	if err != nil || st.Error != "" || updated != 1 {
		t.Fatalf("not newer: %+v %v", st, err)
	}

	// invalid URL for the request
	if _, err := e.s.get(ctx, "https://\x7f"); err == nil {
		t.Fatal("invalid url must fail")
	}
	// default client is used when none is set (transport error to a closed server)
	closed := httptest.NewTLSServer(http.NotFoundHandler())
	closed.Close()
	if _, err := (&Service{}).get(ctx, closed.URL); err == nil {
		t.Fatal("closed server must fail")
	}
}

func TestCovRun(t *testing.T) {
	e := newCovEnv(t)
	t.Setenv("CORVUS_RULES_URL", "")
	data, sig := e.bundle(t, 3, "v-run")
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if strings.HasSuffix(r.URL.Path, ".sig") {
			_, _ = w.Write(sig)
			return
		}
		_, _ = w.Write(data)
	}))
	defer srv.Close()
	e.s.Client = srv.Client()

	// auto-update off: no request
	e.setSettings(t, Settings{URL: srv.URL + "/b.json"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.s.Run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	if hits.Load() != 0 {
		t.Fatal("auto-update is off")
	}

	// auto-update on: checks at start
	e.setSettings(t, Settings{URL: srv.URL + "/b.json", AutoUpdate: true})
	ctx, cancel = context.WithCancel(context.Background())
	done = make(chan struct{})
	go func() { e.s.Run(ctx); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	for e.s.Bundle().Version != "v-run" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if e.s.Bundle().Version != "v-run" {
		t.Fatal("Run did not install the update")
	}

	// failing check is logged, not fatal
	e.setSettings(t, Settings{URL: srv.URL + "/b.json", AutoUpdate: true})
	srv.Close()
	ctx, cancel = context.WithCancel(context.Background())
	done = make(chan struct{})
	go func() { e.s.Run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
}

func TestCovInstallWriteErrors(t *testing.T) {
	e := newCovEnv(t)
	data, sig := e.bundle(t, 1, "v")
	f := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(f, nil, 0o600)
	e.s.Dir = filepath.Join(f, "sub")
	if _, err := e.s.Install(context.Background(), data, sig); err == nil {
		t.Fatal("install into an unusable dir must fail")
	}
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "bundle.json.tmp", "x"), 0o750)
	_ = os.MkdirAll(filepath.Join(dir, "bundle.json.sig.tmp", "x"), 0o750)
	e.s.Dir = dir
	if _, err := e.s.Install(context.Background(), data, sig); err == nil {
		t.Fatal("unwritable temp file must fail")
	}
	dir2 := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir2, "bundle.json", "x"), 0o750)
	_ = os.MkdirAll(filepath.Join(dir2, "bundle.json.sig", "x"), 0o750)
	e.s.Dir = dir2
	if _, err := e.s.Install(context.Background(), data, sig); err == nil {
		t.Fatal("rename onto a directory must fail")
	}
	if e.s.Bundle().Version == "v" {
		t.Fatal("failed install must not activate the bundle")
	}
	// bad JSON with a valid signature
	bad := []byte(`{"serial":0}`)
	if _, err := e.s.Install(context.Background(), bad, []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(e.priv, bad)))); err == nil {
		t.Fatal("invalid bundle accepted")
	}
}
