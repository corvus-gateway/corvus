import type { StaticPageKey } from "../staticpage.i18n";

export const staticPageStrings: Record<StaticPageKey, string> = {
  spContent: "Contenu",
  spForward: "Transférer vers un service",
  spPage: "Afficher une page",
  spPageDesc: "La passerelle répond elle-même avec une page, sans backend. Les règles de sécurité, l’authentification Basic, les limites de débit, HTTPS, le journal d’accès et les statistiques restent appliqués ; les emplacements personnalisés continuent de transférer vers leurs backends.",
  spTemplate: "Modèle",
  spTplParked: "Domaine parqué",
  spTplComingSoon: "Bientôt disponible",
  spTplMaintenance: "Maintenance",
  spTplCustom: "HTML personnalisé",
  spStatus: "Statut HTTP",
  spStatusHint: "503 est l’usage pour une maintenance : les moteurs de recherche reviennent plus tard au lieu d’indexer la page. Les réponses autres que 200 ne sont jamais mises en cache.",
  spTitle: "Titre",
  spMessage: "Message",
  spTextHint: "Laissez vide pour le texte par défaut, affiché dans la langue du navigateur du visiteur. Votre texte est affiché tel quel (sans HTML).",
  spHtmlHint: "Envoyé exactement tel que saisi. Intégrez images et CSS dans la page ou chargez-les depuis un autre site.",
  spPreviewHint: "Aperçu dans la langue de la console ; les visiteurs voient les textes par défaut dans la langue de leur navigateur.",
  spPreviewFailed: "Impossible d’afficher l’aperçu",
  spListPage: "Page : {name}",
  spHtmlNotLoaded: "Le HTML de la page n’a pas pu être chargé. Il reste tel qu’enregistré, sauf si vous saisissez ici un nouveau HTML.",
};
