import type { K } from "../templates.i18n";

export const templatesStrings: Record<K, string> = {
  fromTemplate: "テンプレートから追加",
  templatesTitle: "仮想パッチテンプレート",
  templatesDesc: "パッチを適用するまで既知の CVE を防ぐルールです。まず「ログ」で適用して誤検知がないことを確認し、その後「ブロック」に切り替えてください。",
  tplSevCritical: "緊急", tplSevHigh: "高",
  tplProducts: "対象", tplFalsePositives: "誤検知の可能性",
  tplExemptList: "除外 IP リスト（管理者ネットワーク）", tplExemptNone: "除外なし",
  tplExemptHint: "このテンプレートは管理者ネットワークを除外できます。指定しない場合はすべての IP に適用されます。",
  tplApply: "ルールを作成", tplCreated: "テンプレートからルールを作成しました", tplSelect: "テンプレートを選択してください",
  managedRules: "マネージドルール",
  managedRulesDesc: "ボディ検査シグネチャと仮想パッチテンプレートのバンドルです。署名が検証された新しいバージョンのみ適用されます。",
  mrVersion: "バージョン", mrSource: "提供元", mrBuiltin: "組み込み", mrInstalled: "更新済み", mrSignatures: "シグネチャ", mrTemplates: "テンプレート", mrChecked: "最終確認",
  mrUrl: "更新 URL", mrUrlHint: "署名は同じアドレスの末尾に .sig を付けた場所から取得します（https のみ）。",
  mrAuto: "毎日自動確認", mrCheckNow: "今すぐ確認",
  mrUpload: "手動アップロード", mrUploadHint: "オフライン環境では bundle.json と bundle.json.sig をアップロードしてください。",
  mrBundleFile: "バンドル (bundle.json)", mrSigFile: "署名 (bundle.json.sig)",
  mrUploaded: "ルールバンドルを適用しました", mrUpToDate: "確認しました（新しいバージョンがあれば適用されます）", mrSaved: "更新設定を保存しました",
};
