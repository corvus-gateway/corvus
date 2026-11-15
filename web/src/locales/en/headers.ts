import type { HeadersKey } from "../headers.i18n";

export const headersStrings: Record<HeadersKey, string> = {
  secHeaders: "Security headers",
  secHeadersHint: "Adds browser security headers to responses. Headers the application already sends are left as they are.",
  shOff: "Off",
  shBasic: "Basic",
  shStrict: "Strict",
  shOffDesc: "Responses are not changed.",
  shBasicDesc: "Adds X-Content-Type-Options, X-Frame-Options (SAMEORIGIN), Referrer-Policy and Permissions-Policy, and hides the backend's X-Powered-By. Safe for most sites.",
  shStrictDesc: "Basic plus a report-only Content Security Policy (nothing is blocked) and Cross-Origin-Opener-Policy; backend cookies get HttpOnly, SameSite=Lax and Secure (on HTTPS) where missing. Check first if scripts must read cookies (e.g. CSRF tokens) or the site uses popup sign-in.",
  headersShort: "Headers",
};
