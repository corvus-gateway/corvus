import type { LocationsKey } from "../locations.i18n";

export const locationsStrings: Record<LocationsKey, string> = {
  secLocations: "Eigene Locations",
  locationsDesc: "Leitet Pfade an einen anderen Upstream weiter. Jede Location behält den Schutz des Hosts (Sicherheitsregeln, Ratenlimit, Basic-Auth, Zugangswarteschlange). Die Location / ersetzt das Weiterleitungsziel.",
  locationsCount: "{n} Locations",
  locationOne: "1 Location",
  addLocation: "Location hinzufügen",
  locPath: "Pfad",
  locForwardPath: "Zielpfad",
  locForwardPathHint: "Optional. Ersetzt beim Weiterleiten den passenden Pfad (z. B. /api → /v2/).",
  locAdvanced: "Erweiterte Konfiguration (nur Direktiven, nur Administratoren)",
};
