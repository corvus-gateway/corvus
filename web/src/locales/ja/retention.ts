import type { K } from "../retention.i18n";

export const retentionStrings: Record<K, string> = {
  navRetention: "データ保持",
  retentionDesc: "トラフィック統計・セキュリティイベント・リクエストログを保持する期間と容量を設定します。上限を超えたデータは 1 時間ごとに自動で整理されます。",
  retentionHow: "統計とイベントは再起動後も保持されます。リクエストログは日ごとのファイルに保存され、過去の日付のファイルは圧縮されます。容量上限を超えると、最も古い日付のファイルから削除されます。",
  logMode: "リクエストログ",
  logModeOff: "保存しない", logModeNotable: "重要なリクエストのみ", logModeAll: "すべてのリクエスト",
  logModeDesc: "重要なリクエスト：4xx/5xx 応答、ブロック、セキュリティ検知、アクセスキューの待機。「すべてのリクエスト」はトラフィックが多いと 1 日数 GB になる場合があります。",
  logDays: "ログ保持期間（日）", logMaxMb: "ログ容量上限 (MB)", logMaxHint: "超えると最も古い日付のファイルから削除します。",
  statsDays: "統計保持期間（日）", eventsDays: "イベント保持期間（日）",
  storageUsage: "ストレージ", dbSize: "統計・イベント DB", logSize: "リクエストログ", logFilesN: "{n} ファイル", logLimit: "上限 {mb} MB",
  droppedLines: "ディスク書き込みが追いつかず記録できなかったログ {n} 行",
  archives: "リクエストログファイル", noArchives: "ログファイルはありません", noArchivesDesc: "リクエストログを有効にすると、日ごとのファイルがここに表示されます。",
  fileName: "ファイル", fileSize: "サイズ", fileModified: "更新日時", retentionSaved: "保持設定を保存しました",
};
