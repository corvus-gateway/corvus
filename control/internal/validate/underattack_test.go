package validate

import (
	"testing"

	"corvus/control/internal/store"
)

func TestUnderAttackDefaults(t *testing.T) {
	h := validHost()
	if err := ProxyHost(&h); err != nil {
		t.Fatal(err)
	}
	c := h.UnderAttackConfig
	if h.UnderAttack != "off" || c.Mode != "js" || c.Difficulty != 16 || c.TTL != 3600 || c.Bind != "ip" ||
		c.RPS != 200 || c.BlockedPct == nil || *c.BlockedPct != 50 || c.WindowS != 30 || c.DurationS != 900 {
		t.Fatalf("defaults: %q %+v", h.UnderAttack, c)
	}
	// an explicit 0 % disables the ratio trigger and is kept
	h = validHost()
	zero := 0.0
	h.UnderAttack, h.UnderAttackConfig.BlockedPct = "auto", &zero
	if err := ProxyHost(&h); err != nil || *h.UnderAttackConfig.BlockedPct != 0 {
		t.Fatalf("blocked_pct 0: %v %v", *h.UnderAttackConfig.BlockedPct, err)
	}
}

func TestUnderAttackModes(t *testing.T) {
	for _, v := range []string{"off", "on", "auto"} {
		h := validHost()
		h.UnderAttack = v
		if err := ProxyHost(&h); err != nil || h.UnderAttack != v {
			t.Errorf("%s: %q %v", v, h.UnderAttack, err)
		}
	}
	pct := func(v float64) *float64 { return &v }
	bad := map[string]func(h *store.ProxyHost){
		"under_attack":                   func(h *store.ProxyHost) { h.UnderAttack = "ON" },
		"under_attack_config.mode":       func(h *store.ProxyHost) { h.UnderAttackConfig.Mode = "captcha" },
		"under_attack_config.difficulty": func(h *store.ProxyHost) { h.UnderAttackConfig.Difficulty = 30 },
		"under_attack_config.ttl_s":      func(h *store.ProxyHost) { h.UnderAttackConfig.TTL = 60 },
		"under_attack_config.bind":       func(h *store.ProxyHost) { h.UnderAttackConfig.Bind = "ua" },
		"under_attack_config.rps":        func(h *store.ProxyHost) { h.UnderAttackConfig.RPS = -5 },
		"under_attack_config.blocked_pct": func(h *store.ProxyHost) {
			h.UnderAttackConfig.BlockedPct = pct(150)
		},
		"under_attack_config.window_s":   func(h *store.ProxyHost) { h.UnderAttackConfig.WindowS = 5 },
		"under_attack_config.duration_s": func(h *store.ProxyHost) { h.UnderAttackConfig.DurationS = 86401 },
	}
	for field, mut := range bad {
		h := validHost()
		mut(&h)
		err := ProxyHost(&h)
		if fe, ok := err.(Errors); !ok || len(fe) != 1 || fe[0].Field != field {
			t.Errorf("%s: expected one error for the field, got %v", field, err)
		}
	}
}
