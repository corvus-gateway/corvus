package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

// Static pages through the API: old clients (no content/page) keep working and get
// forward hosts; a page host needs no forward target; omitted content/page are
// unchanged on update; switching back to forward mode works; the audit log keeps the
// size of custom HTML instead of the page; the page is rendered into the deployed
// configuration.
func TestProxyHostStaticPageAPI(t *testing.T) {
	e := newFullEnv(t)
	ctx := context.Background()
	// old shape: a forward host, content "forward" and an empty page
	m := zsObj(t, e.must(201, "POST", "/api/proxy-hosts", map[string]any{
		"domains": []string{"old.example.com"}, "forward_scheme": "http", "forward_host": "app", "forward_port": 8080,
	}), "proxy_host")
	if m["content"] != "forward" || m["page"].(map[string]any)["template"] != "" {
		t.Fatalf("old shape: %v", m)
	}
	// still required in forward mode
	w := e.must(422, "POST", "/api/proxy-hosts", map[string]any{"domains": []string{"x.example.com"}})
	if !strings.Contains(fmt.Sprint(w["fields"]), "forward_host") {
		t.Fatalf("forward target not required: %v", w)
	}

	// a page host without a forward target
	m = zsObj(t, e.must(201, "POST", "/api/proxy-hosts", map[string]any{
		"domains": []string{"maint.example.com"}, "content": "page",
		"page": map[string]any{"template": "maintenance", "message": "Back at 18:00"},
	}), "proxy_host")
	ph := id(m)
	pg := m["page"].(map[string]any)
	if m["content"] != "page" || pg["status"] != float64(503) || m["forward_host"] != "" || m["forward_port"] != float64(0) {
		t.Fatalf("page host: %v", m)
	}
	path := fmt.Sprintf("/api/proxy-hosts/%d", ph)
	live := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(e.dir, "nginx", "current", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	conf := live(fmt.Sprintf("hosts/%d.conf", ph))
	if !strings.Contains(conf, `require("corvus.page").serve(`) || !strings.Contains(conf, ", 503, true) }") || strings.Contains(conf, fmt.Sprintf("upstream corvus_up_%d {", ph)) {
		t.Fatalf("deployed page host:\n%s", conf)
	}
	if p := live(fmt.Sprintf("pages/host-%d.html", ph)); !strings.Contains(p, "Back at 18:00") {
		t.Fatalf("deployed page:\n%s", p)
	}

	// an older client updates the host without content/page: it keeps serving the page
	e.must(200, "PUT", path, map[string]any{"domains": []string{"maint.example.com"}, "forward_host": "", "forward_port": 0, "notes": "x"})
	h, _ := e.s.Store.ProxyHostByID(ctx, ph)
	if !h.IsPage() || h.Page.Template != store.PageMaintenance || h.Page.Message != "Back at 18:00" {
		t.Fatalf("old client update: %+v", h)
	}

	// custom HTML, exactly as entered; the audit log has its size only
	html := "<!doctype html>\n<h1>Gone</h1>" + strings.Repeat("<!-- pad -->", 2000)
	e.must(200, "PUT", path, map[string]any{"domains": []string{"maint.example.com"}, "content": "page",
		"page": map[string]any{"template": "custom", "status": 410, "html": html}})
	if p := live(fmt.Sprintf("pages/host-%d.html", ph)); p != html {
		t.Fatalf("custom page changed (%d bytes, want %d)", len(p), len(html))
	}
	list, _ := e.s.Store.ListAudit(ctx, 5)
	if len(list) == 0 || list[0].Action != "proxy_host.update" || strings.Contains(string(list[0].Detail), "pad") ||
		!strings.Contains(string(list[0].Detail), fmt.Sprintf("(%d bytes)", len(html))) {
		t.Fatalf("audit detail: %s", list[0].Detail)
	}

	// validation errors name the page fields
	for _, c := range []struct {
		page  map[string]any
		field string
	}{
		{map[string]any{"template": "custom", "html": ""}, "page.html"},
		{map[string]any{"template": "custom", "html": strings.Repeat("a", 256*1024+1)}, "page.html"},
		{map[string]any{"template": "parked", "status": 500}, "page.status"},
		{map[string]any{"template": "banner"}, "page.template"},
	} {
		w := e.must(422, "PUT", path, map[string]any{"domains": []string{"maint.example.com"}, "content": "page", "page": c.page})
		if !strings.Contains(fmt.Sprint(w["fields"]), c.field) {
			t.Errorf("%v: %v, want %s", c.page["template"], w["fields"], c.field)
		}
	}
	w = e.must(422, "PUT", path, map[string]any{"domains": []string{"maint.example.com"}, "content": "page",
		"locations": []map[string]any{{"path": "/", "forward_host": "web", "forward_port": 80}}})
	if !strings.Contains(fmt.Sprint(w["fields"]), "locations[0].path") {
		t.Errorf("location / on a page host: %v", w)
	}
	// a location that forwards is fine
	e.must(200, "PUT", path, map[string]any{"domains": []string{"maint.example.com"}, "content": "page",
		"locations": []map[string]any{{"path": "/api", "forward_host": "api", "forward_port": 9000}}})
	if conf := live(fmt.Sprintf("hosts/%d.conf", ph)); !strings.Contains(conf, fmt.Sprintf("proxy_pass http://corvus_up_%d_1;", ph)) {
		t.Fatalf("location on a page host:\n%s", conf)
	}

	// back to forward mode: the forward target is required again, the page is kept
	e.must(422, "PUT", path, map[string]any{"domains": []string{"maint.example.com"}, "content": "forward"})
	m = zsObj(t, e.must(200, "PUT", path, map[string]any{"domains": []string{"maint.example.com"}, "content": "forward",
		"forward_host": "app", "forward_port": 8080}), "proxy_host")
	if m["content"] != "forward" || m["page"].(map[string]any)["template"] != "custom" {
		t.Fatalf("switched back: %v", m)
	}
	conf = live(fmt.Sprintf("hosts/%d.conf", ph))
	if strings.Contains(conf, "corvus.page") || !strings.Contains(conf, fmt.Sprintf("upstream corvus_up_%d {", ph)) {
		t.Fatalf("forward again:\n%s", conf)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "nginx", "current", "pages", fmt.Sprintf("host-%d.html", ph))); !os.IsNotExist(err) {
		t.Fatalf("page file of a forwarding host: %v", err)
	}
	// the list carries content and page
	code, hosts := e.callList("GET", "/api/proxy-hosts")
	b, _ := json.Marshal(hosts)
	if code != 200 || !strings.Contains(string(b), `"content":"forward"`) {
		t.Fatalf("list: %d %s", code, b)
	}
}

// The console's page preview: built-in texts in the console language with the host
// name; custom HTML as entered; invalid pages answer 422.
func TestStaticPagePreview(t *testing.T) {
	e := newFullEnv(t)
	m := e.must(200, "POST", "/api/proxy-hosts/page-preview", map[string]any{"page": map[string]any{"template": "maintenance"}, "host": "shop.example.com", "lang": "ko"})
	if s, _ := m["html"].(string); !strings.Contains(s, "점검 중입니다") || !strings.Contains(s, "shop.example.com") || m["status"] != float64(503) {
		t.Fatalf("preview: %v", m)
	}
	m = e.must(200, "POST", "/api/proxy-hosts/page-preview", map[string]any{"page": map[string]any{"template": "custom", "html": "<p>{{HOST}}</p>"}})
	if m["html"] != "<p>{{HOST}}</p>" || m["status"] != float64(200) {
		t.Fatalf("custom preview: %v", m)
	}
	e.must(422, "POST", "/api/proxy-hosts/page-preview", map[string]any{"page": map[string]any{"template": "custom"}})
	viewer := e.as("viewer")
	if code, _ := e.zsAs(viewer, "POST", "/api/proxy-hosts/page-preview", map[string]any{"page": map[string]any{}}); code.Code != 403 {
		t.Fatalf("viewer preview: %d", code.Code)
	}
}
