// PROXY protocol listeners (Network page) — Korean source strings + translations.
export const koProxyProtocol = {
  ppTitle: "PROXY protocol 받기",
  ppDesc: "FRP·HAProxy 같은 TCP 터널이나 로드밸런서가 PROXY protocol(v1/v2) 헤더로 클라이언트 IP를 넘겨줄 때 켭니다. 80/443 포트는 그대로 두고, 헤더를 받는 포트를 따로 엽니다.",
  ppHttpPort: "HTTP 포트",
  ppHttpsPort: "HTTPS 포트",
  ppHint: "터널이 이 포트로 연결하도록 설정하고 docker-compose에서 포트를 공개하세요(예: '8443:8443'). 터널이 접속해 오는 주소를 위의 신뢰 프록시에 추가해야 하며, 그 주소에서 온 연결만 헤더의 클라이언트 IP를 사용합니다.",
};

type ProxyProtocolKey = keyof typeof koProxyProtocol;

export const proxyProtocolStrings: Record<"en" | "zh" | "ja" | "ru" | "fr" | "de" | "es", Record<ProxyProtocolKey, string>> = {
  en: {
    ppTitle: "Accept PROXY protocol",
    ppDesc: "Turn on when a TCP tunnel or load balancer (FRP, HAProxy) passes the client IP in a PROXY protocol (v1/v2) header. Ports 80/443 stay as they are; separate ports accept the header.",
    ppHttpPort: "HTTP port",
    ppHttpsPort: "HTTPS port",
    ppHint: "Point the tunnel at these ports and publish them in docker-compose (e.g. '8443:8443'). Add the address the tunnel connects from to the trusted proxies above: only connections from there use the client IP in the header.",
  },
  zh: {
    ppTitle: "接受 PROXY protocol",
    ppDesc: "当 TCP 隧道或负载均衡器（FRP、HAProxy）通过 PROXY protocol（v1/v2）头传递客户端 IP 时开启。80/443 端口保持不变，另开端口接收该头。",
    ppHttpPort: "HTTP 端口",
    ppHttpsPort: "HTTPS 端口",
    ppHint: "将隧道指向这些端口，并在 docker-compose 中发布它们（例如 '8443:8443'）。请把隧道连接过来的地址加入上方的受信任代理：只有来自该地址的连接才会使用头中的客户端 IP。",
  },
  ja: {
    ppTitle: "PROXY protocol を受け付ける",
    ppDesc: "TCP トンネルやロードバランサー（FRP、HAProxy）が PROXY protocol（v1/v2）ヘッダーでクライアント IP を渡す場合にオンにします。80/443 ポートはそのままで、ヘッダーを受け付ける別のポートを開きます。",
    ppHttpPort: "HTTP ポート",
    ppHttpsPort: "HTTPS ポート",
    ppHint: "トンネルの接続先をこれらのポートにし、docker-compose でポートを公開してください（例: '8443:8443'）。トンネルの接続元アドレスを上の信頼するプロキシに追加する必要があり、そこからの接続だけがヘッダーのクライアント IP を使います。",
  },
  ru: {
    ppTitle: "Принимать PROXY protocol",
    ppDesc: "Включите, если TCP-туннель или балансировщик (FRP, HAProxy) передаёт IP клиента в заголовке PROXY protocol (v1/v2). Порты 80/443 не меняются: заголовок принимают отдельные порты.",
    ppHttpPort: "Порт HTTP",
    ppHttpsPort: "Порт HTTPS",
    ppHint: "Направьте туннель на эти порты и опубликуйте их в docker-compose (например, '8443:8443'). Добавьте адрес, с которого подключается туннель, в доверенные прокси выше: IP клиента из заголовка используется только для подключений с этого адреса.",
  },
  fr: {
    ppTitle: "Accepter le PROXY protocol",
    ppDesc: "À activer lorsqu'un tunnel TCP ou un répartiteur de charge (FRP, HAProxy) transmet l'IP du client dans un en-tête PROXY protocol (v1/v2). Les ports 80/443 restent inchangés ; des ports distincts acceptent l'en-tête.",
    ppHttpPort: "Port HTTP",
    ppHttpsPort: "Port HTTPS",
    ppHint: "Faites pointer le tunnel vers ces ports et publiez-les dans docker-compose (par ex. '8443:8443'). Ajoutez l'adresse depuis laquelle le tunnel se connecte aux proxys de confiance ci-dessus : seules les connexions venant de là utilisent l'IP client de l'en-tête.",
  },
  de: {
    ppTitle: "PROXY protocol annehmen",
    ppDesc: "Einschalten, wenn ein TCP-Tunnel oder Load Balancer (FRP, HAProxy) die Client-IP in einem PROXY-protocol-Header (v1/v2) übergibt. Die Ports 80/443 bleiben unverändert; eigene Ports nehmen den Header an.",
    ppHttpPort: "HTTP-Port",
    ppHttpsPort: "HTTPS-Port",
    ppHint: "Richte den Tunnel auf diese Ports aus und veröffentliche sie in docker-compose (z. B. '8443:8443'). Die Adresse, von der aus der Tunnel verbindet, muss oben bei den vertrauenswürdigen Proxys stehen: Nur Verbindungen von dort verwenden die Client-IP aus dem Header.",
  },
  es: {
    ppTitle: "Aceptar PROXY protocol",
    ppDesc: "Actívalo cuando un túnel TCP o un balanceador de carga (FRP, HAProxy) pase la IP del cliente en una cabecera PROXY protocol (v1/v2). Los puertos 80/443 no cambian; la cabecera se acepta en puertos aparte.",
    ppHttpPort: "Puerto HTTP",
    ppHttpsPort: "Puerto HTTPS",
    ppHint: "Apunta el túnel a estos puertos y publícalos en docker-compose (p. ej. '8443:8443'). Añade la dirección desde la que se conecta el túnel a los proxies de confianza de arriba: solo las conexiones desde allí usan la IP del cliente de la cabecera.",
  },
};
