import type { K } from "../pages.i18n";

export const pagesStrings: Record<K, string> = {
  navPages: "ブロック・エラーページ",
  pagesDesc: "セキュリティルール・BAN・攻撃パターンでリクエストを拒否したときのブロックページと、バックエンドに接続できないとき（502/503/504）のエラーページをカスタマイズします。保存するとすぐにゲートウェイへデプロイされます。",
  blockPage: "ブロックページ", errorPage: "エラーページ",
  blockPageDesc: "セキュリティルール、自動・手動 BAN、攻撃パターンのブロックでリクエストが拒否されたときに表示されます。",
  errorPageDesc: "プロキシホストのバックエンドが応答しない（接続拒否・タイムアウト）または不正な応答を返したときに表示されます。バックエンド自身が返したエラー応答はそのまま転送されます。",
  pageCustom: "カスタム", pageBuiltin: "標準ページ",
  pageHtml: "HTML", pageBytes: "{n} / {max} KB", pageTooLarge: "ページが大きすぎます（最大 {max} KB）",
  pagePlaceholders: "プレースホルダー",
  pagePlaceholdersHint: "ページ送信時に実際の値に置き換えられます。値は HTML エスケープされます。",
  pageVarStatus: "ステータスコード", pageVarRid: "リクエストID", pageVarHost: "リクエスト先サイト（ホスト）", pageVarIp: "訪問者の IP", pageVarTime: "日時",
  pagePreview: "プレビュー", pagePreviewHint: "サンプル値で表示します。プレビューではスクリプトは実行されません（標準ページの自動翻訳を含む）。",
  pageReset: "既定に戻す", pageResetTitle: "標準ページに戻しますか？",
  pageResetDesc: "カスタムページは削除され、標準ページがすぐにデプロイされます。", pageResetDone: "標準ページに戻しました",
  unknownHosts: "不明なホスト",
  unknownHostsDesc: "どのプロキシホストにもリダイレクトホストにも設定されていないホスト名への HTTP リクエストに、Corvus がどう応答するかを設定します。こうした名前への HTTPS 接続は常に TLS ハンドシェイクで拒否されます。",
  unknownMode: "応答方法",
  unknownNotFound: "Not Found ページ（404）", unknownClose: "接続を切断", unknownRedirect: "URL へリダイレクト",
  unknownRedirectUrl: "リダイレクト先 URL",
  unknownRedirectHint: "http:// または https:// で始まる完全な URL。302 で応答し、リクエストのパスは付加しません。",
  importUnknownHosts: "NPM の既定サイト設定を不明なホストに適用：{mode}",
};
