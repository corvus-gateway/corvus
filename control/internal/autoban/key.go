package autoban

import (
	"fmt"
	"net"
	"strings"
	"time"

	"corvus/control/internal/traffic"
)

// Automatic bans cover an IPv4 address or a whole IPv6 /64: one IPv6 host can use any
// address of its /64 (privacy addresses), so banning single addresses would not stop it.

// BanKey returns what an automatic ban of ip covers: the IPv4 address itself, or the
// IPv6 /64 written as a CIDR ("2001:db8:1:2::/64"). "" for an invalid address.
func BanKey(ip string) string {
	p := net.ParseIP(ip)
	if p == nil {
		return ""
	}
	if v4 := p.To4(); v4 != nil {
		return v4.String()
	}
	n := net.IPNet{IP: p.Mask(net.CIDRMask(64, 128)), Mask: net.CIDRMask(64, 128)}
	return n.String()
}

// GatewayKey returns the gateway's ban-table key for a stored ban: the address as is,
// or for an IPv6 /64 its first four groups in full ("2001:0db8:0001:0002"), the form the
// gateway derives from the client address.
func GatewayKey(stored string) string {
	ip, n, err := net.ParseCIDR(stored)
	if err != nil {
		return stored
	}
	if ones, bits := n.Mask.Size(); ones != 64 || bits != 128 {
		return stored
	}
	b := ip.Mask(n.Mask).To16()
	return fmt.Sprintf("%02x%02x:%02x%02x:%02x%02x:%02x%02x", b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7])
}

// parseHitKey reads the client of a gateway bad-behaviour record: an address, or an
// IPv6 /64 in the gateway's form. Returns the ban key and an address inside it.
func parseHitKey(s string) (string, net.IP) {
	if ip := net.ParseIP(s); ip != nil {
		return BanKey(s), ip
	}
	if strings.Count(s, ":") == 3 && !strings.Contains(s, "::") {
		if ip := net.ParseIP(s + "::"); ip != nil {
			return BanKey(ip.String()), ip
		}
	}
	return "", nil
}

// keyNet returns the network a ban key covers.
func keyNet(key string) *net.IPNet {
	if _, n, err := net.ParseCIDR(key); err == nil {
		return n
	}
	if ip := net.ParseIP(key); ip != nil {
		bits := 128
		if ip.To4() != nil {
			ip, bits = ip.To4(), 32
		}
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
	}
	return nil
}

// exemptKey: the ban would cover a loopback address or overlap an exempt network.
func exemptKey(key string, nets []*net.IPNet) bool {
	k := keyNet(key)
	if k == nil || k.IP.IsLoopback() {
		return true
	}
	for _, n := range nets {
		if n.Contains(k.IP) || k.Contains(n.IP) {
			return true
		}
	}
	return false
}

// eventsByKey sums detections per ban key since t.
func eventsByKey(c *traffic.Collector, since time.Time, kinds ...string) map[string]*traffic.SourceActivity {
	out := map[string]*traffic.SourceActivity{}
	for ip, a := range c.EventsByIP(since, kinds...) {
		k := BanKey(ip)
		if k == "" {
			continue
		}
		o := out[k]
		if o == nil {
			o = &traffic.SourceActivity{Hosts: map[string]bool{}, Kinds: map[string]bool{}}
			out[k] = o
		}
		o.Count += a.Count
		for h := range a.Hosts {
			o.Hosts[h] = true
		}
		for x := range a.Kinds {
			o.Kinds[x] = true
		}
	}
	return out
}

// countsByKey sums requests and 4xx responses per ban key over the last d.
func countsByKey(c *traffic.Collector, d time.Duration, now time.Time) map[string]traffic.IPActivity {
	out := map[string]traffic.IPActivity{}
	for ip, a := range c.IPCounts(d, now) {
		k := BanKey(ip)
		if k == "" {
			continue
		}
		o := out[k]
		o.Requests += a.Requests
		o.S4xx += a.S4xx
		o.Host = a.Host
		out[k] = o
	}
	return out
}
