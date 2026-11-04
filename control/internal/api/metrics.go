package api

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"corvus/control/internal/traffic"
)

// GET /metrics — Prometheus text exposition format (0.0.4), hand-written to avoid a
// dependency. Requires "Authorization: Bearer cvs_…" (any role; viewer is enough).
// Label cardinality is bounded: hosts are capped by the traffic collector
// (100 + "(other)"), rooms/certificates/rules by configuration.

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	tok, ok := bearerToken(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="corvus"`)
		writeErr(w, http.StatusUnauthorized, "unauthorized", "an API token is required (Authorization: Bearer cvs_…)")
		return
	}
	if u, _, msg := s.bearerPrincipal(r.Context(), tok); u == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="corvus"`)
		writeErr(w, http.StatusUnauthorized, "unauthorized", msg)
		return
	}
	body := s.renderMetrics(r.Context(), time.Now())
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

type promWriter struct{ b bytes.Buffer }

func (p *promWriter) family(name, typ, help string) {
	fmt.Fprintf(&p.b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}

// sample writes one line; labels are name/value pairs.
func (p *promWriter) sample(name string, v float64, labels ...string) {
	p.b.WriteString(name)
	if len(labels) > 0 {
		p.b.WriteByte('{')
		for i := 0; i+1 < len(labels); i += 2 {
			if i > 0 {
				p.b.WriteByte(',')
			}
			p.b.WriteString(labels[i])
			p.b.WriteString(`="`)
			p.b.WriteString(escapeLabel(labels[i+1]))
			p.b.WriteByte('"')
		}
		p.b.WriteByte('}')
	}
	p.b.WriteByte(' ')
	p.b.WriteString(formatValue(v))
	p.b.WriteByte('\n')
}

var labelEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

func escapeLabel(v string) string { return labelEscaper.Replace(v) }

func formatValue(v float64) string {
	switch {
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case math.IsNaN(v):
		return "NaN"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func (s *Server) renderMetrics(ctx context.Context, now time.Time) []byte {
	p := &promWriter{}

	p.family("corvus_build_info", "gauge", "Corvus build information (always 1).")
	p.sample("corvus_build_info", 1, "version", s.Version)

	var cfg float64
	if v, err := s.Store.ActiveVersion(ctx); err == nil && v != nil {
		cfg = float64(v.ID)
	}
	p.family("corvus_config_version", "gauge", "ID of the active (deployed) configuration version.")
	p.sample("corvus_config_version", cfg)

	gw := false
	if s.Gateway != nil {
		gw, _ = s.Gateway()
	}
	p.family("corvus_gateway_up", "gauge", "Whether the gateway (OpenResty) is running.")
	p.sample("corvus_gateway_up", b2f(gw))
	p.family("corvus_redis_up", "gauge", "Whether the cache DB (access queue state) is reachable.")
	p.sample("corvus_redis_up", b2f(s.Live.Ping(ctx)))

	if s.Traffic != nil {
		counters := s.Traffic.Counters()
		p.family("corvus_requests_total", "counter", "Requests seen by the gateway, by host and status class.")
		for _, hc := range counters {
			for _, cl := range []string{"2xx", "3xx", "4xx", "5xx"} {
				p.sample("corvus_requests_total", float64(hc.Class[cl]), "host", hc.Host, "class", cl)
			}
		}
		p.family("corvus_blocked_total", "counter", "Requests blocked by the gateway, by host and reason (exploit, rule, body, rate_limit).")
		for _, hc := range counters {
			for _, k := range traffic.BlockKinds {
				p.sample("corvus_blocked_total", float64(hc.Blocked[k]), "host", hc.Host, "kind", k)
			}
		}
		p.family("corvus_request_duration_seconds", "gauge", "Request latency quantiles per host over the last 5 minutes (histogram bucket upper bounds).")
		for _, l := range s.Traffic.Latency(5*time.Minute, now) {
			p.sample("corvus_request_duration_seconds", l.P50, "host", l.Host, "quantile", "0.5")
			p.sample("corvus_request_duration_seconds", l.P95, "host", l.Host, "quantile", "0.95")
			p.sample("corvus_request_duration_seconds", l.P99, "host", l.Host, "quantile", "0.99")
		}
		hits := s.Traffic.RuleHits(now)
		ids := make([]int64, 0, len(hits))
		for id := range hits {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		p.family("corvus_security_rule_hits_24h", "gauge", "Security rule matches in the last 24 hours.")
		for _, id := range ids {
			p.sample("corvus_security_rule_hits_24h", float64(hits[id].Hits24h), "rule_id", strconv.FormatInt(id, 10))
		}
	}

	if rooms, err := s.Store.ListWaitrooms(ctx); err == nil {
		type rs struct {
			name            string
			waiting, active int64
		}
		var live []rs
		for _, wr := range rooms {
			if st := s.Live.Room(ctx, wr); st.Available {
				live = append(live, rs{wr.Name, st.Waiting, st.Active})
			}
		}
		p.family("corvus_access_queue_waiting", "gauge", "Visitors waiting in the access queue.")
		for _, x := range live {
			p.sample("corvus_access_queue_waiting", float64(x.waiting), "room", x.name)
		}
		p.family("corvus_access_queue_active", "gauge", "Visitors currently admitted (active) in the access queue.")
		for _, x := range live {
			p.sample("corvus_access_queue_active", float64(x.active), "room", x.name)
		}
	}

	if certs, err := s.Store.ListCertificates(ctx); err == nil {
		p.family("corvus_certificate_expiry_seconds", "gauge", "Certificate expiry as a Unix timestamp (seconds); alert on value - time().")
		for _, c := range certs {
			if c.NotAfter.IsZero() {
				continue
			}
			p.sample("corvus_certificate_expiry_seconds", float64(c.NotAfter.Unix()), "certificate", c.Name, "id", strconv.FormatInt(c.ID, 10))
		}
	}
	return p.b.Bytes()
}
