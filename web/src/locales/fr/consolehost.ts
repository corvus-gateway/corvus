import type { ConsoleHostKey } from "../consolehost.i18n";

export const consoleHostStrings: Record<ConsoleHostKey, string> = {
  consoleHostTitle: "Vous utilisez la console via cet hôte",
  consoleHostEdit: "Une mauvaise modification peut vous empêcher d'accéder à la console à cette adresse. Dans ce cas, ouvrez directement le port 9443 du serveur (https://ip-du-serveur:9443).",
  consoleHostDisableTitle: "Désactiver l'hôte utilisé par la console ?",
  consoleHostDisable: "Une fois cet hôte désactivé, la console n'est plus accessible à cette adresse. Vérifiez d'abord que le port 9443 du serveur s'ouvre directement (https://ip-du-serveur:9443).",
  consoleHostDelete: "Vous utilisez la console via cet hôte. Après sa suppression, la console n'est plus accessible à cette adresse. Vérifiez d'abord que le port 9443 du serveur s'ouvre directement (https://ip-du-serveur:9443).",
  consoleHostBadge: "Console",
  disable: "Désactiver",
};
