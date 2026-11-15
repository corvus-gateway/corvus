// PROXY protocol listeners (Network page) — Korean source strings + translations.
export const koProxyProtocol = {
  ppTitle: "PROXY protocol 받기",
  ppDesc: "FRP·HAProxy 같은 TCP 터널이나 로드밸런서가 PROXY protocol(v1/v2) 헤더로 클라이언트 IP를 넘겨줄 때 켭니다. 80/443 포트는 그대로 두고, 헤더를 받는 포트를 따로 엽니다.",
  ppHttpPort: "HTTP 포트",
  ppHttpsPort: "HTTPS 포트",
  ppPortInvalid: "1~65535 사이의 포트를 입력하세요.",
  ppTrusted: "신뢰할 PROXY protocol 출처",
  ppTrustedHint: "헤더를 보내는 터널·로드밸런서의 주소입니다. 이 주소에서 온 연결만 헤더의 클라이언트 IP를 사용합니다. 위의 신뢰 프록시와는 별개이며, 터널 주소를 신뢰 프록시에 넣지 마세요.",
  ppHint: "터널이 이 포트로 연결하도록 설정하고 docker-compose에서 포트를 공개하세요. 터널이 같은 호스트에 있으면 '127.0.0.1:8443:8443'처럼 루프백에만 공개하고, 아니면 방화벽으로 터널 주소만 허용하세요.",
};

export type ProxyProtocolKey = keyof typeof koProxyProtocol;
