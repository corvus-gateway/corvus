import type { CrashKey } from "../crash.i18n";

export const crashStrings: Record<CrashKey, string> = {
  crashTitle: "このページを表示できませんでした",
  crashDesc: "このページの描画中にエラーが発生しました。他のメニューは引き続き使用できます。再読み込みしても繰り返す場合は、以下の内容をお知らせください。",
  crashUpdated: "コンソールが新しいバージョンに更新されました。再読み込みすると続行できます。",
  crashReload: "再読み込み",
  serverDownTitle: "サーバーに接続できません",
  serverDownDesc: "Corvus が再起動中か、ネットワーク接続が切れた可能性があります。復旧すると自動的に続行します。",
  serverDownRetry: "今すぐ再試行",
};
