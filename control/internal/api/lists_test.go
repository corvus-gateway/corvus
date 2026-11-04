package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"corvus/control/internal/compiler"
	"corvus/control/internal/deploy"
	"corvus/control/internal/store"
)

type fakeFeeds struct {
	kicked []int64
	st     *store.Store
}

func (f *fakeFeeds) Kick(id int64) { f.kicked = append(f.kicked, id) }
func (f *fakeFeeds) Refresh(ctx context.Context, id int64) (*store.IPList, error) {
	_ = f.st.SetIPListFetch(ctx, id, store.ListFetch{Error: "HTTP 503"})
	l, _ := f.st.IPListByID(ctx, id)
	return l, fmt.Errorf("HTTP 503")
}

func TestListFeedsAPI(t *testing.T) {
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
	e.s.Deployer = &deploy.Deployer{Store: e.s.Store, Compiler: &compiler.Compiler{P: p}, Runner: okRunner{}, Dir: filepath.Join(dir, "nginx")}
	ff := &fakeFeeds{st: e.s.Store}
	e.s.Feeds = ff
	op := e.session("op@example.com", "operator")
	ctx := context.Background()

	// A feed list ignores submitted items and is fetched in the background.
	w, m := e.do("POST", "/api/ip-lists", map[string]any{"name": "tor", "source_url": "https://check.torproject.org/torbulkexitlist",
		"items": []string{"1.1.1.1"}}, withCookie(op), withCSRF())
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	l := m["ip_list"].(map[string]any)
	id := int64(l["id"].(float64))
	if len(l["items"].([]any)) != 0 || l["kind"] != "ip" || l["refresh_s"].(float64) != 3600 || len(ff.kicked) != 1 {
		t.Fatalf("created: %v kicked=%v", l, ff.kicked)
	}
	// Fetched entries survive an edit of the name.
	if err := e.s.Store.SetIPListFetch(ctx, id, store.ListFetch{Items: []string{"9.9.9.9/32"}}); err != nil {
		t.Fatal(err)
	}
	w, m = e.do("PUT", fmt.Sprintf("/api/ip-lists/%d", id), map[string]any{"name": "Tor exits", "source_url": "https://check.torproject.org/torbulkexitlist",
		"refresh_s": 7200, "items": []string{}}, withCookie(op), withCSRF())
	if w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body)
	}
	if it := m["ip_list"].(map[string]any)["items"].([]any); len(it) != 1 || it[0] != "9.9.9.9/32" || len(ff.kicked) != 1 {
		t.Fatalf("items after edit: %v kicked=%v", it, ff.kicked)
	}
	// Private targets are refused.
	if w, _ := e.do("POST", "/api/ip-lists", map[string]any{"name": "x", "source_url": "https://127.0.0.1/list"}, withCookie(op), withCSRF()); w.Code != 422 {
		t.Fatalf("private URL: %d %s", w.Code, w.Body)
	}
	// Refresh reports the fetch error in the list.
	w, m = e.do("POST", fmt.Sprintf("/api/ip-lists/%d/refresh", id), nil, withCookie(op), withCSRF())
	if w.Code != 200 || m["ip_list"].(map[string]any)["fetch_error"] != "HTTP 503" {
		t.Fatalf("refresh: %d %s", w.Code, w.Body)
	}

	// UA list + rule; its kind cannot change while in use.
	w, m = e.do("POST", "/api/ip-lists", map[string]any{"name": "bots", "kind": "user_agent", "items": []string{"AhrefsBot"}}, withCookie(op), withCSRF())
	if w.Code != 201 {
		t.Fatalf("UA list: %d %s", w.Code, w.Body)
	}
	ua := int64(m["ip_list"].(map[string]any)["id"].(float64))
	rule := map[string]any{"name": "bots", "action": "block", "expression": map[string]any{"groups": [][]map[string]any{{{"field": "http.user_agent_list", "operator": "in", "value": []string{fmt.Sprintf("list:%d", ua)}}}}}}
	if w, _ := e.do("POST", "/api/security-rules", rule, withCookie(op), withCSRF()); w.Code != 201 {
		t.Fatalf("UA rule: %d %s", w.Code, w.Body)
	}
	if w, _ := e.do("PUT", fmt.Sprintf("/api/ip-lists/%d", ua), map[string]any{"name": "bots", "kind": "ip", "items": []string{}}, withCookie(op), withCSRF()); w.Code != 422 {
		t.Fatalf("kind change of a used list: %d %s", w.Code, w.Body)
	}
}
