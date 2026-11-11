package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"corvus/control/internal/store"
)

// waitCount waits up to 5 s for n to reach want and returns its value.
func waitCount(n *atomic.Int32, want int32) int32 {
	deadline := time.Now().Add(5 * time.Second)
	for n.Load() < want && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	return n.Load()
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for " + what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// At most two orders run at once; the others wait for a slot, and concurrent
// orders share one ACME account.
func TestOrdersBoundedAndShareAccount(t *testing.T) {
	s, f, issued := acmeEnv(t)
	ctx := context.Background()
	f.mu.Lock()
	f.gate = make(chan struct{})
	gate := f.gate
	f.mu.Unlock()
	var ids []int64
	for _, d := range []string{"a.example.com", "b.example.com", "c.example.com"} {
		ids = append(ids, newACMECert(t, s, "pending", time.Time{}, store.ACMEInfo{Email: "ops@example.com"}, d))
	}
	for _, id := range ids {
		s.Start(id)
	}
	gated := func() int { f.mu.Lock(); defer f.mu.Unlock(); return f.atGate }
	waitFor(t, "two orders", func() bool { return gated() == 2 })
	time.Sleep(200 * time.Millisecond)
	if n := gated(); n != 2 {
		t.Fatalf("orders in flight = %d, want 2", n)
	}
	s.mu.Lock()
	queued := len(s.running)
	s.mu.Unlock()
	if queued != 3 {
		t.Fatalf("running/queued = %d, want 3", queued)
	}
	close(gate)
	waitFor(t, "all issued", func() bool {
		for _, id := range ids {
			if c, _ := s.Store.CertificateByID(ctx, id); c.Status != "valid" {
				return false
			}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.running) == 0
	})
	f.mu.Lock()
	accounts, orders := f.accounts, f.orders
	f.mu.Unlock()
	if accounts != 1 || orders != 3 {
		t.Fatalf("accounts=%d orders=%d", accounts, orders)
	}
	if waitCount(issued, 1) < 1 {
		t.Fatal("no redeploy after the burst")
	}
}

// Completions in a burst trigger one redeploy, DeployDelay after the last.
func TestRedeployDebounced(t *testing.T) {
	s := testService(t)
	var calls atomic.Int32
	s.OnIssued = func(context.Context) { calls.Add(1) }
	s.DeployDelay = 150 * time.Millisecond
	s.mu.Lock()
	s.init()
	s.running[1], s.running[2], s.running[3] = true, true, true
	s.mu.Unlock()
	start := time.Now()
	for i := 0; i < 4; i++ {
		s.issued(1)
		time.Sleep(50 * time.Millisecond) // each completion pushes the redeploy back
	}
	if calls.Load() != 0 {
		t.Fatal("redeployed during the burst")
	}
	if waitCount(&calls, 1) != 1 || time.Since(start) < 300*time.Millisecond {
		t.Fatalf("calls=%d after %v", calls.Load(), time.Since(start))
	}
	time.Sleep(300 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
	// nothing else in flight: the redeploy follows within about a second
	s.DeployDelay = time.Hour
	s.mu.Lock()
	s.running = map[int64]bool{7: true}
	s.mu.Unlock()
	s.issued(7)
	if waitCount(&calls, 2) != 2 {
		t.Fatal("single issuance not deployed promptly")
	}
	(&Service{}).issued(1) // no callback: nothing to do
}

// A steady stream of completions still redeploys DeployMaxDelay after the first one.
func TestRedeployDelayCapped(t *testing.T) {
	s := testService(t)
	var calls atomic.Int32
	s.OnIssued = func(context.Context) { calls.Add(1) }
	s.DeployDelay, s.DeployMaxDelay = 150*time.Millisecond, 200*time.Millisecond
	s.mu.Lock()
	s.init()
	s.running[1], s.running[2] = true, true
	s.mu.Unlock()
	for i := 0; i < 15; i++ { // 750 ms, each completion within DeployDelay of the last
		s.issued(1)
		time.Sleep(50 * time.Millisecond)
	}
	if calls.Load() == 0 {
		t.Fatal("no redeploy during a long burst")
	}
	time.Sleep(400 * time.Millisecond)
	if n := calls.Load(); n < 2 || n > 5 {
		t.Fatalf("calls = %d, want a few (capped bursts plus the last)", n)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.burstStart.IsZero() {
		t.Error("burst start kept after the redeploy")
	}
}

func TestRetryBackoff(t *testing.T) {
	for attempts, want := range map[int]time.Duration{1: time.Hour, 2: 2 * time.Hour, 3: 4 * time.Hour, 4: 8 * time.Hour, 5: 12 * time.Hour, 9: 12 * time.Hour} {
		if got := retryAfter(attempts); got != want {
			t.Errorf("retryAfter(%d) = %v, want %v", attempts, got, want)
		}
	}
}

// A failed order is retried after the backoff, not on every check.
func TestFailedOrderRetriedAfterBackoff(t *testing.T) {
	s, f, _ := acmeEnv(t)
	ctx := context.Background()
	f.rejectAll = true
	id := newACMECert(t, s, "pending", time.Time{}, store.ACMEInfo{Email: "ops@example.com"}, "www.example.com")
	idle := func() bool { s.mu.Lock(); defer s.mu.Unlock(); return len(s.running) == 0 }
	s.Start(id)
	waitFor(t, "first order", idle)
	s.mu.Lock()
	fl := s.failures[id]
	s.mu.Unlock()
	if fl.attempts != 1 || time.Until(fl.next) < 59*time.Minute {
		t.Fatalf("failure = %+v", fl)
	}
	accounts := func() int { f.mu.Lock(); defer f.mu.Unlock(); return f.accounts }
	before := accounts()
	s.checkDue(ctx, time.Now().Add(30*time.Minute)) // not yet due (and in the renewal window)
	waitFor(t, "idle", idle)
	if c, _ := s.Store.CertificateByID(ctx, id); c.Status != "error" || accounts() != before {
		t.Fatalf("retried before the backoff: %q", c.Status)
	}
	f.mu.Lock()
	f.rejectAll = false
	f.mu.Unlock()
	s.checkDue(ctx, time.Now().Add(61*time.Minute))
	waitFor(t, "retry", func() bool {
		c, _ := s.Store.CertificateByID(ctx, id)
		return c.Status == "valid" && idle()
	})
	s.mu.Lock()
	_, still := s.failures[id]
	s.mu.Unlock()
	if still {
		t.Fatal("success must clear the backoff")
	}
	// entries of deleted certificates are dropped
	s.mu.Lock()
	s.failures[9999] = failure{attempts: 1, next: time.Now()}
	s.mu.Unlock()
	s.checkDue(ctx, time.Now())
	waitFor(t, "idle", idle)
	s.mu.Lock()
	_, kept := s.failures[9999]
	s.mu.Unlock()
	if kept {
		t.Fatal("failure of a deleted certificate kept")
	}
}

func TestCheckDueRenewalWindow(t *testing.T) {
	s, _, _ := acmeEnv(t)
	ctx := context.Background()
	s.Concurrency = 1
	// not in the window: nothing starts
	far := newACMECert(t, s, "valid", time.Now().Add(60*24*time.Hour), store.ACMEInfo{Email: "ops@example.com"}, "far.example.com")
	// in the window, but auto-renew is off
	mc := &store.Certificate{Name: "m", Kind: "acme", Domains: []string{"manual.example.com"}, Status: "valid", NotAfter: time.Now().Add(24 * time.Hour), ACME: store.ACMEInfo{Email: "ops@example.com"}}
	if err := s.Store.CreateCertificate(ctx, mc); err != nil {
		t.Fatal(err)
	}
	manual := mc.ID
	s.checkDue(ctx, time.Now())
	s.mu.Lock()
	n := len(s.running)
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("started %d orders outside the window", n)
	}
	// a short-lived certificate (6 days) is renewed in its last third: 2 days
	writeLeaf(t, s.CertDir(far), 6*24*time.Hour)
	if w := s.renewWindow(far); w != 2*24*time.Hour {
		t.Fatalf("window = %v", w)
	}
	writeLeaf(t, s.CertDir(manual), 90*24*time.Hour)
	if w := s.renewWindow(manual); w != 30*24*time.Hour {
		t.Fatalf("window = %v", w)
	}
	if w := s.renewWindow(424242); w != 30*24*time.Hour {
		t.Fatalf("no file: %v", w)
	}
	_ = os.WriteFile(filepath.Join(s.CertDir(far), "fullchain.pem"), []byte("junk"), 0o600)
	if w := s.renewWindow(far); w != 30*24*time.Hour {
		t.Fatalf("unreadable file: %v", w)
	}
	_ = os.WriteFile(filepath.Join(s.CertDir(far), "fullchain.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("x")}), 0o600)
	if w := s.renewWindow(far); w != 30*24*time.Hour {
		t.Fatalf("bad DER: %v", w)
	}
	if w := (&Service{}).renewWindow(1); w != 30*24*time.Hour {
		t.Fatalf("no cert dir: %v", w)
	}
}

func writeLeaf(t *testing.T, dir string, life time.Duration) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	nb := time.Now().Add(-time.Hour)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "x"}, NotBefore: nb, NotAfter: nb.Add(life)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fullchain.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}
