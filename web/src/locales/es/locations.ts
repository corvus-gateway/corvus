import type { LocationsKey } from "../locations.i18n";

export const locationsStrings: Record<LocationsKey, string> = {
  secLocations: "Ubicaciones personalizadas",
  locationsDesc: "Envía rutas a otro upstream. Cada ubicación mantiene las protecciones del host (reglas de seguridad, límite de tasa, autenticación Basic, cola de acceso). La ubicación / reemplaza el destino de reenvío.",
  locationsCount: "{n} ubicaciones",
  addLocation: "Añadir ubicación",
  locPath: "Ruta",
  locForwardPath: "Ruta de destino",
  locForwardPathHint: "Opcional. Reemplaza la ruta coincidente al reenviar (p. ej., /api → /v2/).",
  locAdvanced: "Configuración avanzada (solo directivas, administradores)",
};
