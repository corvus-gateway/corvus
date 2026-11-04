import {
  Activity, Archive, ArrowLeftRight, Ban, Bell, FileInput, FileWarning, Fingerprint, Gauge, Globe, KeySquare, LayoutDashboard, ListChecks, ShieldBan, Network, ScrollText, ShieldCheck, Signpost, UserCog, Users, UsersRound, type LucideIcon,
} from "lucide-react";
import type { Key } from "../../i18n";

export interface NavItem { to: string; label: Key; icon: LucideIcon; adminOnly?: boolean }
export interface NavGroup { label: Key; items: NavItem[] }

export const groups: NavGroup[] = [
  { label: "navGroupOverview", items: [{ to: "/", label: "navDashboard", icon: LayoutDashboard }] },
  {
    label: "navGroupTraffic", items: [
      { to: "/proxy-hosts", label: "navHosts", icon: Globe },
      { to: "/redirection-hosts", label: "navRedirects", icon: Signpost },
      { to: "/waitrooms", label: "navRooms", icon: Users },
      { to: "/traffic", label: "navTraffic", icon: Activity },
      { to: "/rate-limits", label: "navRateLimits", icon: Gauge },
    ],
  },
  {
    label: "navGroupSecurity", items: [
      { to: "/security-rules", label: "navSecurityRules", icon: ShieldBan },
      { to: "/ip-lists", label: "navIpLists", icon: ListChecks },
      { to: "/bans", label: "navBans", icon: Ban },
      { to: "/certificates", label: "navCerts", icon: ShieldCheck },
      { to: "/network", label: "navNetwork", icon: Network },
      { to: "/settings/pages", label: "navPages", icon: FileWarning },
    ],
  },
  {
    label: "navGroupSystem", items: [
      { to: "/deployments", label: "navDeploy", icon: ArrowLeftRight },
      { to: "/audit", label: "navAudit", icon: ScrollText },
      { to: "/alerts", label: "navAlerts", icon: Bell },
      { to: "/users", label: "navUsers", icon: UsersRound, adminOnly: true },
      { to: "/settings/sso", label: "navSso", icon: Fingerprint, adminOnly: true },
      { to: "/settings/tokens", label: "navApiTokens", icon: KeySquare, adminOnly: true },
      { to: "/settings/retention", label: "navRetention", icon: Archive, adminOnly: true },
      { to: "/import", label: "navImport", icon: FileInput },
      { to: "/account", label: "navAccount", icon: UserCog },
    ],
  },
];

export const flat = groups.flatMap((g) => g.items.map((i) => ({ ...i, group: g.label })));
export const findNav = (pathname: string) => flat.find((i) => (i.to === "/" ? pathname === "/" : pathname.startsWith(i.to)));
