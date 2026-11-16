// Package conncheck tries a forward target the way the gateway reaches it: the name is
// resolved through the gateway's DNS resolver as a fully-qualified name (nginx applies
// no resolv.conf search domains), then a TCP connection is opened and, for https, a TLS
// handshake is made with the forward host as SNI and no certificate verification (the
// gateway does not verify backend certificates either).
package conncheck

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Failure kinds.
const (
	KindDNS         = "dns"         // the name does not resolve (or the resolver does not answer)
	KindRefused     = "refused"     // nothing listens on the port (or a firewall rejects)
	KindTimeout     = "timeout"     // no answer: firewall dropping, wrong address, service down
	KindUnreachable = "unreachable" // no route to the address (wrong network, overlapping subnet)
	KindTLS         = "tls"         // TCP works, the TLS handshake does not (plain HTTP port?)
	KindOther       = "other"
)

// DefaultTimeout bounds a whole check (DNS + connect + TLS).
const DefaultTimeout = 5 * time.Second

// Result of a check.
type Result struct {
	OK      bool   `json:"ok"`
	Kind    string `json:"kind,omitempty"`    // failure kind (see the Kind constants)
	Message string `json:"message,omitempty"` // the underlying error
	Target  string `json:"target"`            // forward host as the gateway uses it (after mapping)
	Address string `json:"address,omitempty"` // address connected to (or tried)
	TimeMS  int64  `json:"time_ms"`
}

// Checker checks forward targets. The zero value resolves with the system resolver;
// set Resolver to the gateway's.
type Checker struct {
	Resolver string // nginx "resolver" value, e.g. "127.0.0.11" or "[fd00::1]:53 1.1.1.1"
	IPv6     bool   // the resolver may return IPv6 addresses (resolver ... ipv6=on)
	Timeout  time.Duration

	// Lookup and Dial replace DNS and TCP (tests); nil = real ones.
	Lookup func(ctx context.Context, network, host string) ([]net.IP, error)
	Dial   func(ctx context.Context, network, addr string) (net.Conn, error)
}

// ResolverAddr turns an nginx resolver value into a host:port for DNS queries: the first
// address, port 53 unless given. "" when there is none.
func ResolverAddr(v string) string {
	for _, f := range strings.Fields(v) {
		if strings.Contains(f, "=") { // valid=30s, ipv6=off, ...
			continue
		}
		if ip := net.ParseIP(strings.Trim(f, "[]")); ip != nil {
			return net.JoinHostPort(ip.String(), "53")
		}
		if h, p, err := net.SplitHostPort(f); err == nil {
			return net.JoinHostPort(strings.Trim(h, "[]"), p)
		}
		return net.JoinHostPort(f, "53")
	}
	return ""
}

func (c *Checker) lookup(ctx context.Context, host string) ([]net.IP, error) {
	network := "ip4"
	if c.IPv6 {
		network = "ip"
	}
	// A trailing dot makes the name fully qualified: no search domains, like nginx.
	fqdn := strings.TrimSuffix(host, ".") + "."
	if c.Lookup != nil {
		return c.Lookup(ctx, network, fqdn)
	}
	r := &net.Resolver{PreferGo: true, StrictErrors: true}
	if addr := ResolverAddr(c.Resolver); addr != "" {
		r.Dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		}
	}
	return r.LookupIP(ctx, network, fqdn)
}

func (c *Checker) dial(ctx context.Context, addr string) (net.Conn, error) {
	if c.Dial != nil {
		return c.Dial(ctx, "tcp", addr)
	}
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr)
}

// Check tries scheme://host:port. host is the forward host as the gateway uses it
// (compiler mapping applied: an IP address, possibly bracketed, or a DNS name).
func (c *Checker) Check(ctx context.Context, scheme, host string, port int) Result {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	name := strings.Trim(host, "[]")
	res := Result{Target: name}
	done := func(kind string, err error) Result {
		res.TimeMS = time.Since(start).Milliseconds()
		if err != nil {
			res.Kind, res.Message = kind, err.Error()
		} else {
			res.OK = true
		}
		return res
	}

	ip := net.ParseIP(name)
	if ip == nil {
		ips, err := c.lookup(ctx, name)
		if err != nil {
			return done(KindDNS, dnsError(name, err))
		}
		if len(ips) == 0 {
			return done(KindDNS, errors.New(name+": no address"))
		}
		ip = ips[0]
	}
	res.Address = net.JoinHostPort(ip.String(), strconv.Itoa(port))
	conn, err := c.dial(ctx, res.Address)
	if err != nil {
		return done(dialKind(ctx, err), err)
	}
	defer conn.Close()
	if scheme == "https" {
		cfg := &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- like the gateway: backend certificates are not verified
		if net.ParseIP(name) == nil {
			cfg.ServerName = name // SNI: the forward host (an IP address sends none)
		}
		tc := tls.Client(conn, cfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			if ctx.Err() != nil {
				return done(KindTimeout, errors.New("TLS handshake timed out"))
			}
			return done(KindTLS, err)
		}
	}
	return done("", nil)
}

// dnsError shortens a resolver error to what matters.
func dnsError(name string, err error) error {
	var de *net.DNSError
	if errors.As(err, &de) {
		switch {
		case de.IsNotFound:
			return errors.New(name + ": no such name (NXDOMAIN or no address)")
		case de.IsTimeout:
			return errors.New(name + ": the DNS resolver did not answer")
		}
	}
	return err
}

// dialKind classifies a connection error.
func dialKind(ctx context.Context, err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return KindRefused
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return KindUnreachable
	case ctx.Err() != nil, errors.Is(err, os.ErrDeadlineExceeded), errors.Is(err, context.DeadlineExceeded),
		errors.As(err, &ne) && ne.Timeout():
		return KindTimeout
	}
	return KindOther
}
