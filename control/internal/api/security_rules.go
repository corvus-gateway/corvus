package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"corvus/control/internal/feeds"
	"corvus/control/internal/geoip"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// GeoIPService is the country database used by security rules (internal/geoip).
type GeoIPService interface {
	Status() geoip.Status
	Refresh(ctx context.Context) (geoip.Status, error)
}

type ruleInput struct {
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Enabled       *bool            `json:"enabled"`
	Action        string           `json:"action"`
	StatusCode    int              `json:"status_code"`
	SkipBasicAuth bool             `json:"skip_basic_auth"`
	ChallengeMode string           `json:"challenge_mode"`
	ChallengeDiff int              `json:"challenge_difficulty"`
	ChallengeTTL  int              `json:"challenge_ttl_s"`
	ChallengeBind string           `json:"challenge_bind"`
	Scope         string           `json:"scope"`
	HostIDs       []int64          `json:"host_ids"`
	Expression    store.Expression `json:"expression"`
	// read-only fields the UI may send back unchanged
	ID        int64      `json:"id"`
	Position  int        `json:"position"`
	Hits24h   int64      `json:"hits_24h"`
	LastHitAt *time.Time `json:"last_hit_at"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func (in ruleInput) build(ctx context.Context, tx *store.Store, r *store.SecurityRule) error {
	r.Name, r.Description, r.Action, r.StatusCode, r.SkipBasicAuth = in.Name, in.Description, in.Action, in.StatusCode, in.SkipBasicAuth
	r.Scope, r.HostIDs, r.Expression = in.Scope, in.HostIDs, in.Expression
	r.ChallengeMode, r.ChallengeDifficulty, r.ChallengeTTL, r.ChallengeBind = in.ChallengeMode, in.ChallengeDiff, in.ChallengeTTL, in.ChallengeBind
	if in.Enabled != nil {
		r.Enabled = *in.Enabled
	} else if r.ID == 0 {
		r.Enabled = true
	}
	lists, err := tx.ListIPLists(ctx)
	if err != nil {
		return err
	}
	listKind := map[int64]string{}
	for _, l := range lists {
		listKind[l.ID] = l.Kind
	}
	hosts, err := tx.ListProxyHosts(ctx)
	if err != nil {
		return err
	}
	hostIDs := map[int64]bool{}
	for _, h := range hosts {
		hostIDs[h.ID] = true
	}
	return validate.SecurityRule(r, func(id int64) string { return listKind[id] }, func(id int64) bool { return hostIDs[id] })
}

func (s *Server) withHits(list []store.SecurityRule) {
	if s.Traffic == nil {
		return
	}
	hits := s.Traffic.RuleHits(time.Now())
	for i := range list {
		if h, ok := hits[list[i].ID]; ok {
			list[i].Hits24h, list[i].LastHitAt = h.Hits24h, h.Last
		}
	}
}

func (s *Server) listSecurityRules(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListSecurityRules(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	s.withHits(list)
	writeJSON(w, 200, list)
}

func (s *Server) createSecurityRule(w http.ResponseWriter, r *http.Request) {
	var in ruleInput
	if !decode(w, r, &in) {
		return
	}
	rule := &store.SecurityRule{}
	v, err := s.Deployer.Apply(r.Context(), userID(r), "create security rule "+in.Name, func(tx *store.Store) error {
		list, err := tx.ListSecurityRules(r.Context())
		if err != nil {
			return err
		}
		if len(list) >= validate.MaxRules {
			return conflictError{fmt.Sprintf("at most %d security rules", validate.MaxRules)}
		}
		if err := in.build(r.Context(), tx, rule); err != nil {
			return err
		}
		return tx.SaveSecurityRule(r.Context(), rule)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "security_rule.create", fmt.Sprintf("security_rule:%d", rule.ID), rule)
	writeJSON(w, 201, map[string]any{"rule": rule, "version": v})
}

func (s *Server) updateSecurityRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in ruleInput
	if !decode(w, r, &in) {
		return
	}
	var before, rule *store.SecurityRule
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("update security rule #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.SecurityRuleByID(r.Context(), id); err != nil {
			return err
		}
		cp := *before
		rule = &cp
		if err := in.build(r.Context(), tx, rule); err != nil {
			return err
		}
		return tx.SaveSecurityRule(r.Context(), rule)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "security_rule.update", fmt.Sprintf("security_rule:%d", id), map[string]any{"before": before, "after": rule})
	writeJSON(w, 200, map[string]any{"rule": rule, "version": v})
}

func (s *Server) deleteSecurityRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var before *store.SecurityRule
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("delete security rule #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.SecurityRuleByID(r.Context(), id); err != nil {
			return err
		}
		return tx.DeleteSecurityRule(r.Context(), id)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "security_rule.delete", fmt.Sprintf("security_rule:%d", id), before)
	writeJSON(w, 200, map[string]any{"version": v})
}

func (s *Server) reorderSecurityRules(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := s.Deployer.Apply(r.Context(), userID(r), "reorder security rules", func(tx *store.Store) error {
		if err := tx.ReorderSecurityRules(r.Context(), in.IDs); err != nil {
			return validate.Errors{{Field: "ids", Code: "invalid", Message: err.Error()}}
		}
		return nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "security_rule.reorder", "security_rules", in.IDs)
	writeJSON(w, 200, map[string]any{"version": v})
}

func (s *Server) securityRuleEvents(w http.ResponseWriter, r *http.Request) {
	if s.Traffic == nil {
		writeJSON(w, 200, []any{})
		return
	}
	ruleID, _ := strconv.ParseInt(r.URL.Query().Get("rule_id"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events := s.Traffic.RuleEvents(ruleID, limit)
	if rules, err := s.Store.ListSecurityRules(r.Context()); err == nil {
		names := map[int64]string{}
		for _, x := range rules {
			names[x.ID] = x.Name
		}
		for i := range events {
			if events[i].RuleID != 0 {
				events[i].Rule = names[events[i].RuleID]
			}
		}
	}
	writeJSON(w, 200, events)
}

func (s *Server) geoipStatus(w http.ResponseWriter, r *http.Request) {
	if s.GeoIP == nil {
		writeJSON(w, 200, geoip.Status{Attribution: geoip.Attribution, Disabled: true})
		return
	}
	writeJSON(w, 200, s.GeoIP.Status())
}

func (s *Server) geoipRefresh(w http.ResponseWriter, r *http.Request) {
	if s.GeoIP == nil {
		writeErr(w, http.StatusConflict, "conflict", "country database is not available")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	st, err := s.GeoIP.Refresh(ctx)
	s.audit(r, "geoip.refresh", "geoip", map[string]any{"ok": err == nil, "source": st.Source})
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": "geoip_failed", "message": err.Error(), "status": st})
		return
	}
	writeJSON(w, 200, st)
}

// ------------------------------------------------------------------ IP lists

type ipListInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Kind        string   `json:"kind"`
	Items       []string `json:"items"` // ignored for lists with a source URL
	SourceURL   string   `json:"source_url"`
	RefreshS    int      `json:"refresh_s"`
	// read-only fields the UI may send back unchanged
	ID           int64      `json:"id"`
	Rules        int        `json:"rules"`
	FetchedAt    *time.Time `json:"fetched_at"`
	CheckedAt    *time.Time `json:"checked_at"`
	FetchError   string     `json:"fetch_error"`
	FetchWarning string     `json:"fetch_warning"`
	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at"`
}

// ListFeeds refreshes lists that have a source URL (internal/feeds).
type ListFeeds interface {
	Refresh(ctx context.Context, id int64) (*store.IPList, error)
	Kick(id int64)
}

// decodeLarge is decode with a larger body limit (IP lists may hold 50,000 entries).
func decodeLarge(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func (s *Server) listIPLists(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListIPLists(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) saveIPList(w http.ResponseWriter, r *http.Request, id int64) {
	var in ipListInput
	if !decodeLarge(w, r, &in) {
		return
	}
	l := &store.IPList{ID: id, Name: in.Name, Description: in.Description, Kind: in.Kind, Items: in.Items,
		SourceURL: in.SourceURL, RefreshS: in.RefreshS}
	if l.Items == nil || l.SourceURL != "" {
		l.Items = []string{} // feed lists keep their fetched entries (below)
	}
	msg := "create list " + in.Name
	if id != 0 {
		msg = fmt.Sprintf("update list #%d", id)
	}
	fetch := false
	v, err := s.Deployer.Apply(r.Context(), userID(r), msg, func(tx *store.Store) error {
		var old *store.IPList
		if id != 0 {
			var err error
			if old, err = tx.IPListByID(r.Context(), id); err != nil {
				return err
			}
			if l.Kind == "" {
				l.Kind = old.Kind
			}
			if l.Kind != old.Kind && old.Rules > 0 {
				return validate.Errors{{Field: "kind", Code: "in_use", Message: "the kind of a list used by security rules cannot be changed"}}
			}
		}
		if err := validate.IPList(l); err != nil {
			return err
		}
		reset := false
		if l.SourceURL != "" {
			if old != nil && old.SourceURL == l.SourceURL && old.Kind == l.Kind {
				l.Items = old.Items
			} else {
				reset, fetch = old != nil, true
			}
		} else if old != nil && old.SourceURL != "" {
			reset = true
		}
		if err := tx.SaveIPList(r.Context(), l); err != nil {
			if store.IsUnique(err) {
				return validate.Errors{{Field: "name", Code: "duplicate", Message: "a list with this name already exists"}}
			}
			return err
		}
		if reset {
			return tx.ResetIPListFetch(r.Context(), l.ID)
		}
		return nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	if fetch && s.Feeds != nil {
		s.Feeds.Kick(l.ID) // first fetch in the background
	}
	if cur, err := s.Store.IPListByID(r.Context(), l.ID); err == nil {
		l = cur
	}
	code, action := 201, "ip_list.create"
	if id != 0 {
		code, action = 200, "ip_list.update"
	}
	s.audit(r, action, fmt.Sprintf("ip_list:%d", l.ID), map[string]any{"name": l.Name, "kind": l.Kind, "items": len(l.Items), "source_url": l.SourceURL})
	writeJSON(w, code, map[string]any{"ip_list": l, "version": v})
}

func (s *Server) createIPList(w http.ResponseWriter, r *http.Request) { s.saveIPList(w, r, 0) }

func (s *Server) updateIPList(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r); ok {
		s.saveIPList(w, r, id)
	}
}

// refreshIPList fetches a feed list now. A failed fetch is reported in fetch_error
// (the previous entries stay in effect).
func (s *Server) refreshIPList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.Feeds == nil {
		writeErr(w, http.StatusServiceUnavailable, "unavailable", "list feeds are not available")
		return
	}
	l, err := s.Feeds.Refresh(r.Context(), id)
	if l == nil {
		fail(w, err)
		return
	}
	if errors.Is(err, feeds.ErrNoFeed) {
		writeErr(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	s.audit(r, "ip_list.refresh", fmt.Sprintf("ip_list:%d", id), map[string]any{"items": len(l.Items), "error": l.FetchError})
	writeJSON(w, 200, map[string]any{"ip_list": l})
}

func (s *Server) deleteIPList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("delete IP list #%d", id), func(tx *store.Store) error {
		if _, err := tx.IPListByID(r.Context(), id); err != nil {
			return err
		}
		if err := tx.DeleteIPList(r.Context(), id); err != nil {
			return conflictError{err.Error()}
		}
		return nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "ip_list.delete", fmt.Sprintf("ip_list:%d", id), nil)
	writeJSON(w, 200, map[string]any{"version": v})
}
