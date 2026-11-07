package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultResolver(t *testing.T) {
	cases := map[string]string{
		"nameserver 127.0.0.11\noptions ndots:0\n":     "127.0.0.11", // Docker network
		"# systemd\nnameserver 127.0.0.53\nsearch x\n": "127.0.0.53", // network_mode: host
		"nameserver 2001:4860:4860::8888\n":            "[2001:4860:4860::8888]",
		"":                                             "127.0.0.11",
	}
	for body, want := range cases {
		p := filepath.Join(t.TempDir(), "resolv.conf")
		os.WriteFile(p, []byte(body), 0o644)
		if got := defaultResolver(p); got != want {
			t.Errorf("%q: got %q, want %q", body, got, want)
		}
	}
	if got := defaultResolver(filepath.Join(t.TempDir(), "missing")); got != "127.0.0.11" {
		t.Errorf("missing file: got %q", got)
	}
}

func TestIPv6Settings(t *testing.T) {
	yes := func() bool { return true }
	no := func() bool { return false }
	cases := []struct {
		mode             string
		avail            func() bool
		listen, resolver bool
	}{
		{"", yes, true, false},
		{"auto", no, false, false},
		{"AUTO", yes, true, false},
		{"on", no, true, true},
		{"1", yes, true, true},
		{"off", yes, false, false},
		{"false", yes, false, false},
	}
	for _, c := range cases {
		l, r := ipv6Settings(c.mode, c.avail)
		if l != c.listen || r != c.resolver {
			t.Errorf("%q: got listen=%v resolver=%v", c.mode, l, r)
		}
	}
}
