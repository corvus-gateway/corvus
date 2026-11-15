// Streams page (/streams, TCP/UDP port forwarding) — Korean source strings + translations.
export const koStreams = {
  navStreams: "스트림",
  streamsDesc: "TCP/UDP 포트로 들어온 연결을 다른 호스트로 전달합니다(데이터베이스, SSH, MQTT, 게임 서버 등). 저장하면 즉시 게이트웨이에 배포됩니다.",
  addStream: "스트림 추가",
  editStream: "스트림 편집",
  streamSheetDesc: "받을 포트와 프로토콜, 전달할 호스트와 포트를 설정합니다.",
  noStreams: "스트림이 없습니다",
  noStreamsDesc: "HTTP가 아닌 TCP/UDP 서비스를 전달하려면 스트림을 추가하세요.",
  searchStreams: "포트 또는 호스트 검색",
  incomingPort: "수신 포트",
  incomingPortHint: "게이트웨이 컨테이너가 받을 포트입니다. Docker 브리지 네트워크에서는 compose의 ports에도 게시해야 합니다(예: \"5432:5432\").",
  streamProtocol: "프로토콜",
  streamProtocolHint: "TCP, UDP 또는 둘 다 선택합니다.",
  secStreamSource: "수신",
  secStreamForward: "전달 대상",
  streamTlsHint: "선택하면 TCP 연결의 TLS를 게이트웨이에서 종료하고 평문으로 전달합니다. UDP에는 적용되지 않습니다.",
  streamTlsPending: "인증서가 발급될 때까지 TCP는 열리지 않습니다.",
  streamEnabledDesc: "끄면 이 포트가 열리지 않습니다.",
  deleteStreamTitle: "스트림 삭제",
  secStreamPP: "PROXY protocol",
  streamPPBoth: "TCP 연결에만 적용되며 UDP는 그대로입니다.",
  streamPPIn: "PROXY protocol 받기",
  streamPPInDesc: "모든 연결이 PROXY protocol 헤더로 시작해야 합니다(FRP, HAProxy, 로드밸런서). 네트워크 페이지의 '신뢰할 PROXY protocol 출처'에서 온 연결만 헤더의 클라이언트 IP를 사용합니다.",
  streamPPOut: "백엔드에 PROXY protocol 보내기",
  streamPPOutDesc: "백엔드가 클라이언트 IP를 헤더(v1)로 받습니다. 백엔드가 PROXY protocol을 기대하도록 설정되어 있어야 하며, 아니면 연결이 실패합니다.",
};

export type StreamKey = keyof typeof koStreams;
