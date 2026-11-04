package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"corvus/control/internal/store"
)

// Location snippets are raw nginx configuration: only administrators may change them;
// operators may still edit the other location fields.
func TestLocationsAdvancedAllowed(t *testing.T) {
	type wrapped struct{ r *http.Request }
	req := func(role string) wrapped {
		r := httptest.NewRequest("PUT", "/api/proxy-hosts/1", nil)
		return wrapped{r.WithContext(context.WithValue(r.Context(), ctxKey{}, &store.User{Role: role}))}
	}
	before := []store.Location{{Path: "/api", ForwardHost: "a", ForwardPort: 80, AdvancedConfig: "proxy_read_timeout 5s;"}}
	same := []store.Location{{Path: "/api", ForwardHost: "b", ForwardPort: 81, AdvancedConfig: " proxy_read_timeout 5s; "}, {Path: "/x", ForwardHost: "x", ForwardPort: 80}}
	changed := []store.Location{{Path: "/api", ForwardHost: "a", ForwardPort: 80, AdvancedConfig: "proxy_read_timeout 9s;"}}
	moved := []store.Location{{Path: "/other", ForwardHost: "a", ForwardPort: 80, AdvancedConfig: "proxy_read_timeout 5s;"}}
	if err := locationsAdvancedAllowed(req("operator").r, before, same); err != nil {
		t.Errorf("operator, snippet unchanged: %v", err)
	}
	if err := locationsAdvancedAllowed(req("operator").r, before, nil); err != nil {
		t.Errorf("operator, locations removed: %v", err)
	}
	for _, after := range [][]store.Location{changed, moved} {
		if err := locationsAdvancedAllowed(req("operator").r, before, after); err == nil {
			t.Errorf("operator changed a snippet: %+v", after)
		}
		if err := locationsAdvancedAllowed(req("admin").r, before, after); err != nil {
			t.Errorf("admin: %v", err)
		}
	}
}

// API input: omitted locations keep the stored ones, [] removes them.
func TestProxyHostInputLocations(t *testing.T) {
	h := &store.ProxyHost{Locations: []store.Location{{Path: "/api"}}}
	var in proxyHostInput
	if err := json.Unmarshal([]byte(`{"domains":["a.example.com"]}`), &in); err != nil {
		t.Fatal(err)
	}
	in.apply(h)
	if len(h.Locations) != 1 {
		t.Fatalf("omitted locations dropped: %+v", h.Locations)
	}
	if err := json.Unmarshal([]byte(`{"locations":[]}`), &in); err != nil {
		t.Fatal(err)
	}
	in.apply(h)
	if len(h.Locations) != 0 {
		t.Fatalf("[] did not clear: %+v", h.Locations)
	}
}
