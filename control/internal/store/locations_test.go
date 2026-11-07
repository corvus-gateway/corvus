package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestProxyHostLocationsRoundTrip(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h := &ProxyHost{Domains: []string{"a.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80}
	if err := st.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	got, err := st.ProxyHostByID(ctx, h.ID)
	if err != nil || got.Locations == nil || len(got.Locations) != 0 {
		t.Fatalf("new host: %+v %v", got.Locations, err)
	}
	loc := Location{Path: "/api", ForwardScheme: "https", ForwardHost: "api", ForwardPort: 8443, ForwardPath: "/v2/", AdvancedConfig: "proxy_read_timeout 5s;"}
	h.Locations = []Location{loc}
	if err := st.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	list, err := st.ListProxyHosts(ctx)
	if err != nil || len(list) != 1 || len(list[0].Locations) != 1 || list[0].Locations[0] != loc {
		t.Fatalf("locations not stored: %+v %v", list, err)
	}
}
