package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"corvus/control/internal/alerts"
	"corvus/control/internal/seal"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

type channelInput struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Enabled     *bool    `json:"enabled"`
	MinSeverity string   `json:"min_severity"`
	Events      []string `json:"events"`
	URL         string   `json:"url"`
	Username    *string  `json:"username"` // Mattermost; omitted = unchanged
	Channel     *string  `json:"channel"`  // Mattermost; omitted = unchanged
	BotToken    string   `json:"bot_token"`
	ChatID      string   `json:"chat_id"`
	Secret      string   `json:"secret"`
	ClearSecret bool     `json:"clear_secret"` // webhook: remove the signing secret
}

// channelView is a channel as the API returns it: the destination stays sealed, but the
// optional values that are not secret are shown so they can be edited and cleared.
type channelView struct {
	store.AlertChannel
	Username  string `json:"username,omitempty"`
	Channel   string `json:"channel,omitempty"`
	HasSecret bool   `json:"has_secret,omitempty"`
}

func (s *Server) channelView(c store.AlertChannel) channelView {
	v := channelView{AlertChannel: c}
	var t alerts.Target
	if raw, err := seal.Open(s.DataKey, c.Target); err == nil && json.Unmarshal([]byte(raw), &t) == nil {
		v.Username, v.Channel, v.HasSecret = t.Username, t.Channel, t.Secret != ""
	}
	return v
}

// build fills c from the input; empty secret fields keep the existing target values.
func (s *Server) buildChannel(in channelInput, c *store.AlertChannel) error {
	var errs validate.Errors
	add := func(f, code, msg string) {
		errs = append(errs, validate.FieldError{Field: f, Code: code, Message: msg})
	}
	c.Name = strings.TrimSpace(in.Name)
	if c.Name == "" || len(c.Name) > 64 {
		add("name", "invalid", "name must be 1-64 characters")
	}
	if c.ID == 0 || in.Kind != "" {
		c.Kind = in.Kind
	}
	if !alerts.ValidKind(c.Kind) {
		add("kind", "invalid", "unsupported channel kind")
	}
	c.Enabled = in.Enabled == nil || *in.Enabled
	c.MinSeverity = in.MinSeverity
	if c.MinSeverity == "" {
		c.MinSeverity = "warning"
	}
	if !alerts.ValidSeverity(c.MinSeverity) {
		add("min_severity", "invalid", "min_severity must be info, warning or critical")
	}
	known := map[string]bool{}
	for _, r := range alerts.Catalog {
		known[r.ID] = true
	}
	c.Events = []string{}
	for _, e := range in.Events {
		if !known[e] {
			add("events", "invalid", fmt.Sprintf("unknown rule %q", e))
			continue
		}
		c.Events = append(c.Events, e)
	}

	var t alerts.Target
	if c.Target != "" { // keep existing secrets unless replaced
		if raw, err := seal.Open(s.DataKey, c.Target); err == nil {
			_ = json.Unmarshal([]byte(raw), &t)
		}
	}
	set := func(dst *string, v string) {
		if v = strings.TrimSpace(v); v != "" {
			*dst = v
		}
	}
	set(&t.URL, in.URL)
	set(&t.BotToken, in.BotToken)
	set(&t.ChatID, in.ChatID)
	set(&t.Secret, in.Secret)
	if in.ClearSecret {
		t.Secret = ""
	}
	if in.Username != nil {
		t.Username = strings.TrimSpace(*in.Username)
	}
	if in.Channel != nil {
		t.Channel = strings.TrimSpace(*in.Channel)
	}
	if c.Kind == "telegram" {
		t.URL = ""
	} else {
		t.BotToken, t.ChatID = "", ""
	}
	if c.Kind != "webhook" {
		t.Secret = ""
	}
	if c.Kind != "mattermost" {
		t.Username, t.Channel = "", ""
	}
	if err := alerts.CheckTarget(c.Kind, t); err != nil && alerts.ValidKind(c.Kind) {
		field := "url"
		if c.Kind == "telegram" {
			field = "bot_token"
		}
		add(field, "invalid", err.Error())
	}
	if len(errs) > 0 {
		return errs
	}
	raw, _ := json.Marshal(t)
	sealed, err := seal.Seal(s.DataKey, string(raw))
	if err != nil {
		return err
	}
	c.Target, c.TargetHint = sealed, alerts.Hint(c.Kind, t)
	return nil
}

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListAlertChannels(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]channelView, 0, len(list))
	for _, c := range list {
		out = append(out, s.channelView(c))
	}
	writeJSON(w, 200, out)
}

func (s *Server) createChannel(w http.ResponseWriter, r *http.Request) {
	var in channelInput
	if !decode(w, r, &in) {
		return
	}
	c := &store.AlertChannel{}
	if err := s.buildChannel(in, c); err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.SaveAlertChannel(r.Context(), c); err != nil {
		if store.IsUnique(err) {
			fail(w, validate.Errors{{Field: "name", Code: "duplicate", Message: "a channel with this name already exists"}})
			return
		}
		fail(w, err)
		return
	}
	s.audit(r, "alert_channel.create", fmt.Sprintf("alert_channel:%d", c.ID), map[string]any{"name": c.Name, "kind": c.Kind, "target": c.TargetHint})
	writeJSON(w, 201, map[string]any{"channel": s.channelView(*c)})
}

func (s *Server) updateChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in channelInput
	if !decode(w, r, &in) {
		return
	}
	c, err := s.Store.AlertChannelByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	if in.Kind != "" && in.Kind != c.Kind {
		c.Target = "" // a different kind needs new destination details
	}
	if err := s.buildChannel(in, c); err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.SaveAlertChannel(r.Context(), c); err != nil {
		if store.IsUnique(err) {
			fail(w, validate.Errors{{Field: "name", Code: "duplicate", Message: "a channel with this name already exists"}})
			return
		}
		fail(w, err)
		return
	}
	s.audit(r, "alert_channel.update", fmt.Sprintf("alert_channel:%d", id), map[string]any{"name": c.Name, "kind": c.Kind, "target": c.TargetHint, "enabled": c.Enabled})
	writeJSON(w, 200, map[string]any{"channel": s.channelView(*c)})
}

func (s *Server) deleteChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteAlertChannel(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "alert_channel.delete", fmt.Sprintf("alert_channel:%d", id), nil)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) testChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c, err := s.Store.AlertChannelByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	res := s.Alerts.Test(r.Context(), *c)
	s.audit(r, "alert_channel.test", fmt.Sprintf("alert_channel:%d", id), map[string]any{"ok": res.OK})
	writeJSON(w, 200, map[string]any{"ok": res.OK, "error": res.Error})
}

func (s *Server) getAlertRules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"settings": alerts.Load(r.Context(), s.Store), "rules": alerts.Catalog})
}

func (s *Server) putAlertRules(w http.ResponseWriter, r *http.Request) {
	var in alerts.Settings
	if !decode(w, r, &in) {
		return
	}
	if in.Rules == nil {
		in.Rules = map[string]alerts.Rule{}
	}
	if err := in.Validate(); err != nil {
		fail(w, validate.Errors{{Field: "settings", Code: "invalid", Message: err.Error()}})
		return
	}
	raw, _ := json.Marshal(in)
	if err := s.Store.SetMeta(r.Context(), "alert_settings", string(raw)); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "alert_rules.update", "settings:alerts", in)
	writeJSON(w, 200, map[string]any{"settings": alerts.Load(r.Context(), s.Store), "rules": alerts.Catalog})
}

func (s *Server) alertHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	list, err := s.Store.ListAlerts(r.Context(), limit)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) alertActive(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Alerts.Active())
}
