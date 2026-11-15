import type { ConsoleHostKey } from "../consolehost.i18n";

export const consoleHostStrings: Record<ConsoleHostKey, string> = {
  consoleHostTitle: "您正在通过此主机访问控制台",
  consoleHostEdit: "错误的更改可能导致无法通过此地址访问控制台。如果发生这种情况，请直接访问服务器的 9443 端口（https://服务器IP:9443）。",
  consoleHostDisableTitle: "要停用控制台正在使用的主机吗？",
  consoleHostDisable: "停用此主机后，将无法通过当前地址访问控制台。请先确认可以直接访问服务器的 9443 端口（https://服务器IP:9443）。",
  consoleHostDelete: "您正在通过此主机访问控制台。删除后将无法通过当前地址访问控制台。请先确认可以直接访问服务器的 9443 端口（https://服务器IP:9443）。",
  consoleHostBadge: "控制台",
  disable: "停用",
};
