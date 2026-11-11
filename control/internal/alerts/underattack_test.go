package alerts

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/underattack"
)

// Automatic under-attack transitions are sent once each (on, off) to the channels.
func TestUnderAttackAlerts(t *testing.T) {
	e, _, h, _ := setup(t)
	ctx := context.Background()
	t0 := time.Now()
	var queue []underattack.Transition
	e.Src.UnderAttack = func() []underattack.Transition { q := queue; queue = nil; return q }

	queue = []underattack.Transition{{HostID: 3, Host: "shop.example.com", On: true, Reason: "300 requests/s over 30 s (threshold 200)", Since: t0, Until: t0.Add(15 * time.Minute), At: t0}}
	evalWait(e, ctx, t0)
	if h.n() != 1 {
		t.Fatalf("on: %d messages", h.n())
	}
	evalWait(e, ctx, t0.Add(30*time.Second))
	if h.n() != 1 {
		t.Fatalf("one-shot alert repeated: %d", h.n())
	}
	queue = []underattack.Transition{{HostID: 3, Host: "shop.example.com", Reason: "x", Since: t0, Until: t0.Add(15 * time.Minute), At: t0.Add(20 * time.Minute)}}
	evalWait(e, ctx, t0.Add(20*time.Minute))
	if h.n() != 2 {
		t.Fatalf("off: %d messages", h.n())
	}
	att := func(i int) map[string]any {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.msgs[i]["attachments"].([]any)[0].(map[string]any)
	}
	if on := att(0); !strings.Contains(on["title"].(string), "Under attack mode turned on · shop.example.com") || !strings.Contains(on["text"].(string), "300 requests/s") {
		t.Errorf("on message: %v", on)
	}
	if off := att(1); !strings.Contains(off["title"].(string), "Under attack mode turned off · shop.example.com") || !strings.Contains(off["text"].(string), "20 min") {
		t.Errorf("off message: %v", off)
	}

	// rule disabled: transitions are drained but not sent
	set := DefaultSettings()
	r := set.Rules["under_attack"]
	r.Enabled = false
	set.Rules["under_attack"] = r
	raw, _ := json.Marshal(set)
	if err := e.Store.SetMeta(ctx, "alert_settings", string(raw)); err != nil {
		t.Fatal(err)
	}
	queue = []underattack.Transition{{HostID: 3, Host: "shop.example.com", On: true, At: t0.Add(time.Hour), Since: t0.Add(time.Hour), Until: t0.Add(2 * time.Hour)}}
	evalWait(e, ctx, t0.Add(time.Hour))
	if h.n() != 2 || len(queue) != 0 {
		t.Fatalf("disabled rule: %d messages, queue %d", h.n(), len(queue))
	}
}
