import type { ConsoleHostKey } from "../consolehost.i18n";

export const consoleHostStrings: Record<ConsoleHostKey, string> = {
  consoleHostTitle: "Está usando la consola a través de este host",
  consoleHostEdit: "Un cambio incorrecto puede dejarle sin acceso a la consola en esta dirección. Si ocurre, abra directamente el puerto 9443 del servidor (https://ip-del-servidor:9443).",
  consoleHostDisableTitle: "¿Deshabilitar el host que usa la consola?",
  consoleHostDisable: "Al deshabilitar este host, la consola deja de estar disponible en esta dirección. Compruebe antes que puede abrir directamente el puerto 9443 del servidor (https://ip-del-servidor:9443).",
  consoleHostDelete: "Está usando la consola a través de este host. Si lo elimina, la consola deja de estar disponible en esta dirección. Compruebe antes que puede abrir directamente el puerto 9443 del servidor (https://ip-del-servidor:9443).",
  consoleHostBadge: "Consola",
  disable: "Deshabilitar",
};
