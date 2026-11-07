package traffic

import "time"

// Persistence hooks used by internal/history: completed 5-minute buckets, finished
// security events and rule events are exported incrementally and restored at start,
// so dashboards survive restarts. Everything else stays in memory.

// CoarseRow is one completed 5-minute bucket of a host ("" = all hosts).
type CoarseRow struct {
	Host                                          string
	Start                                         int64
	Count, S2, S3, S4, S5, Blocked, Queued, Bytes int64
	Hist                                          [nLat]uint32
}

// CoarseStep is the bucket width of CoarseRow (seconds).
const CoarseStep = coarseStep

func rowOf(host string, b *bucket) CoarseRow {
	return CoarseRow{Host: host, Start: b.start, Count: b.count, S2: b.s2, S3: b.s3, S4: b.s4, S5: b.s5,
		Blocked: b.blocked, Queued: b.queued, Bytes: b.bytes, Hist: b.hist}
}

// CompletedCoarse returns buckets that started after `after` and are complete at now.
func (c *Collector) CompletedCoarse(after int64, now time.Time) []CoarseRow {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []CoarseRow
	cut := now.Unix()
	add := func(host string, s *series) {
		for i := range s.coarse {
			b := &s.coarse[i]
			if b.count > 0 && b.start > after && b.start+coarseStep <= cut {
				out = append(out, rowOf(host, b))
			}
		}
	}
	add("", &c.global)
	for h, s := range c.hosts {
		add(h, s)
	}
	return out
}

// RestoreCoarse loads persisted buckets (only those still inside the 24h ring).
func (c *Collector) RestoreCoarse(rows []CoarseRow, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	oldest := now.Unix() - coarseStep*coarseSlots
	for _, r := range rows {
		if r.Start <= oldest || r.Start > now.Unix() {
			continue
		}
		s := &c.global
		if r.Host != "" {
			s = c.hosts[r.Host]
			if s == nil {
				if len(c.hosts) >= maxHosts {
					continue
				}
				s = &series{}
				c.hosts[r.Host] = s
			}
		}
		if r.Start > s.last {
			s.last = r.Start
		}
		b := &s.coarse[(r.Start/coarseStep)%coarseSlots]
		if b.start != r.Start {
			*b = bucket{start: r.Start}
		}
		b.merge(&bucket{count: r.Count, s2: r.S2, s3: r.S3, s4: r.S4, s5: r.S5, blocked: r.Blocked, queued: r.Queued, bytes: r.Bytes, hist: r.Hist})
	}
}

// EventsAfter returns events (minute merging finished) with a sequence number above seq,
// plus the highest sequence number returned.
func (c *Collector) EventsAfter(seq int64, now time.Time) ([]Event, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	curMin := now.Unix() / 60
	var out []Event
	max := seq
	for _, e := range c.events {
		if e.seq <= seq || int64(e.TS)/60 >= curMin {
			continue
		}
		out = append(out, e)
		if e.seq > max {
			max = e.seq
		}
	}
	return out, max
}

// RestoreEvents appends persisted events (oldest first); returns the sequence watermark.
func (c *Collector) RestoreEvents(evs []Event) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range evs {
		if len(c.events) >= eventCap {
			c.events = append(c.events[:0:0], c.events[eventCap/5:]...)
			c.evIndex = map[string]int{}
		}
		c.evSeq++
		e.seq = c.evSeq
		c.events = append(c.events, e)
		c.countSource(int64(e.TS), e.Kind, e.Host, e.IP, e.Count)
	}
	return c.evSeq
}

// RuleEventsAfter returns rule/body events with a sequence number above seq (oldest first).
func (c *Collector) RuleEventsAfter(seq int64) ([]RuleEvent, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []RuleEvent
	max := seq
	for _, e := range c.ruleEvents {
		if e.seq > seq {
			out = append(out, e)
			if e.seq > max {
				max = e.seq
			}
		}
	}
	// ring order is not chronological once wrapped; callers only need the set
	return out, max
}

// RestoreRuleEvents loads persisted rule events (oldest first) and rebuilds the
// 24-hour hit counters; returns the sequence watermark.
func (c *Collector) RestoreRuleEvents(evs []RuleEvent) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ruleStats == nil {
		c.ruleStats = map[int64]*ruleStat{}
	}
	for _, e := range evs {
		c.ruleSeq++
		e.seq = c.ruleSeq
		if len(c.ruleEvents) < ruleEventCap {
			c.ruleEvents = append(c.ruleEvents, e)
		} else {
			c.ruleEvents[c.rulePos] = e
			c.rulePos = (c.rulePos + 1) % ruleEventCap
		}
		if e.RuleID <= 0 {
			continue
		}
		st := c.ruleStats[e.RuleID]
		if st == nil {
			st = &ruleStat{}
			c.ruleStats[e.RuleID] = st
		}
		ts := int64(e.TS)
		h := &st.hours[(ts/3600)%24]
		if start := ts - ts%3600; h.start != start {
			*h = hourCount{start: start}
		}
		h.n++
		if e.TS > st.last {
			st.last = e.TS
		}
	}
	return c.ruleSeq
}
