import type { K } from "../pages.i18n";

export const pagesStrings: Record<K, string> = {
  navPages: "Pages de blocage et d’erreur",
  pagesDesc: "Personnalisez la page affichée lorsqu’une règle de sécurité, un bannissement ou le blocage d’attaques refuse une requête, et celle affichée lorsqu’un backend est injoignable (502/503/504). L’enregistrement déploie immédiatement sur la passerelle.",
  blockPage: "Page de blocage", errorPage: "Page d’erreur",
  blockPageDesc: "Affichée lorsqu’une requête est refusée par une règle de sécurité, un bannissement automatique ou manuel, ou le blocage des motifs d’attaque.",
  errorPageDesc: "Affichée lorsque le backend d’un hôte proxy ne répond pas (connexion refusée, délai dépassé) ou renvoie une réponse invalide. Les réponses d’erreur envoyées par le backend lui-même sont transmises telles quelles.",
  pageCustom: "Personnalisée", pageBuiltin: "Intégrée",
  pageHtml: "HTML", pageBytes: "{n} / {max} Ko", pageTooLarge: "La page est trop volumineuse (max. {max} Ko)",
  pagePlaceholders: "Variables",
  pagePlaceholdersHint: "Remplacées par les valeurs réelles lors de l’envoi de la page. Les valeurs sont échappées en HTML.",
  pageVarStatus: "Code d’état", pageVarRid: "ID de requête", pageVarHost: "Site demandé (hôte)", pageVarIp: "IP du visiteur", pageVarTime: "Heure",
  pagePreview: "Aperçu", pagePreviewHint: "Affiché avec des valeurs d’exemple. Les scripts ne s’exécutent pas dans l’aperçu (y compris la traduction automatique des pages intégrées).",
  pageReset: "Rétablir par défaut", pageResetTitle: "Rétablir la page intégrée ?",
  pageResetDesc: "La page personnalisée est supprimée et la page intégrée est déployée immédiatement.", pageResetDone: "Page intégrée rétablie",
  unknownHosts: "Hôtes inconnus",
  unknownHostsDesc: "Ce que Corvus répond à une requête HTTP pour un nom d’hôte qu’aucun hôte proxy ni hôte de redirection ne sert. Les connexions HTTPS pour ces noms sont toujours refusées lors de la négociation TLS.",
  unknownMode: "Répondre par",
  unknownNotFound: "Page introuvable (404)", unknownClose: "Fermer la connexion", unknownRedirect: "Rediriger vers une URL",
  unknownRedirectUrl: "URL de redirection",
  unknownRedirectHint: "Une URL complète commençant par http:// ou https://. Réponse 302 ; le chemin demandé n’est pas ajouté.",
  importUnknownHosts: "Appliquer le site par défaut de NPM aux hôtes inconnus : {mode}",
};
