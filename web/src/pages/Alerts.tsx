import { Fragment, useEffect, useMemo, useState } from "react";
import { Activity, AlertTriangle, Ban, Bell, BellRing, Check, ChevronDown, FlaskConical, Hash, MessageSquare, MessagesSquare, Pencil, Plus, Save, Send, Trash2, Search, Users, Webhook, X, type LucideIcon } from "lucide-react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import {
  api, type AlertActive, type AlertChannel, type AlertHistoryItem, type AlertKind, type AlertRuleCfg, type AlertRuleInfo, type AlertSettings, type AlertSeverity,
} from "../api";
import { ErrorBox, PageHeader, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Badge, Callout, Card, CardHeader, ComingSoon, EmptyState, Skeleton, TableSkeleton, type Tone } from "../components/ui/display";
import { Field, FormSection, IdField, Input, NumberInput, Segmented, Select, Switch, SwitchRow, fieldErr } from "../components/ui/form";
import { ConfirmDialog, RowMenu, Sheet, Tip } from "../components/ui/overlay";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n, type Key, type TFn } from "../i18n";
import { cn, fmtDateTime, relTime, toMs } from "../lib/utils";

const KINDS: { kind: AlertKind; label: string; icon: LucideIcon; color: string; hint: Key }[] = [
  { kind: "slack", label: "Slack", icon: Hash, color: "text-[#e01e5a]", hint: "hintSlack" },
  { kind: "mattermost", label: "Mattermost", icon: MessageSquare, color: "text-[#1e5ba3] dark:text-[#5b9bf0]", hint: "hintMattermost" },
  { kind: "discord", label: "Discord", icon: MessagesSquare, color: "text-[#5865f2]", hint: "hintDiscord" },
  { kind: "teams", label: "Microsoft Teams", icon: Users, color: "text-[#6264a7] dark:text-[#9ea0e0]", hint: "hintTeams" },
  { kind: "telegram", label: "Telegram", icon: Send, color: "text-[#229ed9]", hint: "hintTelegram" },
  { kind: "webhook", label: "Webhook", icon: Webhook, color: "text-fg-muted", hint: "hintWebhook" },
];
const kindMeta = (k: string) => KINDS.find((x) => x.kind === k) ?? KINDS[5];

const SEV: AlertSeverity[] = ["info", "warning", "critical"];
const sevTone: Record<string, Tone> = { info: "info", warning: "warn", critical: "danger" };
const sevKey: Record<AlertSeverity, Key> = { info: "sevInfo", warning: "sevWarning", critical: "sevCritical" };

const RULE_IDS = ["error_rate", "latency_p95", "traffic_spike", "rate_limited", "exploit", "security_events", "attack_source", "scan_suspect", "waitroom_queue", "redis_down", "cert_expiry", "acme_failed", "deploy_failed", "login_failed", "under_attack"];
const THR_UNIT: Record<string, Key | ""> = {
  error_rate: "", latency_p95: "", traffic_spike: "unitTimes", rate_limited: "unitCount", exploit: "unitCount", waitroom_queue: "unitPeopleShort", cert_expiry: "unitDays", login_failed: "unitCount",
};
const THR_SUFFIX: Record<string, string> = { error_rate: "%", latency_p95: "ms" };

const ruleName = (t: TFn, id: string) => ((`rule_${id}`) in ruleKeys ? t(`rule_${id}` as Key) : id);
const ruleKeys: Record<string, true> = Object.fromEntries(RULE_IDS.map((r) => [`rule_${r}`, true]));

export default function Alerts() {
  const { t } = useI18n();
  const [tab, setTab] = useState<"channels" | "rules" | "history">("channels");
  const active = useResource<AlertActive[]>("/api/alerts/active", { poll: 10000 });
  const channels = useResource<AlertChannel[]>("/api/alerts/channels");

  if (channels.missing) {
    return (<><PageHeader title={t("navAlerts")} description={t("alertsDesc")} /><Card><ComingSoon icon={Bell} /></Card></>);
  }
  const tabs = [["channels", t("tabChannels")], ["rules", t("tabRules")], ["history", t("tabHistory")]] as const;
  return (
    <>
      <PageHeader title={t("navAlerts")} description={t("alertsDesc")} />
      <ActiveStrip items={active.data} />
      <div role="tablist" aria-label="alerts" className="mb-4 flex gap-1 border-b border-line">
        {tabs.map(([k, l]) => (
          <button key={k} role="tab" aria-selected={tab === k} onClick={() => setTab(k)}
            className={cn("-mb-px border-b-2 px-3.5 py-2.5 text-sm font-medium transition", tab === k ? "border-brand text-fg" : "border-transparent text-fg-subtle hover:text-fg")}>{l}</button>
        ))}
      </div>
      {tab === "channels" && <Channels res={channels} />}
      {tab === "rules" && <Rules />}
      {tab === "history" && <History />}
    </>
  );
}

const IP_RULES = new Set(["attack_source", "scan_suspect"]);
const HOST_RULES = new Set(["error_rate", "latency_p95", "rate_limited", "exploit"]);
const PAGE_FOR: Record<string, string> = { cert_expiry: "/certificates", acme_failed: "/certificates", waitroom_queue: "/waitrooms", deploy_failed: "/deployments", login_failed: "/audit" };

function ActiveStrip({ items }: { items: AlertActive[] | null }) {
  const { t, lang } = useI18n();
  const { can } = useCan();
  const ban = useSubmit();
  if (!items?.length) return null;
  const banIP = (a: AlertActive) => ban.run(async () => {
    await api.post("/api/bans", { ip: a.key, duration_s: 86400, reason: a.title });
    toast.success(t("alBanned", { ip: a.key }));
  }, { deployed: false });
  return (
    <div className="mb-4 rounded-[3px] border border-danger/40 bg-danger/10 p-3.5">
      <div className="mb-2 flex items-center gap-2 text-sm font-semibold"><BellRing className="h-4 w-4 text-danger" aria-hidden />{t("activeNowN", { n: items.length })}</div>
      <ul className="space-y-2">
        {items.map((a) => (
          <li key={a.rule + a.key} className="rounded-lg border border-line bg-surface px-3 py-2 text-xs">
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone={sevTone[a.severity]} dot>{t(sevKey[a.severity])}</Badge>
              <span className="font-medium">{a.title}</span>
              <span className="text-fg-subtle">{t("since", { t: relTime(new Date(toMs(a.since)).toISOString(), lang) })}</span>
              <span className="ml-auto flex flex-wrap gap-1.5">
                {IP_RULES.has(a.rule) && <>
                  <Link to={`/traffic?tab=logs&range=1h&q=${encodeURIComponent(a.key)}`}><Button size="sm" variant="ghost"><Search className="h-3.5 w-3.5" />{t("alViewLogs")}</Button></Link>
                  {can && <Button size="sm" variant="danger" loading={ban.busy} onClick={() => banIP(a)}><Ban className="h-3.5 w-3.5" />{t("alBanIp")}</Button>}
                </>}
                {HOST_RULES.has(a.rule) && <Link to={`/traffic?host=${encodeURIComponent(a.key)}`}><Button size="sm" variant="ghost"><Activity className="h-3.5 w-3.5" />{t("alViewTraffic")}</Button></Link>}
                {PAGE_FOR[a.rule] && <Link to={PAGE_FOR[a.rule]}><Button size="sm" variant="ghost">{t("alOpen")}</Button></Link>}
              </span>
            </div>
            {a.text && <p className="mt-1.5 text-fg-muted">{a.text}</p>}
          </li>
        ))}
      </ul>
      <ErrorBox error={ban.error} className="mt-2" />
    </div>
  );
}

/* ---------------------------------------------------------------- channels */

interface ChForm {
  name: string; kind: AlertKind; enabled: boolean; min_severity: AlertSeverity; events: string[];
  url: string; username: string; channel: string; bot_token: string; chat_id: string; secret: string;
}
const chBlank: ChForm = { name: "", kind: "slack", enabled: true, min_severity: "warning", events: [], url: "", username: "", channel: "", bot_token: "", chat_id: "", secret: "" };

function Channels({ res }: { res: ReturnType<typeof useResource<AlertChannel[]>> }) {
  const { t } = useI18n();
  const { can } = useCan();
  const [editing, setEditing] = useState<{ id: number | null; form: ChForm } | null>(null);
  const [created, setCreated] = useState<AlertChannel | null>(null);
  const [deleting, setDeleting] = useState<AlertChannel | null>(null);
  const { busy, error, setError, run } = useSubmit();
  const row = useSubmit();

  const open = (c?: AlertChannel) => { setError(null); setEditing({ id: c?.id ?? null, form: c ? { ...chBlank, name: c.name, kind: c.kind, enabled: c.enabled, min_severity: c.min_severity, events: c.events ?? [] } : { ...chBlank } }); };
  const body = (f: ChForm, editingId: number | null) => {
    const b: Record<string, unknown> = { name: f.name, kind: f.kind, enabled: f.enabled, min_severity: f.min_severity, events: f.events };
    const put = (k: string, v: string) => { if (v !== "" || !editingId) b[k] = v; };
    if (f.kind === "telegram") { put("bot_token", f.bot_token); put("chat_id", f.chat_id); }
    else { put("url", f.url); if (f.kind === "mattermost") { put("username", f.username); put("channel", f.channel); } if (f.kind === "webhook") put("secret", f.secret); }
    return b;
  };
  const save = async () => {
    if (!editing) return;
    const r = await run(() => (editing.id ? api.put<unknown>(`/api/alerts/channels/${editing.id}`, body(editing.form, editing.id)) : api.post<unknown>("/api/alerts/channels", body(editing.form, null))), { deployed: false, success: undefined });
    if (r === undefined) return;
    res.reload(); setEditing(null);
    if (!editing.id) {
      const ch = ((r as { channel?: AlertChannel })?.channel ?? r) as AlertChannel;
      if (ch && typeof ch.id === "number") { setCreated(ch); return; }
    }
    toast.success(t("savedOk"));
  };
  const test = async (c: AlertChannel) => {
    try {
      const r = await api.post<{ ok: boolean; status_code?: number; error?: string }>(`/api/alerts/channels/${c.id}/test`);
      if (r.ok) toast.success(`${t("testOk")} · ${c.name}`);
      else toast.error(`${t("testFail")}: ${r.error || r.status_code || ""}`);
    } catch (e) { toast.error(`${t("testFail")}: ${(e as Error).message}`); }
  };
  const toggle = async (c: AlertChannel, enabled: boolean) => {
    const r = await row.run(() => api.put(`/api/alerts/channels/${c.id}`, { name: c.name, kind: c.kind, enabled, min_severity: c.min_severity, events: c.events ?? [] }), { deployed: false });
    if (r !== undefined) res.reload();
  };
  const remove = async () => {
    if (!deleting) return;
    const r = await row.run(() => api.del(`/api/alerts/channels/${deleting.id}`), { deployed: false });
    if (r !== undefined) res.reload();
  };

  const f = editing?.form;
  const set = (p: Partial<ChForm>) => setEditing((e) => (e ? { ...e, form: { ...e.form, ...p } } : e));
  const meta = f ? kindMeta(f.kind) : KINDS[0];
  const isEdit = !!editing?.id;
  const secretPh = isEdit ? t("secretKeep") : undefined;

  return (
    <>
      <div className="mb-3 flex justify-end">{can && <Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addChannel")}</Button>}</div>
      {res.loading ? <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-36 rounded-xl" />)}</div>
        : res.error && !res.data ? <ErrorBox error={res.error} />
          : !res.data?.length ? <Card><EmptyState icon={Bell} title={t("noChannels")} description={t("noChannelsDesc")} action={<Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addChannel")}</Button>} /></Card> : (
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
              {res.data.map((c) => {
                const m = kindMeta(c.kind);
                return (
                  <Card key={c.id} className={cn("flex flex-col p-4 animate-rise", !c.enabled && "opacity-80")}>
                    <div className="flex items-start justify-between gap-2">
                      <div className="flex min-w-0 items-center gap-3">
                        <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-line bg-surface-2"><m.icon className={cn("h-5 w-5", m.color)} aria-hidden /></span>
                        <div className="min-w-0"><div className="truncate text-sm font-semibold">{c.name}</div><div className="text-xs text-fg-subtle">{m.label}</div></div>
                      </div>
                      <RowMenu items={[
                        { label: t("edit"), icon: Pencil, onSelect: () => open(c) },
                        { label: t("testSend"), icon: FlaskConical, onSelect: () => test(c) },
                        { label: t("delete"), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => setDeleting(c) },
                      ]} />
                    </div>
                    <div className="mt-3 flex flex-wrap gap-1.5">
                      <Badge tone={sevTone[c.min_severity]}>≥ {t(sevKey[c.min_severity])}</Badge>
                      <Badge>{c.events?.length ? t("nRules", { n: c.events.length }) : t("allRules")}</Badge>
                    </div>
                    {c.target_hint && <div className="mt-2 truncate font-mono text-xs text-fg-subtle" title={c.target_hint}>{c.target_hint}</div>}
                    <div className="mt-auto flex items-center justify-between pt-4">
                      <Switch checked={c.enabled} onCheckedChange={(v) => toggle(c, v)} label={`${c.name} ${t("enabled")}`} disabled={!can} />
                      {can && <Button size="sm" onClick={() => test(c)}><FlaskConical className="h-3.5 w-3.5" />{t("testSend")}</Button>}
                    </div>
                  </Card>
                );
              })}
            </div>
          )}

      <Sheet open={!!editing} onOpenChange={(o) => !o && setEditing(null)} onSubmit={save} title={isEdit ? t("editChannel") : t("addChannel")} description={t("channelSheetDesc")}
        footer={<><Button onClick={() => setEditing(null)}>{t("cancel")}</Button><Button type="submit" variant="primary" loading={busy}>{t("save")}</Button></>}>
        {f && (<>
          {error && <div className="px-6 pt-5"><ErrorBox error={error} /></div>}
          <FormSection title={t("chKindPick")}>
            <div role="radiogroup" aria-label={t("chKindPick")} className="grid grid-cols-2 gap-2 sm:grid-cols-3">
              {KINDS.map((k) => (
                <button key={k.kind} type="button" role="radio" aria-checked={f.kind === k.kind} disabled={isEdit && f.kind !== k.kind} onClick={() => set({ kind: k.kind })}
                  className={cn("flex flex-col items-center gap-1.5 rounded-xl border p-3 text-xs font-medium transition disabled:opacity-40",
                    f.kind === k.kind ? "border-brand bg-brand/5 ring-1 ring-brand/40" : "border-line hover:border-line-strong hover:bg-surface-2")}>
                  <k.icon className={cn("h-5 w-5", k.color)} aria-hidden />{k.label}
                </button>
              ))}
            </div>
            <p className="text-xs leading-5 text-fg-subtle">{t(meta.hint)}</p>
          </FormSection>
          <FormSection title={t("secBasic")}>
            <IdField label={t("name")} required error={fieldErr(error, "name")}>{(id) => <Input id={id} value={f.name} onChange={(e) => set({ name: e.target.value })} placeholder="ops-alerts" invalid={!!fieldErr(error, "name")} />}</IdField>
            {f.kind === "telegram" ? (<>
              <IdField label={t("fBotToken")} required={!isEdit} error={fieldErr(error, "bot_token")}>{(id) => <Input id={id} type="password" autoComplete="off" className="font-mono" value={f.bot_token} onChange={(e) => set({ bot_token: e.target.value })} placeholder={secretPh ?? "123456:ABC-DEF…"} />}</IdField>
              <IdField label={t("fChatId")} required={!isEdit} error={fieldErr(error, "chat_id")}>{(id) => <Input id={id} className="font-mono" value={f.chat_id} onChange={(e) => set({ chat_id: e.target.value })} placeholder={secretPh ?? "-1001234567890"} />}</IdField>
            </>) : (<>
              <IdField label={t("fUrl")} required={!isEdit} error={fieldErr(error, "url")}>{(id) => <Input id={id} type="password" autoComplete="off" className="font-mono" value={f.url} onChange={(e) => set({ url: e.target.value })} placeholder={secretPh ?? "https://…"} invalid={!!fieldErr(error, "url")} />}</IdField>
              {f.kind === "mattermost" && (<div className="grid gap-4 sm:grid-cols-2">
                <IdField label={t("fUsername")}>{(id) => <Input id={id} value={f.username} onChange={(e) => set({ username: e.target.value })} placeholder="Corvus" />}</IdField>
                <IdField label={t("fChannel")}>{(id) => <Input id={id} value={f.channel} onChange={(e) => set({ channel: e.target.value })} placeholder="town-square" />}</IdField>
              </div>)}
              {f.kind === "webhook" && <IdField label={t("fSecret")}>{(id) => <Input id={id} type="password" autoComplete="off" className="font-mono" value={f.secret} onChange={(e) => set({ secret: e.target.value })} placeholder={secretPh} />}</IdField>}
            </>)}
            <SwitchRow label={t("enabled")} checked={f.enabled} onChange={(v) => set({ enabled: v })} />
          </FormSection>
          <FormSection title={t("minSeverity")}>
            <Segmented label={t("minSeverity")} value={f.min_severity} onChange={(v) => set({ min_severity: v })} options={SEV.map((s) => ({ value: s, label: t(sevKey[s]) }))} />
          </FormSection>
          <FormSection title={t("chEvents")} description={t("chEventsDesc")}>
            <div className="grid gap-1 sm:grid-cols-2">
              {RULE_IDS.map((r) => (
                <label key={r} className="flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-sm hover:bg-surface-2">
                  <input type="checkbox" className="h-4 w-4 accent-[rgb(var(--brand-solid))]" checked={f.events.includes(r)}
                    onChange={(e) => set({ events: e.target.checked ? [...f.events, r] : f.events.filter((x) => x !== r) })} />{ruleName(t, r)}
                </label>
              ))}
            </div>
          </FormSection>
        </>)}
      </Sheet>

      <ConfirmDialog open={!!created} danger={false} onOpenChange={(o) => !o && setCreated(null)} title={t("chCreated")} description={t("chCreatedTest")} confirmLabel={t("testSend")}
        onConfirm={async () => { if (created) await test(created); }} />
      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={t("deleteChannelTitle")} description={t("confirmDelete")} confirmLabel={t("delete")} onConfirm={remove}>
        {deleting && <div className="rounded-lg border border-line bg-surface-2 px-3 py-2 text-sm font-medium">{deleting.name}</div>}
      </ConfirmDialog>
    </>
  );
}

/* ------------------------------------------------------------------- rules */

function Rules() {
  const { t } = useI18n();
  const { can } = useCan();
  const res = useResource<{ settings: AlertSettings; rules: AlertRuleInfo[] }>("/api/alerts/rules");
  const [s, setS] = useState<AlertSettings | null>(null);
  const { busy, error, run } = useSubmit();
  useEffect(() => { if (res.data) setS(structuredClone(res.data.settings)); }, [res.data]);

  if (res.loading) return <Skeleton className="h-72 rounded-xl" />;
  if (res.missing) return <Card><ComingSoon icon={Bell} /></Card>;
  if (res.error && !res.data) return <ErrorBox error={res.error} />;
  if (!s || !res.data) return <Callout tone="warn">{t("rulesUnavailable")}</Callout>;

  const setRule = (id: string, p: Partial<AlertRuleCfg>) => setS({ ...s, rules: { ...s.rules, [id]: { ...(s.rules[id] ?? res.data!.rules.find((r) => r.id === id)!.default), ...p } } });
  const save = async () => {
    const r = await run(() => api.put("/api/alerts/rules", s), { deployed: false, success: t("rulesSaved") });
    if (r !== undefined) res.reload();
  };
  const infos = res.data.rules;

  return (
    <fieldset disabled={!can} className="min-w-0 space-y-5">
      <ErrorBox error={error} />
      {/* the API reports rule settings errors under the field "settings" */}
      {fieldErr(error, "settings") && <p role="alert" className="text-sm text-danger">{fieldErr(error, "settings")}</p>}
      <Card>
        <CardHeader icon={Bell} title={t("globalSettings")} />
        <div className="grid gap-5 px-4 pb-4 pt-1 md:grid-cols-2">
          <IdField label={t("cooldown")} hint={t("cooldownHint")} error={fieldErr(error, "cooldown_s")}>
            {(id) => <NumberInput id={id} min={1} value={Math.round(s.cooldown_s / 60)} onChange={(v) => setS({ ...s, cooldown_s: v * 60 })} suffix={t("unitMin")} />}
          </IdField>
          <Field label={t("alertLang")}>
            <Segmented label={t("alertLang")} value={s.language} onChange={(v) => setS({ ...s, language: v })} options={[{ value: "ko", label: "한국어" }, { value: "en", label: "English" }]} />
          </Field>
          <IdField label={t("publicUrl")} hint={t("publicUrlHint")} error={fieldErr(error, "public_url")}>
            {(id) => <Input id={id} className="font-mono" value={s.public_url} onChange={(e) => setS({ ...s, public_url: e.target.value })} placeholder="https://corvus.example.com:9443" />}
          </IdField>
          <div className="self-end"><SwitchRow label={t("sendResolved")} description={t("sendResolvedDesc")} checked={s.send_resolved} onChange={(v) => setS({ ...s, send_resolved: v })} /></div>
        </div>
      </Card>

      <Card>
        <ul className="divide-y divide-line">
          {infos.map((info) => {
            const cfg = s.rules[info.id] ?? info.default;
            const known = ruleKeys[`rule_${info.id}`];
            const has = (p: string) => info.params?.includes(p);
            const unitKey = THR_UNIT[info.id];
            return (
              <li key={info.id} className="grid gap-4 px-4 py-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
                <div className="flex min-w-0 items-start gap-3">
                  <div className="pt-0.5"><Switch checked={cfg.enabled} onCheckedChange={(v) => setRule(info.id, { enabled: v })} label={ruleName(t, info.id)} disabled={!can} /></div>
                  <div className="min-w-0">
                    <div className="text-sm font-semibold">{ruleName(t, info.id)}</div>
                    <p className="mt-0.5 text-xs text-fg-subtle">{known ? t(`ruled_${info.id}` as Key) : ""}</p>
                  </div>
                </div>
                <div className={cn("flex flex-wrap items-end gap-3", !cfg.enabled && "opacity-50")}>
                  {has("threshold") && (
                    <label className="w-36 text-xs text-fg-subtle">{known && THR_UNIT[info.id] !== undefined ? t(`thrl_${info.id}` as Key) : t("threshold")}
                      <NumberInput min={0} step={info.id === "traffic_spike" ? 0.5 : 1} value={cfg.threshold ?? 0} onChange={(v) => setRule(info.id, { threshold: v })}
                        suffix={THR_SUFFIX[info.id] ?? (unitKey ? t(unitKey) : info.unit)} aria-label={t("threshold")} />
                    </label>
                  )}
                  {has("window_s") && (
                    <label className="w-28 text-xs text-fg-subtle">{t("windowMin")}
                      <NumberInput min={1} value={Math.round((cfg.window_s ?? 60) / 60)} onChange={(v) => setRule(info.id, { window_s: v * 60 })} suffix={t("unitMin")} />
                    </label>
                  )}
                  {has("min_requests") && (
                    <label className="w-32 text-xs text-fg-subtle">{info.id === "traffic_spike" ? t("minRps") : t("minRequests")}
                      <NumberInput min={0} value={cfg.min_requests ?? 0} onChange={(v) => setRule(info.id, { min_requests: v })} suffix={info.id === "traffic_spike" ? "rps" : t("unitCount")} />
                    </label>
                  )}
                  <label className="w-28 text-xs text-fg-subtle">{t("severity")}
                    <Select value={cfg.severity} onChange={(e) => setRule(info.id, { severity: e.target.value as AlertSeverity })} aria-label={t("severity")}>
                      {SEV.map((x) => <option key={x} value={x}>{t(sevKey[x])}</option>)}
                    </Select>
                  </label>
                </div>
              </li>
            );
          })}
        </ul>
      </Card>
      {can && <div className="sticky bottom-0 -mx-4 flex justify-end border-t border-line bg-surface px-4 py-3 sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
        <Button variant="primary" loading={busy} onClick={save}><Save className="h-4 w-4" />{t("rulesSave")}</Button>
      </div>}
    </fieldset>
  );
}

/* ----------------------------------------------------------------- history */

function History() {
  const { t, lang } = useI18n();
  const res = useResource<AlertHistoryItem[]>("/api/alerts/history?limit=200", { poll: 15000 });
  const [open, setOpen] = useState<number | null>(null);
  const items = useMemo(() => res.data ?? [], [res.data]);
  const stTone: Record<string, Tone> = { firing: "danger", resolved: "ok", test: "neutral" };
  const stKey: Record<string, Key> = { firing: "stFiring", resolved: "stResolved", test: "stTest" };

  return (
    <Card>
      {res.loading ? <TableSkeleton cols={5} rows={6} /> : res.missing ? <ComingSoon icon={Bell} /> : res.error && !res.data ? <div className="p-4"><ErrorBox error={res.error} /></div> : !items.length ? (
        <EmptyState icon={Bell} title={t("noHistory")} description={t("noHistoryDesc")} />
      ) : (
        <Table>
          <THead><tr><Th>{t("when")}</Th><Th>{t("status")}</Th><Th>{t("severity")}</Th><Th>{t("action")}</Th><Th className="hidden md:table-cell">{t("delivery")}</Th><Th className="w-10"><span className="sr-only">{t("details")}</span></Th></tr></THead>
          <tbody>
            {items.map((h) => {
              const isOpen = open === h.id;
              return (
                <Fragment key={h.id}>
                  <Tr className="cursor-pointer" onClick={() => setOpen(isOpen ? null : h.id)}>
                    <Td className="whitespace-nowrap"><div className="text-sm">{relTime(new Date(toMs(h.ts)).toISOString(), lang)}</div><div className="num text-xs text-fg-subtle">{fmtDateTime(toMs(h.ts), lang)}</div></Td>
                    <Td><Badge tone={stTone[h.status] ?? "neutral"} dot>{t(stKey[h.status] ?? "stTest")}</Badge></Td>
                    <Td><Badge tone={sevTone[h.severity] ?? "neutral"}>{t(sevKey[h.severity] ?? "sevInfo")}</Badge></Td>
                    <Td className="max-w-[24rem]"><div className="truncate text-sm font-medium">{h.title}</div><div className="text-xs text-fg-subtle">{ruleName(t, h.rule)}</div></Td>
                    <Td className="hidden md:table-cell">
                      <div className="flex flex-wrap gap-1.5">
                        {h.results?.length ? h.results.map((r) => (
                          <Tip key={r.channel_id} label={r.ok ? r.channel : `${r.channel}: ${r.error ?? ""}`}>
                            <span><Badge tone={r.ok ? "ok" : "danger"} icon={r.ok ? Check : X}>{r.channel}</Badge></span>
                          </Tip>
                        )) : <span className="text-xs text-fg-subtle">{t("noDelivery")}</span>}
                      </div>
                    </Td>
                    <Td><ChevronDown className={cn("h-4 w-4 text-fg-subtle transition", isOpen && "rotate-180")} aria-hidden /></Td>
                  </Tr>
                  {isOpen && (
                    <tr className="border-b border-line bg-surface-2/40"><td colSpan={6} className="px-4 py-3">
                      <pre className="whitespace-pre-wrap break-words font-sans text-sm text-fg-muted">{h.message}</pre>
                      {h.results?.some((r) => !r.ok) && (
                        <ul className="mt-2 space-y-1 text-xs text-danger"><AlertTriangle className="mr-1 inline h-3 w-3" />{h.results.filter((r) => !r.ok).map((r) => <li key={r.channel_id} className="ml-4 list-disc">{r.channel}: {r.error}</li>)}</ul>
                      )}
                    </td></tr>
                  )}
                </Fragment>
              );
            })}
          </tbody>
        </Table>
      )}
    </Card>
  );
}
