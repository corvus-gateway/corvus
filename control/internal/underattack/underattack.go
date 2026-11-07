// Package underattack runs the automatic part of the per-host "under attack" mode:
// it watches each auto-mode host's traffic, activates the mode (every visitor must
// pass a challenge, gateway corvus/underattack.lua) when a threshold is exceeded and
// pushes the active hosts to the gateway's shared memory over the loopback status
// server — no redeployment. Activations are stored (meta "under_attack"), so they
// survive restarts of corvus-control.
//
// Hysteresis: an activation lasts DurationS seconds after the last evaluation round
// in which the host was still over a threshold, so it ends only once the traffic has
// stayed below the thresholds for the whole period.
package underattack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"corvus/control/internal/autoban"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// MinRatioRequests is the least number of requests in the window before the blocked
// share is considered (a handful of blocked requests on an idle host is no attack).
const MinRatioRequests = 50

const metaKey = "under_attack"

// Sampler provides per-host traffic (traffic.Collector.Window).
type Sampler interface {
	Window(host string, d time.Duration, now time.Time) (requests, blocked, secs int64)
}

// Activation is an automatic activation of a host's mode.
type Activation struct {
	Host   string    `json:"host"`
	Since  time.Time `json:"since"`
	Until  time.Time `json:"until"` // extended while the host stays over a threshold
	Reason string    `json:"reason"`
}

type persisted struct {
	Active map[int64]*Activation `json:"active"`
	// Manual deactivation: no automatic activation of the host before this time.
	Hold map[int64]time.Time `json:"hold"`
}

// Transition is an automatic activation or deactivation (reported as an alert).
type Transition struct {
	HostID int64
	Host   string
	On     bool
	Manual bool // deactivated by an operator
	Reason string
	Since  time.Time
	Until  time.Time
	At     time.Time
}

// Status is a host's under-attack state for the API.
type Status struct {
	HostID int64      `json:"host_id"`
	Host   string     `json:"host"`
	Mode   string     `json:"mode"`   // on | auto
	Active bool       `json:"active"` // challenging every visitor now
	Since  *time.Time `json:"since,omitempty"`
	Until  *time.Time `json:"until,omitempty"`
	Reason string     `json:"reason,omitempty"`
}

// ErrNotActive: the host has no automatic activation to end.
var ErrNotActive = errors.New("under attack mode is not automatically active for this host")

type Service struct {
	Store   *store.Store
	Traffic Sampler
	PushURL string // gateway status endpoint, e.g. http://127.0.0.1:9180/under-attack
	Client  *http.Client

	mu      sync.Mutex
	loaded  bool
	st      persisted
	pending []Transition
	trigger chan struct{}
}

func (s *Service) load(ctx context.Context) {
	if s.loaded {
		return
	}
	s.st = persisted{}
	if v, err := s.Store.GetMeta(ctx, metaKey); err == nil && v != "" {
		if err := json.Unmarshal([]byte(v), &s.st); err != nil {
			slog.Warn("under attack state unreadable, starting empty", "err", err)
		}
	}
	if s.st.Active == nil {
		s.st.Active = map[int64]*Activation{}
	}
	if s.st.Hold == nil {
		s.st.Hold = map[int64]time.Time{}
	}
	s.loaded = true
}

func (s *Service) save(ctx context.Context) {
	b, _ := json.Marshal(s.st)
	if err := s.Store.SetMeta(ctx, metaKey, string(b)); err != nil {
		slog.Error("under attack state not saved", "err", err)
	}
}

// over reports whether the host exceeds a threshold, and why.
func (s *Service) over(h *store.ProxyHost, c store.UnderAttackConfig, now time.Time) (bool, string) {
	if s.Traffic == nil {
		return false, ""
	}
	req, blocked, secs := s.Traffic.Window(h.Domains[0], time.Duration(c.WindowS)*time.Second, now)
	if secs <= 0 {
		return false, ""
	}
	if rps := float64(req) / float64(secs); rps > c.RPS {
		return true, fmt.Sprintf("%.0f requests/s over %d s (threshold %g)", rps, secs, c.RPS)
	}
	if pct := *c.BlockedPct; pct > 0 && req >= MinRatioRequests {
		if share := float64(blocked) * 100 / float64(req); share >= pct {
			return true, fmt.Sprintf("%.0f%% of %d requests blocked or rate-limited over %d s (threshold %g%%)", share, req, secs, pct)
		}
	}
	return false, ""
}

// Evaluate runs one round: activates auto-mode hosts over a threshold, extends active
// ones still over it and ends those whose period has passed. Returns the transitions.
func (s *Service) Evaluate(ctx context.Context, now time.Time) []Transition {
	hosts, err := s.Store.ListProxyHosts(ctx)
	if err != nil {
		slog.Debug("under attack: list hosts", "err", err)
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load(ctx)
	changed := false
	seen := map[int64]bool{}
	var out []Transition
	for i := range hosts {
		h := &hosts[i]
		if !h.Enabled || h.UnderAttack != "auto" || len(h.Domains) == 0 {
			continue
		}
		seen[h.ID] = true
		c := h.UnderAttackConfig
		validate.UnderAttackDefaults(&c)
		period := time.Duration(c.DurationS) * time.Second
		hot, why := s.over(h, c, now)
		a := s.st.Active[h.ID]
		switch {
		case a != nil && hot:
			a.Until, a.Host = now.Add(period).UTC(), h.Domains[0]
			changed = true
		case a != nil && !now.Before(a.Until):
			delete(s.st.Active, h.ID)
			changed = true
			out = append(out, Transition{HostID: h.ID, Host: h.Domains[0], Reason: a.Reason, Since: a.Since, Until: a.Until, At: now})
			slog.Warn("under attack mode deactivated", "host", h.Domains[0], "since", a.Since)
		case a == nil && hot:
			if hold, ok := s.st.Hold[h.ID]; ok && now.Before(hold) {
				continue
			}
			a = &Activation{Host: h.Domains[0], Since: now.UTC(), Until: now.Add(period).UTC(), Reason: why}
			s.st.Active[h.ID] = a
			changed = true
			out = append(out, Transition{HostID: h.ID, Host: a.Host, On: true, Reason: why, Since: a.Since, Until: a.Until, At: now})
			slog.Warn("under attack mode activated", "host", a.Host, "reason", why, "until", a.Until)
		}
	}
	// hosts deleted, disabled or switched away from auto: their activation ends silently
	for id := range s.st.Active {
		if !seen[id] {
			delete(s.st.Active, id)
			changed = true
		}
	}
	for id, t := range s.st.Hold {
		if !now.Before(t) || !seen[id] {
			delete(s.st.Hold, id)
			changed = true
		}
	}
	if changed {
		s.save(ctx)
	}
	for _, t := range out {
		_ = s.Store.Audit(ctx, nil, map[bool]string{true: "under_attack.activate", false: "under_attack.deactivate"}[t.On],
			fmt.Sprintf("proxy_host:%d", t.HostID), map[string]any{"host": t.Host, "reason": t.Reason, "until": t.Until.Format(time.RFC3339)}, "")
	}
	s.pending = append(s.pending, out...)
	return out
}

// Deactivate ends an automatic activation now. The host is not activated again
// automatically for one activation period (it would otherwise come straight back
// while the traffic is still high).
func (s *Service) Deactivate(ctx context.Context, hostID int64, now time.Time) error {
	h, err := s.Store.ProxyHostByID(ctx, hostID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.load(ctx)
	a := s.st.Active[hostID]
	if a == nil {
		s.mu.Unlock()
		return ErrNotActive
	}
	c := h.UnderAttackConfig
	validate.UnderAttackDefaults(&c)
	delete(s.st.Active, hostID)
	s.st.Hold[hostID] = now.Add(time.Duration(c.DurationS) * time.Second).UTC()
	s.save(ctx)
	s.pending = append(s.pending, Transition{HostID: hostID, Host: a.Host, Manual: true, Reason: a.Reason, Since: a.Since, Until: a.Until, At: now})
	s.mu.Unlock()
	s.Trigger()
	return nil
}

// Drain returns and forgets the transitions since the last call (alerts).
func (s *Service) Drain() []Transition {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.pending
	s.pending = nil
	return out
}

// active returns the automatic activations still running at now.
func (s *Service) active(ctx context.Context, now time.Time) map[int64]Activation {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load(ctx)
	out := map[int64]Activation{}
	for id, a := range s.st.Active {
		if now.Before(a.Until) {
			out[id] = *a
		}
	}
	return out
}

// Statuses lists every host whose mode is not off, sorted by host id.
func (s *Service) Statuses(ctx context.Context, now time.Time) ([]Status, error) {
	hosts, err := s.Store.ListProxyHosts(ctx)
	if err != nil {
		return nil, err
	}
	act := s.active(ctx, now)
	out := []Status{}
	for _, h := range hosts {
		if h.UnderAttack != "on" && h.UnderAttack != "auto" || len(h.Domains) == 0 {
			continue
		}
		st := Status{HostID: h.ID, Host: h.Domains[0], Mode: h.UnderAttack, Active: h.Enabled && h.UnderAttack == "on"}
		if a, ok := act[h.ID]; ok && h.Enabled && h.UnderAttack == "auto" {
			since, until := a.Since, a.Until
			st.Active, st.Since, st.Until, st.Reason = true, &since, &until, a.Reason
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HostID < out[j].HostID })
	return out, nil
}

// Payload is the body pushed to the gateway (PUT /under-attack):
//
//	{"hosts": {"<proxy host id>": <activation ends, unix>}, "exempt": ["<hex net>/<prefix>", ...], "now": <unix>}
//
// It replaces the gateway's set. Exempt networks (automatic-ban exemptions and
// loopback) are never challenged by the mode, whether it is on or auto.
func (s *Service) Payload(ctx context.Context, now time.Time) map[string]any {
	hosts := map[string]int64{}
	for id, a := range s.active(ctx, now) {
		hosts[strconv.FormatInt(id, 10)] = a.Until.Unix()
	}
	ex := autoban.Load(ctx, s.Store).Exempt
	return map[string]any{"hosts": hosts, "now": now.Unix(),
		"exempt": autoban.EncodeNets(append([]string{"127.0.0.0/8", "::1/128"}, ex...))}
}

// Push sends the active hosts to the gateway.
func (s *Service) Push(ctx context.Context, now time.Time) error {
	if s.PushURL == "" {
		return nil
	}
	body, _ := json.Marshal(s.Payload(ctx, now))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.PushURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c := s.Client
	if c == nil {
		c = &http.Client{Timeout: 3 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("gateway answered %d", resp.StatusCode)
	}
	return nil
}

// Trigger evaluates and pushes soon (after host changes or a manual deactivation).
func (s *Service) Trigger() {
	s.mu.Lock()
	if s.trigger == nil {
		s.trigger = make(chan struct{}, 1)
	}
	ch := s.trigger
	s.mu.Unlock()
	select {
	case ch <- struct{}{}:
	default:
	}
}

// Run evaluates and pushes every 10 seconds (and on Trigger) until ctx is done. The
// push is a full replacement, so a restarted gateway is back in sync within a round.
func (s *Service) Run(ctx context.Context) {
	s.Trigger()
	s.mu.Lock()
	ch := s.trigger
	s.mu.Unlock()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-ch:
		}
		now := time.Now()
		s.Evaluate(ctx, now)
		if err := s.Push(ctx, now); err != nil {
			slog.Debug("under attack push failed", "err", err)
		}
	}
}
