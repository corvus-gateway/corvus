package api

import (
	"fmt"
	"net/http"
	"strings"

	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// ------------------------------------------------------------------ redirection hosts

type redirectionHostInput struct {
	Domains        []string `json:"domains"`
	ForwardScheme  string   `json:"forward_scheme"`
	ForwardDomain  string   `json:"forward_domain"`
	HTTPCode       int      `json:"http_code"`
	PreservePath   *bool    `json:"preserve_path"`
	CertificateID  *int64   `json:"certificate_id"`
	SSLForced      bool     `json:"ssl_forced"`
	HTTP2          *bool    `json:"http2"`
	BlockExploits  *bool    `json:"block_exploits"`
	AdvancedConfig string   `json:"advanced_config"`
	Enabled        *bool    `json:"enabled"`
	Notes          *string  `json:"notes"` // omitted = unchanged
	// CertificateRequest orders a new ACME certificate for the host's domains; it replaces certificate_id.
	CertificateRequest *certRequest `json:"certificate_request"`
}

func (in redirectionHostInput) apply(h *store.RedirectionHost) {
	h.Domains, h.ForwardScheme, h.ForwardDomain, h.HTTPCode = in.Domains, in.ForwardScheme, in.ForwardDomain, in.HTTPCode
	h.CertificateID, h.SSLForced, h.AdvancedConfig = in.CertificateID, in.SSLForced, in.AdvancedConfig
	h.PreservePath = in.PreservePath == nil || *in.PreservePath
	h.HTTP2 = in.HTTP2 == nil || *in.HTTP2
	h.BlockExploits = in.BlockExploits == nil || *in.BlockExploits
	h.Enabled = in.Enabled == nil || *in.Enabled
	if in.Notes != nil {
		h.Notes = *in.Notes
	}
}

// checkRedirectionHost validates a redirection host against the rest of the configuration (inside the tx).
func checkRedirectionHost(r *http.Request, tx *store.Store, h *store.RedirectionHost) error {
	if err := validate.RedirectionHost(h); err != nil {
		return err
	}
	ctx := r.Context()
	var errs validate.Errors
	if h.CertificateID != nil {
		// A pending/failed certificate is allowed: the host serves HTTP until it is issued.
		if _, err := tx.CertificateByID(ctx, *h.CertificateID); err != nil {
			errs = append(errs, validate.FieldError{Field: "certificate_id", Code: "not_found", Message: "certificate does not exist"})
		}
	}
	dup, err := domainConflicts(ctx, tx, h.Domains, 0, h.ID)
	if err != nil {
		return err
	}
	return append(errs, dup...).OrNil()
}

func (s *Server) listRedirectionHosts(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListRedirectionHosts(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) getRedirectionHost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	h, err := s.Store.RedirectionHostByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, h)
}

func (s *Server) createRedirectionHost(w http.ResponseWriter, r *http.Request) {
	var in redirectionHostInput
	if !decode(w, r, &in) {
		return
	}
	h := &store.RedirectionHost{}
	in.apply(h)
	if err := advancedAllowed(r, "", in.AdvancedConfig); err != nil {
		fail(w, err)
		return
	}
	var cert *store.Certificate
	var newCred *store.DNSCredential
	v, err := s.Deployer.Apply(r.Context(), userID(r), "create redirection host "+strings.Join(in.Domains, ","), func(tx *store.Store) error {
		if in.CertificateRequest != nil {
			var err error
			if cert, newCred, err = s.orderCertificate(r, tx, h.Domains, in.CertificateRequest); err != nil {
				return err
			}
			h.CertificateID = &cert.ID
		}
		if err := checkRedirectionHost(r, tx, h); err != nil {
			return err
		}
		return tx.SaveRedirectionHost(r.Context(), h)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.startOrder(r, cert, newCred)
	s.audit(r, "redirection_host.create", fmt.Sprintf("redirection_host:%d", h.ID), h)
	writeJSON(w, 201, map[string]any{"redirection_host": h, "version": v, "certificate": cert})
}

func (s *Server) updateRedirectionHost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in redirectionHostInput
	if !decode(w, r, &in) {
		return
	}
	var before, h *store.RedirectionHost
	var cert *store.Certificate
	var newCred *store.DNSCredential
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("update redirection host #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.RedirectionHostByID(r.Context(), id); err != nil {
			return err
		}
		cp := *before
		h = &cp
		in.apply(h)
		if err := advancedAllowed(r, before.AdvancedConfig, h.AdvancedConfig); err != nil {
			return err
		}
		if in.CertificateRequest != nil {
			if cert, newCred, err = s.orderCertificate(r, tx, h.Domains, in.CertificateRequest); err != nil {
				return err
			}
			h.CertificateID = &cert.ID
		}
		if err := checkRedirectionHost(r, tx, h); err != nil {
			return err
		}
		return tx.SaveRedirectionHost(r.Context(), h)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.startOrder(r, cert, newCred)
	s.audit(r, "redirection_host.update", fmt.Sprintf("redirection_host:%d", id), map[string]any{"before": before, "after": h})
	writeJSON(w, 200, map[string]any{"redirection_host": h, "version": v, "certificate": cert})
}

func (s *Server) deleteRedirectionHost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var before *store.RedirectionHost
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("delete redirection host #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.RedirectionHostByID(r.Context(), id); err != nil {
			return err
		}
		return tx.DeleteRedirectionHost(r.Context(), id)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "redirection_host.delete", fmt.Sprintf("redirection_host:%d", id), before)
	writeJSON(w, 200, map[string]any{"version": v})
}
