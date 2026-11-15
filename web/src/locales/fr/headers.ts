import type { HeadersKey } from "../headers.i18n";

export const headersStrings: Record<HeadersKey, string> = {
  secHeaders: "En-têtes de sécurité",
  secHeadersHint: "Ajoute des en-têtes de sécurité navigateur aux réponses. Les en-têtes déjà envoyés par l'application restent inchangés.",
  shOff: "Désactivé",
  shBasic: "Basique",
  shStrict: "Strict",
  shOffDesc: "Les réponses ne sont pas modifiées.",
  shBasicDesc: "Ajoute X-Content-Type-Options, X-Frame-Options (SAMEORIGIN), Referrer-Policy et Permissions-Policy, et masque le X-Powered-By du backend. Convient à la plupart des sites.",
  shStrictDesc: "Basique plus une Content Security Policy en mode rapport seul (rien n'est bloqué) et Cross-Origin-Opener-Policy ; les cookies du backend reçoivent HttpOnly, SameSite=Lax et Secure (en HTTPS) s'ils manquent. Vérifiez d'abord si des scripts lisent des cookies (jetons CSRF, etc.) ou si le site utilise une connexion par popup.",
  headersShort: "En-têtes",
};
