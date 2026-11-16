import type { ConnCheckKey } from "../conncheck.i18n";

export const connCheckStrings: Record<ConnCheckKey, string> = {
  ccButton: "Check connection",
  ccChecking: "Checking…",
  ccOk: "The gateway reached {addr} in {ms} ms",
  ccMapped: "The gateway connects to {target} for this host.",
  ccDnsTitle: "Name does not resolve",
  ccDnsHint: "The gateway looks forward hosts up as full DNS names, without search domains. Enter the full name (such as nas.home.lan) or the IP address, or add the name to extra_hosts in docker-compose.yml.",
  ccRefusedTitle: "Connection refused",
  ccRefusedHint: "The address answers, but nothing accepts connections on this port. Check the port, that the service is running, and that it listens on 0.0.0.0 rather than only 127.0.0.1. A firewall may also be rejecting the connection.",
  ccTimeoutTitle: "No answer (timed out)",
  ccTimeoutHint: "Nothing answered within 5 seconds. Usually a firewall on the target server drops connections from the gateway server: allow the gateway server there. The address may also be wrong, or the server switched off.",
  ccUnreachableTitle: "Network unreachable",
  ccUnreachableHint: "There is no route from the gateway to this address. Check the IP address. If it lies in a Docker network range (such as 172.17–172.31.x.x), a Docker subnet overlaps your LAN: change the subnet of the Docker network.",
  ccTlsTitle: "TLS handshake failed",
  ccTlsHint: "The port accepts connections but does not speak HTTPS. Switch the scheme to http, or enter the service's HTTPS port.",
  ccOtherTitle: "Connection failed",
  ccOtherHint: "See the message below and the gateway log.",
  ueBadge: "Upstream error",
  ueTip: "Upstream failed {ago}: {reason}",
  ueTipAction: "Edit the host and use Check connection to find the cause.",
  ueSheetTitle: "Visitors got an error page {ago}",
};
