package compiler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

// Challenge rules carry their settings into rules.json; other actions do not, and every
// proxy and redirection host gets the verify endpoint.
func TestRenderChallengeRules(t *testing.T) {
	c, st := renderPagesEnv(t)
	ctx := context.Background()
	h := &store.ProxyHost{Domains: []string{"a.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true}
	if err := st.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	rd := &store.RedirectionHost{Domains: []string{"old.example.com"}, ForwardScheme: "https", ForwardDomain: "a.example.com", HTTPCode: 301, Enabled: true}
	if err := st.SaveRedirectionHost(ctx, rd); err != nil {
		t.Fatal(err)
	}
	expr := store.Expression{Groups: [][]store.Condition{{{Field: "ip.country", Operator: "in", Value: []string{"XX"}}}}}
	js := &store.SecurityRule{Name: "challenge", Enabled: true, Action: "challenge", Scope: "all", Expression: expr,
		ChallengeMode: "js", ChallengeDifficulty: 18, ChallengeTTL: 3600, ChallengeBind: "ip_ua"}
	ck := &store.SecurityRule{Name: "cookie", Enabled: true, Action: "challenge", Scope: "all", Expression: expr, ChallengeMode: "cookie"}
	blk := &store.SecurityRule{Name: "block", Enabled: true, Action: "block", StatusCode: 403, Scope: "all", Expression: expr}
	for _, r := range []*store.SecurityRule{js, ck, blk} {
		if err := st.SaveSecurityRule(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.SecurityRuleByID(ctx, ck.ID)
	if err != nil || got.ChallengeMode != "cookie" || got.ChallengeTTL != 86400 || got.ChallengeBind != "ip" || got.ChallengeDifficulty != 16 {
		t.Fatalf("stored cookie challenge: %+v %v", got, err)
	}

	dir := filepath.Join(t.TempDir(), "v1")
	if err := c.Render(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
	raw := readFile(t, filepath.Join(dir, "rules.json"))
	var rules rulesFile
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		t.Fatal(err)
	}
	if rules.ChallengePage != "/opt/corvus/html/challenge.html" {
		t.Errorf("challenge_page = %q", rules.ChallengePage)
	}
	if len(rules.Rules) != 3 {
		t.Fatalf("rules: %+v", rules.Rules)
	}
	want := []*challengeJSON{
		{Mode: "js", Difficulty: 18, TTL: 3600, Bind: "ip_ua"},
		{Mode: "cookie", Difficulty: 16, TTL: 86400, Bind: "ip"},
		nil,
	}
	for i, r := range rules.Rules {
		switch {
		case want[i] == nil && r.Challenge != nil:
			t.Errorf("rule %d (%s): unexpected challenge settings %+v", i, r.Action, r.Challenge)
		case want[i] != nil && (r.Challenge == nil || *r.Challenge != *want[i]):
			t.Errorf("rule %d (%s): challenge = %+v, want %+v", i, r.Action, r.Challenge, want[i])
		case want[i] != nil && (r.Action != "challenge" || r.Status != 403):
			t.Errorf("rule %d: action %q status %d", i, r.Action, r.Status)
		}
	}
	if strings.Count(raw, `"challenge":`) != 2 {
		t.Errorf("challenge objects in rules.json:\n%s", raw)
	}

	host := readFile(t, filepath.Join(dir, "hosts", "1.conf"))
	redirect := readFile(t, filepath.Join(dir, "hosts", "redirect-1.conf"))
	for name, conf := range map[string]string{"1.conf": host, "redirect-1.conf": redirect} {
		for _, w := range []string{"location = /__corvus/challenge/verify {", "client_max_body_size 4k;", "limit_req zone=corvus_ch_api burst=20 nodelay;"} {
			if !strings.Contains(conf, w) {
				t.Errorf("%s: missing %q\n%s", name, w, conf)
			}
		}
	}
	if !strings.Contains(host, `require("corvus.rules").challenge_verify(1)`) || !strings.Contains(redirect, `require("corvus.rules").challenge_verify("r1")`) {
		t.Error("verify endpoint not bound to the host's rule key")
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, "nginx.conf")), "limit_req_zone $binary_remote_addr zone=corvus_ch_api:10m rate=5r/s;") {
		t.Error("nginx.conf: challenge rate-limit zone missing")
	}
}
