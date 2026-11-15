import type { ConsoleHostKey } from "../consolehost.i18n";

export const consoleHostStrings: Record<ConsoleHostKey, string> = {
  consoleHostTitle: "Sie nutzen die Konsole über diesen Host",
  consoleHostEdit: "Eine falsche Änderung kann Sie unter dieser Adresse aus der Konsole aussperren. Öffnen Sie dann Port 9443 des Servers direkt (https://server-ip:9443).",
  consoleHostDisableTitle: "Host deaktivieren, den die Konsole nutzt?",
  consoleHostDisable: "Nach dem Deaktivieren ist die Konsole unter dieser Adresse nicht mehr erreichbar. Prüfen Sie zuerst, ob Port 9443 des Servers direkt erreichbar ist (https://server-ip:9443).",
  consoleHostDelete: "Sie nutzen die Konsole über diesen Host. Nach dem Löschen ist die Konsole unter dieser Adresse nicht mehr erreichbar. Prüfen Sie zuerst, ob Port 9443 des Servers direkt erreichbar ist (https://server-ip:9443).",
  consoleHostBadge: "Konsole",
  disable: "Deaktivieren",
};
