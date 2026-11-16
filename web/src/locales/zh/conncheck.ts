import type { ConnCheckKey } from "../conncheck.i18n";

export const connCheckStrings: Record<ConnCheckKey, string> = {
  ccButton: "检查连接",
  ccChecking: "正在检查…",
  ccOk: "网关已连接到 {addr}（{ms} ms）",
  ccMapped: "对于此主机，网关会连接到 {target}。",
  ccDnsTitle: "无法解析名称",
  ccDnsHint: "网关以完整 DNS 名称查询转发主机，不使用搜索域。请输入完整名称（如 nas.home.lan）或 IP 地址，或在 docker-compose.yml 的 extra_hosts 中添加该名称。",
  ccRefusedTitle: "连接被拒绝",
  ccRefusedHint: "地址可以访问，但此端口上没有服务接受连接。请检查端口、服务是否在运行，以及服务是否监听 0.0.0.0 而不只是 127.0.0.1。也可能是防火墙拒绝了连接。",
  ccTimeoutTitle: "无响应（超时）",
  ccTimeoutHint: "5 秒内没有任何响应。通常是目标服务器的防火墙丢弃了来自网关服务器的连接，请在该服务器上放行网关服务器。也可能是地址错误或服务器已关机。",
  ccUnreachableTitle: "网络不可达",
  ccUnreachableHint: "网关没有到达此地址的路由。请检查 IP 地址。如果它位于 Docker 网络网段（如 172.17–172.31.x.x），说明 Docker 子网与局域网重叠，请修改 Docker 网络的子网。",
  ccTlsTitle: "TLS 握手失败",
  ccTlsHint: "该端口接受连接，但不使用 HTTPS。请将协议改为 http，或填写服务的 HTTPS 端口。",
  ccOtherTitle: "连接失败",
  ccOtherHint: "请查看下方消息和网关日志。",
  ueBadge: "上游错误",
  ueTip: "连接上游失败（{ago}）：{reason}",
  ueTipAction: "编辑该主机并使用“检查连接”查找原因。",
  ueSheetTitle: "访客看到了错误页面（{ago}）",
};
