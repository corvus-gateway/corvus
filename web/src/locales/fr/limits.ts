import type { LimitsKey } from "../limits.i18n";

export const limitsStrings: Record<LimitsKey, string> = {
  maxUpload: "Taille maximale d'envoi",
  maxUploadHint: "Vide : 100 Mo par défaut. 0 : illimité.",
  upstreamTimeout: "Délai d'attente de l'amont",
  upstreamTimeoutHint: "Durée d'attente de la réponse du backend ou de l'envoi de la requête. Vide : par défaut (60 s, WebSocket 1 h).",
  gzip: "Compression gzip",
  gzipDesc: "Compresser les réponses texte (HTML, CSS, JS, JSON, …).",
};
