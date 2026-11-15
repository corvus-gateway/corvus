// Security header presets in the proxy host sheet — Korean source strings + translations.
export const koHeaders = {
  secHeaders: "보안 헤더",
  secHeadersHint: "응답에 브라우저 보안 헤더를 추가합니다. 애플리케이션이 이미 보내는 헤더는 그대로 둡니다.",
  shOff: "끔",
  shBasic: "기본",
  shStrict: "엄격",
  shOffDesc: "응답 헤더를 바꾸지 않습니다.",
  shBasicDesc: "X-Content-Type-Options, X-Frame-Options(SAMEORIGIN), Referrer-Policy, Permissions-Policy를 추가하고 백엔드의 X-Powered-By를 숨깁니다. 대부분의 사이트에서 문제없이 쓸 수 있습니다.",
  shStrictDesc: "기본에 더해 보고 전용 CSP(차단하지 않음)와 Cross-Origin-Opener-Policy를 추가하고, 백엔드 쿠키에 HttpOnly·SameSite=Lax·Secure(HTTPS)를 빠진 경우에만 붙입니다. 스크립트가 읽어야 하는 쿠키(CSRF 토큰 등)나 팝업 로그인을 쓰는 사이트는 확인 후 사용하세요.",
  headersShort: "헤더",
};

export type HeadersKey = keyof typeof koHeaders;
