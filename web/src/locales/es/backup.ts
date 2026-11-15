import type { K } from "../backup.i18n";

export const backupStrings: Record<K, string> = {
  backupTitle: "Copia de seguridad y restauración",
  backupDesc: "Descarga la base de configuración, los certificados y los secretos en un solo archivo, o restaura desde una copia.",
  backupSecretsTitle: "La copia de seguridad contiene secretos",
  backupSecretsWarn: "Incluye las claves privadas de los certificados y la clave que descifra los secretos de la base de datos. Trátala como una contraseña: guárdala cifrada y con acceso restringido.",
  backupPgNote: "Esta instalación usa PostgreSQL: la copia solo contiene certificados y secretos. Haz copia de la base de datos con pg_dump.",
  backupExcluded: "No incluido: versiones de configuración generadas, historial de tráfico, registros de solicitudes y la base de países (se regeneran o se vuelven a descargar).",
  backupDownload: "Descargar copia", backupStarted: "Creando la copia; la descarga empezará en breve.",
  restoreTitle: "Restaurar desde una copia",
  restoreHow: "El archivo subido se comprueba y queda preparado. En el próximo reinicio sustituye la configuración, los certificados y los secretos actuales; los archivos actuales se guardan en restore-backup-<hora>/ del directorio de datos.",
  restoreChoose: "Elegir archivo de copia", restoreUpload: "Subir y comprobar", restoreChecked: "Copia comprobada",
  restoreVersion: "Versión de Corvus", restoreCreated: "Creada", restoreEngine: "Base de datos", restoreFiles: "Archivos",
  restoreCerts: "Certificados", restoreSecrets: "Secretos",
  restoreNoDb: "Esta copia no contiene base de datos (PostgreSQL): restáurala con pg_restore.",
  restoreVersionDiffers: "La copia se creó con Corvus {v} (esta es {cur}). La base de datos se migra al arrancar si hace falta.",
  restoreConfirm: "Restaurar en el próximo reinicio",
  restoreConfirmWarn: "Toda la configuración, los certificados y los secretos actuales se sustituirán por la copia. Los cambios hechos hasta el reinicio se perderán.",
  restoreCancel: "Cancelar restauración", restoreStaged: "Restauración programada", restoreDiscarded: "Restauración cancelada",
  restorePending: "Restauración lista: reinicia el contenedor para terminar",
  restorePendingDesc: "Ejecuta este comando en el directorio de docker-compose.yml. Hasta entonces Corvus sigue funcionando con la configuración actual.",
};
