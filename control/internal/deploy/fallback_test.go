package deploy

import (
	"context"
	"errors"
	"testing"
)

// A safe-mode version is no earlier configuration to fall back to: the fallback skips it
// (and rollbacks to it) and takes the newest real configuration.
func TestFallbackSkipsSafeMode(t *testing.T) {
	d, _, _ := setup(t)
	ctx := context.Background()
	v1, err := d.Apply(ctx, nil, "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	sv, err := d.SafeMode(ctx, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Rollback(ctx, nil, sv.ID); err != nil { // a rollback to safe mode
		t.Fatal(err)
	}
	v2, err := d.Apply(ctx, nil, "second", addHost(ctx, ""))
	if err != nil {
		t.Fatal(err)
	}
	fv, err := d.Fallback(ctx, v2.Dir, "boot failure")
	if err != nil {
		t.Fatal(err)
	}
	if fv.RollbackOf == nil || *fv.RollbackOf != v1.ID || d.liveDir() != v1.Dir || IsSafeMode(fv) || !fv.Automatic {
		t.Fatalf("fallback %+v (live %s), want a rollback to #%d", fv, d.liveDir(), v1.ID)
	}
}

// A fallback for a configuration that is no longer live (an operator deployed meanwhile)
// is abandoned and leaves the new deployment in place.
func TestFallbackAbandonedWhenLiveChanged(t *testing.T) {
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
	if _, err := d.Fallback(ctx, v1.Dir, "boot failure"); !errors.Is(err, ErrLiveChanged) {
		t.Fatalf("fallback: %v", err)
	}
	if _, err := d.SafeMode(ctx, v1.Dir, "boot failure"); !errors.Is(err, ErrLiveChanged) {
		t.Fatalf("safe mode: %v", err)
	}
	if a, _ := d.Store.ActiveVersion(ctx); d.liveDir() != v2.Dir || a.ID != v2.ID {
		t.Fatalf("live %s, active %+v; want %s", d.liveDir(), a, v2.Dir)
	}
}

// An automatic fallback is not kept at the next start: the stored settings are deployed
// again. A rollback chosen by an operator is kept.
func TestEnsureCurrentRetriesAutomaticFallback(t *testing.T) {
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
	if _, err := d.Fallback(ctx, v2.Dir, "boot failure"); err != nil || d.liveDir() != v1.Dir {
		t.Fatalf("fallback: %v (live %s)", err, d.liveDir())
	}
	if !d.RollbackActive(ctx) {
		t.Fatal("the fallback must count as a rollback while it is active")
	}
	if err := d.EnsureCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := d.Store.ActiveVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a.Automatic || a.RollbackOf != nil || d.liveDir() != a.Dir || d.liveDir() == v1.Dir {
		t.Fatalf("after restart: active %+v, live %s", a, d.liveDir())
	}
	if dirty, _ := d.Dirty(ctx); dirty {
		t.Fatal("the stored settings must be deployed again")
	}

	// a rollback by the operator stays
	if _, err := d.Rollback(ctx, nil, v1.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	if a, _ := d.Store.ActiveVersion(ctx); d.liveDir() != v1.Dir || a.RollbackOf == nil || *a.RollbackOf != v1.ID {
		t.Fatalf("operator rollback not kept: active %+v, live %s", a, d.liveDir())
	}
}
