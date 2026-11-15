import type { LimitsKey } from "../limits.i18n";

export const limitsStrings: Record<LimitsKey, string> = {
  maxUpload: "最大上传大小",
  maxUploadHint: "留空：默认 100 MB。0：不限制。",
  upstreamTimeout: "上游超时",
  upstreamTimeoutHint: "等待后端响应或接收请求的时间。留空：默认（60 秒，WebSocket 1 小时）。",
  gzip: "Gzip 压缩",
  gzipDesc: "压缩文本响应（HTML、CSS、JS、JSON 等）。",
};
