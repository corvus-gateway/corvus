package compiler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

// A disabled room (emergency release) no longer guards the host, but its status
// endpoints and waiting page stay, so visitors still on the waiting page are told to go
// in; the policy tells the gateway the room is disabled.
func TestRenderDisabledRoom(t *testing.T) {
	c, st := renderPagesEnv(t)
	ctx := context.Background()
	room := &store.Waitroom{Name: "event", Enabled: false, Capacity: 500, ActiveTTL: 120, GrantTTL: 30, QueueTTL: 120, Refresh: 15, FailMode: "open"}
	if err := st.SaveWaitroom(ctx, room); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, wp := range []string{"/", "/event"} {
		h := &store.ProxyHost{Domains: []string{"h" + strconv.Itoa(len(ids)) + ".example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080,
			Enabled: true, WaitroomID: &room.ID, WaitroomPath: wp}
		if err := st.SaveProxyHost(ctx, h); err != nil {
			t.Fatal(err)
		}
		ids[wp] = h.ID
	}
	dir := filepath.Join(t.TempDir(), "v1")
	if err := c.Render(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
	for wp, id := range ids {
		b, err := os.ReadFile(filepath.Join(dir, "hosts", strconv.FormatInt(id, 10)+".conf"))
		if err != nil {
			t.Fatal(err)
		}
		conf := string(b)
		for _, want := range []string{`set $corvus_host_room "event";`, "location ^~ /__corvus/wr/", "try_files /wait.html =404;", "disabled: status endpoints only"} {
			if !strings.Contains(conf, want) {
				t.Errorf("path %s: missing %q", wp, want)
			}
		}
		if strings.Contains(conf, "waitroom\").access()") || strings.Contains(conf, `set $corvus_room "event"`) {
			t.Errorf("path %s: disabled room still guards the host\n%s", wp, conf)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pol struct {
		Waitrooms map[string]struct {
			Enabled bool `json:"enabled"`
		} `json:"waitrooms"`
	}
	if err := json.Unmarshal(b, &pol); err != nil || len(pol.Waitrooms) != 1 || pol.Waitrooms["event"].Enabled {
		t.Fatalf("policy: %s %v", b, err)
	}
}
