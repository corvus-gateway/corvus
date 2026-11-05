package compiler

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

func TestRenderUnknownHosts(t *testing.T) {
	const notFound = `        location / {
            default_type text/html;
            return 404 "<!doctype html><meta charset=utf-8><title>Corvus</title><p>No site is configured for this host.</p>\n";
        }
    }
`
	for _, c := range []struct {
		name string
		set  *store.UnknownHosts
		want string // the default server's `location /` block
	}{
		{"default", nil, notFound},
		{"not found", &store.UnknownHosts{Mode: "not_found"}, notFound},
		{"close", &store.UnknownHosts{Mode: "close"}, "        location / {\n            return 444;\n        }\n    }\n"},
		{"redirect", &store.UnknownHosts{Mode: "redirect", RedirectURL: "https://www.example.com/welcome"},
			"        location / {\n            return 302 https://www.example.com/welcome;\n        }\n    }\n"},
	} {
		comp, st := renderPagesEnv(t)
		ctx := context.Background()
		if c.set != nil {
			if err := st.SetUnknownHosts(ctx, *c.set); err != nil {
				t.Fatal(err)
			}
		}
		dir := filepath.Join(t.TempDir(), "v1")
		if err := comp.Render(ctx, st, dir); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		conf := readFile(t, filepath.Join(dir, "nginx.conf"))
		i := strings.Index(conf, "# Requests for unknown hosts.")
		if i < 0 {
			t.Fatalf("%s: no default server", c.name)
		}
		def := conf[i:]
		def = def[:strings.Index(def, "include ")]
		for _, want := range []string{c.want, "ssl_reject_handshake on;", "location ^~ /.well-known/acme-challenge/ {", "location = /__corvus/healthz {"} {
			if !strings.Contains(def, want) {
				t.Errorf("%s: default server missing %q:\n%s", c.name, want, def)
			}
		}
	}

	// A stored value that does not validate fails the render instead of reaching nginx.
	comp, st := renderPagesEnv(t)
	ctx := context.Background()
	if err := st.SetUnknownHosts(ctx, store.UnknownHosts{Mode: "redirect", RedirectURL: "https://example.com; return 200"}); err != nil {
		t.Fatal(err)
	}
	if err := comp.Render(ctx, st, filepath.Join(t.TempDir(), "v1")); err == nil {
		t.Error("invalid redirect URL rendered")
	}
}
