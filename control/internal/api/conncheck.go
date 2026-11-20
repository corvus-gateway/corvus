package api

import (
	"fmt"
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

// connCheckConcurrent bounds the checks running at once, for all users together.
const connCheckConcurrent = 4

// upstreamErrorWindow: how long the console shows a host's last upstream failure.
const upstreamErrorWindow = 15 * time.Minute

type connCheckInput struct {
	ForwardScheme string `json:"forward_scheme"`
	ForwardHost   string `json:"forward_host"`
	ForwardPort   int    `json:"forward_port"`
	// A proxy host's further backends (see store.ProxyHost.ExtraTargets): each one is
	// checked too, with the same scheme, and the answer lists them all under "targets".
	ExtraTargets []store.Target `json:"extra_targets"`
}

// connCheckResult answers a check with several backends: the fields of the first failing
// backend (or of the first one when all work; ok only when every backend works), and
// the result of each backend in order.
type connCheckResult struct {
	conncheck.Result
	Targets []conncheck.Result `json:"targets"`
}

// POST /api/proxy-hosts/check — tries a forward target the way the gateway reaches it
// (same forward-host mapping, the gateway's DNS resolver without search domains, TCP,
// TLS for https). The answer is 200 also when the target does not answer: ok=false with
// a failure kind the console explains. With extra_targets, every backend is checked
// (one check against the rate limit).
func (s *Server) checkConnection(w http.ResponseWriter, r *http.Request) {
	var in connCheckInput
	if !decode(w, r, &in) {
		return
	}
	if len(in.ExtraTargets)+1 > validate.MaxTargets {
		fail(w, validate.Errors{{Field: "extra_targets", Code: "too_many", Message: fmt.Sprintf("at most %d backends per host", validate.MaxTargets)}})
		return
	}
	l := store.Location{Path: "/", ForwardScheme: in.ForwardScheme, ForwardHost: in.ForwardHost, ForwardPort: in.ForwardPort}
	if err := validate.Location(&l); err != nil {
		fail(w, err)
		return
	}
	targets := []store.Target{{ForwardHost: l.ForwardHost, ForwardPort: l.ForwardPort}}
	fields := []string{"forward_host"}
	for i, t := range in.ExtraTargets {
		el := store.Location{Path: "/", ForwardScheme: l.ForwardScheme, ForwardHost: t.ForwardHost, ForwardPort: t.ForwardPort}
		if err := validate.Location(&el); err != nil {
			var errs validate.Errors
			for _, f := range err.(validate.Errors) {
				f.Field = fmt.Sprintf("extra_targets[%d].%s", i, f.Field)
				errs = append(errs, f)
			}
			fail(w, errs)
			return
		}
		targets = append(targets, store.Target{ForwardHost: el.ForwardHost, ForwardPort: el.ForwardPort})
		fields = append(fields, fmt.Sprintf("extra_targets[%d].forward_host", i))
	}
	key := "conncheck:" + clientIP(r)
	if u := userFrom(r); u != nil {
		key = "conncheck:user:" + strconv.FormatInt(u.ID, 10)
	}
	if !s.connLimit.Allow(key) {
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "too many connection checks, try again in a minute")
		return
	}

	// Corvus's own loopback services are refused when saving, so they aren't probed either.
	internal := func(field, why string) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation", "message": why,
			"fields": validate.Errors{{Field: field, Code: "internal", Message: why}}})
	}
	check := make([]conncheck.Target, len(targets))
	for i, t := range targets {
		addr := t.ForwardHost
		if s.Deployer != nil && s.Deployer.Compiler != nil {
			addr = s.Deployer.Compiler.ForwardAddr(t.ForwardHost)
		}
		if why := s.Paths.InternalTarget(addr, t.ForwardPort); why != "" {
			internal(fields[i], why)
			return
		}
		check[i] = conncheck.Target{Host: addr, Port: t.ForwardPort}
	}
	select {
	case s.connSem <- struct{}{}:
		defer func() { <-s.connSem }()
	default:
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "too many connection checks running, try again in a moment")
		return
	}
	c := conncheck.Checker{Resolver: s.Paths.Resolver, IPv6: s.Paths.ResolverIPv6}
	if s.ConnCheck != nil {
		c = *s.ConnCheck
	}
	// ... nor through a name that resolves to loopback
	c.Refuse = func(ip string, port int) string { return s.Paths.InternalTarget(ip, port) }
	res := c.CheckAll(r.Context(), l.ForwardScheme, check)
	for i, x := range res {
		if x.Refused != "" {
			internal(fields[i], x.Refused)
			return
		}
	}
	if len(res) == 1 {
		writeJSON(w, http.StatusOK, res[0]) // a single target: the answer as before
		return
	}
	out := connCheckResult{Result: res[0], Targets: res}
	for _, x := range res {
		if !x.OK {
			out.Result = x
			break
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// upstreamError is a proxy host's latest failure to reach its upstream (gateway log).
type upstreamError struct {
	HostID   int64     `json:"host_id"`
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	Upstream string    `json:"upstream,omitempty"`
	Message  string    `json:"message"`
	// Backend pools: the host's number of backends (omitted for a single one), and
	// whether none of them was available (nginx "no live upstreams": every backend is
	// failing). Otherwise the failure is one backend's, which the others cover.
	Backends int  `json:"backends,omitempty"`
	AllDown  bool `json:"all_down,omitempty"`
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
		e := upstreamError{HostID: h.ID, At: f.At, Kind: f.Kind, Upstream: f.Upstream, Message: f.Message}
		if n := len(h.ExtraTargets) + 1; n > 1 {
			e.Backends = n
			e.AllDown = strings.Contains(strings.ToLower(f.Message), "no live upstreams")
			if e.AllDown {
				e.Kind = conncheck.KindOther // every backend failed: not a DNS problem
			}
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, out)
}
