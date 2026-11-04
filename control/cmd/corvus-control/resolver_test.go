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
