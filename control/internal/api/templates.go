package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"corvus/control/internal/sigs"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// RulesService is the managed rule bundle (body signatures + virtual-patch templates).
type RulesService interface {
	Bundle() *sigs.Bundle
	Status(ctx context.Context) sigs.Status
	Check(ctx context.Context) (sigs.Status, error)
	Install(ctx context.Context, data, sig []byte) (sigs.Status, error)
}

type templateView struct {
	sigs.Template
	NeedsExempt bool `json:"needs_exempt"`
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	out := []templateView{}
	if s.Rules != nil {
		for _, t := range s.Rules.Bundle().Templates {
			out = append(out, templateView{t, t.NeedsExempt()})
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) ruleFromTemplate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TemplateID   string  `json:"template_id"`
		Action       string  `json:"action"`
		StatusCode   int     `json:"status_code"`
		Scope        string  `json:"scope"`
		HostIDs      []int64 `json:"host_ids"`
		ExemptListID int64   `json:"exempt_list_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if s.Rules == nil {
		writeErr(w, http.StatusNotFound, "not_found", "no rule bundle")
		return
	}
	var tpl *sigs.Template
	for _, t := range s.Rules.Bundle().Templates {
		if t.ID == in.TemplateID {
			t := t
			tpl = &t
		}
	}
	if tpl == nil {
		fail(w, validate.Errors{{Field: "template_id", Code: "not_found", Message: "unknown template"}})
		return
	}
	if in.Action == "" {
		in.Action = "log" // virtual patches start in log-only mode by default
	}
	rule := tpl.Rule(in.Action, in.StatusCode, in.Scope, in.HostIDs, in.ExemptListID)
	v, err := s.Deployer.Apply(r.Context(), userID(r), "create security rule from template "+tpl.ID, func(tx *store.Store) error {
		list, err := tx.ListSecurityRules(r.Context())
		if err != nil {
			return err
		}
		if len(list) >= validate.MaxRules {
			return conflictError{fmt.Sprintf("at most %d security rules", validate.MaxRules)}
		}
		lists, err := tx.ListIPLists(r.Context())
		if err != nil {
			return err
		}
		hosts, err := tx.ListProxyHosts(r.Context())
		if err != nil {
			return err
		}
		listKind := map[int64]string{}
		for _, l := range lists {
			listKind[l.ID] = l.Kind
		}
		hostOK := map[int64]bool{}
		for _, h := range hosts {
			hostOK[h.ID] = true
		}
		if err := validate.SecurityRule(&rule, func(id int64) string { return listKind[id] }, func(id int64) bool { return hostOK[id] }); err != nil {
			return err
		}
		return tx.SaveSecurityRule(r.Context(), &rule)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "security_rule.create", fmt.Sprintf("security_rule:%d", rule.ID), map[string]any{"template": tpl.ID, "rule": rule})
	writeJSON(w, 201, map[string]any{"rule": rule, "version": v})
}

func (s *Server) managedRules(w http.ResponseWriter, r *http.Request) {
	if s.Rules == nil {
		writeErr(w, http.StatusNotFound, "not_found", "no rule bundle")
		return
	}
	writeJSON(w, 200, s.Rules.Status(r.Context()))
}

func (s *Server) putManagedRules(w http.ResponseWriter, r *http.Request) {
	var in sigs.Settings
	if !decode(w, r, &in) {
		return
	}
	in.URL = strings.TrimSpace(in.URL)
	if in.URL != "" && (!strings.HasPrefix(in.URL, "https://") || len(in.URL) > 500 || strings.ContainsAny(in.URL, " \n\r")) {
		fail(w, validate.Errors{{Field: "url", Code: "invalid", Message: "the update URL must be an https:// URL"}})
		return
	}
	raw, _ := json.Marshal(in)
	if err := s.Store.SetMeta(r.Context(), "managed_rules", string(raw)); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "managed_rules.update", "settings:managed_rules", in)
	writeJSON(w, 200, s.Rules.Status(r.Context()))
}

func (s *Server) checkManagedRules(w http.ResponseWriter, r *http.Request) {
	st, err := s.Rules.Check(r.Context())
	s.audit(r, "managed_rules.check", "managed_rules", map[string]any{"ok": err == nil, "version": st.Version})
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": "update_failed", "message": err.Error(), "status": st})
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) uploadManagedRules(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Bundle    string `json:"bundle"`    // bundle.json content
		Signature string `json:"signature"` // bundle.json.sig content (base64)
	}
	r.Body = http.MaxBytesReader(w, r.Body, 9<<20)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	st, err := s.Rules.Install(r.Context(), []byte(in.Bundle), []byte(in.Signature))
	s.audit(r, "managed_rules.upload", "managed_rules", map[string]any{"ok": err == nil, "version": st.Version})
	if err != nil {
		fail(w, validate.Errors{{Field: "bundle", Code: "rejected", Message: err.Error()}})
		return
	}
	writeJSON(w, 200, st)
}
