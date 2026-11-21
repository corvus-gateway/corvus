import type { StaticPageKey } from "../staticpage.i18n";

export const staticPageStrings: Record<StaticPageKey, string> = {
  spContent: "Inhalt",
  spForward: "An einen Dienst weiterleiten",
  spPage: "Eine Seite anzeigen",
  spPageDesc: "Das Gateway antwortet selbst mit einer Seite, ohne Backend. Sicherheitsregeln, Basic Auth, Ratenbegrenzung, HTTPS, Zugriffsprotokoll und Statistiken gelten weiterhin; eigene Locations leiten weiter an ihre Backends weiter.",
  spTemplate: "Vorlage",
  spTplParked: "Geparkte Domain",
  spTplComingSoon: "Demnächst verfügbar",
  spTplMaintenance: "Wartung",
  spTplCustom: "Eigenes HTML",
  spStatus: "HTTP-Status",
  spStatusHint: "Für Wartungsarbeiten ist 503 üblich: Suchmaschinen kommen später wieder, statt die Seite zu indexieren. Antworten außer 200 werden nie zwischengespeichert.",
  spTitle: "Titel",
  spMessage: "Nachricht",
  spTextHint: "Leer lassen für den Standardtext in der Browsersprache der Besucher. Ihr Text wird wie eingegeben angezeigt (ohne HTML).",
  spHtmlHint: "Wird genau wie eingegeben ausgeliefert. Bilder und CSS in die Seite einbetten oder von einer anderen Website laden.",
  spPreviewHint: "Vorschau in der Sprache der Konsole; Besucher sehen die Standardtexte in ihrer Browsersprache.",
  spPreviewFailed: "Die Vorschau konnte nicht angezeigt werden",
  spListPage: "Seite: {name}",
};
