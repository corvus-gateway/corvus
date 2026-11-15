import type { K } from "../templates.i18n";

export const templatesStrings: Record<K, string> = {
  fromTemplate: "Añadir desde plantilla",
  templatesTitle: "Plantillas de parches virtuales",
  templatesDesc: "Reglas que bloquean CVE conocidas hasta que pueda aplicar el parche. Aplíquelas primero como «Registro», compruebe que no hay falsos positivos y después cambie a «Bloquear».",
  tplSevCritical: "Crítica", tplSevHigh: "Alta",
  tplProducts: "Afecta a", tplFalsePositives: "Falsos positivos",
  tplExemptList: "Lista de IP exentas (red de administración)", tplExemptNone: "Sin exención",
  tplExemptHint: "Esta plantilla puede eximir su red de administración. Sin ella, la regla se aplica a todas las IP.",
  tplApply: "Crear regla", tplCreated: "Regla creada desde la plantilla", tplSelect: "Seleccione una plantilla",
  managedRules: "Reglas gestionadas",
  managedRulesDesc: "Paquete de firmas de inspección del cuerpo y plantillas de parches virtuales. Solo se aplican versiones más recientes con firma válida.",
  mrVersion: "Versión", mrSource: "Origen", mrBuiltin: "Integrado", mrInstalled: "Actualizado", mrSignatures: "Firmas", mrTemplates: "Plantillas", mrChecked: "Última comprobación",
  mrUrl: "URL de actualización", mrUrlHint: "La firma se obtiene de la misma dirección con .sig añadido (solo https).",
  mrAuto: "Comprobar a diario", mrCheckNow: "Comprobar ahora",
  mrUpload: "Subir", mrUploadHint: "En instalaciones sin conexión, suba bundle.json y bundle.json.sig.",
  mrBundleFile: "Paquete (bundle.json)", mrSigFile: "Firma (bundle.json.sig)",
  mrUploaded: "Paquete de reglas aplicado", mrUpToDate: "Comprobado (se aplica una versión más reciente si existe)", mrSaved: "Configuración de actualización guardada",
};
