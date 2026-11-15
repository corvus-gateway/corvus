import type { ProxyProtocolKey } from "../proxyprotocol.i18n";

export const proxyProtocolStrings: Record<ProxyProtocolKey, string> = {
  ppTitle: "PROXY protocol annehmen",
  ppDesc: "Einschalten, wenn ein TCP-Tunnel oder Load Balancer (FRP, HAProxy) die Client-IP in einem PROXY-protocol-Header (v1/v2) übergibt. Die Ports 80/443 bleiben unverändert; eigene Ports nehmen den Header an.",
  ppHttpPort: "HTTP-Port",
  ppHttpsPort: "HTTPS-Port",
  ppPortInvalid: "Gib einen Port zwischen 1 und 65535 ein.",
  ppTrusted: "Vertrauenswürdige PROXY-protocol-Quellen",
  ppTrustedHint: "Adressen der Tunnel oder Load Balancer, die den Header senden. Nur Verbindungen von dort verwenden die Client-IP aus dem Header. Getrennt von den vertrauenswürdigen Proxys oben: Trage den Tunnel dort nicht ein.",
  ppHint: "Richte den Tunnel auf diese Ports aus und veröffentliche sie in docker-compose. Läuft der Tunnel auf demselben Host, veröffentliche sie nur auf Loopback (z. B. '127.0.0.1:8443:8443'); sonst lass per Firewall nur die Adresse des Tunnels zu.",
};
