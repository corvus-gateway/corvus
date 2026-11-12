package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/deploy"
	"corvus/control/internal/gateway"
)

func failBody(t *testing.T, err error) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	fail(w, err)
	var m map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &m); e != nil {
		t.Fatalf("body %q: %v", w.Body.String(), e)
	}
	return w.Code, m
}

func TestFailHints(t *testing.T) {
	out := "nginx: [emerg] unknown directive \"proxy_foo\" in /data/nginx/versions/v000012/hosts/3.conf:25\nnginx: configuration file test failed"
	code, m := failBody(t, fmt.Errorf("apply: %w", &deploy.TestError{Output: out, VersionID: 7}))
	if code != 422 || m["error"] != "config_rejected" || m["hint"] != "deploy.advanced_config" {
		t.Errorf("config_rejected: %d %v", code, m)
	}
	if _, m := failBody(t, &deploy.TestError{Output: "nginx: [emerg] cannot load certificate"}); m["hint"] != "" {
		t.Errorf("unrecognised output got hint %v", m["hint"])
	}
	// the message does not say why the gateway kept the old configuration: no hint
	code, m = failBody(t, fmt.Errorf("x: %w", &gateway.NotAcceptedError{Want: "v000009", Serving: "v000008"}))
	if _, ok := m["hint"]; code != 422 || m["error"] != "reload_rejected" || ok || m["message"] == "" || m["version_id"] != nil {
		t.Errorf("reload_rejected: %d %v", code, m)
	}
	// with the gateway's error line the cause is known: message and hint
	code, m = failBody(t, &gateway.NotAcceptedError{Want: "v000009", Serving: "v000008", Log: []string{
		"2026/11/12 01:55:16 [emerg] 1#1: bind() to 0.0.0.0:8090 failed (98: Address already in use)"}})
	if msg, _ := m["message"].(string); code != 422 || m["hint"] != "deploy.port_in_use" || !strings.Contains(msg, "0.0.0.0:8090") {
		t.Errorf("reload_rejected with gateway log: %d %v", code, m)
	}
	code, m = failBody(t, &deploy.ReloadError{VersionID: 4, Err: &gateway.NotAcceptedError{Want: "v000009", Serving: "v000008"}})
	if code != 422 || m["error"] != "reload_rejected" || m["version_id"] != float64(4) {
		t.Errorf("reload_rejected with version: %d %v", code, m)
	}
	// unrelated errors stay 500 without details
	if code, m := failBody(t, fmt.Errorf("boom")); code != 500 || m["hint"] != nil {
		t.Errorf("internal: %d %v", code, m)
	}

	w := httptest.NewRecorder()
	writeErrHint(w, 502, "fetch_failed", "m", "")
	if _, ok := decodeMap(w)["hint"]; ok {
		t.Error("empty hint must be omitted")
	}
	w = httptest.NewRecorder()
	failFieldHint(w, "issuer", "discovery", "no such host", "net.dns")
	if m := decodeMap(w); w.Code != 422 || m["error"] != "validation" || m["hint"] != "net.dns" || m["fields"] == nil {
		t.Errorf("failFieldHint: %d %v", w.Code, m)
	}
}

func decodeMap(w *httptest.ResponseRecorder) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return m
}

// notAcceptGW passes the configuration test but keeps serving the previous version.
type notAcceptGW struct{ fakeGW }

func (g *notAcceptGW) WaitActive(_ context.Context, name string, _ time.Duration) error {
	return &gateway.NotAcceptedError{Want: name, Serving: "previous"}
}

// A configuration the gateway did not take at reload time is reported as 422
// reload_rejected (not 500) and recorded as a failed version.
func TestReloadRejected(t *testing.T) {
	e := newFullEnv(t)
	e.must(201, "POST", "/api/deployments", map[string]any{"message": "first"})
	e.s.Deployer.Runner = &notAcceptGW{}
	code, m := e.call("POST", "/api/deployments", map[string]any{"message": "port clash"})
	if _, ok := m["hint"]; code != 422 || m["error"] != "reload_rejected" || ok {
		t.Fatalf("redeploy: %d %v", code, m)
	}
	vs, err := e.s.Store.FailedVersionsSince(context.Background(), 0)
	if err != nil || len(vs) != 1 || vs[0].Message != "port clash" || vs[0].TestOutput == "" || m["version_id"] != float64(vs[0].ID) {
		t.Errorf("failed version = %+v, %v (response %v)", vs, err, m)
	}
}
