package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"corvus/control/internal/conncheck"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// Connection checks per user and minute: enough to try a few hosts and ports while
// editing, too few to scan a network through the console.
const connCheckPerMinute = 20

// upstreamErrorWindow: how long the console shows a host's last upstream failure.
const upstreamErrorWindow = 15 * time.Minute

type connCheckInput struct {
	ForwardScheme string `json:"forward_scheme"`
	ForwardHost   string `json:"forward_host"`
	ForwardPort   int    `json:"forward_port"`
}

// POST /api/proxy-hosts/check — tries a forward target the way the gateway reaches it
// (same forward-host mapping, the gateway's DNS resolver without search domains, TCP,
// TLS for https). The answer is 200 also when the target does not answer: ok=false with
// a failure kind the console explains.
func (s *Server) checkConnection(w http.ResponseWriter, r *http.Request) {
	var in connCheckInput
	if !decode(w, r, &in) {
		return
	}
	l := store.Location{Path: "/", ForwardScheme: in.ForwardScheme, ForwardHost: in.ForwardHost, ForwardPort: in.ForwardPort}
	if err := validate.Location(&l); err != nil {
		fail(w, err)
		return
	}
	key := "conncheck:" + clientIP(r)
	if u := userFrom(r); u != nil {
		key = "conncheck:user:" + strconv.FormatInt(u.ID, 10)
	}
	if blocked, _ := s.connLimit.Blocked(key); blocked {
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "too many connection checks, try again in a minute")
		return
	}
	s.connLimit.Fail(key)

	addr := l.ForwardHost
	if s.Deployer != nil && s.Deployer.Compiler != nil {
		addr = s.Deployer.Compiler.ForwardAddr(l.ForwardHost)
	}
	// Corvus's own loopback services are refused when saving, so they aren't probed either.
	if why := s.Paths.InternalTarget(addr, l.ForwardPort); why != "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation", "message": why,
			"fields": validate.Errors{{Field: "forward_host", Code: "internal", Message: why}}})
		return
	}
	c := s.ConnCheck
	if c == nil {
		c = &conncheck.Checker{Resolver: s.Paths.Resolver, IPv6: s.Paths.ResolverIPv6}
	}
	writeJSON(w, http.StatusOK, c.Check(r.Context(), l.ForwardScheme, addr, l.ForwardPort))
}

// upstreamError is a proxy host's latest failure to reach its upstream (gateway log).
type upstreamError struct {
	HostID   int64     `json:"host_id"`
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	Upstream string    `json:"upstream,omitempty"`
	Message  string    `json:"message"`
}

// GET /api/upstream-errors — proxy hosts whose upstream failed recently: the latest
// failure within upstreamErrorWindow that is newer than the host's last change (a saved
// fix clears the warning; otherwise it ages out).
func (s *Server) listUpstreamErrors(w http.ResponseWriter, r *http.Request) {
	out := []upstreamError{}
	if s.UpstreamFailures == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	fails := s.UpstreamFailures()
	if len(fails) == 0 {
		writeJSON(w, http.StatusOK, out)
		return
	}
	hosts, err := s.Store.ListProxyHosts(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	byServer := map[string]*store.ProxyHost{}
	for i := range hosts {
		if h := &hosts[i]; h.Enabled && len(h.Domains) > 0 {
			byServer[strings.ToLower(h.Domains[0])] = h
		}
	}
	since := time.Now().Add(-upstreamErrorWindow)
	for _, f := range fails {
		h := byServer[f.Server]
		// updated_at has second precision: a failure within the second of a save counts
		// as before it
		if h == nil || f.At.Before(since) || !f.At.After(h.UpdatedAt.Add(time.Second)) {
			continue
		}
		out = append(out, upstreamError{HostID: h.ID, At: f.At, Kind: f.Kind, Upstream: f.Upstream, Message: f.Message})
	}
	writeJSON(w, http.StatusOK, out)
}
