import type { CrashKey } from "../crash.i18n";

export const crashStrings: Record<CrashKey, string> = {
  crashTitle: "No se pudo mostrar esta página",
  crashDesc: "Se produjo un error al mostrar esta página. Los demás menús siguen funcionando. Si vuelve a ocurrir tras recargar, comunique los detalles de abajo.",
  crashUpdated: "La consola se actualizó a una nueva versión. Recargue la página para continuar.",
  crashReload: "Recargar",
  serverDownTitle: "No se puede conectar con el servidor",
  serverDownDesc: "Es posible que Corvus se esté reiniciando o que se haya perdido la conexión de red. Esta página continúa sola en cuanto vuelva.",
  serverDownRetry: "Reintentar ahora",
};
