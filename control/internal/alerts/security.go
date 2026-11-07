package alerts

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"corvus/control/internal/traffic"
)

// securityKinds are traffic events that count as security detections — including
// "log only" ones, so repeated anomalies are reported even when nothing is blocked.
var securityKinds = []string{"blocked_exploit", "exploit_detected", "blocked_rule", "blocked_body", "body_detected"}

const maxSourceFindings = 5 // per rule and evaluation, to avoid alert floods

// securityFindings: per-host security event spikes, repeat attack sources and scanners.
func securityFindings(c *traffic.Collector, on func(string) (Rule, bool), win func(Rule) time.Duration,
	mins func(time.Duration) string, tr map[string]string, now time.Time) []finding {
	var out []finding
	if r, ok := on("security_events"); ok {
		since := now.Add(-win(r))
		blocked := c.EventCounts(since, "blocked_rule", "blocked_body")
		for h, n := range c.EventCounts(since, "blocked_rule", "rule_logged", "blocked_body", "body_detected") {
			if float64(n) >= r.Threshold {
				out = append(out, finding{rule: "security_events", key: h, title: fmt.Sprintf(tr["security_events"], h),
					text:   fmt.Sprintf(tr["security_eventsText"], mins(win(r)), n, blocked[h]),
					fields: [][2]string{{tr["host"], h}, {tr["count"], fmt.Sprint(n)}, {tr["blocked"], fmt.Sprint(blocked[h])}}})
			}
		}
	}
	if r, ok := on("attack_source"); ok {
		type src struct {
			ip string
			a  *traffic.SourceActivity
		}
		var list []src
		for ip, a := range c.EventsByIP(now.Add(-win(r)), securityKinds...) {
			if float64(a.Count) >= r.Threshold {
				list = append(list, src{ip, a})
			}
		}
		sort.Slice(list, func(i, j int) bool { return list[i].a.Count > list[j].a.Count })
		for i, s := range list {
			if i >= maxSourceFindings {
				break
			}
			hosts, kinds := keys(s.a.Hosts), keys(s.a.Kinds)
			out = append(out, finding{rule: "attack_source", key: s.ip, title: fmt.Sprintf(tr["attack_source"], s.ip),
				text:   fmt.Sprintf(tr["attack_sourceText"], mins(win(r)), s.a.Count, strings.Join(hosts, ", ")),
				fields: [][2]string{{"IP", s.ip}, {tr["count"], fmt.Sprint(s.a.Count)}, {tr["kinds"], strings.Join(kinds, ", ")}}})
		}
	}
	if r, ok := on("scan_suspect"); ok {
		type act struct {
			ip string
			a  traffic.IPActivity
		}
		var list []act
		for ip, a := range c.IPCounts(win(r), now) {
			// many client errors that make up most of the IP's traffic: path/content scanning
			if float64(a.S4xx) >= r.Threshold && a.S4xx*2 >= a.Requests {
				list = append(list, act{ip, a})
			}
		}
		sort.Slice(list, func(i, j int) bool { return list[i].a.S4xx > list[j].a.S4xx })
		for i, s := range list {
			if i >= maxSourceFindings {
				break
			}
			out = append(out, finding{rule: "scan_suspect", key: s.ip, title: fmt.Sprintf(tr["scan_suspect"], s.ip),
				text:   fmt.Sprintf(tr["scan_suspectText"], mins(win(r)), s.a.S4xx, s.a.Requests, s.a.Host),
				fields: [][2]string{{"IP", s.ip}, {tr["host"], s.a.Host}, {"4xx", fmt.Sprintf("%d / %d", s.a.S4xx, s.a.Requests)}}})
		}
	}
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
