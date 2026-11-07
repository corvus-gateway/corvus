package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

// GatewayKeyHeader carries the per-start key with which Corvus's own gateway marks the
// requests it proxies to the console (CORVUS_CONSOLE_KEY in the gateway's environment).
const GatewayKeyHeader = "X-Corvus-Gateway"

// ConsoleProxies decides which peers may tell the console the visitor's address in
// X-Forwarded-For:
//   - Corvus's own gateway: it connects from loopback or one of the container's own
//     addresses, always overwrites that header, and adds the gateway key. The key is
//     required because a TCP stream to the console also arrives from those addresses
//     but passes the client's headers through unchanged.
//   - the networks of CORVUS_ADMIN_TRUSTED_PROXIES.
//
// Requests from any other peer keep their connection address, so the header cannot be
// spoofed.
type ConsoleProxies struct {
	Nets  []*net.IPNet
	Key   string // gateway key ("" = the gateway is not trusted)
	local func(net.IP) bool
}

// NewConsoleProxies trusts the gateway (with key) and the given CIDRs (already normalized).
func NewConsoleProxies(cidrs []string, key string) *ConsoleProxies {
	p := &ConsoleProxies{local: ownAddress(), Key: key}
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			p.Nets = append(p.Nets, n)
		}
	}
	return p
}

// ownAddress reports whether ip belongs to this container (loopback or an interface).
func ownAddress() func(net.IP) bool {
	var own []net.IP
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				own = append(own, n.IP)
			}
		}
	}
	return func(ip net.IP) bool {
		if ip.IsLoopback() {
			return true
		}
		for _, o := range own {
			if o.Equal(ip) {
				return true
			}
		}
		return false
	}
}

// fromGateway reports whether the request comes from Corvus's own gateway.
func (p *ConsoleProxies) fromGateway(r *http.Request, peer net.IP) bool {
	if p.Key == "" || p.local == nil || peer == nil || !p.local(peer) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(r.Header.Get(GatewayKeyHeader)), []byte(p.Key)) == 1
}

func (p *ConsoleProxies) inNets(ip net.IP) bool {
	for _, n := range p.Nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (p *ConsoleProxies) trusted(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if p.local != nil && p.local(ip) {
		return true
	}
	for _, n := range p.Nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// visitor returns the visitor's address for a request from a trusted peer: the right-most
// X-Forwarded-For entry that is not itself a trusted proxy ("" = keep the peer address).
func (p *ConsoleProxies) visitor(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	peer := net.ParseIP(host)
	if peer == nil || !(p.fromGateway(r, peer) || p.inNets(peer)) {
		return ""
	}
	var hops []string
	for _, h := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(h, ",")...)
	}
	found := ""
	for i := len(hops) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(hops[i]))
		if ip == nil {
			return found // malformed entry: stop at the last good one
		}
		found = ip.String()
		if !p.trusted(ip) {
			break
		}
	}
	return found
}

// realIP sets r.RemoteAddr to the visitor's address for requests forwarded by a trusted
// proxy, so rate limits, the admin allowlist, sessions and the audit log see the visitor.
func (s *Server) realIP(next http.Handler) http.Handler {
	if s.Proxies == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := s.Proxies.visitor(r)
		if ip != "" || r.Header.Get(GatewayKeyHeader) != "" {
			r = r.Clone(r.Context())
			r.Header.Del(GatewayKeyHeader) // never visible to handlers or logs
			if ip != "" {
				r.RemoteAddr = net.JoinHostPort(ip, "0")
			}
		}
		next.ServeHTTP(w, r)
	})
}
