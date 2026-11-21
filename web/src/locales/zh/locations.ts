import type { LocationsKey } from "../locations.i18n";

export const locationsStrings: Record<LocationsKey, string> = {
  secLocations: "自定义位置",
  locationsDesc: "将指定路径转发到其他上游。每个位置都保留主机的防护(安全规则、速率限制、Basic 认证、访问队列)。位置 / 将替换转发目标。",
  locationsCount: "{n} 个位置",
  locationOne: "1 个位置",
  addLocation: "添加位置",
  locPath: "路径",
  locForwardPath: "转发路径",
  locForwardPathHint: "可选。转发时用此路径替换匹配的路径(例如 /api → /v2/)。",
  locAdvanced: "高级配置(仅指令,限管理员)",
};
