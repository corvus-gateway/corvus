// Package autoban bans client IPs that keep triggering security detections
// (fail2ban-style) and pushes the active ban list to the gateway's shared memory.
package autoban

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"corvus/control/internal/netcfg"
	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
)

// Settings are stored in meta "autoban".
type Settings struct {
	Enabled        bool     `json:"enabled"`
	Threshold      int      `json:"threshold"`       // detections from one IP within the window
	WindowS        int      `json:"window_s"`        // detection window
	DurationS      int      `json:"duration_s"`      // first ban; doubles for repeat offenders up to MaxDurationS
	MaxDurationS   int      `json:"max_duration_s"`  // longest automatic ban (up to 30 days)
	PermanentAfter int      `json:"permanent_after"` // ban permanently from this offense on; 0 = never
	IncludeScans   bool     `json:"include_scans"`   // also ban "possible scanner" IPs (many 4xx)
	ScanThreshold  int      `json:"scan_threshold"`
	Exempt         []string `json:"exempt"` // CIDRs never banned automatically
}

func DefaultSettings() Settings {
	return Settings{Enabled: true, Threshold: 20, WindowS: 600, DurationS: 900, MaxDurationS: 7 * 86400, IncludeScans: false, ScanThreshold: 200, Exempt: []string{}}
}

// maxDuration is the upper limit for any automatic ban; offense history is kept as long
// so repeat offenders are recognised.
const maxDuration = 30 * 24 * time.Hour

// Detection kinds that count toward a ban (bans themselves do not).
var Kinds = []string{"blocked_exploit", "exploit_detected", "blocked_rule", "rule_logged", "blocked_body", "body_detected"}

func Load(ctx context.Context, s *store.Store) Settings {
	set := DefaultSettings()
	if v, _ := s.GetMeta(ctx, "autoban"); v != "" {
		_ = json.Unmarshal([]byte(v), &set)
	}
	if set.Exempt == nil {
		set.Exempt = []string{}
	}
	return set
}

// Validate normalises s.
func Validate(s *Settings) error {
	if s.Threshold < 3 || s.Threshold > 100000 {
		return fmt.Errorf("threshold must be 3-100000")
	}
	if s.WindowS < 60 || s.WindowS > 86400 {
		return fmt.Errorf("window must be 60-86400 seconds")
	}
	if s.DurationS < 60 || s.DurationS > int(maxDuration.Seconds()) {
		return fmt.Errorf("ban duration must be 60 seconds to 30 days")
	}
	if s.MaxDurationS == 0 {
		s.MaxDurationS = 7 * 86400
	}
	if s.MaxDurationS < s.DurationS || s.MaxDurationS > int(maxDuration.Seconds()) {
		return fmt.Errorf("maximum ban duration must be between the first ban duration and 30 days")
	}
	if s.PermanentAfter != 0 && (s.PermanentAfter < 2 || s.PermanentAfter > 100) {
		return fmt.Errorf("permanent ban must start at offense 2-100 (0 = never)")
	}
	if s.ScanThreshold == 0 {
		s.ScanThreshold = 200
	}
	if s.ScanThreshold < 20 {
		return fmt.Errorf("scanner threshold must be at least 20")
	}
	ex, err := netcfg.NormalizeCIDRs(s.Exempt)
	if err != nil {
		return err
	}
	s.Exempt = ex
	return nil
}

type Service struct {
	Store   *store.Store
	Traffic *traffic.Collector
	PushURL string // gateway status endpoint, e.g. http://127.0.0.1:9180/bans
	Client  *http.Client

	mu      sync.Mutex
	trigger chan struct{}
}

// Trigger re-evaluates and pushes soon (after manual changes).
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

func (s *Service) Run(ctx context.Context) {
	s.Trigger()
	s.mu.Lock()
	ch := s.trigger
	s.mu.Unlock()
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	lastPrune := time.Time{}
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
			slog.Debug("ban push failed", "err", err)
		}
		if now.Sub(lastPrune) > time.Hour {
			_ = s.Store.PruneBans(ctx, now.Add(-maxDuration))
			lastPrune = now
		}
	}
}

func exempt(ip net.IP, nets []*net.IPNet) bool {
	if ip == nil || ip.IsLoopback() {
		return true
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Evaluate bans IPs over the thresholds; returns the IPs banned in this round.
func (s *Service) Evaluate(ctx context.Context, now time.Time) []string {
	set := Load(ctx, s.Store)
	if !set.Enabled || s.Traffic == nil {
		return nil
	}
	var nets []*net.IPNet
	for _, c := range set.Exempt {
		if _, n, err := net.ParseCIDR(c); err == nil {
			nets = append(nets, n)
		}
	}
	active := map[string]bool{}
	if list, err := s.Store.ActiveBans(ctx, now); err == nil {
		for _, b := range list {
			active[b.IP] = true
		}
	}
	type cand struct {
		ip, reason string
		n          int64
	}
	var cands []cand
	window := time.Duration(set.WindowS) * time.Second
	for ip, a := range s.Traffic.EventsByIP(now.Add(-window), Kinds...) {
		if a.Count >= int64(set.Threshold) {
			kinds := make([]string, 0, len(a.Kinds))
			for k := range a.Kinds {
				kinds = append(kinds, k)
			}
			sort.Strings(kinds)
			cands = append(cands, cand{ip, fmt.Sprintf("%d security detections in %s (%s)", a.Count, window, strings.Join(kinds, ", ")), a.Count})
		}
	}
	if set.IncludeScans {
		for ip, a := range s.Traffic.IPCounts(window, now) {
			if a.S4xx >= int64(set.ScanThreshold) && a.S4xx*2 >= a.Requests {
				cands = append(cands, cand{ip, fmt.Sprintf("possible scanner: %d of %d responses were 4xx in %s", a.S4xx, a.Requests, window), a.S4xx})
			}
		}
	}
	var banned []string
	for _, c := range cands {
		ip := net.ParseIP(c.ip)
		if active[c.ip] || exempt(ip, nets) {
			continue
		}
		offenses := 1
		if old, err := s.Store.BanByIP(ctx, c.ip); err == nil {
			offenses = old.Offenses + 1
			// A ban that ended within the window (expired or lifted by an operator):
			// only detections after it ended count, so an unbanned IP is not re-banned
			// for what it did before.
			if old.ExpiresAt != nil && old.ExpiresAt.After(now.Add(-window)) {
				a := s.Traffic.EventsByIP(*old.ExpiresAt, Kinds...)[c.ip]
				if a == nil || a.Count < int64(set.Threshold) {
					continue
				}
			}
		}
		active[c.ip] = true
		b := &store.IPBan{IP: c.ip, Reason: c.reason, Source: "auto", Offenses: offenses, ExpiresAt: banUntil(set, offenses, now)}
		if err := s.Store.UpsertBan(ctx, b); err != nil {
			slog.Error("auto ban failed", "ip", c.ip, "err", err)
			continue
		}
		until := "permanent"
		if b.ExpiresAt != nil {
			until = b.ExpiresAt.Format(time.RFC3339)
		}
		_ = s.Store.Audit(ctx, nil, "ban.auto", c.ip, map[string]any{"reason": c.reason, "until": until, "offenses": offenses}, "")
		slog.Warn("IP banned automatically", "ip", c.ip, "until", until, "offenses", offenses, "reason", c.reason)
		banned = append(banned, c.ip)
	}
	return banned
}

// Push sends the full active ban list to the gateway: {"ips": {"<ip>": <expires unix or 0>}}.
func (s *Service) Push(ctx context.Context, now time.Time) error {
	if s.PushURL == "" {
		return nil
	}
	list, err := s.Store.ActiveBans(ctx, now)
	if err != nil {
		return err
	}
	ips := map[string]int64{}
	for _, b := range list {
		var exp int64
		if b.ExpiresAt != nil {
			exp = b.ExpiresAt.Unix()
		}
		ips[b.IP] = exp
	}
	body, _ := json.Marshal(map[string]any{"ips": ips, "now": now.Unix()})
	c := s.Client
	if c == nil {
		c = &http.Client{Timeout: 3 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.PushURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
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

// banUntil returns when the automatic ban for the given offense ends; nil = permanent.
// The first ban lasts DurationS and doubles with each repeat offense up to MaxDurationS.
func banUntil(set Settings, offenses int, now time.Time) *time.Time {
	if set.PermanentAfter > 0 && offenses >= set.PermanentAfter {
		return nil
	}
	limit := time.Duration(set.MaxDurationS) * time.Second
	if limit <= 0 || limit > maxDuration {
		limit = maxDuration
	}
	d := time.Duration(set.DurationS) * time.Second
	for i := 1; i < offenses && d < limit; i++ {
		d *= 2
	}
	if d > limit {
		d = limit
	}
	exp := now.Add(d).UTC().Truncate(time.Second)
	return &exp
}
