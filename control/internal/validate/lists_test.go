package validate

import (
	"strings"
	"testing"

	"corvus/control/internal/store"
)

func TestUserAgentListRules(t *testing.T) {
	kinds := map[int64]string{1: store.ListKindIP, 2: store.ListKindUserAgent}
	kind := func(id int64) string { return kinds[id] }
	for _, op := range []string{"in", "not_in"} {
		if err := SecurityRule(rule(store.Condition{Field: "http.user_agent_list", Operator: op, Value: []string{"list:2"}}), kind, nil); err != nil {
			t.Fatalf("valid UA list rule (%s) rejected: %v", op, err)
		}
	}
	for i, c := range []store.Condition{
		{Field: "http.user_agent_list", Operator: "in", Value: []string{"list:1"}},       // IP list
		{Field: "http.user_agent_list", Operator: "in", Value: []string{"list:9"}},       // missing
		{Field: "http.user_agent_list", Operator: "in", Value: []string{"curl"}},         // not a list reference
		{Field: "http.user_agent_list", Operator: "contains", Value: []string{"list:2"}}, // set operators only
		{Field: "ip.src", Operator: "in", Value: []string{"list:2"}},                     // UA list in an IP condition
	} {
		if SecurityRule(rule(c), kind, nil) == nil {
			t.Errorf("case %d accepted: %+v", i, c)
		}
	}
}

func TestListKindsAndFeeds(t *testing.T) {
	ua := &store.IPList{Name: "bots", Kind: store.ListKindUserAgent, Items: []string{" AhrefsBot ", "ahrefsbot", "sqlmap/1", ""}}
	if err := IPList(ua); err != nil || len(ua.Items) != 2 || ua.Items[0] != "AhrefsBot" {
		t.Fatalf("UA list: %v %q", err, ua.Items)
	}
	for _, items := range [][]string{{"ab"}, {"a\x01bc"}, {strings.Repeat("x", MaxUAEntryLen+1)}} {
		if IPList(&store.IPList{Name: "x", Kind: store.ListKindUserAgent, Items: items}) == nil {
			t.Errorf("UA entries %q accepted", items)
		}
	}
	if IPList(&store.IPList{Name: "x", Kind: "regex"}) == nil {
		t.Error("unknown kind accepted")
	}
	feed := &store.IPList{Name: "tor", SourceURL: "https://check.torproject.org/torbulkexitlist"}
	if err := IPList(feed); err != nil || feed.RefreshS != DefListRefreshS || feed.Kind != store.ListKindIP {
		t.Fatalf("feed: %v %+v", err, feed)
	}
	for _, l := range []store.IPList{
		{Name: "x", SourceURL: "http://example.com/list"},
		{Name: "x", SourceURL: "https://192.168.1.10/list"},
		{Name: "x", SourceURL: "https://example.com/list", RefreshS: 60},
		{Name: "x", SourceURL: "https://example.com/list", RefreshS: 8 * 86400},
	} {
		if IPList(&l) == nil {
			t.Errorf("feed accepted: %+v", l)
		}
	}
}
