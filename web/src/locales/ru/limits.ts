import type { LimitsKey } from "../limits.i18n";

export const limitsStrings: Record<LimitsKey, string> = {
  maxUpload: "Максимальный размер загрузки",
  maxUploadHint: "Пусто: по умолчанию 100 МБ. 0: без ограничения.",
  upstreamTimeout: "Тайм-аут апстрима",
  upstreamTimeoutHint: "Сколько ждать ответа бэкенда или приёма запроса. Пусто: по умолчанию (60 с, WebSocket 1 ч).",
  gzip: "Сжатие gzip",
  gzipDesc: "Сжимать текстовые ответы (HTML, CSS, JS, JSON и т. п.).",
};
