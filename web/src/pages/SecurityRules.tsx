import { ManagedRulesCard, TemplateSheet } from "../components/RuleTemplates";
import { useEffect, useMemo, useRef, useState, type Dispatch, type DragEvent, type KeyboardEvent, type SetStateAction } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import {
  Activity, AlertTriangle, ArrowDown, ArrowUp, CheckCircle2, CornerDownRight, Eye, Globe, GripVertical, ListFilter, Pencil, Plus, RefreshCw, ShieldBan, Trash2, FileStack } from "lucide-react";
import { ApiError, api, type ChallengeBind, type ChallengeMode, type GeoipStatus, type IpList, type ProxyHost, type RuleAction, type RuleEvent, type RuleEventAction, type SecurityRule } from "../api";
import { ErrorBox, PageHeader, Toolbar, useSubmit } from "../components/common";
import { RuleExpression } from "../components/RuleExpression";
import { Button } from "../components/ui/button";
import { Badge, Callout, Card, CardHeader, CodeBlock, ComingSoon, EmptyState, Mono, TableSkeleton, type Tone } from "../components/ui/display";
import { Field, FormSection, IdField, Input, SearchInput, Segmented, Select, Switch, SwitchRow, Textarea } from "../components/ui/form";
import { MultiPicker } from "../components/ui/picker";
import { ConfirmDialog, RowMenu, Sheet, Tip } from "../components/ui/overlay";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource, type Resource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n, type Key, type TFn } from "../i18n";
import {
  LIMITS, STATUS_CODES, expressionText, flag, mapServerErrors, summarize, toGroups, toRows, usesCountry, validateRule, newRow,
  type CondRow, type RuleErrors,
} from "../lib/rules";
import { cn, fmtCompact, fmtDateTime, fmtNum, relTime } from "../lib/utils";

type Form = Pick<SecurityRule, "name" | "description" | "enabled" | "action" | "status_code" | "skip_basic_auth" | "scope" | "host_ids"
  | "challenge_mode" | "challenge_difficulty" | "challenge_ttl_s" | "challenge_bind">;
// Challenge limits (control/internal/validate/rules.go).
const CH_DIFFICULTY = { min: 12, max: 24, def: 16 } as const;
const CH_TTLS = [300, 1800, 3600, 21600, 86400, 259200, 604800] as const;
const blank: Form = {
  name: "", description: "", enabled: true, action: "block", status_code: 403, skip_basic_auth: false, scope: "all", host_ids: [],
  challenge_mode: "js", challenge_difficulty: CH_DIFFICULTY.def, challenge_ttl_s: 86400, challenge_bind: "ip",
};
const pick = (r: SecurityRule): Form => ({
  name: r.name, description: r.description ?? "", enabled: r.enabled, action: r.action, status_code: r.status_code || 403,
  skip_basic_auth: !!r.skip_basic_auth, scope: r.scope === "hosts" ? "hosts" : "all", host_ids: r.host_ids ?? [],
  challenge_mode: r.challenge_mode === "cookie" ? "cookie" : "js", challenge_difficulty: r.challenge_difficulty || CH_DIFFICULTY.def,
  challenge_ttl_s: r.challenge_ttl_s || 86400, challenge_bind: r.challenge_bind === "ip_ua" ? "ip_ua" : "ip",
});
const bodyOf = (f: Form, groups: SecurityRule["expression"]["groups"]) => ({
  ...f, name: f.name.trim(), description: f.description.trim(),
  status_code: f.action === "block" ? f.status_code : 403, skip_basic_auth: f.action === "allow" && f.skip_basic_auth,
  host_ids: f.scope === "hosts" ? f.host_ids : [], expression: { groups },
});

const ttlKeys: Record<number, Key> = { 300: "chTtl5m", 1800: "chTtl30m", 3600: "chTtl1h", 21600: "chTtl6h", 86400: "chTtl24h", 259200: "chTtl3d", 604800: "chTtl7d" };
const ttlLabel = (t: TFn, s: number) => (ttlKeys[s] ? t(ttlKeys[s]) : t("chTtlSeconds", { n: s }));
/** Short challenge summary for the rule list: "JavaScript (proof of work) · 16 · 24 hours". */
const challengeSummary = (t: TFn, r: Pick<SecurityRule, "challenge_mode" | "challenge_difficulty" | "challenge_ttl_s">) =>
  [r.challenge_mode === "cookie" ? t("chModeCookie") : t("chModeJs"), r.challenge_mode === "cookie" ? "" : String(r.challenge_difficulty || CH_DIFFICULTY.def), ttlLabel(t, r.challenge_ttl_s || 86400)]
    .filter(Boolean).join(" · ");

export const actionTone: Record<RuleEventAction, Tone> = { block: "danger", allow: "ok", log: "neutral", challenge: "brand", challenge_pass: "ok", challenge_fail: "warn" };
const actionKey: Record<RuleEventAction, Key> = {
  block: "actBlock", allow: "actAllow", log: "actLog", challenge: "actChallenge", challenge_pass: "actChallengePass", challenge_fail: "actChallengeFail",
};
export function ActionBadge({ action, code }: { action: RuleEventAction; code?: number }) {
  const { t } = useI18n();
  const label = actionKey[action] ? t(actionKey[action]) : action;
  return <Badge tone={actionTone[action] ?? "neutral"} dot>{label}{action === "block" && code ? <span className="num opacity-80">· {code}</span> : null}</Badge>;
}

const statusLabel = (t: TFn, c: number) => ({ 403: t("sc403"), 401: t("sc401"), 404: t("sc404"), 410: t("sc410"), 429: t("sc429"), 444: t("sc444") } as Record<number, string>)[c] ?? String(c);

type Tab = "rules" | "events";

export default function SecurityRules() {
  const { t } = useI18n();
  const { can } = useCan();
  const [tab, setTab] = useState<Tab>("rules");
  const [eventRule, setEventRule] = useState<number | "">("");
  const list = useResource<SecurityRule[]>("/api/security-rules", { poll: 20000 });
  const lists = useResource<IpList[]>("/api/ip-lists");
  const hosts = useResource<ProxyHost[]>("/api/proxy-hosts");
  const geo = useResource<GeoipStatus>("/api/security-rules/geoip", { poll: 60000 });
  const count = list.data?.length ?? 0;

  const [editing, setEditing] = useState<Editing | null>(null);
  const [tplOpen, setTplOpen] = useState(false);
  const openEditor = (r?: SecurityRule) => setEditing({ id: r?.id ?? null, form: r ? pick(r) : { ...blank }, rows: r ? toRows(r.expression?.groups ?? []) : [newRow()], tried: false });

  return (
    <>
      <PageHeader title={t("navSecurityRules")} description={t("securityRulesDesc")}
        actions={can && !list.missing && (
          count >= LIMITS.rules
            ? <Tip label={t("rulesLimitReached", { n: LIMITS.rules })}><span><Button variant="primary" disabled><Plus className="h-4 w-4" />{t("addRule")}</Button></span></Tip>
            : <div className="flex gap-2"><Button onClick={() => setTplOpen(true)}><FileStack className="h-4 w-4" />{t("fromTemplate")}</Button><Button variant="primary" onClick={() => openEditor()}><Plus className="h-4 w-4" />{t("addRule")}</Button></div>
        )} />
      <div role="tablist" aria-label={t("navSecurityRules")} className="mb-4 flex gap-1 border-b border-line">
        {([["rules", t("tabSecRules"), ShieldBan], ["events", t("tabRuleEvents"), Activity]] as const).map(([k, l, I]) => (
          <button key={k} role="tab" aria-selected={tab === k} onClick={() => setTab(k)}
            className={cn("-mb-px flex items-center gap-2 border-b-2 px-3.5 py-2.5 text-sm font-medium transition", tab === k ? "border-brand text-fg" : "border-transparent text-fg-subtle hover:text-fg")}>
            <I className="h-4 w-4" aria-hidden />{l}
          </button>
        ))}
      </div>
      {tab === "rules" ? (
        <RulesTab list={list} lists={lists.data} hosts={hosts.data} geo={geo} onEdit={openEditor}
          onEvents={(id) => { setEventRule(id); setTab("events"); }} />
      ) : (
        <EventsTab rules={list.data ?? []} ruleId={eventRule} setRuleId={setEventRule} />
      )}
      {tab === "rules" && <ManagedRulesCard />}
      <TemplateSheet open={tplOpen} onClose={() => setTplOpen(false)} hosts={hosts.data} lists={lists.data}
        onCreated={() => { list.reload(); window.dispatchEvent(new Event("corvus:refresh")); }} />
      <RuleSheet editing={editing} setEditing={setEditing} lists={lists.data} hosts={hosts.data} geoipAvailable={geo.data ? !!geo.data.available : null}
        onSaved={() => { list.reload(); lists.reload(); window.dispatchEvent(new Event("corvus:refresh")); }} />
    </>
  );
}

// ---------------------------------------------------------------- rules tab
function RulesTab({ list, lists, hosts, geo, onEdit, onEvents }: {
  list: Resource<SecurityRule[]>; lists: IpList[] | null; hosts: ProxyHost[] | null;
  geo: Resource<GeoipStatus>; onEdit: (r?: SecurityRule) => void; onEvents: (id: number) => void;
}) {
  const { t, lang } = useI18n();
  const { can } = useCan();
  const [q, setQ] = useState("");
  const [deleting, setDeleting] = useState<SecurityRule | null>(null);
  const [local, setLocal] = useState<SecurityRule[] | null>(null);
  const [drag, setDrag] = useState<{ id: number; over: number | null; after: boolean } | null>(null);
  const [armed, setArmed] = useState<number | null>(null);
  const [announce, setAnnounce] = useState("");
  const pending = useRef(false);
  const handles = useRef(new Map<number, HTMLButtonElement>());
  const row = useSubmit({ toastAll: true });
  const order = useSubmit();

  useEffect(() => { if (!pending.current) setLocal(null); }, [list.data]);
  const rules = useMemo(() => local ?? [...(list.data ?? [])].sort((a, b) => a.position - b.position), [local, list.data]);
  const s = q.trim().toLowerCase();
  const shown = useMemo(() => (s ? rules.filter((r) => `${r.name} ${r.description ?? ""}`.toLowerCase().includes(s)) : rules), [rules, s]);
  const canReorder = can && !s && rules.length > 1;
  const hostName = (id: number) => hosts?.find((h) => h.id === id)?.domains[0] ?? `#${id}`;
  const done = () => { list.reload(); window.dispatchEvent(new Event("corvus:refresh")); };

  const reorder = async (next: SecurityRule[], movedId: number) => {
    setLocal(next); pending.current = true;
    const idx = next.findIndex((r) => r.id === movedId);
    setAnnounce(t("movedTo", { name: next[idx]?.name ?? "", n: idx + 1 }));
    const r = await order.run(() => api.put("/api/security-rules/order", { ids: next.map((x) => x.id) }), { success: t("orderSaved"), silentError: true });
    pending.current = false;
    if (r === undefined) {
      setLocal(null);
      toast.error(t("orderFailed"));
    } else done();
  };
  const move = (id: number, to: number) => {
    const from = rules.findIndex((r) => r.id === id);
    if (from < 0 || to < 0 || to >= rules.length || to === from) return;
    const next = [...rules];
    const [it] = next.splice(from, 1);
    next.splice(to, 0, it);
    reorder(next, id);
    requestAnimationFrame(() => handles.current.get(id)?.focus());
  };

  const onHandleKey = (e: KeyboardEvent<HTMLButtonElement>, r: SecurityRule, i: number) => {
    if (!canReorder || order.busy) return;
    if (e.key === "ArrowUp") { e.preventDefault(); move(r.id, i - 1); }
    else if (e.key === "ArrowDown") { e.preventDefault(); move(r.id, i + 1); }
    else if (e.key === "Home") { e.preventDefault(); move(r.id, 0); }
    else if (e.key === "End") { e.preventDefault(); move(r.id, rules.length - 1); }
  };
  const onDragOver = (e: DragEvent<HTMLTableRowElement>, r: SecurityRule) => {
    if (!drag) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    const rect = e.currentTarget.getBoundingClientRect();
    const after = e.clientY > rect.top + rect.height / 2;
    if (drag.over !== r.id || drag.after !== after) setDrag({ ...drag, over: r.id, after });
  };
  const onDrop = (e: DragEvent<HTMLTableRowElement>) => {
    e.preventDefault();
    if (!drag || drag.over == null) return setDrag(null);
    const from = rules.findIndex((x) => x.id === drag.id);
    let to = rules.findIndex((x) => x.id === drag.over) + (drag.after ? 1 : 0);
    if (from < to) to -= 1;
    setDrag(null); setArmed(null);
    move(drag.id, to);
  };

  const toggle = async (r: SecurityRule, enabled: boolean) => {
    const x = await row.run(() => api.put(`/api/security-rules/${r.id}`, bodyOf({ ...pick(r), enabled }, r.expression?.groups ?? [])), { success: t("savedDeployed") });
    if (x !== undefined) done();
  };
  const remove = async () => {
    if (!deleting) return;
    const x = await row.run(() => api.del(`/api/security-rules/${deleting.id}`), { success: t("deletedDeployed") });
    if (x !== undefined) done();
  };

  const geoMissing = geo.data && !geo.data.available && rules.some((r) => r.enabled && usesCountry(r));

  if (list.missing) return <Card><ComingSoon icon={ShieldBan} /></Card>;
  return (
    <div className="space-y-4">
      {geoMissing && <Callout tone="warn" icon={AlertTriangle} title={t("geoipUnavailable")}>{t("geoipUnavailableDesc")}</Callout>}
      <Card>
        <Toolbar>
          <SearchInput className="w-full sm:w-64" value={q} onChange={setQ} placeholder={t("searchRules")} />
          <span className="ml-auto text-xs text-fg-subtle num">{t("nOfMax", { n: rules.length, max: LIMITS.rules })}</span>
        </Toolbar>
        <div className="flex items-start gap-2 border-b border-line bg-surface-2/50 px-4 py-2 text-xs text-fg-muted">
          <ArrowDown className="mt-0.5 h-3.5 w-3.5 shrink-0 text-fg-subtle" aria-hidden />
          <span>{t("evalOrderNote")}{s && can && <span className="ml-1 text-fg-subtle">{t("reorderDisabledSearch")}</span>}</span>
        </div>
        <div aria-live="polite" className="sr-only">{announce}</div>
        {list.loading ? <TableSkeleton cols={6} /> : list.error && !list.data ? <div className="p-4"><ErrorBox error={list.error} /></div> : !rules.length ? (
          <EmptyState icon={ShieldBan} title={t("noRules")} description={t("noRulesDesc")} action={<Button variant="primary" onClick={() => onEdit()}><Plus className="h-4 w-4" />{t("addRule")}</Button>} />
        ) : !shown.length ? <EmptyState icon={ShieldBan} title={t("noResults")} description={t("noResultsDesc")} /> : (
          <Table>
            <THead><tr>
              <Th className="w-16"><span className="sr-only">{t("order")}</span>#</Th><Th>{t("name")}</Th><Th className="hidden md:table-cell">{t("conditions")}</Th>
              <Th>{t("action")}</Th><Th className="hidden lg:table-cell">{t("scope")}</Th><Th className="hidden text-right sm:table-cell">{t("hits24h")}</Th>
              <Th>{t("status")}</Th><Th className="w-12"><span className="sr-only">{t("actions")}</span></Th>
            </tr></THead>
            <tbody>
              {shown.map((r) => {
                const i = rules.indexOf(r);
                const groups = r.expression?.groups ?? [];
                const dropLine = drag && drag.over === r.id && drag.id !== r.id;
                return (
                  <Tr key={r.id} draggable={canReorder && armed === r.id}
                    onDragStart={(e) => { e.dataTransfer.effectAllowed = "move"; e.dataTransfer.setData("text/plain", String(r.id)); setDrag({ id: r.id, over: null, after: false }); }}
                    onDragEnd={() => { setDrag(null); setArmed(null); }}
                    onDragOver={(e) => onDragOver(e, r)} onDrop={onDrop}
                    className={cn(!r.enabled && "text-fg-muted", drag?.id === r.id && "opacity-40",
                      dropLine && (drag!.after ? "shadow-[inset_0_-2px_0_0_rgb(var(--brand))]" : "shadow-[inset_0_2px_0_0_rgb(var(--brand))]"))}>
                    <Td className="whitespace-nowrap pr-0">
                      <div className="flex items-center gap-1">
                        {canReorder ? (
                          <button type="button" ref={(el) => { if (el) handles.current.set(r.id, el); else handles.current.delete(r.id); }}
                            aria-label={t("reorderHandle", { name: r.name, n: i + 1 })} aria-describedby="reorder-help" disabled={order.busy}
                            onPointerDown={() => setArmed(r.id)} onPointerUp={() => setArmed(null)} onKeyDown={(e) => onHandleKey(e, r, i)}
                            className="grid h-7 w-5 cursor-grab place-items-center rounded text-fg-subtle hover:bg-surface-3 hover:text-fg active:cursor-grabbing disabled:opacity-40">
                            <GripVertical className="h-4 w-4" aria-hidden />
                          </button>
                        ) : <span className="w-5" />}
                        <span className="num text-sm text-fg-subtle">{i + 1}</span>
                      </div>
                    </Td>
                    <Td className="max-w-[16rem]">
                      <button type="button" onClick={() => onEdit(r)} className="block max-w-full truncate text-left font-medium text-fg hover:text-brand">{r.name}</button>
                      {r.description && <div className="truncate text-xs text-fg-subtle" title={r.description}>{r.description}</div>}
                      <div className="mt-0.5 truncate text-xs text-fg-subtle md:hidden">{summarize(groups, lang, lists)}</div>
                    </Td>
                    <Td className="hidden max-w-[22rem] md:table-cell">
                      <Tip label={<span className="font-mono">{expressionText(groups, lists)}</span>}>
                        <span className="block truncate text-xs text-fg-muted" tabIndex={0}>{summarize(groups, lang, lists) || "–"}</span>
                      </Tip>
                    </Td>
                    <Td><ActionBadge action={r.action} code={r.status_code} />{r.action === "allow" && r.skip_basic_auth && <div className="mt-0.5 text-2xs text-fg-subtle">{t("skipsBasicAuth")}</div>}
                      {r.action === "challenge" && <div className="mt-0.5 text-2xs text-fg-subtle">{challengeSummary(t, r)}</div>}</Td>
                    <Td className="hidden max-w-[14rem] lg:table-cell">
                      {r.scope === "hosts" ? (
                        <Tip label={(r.host_ids ?? []).map(hostName).join(", ")}>
                          <span className="block truncate text-xs" tabIndex={0}>
                            {(r.host_ids ?? []).slice(0, 2).map(hostName).join(", ")}{(r.host_ids?.length ?? 0) > 2 && <span className="text-fg-subtle"> +{r.host_ids.length - 2}</span>}
                          </span>
                        </Tip>
                      ) : <span className="text-xs text-fg-muted">{t("allHosts")}</span>}
                    </Td>
                    <Td className="hidden text-right sm:table-cell">
                      <div className="num text-sm">{fmtCompact(r.hits_24h ?? 0)}</div>
                      {r.last_hit_at && <div className="text-2xs text-fg-subtle" title={fmtDateTime(r.last_hit_at, lang)}>{relTime(r.last_hit_at, lang)}</div>}
                    </Td>
                    <Td><Switch checked={r.enabled} onCheckedChange={(v) => toggle(r, v)} label={`${r.name} ${t("enabled")}`} disabled={row.busy || !can} /></Td>
                    <Td className="text-right"><RowMenu items={[
                      { label: t("edit"), icon: Pencil, onSelect: () => onEdit(r) },
                      { label: t("viewEvents"), icon: Activity, onSelect: () => onEvents(r.id) },
                      { label: t("moveUp"), icon: ArrowUp, separatorBefore: true, disabled: !canReorder || i === 0 || order.busy, onSelect: () => move(r.id, i - 1) },
                      { label: t("moveDown"), icon: ArrowDown, disabled: !canReorder || i === rules.length - 1 || order.busy, onSelect: () => move(r.id, i + 1) },
                      { label: t("delete"), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => setDeleting(r) },
                    ]} /></Td>
                  </Tr>
                );
              })}
              {!s && (
                <tr className="bg-surface-2/40">
                  <td colSpan={8} className="px-4 py-2 text-xs text-fg-subtle">
                    <span className="inline-flex items-center gap-1.5 pl-6"><CornerDownRight className="h-3.5 w-3.5" aria-hidden />{t("noMatchPass")}</span>
                  </td>
                </tr>
              )}
            </tbody>
          </Table>
        )}
        <p id="reorder-help" className="sr-only">{t("reorderHelp")}</p>
      </Card>
      <GeoipCard geo={geo} />

      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={t("deleteRuleTitle")} description={t("confirmDelete")} confirmLabel={t("delete")} onConfirm={remove}>
        {deleting && <div className="rounded-lg border border-line bg-surface-2 px-3 py-2 text-sm font-medium">{deleting.name}</div>}
      </ConfirmDialog>
    </div>
  );
}

// ---------------------------------------------------------------- geoip card
function GeoipCard({ geo }: { geo: Resource<GeoipStatus> }) {
  const { t, lang } = useI18n();
  const { isAdmin } = useCan();
  const sub = useSubmit();
  const refresh = async () => {
    const r = await sub.run(() => api.post<GeoipStatus>("/api/security-rules/geoip/refresh"), { deployed: false });
    if (r !== undefined) { toast.success(r.available ? t("geoipRefreshed") : t("geoipRefreshFailed")); geo.reload(); }
  };
  const g = geo.data;
  return (
    <Card>
      <CardHeader icon={Globe} title={t("geoipTitle")} description={t("geoipDesc")}
        actions={isAdmin && !geo.missing && <Button size="sm" loading={sub.busy} onClick={refresh}><RefreshCw className="h-3.5 w-3.5" />{t("refreshNow")}</Button>} />
      <div className="space-y-3 px-4 pb-4">
        {geo.missing ? <p className="text-sm text-fg-subtle">{t("comingSoonDesc")}</p>
          : geo.loading ? <div className="skeleton h-10 w-full" />
            : geo.error && !g ? <ErrorBox error={geo.error} />
              : g && (<>
                <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-4">
                  <div><dt className="text-xs text-fg-subtle">{t("status")}</dt><dd className="mt-0.5">{g.available ? <Badge tone="ok" dot>{t("geoipAvailable")}</Badge> : <Badge tone="warn" dot>{t("geoipNotAvailable")}</Badge>}</dd></div>
                  <div><dt className="text-xs text-fg-subtle">{t("geoipUpdated")}</dt><dd className="mt-0.5" title={g.updated_at ? fmtDateTime(g.updated_at, lang) : undefined}>{g.updated_at ? relTime(g.updated_at, lang) : "–"}</dd></div>
                  <div><dt className="text-xs text-fg-subtle">{t("geoipRanges")}</dt><dd className="num mt-0.5">{g.ranges ? fmtNum(g.ranges) : "–"}</dd></div>
                  <div><dt className="text-xs text-fg-subtle">{t("geoipSource")}</dt><dd className="mt-0.5 truncate text-fg-muted">{g.source || "–"}</dd></div>
                </dl>
                {!g.available && <Callout tone="warn" icon={AlertTriangle}>{t("geoipUnavailableDesc")}</Callout>}
                {g.error && <Callout tone="danger" title={t("geoipError")}><span className="font-mono text-xs">{g.error}</span></Callout>}
              </>)}
        {sub.error != null && !(sub.error instanceof ApiError && sub.error.status === 403) && <ErrorBox error={sub.error} />}
        <p className="text-xs text-fg-subtle">
          <a href="https://db-ip.com" target="_blank" rel="noopener noreferrer" className="text-brand underline-offset-2 hover:underline">IP geolocation by DB-IP</a>
          <span> · CC BY 4.0</span>
        </p>
      </div>
    </Card>
  );
}

// ---------------------------------------------------------------- events tab
const EVENT_LIMITS = [100, 250, 500];
function EventsTab({ rules, ruleId, setRuleId }: { rules: SecurityRule[]; ruleId: number | ""; setRuleId: (v: number | "") => void }) {
  const { t, lang } = useI18n();
  const [limit, setLimit] = useState(100);
  const [action, setAction] = useState<RuleEventAction | "">("");
  const [q, setQ] = useState("");
  const path = `/api/security-rules/events?limit=${limit}${ruleId !== "" ? `&rule_id=${ruleId}` : ""}`;
  const res = useResource<RuleEvent[]>(path, { poll: 10000 });
  const rows = useMemo(() => {
    const s = q.trim().toLowerCase();
    return (res.data ?? []).filter((e) => (!action || e.action === action) && (!s || `${e.ip} ${e.path} ${e.host} ${e.ua ?? ""} ${e.country ?? ""}`.toLowerCase().includes(s)));
  }, [res.data, action, q]);

  if (res.missing) return <Card><ComingSoon icon={Activity} /></Card>;
  return (
    <Card>
      <Toolbar>
        <SearchInput className="w-full sm:w-64" value={q} onChange={setQ} placeholder={t("searchRuleEvents")} />
        <div className="w-48"><Select aria-label={t("rule")} className="!h-8" value={ruleId} onChange={(e) => setRuleId(e.target.value ? Number(e.target.value) : "")}>
          <option value="">{t("allRules")}</option>{rules.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
        </Select></div>
        <div className="w-40"><Select aria-label={t("action")} className="!h-8" value={action} onChange={(e) => setAction(e.target.value as RuleEventAction | "")}>
          <option value="">{t("allActions")}</option>{(Object.keys(actionKey) as RuleEventAction[]).map((a) => <option key={a} value={a}>{t(actionKey[a])}</option>)}
        </Select></div>
        <div className="w-32"><Select aria-label={t("rows")} className="!h-8" value={limit} onChange={(e) => setLimit(Number(e.target.value))}>
          {EVENT_LIMITS.map((l) => <option key={l} value={l}>{t("lastN", { n: l })}</option>)}
        </Select></div>
        <span className="ml-auto inline-flex items-center gap-1.5 text-xs text-fg-subtle"><ListFilter className="h-3.5 w-3.5" aria-hidden /><span className="num">{t("nItems", { n: rows.length })}</span></span>
      </Toolbar>
      {res.loading ? <TableSkeleton cols={6} rows={8} /> : res.error && !res.data ? <div className="p-4"><ErrorBox error={res.error} /></div> : !rows.length ? (
        <EmptyState icon={Activity} title={t("noRuleEvents")} description={res.data?.length ? t("noResultsDesc") : t("noRuleEventsDesc")} />
      ) : (
        <Table>
          <THead><tr>
            <Th>{t("when")}</Th><Th>{t("rule")}</Th><Th>{t("action")}</Th><Th className="hidden md:table-cell">{t("host")}</Th><Th>IP</Th>
            <Th className="hidden sm:table-cell">{t("country")}</Th><Th className="hidden lg:table-cell">{t("request")}</Th><Th className="hidden xl:table-cell">User-Agent</Th>
          </tr></THead>
          <tbody>
            {rows.map((e, i) => (
              <Tr key={`${e.t}-${i}`}>
                <Td className="whitespace-nowrap"><div className="text-sm">{relTime(typeof e.t === "number" ? new Date(e.t > 1e12 ? e.t : e.t * 1000).toISOString() : e.t, lang)}</div><div className="num text-xs text-fg-subtle">{fmtDateTime(e.t, lang)}</div></Td>
                <Td className="max-w-[12rem] truncate font-medium">{e.rule?.startsWith("body:") ? `${t("bodyEventPrefix")} · ${e.rule.slice(5)}` : e.rule || `#${e.rule_id}`}</Td>
                <Td><ActionBadge action={e.action} code={e.action === "block" ? e.status : undefined} /></Td>
                <Td className="hidden max-w-[12rem] truncate text-xs text-fg-muted md:table-cell">{e.host || "–"}</Td>
                <Td><Mono className="text-fg-muted">{e.ip}</Mono></Td>
                <Td className="hidden whitespace-nowrap text-xs sm:table-cell">{e.country ? <span title={e.country}>{flag(e.country)} <span className="font-mono">{e.country}</span></span> : "–"}</Td>
                <Td className="hidden max-w-[18rem] lg:table-cell"><div className="truncate font-mono text-xs" title={e.path}><span className="text-fg-subtle">{e.method}</span> {e.path}</div></Td>
                <Td className="hidden max-w-[14rem] xl:table-cell"><div className="truncate text-xs text-fg-subtle" title={e.ua}>{e.ua || "–"}</div></Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      )}
    </Card>
  );
}

// ---------------------------------------------------------------- editor
type Editing = { id: number | null; form: Form; rows: CondRow[]; tried: boolean };
function RuleSheet({ editing, setEditing, lists, hosts, geoipAvailable, onSaved }: {
  editing: Editing | null; setEditing: Dispatch<SetStateAction<Editing | null>>; lists: IpList[] | null; hosts: ProxyHost[] | null;
  geoipAvailable: boolean | null; onSaved: () => void;
}) {
  const { t } = useI18n();
  const { can } = useCan();
  const { busy, error, setError, run } = useSubmit();
  const ro = !can;
  const isOpen = !!editing;
  useEffect(() => { if (isOpen) setError(null); }, [isOpen, setError]);

  const f = editing?.form;
  const set = (p: Partial<Form>) => setEditing((e) => (e ? { ...e, form: { ...e.form, ...p } } : e));
  const setRows = (rows: CondRow[]) => setEditing((e) => (e ? { ...e, rows } : e));

  const msgs = {
    required: t("fieldRequired"), noConditions: t("errNoConditions"), tooManyGroups: t("errTooManyGroups", { n: LIMITS.groups }), tooManyConds: t("errTooManyConds", { n: LIMITS.conds }),
    valueRequired: t("errValueRequired"), oneValue: t("errOneValue"), tooManyValues: t("errTooManyValues", { n: LIMITS.values }), valueTooLong: t("errValueTooLong", { n: LIMITS.valueLen }),
    badIp: t("errBadIp"), headerName: t("errHeaderName"), regexTooLong: t("errRegexTooLong", { n: LIMITS.regex }), badRegex: t("errBadRegex"), hostsRequired: t("errHostsRequired"),
  };
  const clientErr: RuleErrors = editing && f ? validateRule(f, editing.rows, msgs) : {};
  const serverErr: RuleErrors = error instanceof ApiError && editing ? mapServerErrors(error.fields, editing.rows) : {};
  const errs: RuleErrors = { ...serverErr, ...(editing?.tried ? clientErr : {}) };
  const groups = useMemo(() => (editing ? toGroups(editing.rows) : []), [editing]);
  const preview = expressionText(groups, lists);

  const save = async () => {
    if (!editing || !f || ro) return;
    if (Object.keys(clientErr).length) { setEditing((e) => (e ? { ...e, tried: true } : e)); return; }
    const body = bodyOf(f, groups);
    const r = await run(() => (editing.id ? api.put(`/api/security-rules/${editing.id}`, body) : api.post("/api/security-rules", body)), { success: t("savedDeployed") });
    if (r !== undefined) { setEditing(() => null); onSaved(); }
  };
  const hostOpts = useMemo(() => (hosts ?? []).map((h) => ({ value: String(h.id), label: h.domains[0] ?? `#${h.id}`, hint: h.domains.length > 1 ? `+${h.domains.length - 1}` : undefined })), [hosts]);
  const generalServer = error instanceof ApiError && error.kind === "validation" && Object.keys(serverErr).length === 0;

  return (
    <Sheet open={!!editing} onOpenChange={(o) => !o && setEditing(() => null)} onSubmit={save} width="max-w-[46rem]"
      title={ro ? t("viewRule") : editing?.id ? t("editRule") : t("addRule")} description={t("ruleSheetDesc")}
      footer={ro ? <Button onClick={() => setEditing(() => null)}>{t("close")}</Button>
        : <><Button onClick={() => setEditing(() => null)}>{t("cancel")}</Button><Button type="submit" variant="primary" loading={busy}>{t("saveAndDeploy")}</Button></>}>
      {editing && f && (<>
        {ro && <div className="px-6 pt-5"><Callout tone="neutral" icon={Eye}>{t("readOnlyNotice")}</Callout></div>}
        {error != null && (!(error instanceof ApiError) || error.kind !== "validation" || generalServer || error.fields.some((x) => !x.field)) && <div className="px-6 pt-5"><ErrorBox error={error} /></div>}
        {editing.tried && Object.keys(clientErr).length > 0 && <div className="px-6 pt-5"><Callout tone="danger" icon={AlertTriangle} title={t("validationFailed")}>{t("fixHighlighted")}</Callout></div>}
        <FormSection title={t("secBasic")}>
          <IdField label={t("name")} required error={errs.name}>{(id) => <Input id={id} value={f.name} disabled={ro} maxLength={100} onChange={(e) => set({ name: e.target.value })} placeholder={t("ruleNamePh")} invalid={!!errs.name} />}</IdField>
          <IdField label={t("description")} error={errs.description}>{(id) => <Textarea id={id} rows={2} value={f.description} disabled={ro} maxLength={500} onChange={(e) => set({ description: e.target.value })} invalid={!!errs.description} />}</IdField>
          <SwitchRow label={t("enabled")} description={t("ruleEnabledDesc")} checked={f.enabled} disabled={ro} onChange={(v) => set({ enabled: v })} />
        </FormSection>

        <FormSection title={t("secWhen")} description={t("secWhenDesc")}>
          <RuleExpression rows={editing.rows} onChange={setRows} errors={errs} lists={lists} geoipAvailable={geoipAvailable} disabled={ro} />
          <div className="space-y-1.5">
            <div className="text-xs font-semibold text-fg-muted">{t("exprPreview")}</div>
            <CodeBlock code={preview || "–"} maxHeight="10rem" />
          </div>
        </FormSection>

        <FormSection title={t("secThen")}>
          <Field label={t("action")}>
            <Segmented<RuleAction> label={t("action")} value={f.action} onChange={(v) => !ro && set({ action: v })}
              options={[{ value: "block", label: t("actBlock"), disabled: ro }, { value: "challenge", label: t("actChallenge"), disabled: ro }, { value: "allow", label: t("actAllow"), disabled: ro }, { value: "log", label: t("actLog"), disabled: ro }]} />
          </Field>
          {f.action === "block" && (
            <IdField label={t("responseCode")} error={errs.status_code} hint={f.status_code === 444 ? t("sc444Hint") : t("blockPageHint")}>
              {(id) => (
                <Select id={id} value={f.status_code} disabled={ro} onChange={(e) => set({ status_code: Number(e.target.value) })}>
                  {STATUS_CODES.map((c) => <option key={c} value={c}>{statusLabel(t, c)}</option>)}
                </Select>
              )}
            </IdField>
          )}
          {f.action === "allow" && (<>
            <label className="flex cursor-pointer items-start gap-2 text-sm">
              <input type="checkbox" checked={f.skip_basic_auth} disabled={ro} onChange={(e) => set({ skip_basic_auth: e.target.checked })} className="mt-0.5 h-4 w-4 accent-[rgb(var(--brand-solid))]" />
              <span><span className="font-medium text-fg">{t("skipBasicAuth")}</span><span className="block text-xs text-fg-subtle">{t("skipBasicAuthDesc")}</span></span>
            </label>
            <Callout tone="info" icon={CheckCircle2}>{t("allowNote")}</Callout>
          </>)}
          {f.action === "log" && <Callout tone="neutral">{t("logNote")}</Callout>}
          {f.action === "challenge" && (<>
            <Field label={t("chMode")} hint={f.challenge_mode === "cookie" ? t("chModeCookieDesc") : t("chModeJsDesc")}>
              <Segmented<ChallengeMode> label={t("chMode")} value={f.challenge_mode} onChange={(v) => !ro && set({ challenge_mode: v })}
                options={[{ value: "js", label: t("chModeJs"), disabled: ro }, { value: "cookie", label: t("chModeCookie"), disabled: ro }]} />
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              {f.challenge_mode === "js" && (
                <IdField label={t("chDifficulty")} error={errs.challenge_difficulty} hint={t("chDifficultyHint")}>
                  {(id) => (
                    <Select id={id} value={f.challenge_difficulty} disabled={ro} onChange={(e) => set({ challenge_difficulty: Number(e.target.value) })}>
                      {Array.from({ length: CH_DIFFICULTY.max - CH_DIFFICULTY.min + 1 }, (_, i) => CH_DIFFICULTY.min + i).map((b) => (
                        <option key={b} value={b}>{b}{b === CH_DIFFICULTY.def ? ` (${t("chDefault")})` : b === CH_DIFFICULTY.min ? ` (${t("chLight")})` : b === CH_DIFFICULTY.max ? ` (${t("chHeavy")})` : ""}</option>
                      ))}
                    </Select>
                  )}
                </IdField>
              )}
              <IdField label={t("chTtl")} error={errs.challenge_ttl_s} hint={t("chTtlHint")}>
                {(id) => (
                  <Select id={id} value={f.challenge_ttl_s} disabled={ro} onChange={(e) => set({ challenge_ttl_s: Number(e.target.value) })}>
                    {(CH_TTLS as readonly number[]).includes(f.challenge_ttl_s) ? null : <option value={f.challenge_ttl_s}>{ttlLabel(t, f.challenge_ttl_s)}</option>}
                    {CH_TTLS.map((s) => <option key={s} value={s}>{ttlLabel(t, s)}</option>)}
                  </Select>
                )}
              </IdField>
            </div>
            <IdField label={t("chBind")} error={errs.challenge_bind} hint={t("chBindHint")}>
              {(id) => (
                <Select id={id} value={f.challenge_bind} disabled={ro} onChange={(e) => set({ challenge_bind: e.target.value as ChallengeBind })}>
                  <option value="ip">{t("chBindIp")}</option><option value="ip_ua">{t("chBindIpUa")}</option>
                </Select>
              )}
            </IdField>
            <Callout tone="info">{t("chNote")}</Callout>
          </>)}
        </FormSection>

        <FormSection title={t("scope")} description={t("scopeDesc")}>
          <Segmented<"all" | "hosts"> label={t("scope")} value={f.scope} onChange={(v) => !ro && set({ scope: v })}
            options={[{ value: "all", label: t("allHosts"), disabled: ro }, { value: "hosts", label: t("selectedHosts"), disabled: ro }]} />
          {f.scope === "hosts" && (
            <Field label={t("navHosts")} required error={errs.host_ids}>
              <MultiPicker label={t("navHosts")} options={hostOpts} value={f.host_ids.map(String)} disabled={ro} invalid={!!errs.host_ids} placeholder={t("searchHosts")}
                onChange={(v) => set({ host_ids: v.map(Number) })} />
            </Field>
          )}
        </FormSection>
        <div className="px-6 pb-5 text-xs text-fg-subtle">
          <Link to="/ip-lists" className="text-brand hover:underline" onClick={() => setEditing(() => null)}>{t("manageIpLists")}</Link>
        </div>
      </>)}
    </Sheet>
  );
}
