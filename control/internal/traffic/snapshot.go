package traffic

import (
	"compress/gzip"
	"encoding/gob"
	"errors"
	"io"
	"time"
)

// Live snapshot: the state that only lives in memory (10-second buckets, per-minute
// path and visitor stats, the recent request log) is written on a clean shutdown and
// read back on start, so a restart or upgrade does not empty the live views. The
// 5-minute buckets and the events are persisted separately (see persist.go) and are not
// part of the snapshot.

const snapshotVersion = 1

// snapshotMaxAge is how old a snapshot may be to be loaded: everything in it is at most
// an hour old when it is written.
const snapshotMaxAge = time.Hour

type snapBucket struct {
	Start, Count, S2, S3, S4, S5, Blocked, Queued, Bytes, Banned int64
	Hist                                                         [nLat]uint32
}

type snapSeries struct {
	Host string // "" = all hosts
	Last int64
	Fine []snapBucket // non-empty buckets only
}

type snapPath struct {
	Key              string
	Requests, Errors int64
	Lat              snapBucket
}

type snapIP struct {
	IP       string
	N, S4    int64
	LastHost string
}

type snapMinute struct {
	Start int64
	Paths []snapPath
	IPs   []string
	HIPs  map[string][]string
	IPSt  []snapIP
}

type snapshot struct {
	Version int
	SavedAt int64
	Series  []snapSeries
	Minutes []snapMinute
	Logs    []LogLine // oldest first
}

func toSnap(b *bucket) snapBucket {
	return snapBucket{Start: b.start, Count: b.count, S2: b.s2, S3: b.s3, S4: b.s4, S5: b.s5,
		Blocked: b.blocked, Queued: b.queued, Bytes: b.bytes, Banned: b.banned, Hist: b.hist}
}

func fromSnap(s snapBucket) bucket {
	return bucket{start: s.Start, count: s.Count, s2: s.S2, s3: s.S3, s4: s.S4, s5: s.S5,
		blocked: s.Blocked, queued: s.Queued, bytes: s.Bytes, banned: s.Banned, hist: s.Hist}
}

// WriteSnapshot writes the in-memory live state (gzip-compressed) to w.
func (c *Collector) WriteSnapshot(w io.Writer, now time.Time) error {
	c.mu.Lock()
	snap := snapshot{Version: snapshotVersion, SavedAt: now.Unix()}
	addSeries := func(host string, s *series) {
		ss := snapSeries{Host: host, Last: s.last}
		for i := range s.fine {
			if s.fine[i].count > 0 {
				ss.Fine = append(ss.Fine, toSnap(&s.fine[i]))
			}
		}
		if len(ss.Fine) > 0 {
			snap.Series = append(snap.Series, ss)
		}
	}
	addSeries("", &c.global)
	for h, s := range c.hosts {
		addSeries(h, s)
	}
	for i := range c.minutes {
		m := &c.minutes[i]
		if m.paths == nil {
			continue
		}
		sm := snapMinute{Start: m.start, HIPs: map[string][]string{}}
		for k, p := range m.paths {
			sm.Paths = append(sm.Paths, snapPath{Key: k, Requests: p.requests, Errors: p.errors, Lat: toSnap(&p.lat)})
		}
		for ip := range m.ips {
			sm.IPs = append(sm.IPs, ip)
		}
		for h, set := range m.hips {
			for ip := range set {
				sm.HIPs[h] = append(sm.HIPs[h], ip)
			}
		}
		for ip, st := range m.ipst {
			sm.IPSt = append(sm.IPSt, snapIP{IP: ip, N: st.n, S4: st.s4, LastHost: st.host})
		}
		snap.Minutes = append(snap.Minutes, sm)
	}
	snap.Logs = make([]LogLine, 0, len(c.logs))
	snap.Logs = append(snap.Logs, c.logs[c.logPos:]...)
	snap.Logs = append(snap.Logs, c.logs[:c.logPos]...)
	c.mu.Unlock()

	zw := gzip.NewWriter(w)
	if err := gob.NewEncoder(zw).Encode(&snap); err != nil {
		return err
	}
	return zw.Close()
}

// ErrSnapshotStale is returned by ReadSnapshot for a snapshot older than an hour.
var ErrSnapshotStale = errors.New("traffic snapshot is too old")

// ReadSnapshot loads a snapshot written by WriteSnapshot into a collector that has not
// received traffic yet. Buckets and minutes older than an hour are skipped.
func (c *Collector) ReadSnapshot(r io.Reader, now time.Time) error {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	var snap snapshot
	if err := gob.NewDecoder(zr).Decode(&snap); err != nil {
		return err
	}
	if snap.Version != snapshotVersion {
		return errors.New("unknown traffic snapshot version")
	}
	if age := now.Unix() - snap.SavedAt; age < 0 || age > int64(snapshotMaxAge/time.Second) {
		return ErrSnapshotStale
	}
	oldest := now.Unix() - fineStep*fineSlots

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ss := range snap.Series {
		s := &c.global
		if ss.Host != "" {
			s = c.hosts[ss.Host]
			if s == nil {
				if len(c.hosts) >= maxHosts {
					continue
				}
				s = &series{}
				c.hosts[ss.Host] = s
			}
		}
		if ss.Last > s.last {
			s.last = ss.Last
		}
		for _, sb := range ss.Fine {
			if sb.Start <= oldest || sb.Start > now.Unix() {
				continue
			}
			s.fine[(sb.Start/fineStep)%fineSlots] = fromSnap(sb)
		}
	}
	for _, sm := range snap.Minutes {
		if sm.Start <= now.Unix()-60*pathMinutes || sm.Start > now.Unix() {
			continue
		}
		m := minute{start: sm.Start, paths: map[string]*pathStat{}, ips: map[string]struct{}{},
			hips: map[string]map[string]struct{}{}, ipst: map[string]*ipStat{}}
		for _, p := range sm.Paths {
			m.paths[p.Key] = &pathStat{requests: p.Requests, errors: p.Errors, lat: fromSnap(p.Lat)}
		}
		for _, ip := range sm.IPs {
			m.ips[ip] = struct{}{}
		}
		for h, ips := range sm.HIPs {
			set := make(map[string]struct{}, len(ips))
			for _, ip := range ips {
				set[ip] = struct{}{}
			}
			m.hips[h] = set
		}
		for _, st := range sm.IPSt {
			m.ipst[st.IP] = &ipStat{n: st.N, s4: st.S4, host: st.LastHost}
		}
		c.minutes[(sm.Start/60)%pathMinutes] = m
	}
	logs := snap.Logs
	if len(logs) > logCap {
		logs = logs[len(logs)-logCap:]
	}
	c.logs = append(c.logs[:0], logs...)
	c.logPos = 0 // oldest first: when the ring is full, the next line overwrites index 0
	c.uniq = nil
	return nil
}
