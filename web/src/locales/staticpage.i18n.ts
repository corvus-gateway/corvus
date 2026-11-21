// "Serve a page" in the proxy host sheet and host list — Korean source strings + translations.
export const koStaticPage = {
  spContent: "응답 방식",
  spForward: "서비스로 전달",
  spPage: "페이지 표시",
  spPageDesc: "백엔드 없이 게이트웨이가 페이지를 직접 응답합니다. 보안 규칙, Basic 인증, 속도 제한, HTTPS, 접근 로그와 통계는 그대로 적용되고, 사용자 지정 위치는 계속 백엔드로 전달합니다.",
  spTemplate: "템플릿",
  spTplParked: "주차된 도메인",
  spTplComingSoon: "준비 중",
  spTplMaintenance: "점검 중",
  spTplCustom: "직접 작성한 HTML",
  spStatus: "HTTP 상태 코드",
  spStatusHint: "점검 중에는 503이 일반적입니다. 검색 엔진이 페이지를 색인하지 않고 나중에 다시 방문합니다. 200이 아닌 응답은 캐시되지 않습니다.",
  spTitle: "제목",
  spMessage: "메시지",
  spTextHint: "비워 두면 기본 문구가 방문자의 브라우저 언어로 표시됩니다. 입력한 문구는 그대로(HTML 없이) 표시됩니다.",
  spHtmlHint: "입력한 그대로 전송됩니다. 이미지와 CSS는 페이지 안에 넣거나 다른 사이트에서 불러오세요.",
  spPreviewHint: "콘솔 언어로 표시한 미리보기입니다. 방문자에게는 기본 문구가 브라우저 언어로 표시됩니다.",
  spPreviewFailed: "미리보기를 표시할 수 없습니다",
  spListPage: "페이지: {name}",
} as const;

export type StaticPageKey = keyof typeof koStaticPage;
