import type { LocationsKey } from "../locations.i18n";

export const locationsStrings: Record<LocationsKey, string> = {
  secLocations: "Пользовательские пути",
  locationsDesc: "Направляет пути на другой upstream. Для каждого пути действует защита хоста (правила безопасности, лимит запросов, Basic-аутентификация, очередь доступа). Путь / заменяет цель перенаправления.",
  locationsCount: "Путей: {n}",
  addLocation: "Добавить путь",
  locPath: "Путь",
  locForwardPath: "Путь назначения",
  locForwardPathHint: "Необязательно. Заменяет совпавший путь при перенаправлении (например, /api → /v2/).",
  locAdvanced: "Расширенная конфигурация (только директивы, для администраторов)",
};
