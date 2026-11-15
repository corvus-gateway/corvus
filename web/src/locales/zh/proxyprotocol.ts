import type { ProxyProtocolKey } from "../proxyprotocol.i18n";

export const proxyProtocolStrings: Record<ProxyProtocolKey, string> = {
  ppTitle: "接受 PROXY protocol",
  ppDesc: "当 TCP 隧道或负载均衡器（FRP、HAProxy）通过 PROXY protocol（v1/v2）头传递客户端 IP 时开启。80/443 端口保持不变，另开端口接收该头。",
  ppHttpPort: "HTTP 端口",
  ppHttpsPort: "HTTPS 端口",
  ppPortInvalid: "请输入 1 到 65535 之间的端口。",
  ppTrusted: "受信任的 PROXY protocol 来源",
  ppTrustedHint: "发送该头的隧道或负载均衡器的地址。只有来自这些地址的连接才会使用头中的客户端 IP。它与上方的受信任代理相互独立：不要把隧道地址加到那里。",
  ppHint: "将隧道指向这些端口，并在 docker-compose 中发布它们。隧道在同一主机上时，只在回环地址上发布（例如 '127.0.0.1:8443:8443'）；否则请用防火墙只允许隧道的地址。",
};
