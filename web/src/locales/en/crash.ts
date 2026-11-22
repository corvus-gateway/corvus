import type { CrashKey } from "../crash.i18n";

export const crashStrings: Record<CrashKey, string> = {
  crashTitle: "This page could not be shown",
  crashDesc: "Something went wrong while drawing this page. The other menus still work. If it happens again after reloading, please report the details below.",
  crashUpdated: "The console was updated to a new version. Reload to continue.",
  crashReload: "Reload",
  serverDownTitle: "Cannot reach the server",
  serverDownDesc: "Corvus may be restarting, or the network connection was lost. This page continues by itself once it is back.",
  serverDownRetry: "Retry now",
};
