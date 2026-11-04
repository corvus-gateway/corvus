package autoban

// Bad-behaviour bans: the gateway counts error responses per client IP and proxy host
// (Lua log phase, corvus/rules.lua) and bans an IP in its shared memory as soon as the
// threshold is reached. It keeps a pending record of each such ban; corvus-control
// polls them from the loopback status server every round, records them in the store
// with the normal escalation and acknowledges them with the next ban-list push. Until
// acknowledged, a gateway-local ban survives authoritative pushes; unacknowledged
// records expire with the local ban.

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"corvus/control/internal/store"
)

// Hit is a gateway-local bad-behaviour ban waiting to be recorded.
type Hit struct {
	ID      string           `json:"id"`
	IP      string           `json:"ip"`
	Host    string           `json:"host"`
	Counts  map[string]int64 `json:"counts"` // status code → responses in the window
	WindowS int              `json:"window"`
	T       int64            `json:"t"` // unix time of the ban
}

// Reason renders the ban reason, e.g. "bad behaviour: 12×404 on example.com in 60s".
func (h Hit) Reason() string {
	type kv struct {
		code string
		n    int64
	}
	var parts []kv
	for c, n := range h.Counts {
		if n > 0 {
			parts = append(parts, kv{c, n})
		}
	}
	sort.Slice(parts, func(i, j int) bool {
		if parts[i].n != parts[j].n {
			return parts[i].n > parts[j].n
		}
		return parts[i].code < parts[j].code
	})
	strs := make([]string, len(parts))
	for i, p := range parts {
		strs[i] = fmt.Sprintf("%d×%s", p.n, p.code)
	}
	counts := strings.Join(strs, ", ")
	if counts == "" {
		counts = "error responses"
	}
	host := h.Host
	if host == "" {
		host = "?"
	}
	return fmt.Sprintf("bad behaviour: %s on %s in %ds", counts, host, h.WindowS)
}

// Pull fetches the gateway's pending bad-behaviour bans.
func (s *Service) Pull(ctx context.Context) ([]Hit, error) {
	if s.HitsURL == "" {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.HitsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gateway answered %d", resp.StatusCode)
	}
	var out struct {
		Hits []Hit `json:"hits"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 8<<20)).Decode(&out); err != nil {
		return nil, err
	}
	return out.Hits, nil
}

// RecordHits stores gateway-reported bad-behaviour bans (source "auto", normal
// escalation) and queues their acknowledgement for the next push. Hits that are not
// recorded (disabled, exempt, already banned, invalid) are acknowledged too, so the
// gateway drops its local ban with the next authoritative list. Hits that could not be
// stored are not acknowledged and come back next round. Returns the IPs banned.
func (s *Service) RecordHits(ctx context.Context, now time.Time, hits []Hit) []string {
	set := Load(ctx, s.Store)
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
	} else {
		return nil // store unavailable: retry next round
	}
	ack := func(h Hit) {
		s.mu.Lock()
		if s.acks == nil {
			s.acks = map[string]string{}
		}
		s.acks[h.IP] = h.ID
		s.mu.Unlock()
	}
	var banned []string
	for _, h := range hits {
		ip := net.ParseIP(h.IP)
		if ip == nil || h.ID == "" {
			if h.IP != "" {
				ack(h)
			}
			continue
		}
		h.IP = ip.String()
		if !set.Enabled || !set.BadBehaviour || active[h.IP] || exempt(ip, nets) {
			ack(h)
			continue
		}
		offenses := 1
		if old, err := s.Store.BanByIP(ctx, h.IP); err == nil {
			offenses = old.Offenses + 1
		}
		reason := h.Reason()
		b := &store.IPBan{IP: h.IP, Reason: reason, Source: "auto", Offenses: offenses, ExpiresAt: banUntil(set, offenses, now)}
		if err := s.Store.UpsertBan(ctx, b); err != nil {
			slog.Error("bad behaviour ban failed", "ip", h.IP, "err", err)
			continue
		}
		active[h.IP] = true
		ack(h)
		until := "permanent"
		if b.ExpiresAt != nil {
			until = b.ExpiresAt.Format(time.RFC3339)
		}
		_ = s.Store.Audit(ctx, nil, "ban.auto", h.IP, map[string]any{"reason": reason, "until": until, "offenses": offenses, "detector": "bad_behaviour", "host": h.Host}, "")
		slog.Warn("IP banned automatically", "ip", h.IP, "until", until, "offenses", offenses, "reason", reason)
		banned = append(banned, h.IP)
	}
	return banned
}

// gatewayConfig is the bad-behaviour configuration pushed with the ban list.
func gatewayConfig(set Settings) map[string]any {
	on := set.Enabled && set.BadBehaviour
	cfg := map[string]any{"enabled": on}
	if !on {
		return cfg
	}
	codes := set.BadBehaviourCodes
	if len(codes) == 0 {
		codes = DefaultBadBehaviourCodes
	}
	cfg["codes"] = codes
	cfg["threshold"] = set.BadBehaviourThreshold
	cfg["window"] = set.BadBehaviourWindowS
	cfg["duration"] = set.DurationS
	// loopback is never banned (same as Evaluate)
	cfg["exempt"] = encodeNets(append([]string{"127.0.0.0/8", "::1/128"}, set.Exempt...))
	return cfg
}

// encodeNets renders CIDRs as "<hex network bytes>/<prefix>" for the gateway's IP matcher
// (same encoding as the security rules, see compiler/rules.go).
func encodeNets(cidrs []string) []string {
	out := []string{}
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			continue
		}
		ones, _ := n.Mask.Size()
		ip := n.IP
		if v4 := ip.To4(); v4 != nil {
			ip = v4
			if ones > 32 {
				ones -= 96
			}
		}
		out = append(out, hex.EncodeToString(ip)+"/"+strconv.Itoa(ones))
	}
	return out
}
