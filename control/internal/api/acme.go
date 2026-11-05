package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"corvus/control/internal/acme"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// ACMEService starts background issuance/renewal of an ACME certificate.
type ACMEService interface{ Start(id int64) }

func emailOK(e string) bool {
	at := strings.LastIndex(e, "@")
	return at > 0 && at < len(e)-3 && strings.Contains(e[at:], ".") && len(e) <= 254 && !strings.ContainsAny(e, " \t\r\n")
}

// certRequest is an ACME order as entered in the UI (NPM-compatible fields).
type certRequest struct {
	Email          string          `json:"email"`
	DNSChallenge   bool            `json:"dns_challenge"`
	Challenge      string          `json:"challenge"` // alternative to dns_challenge: http-01 | dns-01
	DNSProvider    string          `json:"dns_provider"`
	DNSCredentials json.RawMessage `json:"dns_credentials"` // certbot ini text, or {"api_token": "..."}
	CredentialsINI string          `json:"dns_credentials_ini"`
	// DNSCredentialID selects a saved DNS credential; without it the credentials above
	// are saved as a new one named DNSCredentialName (empty = automatic name).
	DNSCredentialID   *int64 `json:"dns_credential_id"`
	DNSCredentialName string `json:"dns_credential_name"`
	Staging           bool   `json:"staging"`
}

// acmeOrder is a validated certRequest: the ACME info to store and either an existing
// saved credential (credID) or a new one to save with the certificate (newCred).
type acmeOrder struct {
	info     store.ACMEInfo
	domains  []string
	credID   *int64
	newCred  *acme.Credentials
	credName string
}

// buildACME validates an order for domains. field prefixes error field names.
func (s *Server) buildACME(ctx context.Context, st *store.Store, domains []string, in certRequest, field string) (acmeOrder, validate.Errors) {
	var errs validate.Errors
	add := func(f, code, msg string) {
		errs = append(errs, validate.FieldError{Field: field + f, Code: code, Message: msg})
	}
	var out []string
	wildcard := false
	seen := map[string]bool{}
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		if !validate.Domain(d) || !strings.Contains(d, ".") {
			add("domains", "invalid_domain", fmt.Sprintf("invalid public domain name %q", d))
		}
		wildcard = wildcard || strings.HasPrefix(d, "*.")
		out = append(out, d)
	}
	if len(out) == 0 || len(out) > 50 {
		add("domains", "required", "between 1 and 50 domains are required")
	}
	email := strings.TrimSpace(in.Email)
	if !emailOK(email) {
		add("email", "invalid", "a valid contact email is required")
	}
	challenge := in.Challenge
	if challenge == "" {
		challenge = "http-01"
		if in.DNSChallenge {
			challenge = "dns-01"
		}
	}
	o := acmeOrder{info: store.ACMEInfo{Email: email, Challenge: challenge, Staging: in.Staging}, domains: out}
	switch challenge {
	case "http-01":
		if wildcard {
			add("dns_challenge", "wildcard_needs_dns", "wildcard certificates require the DNS challenge")
		}
	case "dns-01":
		if in.DNSCredentialID != nil {
			saved, err := st.DNSCredentialByID(ctx, *in.DNSCredentialID)
			if err != nil {
				add("dns_credential_id", "not_found", "DNS credential does not exist")
				break
			}
			o.info.DNSProvider, o.credID = saved.Provider, &saved.ID
			break
		}
		provider := in.DNSProvider
		if provider == "" {
			provider = "cloudflare"
		}
		cred, err := acme.ParseCredentials(provider, credentialText(in.CredentialsINI, in.DNSCredentials))
		if err != nil {
			add("dns_credentials", "invalid", err.Error())
			break
		}
		o.credName = strings.TrimSpace(in.DNSCredentialName)
		if msg := credentialNameError(ctx, st, o.credName, 0); msg != "" {
			add("dns_credential_name", "invalid", msg)
			break
		}
		o.info.DNSProvider, o.newCred = provider, &cred
	default:
		add("challenge", "invalid", "challenge must be http-01 or dns-01")
	}
	return o, errs
}

// credentialText returns the certbot ini text of a request: dns_credentials (a string
// or {"api_token": "..."}) wins over dns_credentials_ini.
func credentialText(ini string, raw json.RawMessage) string {
	if len(raw) > 0 {
		var str string
		var obj map[string]string
		if json.Unmarshal(raw, &str) == nil {
			return str
		} else if json.Unmarshal(raw, &obj) == nil && obj["api_token"] != "" {
			return "dns_cloudflare_api_token=" + obj["api_token"]
		}
	}
	return ini
}

// credentialNameError checks a credential name (empty = automatic) against the saved
// credentials other than id.
func credentialNameError(ctx context.Context, st *store.Store, name string, id int64) string {
	if name == "" {
		return ""
	}
	if len(name) > 100 {
		return "name is too long"
	}
	list, _ := st.ListDNSCredentials(ctx)
	for _, c := range list {
		if c.ID != id && strings.EqualFold(c.Name, name) {
			return "a DNS credential with this name already exists"
		}
	}
	return ""
}

// createACMECertificate saves the order's new credential (if any) and a pending
// certificate inside tx; the returned credential is the one created, if any.
func (s *Server) createACMECertificate(ctx context.Context, tx *store.Store, name string, o acmeOrder) (*store.Certificate, *store.DNSCredential, error) {
	var created *store.DNSCredential
	credID := o.credID
	if o.newCred != nil {
		var err error
		if created, err = acme.SaveCredential(ctx, tx, s.DataKey, o.credName, o.info.DNSProvider, *o.newCred); err != nil {
			return nil, nil, err
		}
		credID = &created.ID
	}
	if name == "" {
		name = o.domains[0]
	}
	c := &store.Certificate{Name: name, Kind: "acme", Domains: o.domains, NotAfter: time.Now().UTC(), Status: "pending", AutoRenew: true, ACME: o.info, DNSCredentialID: credID}
	if err := tx.CreateCertificate(ctx, c); err != nil {
		return nil, nil, err
	}
	c.Challenge = o.info.Challenge
	return c, created, nil
}

// startOrder starts issuance of a certificate ordered with a host (after the commit)
// and records the credential that was saved with it.
func (s *Server) startOrder(r *http.Request, c *store.Certificate, created *store.DNSCredential) {
	if c == nil {
		return
	}
	if created != nil {
		s.audit(r, "dns_credential.create", fmt.Sprintf("dns_credential:%d", created.ID), created)
	}
	s.ACME.Start(c.ID)
}

func (s *Server) dnsProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, acme.Providers)
}

func (s *Server) issueACME(w http.ResponseWriter, r *http.Request) {
	if s.ACME == nil {
		writeErr(w, http.StatusNotImplemented, "unavailable", "ACME is not available")
		return
	}
	var in struct {
		certRequest
		Domains []string `json:"domains"`
		Name    string   `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	var c *store.Certificate
	var created *store.DNSCredential
	err := s.Store.WithTx(r.Context(), func(tx *store.Store) error {
		o, errs := s.buildACME(r.Context(), tx, in.Domains, in.certRequest, "")
		if len(errs) > 0 {
			return errs
		}
		var err error
		c, created, err = s.createACMECertificate(r.Context(), tx, strings.TrimSpace(in.Name), o)
		return err
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.startOrder(r, c, created)
	s.audit(r, "certificate.acme_requested", fmt.Sprintf("certificate:%d", c.ID), map[string]any{"domains": c.Domains, "challenge": c.Challenge, "staging": in.Staging, "dns_credential_id": c.DNSCredentialID})
	writeJSON(w, http.StatusAccepted, c)
}

func (s *Server) renewACME(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c, err := s.Store.CertificateByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	if c.Kind != "acme" || s.ACME == nil {
		writeErr(w, http.StatusConflict, "not_acme", "only ACME certificates can be renewed")
		return
	}
	if c.Status != "valid" {
		_ = s.Store.SetCertificateStatus(r.Context(), id, "pending", "")
		c.Status = "pending"
	}
	s.ACME.Start(id)
	s.audit(r, "certificate.acme_renew", fmt.Sprintf("certificate:%d", id), nil)
	writeJSON(w, http.StatusAccepted, c)
}
