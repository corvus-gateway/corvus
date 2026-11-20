package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/ipset"
	"corvus/control/internal/store"
)

// IP list range files stay while a list in the database or a retained version refers to
// them, or while they are recent; other files and stale temporary files are removed.
func TestPruneUnusedListFiles(t *testing.T) {
	d, _, dir := setup(t)
	d.Keep = 2
	ctx := context.Background()
	lists := &ipset.Dir{Path: filepath.Join(dir, "lists")}
	old := time.Now().Add(-time.Hour)
	write := func(entries ...string) string {
		t.Helper()
		s, err := ipset.FromEntries(entries)
		if err != nil {
			t.Fatal(err)
		}
		info, err := lists.Write(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(lists.File(info.Hash), old, old); err != nil {
			t.Fatal(err)
		}
		return info.Hash
	}
	exists := func(h string) bool { _, err := os.Stat(lists.File(h)); return err == nil }

	feedHash := write("198.51.100.0/24") // the stored entries of a feed (database reference)
	orphan := write("203.0.113.0/24")    // referenced by nothing
	recent := write("203.0.113.77")      // referenced by nothing, but just written
	_ = os.Chtimes(lists.File(recent), time.Now(), time.Now())
	staleTmp := filepath.Join(lists.Path, ".tmp-123")
	if err := os.WriteFile(staleTmp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(staleTmp, old, old)
	other := filepath.Join(lists.Path, "README")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(other, old, old)

	feed := &store.IPList{Name: "feed", Kind: store.ListKindIP, Items: []string{}, SourceURL: "https://example.com/l.txt", RefreshS: 3600}
	manual := &store.IPList{Name: "manual", Kind: store.ListKindIP, Items: []string{"192.0.2.1"}}
	host := &store.ProxyHost{Domains: []string{"shop.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, Enabled: true}
	rule := &store.SecurityRule{Name: "block", Enabled: true, Action: "block", StatusCode: 403, Scope: "all"}
	if _, err := d.Apply(ctx, nil, "lists", func(tx *store.Store) error {
		for _, l := range []*store.IPList{feed, manual} {
			if err := tx.SaveIPList(ctx, l); err != nil {
				return err
			}
		}
		at := time.Now()
		if err := tx.SetIPListFetch(ctx, feed.ID, store.ListFetch{Data: &store.ListData{Hash: feedHash, Items: 1, Ranges: 1}, CheckedAt: at, FetchedAt: &at}); err != nil {
			return err
		}
		rule.Expression = store.Expression{Groups: [][]store.Condition{{{Field: "ip.src", Operator: "in", Value: []string{"list:" + strconv.FormatInt(manual.ID, 10)}}}}}
		if err := tx.SaveProxyHost(ctx, host); err != nil {
			return err
		}
		return tx.SaveSecurityRule(ctx, rule)
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readCurrent(t, d, "rules.json"), "/lists/") {
		t.Fatal("rules.json does not refer to a list file")
	}
	manualHash := ""
	entries, _ := os.ReadDir(lists.Path)
	for _, e := range entries {
		h := strings.TrimSuffix(e.Name(), ".bin")
		if ipset.FileName.MatchString(e.Name()) && h != feedHash && h != orphan && h != recent {
			manualHash = h
		}
	}
	if manualHash == "" {
		t.Fatal("the manual list's file was not written")
	}
	if exists(orphan) || !exists(feedHash) || !exists(recent) || !exists(manualHash) {
		t.Fatalf("after the first deployment: orphan %v feed %v recent %v manual %v", exists(orphan), exists(feedHash), exists(recent), exists(manualHash))
	}
	if _, err := os.Stat(staleTmp); err == nil {
		t.Fatal("stale temporary file kept")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("unrelated file removed")
	}

	// The list changes: its old file stays while a retained version refers to it.
	_ = os.Chtimes(lists.File(manualHash), old, old)
	if _, err := d.Apply(ctx, nil, "edit list", func(tx *store.Store) error {
		manual.Items = []string{"192.0.2.2"}
		return tx.SaveIPList(ctx, manual)
	}); err != nil {
		t.Fatal(err)
	}
	if !exists(manualHash) {
		t.Fatal("file removed while a retained version refers to it")
	}
	for i := 0; i < 3; i++ {
		if _, err := d.Apply(ctx, nil, "more", nil); err != nil {
			t.Fatal(err)
		}
	}
	if exists(manualHash) || !exists(feedHash) {
		t.Fatalf("after pruning: old manual file %v, feed file %v", exists(manualHash), exists(feedHash))
	}
}
