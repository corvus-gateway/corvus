// Package acme issues and renews certificates with an ACME CA (Let's Encrypt by
// default) using lego. HTTP-01 challenges are answered by a loopback HTTP server
// that the gateway proxies /.well-known/acme-challenge/ to; DNS-01 uses the
// Cloudflare API (required for wildcard names).
package acme

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"

	"corvus/control/internal/certs"
	"corvus/control/internal/seal"
	"corvus/control/internal/store"
)

// Credentials for DNS providers (stored sealed in a saved DNS credential; older
// versions kept them in the certificate's ACME info).
type Credentials struct {
	// Cloudflare (kept in named fields for compatibility with stored certificates)
	APIToken string `json:"api_token,omitempty"`
	Email    string `json:"email,omitempty"`   // legacy global API key auth
	APIKey   string `json:"api_key,omitempty"` // legacy global API key auth
	// Values holds the certbot keys of every other provider (see providers.go).
	Values map[string]string `json:"values,omitempty"`
}

type Service struct {
	Store     *store.Store
	Key       []byte                // data key for sealed secrets
	CertDir   func(id int64) string // where certificate files are written
	Directory string                // optional CA directory override (testing, private CAs)
	// DirectorySuffix limits the override to certificates whose domains all end with it
	// (e.g. ".corvus.test" → test CA for test domains, Let's Encrypt for real ones).
	DirectorySuffix string
	// OnIssued re-deploys the gateway so new files are loaded. Completions in a burst
	// (an import, a renewal round) are combined into one call, DeployDelay (default
	// 30 s) after the last of them, but no later than DeployMaxDelay (default 2 min)
	// after the first; when no other order is in flight it runs sooner.
	OnIssued       func(ctx context.Context)
	DeployDelay    time.Duration
	DeployMaxDelay time.Duration
	// Concurrency bounds simultaneous orders (default 2); more wait for a slot.
	Concurrency int

	mu          sync.Mutex
	tokens      map[string]string // HTTP-01 token → key authorization
	running     map[int64]bool    // started orders, including those waiting for a slot
	failures    map[int64]failure // failed orders and when to try again (in memory)
	slots       chan struct{}
	deployTimer *time.Timer
	burstStart  time.Time // first completion not yet deployed

	accountMu sync.Mutex // one account lookup/registration at a time
}

// failure tracks retries of a certificate whose last order failed.
type failure struct {
	attempts int
	next     time.Time
}

// Retry backoff after a failed order: 1 h, doubling up to 12 h.
const (
	retryFirst = time.Hour
	retryMax   = 12 * time.Hour
)

func retryAfter(attempts int) time.Duration {
	d := retryFirst
	for i := 1; i < attempts && d < retryMax; i++ {
		d *= 2
	}
	return min(d, retryMax)
}

func (s *Service) init() {
	if s.tokens == nil {
		s.tokens = map[string]string{}
		s.running = map[int64]bool{}
		s.failures = map[int64]failure{}
	}
	if s.slots == nil {
		n := s.Concurrency
		if n <= 0 {
			n = 2
		}
		s.slots = make(chan struct{}, n)
	}
}

// ---------------------------------------------------------------- HTTP-01

// Present / CleanUp implement lego's challenge.Provider for HTTP-01.
func (s *Service) Present(_, token, keyAuth string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	s.tokens[token] = keyAuth
	return nil
}

func (s *Service) CleanUp(_, token, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
	return nil
}

// Handler answers /.well-known/acme-challenge/<token> (bound to loopback only).
func (s *Service) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/.well-known/acme-challenge/")
		s.mu.Lock()
		ka, ok := s.tokens[token]
		s.mu.Unlock()
		if !ok || token == "" || strings.Contains(token, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(ka))
	})
}

// ---------------------------------------------------------------- account

type account struct {
	email string
	reg   *registration.Resource
	key   crypto.PrivateKey
}

func (a *account) GetEmail() string                        { return a.email }
func (a *account) GetRegistration() *registration.Resource { return a.reg }
func (a *account) GetPrivateKey() crypto.PrivateKey        { return a.key }

func (s *Service) directory(staging bool, domains []string) string {
	if s.Directory != "" && overrideApplies(s.DirectorySuffix, domains) {
		return s.Directory
	}
	if staging {
		return lego.LEDirectoryStaging
	}
	return lego.LEDirectoryProduction
}

func overrideApplies(suffix string, domains []string) bool {
	if suffix == "" {
		return true
	}
	if len(domains) == 0 {
		return false
	}
	for _, d := range domains {
		if !strings.HasSuffix(strings.ToLower(strings.TrimPrefix(d, "*.")), strings.ToLower(suffix)) {
			return false
		}
	}
	return true
}

// sameCA reports whether an account registration URI belongs to the directory's CA.
func sameCA(regURI, dir string) bool {
	a, err1 := url.Parse(regURI)
	b, err2 := url.Parse(dir)
	return err1 == nil && err2 == nil && strings.EqualFold(a.Host, b.Host)
}

func (s *Service) client(ctx context.Context, email string, staging bool, domains []string) (*lego.Client, *account, error) {
	// Concurrent orders for the same email must not each register a new account:
	// the first registers and stores it, the others then load it.
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	acc := &account{email: email}
	dir := s.directory(staging, domains)
	stored, err := s.Store.ACMEAccount(ctx, email, staging)
	if err == nil {
		pemKey, err := seal.Open(s.Key, stored.KeyPEM)
		if err != nil {
			return nil, nil, err
		}
		blk, _ := pem.Decode([]byte(pemKey))
		if blk == nil {
			return nil, nil, errors.New("stored ACME account key is corrupt")
		}
		if acc.key, err = x509.ParseECPrivateKey(blk.Bytes); err != nil {
			return nil, nil, err
		}
		if stored.Registration != "" {
			var reg registration.Resource
			if json.Unmarshal([]byte(stored.Registration), &reg) == nil {
				acc.reg = &reg
			}
		}
		// An account registered with a different CA (e.g. the test CA) is not reused:
		// register a fresh one with this CA.
		if acc.reg != nil && !sameCA(acc.reg.URI, dir) {
			k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				return nil, nil, err
			}
			acc.key, acc.reg = k, nil
		}
	} else if errors.Is(err, store.ErrNotFound) {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		acc.key = k
	} else {
		return nil, nil, err
	}

	cfg := lego.NewConfig(acc)
	cfg.CADirURL = dir
	cfg.Certificate.KeyType = certcrypto.EC256
	cfg.Certificate.Timeout = 5 * time.Minute // a busy CA can take longer than lego's 30 s to finalize
	cfg.UserAgent = "corvus"
	cl, err := lego.NewClient(cfg)
	if err != nil {
		return nil, nil, err
	}
	if acc.reg == nil {
		reg, err := cl.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return nil, nil, fmt.Errorf("ACME account registration: %w", err)
		}
		acc.reg = reg
		der, err := x509.MarshalECPrivateKey(acc.key.(*ecdsa.PrivateKey))
		if err != nil {
			return nil, nil, err
		}
		sealed, err := seal.Seal(s.Key, string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})))
		if err != nil {
			return nil, nil, err
		}
		regJSON, _ := json.Marshal(reg)
		if err := s.Store.SaveACMEAccount(ctx, &store.ACMEAccount{Email: email, Staging: staging, KeyPEM: sealed, Registration: string(regJSON)}); err != nil {
			return nil, nil, err
		}
	}
	return cl, acc, nil
}

// ---------------------------------------------------------------- issuance

// Start issues (or renews) certificate id in the background; a second call while
// one is running or waiting is ignored. At most Concurrency orders run at once
// (CA rate limits, a small host); the others wait for a slot.
func (s *Service) Start(id int64) {
	s.mu.Lock()
	s.init()
	if s.running[id] {
		s.mu.Unlock()
		return
	}
	s.running[id] = true
	slots := s.slots
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.running, id)
			s.mu.Unlock()
		}()
		slots <- struct{}{}
		defer func() { <-slots }()
		// No overall deadline: lego does not honour the context during Obtain (DNS
		// propagation can take long), and an expired context would then make the
		// steps after a successful issuance fail and leave the certificate "pending".
		err := s.issue(context.Background(), id)
		s.mu.Lock()
		if err != nil {
			f := s.failures[id]
			f.attempts++
			f.next = time.Now().Add(retryAfter(f.attempts))
			s.failures[id] = f
			slog.Warn("ACME issuance failed", "certificate", id, "err", err, "retry_at", f.next.Format(time.RFC3339))
		} else {
			delete(s.failures, id)
		}
		s.mu.Unlock()
	}()
}

// issued schedules the gateway redeploy after certificate id was written: one
// call for a burst of completions, DeployDelay after the last but at most
// DeployMaxDelay after the first; about a second when no other order is running or
// waiting.
func (s *Service) issued(id int64) {
	if s.OnIssued == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	delay := s.DeployDelay
	if delay <= 0 {
		delay = 30 * time.Second
	}
	others := len(s.running)
	if s.running[id] {
		others--
	}
	if others == 0 {
		delay = min(delay, time.Second)
	}
	maxDelay := s.DeployMaxDelay
	if maxDelay <= 0 {
		maxDelay = 2 * time.Minute
	}
	now := time.Now()
	if s.burstStart.IsZero() {
		s.burstStart = now
	}
	delay = max(min(delay, s.burstStart.Add(maxDelay).Sub(now)), 0)
	if s.deployTimer == nil {
		s.deployTimer = time.AfterFunc(delay, s.redeploy)
		return
	}
	s.deployTimer.Reset(delay)
}

// redeploy runs OnIssued for the completions so far; the next one starts a new burst.
func (s *Service) redeploy() {
	s.mu.Lock()
	s.burstStart = time.Time{}
	s.mu.Unlock()
	s.OnIssued(context.Background())
}

func (s *Service) issue(ctx context.Context, id int64) error {
	c, err := s.Store.CertificateByID(ctx, id)
	if err != nil {
		return err
	}
	renewal := c.Status == "valid"
	var cred Credentials // DNS-01 secret in use, redacted from errors
	fail := func(err error) error {
		status := "error"
		if renewal && time.Now().Before(c.NotAfter) {
			status = "valid" // keep serving the still-valid certificate; surface the error
		}
		msg := redactError(err.Error(), cred)
		_ = s.Store.SetCertificateStatus(ctx, id, status, msg)
		return errors.New(msg)
	}
	cl, _, err := s.client(ctx, c.ACME.Email, c.ACME.Staging, c.Domains)
	if err != nil {
		return fail(err)
	}
	switch c.ACME.Challenge {
	case "dns-01":
		provider, dc, err := s.dnsCredentials(ctx, c)
		if err != nil {
			return fail(err)
		}
		cred = dc
		p, err := dnsProvider(provider, cred)
		if err != nil {
			return fail(err)
		}
		if err := cl.Challenge.SetDNS01Provider(p); err != nil {
			return fail(err)
		}
	default:
		if err := cl.Challenge.SetHTTP01Provider(s); err != nil {
			return fail(err)
		}
	}
	res, err := cl.Certificate.Obtain(certificate.ObtainRequest{Domains: c.Domains, Bundle: true})
	if err != nil {
		return fail(fmt.Errorf("issuance failed: %w", err))
	}
	info, err := certs.Parse(res.Certificate, res.PrivateKey)
	if err != nil {
		return fail(err)
	}
	if err := certs.Write(s.CertDir(id), res.Certificate, res.PrivateKey); err != nil {
		return fail(err)
	}
	if err := s.Store.UpdateCertificateState(ctx, id, "valid", "", info.Domains, info.NotAfter); err != nil {
		return err
	}
	slog.Info("ACME certificate issued", "certificate", id, "domains", info.Domains, "not_after", info.NotAfter)
	s.issued(id)
	return nil
}

// renewCheckEvery is how often RenewLoop looks for due orders; it only starts
// those whose renewal window or retry time has been reached.
const renewCheckEvery = 15 * time.Minute

// RenewLoop renews ACME certificates in their renewal window, resumes orders
// interrupted by a restart and retries failed orders with backoff.
func (s *Service) RenewLoop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Minute):
	}
	s.checkDue(ctx, time.Now())
	t := time.NewTicker(renewCheckEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.checkDue(ctx, time.Now())
		}
	}
}

// checkDue starts the orders that are due at now.
func (s *Service) checkDue(ctx context.Context, now time.Time) {
	list, err := s.Store.ListCertificates(ctx)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.init()
	failures := make(map[int64]failure, len(s.failures))
	exists := map[int64]bool{}
	for _, c := range list {
		exists[c.ID] = true
	}
	for id, f := range s.failures {
		if !exists[id] {
			delete(s.failures, id) // certificate deleted
			continue
		}
		failures[id] = f
	}
	s.mu.Unlock()
	for _, c := range list {
		if c.Kind != "acme" {
			continue
		}
		f, failed := failures[c.ID]
		switch {
		case failed && now.Before(f.next):
			// the last order failed: wait for the retry time
		case failed && (c.AutoRenew || c.Status != "valid"):
			slog.Info("retrying failed ACME order", "certificate", c.ID, "attempt", f.attempts+1)
			s.Start(c.ID)
		case c.Status == "pending":
			// an order interrupted by a restart (Start ignores orders still running)
			slog.Info("resuming ACME order", "certificate", c.ID)
			s.Start(c.ID)
		case c.AutoRenew && c.NotAfter.Sub(now) < s.renewWindow(c.ID):
			slog.Info("renewing ACME certificate", "certificate", c.ID, "not_after", c.NotAfter)
			s.Start(c.ID)
		}
	}
}

// renewWindow is how long before expiry a certificate is renewed: 30 days, or a
// third of its lifetime for short-lived certificates.
func (s *Service) renewWindow(id int64) time.Duration {
	const window = 30 * 24 * time.Hour
	if s.CertDir == nil {
		return window
	}
	raw, err := os.ReadFile(filepath.Join(s.CertDir(id), "fullchain.pem"))
	if err != nil {
		return window
	}
	blk, _ := pem.Decode(raw)
	if blk == nil {
		return window
	}
	leaf, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return window
	}
	if life := leaf.NotAfter.Sub(leaf.NotBefore); life > 0 {
		return min(window, life/3)
	}
	return window
}
