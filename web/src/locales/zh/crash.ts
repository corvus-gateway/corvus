import type { CrashKey } from "../crash.i18n";

export const crashStrings: Record<CrashKey, string> = {
  crashTitle: "无法显示此页面",
  crashDesc: "绘制此页面时出错。其他菜单仍可使用。如果刷新后仍然出现，请报告下面的详细信息。",
  crashUpdated: "控制台已更新到新版本。刷新后即可继续使用。",
  crashReload: "刷新",
  serverDownTitle: "无法连接到服务器",
  serverDownDesc: "Corvus 可能正在重启，或者网络连接已断开。恢复后此页面会自动继续。",
  serverDownRetry: "立即重试",
};
