import type { LimitsKey } from "../limits.i18n";

export const limitsStrings: Record<LimitsKey, string> = {
  maxUpload: "Maximum upload size",
  maxUploadHint: "Empty: default 100 MB. 0: unlimited.",
  upstreamTimeout: "Upstream timeout",
  upstreamTimeoutHint: "How long to wait for the backend to answer or accept the request. Empty: default (60 s, WebSocket 1 h).",
  gzip: "Gzip compression",
  gzipDesc: "Compress text responses (HTML, CSS, JS, JSON, …).",
};
