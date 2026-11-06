// Package live reads real-time access queue state from Redis using the same
// atomic script as the gateway (op "stats"), so numbers always agree.
package live

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	"corvus/control/internal/store"
)

type Stats struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Active    int64  `json:"active"`
	Waiting   int64  `json:"waiting"`
	Capacity  int    `json:"capacity"`
}

type Live struct {
	rdb    *redis.Client
	script *redis.Script
}

func New(addr, user, password, scriptPath string) (*Live, error) {
	src, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("read access queue script: %w", err)
	}
	rdb := redis.NewClient(&redis.Options{
		Addr: addr, Username: user, Password: password, DialTimeout: 500 * time.Millisecond, ReadTimeout: 500 * time.Millisecond,
		WriteTimeout: 500 * time.Millisecond, MaxRetries: 1,
	})
	return &Live{rdb: rdb, script: redis.NewScript(string(src))}, nil
}

func (l *Live) Room(ctx context.Context, w store.Waitroom) Stats {
	st := Stats{Name: w.Name, Capacity: w.Capacity}
	if l == nil {
		return st
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	p := "corvus:wr:{" + w.Name + "}:"
	res, err := l.script.Run(ctx, l.rdb,
		[]string{p + "active", p + "queue", p + "seen", p + "seq", p + "rate", p + "granted"},
		"stats", "", w.Capacity, w.ActiveTTL*1000, w.GrantTTL*1000, w.QueueTTL*1000, w.AdmitRate, 200).Int64Slice()
	if err != nil || len(res) < 5 {
		return st
	}
	st.Available, st.Waiting, st.Active = true, res[3], res[4]
	return st
}

// Ping reports whether Redis is reachable.
func (l *Live) Ping(ctx context.Context) bool {
	if l == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return l.rdb.Ping(ctx).Err() == nil
}
