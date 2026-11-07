package compiler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

// Hosts with under-attack mode on or auto carry the challenge settings (defaults
// filled in) into rules.json; hosts with the mode off carry none.
func TestRenderUnderAttack(t *testing.T) {
	c, st := renderPagesEnv(t)
	ctx := context.Background()
	mk := func(d, mode string, cfg store.UnderAttackConfig) int64 {
		h := &store.ProxyHost{Domains: []string{d}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true,
			UnderAttack: mode, UnderAttackConfig: cfg}
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
		return h.ID
	}
	off := mk("off.example.com", "off", store.UnderAttackConfig{})
	on := mk("on.example.com", "on", store.UnderAttackConfig{Mode: "cookie", TTL: 600, Bind: "ip_ua"})
	auto := mk("auto.example.com", "auto", store.UnderAttackConfig{}) // legacy/empty config

	dir := filepath.Join(t.TempDir(), "v1")
	if err := c.Render(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
	var rules rulesFile
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "rules.json"))), &rules); err != nil {
		t.Fatal(err)
	}
	key := func(id int64) string { return strconv.FormatInt(id, 10) }
	if ua := rules.Hosts[key(off)].UnderAttack; ua != nil {
		t.Errorf("off host: %+v", ua)
	}
	if ua := rules.Hosts[key(on)].UnderAttack; ua == nil || *ua != (underAttackJSON{On: true, challengeJSON: challengeJSON{Mode: "cookie", Difficulty: 16, TTL: 600, Bind: "ip_ua"}}) {
		t.Errorf("on host: %+v", ua)
	}
	if ua := rules.Hosts[key(auto)].UnderAttack; ua == nil || *ua != (underAttackJSON{On: false, challengeJSON: challengeJSON{Mode: "js", Difficulty: 16, TTL: 3600, Bind: "ip"}}) {
		t.Errorf("auto host: %+v", ua)
	}
	conf := readFile(t, filepath.Join(dir, "nginx.conf"))
	for _, w := range []string{"lua_shared_dict corvus_ua 1m;", "location = /under-attack {", `require("corvus.rules").set_under_attack()`} {
		if !strings.Contains(conf, w) {
			t.Errorf("nginx.conf: missing %q", w)
		}
	}
}
