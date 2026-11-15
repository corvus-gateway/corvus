import type { LimitsKey } from "../limits.i18n";

export const limitsStrings: Record<LimitsKey, string> = {
  maxUpload: "最大アップロードサイズ",
  maxUploadHint: "空欄: 既定の 100 MB。0: 無制限。",
  upstreamTimeout: "アップストリームのタイムアウト",
  upstreamTimeoutHint: "バックエンドの応答・リクエスト受け付けを待つ時間。空欄: 既定値(60 秒、WebSocket は 1 時間)。",
  gzip: "Gzip 圧縮",
  gzipDesc: "テキストの応答(HTML、CSS、JS、JSON など)を圧縮して送ります。",
};
