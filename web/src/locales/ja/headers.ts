import type { HeadersKey } from "../headers.i18n";

export const headersStrings: Record<HeadersKey, string> = {
  secHeaders: "セキュリティヘッダー",
  secHeadersHint: "レスポンスにブラウザー向けのセキュリティヘッダーを追加します。アプリケーションが既に送っているヘッダーはそのままです。",
  shOff: "オフ",
  shBasic: "基本",
  shStrict: "厳格",
  shOffDesc: "レスポンスヘッダーを変更しません。",
  shBasicDesc: "X-Content-Type-Options、X-Frame-Options（SAMEORIGIN）、Referrer-Policy、Permissions-Policy を追加し、バックエンドの X-Powered-By を隠します。ほとんどのサイトで安全に使えます。",
  shStrictDesc: "基本に加えてレポート専用の CSP（ブロックしません）と Cross-Origin-Opener-Policy を追加し、バックエンドの Cookie に HttpOnly・SameSite=Lax・Secure（HTTPS）が無い場合だけ付けます。スクリプトが Cookie を読む（CSRF トークンなど）サイトやポップアップでログインするサイトは事前に確認してください。",
  headersShort: "ヘッダー",
};
