package traffic

import (
	"bytes"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"
)

// A snapshot written at shutdown brings back the live views after a restart.
func TestSnapshotRoundTrip(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	c := New()
	for i := 0; i < 120; i++ {
		ts := now.Unix() - 1800 + int64(i*15)
		status := 200
		if i%10 == 0 {
			status = 404
		}
		c.Add(&Record{T: float64(ts), H: "a.example.com", M: "GET", U: "/p" + string(rune('a'+i%3)), S: status, RT: 0.02, B: 100, IP: "10.0.0." + string(rune('1'+i%5))})
	}
	c.Add(&Record{T: float64(now.Unix() - 30), H: "b.example.com", M: "POST", U: "/login", S: 429, IP: "10.0.1.1"})

	var buf bytes.Buffer
	if err := c.WriteSnapshot(&buf, now); err != nil {
		t.Fatal(err)
	}
	later := now.Add(20 * time.Second)
	r := New()
	if err := r.ReadSnapshot(bytes.NewReader(buf.Bytes()), later); err != nil {
		t.Fatal(err)
	}

	for _, rng := range []string{"15m", "1h"} {
		for _, host := range []string{"", "a.example.com", "b.example.com"} {
			want, got := c.Summary(rng, host, later), r.Summary(rng, host, later)
			if !reflect.DeepEqual(want, got) {
				t.Errorf("summary %s %q:\nwant %+v\ngot  %+v", rng, host, want, got)
			}
		}
		_, wantTS := c.Timeseries(rng, "", later)
		_, gotTS := r.Timeseries(rng, "", later)
		if !reflect.DeepEqual(wantTS, gotTS) {
			t.Errorf("timeseries %s differs", rng)
		}
		if want, got := byPath(c.TopPaths(rng, "", 10, later)), byPath(r.TopPaths(rng, "", 10, later)); !reflect.DeepEqual(want, got) {
			t.Errorf("top paths %s:\nwant %+v\ngot  %+v", rng, want, got)
		}
		if want, got := c.Hosts(rng, later), r.Hosts(rng, later); !reflect.DeepEqual(want, got) {
			t.Errorf("hosts %s:\nwant %+v\ngot  %+v", rng, want, got)
		}
	}
	if want, got := c.Logs(LogQuery{Limit: 1000}), r.Logs(LogQuery{Limit: 1000}); !reflect.DeepEqual(want, got) || len(got) != 121 {
		t.Errorf("logs: want %d lines, got %d", len(want), len(got))
	}

	// new traffic continues on top of the restored state
	r.Add(&Record{T: float64(later.Unix()), H: "a.example.com", U: "/new", S: 200, IP: "10.0.0.9"})
	if got := r.Logs(LogQuery{Limit: 1}); len(got) != 1 || got[0].Path != "/new" {
		t.Errorf("newest log line: %+v", got)
	}
}

// A full log ring keeps its order across a snapshot, and new lines replace the oldest.
func TestSnapshotFullLogRing(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	c := New()
	for i := 0; i < logCap+50; i++ {
		c.Add(&Record{T: float64(now.Unix() - 60), H: "a", U: "/", S: 200, IP: "10.0.0.1", B: int64(i)})
	}
	var buf bytes.Buffer
	if err := c.WriteSnapshot(&buf, now); err != nil {
		t.Fatal(err)
	}
	r := New()
	if err := r.ReadSnapshot(&buf, now); err != nil {
		t.Fatal(err)
	}
	r.Add(&Record{T: float64(now.Unix()), H: "a", U: "/", S: 200, IP: "10.0.0.1", B: -1})
	got := r.Logs(LogQuery{Limit: 1000})
	if got[0].Bytes != -1 || got[1].Bytes != int64(logCap+49) || got[999].Bytes != int64(logCap+49-998) {
		t.Fatalf("order after restore: %d %d %d", got[0].Bytes, got[1].Bytes, got[999].Bytes)
	}
}

// A snapshot from a stop of more than an hour is ignored.
func TestSnapshotStale(t *testing.T) {
	now := time.Unix(3_000_000, 0)
	c := New()
	c.Add(&Record{T: float64(now.Unix() - 10), H: "a", U: "/", S: 200, IP: "10.0.0.1"})
	var buf bytes.Buffer
	if err := c.WriteSnapshot(&buf, now); err != nil {
		t.Fatal(err)
	}
	r := New()
	if err := r.ReadSnapshot(&buf, now.Add(2*time.Hour)); !errors.Is(err, ErrSnapshotStale) {
		t.Fatalf("err = %v", err)
	}
	if len(r.Logs(LogQuery{})) != 0 {
		t.Fatal("stale snapshot was loaded")
	}
}

// byPath orders paths with equal counts the same way (ties come from map order).
func byPath(ps []PathStat) []PathStat {
	sort.Slice(ps, func(i, j int) bool { return ps[i].Host+ps[i].Path < ps[j].Host+ps[j].Path })
	return ps
}
