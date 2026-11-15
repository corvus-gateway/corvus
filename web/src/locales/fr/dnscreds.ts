import type { DnsCredsKey } from "../dnscreds.i18n";

export const dnsCredsStrings: Record<DnsCredsKey, string> = {
  dnsCredsTitle: "Identifiants DNS",
  dnsCredsDesc: "Jetons d'API des fournisseurs DNS pour les certificats DNS-01. Enregistrés une fois, ils sont partagés par les certificats et leurs renouvellements. Les secrets sont chiffrés et jamais réaffichés.",
  dnsCredsEmpty: "Aucun identifiant enregistré. Les identifiants saisis lors d'une demande de certificat DNS-01 sont enregistrés ici.",
  dnsCredAdd: "Ajouter des identifiants",
  dnsCredAddDesc: "Enregistrez une fois un fournisseur DNS et un jeton d'API, puis choisissez-les simplement lors des demandes de certificats DNS-01.",
  dnsCredUsedBy: "Certificats",
  dnsCredSelect: "Identifiants DNS",
  dnsCredNew: "Saisir de nouveaux identifiants",
  dnsCredName: "Nom des identifiants",
  dnsCredNameHint: "Enregistrés sous ce nom pour que le prochain certificat puisse les choisir. Laissez vide pour un nom automatique.",
  dnsCredReplace: "Remplacer le secret",
  dnsCredReplaceDesc: "Saisissez les nouveaux identifiants. Les certificats qui les utilisent s'en serviront dès leur prochain renouvellement ; rien n'est réémis maintenant.",
  dnsCredRename: "Renommer",
  dnsCredSaved: "Identifiants enregistrés",
  dnsCredDeleted: "Identifiants supprimés",
  dnsCredDeleteTitle: "Supprimer les identifiants DNS ?",
  dnsCredDeleteDesc: "Le secret enregistré est supprimé. Cette action est irréversible.",
  dnsCredInUse: "Utilisés par des certificats, ils ne peuvent pas être supprimés",
};
