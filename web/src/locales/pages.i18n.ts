// Custom block and error pages — 8 languages.
export const koPages = {
  navPages: "차단·오류 페이지",
  pagesDesc: "보안 규칙·차단 IP·공격 패턴 차단 시 보여 줄 차단 페이지와, 백엔드에 연결할 수 없을 때(502/503/504) 보여 줄 오류 페이지를 직접 꾸밉니다. 저장하면 즉시 게이트웨이에 배포됩니다.",
  blockPage: "차단 페이지", errorPage: "오류 페이지",
  blockPageDesc: "보안 규칙, 자동·수동 차단, 공격 패턴 차단으로 요청이 거부될 때 표시됩니다.",
  errorPageDesc: "프록시 호스트의 백엔드가 응답하지 않거나(연결 거부·시간 초과) 잘못된 응답을 보낼 때 표시됩니다. 백엔드가 직접 보낸 오류 응답은 그대로 전달됩니다.",
  pageCustom: "사용자 지정", pageBuiltin: "기본 페이지",
  pageHtml: "HTML", pageBytes: "{n} / {max} KB", pageTooLarge: "페이지가 너무 큽니다 (최대 {max} KB)",
  pagePlaceholders: "자리표시자",
  pagePlaceholdersHint: "페이지를 보낼 때 실제 값으로 바뀝니다. 값은 HTML 이스케이프됩니다.",
  pageVarStatus: "상태 코드", pageVarRid: "요청 ID", pageVarHost: "요청한 사이트(호스트)", pageVarIp: "방문자 IP", pageVarTime: "시각",
  pagePreview: "미리보기", pagePreviewHint: "예시 값으로 표시하며, 미리보기에서는 스크립트가 실행되지 않습니다(기본 페이지의 자동 번역 포함).",
  pageReset: "기본값으로 복원", pageResetTitle: "기본 페이지로 복원할까요?",
  pageResetDesc: "사용자 지정 페이지가 삭제되고 기본 페이지가 즉시 배포됩니다.", pageResetDone: "기본 페이지로 복원했습니다",
  unknownHosts: "알 수 없는 호스트",
  unknownHostsDesc: "프록시 호스트나 리디렉션 호스트로 등록되지 않은 호스트 이름으로 들어온 HTTP 요청에 어떻게 응답할지 정합니다. 이런 이름의 HTTPS 연결은 항상 TLS 핸드셰이크에서 거부됩니다.",
  unknownMode: "응답 방식",
  unknownNotFound: "찾을 수 없음 페이지 (404)", unknownClose: "연결 끊기", unknownRedirect: "URL로 리디렉션",
  unknownRedirectUrl: "리디렉션 URL",
  unknownRedirectHint: "http:// 또는 https://로 시작하는 전체 URL입니다. 302로 응답하며 요청 경로는 붙이지 않습니다.",
  importUnknownHosts: "NPM의 기본 사이트 설정을 알 수 없는 호스트에 적용: {mode}",
} as const;

export type K = keyof typeof koPages;
