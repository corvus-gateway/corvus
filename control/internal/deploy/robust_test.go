package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/store"
)

// hookRunner accepts every configuration and runs a hook during the test and during the
// reload confirmation (where a console client may disconnect).
type hookRunner struct {
	fakeRunner
	onTest, onWait func()
}

func (h *hookRunner) Test(ctx context.Context, conf string) (string, error) {
	if h.onTest != nil {
		h.onTest()
	}
	out, err := h.fakeRunner.Test(ctx, conf)
	if err == nil && ctx.Err() != nil {
		// like `openresty -t` started with exec.CommandContext
		return "signal: killed", errors.New("configuration test failed")
	}
	return out, err
}

func (h *hookRunner) WaitActive(ctx context.Context, _ string, _ time.Duration) error {
	if h.onWait != nil {
		h.onWait()
	}
	return ctx.Err()
}

// A client that disconnects while the deployment runs must not leave the database and
// the gateway disagreeing, nor record a failed version.
func TestApplyOutlivesCancelledRequest(t *testing.T) {
	for _, during := range []string{"test", "wait"} {
		t.Run(during, func(t *testing.T) {
			d, _, _ := setup(t)
			if err := d.EnsureCurrent(context.Background()); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			hr := &hookRunner{}
			if during == "test" {
				hr.onTest = cancel
			} else {
				hr.onWait = cancel
			}
			d.Runner = hr
			v, err := d.Apply(ctx, nil, "add host", addHost(ctx, ""))
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			bg := context.Background()
			if hs, _ := d.Store.ListProxyHosts(bg); len(hs) != 1 {
				t.Fatal("the change was not committed")
			}
			active, err := d.Store.ActiveVersion(bg)
			if err != nil || active.ID != v.ID || active.Status != "active" {
				t.Fatalf("active version %+v %v, want %d", active, err, v.ID)
			}
			if d.liveDir() != v.Dir {
				t.Fatalf("live %q, want %q", d.liveDir(), v.Dir)
			}
			vs, _ := d.Store.ListVersions(bg, 50, 0)
			for _, x := range vs {
				if x.Status == "failed" {
					t.Fatalf("a failed version was recorded: %+v", x)
				}
			}
		})
	}
}

func TestRollbackOutlivesCancelledRequest(t *testing.T) {
	d, _, _ := setup(t)
	bg := context.Background()
	v1, err := d.Apply(bg, nil, "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Apply(bg, nil, "second", addHost(bg, "")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(bg)
	defer cancel()
	d.Runner = &hookRunner{onWait: cancel}
	rb, err := d.Rollback(ctx, nil, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := d.Store.ActiveVersion(bg); a.ID != rb.ID || d.liveDir() != v1.Dir {
		t.Fatalf("active %d (want %d), live %s", a.ID, rb.ID, d.liveDir())
	}
}

// A rolled-back deployment may have left the gateway on a directory whose id the
// database hands out again: it must be neither overwritten nor removed.
func TestApplyDoesNotReuseLiveDir(t *testing.T) {
	ctx := context.Background()
	// what a rolled-back attempt leaves: the next id on disk and live, without a row
	orphaned := func(t *testing.T) (*Deployer, string) {
		d, _, _ := setup(t)
		v1, err := d.Apply(ctx, nil, "first", nil)
		if err != nil {
			t.Fatal(err)
		}
		orphan := filepath.Join(d.versionsDir(), "v000002")
		if err := os.CopyFS(orphan, os.DirFS(filepath.Join(d.versionsDir(), v1.Dir))); err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(orphan, "marker")
		_ = os.WriteFile(marker, []byte("live"), 0o644)
		if err := d.swapLink("v000002"); err != nil {
			t.Fatal(err)
		}
		return d, marker
	}

	// deployed: the new version skips the id instead of rendering over the live files
	d, _ := orphaned(t)
	v, err := d.Apply(ctx, nil, "good", addHost(ctx, ""))
	if err != nil {
		t.Fatal(err)
	}
	if v.Dir != "v000003" || d.liveDir() != v.Dir {
		t.Fatalf("new version %s, live %s; want v000003", v.Dir, d.liveDir())
	}
	if !strings.Contains(readCurrent(t, d, "hosts/1.conf"), "cov.example.com") {
		t.Fatal("new configuration not live")
	}
	if _, err := d.Store.VersionByID(ctx, 2); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("skipped id must not stay recorded: %v", err)
	}

	// rejected: the live directory must survive the cleanup of the attempt
	d, marker := orphaned(t)
	if _, err := d.Apply(ctx, nil, "bad", addHost(ctx, "REJECT;")); err == nil {
		t.Fatal("REJECT must be rejected")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("the live directory was removed:", err)
	}
	if d.liveDir() != "v000002" {
		t.Fatal("live link moved")
	}
}

// After a restore whose settings are rejected and whose version directories are gone,
// startup falls back to the newest earlier version that still works, and with none
// left to the safe configuration.
func TestFallbackAndSafeMode(t *testing.T) {
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
	_ = os.RemoveAll(filepath.Join(d.versionsDir(), v2.Dir))
	h, _ := d.Store.ProxyHostByID(ctx, 1)
	h.AdvancedConfig = "REJECT;"
	if err := d.Store.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureCurrent(ctx); err == nil {
		t.Fatal("EnsureCurrent must fail: rejected settings, no files of the active version")
	}
	fv, err := d.Fallback(ctx, "", "startup deployment failed")
	if err != nil {
		t.Fatal(err)
	}
	if fv.RollbackOf == nil || *fv.RollbackOf != v1.ID || d.liveDir() != v1.Dir || !strings.Contains(fv.Message, "fallback to #") || !fv.Automatic {
		t.Fatalf("fallback %+v, live %s", fv, d.liveDir())
	}

	// the gateway did not start with v1 either: nothing earlier is left → safe mode
	sv, err := d.Fallback(ctx, d.liveDir(), "boot failure")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sv.Message, "safe mode") || d.liveDir() != sv.Dir || !IsSafeMode(sv) || !sv.Automatic {
		t.Fatalf("safe mode %+v, live %s", sv, d.liveDir())
	}
	if _, err := os.Stat(filepath.Join(d.Dir, "current", "nginx.conf")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d.Dir, "current", "hosts", "1.conf")); err == nil {
		t.Fatal("safe mode must not contain the stored hosts")
	}
	if dirty, _ := d.Dirty(ctx); !dirty {
		t.Fatal("the stored settings must show as not deployed")
	}
	// the next restart keeps safe mode while the settings stay rejected
	if err := d.EnsureCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	if d.liveDir() != sv.Dir {
		t.Fatal("restart left safe mode for a rejected configuration")
	}
}

// Old version rows are pruned with their directories; the active version and rollback
// targets stay.
func TestPruneVersionRows(t *testing.T) {
	d, _, _ := setup(t)
	d.Keep = 3
	ctx := context.Background()
	first, err := d.Apply(ctx, nil, "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	var v1 *store.ConfigVersion // the rollback target: the last of these
	for i := 0; i < 5; i++ {
		if v1, err = d.Apply(ctx, nil, "again", nil); err != nil {
			t.Fatal(err)
		}
		_, _ = d.Apply(ctx, nil, "bad", addHost(ctx, "REJECT;"))
	}
	if _, err := d.Store.VersionByID(ctx, first.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("old version row kept: %v", err)
	}
	if _, err := d.Rollback(ctx, nil, v1.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := d.Apply(ctx, nil, "more", nil); err != nil {
			t.Fatal(err)
		}
	}
	vs, _ := d.Store.ListVersions(ctx, 100, 0)
	var ok, failed int
	for _, v := range vs {
		if v.Status == "failed" {
			failed++
		} else {
			ok++
		}
	}
	// the 3 newest deployed (2 more, the rollback) + v1, which the rollback points to;
	// the 3 newest failed attempts
	if ok != 4 || failed != 3 {
		t.Fatalf("rows kept: %d deployed, %d failed", ok, failed)
	}
	if _, err := d.Store.VersionByID(ctx, v1.ID); err != nil {
		t.Fatal("rollback target pruned:", err)
	}
}
