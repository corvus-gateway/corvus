package compiler

import (
	"bufio"
	"net"
	"os"
	"strings"
)

// StaticHosts reads a hosts file (normally /etc/hosts) into name → IP.
//
// The gateway resolves forward hosts at request time through a DNS resolver, which
// doesn't see entries that only exist in /etc/hosts — such as Docker's extra_hosts
// (host.docker.internal:host-gateway). Such names are replaced by their address when
// the configuration is rendered. IPv4 addresses win over IPv6; the first entry wins.
func StaticHosts(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	v4, v6 := map[string]string{}, map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line, _, _ := strings.Cut(sc.Text(), "#")
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ip := net.ParseIP(fields[0])
		if ip == nil {
			continue
		}
		dst := v6
		if ip.To4() != nil {
			dst = v4
		}
		for _, name := range fields[1:] {
			name = strings.ToLower(name)
			if _, ok := dst[name]; !ok {
				dst[name] = ip.String()
			}
		}
	}
	for name, ip := range v6 {
		if _, ok := v4[name]; !ok {
			v4[name] = ip
		}
	}
	return v4
}

// upstreamAddr returns the address nginx should use for a forward host. Loopback
// (127.0.0.1, localhost, ::1) means the Docker host and becomes hostGW when it is set;
// a name from hosts becomes its address; anything else (container names, IPs, DNS names)
// is used as is. IPv6 is bracketed.
func upstreamAddr(host string, hosts map[string]string, hostGW string) string {
	if hostGW != "" && isLoopback(host) {
		return hostGW
	}
	if ip, ok := hosts[strings.ToLower(host)]; ok {
		host = ip
		if hostGW != "" && isLoopback(host) {
			return hostGW
		}
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return "[" + host + "]"
	}
	return host
}

// isLoopback reports whether a forward host means "this machine".
func isLoopback(host string) bool {
	h := strings.Trim(strings.ToLower(host), "[]")
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// HostGateway returns the address of the Docker host as seen from this container, or ""
// when the container shares the host's network (network_mode: host), where 127.0.0.1
// already is the host. It prefers host.docker.internal from hostsFile (extra_hosts:
// host-gateway) and otherwise uses the default route's gateway, which on a Docker bridge
// network is the host's address on that bridge.
func HostGateway(hostsFile, netDev, route string) string {
	if b, err := os.ReadFile(netDev); err == nil && strings.Contains(string(b), "docker0:") {
		return "" // the host's own interfaces are visible: host networking
	}
	if ip, ok := StaticHosts(hostsFile)["host.docker.internal"]; ok {
		return ip
	}
	f, err := os.Open(route)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		// Iface Destination Gateway ... (hex, little-endian)
		fs := strings.Fields(sc.Text())
		if len(fs) < 3 || fs[1] != "00000000" || len(fs[2]) != 8 {
			continue
		}
		var b [4]byte
		for i := 0; i < 4; i++ {
			var v byte
			for _, c := range fs[2][i*2 : i*2+2] {
				v <<= 4
				switch {
				case c >= '0' && c <= '9':
					v |= byte(c - '0')
				case c >= 'A' && c <= 'F':
					v |= byte(c-'A') + 10
				case c >= 'a' && c <= 'f':
					v |= byte(c-'a') + 10
				}
			}
			b[3-i] = v
		}
		if gw := net.IPv4(b[0], b[1], b[2], b[3]); !gw.IsUnspecified() {
			return gw.String()
		}
	}
	return ""
}
