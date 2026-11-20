package compiler

import (
	"context"
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/ipset"
	"corvus/control/internal/store"
)

func renderRules(t *testing.T, c *Compiler, st *store.Store) rulesFile {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "v")
	if err := c.Render(context.Background(), st, dir); err != nil {
		t.Fatal(err)
	}
	var rules rulesFile
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "rules.json"))), &rules); err != nil {
		t.Fatal(err)
	}
	return rules
}

// IP lists reach the gateway as range files; networks typed into the rule stay inline.
func TestRenderIPListFiles(t *testing.T) {
	c, st := renderPagesEnv(t)
	ctx := context.Background()
	h := &store.ProxyHost{Domains: []string{"a.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true}
	if err := st.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	manual := &store.IPList{Name: "office", Kind: store.ListKindIP, Items: []string{"192.0.2.0/24", "2001:db8::/32"}}
	feed := &store.IPList{Name: "blocklist", Kind: store.ListKindIP, Items: []string{}, SourceURL: "https://example.com/list.txt", RefreshS: 3600}
	for _, l := range []*store.IPList{manual, feed} {
		if err := st.SaveIPList(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	// the feed's entries are stored as a file, outside the database
	set, err := ipset.FromEntries([]string{"198.51.100.0/24", "203.0.113.9"})
	if err != nil {
		t.Fatal(err)
	}
	info, err := c.Lists.Write(set)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	if err := st.SetIPListFetch(ctx, feed.ID, store.ListFetch{Data: &store.ListData{Hash: info.Hash, Items: 2, Ranges: 2}, CheckedAt: at, FetchedAt: &at}); err != nil {
		t.Fatal(err)
	}
	ref := func(id int64) string { return "list:" + itoa(id) }
	r := &store.SecurityRule{Name: "lists", Enabled: true, Action: "block", StatusCode: 403, Scope: "all",
		Expression: store.Expression{Groups: [][]store.Condition{
			{{Field: "ip.src", Operator: "in", Value: []string{ref(manual.ID), "10.0.0.1", ref(feed.ID), ref(manual.ID)}}},
			{{Field: "ip.src", Operator: "not_in", Value: []string{ref(feed.ID)}}},
		}}}
	if err := st.SaveSecurityRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	rules := renderRules(t, c, st)
	c1, c2 := rules.Rules[0].Groups[0][0], rules.Rules[0].Groups[1][0]
	if strings.Join(c1.V, ",") != "0a000001/32" || len(c1.L) != 2 || len(c2.L) != 1 || len(c2.V) != 0 {
		t.Fatalf("conditions %+v %+v", c1, c2)
	}
	if c1.L[1] != c2.L[0] || c1.L[1].SHA256 != info.Hash {
		t.Fatalf("feed file %+v, want %s", c1.L[1], info.Hash)
	}
	// the manual list's file was written and holds its entries
	got, err := c.Lists.Load(c1.L[0].SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := got.Contains(netip.MustParseAddr("2001:db8::7")); !ok || got.Ranges() != 2 {
		t.Fatalf("manual list file: %+v", got)
	}
	if filepath.ToSlash(c.Lists.File(c1.L[0].SHA256)) != c1.L[0].Path {
		t.Fatalf("path %s", c1.L[0].Path)
	}

	// The feed's file is gone (restored backup): the list renders as empty, the
	// deployment still works.
	if err := os.Remove(c.Lists.File(info.Hash)); err != nil {
		t.Fatal(err)
	}
	rules = renderRules(t, c, st)
	empty := rules.Rules[0].Groups[1][0].L[0]
	if empty.SHA256 == info.Hash {
		t.Fatal("missing file still referenced")
	}
	if e, err := c.Lists.Load(empty.SHA256); err != nil || e.Ranges() != 0 {
		t.Fatalf("empty list file: %v", err)
	}

	// A reference to a list that does not exist fails the rendering.
	r.Expression.Groups = [][]store.Condition{{{Field: "ip.src", Operator: "in", Value: []string{"list:999"}}}}
	if err := st.SaveSecurityRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := c.Render(ctx, st, filepath.Join(t.TempDir(), "x")); err == nil {
		t.Fatal("missing list rendered")
	}
}

// Without an explicit store the files go to <DataDir>/lists.
func TestListDirDefault(t *testing.T) {
	dir := t.TempDir()
	c := &Compiler{P: Paths{DataDir: filepath.ToSlash(dir)}}
	if got := c.listDir().Path; got != filepath.Join(dir, "lists") {
		t.Fatalf("list dir %s", got)
	}
}
