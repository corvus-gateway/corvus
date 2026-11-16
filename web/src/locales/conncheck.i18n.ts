// "Check connection" in the proxy host form and the upstream error warning of the host
// list — Korean source strings + translations.
export const koConnCheck = {
  ccButton: "연결 확인",
  ccChecking: "확인 중…",
  ccOk: "게이트웨이가 {addr}에 연결했습니다 ({ms} ms)",
  ccMapped: "게이트웨이는 이 호스트를 {target}(으)로 연결합니다.",
  ccDnsTitle: "이름을 찾을 수 없음",
  ccDnsHint: "게이트웨이는 전달 호스트를 검색 도메인 없이 전체 DNS 이름으로 조회합니다. nas.home.lan 같은 전체 이름이나 IP 주소를 입력하거나, docker-compose.yml의 extra_hosts에 이름을 추가하세요.",
  ccRefusedTitle: "연결 거부됨",
  ccRefusedHint: "주소에는 닿았지만 이 포트에서 연결을 받는 서비스가 없습니다. 포트 번호와 서비스 실행 여부, 서비스가 127.0.0.1이 아닌 0.0.0.0에서 대기하는지 확인하세요. 방화벽이 연결을 거부하는 경우도 있습니다.",
  ccTimeoutTitle: "응답 없음 (시간 초과)",
  ccTimeoutHint: "5초 안에 응답이 없었습니다. 대개 대상 서버의 방화벽이 게이트웨이 서버의 연결을 막고 있으니 그 서버에서 게이트웨이 서버를 허용하세요. 주소가 틀렸거나 서버가 꺼져 있을 수도 있습니다.",
  ccUnreachableTitle: "네트워크에 도달할 수 없음",
  ccUnreachableHint: "게이트웨이에서 이 주소로 가는 경로가 없습니다. IP 주소를 확인하세요. 주소가 Docker 네트워크 대역(예: 172.17~172.31.x.x)과 겹친다면 Docker 서브넷이 LAN과 충돌하는 것이니 Docker 네트워크의 서브넷을 바꾸세요.",
  ccTlsTitle: "TLS 핸드셰이크 실패",
  ccTlsHint: "포트는 연결을 받지만 HTTPS로 응답하지 않습니다. 스킴을 http로 바꾸거나 서비스의 HTTPS 포트를 입력하세요.",
  ccOtherTitle: "연결 실패",
  ccOtherHint: "아래 메시지와 게이트웨이 로그를 확인하세요.",
  ueBadge: "업스트림 오류",
  ueTip: "{ago} 업스트림 연결 실패: {reason}",
  ueTipAction: "호스트를 편집해 ‘연결 확인’으로 원인을 찾아보세요.",
  ueSheetTitle: "{ago} 방문자에게 오류 페이지가 표시되었습니다",
};

export type ConnCheckKey = keyof typeof koConnCheck;
