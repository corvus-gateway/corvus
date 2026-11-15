import type { ProxyProtocolKey } from "../proxyprotocol.i18n";

export const proxyProtocolStrings: Record<ProxyProtocolKey, string> = {
  ppTitle: "Принимать PROXY protocol",
  ppDesc: "Включите, если TCP-туннель или балансировщик (FRP, HAProxy) передаёт IP клиента в заголовке PROXY protocol (v1/v2). Порты 80/443 не меняются: заголовок принимают отдельные порты.",
  ppHttpPort: "Порт HTTP",
  ppHttpsPort: "Порт HTTPS",
  ppPortInvalid: "Укажите порт от 1 до 65535.",
  ppTrusted: "Доверенные источники PROXY protocol",
  ppTrustedHint: "Адреса туннелей или балансировщиков, которые отправляют заголовок. IP клиента из заголовка используется только для подключений с этих адресов. Это отдельный список от доверенных прокси выше: не добавляйте туда туннель.",
  ppHint: "Направьте туннель на эти порты и опубликуйте их в docker-compose. Если туннель на том же хосте, публикуйте их только на loopback (например, '127.0.0.1:8443:8443'); иначе разрешите в файрволе только адрес туннеля.",
};
