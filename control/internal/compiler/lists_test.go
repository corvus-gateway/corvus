package compiler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

func TestRenderUserAgentListRule(t *testing.T) {
	c, st := renderPagesEnv(t)
	ctx := context.Background()
	h := &store.ProxyHost{Domains: []string{"a.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true}
	if err := st.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	bots := &store.IPList{Name: "bots", Kind: store.ListKindUserAgent, Items: []string{"AhrefsBot", "Indy Library"}}
	more := &store.IPList{Name: "more", Kind: store.ListKindUserAgent, Items: []string{"ahrefsbot", "sqlmap"}}
	tor := &store.IPList{Name: "tor", Kind: store.ListKindIP, Items: []string{"1.2.3.4/32"}, SourceURL: "https://check.torproject.org/torbulkexitlist", RefreshS: 3600}
	for _, l := range []*store.IPList{bots, more, tor} {
		if err := st.SaveIPList(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	r := &store.SecurityRule{Name: "bad bots", Enabled: true, Action: "block", StatusCode: 403, Scope: "all",
		Expression: store.Expression{Groups: [][]store.Condition{
			{{Field: "http.user_agent_list", Operator: "in", Value: []string{"list:" + itoa(bots.ID), "list:" + itoa(more.ID)}}},
			{{Field: "ip.src", Operator: "in", Value: []string{"list:" + itoa(tor.ID)}}},
		}}}
	if err := st.SaveSecurityRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "v1")
	if err := c.Render(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
	var rules rulesFile
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "rules.json"))), &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules.Rules) != 1 || len(rules.Rules[0].Groups) != 2 {
		t.Fatalf("rules: %+v", rules.Rules)
	}
	ua := rules.Rules[0].Groups[0][0]
	if ua.F != "http.user_agent_list" || ua.Op != "in" || strings.Join(ua.V, "|") != "ahrefsbot|indy library|sqlmap" {
		t.Fatalf("UA condition = %+v", ua)
	}
	if ip := rules.Rules[0].Groups[1][0]; strings.Join(ip.V, ",") != "01020304/32" {
		t.Fatalf("IP condition = %+v", ip)
	}

	// A UA list referenced from an IP condition never renders.
	r.Expression.Groups = [][]store.Condition{{{Field: "ip.src", Operator: "in", Value: []string{"list:" + itoa(bots.ID)}}}}
	if err := st.SaveSecurityRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := c.Render(ctx, st, filepath.Join(t.TempDir(), "v2")); err == nil {
		t.Fatal("UA list in an IP condition rendered")
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
