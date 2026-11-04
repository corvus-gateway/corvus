package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

// The security header preset is rendered as one server-level header_filter that also
// carries the waiting room's cookie merge; "off" renders nothing.
func TestRenderSecurityHeaders(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := Paths{DataDir: filepath.ToSlash(dir), LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181}
	c := &Compiler{P: p}

	room := &store.Waitroom{Name: "event", Enabled: true, Capacity: 500, ActiveTTL: 120, GrantTTL: 30, QueueTTL: 120, Refresh: 15, FailMode: "open"}
	if err := st.SaveWaitroom(ctx, room); err != nil {
		t.Fatal(err)
	}
	mk := func(domain, level string, roomID *int64) {
		h := &store.ProxyHost{Domains: []string{domain}, ForwardHost: "app", ForwardPort: 8080, Enabled: true, SecurityHeaders: level, WaitroomID: roomID}
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	mk("off.example.com", "off", nil)         // 1
	mk("basic.example.com", "basic", nil)     // 2
	mk("strict.example.com", "strict", nil)   // 3
	mk("room.example.com", "off", &room.ID)   // 4
	mk("both.example.com", "basic", &room.ID) // 5

	out := filepath.Join(dir, "v1")
	if err := c.Render(ctx, st, out); err != nil {
		t.Fatal(err)
	}
	read := func(id string) string {
		b, err := os.ReadFile(filepath.Join(out, "hosts", id+".conf"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	const headers, roomFilter = `require("corvus.headers")`, `require("corvus.waitroom").header_filter()`
	cases := []struct {
		id         string
		want, deny []string
	}{
		{"1", nil, []string{"header_filter_by_lua", headers}},
		{"2", []string{`require("corvus.headers").header_filter("basic")`}, []string{roomFilter}},
		{"3", []string{`require("corvus.headers").header_filter("strict")`}, []string{roomFilter}},
		{"4", []string{roomFilter}, []string{headers}},
		{"5", []string{`require("corvus.headers").header_filter("basic")`, roomFilter}, nil},
	}
	for _, tc := range cases {
		conf := read(tc.id)
		for _, w := range tc.want {
			if !strings.Contains(conf, w) {
				t.Errorf("host %s: missing %q\n%s", tc.id, w, conf)
			}
		}
		for _, d := range tc.deny {
			if strings.Contains(conf, d) {
				t.Errorf("host %s: unexpected %q\n%s", tc.id, d, conf)
			}
		}
		// nginx rejects a second header_filter_by_lua in the same block.
		if n := strings.Count(conf, "header_filter_by_lua"); n > 1 {
			t.Errorf("host %s: %d header_filter_by_lua directives", tc.id, n)
		}
	}
	if conf := read("5"); strings.Index(conf, headers) > strings.Index(conf, roomFilter) {
		t.Error("the preset must run before the waiting room adds its own cookies")
	}
}
