package live

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"corvus/control/internal/store"
)

// Needs a Redis/Valkey server: CORVUS_TEST_REDIS=host:port (skipped otherwise).
func TestForget(t *testing.T) {
	addr := os.Getenv("CORVUS_TEST_REDIS")
	if addr == "" {
		t.Skip("CORVUS_TEST_REDIS not set")
	}
	_, file, _, _ := runtime.Caller(0)
	script := filepath.Join(filepath.Dir(file), "..", "..", "..", "gateway", "redis", "waitroom.lua")
	l, err := New(addr, "", "", script)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	wr := store.Waitroom{Name: "forget-test", Capacity: 1, ActiveTTL: 60, GrantTTL: 30, QueueTTL: 60}
	p := "corvus:wr:{forget-test}:"
	for _, sid := range []string{"a", "b"} { // a gets in, b waits
		if err := l.script.Run(ctx, l.rdb, []string{p + "active", p + "queue", p + "seen", p + "seq", p + "rate", p + "granted"},
			"enter", sid, 1, 60000, 30000, 60000, 0, 200, 0).Err(); err != nil {
			t.Fatal(err)
		}
	}
	if st := l.Room(ctx, wr); st.Active != 1 || st.Waiting != 1 {
		t.Fatalf("before: %+v", st)
	}
	if err := l.Forget(ctx, wr.Name); err != nil {
		t.Fatal(err)
	}
	if n, _ := l.rdb.Exists(ctx, p+"active", p+"queue", p+"seen", p+"seq", p+"rate", p+"granted").Result(); n != 0 {
		t.Fatalf("%d keys left", n)
	}
	if err := (*Live)(nil).Forget(ctx, "x"); err != nil {
		t.Fatal(err)
	}
}
