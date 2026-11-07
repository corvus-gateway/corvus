// Package alerts detects anomalies (traffic, security, system) and notifies
// messengers: Slack, Mattermost, Discord, Microsoft Teams, Telegram and
// generic signed webhooks.
package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"corvus/control/internal/store"
)

// Severity levels, ordered.
var severities = map[string]int{"info": 0, "warning": 1, "critical": 2}

func ValidSeverity(s string) bool { _, ok := severities[s]; return ok }

// atLeast reports whether s is at or above min.
func atLeast(s, min string) bool { return severities[s] >= severities[min] }

// Rule holds the tunable parameters of a detector.
type Rule struct {
	Enabled     bool    `json:"enabled"`
	Severity    string  `json:"severity"`
	Threshold   float64 `json:"threshold,omitempty"`
	WindowS     int     `json:"window_s,omitempty"`
	MinRequests int     `json:"min_requests,omitempty"`
}

type Settings struct {
	CooldownS    int             `json:"cooldown_s"`
	SendResolved bool            `json:"send_resolved"`
	Language     string          `json:"language"`
	PublicURL    string          `json:"public_url"`
	Rules        map[string]Rule `json:"rules"`
}

// RuleInfo describes a rule for the UI.
type RuleInfo struct {
	ID      string   `json:"id"`
	Default Rule     `json:"default"`
	Params  []string `json:"params"`
	Unit    string   `json:"unit,omitempty"`
}

var Catalog = []RuleInfo{
	{"error_rate", Rule{true, "critical", 5, 300, 50}, []string{"threshold", "window_s", "min_requests"}, "%"},
	{"latency_p95", Rule{true, "warning", 2000, 300, 50}, []string{"threshold", "window_s", "min_requests"}, "ms"},
	{"traffic_spike", Rule{true, "warning", 5, 0, 20}, []string{"threshold", "min_requests"}, "x"},
	{"rate_limited", Rule{true, "warning", 100, 300, 0}, []string{"threshold", "window_s"}, ""},
	{"exploit", Rule{true, "warning", 20, 300, 0}, []string{"threshold", "window_s"}, ""},
	{"security_events", Rule{true, "warning", 20, 300, 0}, []string{"threshold", "window_s"}, ""},
	{"attack_source", Rule{true, "warning", 10, 600, 0}, []string{"threshold", "window_s"}, ""},
	{"scan_suspect", Rule{true, "warning", 100, 300, 0}, []string{"threshold", "window_s"}, ""},
	{"waitroom_queue", Rule{true, "info", 1, 0, 0}, []string{"threshold"}, ""},
	{"redis_down", Rule{true, "critical", 0, 0, 0}, nil, ""},
	{"cert_expiry", Rule{true, "warning", 14, 0, 0}, []string{"threshold"}, "d"},
	{"acme_failed", Rule{true, "warning", 0, 0, 0}, nil, ""},
	{"deploy_failed", Rule{true, "warning", 0, 0, 0}, nil, ""},
	{"login_failed", Rule{true, "warning", 10, 600, 0}, []string{"threshold", "window_s"}, ""},
	{"under_attack", Rule{true, "warning", 0, 0, 0}, nil, ""},
}

func DefaultSettings() Settings {
	s := Settings{CooldownS: 900, SendResolved: true, Language: "en", Rules: map[string]Rule{}}
	for _, r := range Catalog {
		s.Rules[r.ID] = r.Default
	}
	return s
}

// Load returns stored settings merged over the defaults (new rules get defaults).
func Load(ctx context.Context, st *store.Store) Settings {
	s := DefaultSettings()
	raw, err := st.GetMeta(ctx, "alert_settings")
	if err != nil || raw == "" {
		return s
	}
	var saved Settings
	if json.Unmarshal([]byte(raw), &saved) != nil {
		return s
	}
	if saved.CooldownS > 0 {
		s.CooldownS = saved.CooldownS
	}
	s.SendResolved, s.PublicURL = saved.SendResolved, saved.PublicURL
	if saved.Language != "" {
		s.Language = saved.Language
	}
	for id, r := range saved.Rules {
		if _, ok := s.Rules[id]; ok {
			s.Rules[id] = r
		}
	}
	return s
}

// Validate normalizes and checks settings before they are stored.
func (s *Settings) Validate() error {
	if s.CooldownS < 60 || s.CooldownS > 86400 {
		return fmt.Errorf("cooldown must be between 60 and 86400 seconds")
	}
	if s.Language != "ko" && s.Language != "en" {
		return fmt.Errorf("language must be ko or en")
	}
	s.PublicURL = strings.TrimRight(strings.TrimSpace(s.PublicURL), "/")
	if s.PublicURL != "" {
		u, err := url.Parse(s.PublicURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("public_url must be an http(s) URL")
		}
	}
	def := DefaultSettings()
	for id, r := range s.Rules {
		if _, ok := def.Rules[id]; !ok {
			return fmt.Errorf("unknown rule %q", id)
		}
		if !ValidSeverity(r.Severity) {
			return fmt.Errorf("rule %s: invalid severity", id)
		}
		if r.Threshold < 0 || r.Threshold > 1e9 || r.WindowS < 0 || r.WindowS > 86400 || r.MinRequests < 0 || r.MinRequests > 1e9 {
			return fmt.Errorf("rule %s: value out of range", id)
		}
		if r.WindowS > 0 && r.WindowS < 60 {
			return fmt.Errorf("rule %s: window must be at least 60 seconds", id)
		}
	}
	for id, r := range def.Rules { // keep rules that were omitted
		if _, ok := s.Rules[id]; !ok {
			s.Rules[id] = r
		}
	}
	return nil
}
