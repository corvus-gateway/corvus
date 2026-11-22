import type { CrashKey } from "../crash.i18n";

export const crashStrings: Record<CrashKey, string> = {
  crashTitle: "Diese Seite konnte nicht angezeigt werden",
  crashDesc: "Beim Aufbau dieser Seite ist ein Fehler aufgetreten. Die anderen Menüs funktionieren weiterhin. Tritt der Fehler nach dem Neuladen erneut auf, melden Sie bitte die Details unten.",
  crashUpdated: "Die Konsole wurde auf eine neue Version aktualisiert. Laden Sie die Seite neu, um fortzufahren.",
  crashReload: "Neu laden",
  serverDownTitle: "Server nicht erreichbar",
  serverDownDesc: "Corvus startet möglicherweise neu, oder die Netzwerkverbindung wurde unterbrochen. Diese Seite macht von selbst weiter, sobald der Server wieder da ist.",
  serverDownRetry: "Jetzt erneut versuchen",
};
