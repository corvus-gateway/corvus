import type { HeadersKey } from "../headers.i18n";

export const headersStrings: Record<HeadersKey, string> = {
  secHeaders: "Заголовки безопасности",
  secHeadersHint: "Добавляет в ответы заголовки безопасности для браузера. Заголовки, которые уже отправляет приложение, не меняются.",
  shOff: "Выкл.",
  shBasic: "Базовые",
  shStrict: "Строгие",
  shOffDesc: "Ответы не изменяются.",
  shBasicDesc: "Добавляет X-Content-Type-Options, X-Frame-Options (SAMEORIGIN), Referrer-Policy и Permissions-Policy и скрывает X-Powered-By бэкенда. Подходит для большинства сайтов.",
  shStrictDesc: "Базовые плюс CSP в режиме только отчётов (ничего не блокирует) и Cross-Origin-Opener-Policy; cookie бэкенда получают HttpOnly, SameSite=Lax и Secure (по HTTPS), если их нет. Сначала проверьте, если скрипты читают cookie (например, CSRF-токены) или сайт использует вход во всплывающем окне.",
  headersShort: "Заголовки",
};
