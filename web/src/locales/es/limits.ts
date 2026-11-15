import type { LimitsKey } from "../limits.i18n";

export const limitsStrings: Record<LimitsKey, string> = {
  maxUpload: "Tamaño máximo de subida",
  maxUploadHint: "Vacío: 100 MB por defecto. 0: sin límite.",
  upstreamTimeout: "Tiempo de espera del upstream",
  upstreamTimeoutHint: "Cuánto esperar a que el backend responda o acepte la petición. Vacío: por defecto (60 s, WebSocket 1 h).",
  gzip: "Compresión gzip",
  gzipDesc: "Comprimir las respuestas de texto (HTML, CSS, JS, JSON, …).",
};
