import type { CertReplaceKey } from "../certreplace.i18n";

export const certReplaceStrings: Record<CertReplaceKey, string> = {
  replaceCertAction: "Ersetzen",
  replaceCertTitle: "Zertifikat ersetzen",
  replaceCertDesc: "Erneuertes Zertifikat und privaten Schlüssel einfügen. Das Zertifikat behält seine ID und Host-Zuordnungen; die Hosts, die es verwenden, werden neu bereitgestellt.",
  certReplaced: "Zertifikat ersetzt",
  renewFailed: "Erneuerung fehlgeschlagen",
  renewFailedTip: "Erneuerung fehlgeschlagen; das aktuelle Zertifikat wird bis zum Ablauf weiter verwendet. Fehler: {e}",
};
