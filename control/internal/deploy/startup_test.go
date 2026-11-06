package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"corvus/control/internal/gateway"
	"corvus/control/internal/store"
)

func countVersions(t *testing.T, d *Deployer) int {
	t.Helper()
	var n int
	for id := int64(1); id < 100; id++ {
		if _, err := d.Store.VersionByID(context.Background(), id); err == nil {
			n++
		}
	}
	return n
}

// Restarting with unchanged settings keeps the active version instead of creating (and
// pruning) one per start; a rolled-back version is kept even when its settings revision
// matches the database (a rollback to a version rendered by older templates).
func TestEnsureCurrentKeepsVersions(t *testing.T) {
	d, _, _ := setup(t)
	ctx := context.Background()
	if err := d.EnsureCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	v1, err := d.Apply(ctx, nil, "add host", func(tx *store.Store) error {
		return tx.SaveProxyHost(ctx, &store.ProxyHost{Domains: []string{"a.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, Enabled: true})
	})
	if err != nil {
		t.Fatal(err)
	}
	n := countVersions(t, d)
	for i := 0; i < 3; i++ {
		if err := d.EnsureCurrent(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := countVersions(t, d); got != n {
		t.Fatalf("restarts created versions: %d → %d", n, got)
	}

	// re-render (as after an upgrade), then roll back to v1: same settings revision
	if _, err := d.Apply(ctx, nil, "startup re-render", nil); err != nil {
		t.Fatal(err)
	}
	rb, err := d.Rollback(ctx, nil, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	if a, _ := d.Store.ActiveVersion(ctx); a.ID != rb.ID {
		t.Fatalf("restart replaced the rollback: active %d, rollback %d", a.ID, rb.ID)
	}
	if !d.RollbackActive(ctx) {
		t.Fatal("RollbackActive = false")
	}
	if err := d.Refresh(ctx, "certificate issued/renewed"); err != nil {
		t.Fatal(err)
	}
	if a, _ := d.Store.ActiveVersion(ctx); a.ID != rb.ID {
		t.Fatal("a background refresh replaced the rollback")
	}
}

// rejectingRunner reloads, but the gateway keeps serving the previous version.
type rejectingRunner struct{ fakeRunner }

func (r *rejectingRunner) WaitActive(_ context.Context, name string, _ time.Duration) error {
	return &gateway.NotAcceptedError{Want: name, Serving: "previous"}
}

func TestReloadNotAccepted(t *testing.T) {
	d, _, _ := setup(t)
	ctx := context.Background()
	if err := d.EnsureCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Readlink(filepath.Join(d.Dir, "current"))
	d.Runner = &rejectingRunner{}
	_, err := d.Apply(ctx, nil, "add stream", func(tx *store.Store) error {
		return tx.SaveProxyHost(ctx, &store.ProxyHost{Domains: []string{"b.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, Enabled: true})
	})
	var na *gateway.NotAcceptedError
	if !errors.As(err, &na) {
		t.Fatalf("err = %v, want NotAcceptedError", err)
	}
	if after, _ := os.Readlink(filepath.Join(d.Dir, "current")); after != before {
		t.Fatalf("current link %q, want it back on %q", after, before)
	}
	if hs, _ := d.Store.ListProxyHosts(ctx); len(hs) != 0 {
		t.Fatal("the rejected change was committed")
	}
}
