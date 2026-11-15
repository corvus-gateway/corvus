package acme

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"corvus/control/internal/store"
)

func failStateWrites(t *testing.T, failing *atomic.Bool, calls *atomic.Int32) {
	t.Helper()
	old, oldWait := updateState, stateRetryWait
	stateRetryWait = time.Millisecond
	updateState = func(ctx context.Context, st *store.Store, id int64, status, lastError string, domains []string, notAfter time.Time) error {
		calls.Add(1)
		if failing.Load() {
			return errors.New("database is locked")
		}
		return old(ctx, st, id, status, lastError, domains, notAfter)
	}
	t.Cleanup(func() { updateState, stateRetryWait = old, oldWait })
}

// An issued certificate whose state could not be recorded is still deployed, and the
// next attempt records it from disk instead of ordering a duplicate.
func TestIssuedButNotRecordedIsNotOrderedAgain(t *testing.T) {
	s, f, issued := acmeEnv(t)
	ctx := context.Background()
	var failing atomic.Bool
	var calls atomic.Int32
	failStateWrites(t, &failing, &calls)
	failing.Store(true)

	id := newACMECert(t, s, "pending", time.Time{}, store.ACMEInfo{Email: "ops@example.com"}, "www.example.com")
	err := s.issue(ctx, id)
	if !errors.Is(err, errNotRecorded) {
		t.Fatalf("issue = %v", err)
	}
	if n := calls.Load(); n != int32(stateAttempts) {
		t.Fatalf("state writes = %d, want %d (retried)", n, stateAttempts)
	}
	if waitCount(issued, 1) != 1 {
		t.Fatal("the gateway must be redeployed: the files are on disk")
	}
	if _, err := os.Stat(filepath.Join(s.CertDir(id), "fullchain.pem")); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.Store.CertificateByID(ctx, id); c.Status != "pending" {
		t.Fatalf("status = %s", c.Status)
	}

	// the database works again: the certificate on disk is recorded, no new order
	failing.Store(false)
	if err := s.issue(ctx, id); err != nil {
		t.Fatal(err)
	}
	c, _ := s.Store.CertificateByID(ctx, id)
	if c.Status != "valid" || time.Until(c.NotAfter) < 80*24*time.Hour {
		t.Fatalf("certificate: %+v", c)
	}
	f.mu.Lock()
	orders := f.orders
	f.mu.Unlock()
	if orders != 1 {
		t.Fatalf("orders = %d, want 1", orders)
	}
	// in sync now: a renewal (e.g. "renew now") orders normally
	if err := s.issue(ctx, id); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	orders = f.orders
	f.mu.Unlock()
	if orders != 2 {
		t.Fatalf("orders after a renewal = %d, want 2", orders)
	}
}

// A certificate on disk for other names (the domains were changed) is not adopted.
func TestDiskCertForOtherNamesIsIgnored(t *testing.T) {
	s, f, _ := acmeEnv(t)
	ctx := context.Background()
	id := newACMECert(t, s, "pending", time.Time{}, store.ACMEInfo{Email: "ops@example.com"}, "www.example.com")
	if err := s.issue(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.UpdateCertificateState(ctx, id, "pending", "", []string{"other.example.com"}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := s.issue(ctx, id); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.orders != 2 {
		t.Fatalf("orders = %d, want 2", f.orders)
	}
}

// A hung order is abandoned after issueTimeout: the slot is freed and the order marked
// failed, no second order starts while lego still runs, and a late success is kept.
func TestHungOrderTimesOut(t *testing.T) {
	s, f, _ := acmeEnv(t)
	ctx := context.Background()
	old := issueTimeout
	issueTimeout = 300 * time.Millisecond
	t.Cleanup(func() { issueTimeout = old })
	f.mu.Lock()
	f.gate = make(chan struct{})
	gate := f.gate
	f.mu.Unlock()
	id := newACMECert(t, s, "pending", time.Time{}, store.ACMEInfo{Email: "ops@example.com"}, "www.example.com")
	s.Start(id)
	waitFor(t, "the order to time out", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, failed := s.failures[id]
		return failed && !s.running[id] && s.late[id] != nil
	})
	if c, _ := s.Store.CertificateByID(ctx, id); c.Status != "error" || c.LastError == "" {
		t.Fatalf("after the timeout: %+v", c)
	}
	if len(s.slots) != 0 {
		t.Fatalf("slots in use = %d", len(s.slots))
	}
	// lego is still working on it: no second order
	s.Start(id)
	s.mu.Lock()
	running := s.running[id]
	s.mu.Unlock()
	if running {
		t.Fatal("a second order started while the first is still in lego")
	}
	close(gate)
	waitFor(t, "the late result", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.late[id] == nil
	})
	c, _ := s.Store.CertificateByID(ctx, id)
	if c.Status != "valid" {
		t.Fatalf("late success not kept: %+v", c)
	}
	s.mu.Lock()
	_, failed := s.failures[id]
	s.mu.Unlock()
	if failed {
		t.Fatal("the failure must be cleared by the late success")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.orders != 1 {
		t.Fatalf("orders = %d", f.orders)
	}
}
