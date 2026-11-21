import type { StaticPageKey } from "../staticpage.i18n";

export const staticPageStrings: Record<StaticPageKey, string> = {
  spContent: "応答方法",
  spForward: "サービスへ転送",
  spPage: "ページを表示",
  spPageDesc: "バックエンドなしでゲートウェイが自らページを返します。セキュリティルール、Basic 認証、レート制限、HTTPS、アクセスログと統計はそのまま適用され、カスタムロケーションは引き続きバックエンドへ転送します。",
  spTemplate: "テンプレート",
  spTplParked: "パーキングドメイン",
  spTplComingSoon: "近日公開",
  spTplMaintenance: "メンテナンス中",
  spTplCustom: "独自の HTML",
  spStatus: "HTTP ステータス",
  spStatusHint: "メンテナンスには 503 が一般的です。検索エンジンはページをインデックスせず、後で再訪問します。200 以外の応答はキャッシュされません。",
  spTitle: "タイトル",
  spMessage: "メッセージ",
  spTextHint: "空欄の場合、既定の文言が訪問者のブラウザの言語で表示されます。入力した文言はそのまま (HTML なし) 表示されます。",
  spHtmlHint: "入力したとおりに送信されます。画像や CSS はページに埋め込むか、別のサイトから読み込んでください。",
  spPreviewHint: "コンソールの言語で表示したプレビューです。訪問者には既定の文言がブラウザの言語で表示されます。",
  spPreviewFailed: "プレビューを表示できません",
  spListPage: "ページ: {name}",
  spHtmlNotLoaded: "ページの HTML を読み込めませんでした。ここに新しい HTML を入力しない限り、保存済みの内容のまま残ります。",
};
