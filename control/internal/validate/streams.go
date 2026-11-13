package validate

import (
	"net"
	"strings"

	"corvus/control/internal/store"
)

// Stream normalizes and validates a TCP/UDP stream in place. Ports used by Corvus itself
// are checked by the caller (they depend on the runtime configuration).
func Stream(st *store.Stream) error {
	var errs Errors
	notes(&errs, &st.Notes)
	if st.IncomingPort < 1 || st.IncomingPort > 65535 {
		errs.add("incoming_port", "invalid", "port must be 1-65535")
	}
	if !st.TCP && !st.UDP {
		errs.add("tcp_forwarding", "required", "choose TCP, UDP or both")
	}
	st.ForwardHost = strings.ToLower(strings.TrimSpace(st.ForwardHost))
	if ip := net.ParseIP(st.ForwardHost); ip != nil {
		if ip.To4() == nil {
			errs.add("forward_host", "ipv6_unsupported", "IPv6 upstreams are not supported yet")
		}
	} else if !upstreamRe.MatchString(st.ForwardHost) {
		errs.add("forward_host", "invalid", "invalid forward host %q", st.ForwardHost)
	}
	if st.ForwardPort < 1 || st.ForwardPort > 65535 {
		errs.add("forward_port", "invalid", "port must be 1-65535")
	}
	if st.CertificateID != nil && !st.TCP {
		errs.add("certificate_id", "needs_tcp", "TLS termination requires TCP")
	}
	// nginx accepts and sends the PROXY protocol only on TCP connections; with TCP and
	// UDP both on, it applies to the TCP listener.
	if st.ProxyProtocolIn && !st.TCP {
		errs.add("proxy_protocol_in", "needs_tcp", "the PROXY protocol requires TCP")
	}
	if st.ProxyProtocolOut && !st.TCP {
		errs.add("proxy_protocol_out", "needs_tcp", "the PROXY protocol requires TCP")
	}
	return errs.OrNil()
}
