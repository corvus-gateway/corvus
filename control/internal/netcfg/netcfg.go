// Package netcfg handles client-IP trust: Cloudflare edge ranges and operator
// defined trusted proxies used by the gateway's real-IP processing.
package netcfg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"corvus/control/internal/store"
)

// Builtin Cloudflare ranges (https://www.cloudflare.com/ips/), used until a refresh succeeds.
var (
	builtinV4 = []string{"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22", "141.101.64.0/18",
		"108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20", "197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15",
		"104.16.0.0/13", "104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22"}
	builtinV6 = []string{"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32", "2405:8100::/32",
		"2a06:98c0::/29", "2c0f:f248::/32"}
)

// Cloudflare returns the stored snapshot or the builtin list.
func Cloudflare(ctx context.Context, s *store.Store) store.CloudflareIPs {
	if c, err := s.CloudflareIPs(ctx); err == nil && c != nil && len(c.IPv4) > 0 {
		return *c
	}
	return store.CloudflareIPs{IPv4: builtinV4, IPv6: builtinV6, Source: "builtin"}
}

// FetchCloudflare downloads the current published ranges.
func FetchCloudflare(ctx context.Context) (store.CloudflareIPs, error) {
	get := func(url string) ([]string, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("User-Agent", "corvus")
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
		}
		var out []string
		sc := bufio.NewScanner(io.LimitReader(resp.Body, 64<<10))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			if _, _, err := net.ParseCIDR(line); err != nil {
				return nil, fmt.Errorf("unexpected entry %q", line)
			}
			out = append(out, line)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("%s: empty list", url)
		}
		return out, sc.Err()
	}
	v4, err := get("https://www.cloudflare.com/ips-v4")
	if err != nil {
		return store.CloudflareIPs{}, err
	}
	v6, err := get("https://www.cloudflare.com/ips-v6")
	if err != nil {
		return store.CloudflareIPs{}, err
	}
	return store.CloudflareIPs{IPv4: v4, IPv6: v6, UpdatedAt: time.Now().UTC(), Source: "remote"}, nil
}

// NormalizeTrustedCIDRs is NormalizeCIDRs but refuses very broad networks
// (IPv4 shorter than /8, IPv6 shorter than /16): trusting "everyone" as a proxy or
// exempting everyone from a rate limit is almost always a mistake.
func NormalizeTrustedCIDRs(in []string) ([]string, error) {
	out, err := NormalizeCIDRs(in)
	if err != nil {
		return nil, err
	}
	for _, c := range out {
		_, n, _ := net.ParseCIDR(c)
		ones, bits := n.Mask.Size()
		if (bits == 32 && ones < 8) || (bits == 128 && ones < 16) {
			return nil, fmt.Errorf("%s is too broad (minimum /8 for IPv4, /16 for IPv6)", c)
		}
	}
	return out, nil
}

// NormalizeCIDRs validates IPs/CIDRs (a bare IP becomes /32 or /128).
func NormalizeCIDRs(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if !strings.Contains(v, "/") {
			ip := net.ParseIP(v)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP %q", v)
			}
			if ip4 := ip.To4(); ip4 != nil {
				v = ip4.String() + "/32" // also ::ffff:a.b.c.d
			} else {
				v += "/128"
			}
		}
		_, n, err := net.ParseCIDR(v)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q", v)
		}
		c, ok := cidrString(n)
		if !ok {
			continue
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out, nil
}

// cidrString formats a parsed network. Go prints an IPv4-mapped IPv6 network
// (::ffff:a.b.c.d/p) in IPv4 form with the 128-bit mask, so ::ffff:0:0/96 would become
// 0.0.0.0/0 (all of IPv4). A mapped network of IPv4 /8 or narrower becomes that IPv4
// network; broader ones (special-use entries of IPv6 lists, which never match a client
// address the gateway sees) are left out (ok = false).
func cidrString(n *net.IPNet) (string, bool) {
	ones, bits := n.Mask.Size()
	if bits == 128 && n.IP.To4() != nil {
		if ones < 104 {
			return "", false
		}
		return (&net.IPNet{IP: n.IP.To4(), Mask: net.CIDRMask(ones-96, 32)}).String(), true
	}
	return n.String(), true
}
