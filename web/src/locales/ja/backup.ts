import type { K } from "../backup.i18n";

export const backupStrings: Record<K, string> = {
  backupTitle: "バックアップと復元",
  backupDesc: "設定データベース・証明書・シークレットを 1 つのアーカイブとしてダウンロードするか、バックアップから復元します。",
  backupSecretsTitle: "バックアップには機密情報が含まれます",
  backupSecretsWarn: "証明書の秘密鍵と、データベース内のシークレットを復号する鍵が含まれます。パスワードと同様に、暗号化しアクセスを制限して保管してください。",
  backupPgNote: "このインストールは PostgreSQL を使用しているため、バックアップには証明書とシークレットのみが含まれます。データベースは pg_dump で別途バックアップしてください。",
  backupExcluded: "含まれないもの: 生成された設定バージョン、トラフィック履歴、リクエストログ、国データベース（再生成または再ダウンロードされます）。",
  backupDownload: "バックアップをダウンロード", backupStarted: "バックアップを作成しています。まもなくダウンロードが始まります。",
  restoreTitle: "バックアップから復元",
  restoreHow: "アップロードしたアーカイブを検査して待機状態にします。次回の再起動時に現在の設定・証明書・シークレットを置き換え、既存のファイルはデータディレクトリの restore-backup-<時刻>/ に保管されます。",
  restoreChoose: "バックアップファイルを選択", restoreUpload: "アップロードして検査", restoreChecked: "バックアップを検査しました",
  restoreVersion: "Corvus バージョン", restoreCreated: "作成日時", restoreEngine: "データベース", restoreFiles: "ファイル",
  restoreCerts: "証明書", restoreSecrets: "シークレット",
  restoreNoDb: "このバックアップにはデータベースがありません（PostgreSQL）。データベースは pg_restore で復元してください。",
  restoreVersionDiffers: "このバックアップは Corvus {v} で作成されました（現在は {cur}）。必要に応じて起動時にデータベースが移行されます。",
  restoreConfirm: "次回の再起動時に復元",
  restoreConfirmWarn: "現在の設定・証明書・シークレットはすべてバックアップの内容に置き換えられます。今から再起動までの変更は失われます。",
  restoreCancel: "復元を取り消す", restoreStaged: "復元を予約しました", restoreDiscarded: "復元を取り消しました",
  restorePending: "復元の準備ができました — コンテナを再起動すると完了します",
  restorePendingDesc: "docker-compose.yml のあるディレクトリで次のコマンドを実行してください。それまでは現在の設定で動作し続けます。",
};
