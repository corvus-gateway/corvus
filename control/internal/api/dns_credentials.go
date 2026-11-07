package api

import (
	"fmt"
	"net/http"
	"strings"

	"corvus/control/internal/acme"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// Saved DNS credentials for DNS-01 certificates. The secret is write-only: the API
// returns name, provider, usage and timestamps, never the credentials themselves.

type dnsCredentialInput struct {
	Name        *string `json:"name"`
	Provider    string  `json:"provider"`
	Credentials *string `json:"credentials"` // certbot ini text; omitted on update = unchanged
}

func (s *Server) listDNSCredentials(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListDNSCredentials(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createDNSCredential(w http.ResponseWriter, r *http.Request) {
	var in dnsCredentialInput
	if !decode(w, r, &in) {
		return
	}
	var c *store.DNSCredential
	err := s.Store.WithTx(r.Context(), func(tx *store.Store) error {
		provider := in.Provider
		if provider == "" {
			provider = "cloudflare"
		}
		var text, name string
		if in.Credentials != nil {
			text = *in.Credentials
		}
		if in.Name != nil {
			name = strings.TrimSpace(*in.Name)
		}
		cred, err := acme.ParseCredentials(provider, text)
		if err != nil {
			return validate.Errors{{Field: "credentials", Code: "invalid", Message: err.Error()}}
		}
		if msg := credentialNameError(r.Context(), tx, name, 0); msg != "" {
			return validate.Errors{{Field: "name", Code: "invalid", Message: msg}}
		}
		c, err = acme.SaveCredential(r.Context(), tx, s.DataKey, name, provider, cred)
		return err
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "dns_credential.create", fmt.Sprintf("dns_credential:%d", c.ID), c)
	writeJSON(w, 201, c)
}

// updateDNSCredential renames a credential and/or replaces its secret (and provider).
// Certificates using it pick up a new secret at their next renewal; nothing is re-issued.
func (s *Server) updateDNSCredential(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in dnsCredentialInput
	if !decode(w, r, &in) {
		return
	}
	var before store.DNSCredential
	var c *store.DNSCredential
	err := s.Store.WithTx(r.Context(), func(tx *store.Store) error {
		var err error
		if c, err = tx.DNSCredentialByID(r.Context(), id); err != nil {
			return err
		}
		before = *c
		if in.Name != nil {
			name := strings.TrimSpace(*in.Name)
			if name == "" {
				return validate.Errors{{Field: "name", Code: "required", Message: "a name is required"}}
			}
			if msg := credentialNameError(r.Context(), tx, name, id); msg != "" {
				return validate.Errors{{Field: "name", Code: "invalid", Message: msg}}
			}
			c.Name = name
		}
		if in.Credentials != nil {
			provider := in.Provider
			if provider == "" {
				provider = c.Provider
			}
			cred, err := acme.ParseCredentials(provider, *in.Credentials)
			if err != nil {
				return validate.Errors{{Field: "credentials", Code: "invalid", Message: err.Error()}}
			}
			if c.Secret, err = acme.SealCredential(s.DataKey, cred); err != nil {
				return err
			}
			c.Provider = provider
		} else if in.Provider != "" && in.Provider != c.Provider {
			return validate.Errors{{Field: "credentials", Code: "required", Message: "enter the credentials of the new provider"}}
		}
		return tx.UpdateDNSCredential(r.Context(), c)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "dns_credential.update", fmt.Sprintf("dns_credential:%d", id), map[string]any{"before": before, "after": c, "secret_replaced": in.Credentials != nil})
	writeJSON(w, 200, c)
}

func (s *Server) deleteDNSCredential(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var before *store.DNSCredential
	err := s.Store.WithTx(r.Context(), func(tx *store.Store) error {
		var err error
		if before, err = tx.DNSCredentialByID(r.Context(), id); err != nil {
			return err
		}
		if before.UsedBy > 0 {
			return validate.Errors{{Field: "id", Code: "in_use", Message: fmt.Sprintf("DNS credential is used by %d certificate(s)", before.UsedBy)}}
		}
		return tx.DeleteDNSCredential(r.Context(), id)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "dns_credential.delete", fmt.Sprintf("dns_credential:%d", id), before)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
