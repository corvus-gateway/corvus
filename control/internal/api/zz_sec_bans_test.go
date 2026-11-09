package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/autoban"
)

func TestSecBans(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")
	op := e.as("operator")
	e.s.AutoBan = &autoban.Service{Store: e.s.Store, Traffic: e.s.Traffic}
	e.h = e.s.Handler()

	for _, c := range []struct {
		body  map[string]any
		field string
	}{
		{map[string]any{"ip": "10.0.0.0/8"}, "ip"},
		{map[string]any{"ip": "nope"}, "ip"},
		{map[string]any{"ip": "127.0.0.1"}, "ip"},
		{map[string]any{"ip": "::"}, "ip"},
		{map[string]any{"ip": "203.0.113.5", "duration_s": -1}, "duration_s"},
		{map[string]any{"ip": "203.0.113.5", "duration_s": 366 * 86400}, "duration_s"},
	} {
		m := e.must(422, "POST", "/api/bans", c.body)
		if zsField(m) != c.field {
			t.Errorf("%v: field %q", c.body, zsField(m))
		}
	}
	e.must(400, "POST", "/api/bans", map[string]any{"ip": "203.0.113.5", "extra": 1})
	if w, _ := e.zsAs(viewer, "POST", "/api/bans", map[string]any{"ip": "203.0.113.5"}); w.Code != 403 {
		t.Fatalf("viewer ban: %d", w.Code)
	}

	// timed ban (operator); an invalid reason falls back to "manual"
	before := time.Now()
	w, m := e.zsAs(op, "POST", "/api/bans", map[string]any{"ip": " 203.0.113.5 ", "duration_s": 3600, "reason": "bad\x01guy"})
	if w.Code != 201 {
		t.Fatalf("ban: %d %s", w.Code, w.Body)
	}
	if m["ip"] != "203.0.113.5" || m["reason"] != "manual" || m["source"] != "manual" || m["offenses"].(float64) != 1 || m["expires_at"] == nil {
		t.Fatalf("ban: %v", m)
	}
	exp, _ := time.Parse(time.RFC3339, m["expires_at"].(string))
	if d := exp.Sub(before); d < 59*time.Minute || d > 61*time.Minute {
		t.Fatalf("expiry in %v", d)
	}
	bid := id(m)

	// banning again counts another offense; duration 0 = until removed
	m = e.must(201, "POST", "/api/bans", map[string]any{"ip": "203.0.113.5", "reason": "scanner"})
	if m["offenses"].(float64) != 2 || m["expires_at"] != nil || m["reason"] != "scanner" {
		t.Fatalf("re-ban: %v", m)
	}
	m = e.must(201, "POST", "/api/bans", map[string]any{"ip": "2001:db8::1", "duration_s": 60})
	v6 := id(m)

	w, _ = e.zsAs(viewer, "GET", "/api/bans", nil)
	var list []map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list) != 2 {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}

	// make permanent
	m = e.must(200, "POST", fmt.Sprintf("/api/bans/%d/permanent", v6), nil)
	if m["expires_at"] != nil || m["ip"] != "2001:db8::1" {
		t.Fatalf("permanent: %v", m)
	}
	e.must(404, "POST", "/api/bans/999/permanent", nil)
	e.must(400, "POST", "/api/bans/x/permanent", nil)
	if w, _ := e.zsAs(viewer, "POST", fmt.Sprintf("/api/bans/%d/permanent", v6), nil); w.Code != 403 {
		t.Fatalf("viewer permanent: %d", w.Code)
	}

	// lift
	m = e.must(200, "DELETE", fmt.Sprintf("/api/bans/%d", bid), nil)
	if m["status"] != "ok" {
		t.Fatalf("lift: %v", m)
	}
	if l := e.zsList("/api/bans"); len(l) != 1 || l[0]["ip"] != "2001:db8::1" {
		t.Fatalf("after lift: %v", l)
	}
	e.must(404, "DELETE", "/api/bans/999", nil)
	e.must(400, "DELETE", "/api/bans/0", nil)

	a := zsAudit(t, e)
	for act, n := range map[string]int{"ban.create": 3, "ban.permanent": 1, "ban.delete": 1} {
		if zsCount(a, act) != n {
			t.Errorf("audit %s: %d (%v)", act, zsCount(a, act), a)
		}
	}
}

func TestSecAutoBanSettings(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")
	op := e.as("operator")

	w, m := e.zsAs(viewer, "GET", "/api/settings/autoban", nil)
	def := autoban.DefaultSettings()
	if w.Code != 200 || m["enabled"] != def.Enabled || m["threshold"].(float64) != float64(def.Threshold) {
		t.Fatalf("defaults: %d %v", w.Code, m)
	}

	valid := map[string]any{"enabled": true, "threshold": 5, "window_s": 120, "duration_s": 600, "max_duration_s": 0,
		"permanent_after": 3, "include_scans": true, "scan_threshold": 0, "exempt": []string{"10.0.0.1"},
		"bad_behaviour": true, "bad_behaviour_codes": []int{404, 403, 404}, "bad_behaviour_threshold": 0, "bad_behaviour_window_s": 0}
	if w, _ := e.zsAs(viewer, "PUT", "/api/settings/autoban", valid); w.Code != 403 {
		t.Fatalf("viewer put: %d", w.Code)
	}
	for _, bad := range []map[string]any{
		{"threshold": 1, "window_s": 120, "duration_s": 600},
		{"threshold": 5, "window_s": 10, "duration_s": 600},
		{"threshold": 5, "window_s": 120, "duration_s": 600, "permanent_after": 1},
		{"threshold": 5, "window_s": 120, "duration_s": 600, "bad_behaviour_codes": []int{200}},
		{"threshold": 5, "window_s": 120, "duration_s": 600, "exempt": []string{"x"}},
	} {
		m := e.must(422, "PUT", "/api/settings/autoban", bad)
		if zsField(m) != "autoban" {
			t.Errorf("%v: %v", bad, m)
		}
	}
	e.must(400, "PUT", "/api/settings/autoban", map[string]any{"threshold": "five"})

	// operator saves; normalised values come back and are stored
	e.s.AutoBan = &autoban.Service{Store: e.s.Store}
	e.h = e.s.Handler()
	w, m = e.zsAs(op, "PUT", "/api/settings/autoban", valid)
	if w.Code != 200 {
		t.Fatalf("put: %d %s", w.Code, w.Body)
	}
	if m["max_duration_s"].(float64) != 7*86400 || m["scan_threshold"].(float64) != 200 || m["bad_behaviour_threshold"].(float64) != 10 ||
		m["bad_behaviour_window_s"].(float64) != 60 {
		t.Fatalf("normalised: %v", m)
	}
	if c := m["bad_behaviour_codes"].([]any); len(c) != 2 || c[0].(float64) != 403 {
		t.Fatalf("codes: %v", c)
	}
	got := autoban.Load(context.Background(), e.s.Store)
	if got.Threshold != 5 || got.PermanentAfter != 3 || len(got.Exempt) != 1 || got.Exempt[0] != "10.0.0.1/32" {
		t.Fatalf("stored: %+v", got)
	}
	m = e.must(200, "GET", "/api/settings/autoban", nil)
	if m["window_s"].(float64) != 120 || m["include_scans"] != true {
		t.Fatalf("get after put: %v", m)
	}
	if zsCount(zsAudit(t, e), "autoban.update") != 1 {
		t.Fatal("not audited")
	}
}

// CORVUS_ADMIN_ALLOW_ANY=1 (lock-out recovery) disables the console allowlist.
func TestSecAdminAllowBypass(t *testing.T) {
	e := newFullEnv(t)
	n, _ := e.s.Store.Network(context.Background())
	n.AdminAllow = []string{"192.0.2.0/24"}
	if err := e.s.Store.SetNetwork(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	outside := func() int {
		r := httptestRequestFrom("198.51.100.7:4000")
		r.URL.Path = "/api/status"
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, r)
		return w.Code
	}
	e.s.invalidateAllow()
	if c := outside(); c != 403 {
		t.Fatalf("outside without bypass: %d", c)
	}
	t.Setenv("CORVUS_ADMIN_ALLOW_ANY", "1")
	e.h = e.s.Handler()
	if c := outside(); c != 200 {
		t.Fatalf("outside with bypass: %d", c)
	}
}

func TestSecAdminAllowValidation(t *testing.T) {
	r := httptestRequestFrom("203.0.113.9:1234")
	if l, err := checkAdminAllow(r, nil); err != nil || len(l) != 0 {
		t.Fatalf("empty: %v %v", l, err)
	}
	if _, err := checkAdminAllow(r, []string{"203.0.113.0/24", "garbage"}); err == nil || !strings.Contains(err.Error(), "admin_allow") {
		t.Fatalf("invalid: %v", err)
	}
	if l, err := checkAdminAllow(r, []string{"198.51.100.0/24", "203.0.113.9"}); err != nil || len(l) != 2 {
		t.Fatalf("own address: %v %v", l, err)
	}
}
