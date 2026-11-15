import type { ProxyProtocolKey } from "../proxyprotocol.i18n";

export const proxyProtocolStrings: Record<ProxyProtocolKey, string> = {
  ppTitle: "Accept PROXY protocol",
  ppDesc: "Turn on when a TCP tunnel or load balancer (FRP, HAProxy) passes the client IP in a PROXY protocol (v1/v2) header. Ports 80/443 stay as they are; separate ports accept the header.",
  ppHttpPort: "HTTP port",
  ppHttpsPort: "HTTPS port",
  ppPortInvalid: "Enter a port between 1 and 65535.",
  ppTrusted: "Trusted PROXY protocol sources",
  ppTrustedHint: "Addresses of the tunnels or load balancers that send the header. Only connections from these use the client IP in the header. Separate from the trusted proxies above: don't add the tunnel there.",
  ppHint: "Point the tunnel at these ports and publish them in docker-compose. With the tunnel on the same host, publish them on loopback only (e.g. '127.0.0.1:8443:8443'); otherwise allow only the tunnel's address with a firewall.",
};
