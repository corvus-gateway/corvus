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
	OnIssued        func(ctx context.Context) // re-deploys the gateway so new files are loaded

	mu      sync.Mutex
	tokens  map[string]string // HTTP-01 token → key authorization
	running map[int64]bool
}

func (s *Service) init() {
	if s.tokens == nil {
		s.tokens = map[string]string{}
		s.running = map[int64]bool{}
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
// one is running is ignored.
func (s *Service) Start(id int64) {
	s.mu.Lock()
	s.init()
	if s.running[id] {
		s.mu.Unlock()
		return
	}
	s.running[id] = true
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.running, id)
			s.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := s.issue(ctx, id); err != nil {
			slog.Warn("ACME issuance failed", "certificate", id, "err", err)
		}
	}()
}

func (s *Service) issue(ctx context.Context, id int64) error {
	c, err := s.Store.CertificateByID(ctx, id)
	if err != nil {
		return err
	}
	renewal := c.Status == "valid"
	fail := func(err error) error {
		status := "error"
		if renewal && time.Now().Before(c.NotAfter) {
			status = "valid" // keep serving the still-valid certificate; surface the error
		}
		_ = s.Store.SetCertificateStatus(ctx, id, status, err.Error())
		return err
	}
	cl, _, err := s.client(ctx, c.ACME.Email, c.ACME.Staging, c.Domains)
	if err != nil {
		return fail(err)
	}
	switch c.ACME.Challenge {
	case "dns-01":
		provider, cred, err := s.dnsCredentials(ctx, c)
		if err != nil {
			return fail(err)
		}
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
	if s.OnIssued != nil {
		s.OnIssued(ctx)
	}
	return nil
}

// RenewLoop renews ACME certificates that expire within 30 days.
func (s *Service) RenewLoop(ctx context.Context) {
	check := func() {
		list, err := s.Store.ListCertificates(ctx)
		if err != nil {
			return
		}
		for _, c := range list {
			if c.Kind == "acme" && c.AutoRenew && c.Status != "pending" && time.Until(c.NotAfter) < 30*24*time.Hour {
				slog.Info("renewing ACME certificate", "certificate", c.ID, "not_after", c.NotAfter)
				s.Start(c.ID)
			}
		}
	}
	t := time.NewTicker(12 * time.Hour)
	defer t.Stop()
	time.Sleep(time.Minute)
	check()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			check()
		}
	}
}
