import type { CrashKey } from "../crash.i18n";

export const crashStrings: Record<CrashKey, string> = {
  crashTitle: "Impossible d'afficher cette page",
  crashDesc: "Une erreur s'est produite lors de l'affichage de cette page. Les autres menus fonctionnent toujours. Si cela se reproduit après un rechargement, merci de signaler les détails ci-dessous.",
  crashUpdated: "La console a été mise à jour vers une nouvelle version. Rechargez la page pour continuer.",
  crashReload: "Recharger",
  serverDownTitle: "Serveur injoignable",
  serverDownDesc: "Corvus est peut-être en cours de redémarrage, ou la connexion réseau a été perdue. Cette page reprend d'elle-même dès son retour.",
  serverDownRetry: "Réessayer maintenant",
};
