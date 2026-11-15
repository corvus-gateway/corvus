// Upload size, upstream timeout and gzip in the proxy host sheet — Korean source strings + translations.
export const koLimits = {
  maxUpload: "최대 업로드 크기",
  maxUploadHint: "비워 두면 기본값 100 MB, 0이면 제한 없음.",
  upstreamTimeout: "업스트림 타임아웃",
  upstreamTimeoutHint: "백엔드 응답 읽기·요청 전송 대기 시간. 비워 두면 기본값(60초, WebSocket은 1시간).",
  gzip: "Gzip 압축",
  gzipDesc: "텍스트 응답(HTML, CSS, JS, JSON 등)을 압축해서 보냅니다.",
};

export type LimitsKey = keyof typeof koLimits;
