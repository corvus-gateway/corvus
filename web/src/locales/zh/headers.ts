import type { HeadersKey } from "../headers.i18n";

export const headersStrings: Record<HeadersKey, string> = {
  secHeaders: "安全响应头",
  secHeadersHint: "为响应添加浏览器安全响应头。应用已发送的响应头保持不变。",
  shOff: "关闭",
  shBasic: "基础",
  shStrict: "严格",
  shOffDesc: "不修改响应头。",
  shBasicDesc: "添加 X-Content-Type-Options、X-Frame-Options（SAMEORIGIN）、Referrer-Policy 和 Permissions-Policy，并隐藏后端的 X-Powered-By。适用于大多数站点。",
  shStrictDesc: "在基础之上添加仅报告的内容安全策略（不拦截）和 Cross-Origin-Opener-Policy；后端 Cookie 缺少时补上 HttpOnly、SameSite=Lax 和 Secure（HTTPS）。如果脚本需要读取 Cookie（如 CSRF 令牌）或站点使用弹窗登录，请先确认。",
  headersShort: "响应头",
};
