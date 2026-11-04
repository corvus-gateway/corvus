package validate

import (
	"testing"

	"corvus/control/internal/store"
)

func TestDomain(t *testing.T) {
	good := []string{"example.com", "a.b.example.co.kr", "*.example.com", "localhost", "xn--9t4b11yi5a.com"}
	bad := []string{"", "example..com", "-a.com", "a-.com", "exa mple.com", "*.*.com", "a.com;", "a.com\n", "$host", "a.com}"}
	for _, d := range good {
		if !Domain(d) {
			t.Errorf("expected valid: %q", d)
		}
	}
	for _, d := range bad {
		if Domain(d) {
			t.Errorf("expected invalid: %q", d)
		}
	}
}

func validHost() store.ProxyHost {
	return store.ProxyHost{Domains: []string{"Shop.Example.com", "shop.example.com", " "}, ForwardHost: "app", ForwardPort: 8080, Enabled: true}
}

func TestProxyHostNormalizes(t *testing.T) {
	h := validHost()
	if err := ProxyHost(&h); err != nil {
		t.Fatal(err)
	}
	if len(h.Domains) != 1 || h.Domains[0] != "shop.example.com" || h.ForwardScheme != "http" || h.WaitroomPath != "/" {
		t.Fatalf("not normalized: %+v", h)
	}
}

func TestProxyHostSecurityHeaders(t *testing.T) {
	h := validHost()
	if err := ProxyHost(&h); err != nil || h.SecurityHeaders != "off" {
		t.Fatalf("default: %q %v", h.SecurityHeaders, err)
	}
	for _, v := range []string{"off", "basic", "strict"} {
		h := validHost()
		h.SecurityHeaders = v
		if err := ProxyHost(&h); err != nil || h.SecurityHeaders != v {
			t.Errorf("%s: %q %v", v, h.SecurityHeaders, err)
		}
	}
	for _, v := range []string{"STRICT", "on", `basic")`, "strict; }"} {
		h := validHost()
		h.SecurityHeaders = v
		err := ProxyHost(&h)
		if fe, ok := err.(Errors); !ok || len(fe) != 1 || fe[0].Field != "security_headers" {
			t.Errorf("%q: expected a security_headers error, got %v", v, err)
		}
	}
}

func TestProxyHostRejectsInjection(t *testing.T) {
	cases := map[string]func(h *store.ProxyHost){
		"upstream injection": func(h *store.ProxyHost) { h.ForwardHost = "app;\n include /etc/passwd" },
		"bad port":           func(h *store.ProxyHost) { h.ForwardPort = 70000 },
		"bad scheme":         func(h *store.ProxyHost) { h.ForwardScheme = "gopher" },
		"path injection":     func(h *store.ProxyHost) { h.WaitroomPath = "/a { }" },
		"reserved path":      func(h *store.ProxyHost) { h.WaitroomPath = "/__corvus/x" },
		"redirect w/o cert":  func(h *store.ProxyHost) { h.SSLForced = true },
		"escape server":      func(h *store.ProxyHost) { h.AdvancedConfig = "} server { listen 1;" },
	}
	for name, mut := range cases {
		h := validHost()
		mut(&h)
		if err := ProxyHost(&h); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestAdvancedConfig(t *testing.T) {
	ok := []string{"", "location /api { proxy_read_timeout 300s; }", "add_header X-A \"}\";", "# } comment\nclient_max_body_size 1m;"}
	bad := []string{"}", "location / {", "add_header X \"unterminated;", "} {"}
	for _, s := range ok {
		if err := AdvancedConfig(s); err != nil {
			t.Errorf("expected ok %q: %v", s, err)
		}
	}
	for _, s := range bad {
		if AdvancedConfig(s) == nil {
			t.Errorf("expected error %q", s)
		}
	}
	if !HasRootLocation("  location / {\n return 200; }") || HasRootLocation("location /api {}") {
		t.Error("HasRootLocation")
	}
}

func TestWaitroom(t *testing.T) {
	w := store.Waitroom{Name: "Event", Capacity: 500, ActiveTTL: 120, GrantTTL: 30, QueueTTL: 120, Refresh: 15, FailMode: "open"}
	if err := Waitroom(&w); err != nil || w.Name != "event" {
		t.Fatalf("valid room rejected: %v", err)
	}
	w.Refresh = 100
	if Waitroom(&w) == nil {
		t.Error("refresh > active_ttl/2 accepted")
	}
	w.Refresh, w.Name = 15, "bad_name"
	if Waitroom(&w) == nil {
		t.Error("bad name accepted")
	}
}

func TestAdvancedConfigSecurity(t *testing.T) {
	bad := map[string]string{
		"hash inside token hides brace": "add_header X a#b }",
		"lua code execution":            "content_by_lua_block { ngx.say(os.getenv('CORVUS_WR_SECRET')) }",
		"lua file":                      "access_by_lua_file /tmp/x.lua;",
		"include arbitrary file":        "include /etc/passwd;",
		"env":                           "env CORVUS_WR_SECRET;",
		"alias to secrets":              "location /x { alias /data/secrets/; }",
		"root at filesystem root":       "location /x { root /; }",
		"quoted alias":                  `location /x { alias "/etc/"; }`,
		"escape server via quote trick": `add_header "X" "a"; } server { listen 1; `,
		"missing semicolon":             "proxy_read_timeout 30s",
		"unterminated variable":         "set $a ${b;",
	}
	for name, cfg := range bad {
		if AdvancedConfig(cfg) == nil {
			t.Errorf("%s: accepted %q", name, cfg)
		}
	}
	good := []string{
		"location /static/ { root /var/www/site; }",
		"proxy_set_header X-Foo ${host}bar;",
		`add_header Content-Security-Policy "default-src 'self'; img-src * data:";`,
		"client_max_body_size 20m; # comment with } brace",
		`location ~* \.(png|jpg)$ { expires 7d; }`,
	}
	for _, cfg := range good {
		if err := AdvancedConfig(cfg); err != nil {
			t.Errorf("rejected %q: %v", cfg, err)
		}
	}
}
