package netcfg

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

func TestFetchCloudflareRejectsBroadOrMixedRanges(t *testing.T) {
	cases := map[string]*fakeTransport{
		"v4 everything":  cf(200, "173.245.48.0/20\n0.0.0.0/0\n", "2400:cb00::/32", 200),
		"v6 everything":  cf(200, "173.245.48.0/20", "::/0\n", 200),
		"v4 in v6 list":  cf(200, "173.245.48.0/20", "10.0.0.0/8\n", 200),
		"v6 in v4 list":  cf(200, "2400:cb00::/32", "2400:cb00::/32", 200),
		"v6 only mapped": cf(200, "173.245.48.0/20", "::ffff:0:0/96\n", 200),
	}
	for name, ft := range cases {
		t.Run(name, func(t *testing.T) {
			useTransport(t, ft)
			if got, err := FetchCloudflare(context.Background()); err == nil {
				t.Fatalf("accepted: %+v", got)
			}
		})
	}
}

func TestCloudflareIgnoresBroadStoredSnapshot(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	for _, c := range []store.CloudflareIPs{
		{IPv4: []string{"0.0.0.0/0"}, IPv6: []string{"2400::/16"}, Source: "remote"},
		{IPv4: []string{"1.0.0.0/8"}, IPv6: []string{"::/0"}, Source: "remote"},
		{IPv4: []string{"not-a-cidr"}, Source: "remote"},
	} {
		_ = st.SetCloudflareIPs(ctx, c)
		if got := Cloudflare(ctx, st); got.Source != "builtin" {
			t.Fatalf("%v: stored snapshot used: %+v", c, got)
		}
	}
}

func TestEmbeddedIPv4(t *testing.T) {
	cases := []struct {
		ip         string
		v4         string
		transition bool
	}{
		{"64:ff9b::7f00:1", "127.0.0.1", true},
		{"64:ff9b::a9fe:a9fe", "169.254.169.254", true},
		{"2002:7f00:1::1", "127.0.0.1", true},
		{"2002:0808:0808::", "8.8.8.8", true},
		{"64:ff9b:1::7f00:1", "", true},
		{"2001:0:4136:e378:8000:63bf:3fff:fdd2", "", true},
		{"2001:db8::1", "", false},
		{"2606:4700::1", "", false},
		{"8.8.8.8", "", false},
		{"::ffff:127.0.0.1", "", false},
	}
	for _, c := range cases {
		v4, tr := EmbeddedIPv4(net.ParseIP(c.ip))
		got := ""
		if v4 != nil {
			got = v4.String()
		}
		if got != c.v4 || tr != c.transition {
			t.Errorf("%s: got %q %v, want %q %v", c.ip, got, tr, c.v4, c.transition)
		}
	}
	if v4, tr := EmbeddedIPv4(net.IP{1, 2}); v4 != nil || tr {
		t.Fatal("invalid IP")
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(strings.ToLower(r.(error).Error()), "invalid") {
			t.Fatalf("mustCIDR: %v", r)
		}
	}()
	mustCIDR("bad")
}
