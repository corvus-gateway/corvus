import type { K } from "../backup.i18n";

export const backupStrings: Record<K, string> = {
  backupTitle: "Sicherung & Wiederherstellung",
  backupDesc: "Einstellungsdatenbank, Zertifikate und Geheimnisse als ein Archiv herunterladen oder aus einer Sicherung wiederherstellen.",
  backupSecretsTitle: "Die Sicherung enthält Geheimnisse",
  backupSecretsWarn: "Sie enthält die privaten Schlüssel der Zertifikate und den Schlüssel, mit dem Geheimnisse in der Datenbank entschlüsselt werden. Behandeln Sie sie wie ein Passwort: verschlüsselt und zugriffsbeschränkt aufbewahren.",
  backupPgNote: "Diese Installation nutzt PostgreSQL: Die Sicherung enthält nur Zertifikate und Geheimnisse. Sichern Sie die Datenbank mit pg_dump.",
  backupExcluded: "Nicht enthalten: erzeugte Konfigurationsversionen, Traffic-Verlauf, Request-Logs und die Länderdatenbank (werden neu erzeugt bzw. neu heruntergeladen).",
  backupDownload: "Sicherung herunterladen", backupStarted: "Sicherung wird erstellt – der Download beginnt gleich.",
  restoreTitle: "Aus einer Sicherung wiederherstellen",
  restoreHow: "Das hochgeladene Archiv wird geprüft und vorgemerkt. Beim nächsten Neustart ersetzt es die aktuellen Einstellungen, Zertifikate und Geheimnisse; die bisherigen Dateien bleiben in restore-backup-<Zeit>/ im Datenverzeichnis erhalten.",
  restoreChoose: "Sicherungsdatei wählen", restoreUpload: "Hochladen und prüfen", restoreChecked: "Sicherung geprüft",
  restoreVersion: "Corvus-Version", restoreCreated: "Erstellt", restoreEngine: "Datenbank", restoreFiles: "Dateien",
  restoreCerts: "Zertifikate", restoreSecrets: "Geheimnisse",
  restoreNoDb: "Diese Sicherung enthält keine Datenbank (PostgreSQL): Stellen Sie die Datenbank mit pg_restore wieder her.",
  restoreVersionDiffers: "Die Sicherung wurde mit Corvus {v} erstellt (aktuell {cur}). Die Datenbank wird beim Start bei Bedarf migriert.",
  restoreConfirm: "Beim nächsten Neustart wiederherstellen",
  restoreConfirmWarn: "Alle aktuellen Einstellungen, Zertifikate und Geheimnisse werden durch die Sicherung ersetzt. Änderungen bis zum Neustart gehen verloren.",
  restoreCancel: "Wiederherstellung abbrechen", restoreStaged: "Wiederherstellung vorgemerkt", restoreDiscarded: "Wiederherstellung abgebrochen",
  restorePending: "Wiederherstellung bereit – starten Sie den Container neu, um abzuschließen",
  restorePendingDesc: "Führen Sie diesen Befehl im Verzeichnis mit der docker-compose.yml aus. Bis dahin läuft Corvus mit der aktuellen Konfiguration weiter.",
};
