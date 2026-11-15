import type { K } from "../pages.i18n";

export const pagesStrings: Record<K, string> = {
  navPages: "Páginas de bloqueo y error",
  pagesDesc: "Personaliza la página que se muestra cuando una regla de seguridad, un bloqueo de IP o el bloqueo de ataques rechaza una solicitud, y la que se muestra cuando no se puede contactar con un backend (502/503/504). Al guardar se despliega de inmediato en la pasarela.",
  blockPage: "Página de bloqueo", errorPage: "Página de error",
  blockPageDesc: "Se muestra cuando una solicitud es rechazada por una regla de seguridad, un bloqueo automático o manual, o el bloqueo de patrones de ataque.",
  errorPageDesc: "Se muestra cuando el backend de un host proxy no responde (conexión rechazada, tiempo agotado) o envía una respuesta no válida. Las respuestas de error que envía el propio backend se transmiten sin cambios.",
  pageCustom: "Personalizada", pageBuiltin: "Integrada",
  pageHtml: "HTML", pageBytes: "{n} / {max} KB", pageTooLarge: "La página es demasiado grande (máx. {max} KB)",
  pagePlaceholders: "Marcadores",
  pagePlaceholdersHint: "Se sustituyen por los valores reales al servir la página. Los valores se escapan para HTML.",
  pageVarStatus: "Código de estado", pageVarRid: "ID de solicitud", pageVarHost: "Sitio solicitado (host)", pageVarIp: "IP del visitante", pageVarTime: "Hora",
  pagePreview: "Vista previa", pagePreviewHint: "Se muestra con valores de ejemplo. Los scripts no se ejecutan en la vista previa (incluida la traducción automática de las páginas integradas).",
  pageReset: "Restablecer predeterminada", pageResetTitle: "¿Restaurar la página integrada?",
  pageResetDesc: "La página personalizada se elimina y la integrada se despliega de inmediato.", pageResetDone: "Página integrada restaurada",
  unknownHosts: "Hosts desconocidos",
  unknownHostsDesc: "Qué responde Corvus a una solicitud HTTP para un nombre de host que no sirve ningún host proxy ni host de redirección. Las conexiones HTTPS para esos nombres siempre se rechazan durante el handshake TLS.",
  unknownMode: "Responder con",
  unknownNotFound: "Página no encontrada (404)", unknownClose: "Cerrar la conexión", unknownRedirect: "Redirigir a una URL",
  unknownRedirectUrl: "URL de redirección",
  unknownRedirectHint: "Una URL completa que empiece por http:// o https://. Se responde con 302; no se añade la ruta solicitada.",
  importUnknownHosts: "Aplicar a los hosts desconocidos el sitio predeterminado de NPM: {mode}",
};
