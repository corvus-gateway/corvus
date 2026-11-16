import type { ConnCheckKey } from "../conncheck.i18n";

export const connCheckStrings: Record<ConnCheckKey, string> = {
  ccButton: "Verbindung prüfen",
  ccChecking: "Wird geprüft…",
  ccOk: "Das Gateway hat {addr} in {ms} ms erreicht",
  ccMapped: "Für diesen Host verbindet sich das Gateway mit {target}.",
  ccDnsTitle: "Name wird nicht aufgelöst",
  ccDnsHint: "Das Gateway fragt Ziel-Hosts als vollständige DNS-Namen ab, ohne Suchdomänen. Geben Sie den vollständigen Namen (etwa nas.home.lan) oder die IP-Adresse ein, oder tragen Sie den Namen in docker-compose.yml unter extra_hosts ein.",
  ccRefusedTitle: "Verbindung abgelehnt",
  ccRefusedHint: "Die Adresse antwortet, aber auf diesem Port nimmt nichts Verbindungen an. Prüfen Sie den Port, ob der Dienst läuft und ob er auf 0.0.0.0 statt nur auf 127.0.0.1 lauscht. Auch eine Firewall kann die Verbindung ablehnen.",
  ccTimeoutTitle: "Keine Antwort (Zeitüberschreitung)",
  ccTimeoutHint: "Innerhalb von 5 Sekunden kam keine Antwort. Meist verwirft eine Firewall auf dem Zielserver Verbindungen vom Gateway-Server – erlauben Sie den Gateway-Server dort. Die Adresse kann auch falsch oder der Server ausgeschaltet sein.",
  ccUnreachableTitle: "Netzwerk nicht erreichbar",
  ccUnreachableHint: "Das Gateway hat keine Route zu dieser Adresse. Prüfen Sie die IP-Adresse. Liegt sie in einem Docker-Netzbereich (etwa 172.17–172.31.x.x), überschneidet sich ein Docker-Subnetz mit Ihrem LAN: Ändern Sie das Subnetz des Docker-Netzwerks.",
  ccTlsTitle: "TLS-Handshake fehlgeschlagen",
  ccTlsHint: "Der Port nimmt Verbindungen an, spricht aber kein HTTPS. Stellen Sie das Schema auf http um oder geben Sie den HTTPS-Port des Dienstes an.",
  ccOtherTitle: "Verbindung fehlgeschlagen",
  ccOtherHint: "Siehe die Meldung unten und das Gateway-Protokoll.",
  ueBadge: "Upstream-Fehler",
  ueTip: "Upstream {ago} nicht erreicht: {reason}",
  ueTipAction: "Bearbeiten Sie den Host und finden Sie die Ursache mit „Verbindung prüfen“.",
  ueSheetTitle: "Besucher erhielten {ago} eine Fehlerseite",
};
