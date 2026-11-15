import type { HeadersKey } from "../headers.i18n";

export const headersStrings: Record<HeadersKey, string> = {
  secHeaders: "Sicherheits-Header",
  secHeadersHint: "Fügt Antworten Browser-Sicherheits-Header hinzu. Header, die die Anwendung bereits sendet, bleiben unverändert.",
  shOff: "Aus",
  shBasic: "Basis",
  shStrict: "Streng",
  shOffDesc: "Antworten werden nicht verändert.",
  shBasicDesc: "Fügt X-Content-Type-Options, X-Frame-Options (SAMEORIGIN), Referrer-Policy und Permissions-Policy hinzu und blendet das X-Powered-By des Backends aus. Für die meisten Websites unbedenklich.",
  shStrictDesc: "Basis plus eine Content Security Policy nur im Berichtsmodus (blockiert nichts) und Cross-Origin-Opener-Policy; Backend-Cookies erhalten HttpOnly, SameSite=Lax und Secure (bei HTTPS), wo sie fehlen. Vorher prüfen, wenn Skripte Cookies lesen müssen (z. B. CSRF-Tokens) oder die Website eine Anmeldung per Pop-up nutzt.",
  headersShort: "Header",
};
