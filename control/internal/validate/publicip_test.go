package validate

import (
	"net"
	"testing"
)

func TestPublicIPTransitionAddresses(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8":                              true,
		"2606:4700::1":                         true,
		"64:ff9b::808:808":                     true,  // NAT64 of 8.8.8.8
		"2002:808:808::1":                      true,  // 6to4 of 8.8.8.8
		"64:ff9b::7f00:1":                      false, // NAT64 of 127.0.0.1
		"64:ff9b::a00:1":                       false, // NAT64 of 10.0.0.1
		"64:ff9b::a9fe:a9fe":                   false, // NAT64 of 169.254.169.254
		"2002:c0a8:101::1":                     false, // 6to4 of 192.168.1.1
		"64:ff9b:1::808:808":                   false, // local-use NAT64
		"2001:0:4136:e378:8000:63bf:3fff:fdd2": false, // Teredo
		"127.0.0.1":                            false,
		"2001:db8::1":                          false,
	}
	for ip, want := range cases {
		if got := PublicIP(net.ParseIP(ip)); got != want {
			t.Errorf("PublicIP(%s) = %v, want %v", ip, got, want)
		}
	}
	if err := FeedURL("https://[64:ff9b::7f00:1]/list.txt"); err == nil {
		t.Error("feed URL with an embedded loopback address accepted")
	}
}
