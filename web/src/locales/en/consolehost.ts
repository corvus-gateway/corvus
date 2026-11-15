import type { ConsoleHostKey } from "../consolehost.i18n";

export const consoleHostStrings: Record<ConsoleHostKey, string> = {
  consoleHostTitle: "You are using the console through this host",
  consoleHostEdit: "A wrong change can lock you out of the console at this address. If that happens, open port 9443 of the server directly (https://server-ip:9443).",
  consoleHostDisableTitle: "Disable the host the console uses?",
  consoleHostDisable: "Once this host is disabled, the console is no longer reachable at this address. Make sure you can open port 9443 of the server directly (https://server-ip:9443) first.",
  consoleHostDelete: "You are using the console through this host. After deleting it, the console is no longer reachable at this address. Make sure you can open port 9443 of the server directly (https://server-ip:9443) first.",
  consoleHostBadge: "Console",
  disable: "Disable",
};
