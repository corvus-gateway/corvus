package validate

import (
	"strings"
	"testing"

	"corvus/control/internal/store"
)

func TestProxyHostLocations(t *testing.T) {
	base := func(locs ...store.Location) *store.ProxyHost {
		return &store.ProxyHost{Domains: []string{"a.example.com"}, ForwardHost: "app", ForwardPort: 80, Locations: locs}
	}
	h := base(store.Location{Path: " /api ", ForwardScheme: "HTTPS", ForwardHost: " API.internal ", ForwardPort: 8443, ForwardPath: "/v2/"},
		store.Location{Path: "/", ForwardHost: "10.0.0.1", ForwardPort: 80, AdvancedConfig: "proxy_read_timeout 5s;"})
	if err := ProxyHost(h); err != nil {
		t.Fatal(err)
	}
	if l := h.Locations[0]; l.Path != "/api" || l.ForwardScheme != "https" || l.ForwardHost != "api.internal" {
		t.Fatalf("not normalized: %+v", l)
	}
	if h.Locations[1].ForwardScheme != "http" {
		t.Fatalf("default scheme: %+v", h.Locations[1])
	}
	if h = base(); ProxyHost(h) != nil || h.Locations == nil {
		t.Fatalf("no locations: %+v", h.Locations)
	}

	ok := store.Location{Path: "/api", ForwardHost: "api", ForwardPort: 80}
	cases := map[string]struct {
		mod   func(l *store.Location, h *store.ProxyHost)
		field string
	}{
		"relative path":   {func(l *store.Location, _ *store.ProxyHost) { l.Path = "api" }, "locations[0].path"},
		"path injection":  {func(l *store.Location, _ *store.ProxyHost) { l.Path = "/a { return 200; }" }, "locations[0].path"},
		"path regex":      {func(l *store.Location, _ *store.ProxyHost) { l.Path = "~ ^/a" }, "locations[0].path"},
		"path traversal":  {func(l *store.Location, _ *store.ProxyHost) { l.Path = "/a/../b" }, "locations[0].path"},
		"reserved corvus": {func(l *store.Location, _ *store.ProxyHost) { l.Path = "/__corvus/x" }, "locations[0].path"},
		"reserved acme":   {func(l *store.Location, _ *store.ProxyHost) { l.Path = "/.well-known/acme-challenge/" }, "locations[0].path"},
		"duplicate":       {func(_ *store.Location, h *store.ProxyHost) { h.Locations = append(h.Locations, h.Locations[0]) }, "locations[1].path"},
		"root twice": {func(l *store.Location, h *store.ProxyHost) {
			l.Path = "/"
			h.AdvancedConfig = "location / {\n return 200;\n}"
		}, "locations[0].path"},
		"bad scheme":       {func(l *store.Location, _ *store.ProxyHost) { l.ForwardScheme = "ftp" }, "locations[0].forward_scheme"},
		"bad host":         {func(l *store.Location, _ *store.ProxyHost) { l.ForwardHost = "a;b" }, "locations[0].forward_host"},
		"no host":          {func(l *store.Location, _ *store.ProxyHost) { l.ForwardHost = "" }, "locations[0].forward_host"},
		"bad port":         {func(l *store.Location, _ *store.ProxyHost) { l.ForwardPort = 0 }, "locations[0].forward_port"},
		"forward path var": {func(l *store.Location, _ *store.ProxyHost) { l.ForwardPath = "/$host" }, "locations[0].forward_path"},
		"forward path rel": {func(l *store.Location, _ *store.ProxyHost) { l.ForwardPath = "v2" }, "locations[0].forward_path"},
		"root with path":   {func(l *store.Location, _ *store.ProxyHost) { l.Path = "/"; l.ForwardPath = "/app/" }, "locations[0].forward_path"},
		"snippet lua":      {func(l *store.Location, _ *store.ProxyHost) { l.AdvancedConfig = "content_by_lua_block { os.exit() }" }, "locations[0].advanced_config"},
		"snippet escape":   {func(l *store.Location, _ *store.ProxyHost) { l.AdvancedConfig = "} server { listen 1;" }, "locations[0].advanced_config"},
		"snippet location": {func(l *store.Location, _ *store.ProxyHost) { l.AdvancedConfig = "location /x { return 200; }" }, "locations[0].advanced_config"},
	}
	for name, c := range cases {
		h := base(ok)
		c.mod(&h.Locations[0], h)
		err := ProxyHost(h)
		if err == nil || !strings.Contains(err.Error(), c.field+":") {
			t.Errorf("%s: want error on %s, got %v", name, c.field, err)
		}
	}
	many := make([]store.Location, MaxLocations+1)
	if err := ProxyHost(base(many...)); err == nil || !strings.Contains(err.Error(), "locations:") {
		t.Errorf("too many locations accepted: %v", err)
	}
}
