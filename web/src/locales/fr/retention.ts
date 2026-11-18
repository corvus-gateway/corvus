import type { K } from "../retention.i18n";

export const retentionStrings: Record<K, string> = {
  navRetention: "Conservation des données",
  retentionDesc: "Définissez la durée et le volume de conservation des statistiques de trafic, des événements de sécurité et des journaux de requêtes. Les données hors limites sont nettoyées automatiquement toutes les heures.",
  retentionHow: "Les statistiques et événements sont conservés après un redémarrage. Les journaux de requêtes sont écrits dans un fichier par jour et les jours précédents sont compressés. En cas de dépassement de la taille maximale, les fichiers les plus anciens sont supprimés en premier.",
  logMode: "Journal des requêtes",
  logModeOff: "Désactivé", logModeNotable: "Requêtes notables", logModeAll: "Toutes les requêtes",
  logModeDesc: "Notables : réponses 4xx/5xx, blocages, détections de sécurité et attentes dans la file d'accès. « Toutes les requêtes » peut atteindre plusieurs Go par jour sur un site chargé.",
  logDays: "Conserver les journaux (jours)", logMaxMb: "Taille maximale des journaux (Mo)", logMaxHint: "Au-delà, les fichiers des jours les plus anciens sont supprimés en premier.",
  statsDays: "Conserver les statistiques (jours)", eventsDays: "Conserver les événements (jours)",
  storageUsage: "Stockage", dbSize: "Base statistiques et événements", logSize: "Journaux de requêtes", logFilesN: "{n} fichiers", logLimit: "max. {mb} Mo",
  droppedLines: "{n} lignes de journal perdues car le disque ne suivait pas",
  archives: "Fichiers du journal des requêtes", noArchives: "Aucun fichier journal", noArchivesDesc: "Lorsque le journal des requêtes est activé, un fichier par jour apparaît ici.",
  fileName: "Fichier", fileSize: "Taille", fileModified: "Modifié", retentionSaved: "Paramètres de conservation enregistrés",
  logModeAllHint: "« Toutes les requêtes » correspond aux journaux d'accès par hôte de Nginx Proxy Manager et est nécessaire pour les statistiques de visite (provenance, campagnes UTM).",
  referrer: "Provenance (référent)", combinedHint: "Télécharger au format combined de nginx (pour les analyseurs de journaux comme GoAccess)",
};
