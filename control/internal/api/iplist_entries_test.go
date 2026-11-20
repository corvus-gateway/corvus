package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"corvus/control/internal/compiler"
	"corvus/control/internal/deploy"
	"corvus/control/internal/ipset"
	"corvus/control/internal/store"
)

// File-backed lists are shown as a bounded preview and can be searched for an address.
func TestIPListEntriesAndCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("deployment uses POSIX symlink swaps; run on Linux")
	}
	e := newTestEnv(t)
	dir := t.TempDir()
	html := filepath.Join(dir, "html")
	_ = os.MkdirAll(html, 0o755)
	p := compiler.Paths{DataDir: dir, LuaDir: "/opt/corvus/lua", HTMLDir: filepath.ToSlash(html), MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181}
	e.s.Paths = p
	lists := &ipset.Dir{Path: p.ListDir()}
	e.s.Deployer = &deploy.Deployer{Store: e.s.Store, Compiler: &compiler.Compiler{P: p, Lists: lists}, Runner: okRunner{}, Dir: filepath.Join(dir, "nginx")}
	viewer := e.session("viewer@example.com", "viewer")
	ctx := context.Background()

	manual := &store.IPList{Name: "office", Kind: store.ListKindIP, Items: []string{"192.0.2.0/24", "198.51.100.7/32", "2001:db8::/32"}}
	feed := &store.IPList{Name: "blocklist", Kind: store.ListKindIP, Items: []string{}, SourceURL: "https://example.com/l.txt", RefreshS: 3600}
	ua := &store.IPList{Name: "bots", Kind: store.ListKindUserAgent, Items: []string{"sqlmap"}}
	for _, l := range []*store.IPList{manual, feed, ua} {
		if err := e.s.Store.SaveIPList(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	var b ipset.Builder
	for i := 0; i < 300; i++ {
		pfx, _, _ := ipset.ParseEntry(fmt.Sprintf("10.0.%d.%d", i/100, (i%100)*2))
		b.Add(pfx)
	}
	info, err := lists.WriteBuilder(&b)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	if err := e.s.Store.SetIPListFetch(ctx, feed.ID, store.ListFetch{Data: &store.ListData{Hash: info.Hash, Items: info.Networks, Ranges: info.Ranges}, CheckedAt: at, FetchedAt: &at}); err != nil {
		t.Fatal(err)
	}

	// listing: counts, no entries for the file-backed list
	w := getRaw(e, "/api/ip-lists", viewer)
	if w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	got := map[string]map[string]any{}
	for _, l := range decodeList(t, w.Body.Bytes()) {
		got[l["name"].(string)] = l
	}
	if f := got["blocklist"]; f["item_count"].(float64) != 300 || f["range_count"].(float64) != 300 || len(f["items"].([]any)) != 0 || f["data_missing"] != nil {
		t.Fatalf("feed list: %v", f)
	}
	if m := got["office"]; m["item_count"].(float64) != 3 {
		t.Fatalf("manual list: %v", m)
	}

	// preview
	_, m := getJSON(e, fmt.Sprintf("/api/ip-lists/%d/entries?limit=2", feed.ID), viewer)
	if fmt.Sprint(m["entries"]) != "[10.0.0.0 10.0.0.2]" || m["truncated"] != true || m["total"].(float64) != 300 || m["ranges"].(float64) != 300 {
		t.Fatalf("feed preview: %v", m)
	}
	_, m = getJSON(e, fmt.Sprintf("/api/ip-lists/%d/entries", manual.ID), viewer)
	if len(m["entries"].([]any)) != 3 || m["truncated"] != false {
		t.Fatalf("manual preview: %v", m)
	}
	if w, _ := getJSON(e, fmt.Sprintf("/api/ip-lists/%d/entries?limit=x", feed.ID), viewer); w.Code != 400 {
		t.Fatalf("bad limit: %d", w.Code)
	}
	if w, _ := getJSON(e, "/api/ip-lists/999/entries", viewer); w.Code != 404 {
		t.Fatalf("unknown list: %d", w.Code)
	}

	// check an address
	for _, c := range []struct {
		id    int64
		ip    string
		match bool
		rng   string
	}{
		{feed.ID, "10.0.1.4", true, "10.0.1.4"},
		{feed.ID, "10.0.1.5", false, ""},
		{feed.ID, "::ffff:10.0.2.98", true, "10.0.2.98"},
		{manual.ID, "192.0.2.200", true, "192.0.2.0/24"},
		{manual.ID, "2001:db8:1::1", true, "2001:db8::/32"},
		{manual.ID, "203.0.113.1", false, ""},
	} {
		w, m := getJSON(e, fmt.Sprintf("/api/ip-lists/%d/check?ip=%s", c.id, c.ip), viewer)
		if w.Code != 200 || m["match"] != c.match || (c.match && m["range"] != c.rng) {
			t.Errorf("check %s in %d: %d %v", c.ip, c.id, w.Code, m)
		}
	}
	if w, _ := getJSON(e, fmt.Sprintf("/api/ip-lists/%d/check?ip=nope", feed.ID), viewer); w.Code != 400 {
		t.Fatalf("bad address: %d", w.Code)
	}
	if w, _ := getJSON(e, fmt.Sprintf("/api/ip-lists/%d/check?ip=1.2.3.4", ua.ID), viewer); w.Code != 409 {
		t.Fatalf("UA list: %d", w.Code)
	}

	// the file goes missing (restored backup)
	if err := os.Remove(lists.File(info.Hash)); err != nil {
		t.Fatal(err)
	}
	w = getRaw(e, "/api/ip-lists", viewer)
	for _, l := range decodeList(t, w.Body.Bytes()) {
		if l["name"] == "blocklist" && l["data_missing"] != true {
			t.Fatalf("missing data not flagged: %v", l)
		}
	}
	if _, m := getJSON(e, fmt.Sprintf("/api/ip-lists/%d/entries", feed.ID), viewer); m["missing"] != true {
		t.Fatalf("preview of missing data: %v", m)
	}
	if _, m := getJSON(e, fmt.Sprintf("/api/ip-lists/%d/check?ip=10.0.0.0", feed.ID), viewer); m["missing"] != true || m["match"] != false {
		t.Fatalf("check of missing data: %v", m)
	}
}

func getJSON(e *testEnv, path, cookie string) (*httptest.ResponseRecorder, map[string]any) {
	return e.do("GET", path, nil, withCookie(cookie))
}

func getRaw(e *testEnv, path, cookie string) *httptest.ResponseRecorder {
	w, _ := e.do("GET", path, nil, withCookie(cookie))
	return w
}

func decodeList(t *testing.T, b []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
