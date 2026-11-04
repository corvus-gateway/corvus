package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"corvus/control/internal/autoban"
	"corvus/control/internal/netcfg"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// ------------------------------------------------------------------ IP bans

func (s *Server) listBans(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ActiveBans(r.Context(), time.Now())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createBan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IP        string `json:"ip"`
		DurationS int    `json:"duration_s"` // 0 = until removed
		Reason    string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	ip := net.ParseIP(strings.TrimSpace(in.IP))
	if ip == nil {
		fail(w, validate.Errors{{Field: "ip", Code: "invalid", Message: "a single IPv4 or IPv6 address is required (use security rules for networks)"}})
		return
	}
	if ip.IsLoopback() || ip.IsUnspecified() {
		fail(w, validate.Errors{{Field: "ip", Code: "invalid", Message: "this address cannot be banned"}})
		return
	}
	if in.DurationS < 0 || in.DurationS > 365*86400 {
		fail(w, validate.Errors{{Field: "duration_s", Code: "invalid", Message: "duration must be 0 (until removed) up to 365 days"}})
		return
	}
	reason := strings.TrimSpace(in.Reason)
	if !validate.DisplayName(reason, 200) {
		reason = "manual"
	}
	b := &store.IPBan{IP: ip.String(), Reason: reason, Source: "manual", Offenses: 1, CreatedBy: userID(r)}
	if in.DurationS > 0 {
		exp := time.Now().Add(time.Duration(in.DurationS) * time.Second).UTC().Truncate(time.Second)
		b.ExpiresAt = &exp
	}
	if old, err := s.Store.BanByIP(r.Context(), b.IP); err == nil {
		b.Offenses = old.Offenses + 1
	}
	if err := s.Store.UpsertBan(r.Context(), b); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "ban.create", b.IP, b)
	if s.AutoBan != nil {
		s.AutoBan.Trigger()
	}
	writeJSON(w, 201, b)
}

func (s *Server) deleteBan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	b, err := s.Store.LiftBan(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "ban.delete", b.IP, b)
	if s.AutoBan != nil {
		s.AutoBan.Trigger()
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// permanentBan removes the end time of a ban (e.g. turn an automatic ban into a permanent one).
func (s *Server) permanentBan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	b, err := s.Store.MakeBanPermanent(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "ban.permanent", b.IP, b)
	if s.AutoBan != nil {
		s.AutoBan.Trigger()
	}
	writeJSON(w, 200, b)
}

func (s *Server) getAutoBan(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, autoban.Load(r.Context(), s.Store))
}

func (s *Server) putAutoBan(w http.ResponseWriter, r *http.Request) {
	var in autoban.Settings
	if !decode(w, r, &in) {
		return
	}
	if err := autoban.Validate(&in); err != nil {
		fail(w, validate.Errors{{Field: "autoban", Code: "invalid", Message: err.Error()}})
		return
	}
	before := autoban.Load(r.Context(), s.Store)
	raw, _ := json.Marshal(in)
	if err := s.Store.SetMeta(r.Context(), "autoban", string(raw)); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "autoban.update", "settings:autoban", map[string]any{"before": before, "after": in})
	if s.AutoBan != nil {
		s.AutoBan.Trigger()
	}
	writeJSON(w, 200, in)
}

// ------------------------------------------------------------------ admin console allowlist

// adminAllow restricts the whole admin listener (UI, API, metrics) to the configured
// networks. CORVUS_ADMIN_ALLOW_ANY=1 disables the check (lock-out recovery).
type allowCache struct {
	mu   sync.Mutex
	nets []*net.IPNet
	at   time.Time
}

func (s *Server) adminAllow(next http.Handler) http.Handler {
	bypass := os.Getenv("CORVUS_ADMIN_ALLOW_ANY") == "1"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bypass {
			next.ServeHTTP(w, r)
			return
		}
		nets := s.adminNets(r)
		if len(nets) > 0 {
			ip := net.ParseIP(clientIP(r))
			allowed := ip != nil && ip.IsLoopback()
			for _, n := range nets {
				if ip != nil && n.Contains(ip) {
					allowed = true
					break
				}
			}
			if !allowed {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("403 Forbidden: this address may not access the Corvus admin console\n"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) adminNets(r *http.Request) []*net.IPNet {
	s.allow.mu.Lock()
	defer s.allow.mu.Unlock()
	if time.Since(s.allow.at) < 5*time.Second {
		return s.allow.nets
	}
	n, err := s.Store.Network(r.Context())
	if err != nil {
		return s.allow.nets // keep the last known list on errors
	}
	var nets []*net.IPNet
	for _, c := range n.AdminAllow {
		if _, nn, err := net.ParseCIDR(c); err == nil {
			nets = append(nets, nn)
		}
	}
	s.allow.nets, s.allow.at = nets, time.Now()
	return nets
}

func (s *Server) invalidateAllow() {
	s.allow.mu.Lock()
	s.allow.at = time.Time{}
	s.allow.mu.Unlock()
}

// checkAdminAllow validates a new allowlist; the caller's own address must stay allowed.
func checkAdminAllow(r *http.Request, in []string) ([]string, error) {
	list, err := netcfg.NormalizeCIDRs(in)
	if err != nil {
		return nil, validate.Errors{{Field: "admin_allow", Code: "invalid", Message: err.Error()}}
	}
	if len(list) == 0 {
		return list, nil
	}
	ip := net.ParseIP(clientIP(r))
	for _, c := range list {
		if _, n, err := net.ParseCIDR(c); err == nil && ip != nil && n.Contains(ip) {
			return list, nil
		}
	}
	return nil, validate.Errors{{Field: "admin_allow", Code: "lockout",
		Message: fmt.Sprintf("your current address %s is not in the list — saving would lock you out", clientIP(r))}}
}
