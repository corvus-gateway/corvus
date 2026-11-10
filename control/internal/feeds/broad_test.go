package feeds

import (
	"strings"
	"testing"

	"corvus/control/internal/store"
)

func TestParseDropsVeryBroadRanges(t *testing.T) {
	feed := "0.0.0.0/0\n::/0\n1.0.0.0/7\n2000::/3\n10.0.0.0/8\n2001:db8::/16\n203.0.113.7\nbogus\n"
	items, warn, err := Parse(store.ListKindIP, []byte(feed))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(items, ",") != "10.0.0.0/8,2001::/16,203.0.113.7/32" {
		t.Fatalf("items = %v", items)
	}
	if !strings.Contains(warn, "1 invalid lines ignored") || !strings.Contains(warn, "4 ranges broader than /8 (IPv4) or /16 (IPv6) ignored") {
		t.Fatalf("warning = %q", warn)
	}
	if _, _, err := Parse(store.ListKindIP, []byte("0.0.0.0/0\n::/0\n")); err == nil {
		t.Fatal("a feed with only catch-all ranges must fail")
	}
}
