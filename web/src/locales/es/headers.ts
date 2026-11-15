import type { HeadersKey } from "../headers.i18n";

export const headersStrings: Record<HeadersKey, string> = {
  secHeaders: "Cabeceras de seguridad",
  secHeadersHint: "Añade cabeceras de seguridad del navegador a las respuestas. Las cabeceras que la aplicación ya envía se mantienen.",
  shOff: "Desactivado",
  shBasic: "Básico",
  shStrict: "Estricto",
  shOffDesc: "Las respuestas no se modifican.",
  shBasicDesc: "Añade X-Content-Type-Options, X-Frame-Options (SAMEORIGIN), Referrer-Policy y Permissions-Policy, y oculta el X-Powered-By del backend. Seguro para la mayoría de los sitios.",
  shStrictDesc: "Básico más una Content Security Policy solo de informe (no bloquea nada) y Cross-Origin-Opener-Policy; las cookies del backend reciben HttpOnly, SameSite=Lax y Secure (en HTTPS) si faltan. Compruébalo antes si los scripts deben leer cookies (p. ej. tokens CSRF) o el sitio usa inicio de sesión en ventana emergente.",
  headersShort: "Cabeceras",
};
