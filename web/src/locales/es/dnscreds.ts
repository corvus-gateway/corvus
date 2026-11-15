import type { DnsCredsKey } from "../dnscreds.i18n";

export const dnsCredsStrings: Record<DnsCredsKey, string> = {
  dnsCredsTitle: "Credenciales DNS",
  dnsCredsDesc: "Tokens de API de proveedores DNS para certificados DNS-01. Se guardan una vez y los comparten los certificados y sus renovaciones. Los secretos se guardan cifrados y no se vuelven a mostrar.",
  dnsCredsEmpty: "Aún no hay credenciales guardadas. Las credenciales que introduzcas al solicitar un certificado DNS-01 se guardan aquí.",
  dnsCredAdd: "Añadir credencial",
  dnsCredAddDesc: "Guarda una vez un proveedor DNS y un token de API y luego solo tendrás que elegirlos al solicitar certificados DNS-01.",
  dnsCredUsedBy: "Certificados",
  dnsCredSelect: "Credencial DNS",
  dnsCredNew: "Introducir credenciales nuevas",
  dnsCredName: "Nombre de la credencial",
  dnsCredNameHint: "Se guarda con este nombre para que el siguiente certificado pueda elegirla. Déjalo vacío para un nombre automático.",
  dnsCredReplace: "Reemplazar secreto",
  dnsCredReplaceDesc: "Introduce las credenciales nuevas. Los certificados que usan esta credencial las usarán desde su próxima renovación; ahora no se vuelve a emitir nada.",
  dnsCredRename: "Cambiar nombre",
  dnsCredSaved: "Credencial guardada",
  dnsCredDeleted: "Credencial eliminada",
  dnsCredDeleteTitle: "¿Eliminar la credencial DNS?",
  dnsCredDeleteDesc: "Se elimina el secreto guardado. No se puede deshacer.",
  dnsCredInUse: "La usan certificados, así que no se puede eliminar",
};
