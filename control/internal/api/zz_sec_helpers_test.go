package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Helpers shared by the zz_sec_* tests (security, traffic, alerts, retention handlers).

// zsAs sends a request with the session cookie of another user (see fullEnv.as).
func (e *fullEnv) zsAs(cookie, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	e.t.Helper()
	return e.do(method, path, body, withCookie(cookie), withCSRF())
}

// zsList decodes a JSON array answer of an admin GET request.
func (e *fullEnv) zsList(path string) []map[string]any {
	e.t.Helper()
	w, _ := e.do("GET", path, nil, withCookie(e.cookie))
	if w.Code != 200 {
		e.t.Fatalf("GET %s: %d %s", path, w.Code, w.Body)
	}
	var out []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		e.t.Fatalf("GET %s: not a JSON array: %s", path, w.Body)
	}
	return out
}

// zsAudit returns the audit log actions, newest first.
func zsAudit(t *testing.T, e *fullEnv) []string {
	t.Helper()
	list, err := e.s.Store.ListAudit(context.Background(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.Action)
	}
	return out
}

func zsCount(list []string, v string) int {
	n := 0
	for _, x := range list {
		if x == v {
			n++
		}
	}
	return n
}

// zsField returns the field name of the first validation error of a 422 answer.
func zsField(m map[string]any) string {
	f, _ := m["fields"].([]any)
	if len(f) == 0 {
		return ""
	}
	first, _ := f[0].(map[string]any)
	s, _ := first["field"].(string)
	return s
}

// zsCode returns the code of the first validation error of a 422 answer.
func zsCode(m map[string]any) string {
	f, _ := m["fields"].([]any)
	if len(f) == 0 {
		return ""
	}
	first, _ := f[0].(map[string]any)
	s, _ := first["code"].(string)
	return s
}

// httptestRequestFrom returns a GET request from the given remote address.
func httptestRequestFrom(remote string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = remote
	return r
}

// zsObj returns m[key] as an object.
func zsObj(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	o, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("no object %q in %v", key, m)
	}
	return o
}
