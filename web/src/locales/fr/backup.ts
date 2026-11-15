import type { K } from "../backup.i18n";

export const backupStrings: Record<K, string> = {
  backupTitle: "Sauvegarde et restauration",
  backupDesc: "Téléchargez la base de paramètres, les certificats et les secrets dans une seule archive, ou restaurez à partir d'une sauvegarde.",
  backupSecretsTitle: "La sauvegarde contient des secrets",
  backupSecretsWarn: "Elle inclut les clés privées des certificats et la clé qui déchiffre les secrets de la base de données. Traitez-la comme un mot de passe : stockez-la chiffrée et à accès restreint.",
  backupPgNote: "Cette installation utilise PostgreSQL : la sauvegarde ne contient que les certificats et les secrets. Sauvegardez la base avec pg_dump.",
  backupExcluded: "Non inclus : versions de configuration générées, historique du trafic, journaux de requêtes et base des pays (ils sont régénérés ou retéléchargés).",
  backupDownload: "Télécharger la sauvegarde", backupStarted: "Création de la sauvegarde — le téléchargement va commencer.",
  restoreTitle: "Restaurer une sauvegarde",
  restoreHow: "L'archive envoyée est vérifiée puis mise en attente. Au prochain redémarrage, elle remplace les paramètres, certificats et secrets actuels ; les fichiers actuels sont conservés dans restore-backup-<heure>/ du répertoire de données.",
  restoreChoose: "Choisir le fichier de sauvegarde", restoreUpload: "Envoyer et vérifier", restoreChecked: "Sauvegarde vérifiée",
  restoreVersion: "Version de Corvus", restoreCreated: "Créée le", restoreEngine: "Base de données", restoreFiles: "Fichiers",
  restoreCerts: "Certificats", restoreSecrets: "Secrets",
  restoreNoDb: "Cette sauvegarde ne contient pas de base de données (PostgreSQL) : restaurez-la avec pg_restore.",
  restoreVersionDiffers: "La sauvegarde a été créée avec Corvus {v} (version actuelle : {cur}). La base est migrée au démarrage si nécessaire.",
  restoreConfirm: "Restaurer au prochain redémarrage",
  restoreConfirmWarn: "Tous les paramètres, certificats et secrets actuels seront remplacés par la sauvegarde. Les modifications faites d'ici au redémarrage seront perdues.",
  restoreCancel: "Annuler la restauration", restoreStaged: "Restauration planifiée", restoreDiscarded: "Restauration annulée",
  restorePending: "Restauration prête — redémarrez le conteneur pour terminer",
  restorePendingDesc: "Exécutez cette commande dans le répertoire contenant docker-compose.yml. D'ici là, Corvus continue avec la configuration actuelle.",
};
