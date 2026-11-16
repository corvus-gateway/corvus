import type { ConnCheckKey } from "../conncheck.i18n";

export const connCheckStrings: Record<ConnCheckKey, string> = {
  ccButton: "Tester la connexion",
  ccChecking: "Test en cours…",
  ccOk: "La passerelle a joint {addr} en {ms} ms",
  ccMapped: "Pour cet hôte, la passerelle se connecte à {target}.",
  ccDnsTitle: "Le nom ne se résout pas",
  ccDnsHint: "La passerelle recherche les hôtes de destination sous leur nom DNS complet, sans domaines de recherche. Saisissez le nom complet (par exemple nas.home.lan) ou l’adresse IP, ou ajoutez le nom à extra_hosts dans docker-compose.yml.",
  ccRefusedTitle: "Connexion refusée",
  ccRefusedHint: "L’adresse répond, mais rien n’accepte de connexion sur ce port. Vérifiez le port, que le service tourne et qu’il écoute sur 0.0.0.0 et pas seulement sur 127.0.0.1. Un pare-feu peut aussi rejeter la connexion.",
  ccTimeoutTitle: "Pas de réponse (délai dépassé)",
  ccTimeoutHint: "Aucune réponse en 5 secondes. Le plus souvent, un pare-feu sur le serveur cible ignore les connexions venant du serveur de la passerelle : autorisez-le sur ce serveur. L’adresse peut aussi être fausse ou le serveur éteint.",
  ccUnreachableTitle: "Réseau injoignable",
  ccUnreachableHint: "La passerelle n’a aucune route vers cette adresse. Vérifiez l’adresse IP. Si elle se trouve dans une plage de réseaux Docker (par exemple 172.17–172.31.x.x), un sous-réseau Docker chevauche votre réseau local : changez le sous-réseau du réseau Docker.",
  ccTlsTitle: "Échec de la négociation TLS",
  ccTlsHint: "Le port accepte les connexions mais ne parle pas HTTPS. Passez le schéma en http ou indiquez le port HTTPS du service.",
  ccOtherTitle: "Échec de la connexion",
  ccOtherHint: "Consultez le message ci-dessous et le journal de la passerelle.",
  ueBadge: "Erreur d’upstream",
  ueTip: "Échec vers l’upstream ({ago}) : {reason}",
  ueTipAction: "Modifiez l’hôte et utilisez « Tester la connexion » pour trouver la cause.",
  ueSheetTitle: "Les visiteurs ont reçu une page d’erreur ({ago})",
};
