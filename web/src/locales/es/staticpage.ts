import type { StaticPageKey } from "../staticpage.i18n";

export const staticPageStrings: Record<StaticPageKey, string> = {
  spContent: "Contenido",
  spForward: "Reenviar a un servicio",
  spPage: "Mostrar una página",
  spPageDesc: "La pasarela responde ella misma con una página, sin backend. Las reglas de seguridad, la autenticación Basic, los límites de tasa, HTTPS, el registro de accesos y las estadísticas siguen aplicándose; las ubicaciones personalizadas siguen reenviando a sus backends.",
  spTemplate: "Plantilla",
  spTplParked: "Dominio aparcado",
  spTplComingSoon: "Próximamente",
  spTplMaintenance: "Mantenimiento",
  spTplCustom: "HTML propio",
  spStatus: "Estado HTTP",
  spStatusHint: "Para mantenimiento lo habitual es 503: los buscadores vuelven más tarde en lugar de indexar la página. Las respuestas distintas de 200 nunca se almacenan en caché.",
  spTitle: "Título",
  spMessage: "Mensaje",
  spTextHint: "Déjelo vacío para el texto predeterminado, mostrado en el idioma del navegador del visitante. Su texto se muestra tal cual (sin HTML).",
  spHtmlHint: "Se envía exactamente como se introduce. Incluya imágenes y CSS en la página o cárguelos desde otro sitio.",
  spPreviewHint: "Vista previa en el idioma de la consola; los visitantes ven los textos predeterminados en el idioma de su navegador.",
  spPreviewFailed: "No se pudo mostrar la vista previa",
  spListPage: "Página: {name}",
  spHtmlNotLoaded: "No se pudo cargar el HTML de la página. Se mantiene tal como está guardado salvo que introduzca aquí un HTML nuevo.",
};
