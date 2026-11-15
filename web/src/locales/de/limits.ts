import type { LimitsKey } from "../limits.i18n";

export const limitsStrings: Record<LimitsKey, string> = {
  maxUpload: "Maximale Upload-Größe",
  maxUploadHint: "Leer: Standard 100 MB. 0: unbegrenzt.",
  upstreamTimeout: "Upstream-Timeout",
  upstreamTimeoutHint: "Wie lange auf die Antwort des Backends oder die Annahme der Anfrage gewartet wird. Leer: Standard (60 s, WebSocket 1 h).",
  gzip: "Gzip-Komprimierung",
  gzipDesc: "Textantworten (HTML, CSS, JS, JSON, …) komprimieren.",
};
