package netcfg

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/store"
)

// fakeTransport answers requests from a map of URL → (status, body) without any network.
type fakeTransport struct {
	resp map[string]struct {
		code int
		body string
	}
	err  error
	seen []string
}

func (f *fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.seen = append(f.seen, r.URL.String()+" UA="+r.Header.Get("User-Agent"))
	if f.err != nil {
		return nil, f.err
	}
	x, ok := f.resp[r.URL.String()]
	if !ok {
		x.code = 404
	}
	return &http.Response{StatusCode: x.code, Body: io.NopCloser(strings.NewReader(x.body)), Header: http.Header{}, Request: r}, nil
}

// useTransport swaps http.DefaultTransport (FetchCloudflare builds a client without one).
func useTransport(t *testing.T, rt http.RoundTripper) {
	old := http.DefaultTransport
	http.DefaultTransport = rt
	t.Cleanup(func() { http.DefaultTransport = old })
}

func cf(v4code int, v4, v6 string, v6code int) *fakeTransport {
	return &fakeTransport{resp: map[string]struct {
		code int
		body string
	}{
		"https://www.cloudflare.com/ips-v4": {v4code, v4},
		"https://www.cloudflare.com/ips-v6": {v6code, v6},
	}}
}

func TestCovFetchCloudflare(t *testing.T) {
	ft := cf(200, "173.245.48.0/20\n\n103.21.244.0/22\n", "2400:cb00::/32\n", 200)
	useTransport(t, ft)
	before := time.Now().UTC().Add(-time.Second)
	got, err := FetchCloudflare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "remote" || len(got.IPv4) != 2 || got.IPv4[1] != "103.21.244.0/22" || len(got.IPv6) != 1 || got.UpdatedAt.Before(before) {
		t.Fatalf("fetched: %+v", got)
	}
	if len(ft.seen) != 2 || !strings.HasSuffix(ft.seen[0], "UA=corvus") {
		t.Fatalf("requests: %v", ft.seen)
	}
}

func TestCovFetchCloudflareErrors(t *testing.T) {
	cases := map[string]struct {
		ft   *fakeTransport
		want string
	}{
		"v4 status":   {cf(503, "", "", 200), "HTTP 503"},
		"v4 garbage":  {cf(200, "<html>", "", 200), `unexpected entry "<html>"`},
		"v4 empty":    {cf(200, "\n\n", "", 200), "empty list"},
		"v6 status":   {cf(200, "1.0.0.0/8", "", 500), "ips-v6: HTTP 500"},
		"v6 empty":    {cf(200, "1.0.0.0/8", "", 200), "empty list"},
		"transport":   {&fakeTransport{err: errors.New("dial refused")}, "dial refused"},
		"v6 bad cidr": {cf(200, "1.0.0.0/8", "2400:zz::/32", 200), "unexpected entry"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			useTransport(t, c.ft)
			got, err := FetchCloudflare(context.Background())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
			if got.Source != "" || got.IPv4 != nil {
				t.Fatalf("partial result returned: %+v", got)
			}
		})
	}
}

func TestCovCloudflareSnapshot(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	got := Cloudflare(ctx, st)
	if got.Source != "builtin" || len(got.IPv4) != len(builtinV4) || len(got.IPv6) != len(builtinV6) {
		t.Fatalf("builtin: %+v", got)
	}
	// every builtin range is a valid, normalized CIDR
	if n, err := NormalizeCIDRs(append(append([]string{}, builtinV4...), builtinV6...)); err != nil || len(n) != len(builtinV4)+len(builtinV6) {
		t.Fatalf("builtin ranges invalid: %v %v", n, err)
	}
	// an empty stored snapshot falls back to the builtin list
	_ = st.SetCloudflareIPs(ctx, store.CloudflareIPs{Source: "remote"})
	if got := Cloudflare(ctx, st); got.Source != "builtin" {
		t.Fatalf("empty snapshot: %+v", got)
	}
	_ = st.SetCloudflareIPs(ctx, store.CloudflareIPs{IPv4: []string{"1.0.0.0/8"}, IPv6: []string{"2400::/16"}, Source: "remote"})
	if got := Cloudflare(ctx, st); got.Source != "remote" || got.IPv4[0] != "1.0.0.0/8" {
		t.Fatalf("stored: %+v", got)
	}
}

func TestCovNormalizeTrustedCIDRs(t *testing.T) {
	got, err := NormalizeTrustedCIDRs([]string{"10.0.0.0/8", " 192.168.1.5 ", "2001:db8::/32", "10.0.0.0/8"})
	if err != nil || strings.Join(got, ",") != "10.0.0.0/8,192.168.1.5/32,2001:db8::/32" {
		t.Fatalf("got %v %v", got, err)
	}
	for _, in := range []string{"0.0.0.0/0", "8.0.0.0/7", "::/0", "2000::/15"} {
		if _, err := NormalizeTrustedCIDRs([]string{in}); err == nil || !strings.Contains(err.Error(), "too broad") {
			t.Errorf("%s: %v", in, err)
		}
	}
	if _, err := NormalizeTrustedCIDRs([]string{"nonsense"}); err == nil {
		t.Fatal("invalid input must fail")
	}
	if _, err := NormalizeCIDRs([]string{"1.2.3.4/99"}); err == nil {
		t.Fatal("invalid prefix must fail")
	}
	if got, _ := NormalizeCIDRs([]string{"2001:db8::1", "", "::ffff:0:0/96"}); strings.Join(got, ",") != "2001:db8::1/128" {
		t.Fatalf("v6 host / dropped mapped: %v", got)
	}
}
