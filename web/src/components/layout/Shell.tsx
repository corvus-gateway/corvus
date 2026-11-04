import { Suspense, lazy, useEffect, useState, type ReactNode } from "react";
import { Link, NavLink, Navigate, Route, Routes, matchPath, useLocation } from "react-router-dom";
import * as Dialog from "@radix-ui/react-dialog";
import { ChevronsLeft, ChevronsRight, Languages, LogOut, Menu as MenuIcon, Monitor, Moon, Sun, UserCog } from "lucide-react";
import { api } from "../../api";
import { useApp } from "../../lib/app";
import { LANGS, LANG_NAMES, isLang, useI18n } from "../../i18n";
import { groups } from "./nav";
import { roleKey, useCan } from "../../lib/rbac";
import { useTheme, type ThemeMode } from "../../lib/theme";
import { cn } from "../../lib/utils";
import { Wordmark } from "../Logo";
import { DirtyBanner } from "../common";
import { Menu, Tip } from "../ui/overlay";
import { Skeleton } from "../ui/display";

const Dashboard = lazy(() => import("../../pages/Dashboard"));
const ProxyHosts = lazy(() => import("../../pages/ProxyHosts"));
const RedirectionHosts = lazy(() => import("../../pages/RedirectionHosts"));
const Waitrooms = lazy(() => import("../../pages/Waitrooms"));
const Traffic = lazy(() => import("../../pages/Traffic"));
const RateLimits = lazy(() => import("../../pages/RateLimits"));
const SecurityRules = lazy(() => import("../../pages/SecurityRules"));
const IpLists = lazy(() => import("../../pages/IpLists"));
const Bans = lazy(() => import("../../pages/Bans"));
const Retention = lazy(() => import("../../pages/Retention"));
const Certificates = lazy(() => import("../../pages/Certificates"));
const NetworkPage = lazy(() => import("../../pages/Network"));
const CustomPages = lazy(() => import("../../pages/CustomPages"));
const Deployments = lazy(() => import("../../pages/Deployments"));
const Audit = lazy(() => import("../../pages/Audit"));
const ImportNpm = lazy(() => import("../../pages/ImportNpm"));
const Alerts = lazy(() => import("../../pages/Alerts"));
const UsersPage = lazy(() => import("../../pages/Users"));
const SsoPage = lazy(() => import("../../pages/Sso"));
const ApiTokens = lazy(() => import("../../pages/ApiTokens"));
const Account = lazy(() => import("../../pages/Account"));

function useCollapsed() {
  const [c, setC] = useState(() => { try { return localStorage.getItem("corvus.sidebar") === "1"; } catch { return false; } });
  const set = (v: boolean) => { setC(v); try { localStorage.setItem("corvus.sidebar", v ? "1" : "0"); } catch { /* ignore */ } };
  return [c, set] as const;
}

const SIDEBAR = "bg-[#212427] text-[#d2d2d2]";

function NavList({ collapsed, onNavigate }: { collapsed: boolean; onNavigate?: () => void }) {
  const { t } = useI18n();
  const { isAdmin } = useCan();
  const { pathname } = useLocation();
  return (
    <nav aria-label="primary" className="flex-1 space-y-4 overflow-y-auto py-3">
      {groups.map((g) => ({ ...g, items: g.items.filter((i) => isAdmin || !i.adminOnly) })).map((g) => (
        <div key={g.label}>
          {!collapsed ? (
            <div className="px-4 pb-1 text-xs font-semibold text-[#8a8d90]">{t(g.label)}</div>
          ) : <div className="mx-3 mb-1 h-px bg-[#3c3f42]" />}
          <ul>
            {g.items.map((i) => {
              // Active state is computed here (not via NavLink's className function): the
              // collapsed sidebar wraps links in a tooltip trigger (asChild), which can only
              // merge plain class strings.
              const isActive = !!matchPath({ path: i.to, end: i.to === "/" }, pathname);
              const link = (
                <NavLink to={i.to} end={i.to === "/"} onClick={onNavigate} aria-label={collapsed ? t(i.label) : undefined}
                  className={cn(
                    "relative flex h-9 items-center gap-3 px-4 text-sm transition-colors focus-visible:ring-offset-0",
                    collapsed && "justify-center px-0",
                    isActive ? "bg-[#2f3338] text-white" : "text-[#d2d2d2] hover:bg-[#2a2d31] hover:text-white")}>
                  {isActive && <span aria-hidden className="absolute inset-y-0 left-0 w-[3px] bg-[#2b9af3]" />}
                  <i.icon className={cn("h-4 w-4 shrink-0", isActive ? "text-white" : "text-[#a3a6a9]")} aria-hidden />
                  {!collapsed && <span className="truncate">{t(i.label)}</span>}
                </NavLink>
              );
              return <li key={i.to} className="relative">{collapsed ? <Tip side="right" label={t(i.label)}>{link}</Tip> : link}</li>;
            })}
          </ul>
        </div>
      ))}
    </nav>
  );
}

function Brand() {
  return (
    <Link to="/" className="flex shrink-0 items-center gap-2.5" aria-label="Corvus AWPG — Active Web Protection Gateway" title="Corvus AWPG — Active Web Protection Gateway">
      <Wordmark className="h-[22px] w-auto shrink-0 text-white" />
      <span className="text-xs font-semibold tracking-[.12em] text-[#b8bbbe]">AWPG</span>
    </Link>
  );
}

export function Shell() {
  const { user, overview, logout } = useApp();
  const { t } = useI18n();
  const [collapsed, setCollapsed] = useCollapsed();
  const [drawer, setDrawer] = useState(false);
  // Lock document scrolling while the shell is mounted (only <main> scrolls).
  useEffect(() => {
    document.documentElement.classList.add("corvus-app-shell");
    return () => document.documentElement.classList.remove("corvus-app-shell");
  }, []);
  const loc = useLocation();
  useEffect(() => { setDrawer(false); }, [loc.pathname]);

  return (
    <div className="relative flex h-dvh flex-col overflow-hidden bg-bg">
      <a href="#main" className="sr-only focus:not-sr-only focus:fixed focus:left-3 focus:top-3 focus:z-[70] focus:bg-surface focus:px-3 focus:py-2 focus:shadow-pop">{t("skipToContent")}</a>

      <header className="z-30 flex h-14 shrink-0 items-center gap-3 border-b border-[#3c3f42] bg-[#151515] pl-2 pr-4 text-white sm:pr-6">
        <button type="button" onClick={() => setDrawer(true)} aria-label="Menu" className="grid h-9 w-9 place-items-center text-[#d2d2d2] hover:bg-[#2a2d31] hover:text-white lg:hidden">
          <MenuIcon className="h-5 w-5" />
        </button>
        <button type="button" onClick={() => setCollapsed(!collapsed)} aria-label={collapsed ? t("expandSidebar") : t("collapseSidebar")}
          className="hidden h-9 w-9 place-items-center text-[#d2d2d2] hover:bg-[#2a2d31] hover:text-white lg:grid">
          <MenuIcon className="h-5 w-5" />
        </button>
        <Brand />
        <div className="ml-auto flex items-center gap-3">
          <StatusPills />
          <UserMenu onLogout={() => { api.post("/api/auth/logout").catch(() => undefined).finally(logout); }} name={user.name || user.email} email={user.email} />
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        <aside className={cn("hidden shrink-0 flex-col transition-[width] duration-150 lg:flex", SIDEBAR, collapsed ? "w-[56px]" : "w-60")}>
          <NavList collapsed={collapsed} />
          <div className={cn("flex items-center border-t border-[#3c3f42] p-2", collapsed ? "justify-center" : "justify-between")}>
            {!collapsed && <span className="px-2 text-xs text-[#8a8d90] num">v{overview?.version ?? "–"}</span>}
            <button type="button" onClick={() => setCollapsed(!collapsed)} aria-label={collapsed ? t("expandSidebar") : t("collapseSidebar")}
              className="grid h-7 w-7 place-items-center text-[#a3a6a9] hover:bg-[#2a2d31] hover:text-white">
              {collapsed ? <ChevronsRight className="h-4 w-4" /> : <ChevronsLeft className="h-4 w-4" />}
            </button>
          </div>
        </aside>

        <Dialog.Root open={drawer} onOpenChange={setDrawer}>
          <Dialog.Portal>
            <Dialog.Overlay className="fixed inset-0 z-50 bg-black/60 data-[state=open]:animate-fade-in lg:hidden" />
            <Dialog.Content className={cn("fixed inset-y-0 left-0 z-50 flex w-72 max-w-[85vw] flex-col shadow-pop focus:outline-none data-[state=open]:animate-slide-in-left data-[state=closed]:animate-slide-out-left lg:hidden", SIDEBAR)}>
              <Dialog.Title className="sr-only">Menu</Dialog.Title>
              <Dialog.Description className="sr-only">Navigation</Dialog.Description>
              <div className="flex h-14 shrink-0 items-center border-b border-[#3c3f42] bg-[#151515] px-4"><Brand /></div>
              <NavList collapsed={false} onNavigate={() => setDrawer(false)} />
            </Dialog.Content>
          </Dialog.Portal>
        </Dialog.Root>

        {/* overflow-x-hidden + stable gutter: a scrollbar appearing must not change the content width (prevents resize/scrollbar feedback loops) */}
        {/* relative: absolutely positioned descendants (e.g. sr-only labels) must use main as their
            containing block; otherwise they escape the scroll container and lengthen the whole page. */}
        <main id="main" className="relative min-h-0 min-w-0 flex-1 overflow-y-auto overflow-x-hidden [scrollbar-gutter:stable]">
          {overview?.dirty && <DirtyBanner />}
          <div className="w-full px-4 pb-10 sm:px-6 lg:px-8">
            <Suspense fallback={<div className="space-y-4 pt-6"><Skeleton className="h-7 w-48" /><Skeleton className="h-40" /><Skeleton className="h-64" /></div>}>
              <Routes>
                <Route path="/" element={<Dashboard />} />
                <Route path="/proxy-hosts" element={<ProxyHosts />} />
                <Route path="/redirection-hosts" element={<RedirectionHosts />} />
                <Route path="/waitrooms" element={<Waitrooms />} />
                <Route path="/traffic" element={<Traffic />} />
                <Route path="/rate-limits" element={<RateLimits />} />
                <Route path="/security-rules" element={<SecurityRules />} />
                <Route path="/ip-lists" element={<IpLists />} />
                <Route path="/bans" element={<Bans />} />
                <Route path="/settings/retention" element={<Retention />} />
                <Route path="/access-lists" element={<Navigate to="/proxy-hosts" replace />} />
                <Route path="/certificates" element={<Certificates />} />
                <Route path="/network" element={<NetworkPage />} />
                <Route path="/settings/pages" element={<CustomPages />} />
                <Route path="/deployments" element={<Deployments />} />
                <Route path="/audit" element={<Audit />} />
                <Route path="/alerts" element={<Alerts />} />
                <Route path="/import" element={<ImportNpm />} />
                <Route path="/users" element={<UsersPage />} />
                <Route path="/settings/sso" element={<SsoPage />} />
                <Route path="/settings/tokens" element={<ApiTokens />} />
                <Route path="/account" element={<Account />} />
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </Suspense>
          </div>
        </main>
      </div>
    </div>
  );
}

function MastPill({ tone, children }: { tone: "ok" | "warn" | "danger" | "neutral"; children: ReactNode }) {
  const dot = { ok: "bg-[#87bb62]", warn: "bg-[#f6be00]", danger: "bg-[#ff7468]", neutral: "bg-[#8a8d90]" }[tone];
  return <span className="inline-flex h-6 items-center gap-1.5 whitespace-nowrap border border-[#52565a] px-2 text-xs text-[#d2d2d2]"><span aria-hidden className={cn("h-1.5 w-1.5 rounded-full", dot)} />{children}</span>;
}

function StatusPills() {
  const { overview, overviewError } = useApp();
  const { t } = useI18n();
  if (!overview) return overviewError ? <MastPill tone="danger">{t("apiUnreachable")}</MastPill> : null;
  const gw = overview.gateway;
  return (
    <div className="hidden items-center gap-1.5 md:flex" aria-label="status">
      {gw && <MastPill tone={gw.running ? "ok" : "danger"}>{t("gateway")} · {gw.running ? t("running") : t("stopped")}</MastPill>}
      <MastPill tone={overview.redis ? "ok" : "warn"}>{t("cacheDb")} · {overview.redis ? t("connected") : t("disconnected")}</MastPill>
      <MastPill tone={overview.active_version ? (overview.dirty ? "warn" : "neutral") : "neutral"}>
        {overview.active_version ? t("appliedVersion", { n: overview.active_version.id }) : t("noVersion")}
      </MastPill>
    </div>
  );
}

function UserMenu({ name, email, onLogout }: { name: string; email: string; onLogout: () => void }) {
  const { t, lang, setLang } = useI18n();
  const { role } = useCan();
  const { mode, setMode } = useTheme();
  const initial = (name || email).trim().charAt(0).toUpperCase();
  const themeIcon = mode === "dark" ? Moon : mode === "light" ? Sun : Monitor;
  const TI = themeIcon;
  return (
    <Menu.Root>
      <Menu.Trigger asChild>
        <button type="button" aria-label={t("userMenu")}
          className="flex h-9 items-center gap-2 px-2 text-white hover:bg-[#2a2d31] data-[state=open]:bg-[#2a2d31]">
          <span className="grid h-7 w-7 place-items-center rounded-full bg-[#2b9af3] text-xs font-semibold text-[#151515]">{initial}</span>
          <span className="hidden max-w-[10rem] truncate text-sm text-white sm:block">{name}</span>
          <TI className="hidden h-3.5 w-3.5 text-[#a3a6a9] sm:block" aria-hidden />
        </button>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content align="end" className="w-60">
          <div className="px-2.5 py-2">
            <div className="truncate text-sm font-medium text-fg">{name}</div>
            <div className="truncate text-xs text-fg-subtle">{email}</div>
            <div className="mt-1 inline-block border border-line px-1.5 text-2xs text-fg-muted">{roleKey[role] ? t(roleKey[role]) : role}</div>
          </div>
          <Menu.Separator />
          <Menu.Item asChild><Link to="/account"><UserCog className="h-3.5 w-3.5 opacity-70" />{t("navAccount")}</Link></Menu.Item>
          <Menu.Separator />
          <Menu.Label><Languages className="mr-1 inline h-3 w-3" />{t("language")}</Menu.Label>
          <Menu.RadioGroup value={lang} onValueChange={(v) => isLang(v) && setLang(v)}>
            {LANGS.map((l) => <Menu.RadioItem key={l} value={l} lang={l} onSelect={(e) => e.preventDefault()}>{LANG_NAMES[l]}</Menu.RadioItem>)}
          </Menu.RadioGroup>
          <Menu.Separator />
          <Menu.Label><Sun className="mr-1 inline h-3 w-3" />{t("theme")}</Menu.Label>
          <Menu.RadioGroup value={mode} onValueChange={(v) => setMode(v as ThemeMode)}>
            <Menu.RadioItem value="system" onSelect={(e) => e.preventDefault()}>{t("themeSystem")}</Menu.RadioItem>
            <Menu.RadioItem value="light" onSelect={(e) => e.preventDefault()}>{t("themeLight")}</Menu.RadioItem>
            <Menu.RadioItem value="dark" onSelect={(e) => e.preventDefault()}>{t("themeDark")}</Menu.RadioItem>
          </Menu.RadioGroup>
          <Menu.Separator />
          <Menu.Item onSelect={onLogout} className="text-danger data-[highlighted]:bg-danger/10"><LogOut className="h-3.5 w-3.5" />{t("logout")}</Menu.Item>
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

