package netcfg

import (
	"reflect"
	"testing"
)

// IPv4-mapped IPv6 input becomes the IPv4 network it covers; ::ffff:0:0/96 (all of IPv4)
// is left out instead of becoming 0.0.0.0/0.
func TestNormalizeCIDRsMapped(t *testing.T) {
	got, err := NormalizeCIDRs([]string{"::ffff:0:0/96", "::ffff:192.0.2.0/120", "::ffff:198.51.100.7", "2001:db8::/32", "203.0.113.9"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"192.0.2.0/24", "198.51.100.7/32", "2001:db8::/32", "203.0.113.9/32"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got, err := NormalizeCIDRs([]string{"::ffff:0:0/100"}); err != nil || len(got) != 0 {
		t.Fatalf("broad mapped network: %v %v", got, err)
	}
}
