package acme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"corvus/control/internal/seal"
	"corvus/control/internal/store"
)

// Saved DNS credentials: a provider secret entered once and referenced by every
// DNS-01 certificate that uses it, so replacing the secret updates all renewals.

// OpenCredential decrypts a saved credential.
func OpenCredential(key []byte, c *store.DNSCredential) (Credentials, error) {
	var cred Credentials
	raw, err := seal.Open(key, c.Secret)
	if err != nil {
		return cred, err
	}
	err = json.Unmarshal([]byte(raw), &cred)
	return cred, err
}

// SealCredential encrypts cred for storage.
func SealCredential(key []byte, cred Credentials) (string, error) {
	raw, _ := json.Marshal(cred)
	return seal.Seal(key, string(raw))
}

// AutoName returns the first free name of the form "Cloudflare", "Cloudflare 2", ...
func AutoName(list []store.DNSCredential, provider string) string {
	base := provider
	if p, ok := ProviderByID(provider); ok {
		base = p.Name
	}
	used := map[string]bool{}
	for _, c := range list {
		used[c.Name] = true
	}
	name := base
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s %d", base, i)
	}
	return name
}

// SaveCredential stores cred as a new saved credential; an empty name gets AutoName.
func SaveCredential(ctx context.Context, st *store.Store, key []byte, name, provider string, cred Credentials) (*store.DNSCredential, error) {
	sealed, err := SealCredential(key, cred)
	if err != nil {
		return nil, err
	}
	if name == "" {
		list, err := st.ListDNSCredentials(ctx)
		if err != nil {
			return nil, err
		}
		name = AutoName(list, provider)
	}
	c := &store.DNSCredential{Name: name, Provider: provider, Secret: sealed}
	if err := st.CreateDNSCredential(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func sameCredential(a, b Credentials) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// ReuseOrSaveCredential returns the saved credential with the same provider and secret,
// or saves a new auto-named one (created reports which).
func ReuseOrSaveCredential(ctx context.Context, st *store.Store, key []byte, provider string, cred Credentials) (c *store.DNSCredential, created bool, err error) {
	list, err := st.ListDNSCredentials(ctx)
	if err != nil {
		return nil, false, err
	}
	for i := range list {
		if list[i].Provider != provider {
			continue
		}
		if have, err := OpenCredential(key, &list[i]); err == nil && sameCredential(have, cred) {
			return &list[i], false, nil
		}
	}
	c, err = SaveCredential(ctx, st, key, "", provider, cred)
	return c, err == nil, err
}

// AdoptLegacyCredentials moves per-certificate DNS secrets of older versions into saved
// credentials (de-duplicated by provider and secret) and points the certificates at
// them. It is idempotent; certificates whose secret can't be decrypted are left as
// they are and keep renewing from their own secret.
func (s *Service) AdoptLegacyCredentials(ctx context.Context) (int, error) {
	list, err := s.Store.ListCertificates(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range list {
		if c.DNSCredentialID != nil || c.ACME.Challenge != "dns-01" || c.ACME.Credentials == "" {
			continue
		}
		raw, err := seal.Open(s.Key, c.ACME.Credentials)
		if err != nil {
			slog.Warn("DNS credentials of certificate can't be decrypted; left in place", "certificate", c.ID, "err", err)
			continue
		}
		var cred Credentials
		if err := json.Unmarshal([]byte(raw), &cred); err != nil {
			continue
		}
		provider := c.ACME.DNSProvider
		if provider == "" {
			provider = "cloudflare"
		}
		err = s.Store.WithTx(ctx, func(tx *store.Store) error {
			saved, _, err := ReuseOrSaveCredential(ctx, tx, s.Key, provider, cred)
			if err != nil {
				return err
			}
			info := c.ACME
			info.DNSProvider, info.Credentials = provider, ""
			return tx.SetCertificateDNSCredential(ctx, c.ID, saved.ID, info)
		})
		if err != nil {
			return n, err
		}
		n++
	}
	if n > 0 {
		slog.Info("moved per-certificate DNS credentials to saved credentials", "certificates", n)
	}
	return n, nil
}

// dnsCredentials returns the provider and secret a DNS-01 certificate is issued with:
// its saved credential, or (older certificates not yet moved) its own secret.
func (s *Service) dnsCredentials(ctx context.Context, c *store.Certificate) (string, Credentials, error) {
	if c.DNSCredentialID != nil {
		saved, err := s.Store.DNSCredentialByID(ctx, *c.DNSCredentialID)
		if err != nil {
			return "", Credentials{}, fmt.Errorf("DNS credential: %w", err)
		}
		cred, err := OpenCredential(s.Key, saved)
		return saved.Provider, cred, err
	}
	if c.ACME.Credentials == "" {
		return "", Credentials{}, errors.New("no DNS credentials")
	}
	var cred Credentials
	raw, err := seal.Open(s.Key, c.ACME.Credentials)
	if err != nil {
		return "", cred, err
	}
	_ = json.Unmarshal([]byte(raw), &cred)
	return c.ACME.DNSProvider, cred, nil
}
