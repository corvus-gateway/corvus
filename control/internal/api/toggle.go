package api

import (
	"fmt"
	"net/http"

	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// ------------------------------------------------------------------ enable / disable
//
// PATCH /api/{proxy-hosts,redirection-hosts,streams,security-rules}/{id} with
// {"enabled": bool} switches one item on or off. Unlike PUT it changes nothing else, so
// a switch in a list never writes back a stale copy over another administrator's edit.
// The change is validated and deployed like an update and audited as one.

// decodeEnabled reads the id and {"enabled": bool}; false = a response was written.
func decodeEnabled(w http.ResponseWriter, r *http.Request) (int64, bool, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return 0, false, false
	}
	var in struct {
		Enabled *bool `json:"enabled"`
	}
	if !decode(w, r, &in) {
		return 0, false, false
	}
	if in.Enabled == nil {
		fail(w, validate.Errors{{Field: "enabled", Code: "required", Message: "enabled is required"}})
		return 0, false, false
	}
	return id, *in.Enabled, true
}

func onOff(on bool) string {
	if on {
		return "enable"
	}
	return "disable"
}

func (s *Server) patchProxyHost(w http.ResponseWriter, r *http.Request) {
	id, on, ok := decodeEnabled(w, r)
	if !ok {
		return
	}
	var before, h *store.ProxyHost
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("%s proxy host #%d", onOff(on), id), func(tx *store.Store) error {
		var err error
		if before, err = tx.ProxyHostByID(r.Context(), id); err != nil {
			return err
		}
		cp := *before
		h = &cp
		h.Enabled = on
		if err := checkProxyHost(r, tx, h); err != nil {
			return err
		}
		return tx.SaveProxyHost(r.Context(), h)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "proxy_host.update", fmt.Sprintf("proxy_host:%d", id), map[string]any{"before": auditHost(before), "after": auditHost(h)})
	s.underAttackChanged()
	s.signinChanged()
	writeJSON(w, 200, map[string]any{"proxy_host": h, "version": v})
}

func (s *Server) patchRedirectionHost(w http.ResponseWriter, r *http.Request) {
	id, on, ok := decodeEnabled(w, r)
	if !ok {
		return
	}
	var before, h *store.RedirectionHost
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("%s redirection host #%d", onOff(on), id), func(tx *store.Store) error {
		var err error
		if before, err = tx.RedirectionHostByID(r.Context(), id); err != nil {
			return err
		}
		cp := *before
		h = &cp
		h.Enabled = on
		if err := checkRedirectionHost(r, tx, h); err != nil {
			return err
		}
		return tx.SaveRedirectionHost(r.Context(), h)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "redirection_host.update", fmt.Sprintf("redirection_host:%d", id), map[string]any{"before": before, "after": h})
	writeJSON(w, 200, map[string]any{"redirection_host": h, "version": v})
}

func (s *Server) patchStream(w http.ResponseWriter, r *http.Request) {
	id, on, ok := decodeEnabled(w, r)
	if !ok {
		return
	}
	var before, st *store.Stream
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("%s stream #%d", onOff(on), id), func(tx *store.Store) error {
		var err error
		if before, err = tx.StreamByID(r.Context(), id); err != nil {
			return err
		}
		cp := *before
		st = &cp
		st.Enabled = on
		if err := checkStream(r, tx, s.Paths, st); err != nil {
			return err
		}
		return tx.SaveStream(r.Context(), st)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "stream.update", fmt.Sprintf("stream:%d", id), map[string]any{"before": before, "after": st})
	writeJSON(w, 200, map[string]any{"stream": st, "version": v})
}

func (s *Server) patchSecurityRule(w http.ResponseWriter, r *http.Request) {
	id, on, ok := decodeEnabled(w, r)
	if !ok {
		return
	}
	var before, rule *store.SecurityRule
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("%s security rule #%d", onOff(on), id), func(tx *store.Store) error {
		var err error
		if before, err = tx.SecurityRuleByID(r.Context(), id); err != nil {
			return err
		}
		cp := *before
		rule = &cp
		rule.Enabled = on
		return tx.SaveSecurityRule(r.Context(), rule)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "security_rule.update", fmt.Sprintf("security_rule:%d", id), map[string]any{"before": before, "after": rule})
	writeJSON(w, 200, map[string]any{"rule": rule, "version": v})
}
