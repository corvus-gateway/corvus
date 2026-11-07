package traffic

import (
	"strconv"
	"strings"
	"time"
)

// Security rule activity: individual recent matches plus hourly hit counters per rule
// (in memory, like the rest of the collector — reset on restart).

const ruleEventCap = 1000

type RuleEvent struct {
	TS      float64 `json:"t"`
	RuleID  int64   `json:"rule_id"`
	Rule    string  `json:"rule"`
	Action  string  `json:"action"`
	Host    string  `json:"host"`
	IP      string  `json:"ip"`
	Country string  `json:"country"`
	Method  string  `json:"method"`
	Path    string  `json:"path"`
	UA      string  `json:"ua"`
	Status  int     `json:"status"`
	seq     int64
}

type hourCount struct {
	start int64
	n     int64
}

type ruleStat struct {
	hours [24]hourCount
	last  float64
}

// RuleHit is a rule's activity over the last 24 hours.
type RuleHit struct {
	Hits24h int64
	Last    *time.Time
}

// UnderAttackRule is the rule part of $corvus_rule for challenges of a host's "under
// attack" mode ("ua:challenge", "ua:challenge_pass", "ua:challenge_fail"). They count
// as challenge events but belong to no security rule.
const UnderAttackRule = "ua"

// ruleAction returns the action of the log field "<id>:<action>" or "ua:<action>".
func ruleAction(v string) (string, bool) {
	if a, ok := strings.CutPrefix(v, UnderAttackRule+":"); ok {
		return a, a != ""
	}
	_, a, ok := parseRule(v)
	return a, ok
}

// parseRule splits the log field "<id>:<action>".
func parseRule(v string) (int64, string, bool) {
	i := strings.IndexByte(v, ':')
	if i <= 0 {
		return 0, "", false
	}
	id, err := strconv.ParseInt(v[:i], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", false
	}
	return id, v[i+1:], true
}

// addRule records a rule match; caller holds c.mu.
func (c *Collector) addRule(r *Record) {
	id, action, ok := parseRule(r.R)
	if !ok {
		return
	}
	if c.ruleStats == nil {
		c.ruleStats = map[int64]*ruleStat{}
	}
	st := c.ruleStats[id]
	if st == nil {
		st = &ruleStat{}
		c.ruleStats[id] = st
	}
	ts := int64(r.T)
	h := &st.hours[(ts/3600)%24]
	if start := ts - ts%3600; h.start != start {
		*h = hourCount{start: start}
	}
	h.n++
	if r.T > st.last {
		st.last = r.T
	}
	c.ruleSeq++
	ev := RuleEvent{TS: r.T, RuleID: id, Action: action, Host: r.H, IP: r.IP, Country: r.CC, Method: r.M, Path: r.U, UA: r.UA, Status: r.S, seq: c.ruleSeq}
	if len(c.ruleEvents) < ruleEventCap {
		c.ruleEvents = append(c.ruleEvents, ev)
	} else {
		c.ruleEvents[c.rulePos] = ev
		c.rulePos = (c.rulePos + 1) % ruleEventCap
	}
}

// parseBody splits the log field "<signature>:<action>".
func parseBody(v string) (string, string, bool) {
	i := strings.LastIndexByte(v, ':')
	if i <= 0 {
		return "", "", false
	}
	return v[:i], v[i+1:], true
}

// addBody records a body-inspection match as a rule event (rule id 0, "body:<signature>").
func (c *Collector) addBody(r *Record) {
	sig, action, ok := parseBody(r.BD)
	if !ok || (action != "block" && action != "log") {
		return
	}
	c.ruleSeq++
	ev := RuleEvent{TS: r.T, Rule: "body:" + sig, Action: action, Host: r.H, IP: r.IP, Country: r.CC, Method: r.M, Path: r.U, UA: r.UA, Status: r.S, seq: c.ruleSeq}
	if len(c.ruleEvents) < ruleEventCap {
		c.ruleEvents = append(c.ruleEvents, ev)
	} else {
		c.ruleEvents[c.rulePos] = ev
		c.rulePos = (c.rulePos + 1) % ruleEventCap
	}
}

// RuleEvents returns recent matches, newest first (optionally for one rule).
func (c *Collector) RuleEvents(ruleID int64, limit int) []RuleEvent {
	if limit <= 0 || limit > ruleEventCap {
		limit = 100
	}
	mask := c.MaskIPs != nil && c.MaskIPs()
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []RuleEvent{}
	n := len(c.ruleEvents)
	for k := 0; k < n && len(out) < limit; k++ {
		// newest is just before rulePos once the ring is full
		i := (c.rulePos - 1 - k + 2*n) % n
		if n < ruleEventCap {
			i = n - 1 - k
		}
		e := c.ruleEvents[i]
		if ruleID != 0 && e.RuleID != ruleID {
			continue
		}
		if mask {
			e.IP = MaskIP(e.IP)
		}
		out = append(out, e)
	}
	return out
}

// RuleHits returns per-rule hits in the last 24 hours and the last match time.
func (c *Collector) RuleHits(now time.Time) map[int64]RuleHit {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[int64]RuleHit{}
	cut := now.Unix() - 24*3600
	for id, st := range c.ruleStats {
		var h RuleHit
		for _, b := range st.hours {
			if b.start > cut-3600 && b.start <= now.Unix() {
				h.Hits24h += b.n
			}
		}
		if st.last > 0 {
			t := time.UnixMilli(int64(st.last * 1000)).UTC()
			h.Last = &t
		}
		out[id] = h
	}
	return out
}
