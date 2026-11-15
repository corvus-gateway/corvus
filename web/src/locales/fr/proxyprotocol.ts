import type { ProxyProtocolKey } from "../proxyprotocol.i18n";

export const proxyProtocolStrings: Record<ProxyProtocolKey, string> = {
  ppTitle: "Accepter le PROXY protocol",
  ppDesc: "À activer lorsqu'un tunnel TCP ou un répartiteur de charge (FRP, HAProxy) transmet l'IP du client dans un en-tête PROXY protocol (v1/v2). Les ports 80/443 restent inchangés ; des ports distincts acceptent l'en-tête.",
  ppHttpPort: "Port HTTP",
  ppHttpsPort: "Port HTTPS",
  ppPortInvalid: "Saisissez un port entre 1 et 65535.",
  ppTrusted: "Sources PROXY protocol de confiance",
  ppTrustedHint: "Adresses des tunnels ou répartiteurs de charge qui envoient l'en-tête. Seules les connexions venant de là utilisent l'IP client de l'en-tête. Liste distincte des proxys de confiance ci-dessus : n'y ajoutez pas le tunnel.",
  ppHint: "Faites pointer le tunnel vers ces ports et publiez-les dans docker-compose. Si le tunnel est sur le même hôte, publiez-les uniquement sur la boucle locale (par ex. '127.0.0.1:8443:8443') ; sinon n'autorisez que l'adresse du tunnel avec un pare-feu.",
};
