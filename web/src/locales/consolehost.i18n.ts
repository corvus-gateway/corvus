// Warnings when changing the proxy host the console itself is opened through — 8 languages.
export const koConsoleHost = {
  consoleHostTitle: "지금 이 호스트를 통해 콘솔에 접속 중입니다",
  consoleHostEdit: "설정을 잘못 바꾸면 이 주소로는 콘솔에 들어올 수 없게 됩니다. 그럴 때는 서버의 9443 포트(https://서버-IP:9443)로 직접 접속하세요.",
  consoleHostDisableTitle: "콘솔이 쓰는 호스트를 끌까요?",
  consoleHostDisable: "이 호스트를 끄면 지금 주소로는 콘솔에 접속할 수 없습니다. 서버의 9443 포트(https://서버-IP:9443)로 직접 접속할 수 있는지 먼저 확인하세요.",
  consoleHostDelete: "지금 이 호스트를 통해 콘솔에 접속 중입니다. 삭제하면 이 주소로는 콘솔에 접속할 수 없습니다. 서버의 9443 포트(https://서버-IP:9443)로 직접 접속할 수 있는지 먼저 확인하세요.",
  consoleHostBadge: "콘솔 접속 중",
  disable: "끄기",
};

type ConsoleHostKey = keyof typeof koConsoleHost;

export const consoleHostStrings: Record<"en" | "zh" | "ja" | "ru" | "fr" | "de" | "es", Record<ConsoleHostKey, string>> = {
  en: {
    consoleHostTitle: "You are using the console through this host",
    consoleHostEdit: "A wrong change can lock you out of the console at this address. If that happens, open port 9443 of the server directly (https://server-ip:9443).",
    consoleHostDisableTitle: "Disable the host the console uses?",
    consoleHostDisable: "Once this host is disabled, the console is no longer reachable at this address. Make sure you can open port 9443 of the server directly (https://server-ip:9443) first.",
    consoleHostDelete: "You are using the console through this host. After deleting it, the console is no longer reachable at this address. Make sure you can open port 9443 of the server directly (https://server-ip:9443) first.",
    consoleHostBadge: "Console",
    disable: "Disable",
  },
  zh: {
    consoleHostTitle: "您正在通过此主机访问控制台",
    consoleHostEdit: "错误的更改可能导致无法通过此地址访问控制台。如果发生这种情况，请直接访问服务器的 9443 端口（https://服务器IP:9443）。",
    consoleHostDisableTitle: "要停用控制台正在使用的主机吗？",
    consoleHostDisable: "停用此主机后，将无法通过当前地址访问控制台。请先确认可以直接访问服务器的 9443 端口（https://服务器IP:9443）。",
    consoleHostDelete: "您正在通过此主机访问控制台。删除后将无法通过当前地址访问控制台。请先确认可以直接访问服务器的 9443 端口（https://服务器IP:9443）。",
    consoleHostBadge: "控制台",
    disable: "停用",
  },
  ja: {
    consoleHostTitle: "このホスト経由でコンソールに接続しています",
    consoleHostEdit: "設定を誤ると、このアドレスからコンソールに入れなくなります。その場合はサーバーの 9443 番ポート（https://サーバーIP:9443）に直接接続してください。",
    consoleHostDisableTitle: "コンソールが使っているホストを無効にしますか？",
    consoleHostDisable: "このホストを無効にすると、現在のアドレスではコンソールに接続できなくなります。先にサーバーの 9443 番ポート（https://サーバーIP:9443）へ直接接続できることを確認してください。",
    consoleHostDelete: "このホスト経由でコンソールに接続しています。削除すると、現在のアドレスではコンソールに接続できなくなります。先にサーバーの 9443 番ポート（https://サーバーIP:9443）へ直接接続できることを確認してください。",
    consoleHostBadge: "コンソール",
    disable: "無効にする",
  },
  ru: {
    consoleHostTitle: "Вы работаете с консолью через этот хост",
    consoleHostEdit: "Неверное изменение может закрыть доступ к консоли по этому адресу. В таком случае откройте порт 9443 сервера напрямую (https://IP-сервера:9443).",
    consoleHostDisableTitle: "Отключить хост, через который открыта консоль?",
    consoleHostDisable: "После отключения консоль будет недоступна по текущему адресу. Сначала убедитесь, что порт 9443 сервера открывается напрямую (https://IP-сервера:9443).",
    consoleHostDelete: "Вы работаете с консолью через этот хост. После удаления консоль будет недоступна по текущему адресу. Сначала убедитесь, что порт 9443 сервера открывается напрямую (https://IP-сервера:9443).",
    consoleHostBadge: "Консоль",
    disable: "Отключить",
  },
  fr: {
    consoleHostTitle: "Vous utilisez la console via cet hôte",
    consoleHostEdit: "Une mauvaise modification peut vous empêcher d'accéder à la console à cette adresse. Dans ce cas, ouvrez directement le port 9443 du serveur (https://ip-du-serveur:9443).",
    consoleHostDisableTitle: "Désactiver l'hôte utilisé par la console ?",
    consoleHostDisable: "Une fois cet hôte désactivé, la console n'est plus accessible à cette adresse. Vérifiez d'abord que le port 9443 du serveur s'ouvre directement (https://ip-du-serveur:9443).",
    consoleHostDelete: "Vous utilisez la console via cet hôte. Après sa suppression, la console n'est plus accessible à cette adresse. Vérifiez d'abord que le port 9443 du serveur s'ouvre directement (https://ip-du-serveur:9443).",
    consoleHostBadge: "Console",
    disable: "Désactiver",
  },
  de: {
    consoleHostTitle: "Sie nutzen die Konsole über diesen Host",
    consoleHostEdit: "Eine falsche Änderung kann Sie unter dieser Adresse aus der Konsole aussperren. Öffnen Sie dann Port 9443 des Servers direkt (https://server-ip:9443).",
    consoleHostDisableTitle: "Host deaktivieren, den die Konsole nutzt?",
    consoleHostDisable: "Nach dem Deaktivieren ist die Konsole unter dieser Adresse nicht mehr erreichbar. Prüfen Sie zuerst, ob Port 9443 des Servers direkt erreichbar ist (https://server-ip:9443).",
    consoleHostDelete: "Sie nutzen die Konsole über diesen Host. Nach dem Löschen ist die Konsole unter dieser Adresse nicht mehr erreichbar. Prüfen Sie zuerst, ob Port 9443 des Servers direkt erreichbar ist (https://server-ip:9443).",
    consoleHostBadge: "Konsole",
    disable: "Deaktivieren",
  },
  es: {
    consoleHostTitle: "Está usando la consola a través de este host",
    consoleHostEdit: "Un cambio incorrecto puede dejarle sin acceso a la consola en esta dirección. Si ocurre, abra directamente el puerto 9443 del servidor (https://ip-del-servidor:9443).",
    consoleHostDisableTitle: "¿Deshabilitar el host que usa la consola?",
    consoleHostDisable: "Al deshabilitar este host, la consola deja de estar disponible en esta dirección. Compruebe antes que puede abrir directamente el puerto 9443 del servidor (https://ip-del-servidor:9443).",
    consoleHostDelete: "Está usando la consola a través de este host. Si lo elimina, la consola deja de estar disponible en esta dirección. Compruebe antes que puede abrir directamente el puerto 9443 del servidor (https://ip-del-servidor:9443).",
    consoleHostBadge: "Consola",
    disable: "Deshabilitar",
  },
};

/** True when the browser reached the console through one of these host names (wildcards included). */
export function servesConsole(domains: string[], host = typeof location === "undefined" ? "" : location.hostname): boolean {
  const h = host.toLowerCase();
  return domains.some((d) => {
    const v = d.toLowerCase();
    return v === h || (v.startsWith("*.") && h.endsWith(v.slice(1)) && h.length > v.length - 1);
  });
}
