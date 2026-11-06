package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/netcfg"
	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
	"corvus/control/internal/validate"
)

// ------------------------------------------------------------------ rate limits

type rateLimitInput struct {
	Name              string   `json:"name"`
	RequestsPerSecond int      `json:"requests_per_second"`
	Burst             int      `json:"burst"`
	MaxConnections    int      `json:"max_connections"`
	StatusCode        int      `json:"status_code"`
	DryRun            bool     `json:"dry_run"`
	ExemptCIDRs       []string `json:"exempt_cidrs"`
	Enabled           *bool    `json:"enabled"`
}

func (in rateLimitInput) build(r *store.RateLimit) error {
	r.Name, r.RequestsPerSecond, r.Burst, r.MaxConnections = in.Name, in.RequestsPerSecond, in.Burst, in.MaxConnections
	r.StatusCode, r.DryRun = in.StatusCode, in.DryRun
	r.Enabled = in.Enabled == nil || *in.Enabled
	cidrs, err := netcfg.NormalizeTrustedCIDRs(in.ExemptCIDRs)
	if err != nil {
		return validate.Errors{{Field: "exempt_cidrs", Code: "invalid", Message: err.Error()}}
	}
	r.ExemptCIDRs = cidrs
	return validate.RateLimit(r)
}

func (s *Server) listRateLimits(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListRateLimits(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createRateLimit(w http.ResponseWriter, r *http.Request) {
	var in rateLimitInput
	if !decode(w, r, &in) {
		return
	}
	rl := &store.RateLimit{}
	v, err := s.Deployer.Apply(r.Context(), userID(r), "create rate limit "+in.Name, func(tx *store.Store) error {
		if err := in.build(rl); err != nil {
			return err
		}
		if err := tx.SaveRateLimit(r.Context(), rl); err != nil {
			if store.IsUnique(err) {
				return validate.Errors{{Field: "name", Code: "duplicate", Message: "a policy with this name already exists"}}
			}
			return err
		}
		return nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "rate_limit.create", fmt.Sprintf("rate_limit:%d", rl.ID), rl)
	writeJSON(w, 201, map[string]any{"rate_limit": rl, "version": v})
}

func (s *Server) updateRateLimit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in rateLimitInput
	if !decode(w, r, &in) {
		return
	}
	var before, rl *store.RateLimit
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("update rate limit #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.RateLimitByID(r.Context(), id); err != nil {
			return err
		}
		cp := *before
		rl = &cp
		if err := in.build(rl); err != nil {
			return err
		}
		return tx.SaveRateLimit(r.Context(), rl)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "rate_limit.update", fmt.Sprintf("rate_limit:%d", id), map[string]any{"before": before, "after": rl})
	writeJSON(w, 200, map[string]any{"rate_limit": rl, "version": v})
}

func (s *Server) deleteRateLimit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var before *store.RateLimit
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("delete rate limit #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.RateLimitByID(r.Context(), id); err != nil {
			return err
		}
		if err := tx.DeleteRateLimit(r.Context(), id); err != nil {
			return conflictError{err.Error()}
		}
		return nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "rate_limit.delete", fmt.Sprintf("rate_limit:%d", id), before)
	writeJSON(w, 200, map[string]any{"version": v})
}

// ------------------------------------------------------------------ access lists

type accessListInput struct {
	Name    string   `json:"name"`
	Satisfy string   `json:"satisfy"`
	Allow   []string `json:"allow"`
	Deny    []string `json:"deny"`
	Users   []struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"users"`
}

func rules(field string, in []string) ([]string, error) {
	var cidrs []string
	out := []string{}
	for _, v := range in {
		if strings.EqualFold(strings.TrimSpace(v), "all") {
			out = append(out, "all")
		} else {
			cidrs = append(cidrs, v)
		}
	}
	n, err := netcfg.NormalizeCIDRs(cidrs)
	if err != nil {
		return nil, validate.Errors{{Field: field, Code: "invalid", Message: err.Error()}}
	}
	return append(n, out...), nil // "all" last so specific rules match first
}

// build fills a; existing users keep their hash when no new password is given.
func (in accessListInput) build(a *store.AccessList, existing []store.AccessListUser) error {
	// Basic-auth only: IP allow/deny are configured as security rules (§15).
	a.Name = in.Name
	old := map[string]string{}
	for _, u := range existing {
		old[u.Username] = u.PasswordHash
	}
	a.Users = nil
	for _, u := range in.Users {
		name := strings.TrimSpace(u.Username)
		hash := old[name]
		if u.Password != "" {
			if len(u.Password) < 8 {
				return validate.Errors{{Field: "users", Code: "weak", Message: "passwords must be at least 8 characters"}}
			}
			if len(u.Password) > 72 {
				return validate.Errors{{Field: "users", Code: "too_long", Message: "passwords must be at most 72 bytes"}}
			}
			h, err := auth.HtpasswdHash(u.Password)
			if err != nil {
				return err
			}
			hash = h
		}
		if hash == "" {
			return validate.Errors{{Field: "users", Code: "password_required", Message: fmt.Sprintf("a password is required for new user %q", name)}}
		}
		a.Users = append(a.Users, store.AccessListUser{Username: name, PasswordHash: hash})
	}
	return validate.AccessList(a)
}

func (s *Server) listAccessLists(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListAccessLists(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createAccessList(w http.ResponseWriter, r *http.Request) {
	var in accessListInput
	if !decode(w, r, &in) {
		return
	}
	a := &store.AccessList{}
	v, err := s.Deployer.Apply(r.Context(), userID(r), "create access list "+in.Name, func(tx *store.Store) error {
		if err := in.build(a, nil); err != nil {
			return err
		}
		if err := tx.SaveAccessList(r.Context(), a); err != nil {
			if store.IsUnique(err) {
				return validate.Errors{{Field: "name", Code: "duplicate", Message: "an access list with this name already exists"}}
			}
			return err
		}
		return nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "access_list.create", fmt.Sprintf("access_list:%d", a.ID), a)
	writeJSON(w, 201, map[string]any{"access_list": a, "version": v})
}

func (s *Server) updateAccessList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in accessListInput
	if !decode(w, r, &in) {
		return
	}
	var before, a *store.AccessList
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("update access list #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.AccessListByID(r.Context(), id); err != nil {
			return err
		}
		a = &store.AccessList{ID: id, CreatedAt: before.CreatedAt}
		if err := in.build(a, before.Users); err != nil {
			return err
		}
		return tx.SaveAccessList(r.Context(), a)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "access_list.update", fmt.Sprintf("access_list:%d", id), map[string]any{"before": before, "after": a})
	writeJSON(w, 200, map[string]any{"access_list": a, "version": v})
}

func (s *Server) deleteAccessList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var before *store.AccessList
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("delete access list #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.AccessListByID(r.Context(), id); err != nil {
			return err
		}
		if err := tx.DeleteAccessList(r.Context(), id); err != nil {
			return conflictError{err.Error()}
		}
		return nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "access_list.delete", fmt.Sprintf("access_list:%d", id), before)
	writeJSON(w, 200, map[string]any{"version": v})
}

// ------------------------------------------------------------------ network settings

type networkView struct {
	store.NetworkSettings
	Cloudflare struct {
		IPv4      int        `json:"ipv4"`
		IPv6      int        `json:"ipv6"`
		UpdatedAt *time.Time `json:"updated_at,omitempty"`
		Source    string     `json:"source"`
	} `json:"cloudflare"`
	RealIPHeaders []string `json:"real_ip_headers"`
}

func (s *Server) networkView(r *http.Request) (*networkView, error) {
	n, err := s.Store.Network(r.Context())
	if err != nil {
		return nil, err
	}
	v := &networkView{NetworkSettings: n, RealIPHeaders: validate.RealIPHeaders}
	cf := netcfg.Cloudflare(r.Context(), s.Store)
	v.Cloudflare.IPv4, v.Cloudflare.IPv6, v.Cloudflare.Source = len(cf.IPv4), len(cf.IPv6), cf.Source
	if !cf.UpdatedAt.IsZero() {
		v.Cloudflare.UpdatedAt = &cf.UpdatedAt
	}
	return v, nil
}

func (s *Server) getNetwork(w http.ResponseWriter, r *http.Request) {
	v, err := s.networkView(r)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) putNetwork(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TrustedProxies []string  `json:"trusted_proxies"`
		RealIPHeader   string    `json:"real_ip_header"`
		MaskIPs        *bool     `json:"mask_ips"`
		AdminAllow     *[]string `json:"admin_allow"`
	}
	if !decode(w, r, &in) {
		return
	}
	var allow []string
	if in.AdminAllow != nil {
		var err error
		if allow, err = checkAdminAllow(r, *in.AdminAllow); err != nil {
			fail(w, err)
			return
		}
	}
	var before store.NetworkSettings
	ver, err := s.Deployer.Apply(r.Context(), userID(r), "update network settings", func(tx *store.Store) error {
		var err error
		if before, err = tx.Network(r.Context()); err != nil {
			return err
		}
		n := before
		if n.TrustedProxies, err = netcfg.NormalizeTrustedCIDRs(in.TrustedProxies); err != nil {
			return validate.Errors{{Field: "trusted_proxies", Code: "invalid", Message: err.Error()}}
		}
		if in.RealIPHeader != "" {
			if !validate.RealIPHeader(in.RealIPHeader) {
				return validate.Errors{{Field: "real_ip_header", Code: "invalid", Message: "unsupported header"}}
			}
			n.RealIPHeader = in.RealIPHeader
		}
		if in.MaskIPs != nil {
			n.MaskIPs = *in.MaskIPs
		}
		if in.AdminAllow != nil {
			n.AdminAllow = allow
		}
		return tx.SetNetwork(r.Context(), n)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.invalidateAllow()
	s.audit(r, "network.update", "settings:network", map[string]any{"before": before, "after": in})
	v, _ := s.networkView(r)
	writeJSON(w, 200, map[string]any{"network": v, "version": ver})
}

func (s *Server) refreshCloudflare(w http.ResponseWriter, r *http.Request) {
	cf, err := netcfg.FetchCloudflare(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "fetch_failed", "could not download Cloudflare IP ranges: "+err.Error())
		return
	}
	ver, err := s.Deployer.Apply(r.Context(), userID(r), "refresh Cloudflare IP ranges", func(tx *store.Store) error {
		return tx.SetCloudflareIPs(r.Context(), cf)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "network.cloudflare_refresh", "settings:network", map[string]int{"ipv4": len(cf.IPv4), "ipv6": len(cf.IPv6)})
	v, _ := s.networkView(r)
	writeJSON(w, 200, map[string]any{"network": v, "version": ver})
}

// ------------------------------------------------------------------ traffic

func (s *Server) trafficSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	writeJSON(w, 200, s.Traffic.Summary(q.Get("range"), q.Get("host"), time.Now()))
}

func (s *Server) trafficTimeseries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	step, pts := s.Traffic.Timeseries(q.Get("range"), q.Get("host"), time.Now())
	writeJSON(w, 200, map[string]any{"step_s": step, "points": pts})
}

func (s *Server) trafficHosts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Traffic.Hosts(r.URL.Query().Get("range"), time.Now()))
}

func (s *Server) trafficTopPaths(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	writeJSON(w, 200, s.Traffic.TopPaths(q.Get("range"), q.Get("host"), limit, time.Now()))
}

func (s *Server) trafficEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	writeJSON(w, 200, s.Traffic.Events(q.Get("kind"), limit))
}

func (s *Server) trafficLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	items := s.Traffic.Logs(traffic.LogQuery{Host: q.Get("host"), Status: q.Get("status"), Q: q.Get("q"), Limit: limit})
	stamp := time.Now().UTC().Format("20060102-150405")
	switch q.Get("format") {
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="corvus-logs-`+stamp+`.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"time", "host", "method", "path", "status", "rt_ms", "bytes", "ip", "waitroom", "rule", "user_agent"})
		for _, l := range items {
			_ = cw.Write([]string{time.UnixMilli(int64(l.TS * 1000)).UTC().Format(time.RFC3339Nano), l.Host, l.Method, csvSafe(l.Path),
				strconv.Itoa(l.Status), strconv.FormatFloat(l.RTms, 'f', 1, 64), strconv.FormatInt(l.Bytes, 10), l.IP, l.WR, l.Rule, csvSafe(l.UA)})
		}
		cw.Flush()
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="corvus-logs-`+stamp+`.json"`)
		_ = json.NewEncoder(w).Encode(items)
	default:
		writeJSON(w, 200, map[string]any{"items": items})
	}
	// Exports always; viewing at most once per user every 10 minutes (the console polls
	// the logs every few seconds while the tab is open).
	if q.Get("format") != "" || s.firstLogView(r) {
		s.audit(r, "traffic.logs_viewed", q.Get("host"), map[string]string{"format": q.Get("format"), "status": q.Get("status"), "q": q.Get("q")})
	}
}

var logViews sync.Map // user id → time of the last audited log view

func (s *Server) firstLogView(r *http.Request) bool {
	u := userFrom(r)
	if u == nil {
		return true
	}
	now := time.Now()
	if last, ok := logViews.Load(u.ID); ok && now.Sub(last.(time.Time)) < 10*time.Minute {
		return false
	}
	logViews.Store(u.ID, now)
	return true
}

// csvSafe neutralizes spreadsheet formula injection in attacker-controlled fields.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}
