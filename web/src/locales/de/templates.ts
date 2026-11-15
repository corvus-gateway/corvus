import type { K } from "../templates.i18n";

export const templatesStrings: Record<K, string> = {
  fromTemplate: "Aus Vorlage hinzufügen",
  templatesTitle: "Vorlagen für virtuelle Patches",
  templatesDesc: "Regeln, die bekannte CVEs blockieren, bis Sie patchen können. Wenden Sie sie zuerst als „Protokoll“ an, prüfen Sie auf Fehlalarme und wechseln Sie dann zu „Blockieren“.",
  tplSevCritical: "Kritisch", tplSevHigh: "Hoch",
  tplProducts: "Betrifft", tplFalsePositives: "Fehlalarme",
  tplExemptList: "Ausgenommene IP-Liste (Admin-Netz)", tplExemptNone: "Keine Ausnahme",
  tplExemptHint: "Diese Vorlage kann Ihr Admin-Netz ausnehmen. Ohne Angabe gilt die Regel für alle IPs.",
  tplApply: "Regel erstellen", tplCreated: "Regel aus Vorlage erstellt", tplSelect: "Vorlage auswählen",
  managedRules: "Verwaltete Regeln",
  managedRulesDesc: "Paket aus Body-Prüfungssignaturen und Vorlagen für virtuelle Patches. Nur neuere, gültig signierte Versionen werden übernommen.",
  mrVersion: "Version", mrSource: "Quelle", mrBuiltin: "Integriert", mrInstalled: "Aktualisiert", mrSignatures: "Signaturen", mrTemplates: "Vorlagen", mrChecked: "Letzte Prüfung",
  mrUrl: "Update-URL", mrUrlHint: "Die Signatur wird von derselben Adresse mit angehängtem .sig geladen (nur https).",
  mrAuto: "Täglich prüfen", mrCheckNow: "Jetzt prüfen",
  mrUpload: "Hochladen", mrUploadHint: "Für Offline-Installationen bundle.json und bundle.json.sig hochladen.",
  mrBundleFile: "Paket (bundle.json)", mrSigFile: "Signatur (bundle.json.sig)",
  mrUploaded: "Regelpaket übernommen", mrUpToDate: "Geprüft (eine neuere Version wird bei Verfügbarkeit übernommen)", mrSaved: "Update-Einstellungen gespeichert",
};
