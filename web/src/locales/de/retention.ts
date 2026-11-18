import type { K } from "../retention.i18n";

export const retentionStrings: Record<K, string> = {
  navRetention: "Datenaufbewahrung",
  retentionDesc: "Legen Sie fest, wie lange und wie viel Traffic-Statistik, Sicherheitsereignisse und Anfrageprotokolle aufbewahrt werden. Daten über den Grenzen werden stündlich automatisch bereinigt.",
  retentionHow: "Statistiken und Ereignisse bleiben nach einem Neustart erhalten. Anfrageprotokolle werden pro Tag in eine Datei geschrieben, frühere Tage werden komprimiert. Bei Überschreitung der Größengrenze werden die ältesten Tagesdateien zuerst gelöscht.",
  logMode: "Anfrageprotokoll",
  logModeOff: "Aus", logModeNotable: "Nur relevante Anfragen", logModeAll: "Alle Anfragen",
  logModeDesc: "Relevant: 4xx/5xx-Antworten, Blockierungen, Sicherheitstreffer und Wartezeiten in der Zugriffswarteschlange. „Alle Anfragen“ kann bei viel Traffic mehrere GB pro Tag erreichen.",
  logDays: "Protokolle aufbewahren (Tage)", logMaxMb: "Größengrenze der Protokolle (MB)", logMaxHint: "Bei Überschreitung werden die ältesten Tagesdateien zuerst gelöscht.",
  statsDays: "Statistik aufbewahren (Tage)", eventsDays: "Ereignisse aufbewahren (Tage)",
  storageUsage: "Speicher", dbSize: "Statistik- und Ereignisdatenbank", logSize: "Anfrageprotokolle", logFilesN: "{n} Dateien", logLimit: "max. {mb} MB",
  droppedLines: "{n} Protokollzeilen verworfen, weil die Festplatte nicht nachkam",
  archives: "Anfrageprotokoll-Dateien", noArchives: "Keine Protokolldateien", noArchivesDesc: "Bei aktiviertem Anfrageprotokoll erscheint hier eine Datei pro Tag.",
  fileName: "Datei", fileSize: "Größe", fileModified: "Geändert", retentionSaved: "Aufbewahrungseinstellungen gespeichert",
  logModeAllHint: "„Alle Anfragen“ entspricht den Zugriffsprotokollen pro Host in Nginx Proxy Manager und wird für Besucherstatistiken (Herkunft, UTM-Kampagnen) benötigt.",
  referrer: "Herkunft (Referrer)", combinedHint: "Im nginx-Format „combined“ herunterladen (für Log-Analyse-Tools wie GoAccess)",
};
