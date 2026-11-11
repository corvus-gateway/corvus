// Upload size, upstream timeout and gzip in the proxy host sheet — Korean source strings + translations.
export const koLimits = {
  maxUpload: "최대 업로드 크기",
  maxUploadHint: "비워 두면 기본값 100 MB, 0이면 제한 없음.",
  upstreamTimeout: "업스트림 타임아웃",
  upstreamTimeoutHint: "백엔드 응답 읽기·요청 전송 대기 시간. 비워 두면 기본값(60초, WebSocket은 1시간).",
  gzip: "Gzip 압축",
  gzipDesc: "텍스트 응답(HTML, CSS, JS, JSON 등)을 압축해서 보냅니다.",
};

type LimitsKey = keyof typeof koLimits;

export const limitsStrings: Record<"en" | "zh" | "ja" | "ru" | "fr" | "de" | "es", Record<LimitsKey, string>> = {
  en: {
    maxUpload: "Maximum upload size",
    maxUploadHint: "Empty: default 100 MB. 0: unlimited.",
    upstreamTimeout: "Upstream timeout",
    upstreamTimeoutHint: "How long to wait for the backend to answer or accept the request. Empty: default (60 s, WebSocket 1 h).",
    gzip: "Gzip compression",
    gzipDesc: "Compress text responses (HTML, CSS, JS, JSON, …).",
  },
  zh: {
    maxUpload: "最大上传大小",
    maxUploadHint: "留空：默认 100 MB。0：不限制。",
    upstreamTimeout: "上游超时",
    upstreamTimeoutHint: "等待后端响应或接收请求的时间。留空：默认（60 秒，WebSocket 1 小时）。",
    gzip: "Gzip 压缩",
    gzipDesc: "压缩文本响应（HTML、CSS、JS、JSON 等）。",
  },
  ja: {
    maxUpload: "最大アップロードサイズ",
    maxUploadHint: "空欄: 既定の 100 MB。0: 無制限。",
    upstreamTimeout: "アップストリームのタイムアウト",
    upstreamTimeoutHint: "バックエンドの応答・リクエスト受け付けを待つ時間。空欄: 既定値(60 秒、WebSocket は 1 時間)。",
    gzip: "Gzip 圧縮",
    gzipDesc: "テキストの応答(HTML、CSS、JS、JSON など)を圧縮して送ります。",
  },
  ru: {
    maxUpload: "Максимальный размер загрузки",
    maxUploadHint: "Пусто: по умолчанию 100 МБ. 0: без ограничения.",
    upstreamTimeout: "Тайм-аут апстрима",
    upstreamTimeoutHint: "Сколько ждать ответа бэкенда или приёма запроса. Пусто: по умолчанию (60 с, WebSocket 1 ч).",
    gzip: "Сжатие gzip",
    gzipDesc: "Сжимать текстовые ответы (HTML, CSS, JS, JSON и т. п.).",
  },
  fr: {
    maxUpload: "Taille maximale d'envoi",
    maxUploadHint: "Vide : 100 Mo par défaut. 0 : illimité.",
    upstreamTimeout: "Délai d'attente de l'amont",
    upstreamTimeoutHint: "Durée d'attente de la réponse du backend ou de l'envoi de la requête. Vide : par défaut (60 s, WebSocket 1 h).",
    gzip: "Compression gzip",
    gzipDesc: "Compresser les réponses texte (HTML, CSS, JS, JSON, …).",
  },
  de: {
    maxUpload: "Maximale Upload-Größe",
    maxUploadHint: "Leer: Standard 100 MB. 0: unbegrenzt.",
    upstreamTimeout: "Upstream-Timeout",
    upstreamTimeoutHint: "Wie lange auf die Antwort des Backends oder die Annahme der Anfrage gewartet wird. Leer: Standard (60 s, WebSocket 1 h).",
    gzip: "Gzip-Komprimierung",
    gzipDesc: "Textantworten (HTML, CSS, JS, JSON, …) komprimieren.",
  },
  es: {
    maxUpload: "Tamaño máximo de subida",
    maxUploadHint: "Vacío: 100 MB por defecto. 0: sin límite.",
    upstreamTimeout: "Tiempo de espera del upstream",
    upstreamTimeoutHint: "Cuánto esperar a que el backend responda o acepte la petición. Vacío: por defecto (60 s, WebSocket 1 h).",
    gzip: "Compresión gzip",
    gzipDesc: "Comprimir las respuestas de texto (HTML, CSS, JS, JSON, …).",
  },
};
