import type { LocationsKey } from "../locations.i18n";

export const locationsStrings: Record<LocationsKey, string> = {
  secLocations: "Custom locations",
  locationsDesc: "Send paths to another upstream. Each location keeps the host's protections (security rules, rate limit, Basic auth, access queue). A location / replaces the forward target.",
  locationsCount: "{n} locations",
  addLocation: "Add location",
  locPath: "Path",
  locForwardPath: "Forward path",
  locForwardPathHint: "Optional. Replaces the matched path when forwarding (e.g. /api → /v2/).",
  locAdvanced: "Advanced config (directives only, administrators)",
};
