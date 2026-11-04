package compiler

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, name, body string) string {
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUpstreamAddr(t *testing.T) {
	hosts := StaticHosts(write(t, "hosts", "127.0.0.1 localhost\n::1 localhost ip6-localhost\n172.17.0.1 host.docker.internal # extra_hosts\n"))
	cases := []struct{ in, gw, want string }{
		{"127.0.0.1", "172.18.0.1", "172.18.0.1"}, // loopback = Docker host
		{"localhost", "172.18.0.1", "172.18.0.1"},
		{"::1", "172.18.0.1", "172.18.0.1"},
		{"127.0.0.1", "", "127.0.0.1"}, // host networking: loopback already is the host
		{"host.docker.internal", "", "172.17.0.1"},
		{"xoshop-web", "172.18.0.1", "xoshop-web"}, // container names are resolved by Docker DNS
		{"10.3.2.39", "172.18.0.1", "10.3.2.39"},
		{"fd00::5", "", "[fd00::5]"},
	}
	for _, c := range cases {
		if got := upstreamAddr(c.in, hosts, c.gw); got != c.want {
			t.Errorf("upstreamAddr(%q, gw=%q) = %q, want %q", c.in, c.gw, got, c.want)
		}
	}
}

func TestHostGateway(t *testing.T) {
	bridgeDev := write(t, "dev", "Inter-|\n lo: 0 0\n eth0: 0 0\n")
	hostDev := write(t, "dev", "Inter-|\n lo: 0 0\n docker0: 0 0\n ens3: 0 0\n")
	route := write(t, "route", "Iface\tDestination\tGateway\tFlags\neth0\t00000000\t010012AC\t0003\neth0\t000012AC\t00000000\t0001\n")
	noHosts := write(t, "hosts", "127.0.0.1 localhost\n")
	if gw := HostGateway(noHosts, bridgeDev, route); gw != "172.18.0.1" {
		t.Fatalf("bridge network: got %q", gw)
	}
	if gw := HostGateway(write(t, "hosts", "172.17.0.1 host.docker.internal\n"), bridgeDev, route); gw != "172.17.0.1" {
		t.Fatalf("extra_hosts should win: got %q", gw)
	}
	if gw := HostGateway(noHosts, hostDev, route); gw != "" {
		t.Fatalf("host networking: got %q", gw)
	}
}
