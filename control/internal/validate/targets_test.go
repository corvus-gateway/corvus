package validate

import (
	"errors"
	"fmt"
	"testing"

	"corvus/control/internal/store"
)

func TestProxyHostExtraTargets(t *testing.T) {
	base := func(extra ...store.Target) *store.ProxyHost {
		return &store.ProxyHost{Domains: []string{"a.example.com"}, ForwardHost: "app", ForwardPort: 80, ExtraTargets: extra}
	}
	h := base(store.Target{ForwardHost: " App2.LAN ", ForwardPort: 80}, store.Target{ForwardHost: "10.0.0.3", ForwardPort: 8080})
	if err := ProxyHost(h); err != nil {
		t.Fatal(err)
	}
	if h.ExtraTargets[0].ForwardHost != "app2.lan" {
		t.Fatalf("not normalized: %+v", h.ExtraTargets)
	}
	if h = base(); ProxyHost(h) != nil || h.ExtraTargets == nil {
		t.Fatalf("no extra targets: %+v", h.ExtraTargets)
	}
	max := base()
	for i := 1; i < MaxTargets; i++ {
		max.ExtraTargets = append(max.ExtraTargets, store.Target{ForwardHost: fmt.Sprintf("app%d", i), ForwardPort: 80})
	}
	if err := ProxyHost(max); err != nil {
		t.Fatalf("%d targets: %v", MaxTargets, err)
	}

	cases := map[string]struct {
		extra []store.Target
		field string
		code  string
	}{
		"invalid host":         {[]store.Target{{ForwardHost: "a b", ForwardPort: 80}}, "extra_targets[0].forward_host", "invalid"},
		"injection":            {[]store.Target{{ForwardHost: "a;return 200", ForwardPort: 80}}, "extra_targets[0].forward_host", "invalid"},
		"ipv6":                 {[]store.Target{{ForwardHost: "::1", ForwardPort: 80}}, "extra_targets[0].forward_host", "ipv6_unsupported"},
		"port 0":               {[]store.Target{{ForwardHost: "b", ForwardPort: 0}}, "extra_targets[0].forward_port", "invalid"},
		"port too big":         {[]store.Target{{ForwardHost: "b", ForwardPort: 70000}}, "extra_targets[0].forward_port", "invalid"},
		"same as forward host": {[]store.Target{{ForwardHost: "APP", ForwardPort: 80}}, "extra_targets[0].forward_host", "duplicate"},
		"listed twice":         {[]store.Target{{ForwardHost: "b", ForwardPort: 80}, {ForwardHost: "b", ForwardPort: 80}}, "extra_targets[1].forward_host", "duplicate"},
		"too many":             {append(max.ExtraTargets, store.Target{ForwardHost: "one-more", ForwardPort: 80}), "extra_targets", "too_many"},
	}
	for name, c := range cases {
		err := ProxyHost(base(c.extra...))
		var errs Errors
		if !errors.As(err, &errs) {
			t.Errorf("%s: accepted", name)
			continue
		}
		found := false
		for _, f := range errs {
			found = found || (f.Field == c.field && f.Code == c.code)
		}
		if !found {
			t.Errorf("%s: %v, want %s %s", name, errs, c.field, c.code)
		}
	}
	// same host on another port is another backend
	if err := ProxyHost(base(store.Target{ForwardHost: "app", ForwardPort: 81})); err != nil {
		t.Fatalf("other port: %v", err)
	}
}
