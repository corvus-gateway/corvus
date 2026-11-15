import type { DnsCredsKey } from "../dnscreds.i18n";

export const dnsCredsStrings: Record<DnsCredsKey, string> = {
  dnsCredsTitle: "DNS-Zugangsdaten",
  dnsCredsDesc: "API-Tokens von DNS-Anbietern für DNS-01-Zertifikate. Einmal gespeichert, nutzen Zertifikate und ihre Verlängerungen sie gemeinsam. Geheimnisse werden verschlüsselt gespeichert und nie wieder angezeigt.",
  dnsCredsEmpty: "Noch keine gespeicherten Zugangsdaten. Beim Anfordern eines DNS-01-Zertifikats eingegebene Zugangsdaten werden hier gespeichert.",
  dnsCredAdd: "Zugangsdaten hinzufügen",
  dnsCredAddDesc: "Speichern Sie DNS-Anbieter und API-Token einmal und wählen Sie sie dann beim Anfordern von DNS-01-Zertifikaten nur noch aus.",
  dnsCredUsedBy: "Zertifikate",
  dnsCredSelect: "DNS-Zugangsdaten",
  dnsCredNew: "Neue Zugangsdaten eingeben",
  dnsCredName: "Name der Zugangsdaten",
  dnsCredNameHint: "Werden unter diesem Namen gespeichert, damit das nächste Zertifikat sie auswählen kann. Leer lassen für einen automatischen Namen.",
  dnsCredReplace: "Geheimnis ersetzen",
  dnsCredReplaceDesc: "Geben Sie die neuen Zugangsdaten ein. Zertifikate, die sie nutzen, verwenden sie ab der nächsten Verlängerung; jetzt wird nichts neu ausgestellt.",
  dnsCredRename: "Umbenennen",
  dnsCredSaved: "Zugangsdaten gespeichert",
  dnsCredDeleted: "Zugangsdaten gelöscht",
  dnsCredDeleteTitle: "DNS-Zugangsdaten löschen?",
  dnsCredDeleteDesc: "Das gespeicherte Geheimnis wird entfernt. Dies kann nicht rückgängig gemacht werden.",
  dnsCredInUse: "Von Zertifikaten verwendet und daher nicht löschbar",
};
