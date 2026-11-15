import type { LocationsKey } from "../locations.i18n";

export const locationsStrings: Record<LocationsKey, string> = {
  secLocations: "Emplacements personnalisés",
  locationsDesc: "Envoie des chemins vers un autre upstream. Chaque emplacement conserve les protections de l'hôte (règles de sécurité, limite de débit, authentification Basic, file d'accès). L'emplacement / remplace la cible de transfert.",
  locationsCount: "{n} emplacements",
  addLocation: "Ajouter un emplacement",
  locPath: "Chemin",
  locForwardPath: "Chemin de destination",
  locForwardPathHint: "Facultatif. Remplace le chemin correspondant lors du transfert (ex. /api → /v2/).",
  locAdvanced: "Configuration avancée (directives uniquement, administrateurs)",
};
