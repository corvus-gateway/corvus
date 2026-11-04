package validate

import (
	"testing"

	"corvus/control/internal/store"
)

func TestRedirectionHost(t *testing.T) {
	cert := int64(1)
	h := store.RedirectionHost{Domains: []string{"Old.Example.com", "old.example.com"}, ForwardScheme: "$scheme", ForwardDomain: " New.Example.com/Path?x=1 "}
	if err := RedirectionHost(&h); err != nil {
		t.Fatal(err)
	}
	if len(h.Domains) != 1 || h.Domains[0] != "old.example.com" || h.ForwardScheme != "auto" || h.HTTPCode != 301 || h.ForwardDomain != "new.example.com/Path?x=1" {
		t.Fatalf("not normalized: %+v", h)
	}
	for _, ok := range []string{"example.com", "10.0.0.1:8080", "example.com:8443/a/b", "example.com/a-b_c.d~e/%20?q=1&r=2"} {
		h := store.RedirectionHost{Domains: []string{"a.example.com"}, ForwardDomain: ok, HTTPCode: 308, CertificateID: &cert, SSLForced: true}
		if err := RedirectionHost(&h); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	cases := map[string]func(h *store.RedirectionHost){
		"target injection":   func(h *store.RedirectionHost) { h.ForwardDomain = "example.com; include /etc/passwd" },
		"target newline":     func(h *store.RedirectionHost) { h.ForwardDomain = "example.com/\nreturn" },
		"target variable":    func(h *store.RedirectionHost) { h.ForwardDomain = "example.com/$host" },
		"target brace":       func(h *store.RedirectionHost) { h.ForwardDomain = "example.com/}" },
		"target quote":       func(h *store.RedirectionHost) { h.ForwardDomain = `example.com/"` },
		"target with scheme": func(h *store.RedirectionHost) { h.ForwardDomain = "https://example.com" },
		"target wildcard":    func(h *store.RedirectionHost) { h.ForwardDomain = "*.example.com" },
		"target empty":       func(h *store.RedirectionHost) { h.ForwardDomain = "" },
		"bad port":           func(h *store.RedirectionHost) { h.ForwardDomain = "example.com:99999" },
		"bad scheme":         func(h *store.RedirectionHost) { h.ForwardScheme = "ftp" },
		"bad code":           func(h *store.RedirectionHost) { h.HTTPCode = 200 },
		"no domains":         func(h *store.RedirectionHost) { h.Domains = nil },
		"bad domain":         func(h *store.RedirectionHost) { h.Domains = []string{"a.com;"} },
		"ssl without cert":   func(h *store.RedirectionHost) { h.SSLForced = true },
		"advanced lua":       func(h *store.RedirectionHost) { h.AdvancedConfig = "content_by_lua_block { os.exit() }" },
	}
	for name, mut := range cases {
		h := store.RedirectionHost{Domains: []string{"a.example.com"}, ForwardScheme: "https", ForwardDomain: "example.com", HTTPCode: 302}
		mut(&h)
		if err := RedirectionHost(&h); err == nil {
			t.Errorf("%s: accepted %+v", name, h)
		}
	}
}
