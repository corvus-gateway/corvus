import type { K } from "../backup.i18n";

export const backupStrings: Record<K, string> = {
  backupTitle: "Backup & restore",
  backupDesc: "Download the settings database, certificates and secrets as one archive, or restore from one.",
  backupSecretsTitle: "The backup contains secrets",
  backupSecretsWarn: "It includes certificate private keys and the key that decrypts secrets in the database. Treat it like a password: store it encrypted and access-controlled.",
  backupPgNote: "This installation uses PostgreSQL: the backup contains certificates and secrets only. Back up the database with pg_dump.",
  backupExcluded: "Not included: generated configuration versions, traffic history, request logs and the country database (they are rebuilt or downloaded again).",
  backupDownload: "Download backup", backupStarted: "Creating the backup — the download starts shortly.",
  restoreTitle: "Restore from a backup",
  restoreHow: "The uploaded archive is checked and staged. On the next restart it replaces the current settings, certificates and secrets; the current files are kept in restore-backup-<time>/ in the data directory.",
  restoreChoose: "Choose backup file", restoreUpload: "Upload and check", restoreChecked: "Backup checked",
  restoreVersion: "Corvus version", restoreCreated: "Created", restoreEngine: "Database", restoreFiles: "Files",
  restoreCerts: "Certificates", restoreSecrets: "Secrets",
  restoreNoDb: "This backup has no database (PostgreSQL): restore the database with pg_restore.",
  restoreVersionDiffers: "The backup was made with Corvus {v} (this is {cur}). The database is migrated on start if needed.",
  restoreConfirm: "Restore on next restart",
  restoreConfirmWarn: "All current settings, certificates and secrets are replaced by the backup. Changes made from now until the restart are lost.",
  restoreCancel: "Cancel restore", restoreStaged: "Restore scheduled", restoreDiscarded: "Restore cancelled",
  restorePending: "Restore ready — restart the container to finish",
  restorePendingDesc: "Run this in the directory with docker-compose.yml. Until then Corvus keeps running with the current configuration.",
};
