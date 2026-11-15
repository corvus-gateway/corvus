import type { CertReplaceKey } from "../certreplace.i18n";

export const certReplaceStrings: Record<CertReplaceKey, string> = {
  replaceCertAction: "Remplacer",
  replaceCertTitle: "Remplacer le certificat",
  replaceCertDesc: "Collez le certificat renouvelé et la clé privée. Le certificat garde son ID et ses hôtes ; les hôtes qui l'utilisent sont redéployés.",
  certReplaced: "Certificat remplacé",
  renewFailed: "Échec du renouvellement",
  renewFailedTip: "Le renouvellement a échoué ; le certificat actuel est utilisé jusqu'à son expiration. Erreur : {e}",
};
