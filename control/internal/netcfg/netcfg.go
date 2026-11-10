// Package netcfg handles client-IP trust: Cloudflare edge ranges and operator
// defined trusted proxies used by the gateway's real-IP processing.
package netcfg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
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

// Cloudflare returns the stored snapshot or the builtin list. A stored snapshot
// that does not pass the trusted-proxy checks (for example one restored from an
// older backup holding a very broad range) is ignored in favour of the builtin list.
func Cloudflare(ctx context.Context, s *store.Store) store.CloudflareIPs {
	if c, err := s.CloudflareIPs(ctx); err == nil && c != nil && len(c.IPv4) > 0 {
		v4, err4 := trustedRanges(c.IPv4, false)
		v6, err6 := trustedRanges(c.IPv6, true)
		if err4 == nil && err6 == nil && len(v4) > 0 {
			c.IPv4, c.IPv6 = v4, v6
			return *c
		}
		slog.Warn("ignoring stored Cloudflare ranges that fail validation; using the builtin list")
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
	list := func(url string, v6 bool) ([]string, error) {
		raw, err := get(url)
		if err != nil {
			return nil, err
		}
		out, err := trustedRanges(raw, v6)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", url, err)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("%s: empty list", url)
		}
		return out, nil
	}
	v4, err := list("https://www.cloudflare.com/ips-v4", false)
	if err != nil {
		return store.CloudflareIPs{}, err
	}
	v6, err := list("https://www.cloudflare.com/ips-v6", true)
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

// trustedRanges normalizes a Cloudflare range list with the same rules as operator
// entered trusted proxies and checks that every entry is of the expected family.
func trustedRanges(in []string, v6 bool) ([]string, error) {
	out, err := NormalizeTrustedCIDRs(in)
	if err != nil {
		return nil, err
	}
	for _, c := range out {
		_, n, _ := net.ParseCIDR(c)
		if (n.IP.To4() == nil) != v6 {
			return nil, fmt.Errorf("unexpected entry %q", c)
		}
	}
	return out, nil
}

// IPv6 transition ranges that carry an IPv4 address: the NAT64 well-known prefix
// (64:ff9b::/96, IPv4 in the last 32 bits), 6to4 (2002::/16, IPv4 in bits 16-47),
// local-use NAT64 (64:ff9b:1::/48, operator-chosen layout) and Teredo (2001::/32,
// obfuscated client address).
var (
	nat64WKP   = mustCIDR("64:ff9b::/96")
	nat64Local = mustCIDR("64:ff9b:1::/48")
	sixToFour  = mustCIDR("2002::/16")
	teredo     = mustCIDR("2001::/32")
)

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// EmbeddedIPv4 reports whether ip is an IPv6 transition address that reaches an
// IPv4 destination. v4 is the embedded address when its position is fixed (NAT64
// well-known prefix, 6to4); it is nil for ranges whose IPv4 part cannot be read
// reliably (local-use NAT64, Teredo), which callers should treat as not public.
func EmbeddedIPv4(ip net.IP) (v4 net.IP, transition bool) {
	if ip.To4() != nil {
		return nil, false
	}
	ip16 := ip.To16()
	if ip16 == nil {
		return nil, false
	}
	switch {
	case nat64WKP.Contains(ip16):
		return net.IPv4(ip16[12], ip16[13], ip16[14], ip16[15]), true
	case sixToFour.Contains(ip16):
		return net.IPv4(ip16[2], ip16[3], ip16[4], ip16[5]), true
	case nat64Local.Contains(ip16), teredo.Contains(ip16):
		return nil, true
	}
	return nil, false
}
