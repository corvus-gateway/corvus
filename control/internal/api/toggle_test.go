package api

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeEnabled(t *testing.T) {
	for _, c := range []struct {
		body string
		code int
		on   bool
	}{
		{`{"enabled":true}`, 200, true},
		{`{"enabled":false}`, 200, false},
		{`{}`, 422, false},                               // required
		{`{"enabled":true,"domains":["x"]}`, 400, false}, // only the switch
		{`not json`, 400, false},
	} {
		r := httptest.NewRequest("PATCH", "/api/streams/7", strings.NewReader(c.body))
		r.SetPathValue("id", "7")
		w := httptest.NewRecorder()
		id, on, ok := decodeEnabled(w, r)
		if c.code == 200 {
			if !ok || id != 7 || on != c.on {
				t.Errorf("%s: id %d on %v ok %v", c.body, id, on, ok)
			}
			continue
		}
		if ok || w.Code != c.code {
			t.Errorf("%s: ok %v status %d, want %d", c.body, ok, w.Code, c.code)
		}
	}
	r := httptest.NewRequest("PATCH", "/api/streams/x", strings.NewReader(`{"enabled":true}`))
	r.SetPathValue("id", "x")
	if _, _, ok := decodeEnabled(httptest.NewRecorder(), r); ok {
		t.Error("invalid id accepted")
	}
}

// A switch in a list changes only "enabled": an edit another administrator saved in the
// meantime stays. Viewers can't; the change is validated, deployed and audited.
func TestPatchEnabled(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")

	ph := zsObj(t, e.must(201, "POST", "/api/proxy-hosts", map[string]any{
		"domains": []string{"p.example.com"}, "forward_scheme": "http", "forward_host": "app", "forward_port": 8080,
	}), "proxy_host")
	pid := id(ph)
	// another admin changes the upstream after this console loaded the list
	e.must(200, "PUT", fmt.Sprintf("/api/proxy-hosts/%d", pid), map[string]any{
		"domains": []string{"p.example.com"}, "forward_scheme": "http", "forward_host": "app", "forward_port": 9090, "notes": "edited elsewhere",
	})

	reloads := e.runner.reloads
	m := e.must(200, "PATCH", fmt.Sprintf("/api/proxy-hosts/%d", pid), map[string]any{"enabled": false})
	got := zsObj(t, m, "proxy_host")
	if got["enabled"] != false || got["forward_port"].(float64) != 9090 || got["notes"] != "edited elsewhere" || m["version"] == nil {
		t.Fatalf("patched host: %v", m)
	}
	if e.runner.reloads == reloads {
		t.Fatal("not deployed")
	}
	h, err := e.s.Store.ProxyHostByID(t.Context(), pid)
	if err != nil || h.Enabled || h.ForwardPort != 9090 {
		t.Fatalf("stored: %+v %v", h, err)
	}
	if w, _ := e.zsAs(viewer, "PATCH", fmt.Sprintf("/api/proxy-hosts/%d", pid), map[string]any{"enabled": true}); w.Code != 403 {
		t.Fatalf("viewer: %d", w.Code)
	}
	e.must(404, "PATCH", "/api/proxy-hosts/9999", map[string]any{"enabled": true})
	e.must(422, "PATCH", fmt.Sprintf("/api/proxy-hosts/%d", pid), map[string]any{})

	rh := zsObj(t, e.must(201, "POST", "/api/redirection-hosts", map[string]any{
		"domains": []string{"old.example.com"}, "forward_domain": "new.example.com", "http_code": 301,
	}), "redirection_host")
	m = e.must(200, "PATCH", fmt.Sprintf("/api/redirection-hosts/%d", id(rh)), map[string]any{"enabled": false})
	if got := zsObj(t, m, "redirection_host"); got["enabled"] != false || got["forward_domain"] != "new.example.com" {
		t.Fatalf("patched redirect: %v", m)
	}

	// streams: enabling is checked like an update (two enabled streams can't share a port)
	s1 := zsObj(t, e.must(201, "POST", "/api/streams", map[string]any{"incoming_port": 5432, "forward_host": "db", "forward_port": 5432}), "stream")
	s2 := zsObj(t, e.must(201, "POST", "/api/streams", map[string]any{"incoming_port": 5432, "forward_host": "db2", "forward_port": 5432, "enabled": false}), "stream")
	m = e.must(422, "PATCH", fmt.Sprintf("/api/streams/%d", id(s2)), map[string]any{"enabled": true})
	if zsField(m) != "incoming_port" {
		t.Fatalf("port conflict: %v", m)
	}
	e.must(200, "PATCH", fmt.Sprintf("/api/streams/%d", id(s1)), map[string]any{"enabled": false})
	m = e.must(200, "PATCH", fmt.Sprintf("/api/streams/%d", id(s2)), map[string]any{"enabled": true})
	if got := zsObj(t, m, "stream"); got["enabled"] != true || got["forward_host"] != "db2" {
		t.Fatalf("patched stream: %v", m)
	}

	rule := zsObj(t, e.must(201, "POST", "/api/security-rules", zsRule("office", "block", "10.0.0.0/8")), "rule")
	m = e.must(200, "PATCH", fmt.Sprintf("/api/security-rules/%d", id(rule)), map[string]any{"enabled": false})
	if got := zsObj(t, m, "rule"); got["enabled"] != false || got["name"] != "office" {
		t.Fatalf("patched rule: %v", m)
	}

	audit := zsAudit(t, e)
	for _, a := range []string{"proxy_host.update", "redirection_host.update", "stream.update", "security_rule.update"} {
		if zsCount(audit, a) == 0 {
			t.Errorf("no %s audit entry", a)
		}
	}
}
