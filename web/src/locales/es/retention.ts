import type { K } from "../retention.i18n";

export const retentionStrings: Record<K, string> = {
  navRetention: "Retención de datos",
  retentionDesc: "Defina durante cuánto tiempo y cuánto se conservan las estadísticas de tráfico, los eventos de seguridad y los registros de solicitudes. Los datos que superan los límites se limpian automáticamente cada hora.",
  retentionHow: "Las estadísticas y los eventos se conservan tras reiniciar. Los registros de solicitudes se escriben en un archivo por día y los días anteriores se comprimen. Si se supera el límite de tamaño, se eliminan primero los archivos más antiguos.",
  logMode: "Registro de solicitudes",
  logModeOff: "Desactivado", logModeNotable: "Solo solicitudes relevantes", logModeAll: "Todas las solicitudes",
  logModeDesc: "Relevantes: respuestas 4xx/5xx, bloqueos, detecciones de seguridad y esperas en la cola de acceso. «Todas las solicitudes» puede alcanzar varios GB al día en sitios con mucho tráfico.",
  logDays: "Conservar registros (días)", logMaxMb: "Límite de tamaño de registros (MB)", logMaxHint: "Al superarlo, se eliminan primero los archivos de los días más antiguos.",
  statsDays: "Conservar estadísticas (días)", eventsDays: "Conservar eventos (días)",
  storageUsage: "Almacenamiento", dbSize: "Base de datos de estadísticas y eventos", logSize: "Registros de solicitudes", logFilesN: "{n} archivos", logLimit: "máx. {mb} MB",
  droppedLines: "{n} líneas de registro descartadas porque el disco no daba abasto",
  archives: "Archivos de registro de solicitudes", noArchives: "No hay archivos de registro", noArchivesDesc: "Con el registro de solicitudes activado, aquí aparece un archivo por día.",
  fileName: "Archivo", fileSize: "Tamaño", fileModified: "Modificado", retentionSaved: "Configuración de retención guardada",
};
