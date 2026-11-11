package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/gateway"
	"corvus/control/internal/store"
)

// flakyRunner accepts every configuration; Reload fails while failReload is set and
// WaitActive returns waitErr.
type flakyRunner struct {
	fakeRunner
	failReload bool
	waitErr    error
	waits      int
	timeout    time.Duration // of the last WaitActive
}

func (f *flakyRunner) Reload() error {
	f.reloads++
	if f.failReload {
		return errors.New("signal failed")
	}
	return nil
}

func (f *flakyRunner) WaitActive(_ context.Context, _ string, timeout time.Duration) error {
	f.timeout = timeout
	f.waits++
	return f.waitErr
}

func addHost(ctx context.Context, advanced string) func(tx *store.Store) error {
	return func(tx *store.Store) error {
		return tx.SaveProxyHost(ctx, &store.ProxyHost{Domains: []string{"cov.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, Enabled: true, AdvancedConfig: advanced})
	}
}

func TestCovSmallHelpers(t *testing.T) {
	e := &TestError{VersionID: 3, Output: "nginx: [emerg] x"}
	if e.Error() != "configuration rejected: nginx: [emerg] x" {
		t.Fatal(e.Error())
	}
	d := &Deployer{Dir: "/data/nginx"}
	if got := d.CurrentConf(); got != "/data/nginx/current/nginx.conf" {
		t.Fatalf("CurrentConf = %q", got)
	}
}

func TestCovRollbackErrors(t *testing.T) {
	d, _, _ := setup(t)
	ctx := context.Background()
	if _, err := d.Rollback(ctx, nil, 99); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown version: %v", err)
	}
	v1, err := d.Apply(ctx, nil, "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	// a rejected deployment has no files to go back to
	_, err = d.Apply(ctx, nil, "bad", addHost(ctx, "REJECT;"))
	var te *TestError
	if !errors.As(err, &te) || te.VersionID == 0 {
		t.Fatalf("rejected apply: %v", err)
	}
	if _, err := d.Rollback(ctx, nil, te.VersionID); err == nil || !strings.Contains(err.Error(), "never deployed") {
		t.Fatalf("rollback to a failed version: %v", err)
	}
	v2, err := d.Apply(ctx, nil, "second", addHost(ctx, ""))
	if err != nil {
		t.Fatal(err)
	}
	// the files of v1 are rejected now (e.g. a removed certificate)
	conf := filepath.Join(d.versionsDir(), v1.Dir, "nginx.conf")
	b, _ := os.ReadFile(conf)
	_ = os.WriteFile(conf, append(b, []byte("\n# REJECT\n")...), 0o644)
	if _, err := d.Rollback(ctx, nil, v1.ID); !errors.As(err, &te) {
		t.Fatalf("rollback to a version that no longer passes the test: %v", err)
	}
	// pruned files
	_ = os.RemoveAll(filepath.Join(d.versionsDir(), v1.Dir))
	if _, err := d.Rollback(ctx, nil, v1.ID); err == nil || !strings.Contains(err.Error(), "pruned") {
		t.Fatalf("rollback to pruned files: %v", err)
	}
	if link, _ := os.Readlink(d.currentLink()); !strings.HasSuffix(link, v2.Dir) {
		t.Fatalf("current must stay on v2: %s", link)
	}
}

func TestCovReloadFailureRestoresLink(t *testing.T) {
	d, _, _ := setup(t)
	ctx := context.Background()
	if _, err := d.Apply(ctx, nil, "first", nil); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Readlink(d.currentLink())
	fr := &flakyRunner{failReload: true}
	d.Runner = fr
	_, err := d.Apply(ctx, nil, "second", addHost(ctx, ""))
	if err == nil || !strings.Contains(err.Error(), "reload failed") {
		t.Fatalf("apply with failing reload: %v", err)
	}
	if after, _ := os.Readlink(d.currentLink()); after != before {
		t.Fatalf("link %q, want restored %q", after, before)
	}
	if hs, _ := d.Store.ListProxyHosts(ctx); len(hs) != 0 {
		t.Fatal("the change must be rolled back")
	}
	// a confirmation timeout (not a rejection) still counts as deployed
	fr.failReload, fr.waitErr = false, errors.New("timed out")
	v, err := d.Apply(ctx, nil, "third", addHost(ctx, ""))
	if err != nil || v.Status != "active" || fr.waits == 0 || fr.timeout != 15*time.Second {
		t.Fatalf("apply with slow confirmation: %+v %v", v, err)
	}
}

func TestCovRefreshAndDirty(t *testing.T) {
	d, _, _ := setup(t)
	ctx := context.Background()
	if dirty, err := d.Dirty(ctx); err != nil || !dirty {
		t.Fatalf("nothing deployed yet is dirty: %v %v", dirty, err)
	}
	v1, err := d.Apply(ctx, nil, "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	if dirty, _ := d.Dirty(ctx); dirty {
		t.Fatal("freshly deployed is clean")
	}
	if _, err := d.Store.BumpConfigRev(ctx); err != nil {
		t.Fatal(err)
	}
	if dirty, _ := d.Dirty(ctx); !dirty {
		t.Fatal("a newer settings revision is dirty")
	}

	fr := &flakyRunner{}
	d.Runner = fr
	if err := d.Refresh(ctx, "certificate issued"); err != nil {
		t.Fatal(err)
	}
	if n := countVersions(t, d); n != 2 {
		t.Fatalf("refresh redeploys: %d versions", n)
	}
	// a failing redeploy still reloads the gateway for renewed files
	fr.reloads = 0
	if err := d.Store.SaveProxyHost(ctx, &store.ProxyHost{Domains: []string{"bad.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, Enabled: true, AdvancedConfig: "REJECT;"}); err != nil {
		t.Fatal(err)
	}
	fr.failReload = true // and its failure is only logged
	if err := d.Refresh(ctx, "geoip"); err == nil {
		t.Fatal("rejected redeploy must be reported")
	}
	if fr.reloads != 1 {
		t.Fatalf("reloads after failed redeploy = %d", fr.reloads)
	}
	fr.failReload = false
	hs, _ := d.Store.ListProxyHosts(ctx)
	_ = d.Store.DeleteProxyHost(ctx, hs[0].ID)

	// during a rollback only the gateway is reloaded
	if _, err := d.Rollback(ctx, nil, v1.ID); err != nil {
		t.Fatal(err)
	}
	if !d.RollbackActive(ctx) {
		t.Fatal("rollback must be active")
	}
	before := countVersions(t, d)
	fr.reloads = 0
	if err := d.Refresh(ctx, "certificate renewed"); err != nil {
		t.Fatal(err)
	}
	if fr.reloads != 1 || countVersions(t, d) != before {
		t.Fatalf("rollback refresh: reloads=%d versions %d→%d", fr.reloads, before, countVersions(t, d))
	}
}

func TestCovEnsureCurrentRerenderRejected(t *testing.T) {
	d, _, _ := setup(t)
	ctx := context.Background()
	v1, err := d.Apply(ctx, nil, "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	// settings now render differently and are rejected: keep v1
	if err := d.Store.SaveProxyHost(ctx, &store.ProxyHost{Domains: []string{"x.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, Enabled: true, AdvancedConfig: "REJECT;"}); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(d.currentLink())
	if err := d.EnsureCurrent(ctx); err != nil {
		t.Fatalf("startup with a rejected re-render keeps the previous version: %v", err)
	}
	if link, _ := os.Readlink(d.currentLink()); !strings.HasSuffix(link, v1.Dir) {
		t.Fatalf("current = %s, want %s", link, v1.Dir)
	}
	// without the files of the active version the rejection is fatal
	_ = os.RemoveAll(filepath.Join(d.versionsDir(), v1.Dir))
	if err := d.EnsureCurrent(ctx); err == nil {
		t.Fatal("no usable configuration must be reported")
	}
}

func TestCovSameTree(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	write := func(dir, name, s string) {
		p := filepath.Join(dir, name)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(s), 0o644)
	}
	write(a, "nginx.conf", "include "+a+"/x.conf;")
	write(b, "nginx.conf", "include "+b+"/x.conf;")
	write(a, "sub/x.conf", "x")
	write(b, "sub/x.conf", "x")
	if !sameTree(a, b) {
		t.Fatal("trees that differ only in their own path are the same")
	}
	write(b, "extra.conf", "")
	if sameTree(a, b) {
		t.Fatal("an extra file differs")
	}
	_ = os.Remove(filepath.Join(b, "extra.conf"))
	write(a, "only-a.conf", "")
	if sameTree(a, b) {
		t.Fatal("a missing file differs")
	}
	if sameTree(filepath.Join(a, "missing"), b) {
		t.Fatal("a missing tree differs")
	}
}

// A reload the gateway did not take (Apply or Rollback) is recorded as a failed version,
// returned as a ReloadError with that id, and the link points back to the serving version.
func TestCovReloadNotAccepted(t *testing.T) {
	d, _, _ := setup(t)
	ctx := context.Background()
	v1, err := d.Apply(ctx, nil, "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := d.Apply(ctx, nil, "second", addHost(ctx, ""))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.Readlink(d.currentLink())
	d.Runner = &flakyRunner{waitErr: &gateway.NotAcceptedError{Want: "x", Serving: v2.Dir}}
	check := func(what string, err error, msg string) {
		t.Helper()
		var re *ReloadError
		var na *gateway.NotAcceptedError
		if !errors.As(err, &re) || re.VersionID == 0 || !errors.As(err, &na) || re.Error() != na.Error() {
			t.Fatalf("%s: %v", what, err)
		}
		fv, err := d.Store.VersionByID(ctx, re.VersionID)
		if err != nil || fv.Status != "failed" || fv.Message != msg || fv.TestOutput != na.Error() {
			t.Fatalf("%s: failed version %+v %v", what, fv, err)
		}
		if after, _ := os.Readlink(d.currentLink()); after != before {
			t.Fatalf("%s: link %q, want %q", what, after, before)
		}
	}
	_, err = d.Apply(ctx, nil, "third", nil)
	check("apply", err, "third")
	_, err = d.Rollback(ctx, nil, v1.ID)
	check("rollback", err, fmt.Sprintf("rollback to #%d", v1.ID))
	if av, err := d.Store.ActiveVersion(ctx); err != nil || av.ID != v2.ID {
		t.Fatalf("active version = %+v %v, want #%d", av, err, v2.ID)
	}
}
