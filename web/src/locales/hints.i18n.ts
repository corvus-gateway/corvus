// Actionable error hints (components/ErrorHint.tsx). Korean is the key source; every
// other language has a full translation. One sentence on the cause plus the action.

export const koHints = {
  hintHowToFix: "해결 방법",
  hintAcmeRateLimited: "Let's Encrypt 발급 한도에 걸렸습니다. 같은 도메인으로 반복 발급하지 말고 오류에 표시된 시각 이후 다시 시도하세요.",
  hintDnsServfail: "도메인의 네임서버가 _acme-challenge 조회에 SERVFAIL/REFUSED로 응답합니다. DNS 서비스에서 DNSSEC과 NS 위임 설정을 확인하세요.",
  hintDnsPropagation: "_acme-challenge TXT 레코드가 제한 시간 안에 모든 네임서버에 반영되지 않았습니다. 잠시 후 다시 시도하고, 도메인의 NS가 DNS 인증 정보의 공급자를 가리키는지 확인하세요.",
  hintDnsCredential: "DNS 공급자가 TXT 레코드 생성을 거부했습니다. API 토큰의 권한(해당 영역의 DNS 편집)과 영역 이름을 확인하세요.",
  hintDnsLookup: "도메인의 권한 네임서버를 찾지 못했습니다. 도메인이 등록·위임되어 있고 Corvus 서버에서 DNS 조회가 되는지 확인하세요.",
  hintAcmeCaa: "도메인의 CAA 레코드가 Let's Encrypt 발급을 막고 있습니다. CAA 레코드에 letsencrypt.org를 추가하거나 레코드를 삭제하세요.",
  hintAcmeRejectedName: "Let's Encrypt는 이 이름으로 인증서를 발급하지 않습니다. IP 주소나 내부 이름이 아닌 공개 도메인 이름을 사용하세요.",
  hintAcmeEmail: "Let's Encrypt가 계정 이메일을 거부했습니다. 실제로 메일을 받을 수 있는 주소로 바꾸세요.",
  hintAcmeDns: "Let's Encrypt가 도메인의 DNS 레코드를 찾지 못했습니다. 공개 DNS에 A/AAAA 레코드가 있는지 확인하세요.",
  hintAcmePort80: "Let's Encrypt가 80번 포트로 이 서버에 접속하지 못했습니다. 방화벽·포트 포워딩과 도메인의 A/AAAA 레코드를 확인하거나 DNS-01 방식을 사용하세요.",
  hintAcmeWrongServer: "HTTP-01 검증 요청이 Corvus가 아닌 다른 서버에 도착했습니다. 도메인의 A/AAAA 레코드와 앞단의 프록시·CDN 설정을 확인하세요.",
  hintAcmeCaUnreachable: "Corvus 서버에서 Let's Encrypt에 연결하지 못했습니다. 서버의 외부 인터넷 접속, DNS, 프록시 설정을 확인하세요.",
  hintDeployPortInUse: "게이트웨이가 새 설정을 적용하지 못했습니다. 대개 다른 프로그램이 이미 쓰고 있는 포트 때문이니 스트림의 포트를 확인하세요.",
  hintDeployAdvanced: "호스트의 고급 설정에 nginx가 받아들이지 않는 지시어가 있습니다. 출력에 표시된 호스트의 고급 설정을 고치세요.",
  hintNetDns: "호스트 이름을 찾을 수 없습니다. 주소에 오타가 없는지, Corvus 서버의 DNS 설정이 올바른지 확인하세요.",
  hintNetRefused: "대상 서버가 연결을 거부했습니다. 호스트와 포트가 맞는지, 서비스가 실행 중인지 확인하세요.",
  hintNetTimeout: "대상 서버가 제한 시간 안에 응답하지 않았습니다. Corvus 서버의 외부 접속(방화벽·프록시)과 대상 서버 상태를 확인하세요.",
  hintNetTls: "대상 서버의 TLS 인증서를 검증할 수 없습니다. 주소를 확인하고, 공인 인증 기관에서 발급한 인증서를 사용하세요.",
  hintNetBlocked: "Corvus는 보안을 위해 이 주소(루프백·링크 로컬·사설망)로 연결하지 않습니다. 외부에서 접근 가능한 주소를 사용하세요.",
  hintNetAuth: "대상 서버가 인증 정보를 거부했습니다(HTTP 401/403). 토큰, 웹훅 URL, 비밀번호를 확인하세요.",
  hintNetNotFound: "주소가 404 Not Found를 반환했습니다. URL을 확인하세요(SSO는 /.well-known/… 을 뺀 발급자 URL).",
  hintNetMysqlAuth: "MySQL이 사용자 이름 또는 비밀번호를 거부했습니다. Nginx Proxy Manager 설정에 있는 DB 계정 정보를 입력하세요.",
  hintNetMysqlDb: "해당 MySQL 데이터베이스가 없습니다. 데이터베이스 이름을 확인하세요(Nginx Proxy Manager 기본값은 npm).",
  hintNetOidcIssuer: "입력한 발급자 URL이 공급자가 알려 준 issuer와 정확히 일치하지 않습니다. 디스커버리 문서의 issuer 값을 끝의 슬래시까지 그대로 복사하세요.",
} as const;

export type HintKey = keyof typeof koHints;
