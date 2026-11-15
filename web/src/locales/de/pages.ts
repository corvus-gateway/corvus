import type { K } from "../pages.i18n";

export const pagesStrings: Record<K, string> = {
  navPages: "Sperr- & Fehlerseiten",
  pagesDesc: "Passen Sie die Seite an, die erscheint, wenn eine Sicherheitsregel, Sperre oder Angriffsmuster-Blockierung eine Anfrage abweist, sowie die Seite für nicht erreichbare Backends (502/503/504). Speichern stellt sofort auf dem Gateway bereit.",
  blockPage: "Sperrseite", errorPage: "Fehlerseite",
  blockPageDesc: "Wird angezeigt, wenn eine Anfrage durch eine Sicherheitsregel, eine automatische oder manuelle Sperre oder die Angriffsmuster-Blockierung abgewiesen wird.",
  errorPageDesc: "Wird angezeigt, wenn das Backend eines Proxy-Hosts nicht antwortet (Verbindung abgelehnt, Zeitüberschreitung) oder eine ungültige Antwort sendet. Fehlerantworten des Backends selbst werden unverändert weitergegeben.",
  pageCustom: "Eigene", pageBuiltin: "Standard",
  pageHtml: "HTML", pageBytes: "{n} / {max} KB", pageTooLarge: "Die Seite ist zu groß (max. {max} KB)",
  pagePlaceholders: "Platzhalter",
  pagePlaceholdersHint: "Werden beim Ausliefern durch die tatsächlichen Werte ersetzt. Die Werte werden HTML-maskiert.",
  pageVarStatus: "Statuscode", pageVarRid: "Anfrage-ID", pageVarHost: "Angefragte Website (Host)", pageVarIp: "IP des Besuchers", pageVarTime: "Zeit",
  pagePreview: "Vorschau", pagePreviewHint: "Mit Beispielwerten dargestellt. In der Vorschau laufen keine Skripte (auch nicht die automatische Übersetzung der Standardseiten).",
  pageReset: "Auf Standard zurücksetzen", pageResetTitle: "Standardseite wiederherstellen?",
  pageResetDesc: "Die eigene Seite wird gelöscht und die Standardseite sofort bereitgestellt.", pageResetDone: "Standardseite wiederhergestellt",
  unknownHosts: "Unbekannte Hosts",
  unknownHostsDesc: "Was Corvus auf eine HTTP-Anfrage für einen Hostnamen antwortet, den kein Proxy- oder Weiterleitungs-Host bedient. HTTPS-Verbindungen für solche Namen werden immer beim TLS-Handshake abgewiesen.",
  unknownMode: "Antwort",
  unknownNotFound: "Seite „Nicht gefunden“ (404)", unknownClose: "Verbindung schließen", unknownRedirect: "Auf eine URL weiterleiten",
  unknownRedirectUrl: "Weiterleitungs-URL",
  unknownRedirectHint: "Eine vollständige URL, die mit http:// oder https:// beginnt. Antwort mit 302; der angefragte Pfad wird nicht angehängt.",
  importUnknownHosts: "Standardseite aus NPM für unbekannte Hosts übernehmen: {mode}",
};
