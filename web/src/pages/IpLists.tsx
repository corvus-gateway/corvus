import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { AlertTriangle, ChevronDown, Eye, Link2, ListChecks, Pencil, Plus, RefreshCw, Sparkles, Trash2 } from "lucide-react";
import { ApiError, api, deployedVersion, type IpList, type ListKind } from "../api";
import { ErrorBox, PageHeader, Toolbar, useSubmit } from "../components/common";
import { ErrorHint, useHintText } from "../components/ErrorHint";
import { Button } from "../components/ui/button";
import { Badge, Callout, Card, ComingSoon, EmptyState, TableSkeleton } from "../components/ui/display";
import { FormSection, IdField, Input, SearchInput, Segmented, Select, Textarea, fieldErr } from "../components/ui/form";
import { ConfirmDialog, Menu, RowMenu, Sheet } from "../components/ui/overlay";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n } from "../i18n";
import { LIMITS, isIpOrCidr, isUaEntry, listKind } from "../lib/rules";
import { cn, fmtDateTime, fmtNum, relTime } from "../lib/utils";

type Source = "manual" | "url";
interface Form { name: string; description: string; kind: ListKind; source: Source; url: string; refresh: number; text: string }
const blank: Form = { name: "", description: "", kind: "ip", source: "manual", url: "", refresh: 3600, text: "" };
const parseItems = (text: string) => [...new Set(text.split(/\r?\n/).map((s) => s.trim()).filter(Boolean))];

/** Presets only prefill the form; nothing is fetched until the operator saves. */
const PRESETS: { id: string; name: string; kind: ListKind; url: string; label: "presetTor" | "presetBadBots"; desc: "presetTorDesc" | "presetBadBotsDesc" }[] = [
  { id: "tor", name: "Tor exit nodes", kind: "ip", url: "https://check.torproject.org/torbulkexitlist", label: "presetTor", desc: "presetTorDesc" },
  { id: "badbots", name: "Bad bots (User-Agent)", kind: "user_agent", label: "presetBadBots", desc: "presetBadBotsDesc",
    url: "https://raw.githubusercontent.com/mitchellkrogza/nginx-ultimate-bad-bot-blocker/master/_generator_lists/bad-user-agents.list" },
];
const REFRESH = [300, 900, 3600, 6 * 3600, 12 * 3600, 86400, 7 * 86400];

export default function IpLists() {
  const { t, lang } = useI18n();
  const hintText = useHintText();
  const { can } = useCan();
  const list = useResource<IpList[]>("/api/ip-lists", { poll: 30000 });
  const [q, setQ] = useState("");
  const [editing, setEditing] = useState<{ id: number | null; form: Form; tried: boolean; orig?: IpList; preset?: string } | null>(null);
  const [deleting, setDeleting] = useState<IpList | null>(null);
  const [refreshing, setRefreshing] = useState<number | null>(null);
  const { busy, error, setError, run } = useSubmit();
  const ro = !can;

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase();
    return (list.data ?? []).filter((r) => !s || `${r.name} ${r.description ?? ""} ${r.source_url ?? ""}`.toLowerCase().includes(s) || r.items?.some((i) => i.toLowerCase().includes(s)));
  }, [list.data, q]);
  const open = (r?: IpList) => {
    setError(null);
    setEditing({
      id: r?.id ?? null, tried: false, orig: r,
      form: r ? { name: r.name, description: r.description ?? "", kind: listKind(r), source: r.source_url ? "url" : "manual", url: r.source_url ?? "", refresh: r.refresh_s || 3600, text: (r.items ?? []).join("\n") } : { ...blank },
    });
  };
  const openPreset = (p: (typeof PRESETS)[number]) => {
    setError(null);
    setEditing({ id: null, tried: false, preset: p.id, form: { ...blank, name: p.name, kind: p.kind, source: "url", url: p.url, description: t(p.desc) } });
  };
  const done = () => { list.reload(); window.dispatchEvent(new Event("corvus:refresh")); };

  const refreshNow = async (r: IpList) => {
    setRefreshing(r.id);
    try {
      const res = await api.post<{ ip_list: IpList }>(`/api/ip-lists/${r.id}/refresh`);
      const l = res.ip_list;
      if (l.fetch_error) toast.error(t("listFetchFailed"), { description: `${l.fetch_error} · ${hintText(l.fetch_error_hint) || t("listFetchFailedDesc")}` });
      else toast.success(t("listRefreshed"), { description: t("nItems", { n: fmtNum(l.items?.length ?? 0) }) });
      setEditing((e) => (e && e.id === l.id ? { ...e, orig: l, form: { ...e.form, text: (l.items ?? []).join("\n") } } : e));
      done();
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : t("requestFailed"));
    } finally { setRefreshing(null); }
  };

  const f = editing?.form;
  const feed = f?.source === "url";
  const items = useMemo(() => (f && !feed ? parseItems(f.text) : []), [f, feed]);
  const isUa = f?.kind === "user_agent";
  const max = isUa ? LIMITS.uaItems : LIMITS.listItems;
  const invalid = useMemo(() => items.filter((i) => !(isUa ? isUaEntry(i) : isIpOrCidr(i))), [items, isUa]);
  const kindLocked = !!editing?.orig?.rules;
  const clientErr = {
    name: f && !f.name.trim() ? t("fieldRequired") : undefined,
    items: feed ? undefined : invalid.length ? (isUa ? t("listUaInvalid", { n: invalid.length }) : t("ipListInvalid", { n: invalid.length })) : items.length > max ? t("ipListTooMany", { n: fmtNum(max) }) : undefined,
    source_url: feed && !/^https:\/\/[^\s/]+/i.test(f?.url.trim() ?? "") ? t("listUrlHint") : undefined,
  };
  type ErrKey = "name" | "items" | "description" | "source_url" | "refresh_s" | "kind";
  const err = (k: ErrKey) => (editing?.tried ? (clientErr as Record<string, string | undefined>)[k] : undefined) ?? fieldErr(error, k);

  const save = async () => {
    if (!editing || !f || ro) return;
    if (clientErr.name || clientErr.items || clientErr.source_url) { setEditing((e) => (e ? { ...e, tried: true } : e)); return; }
    const body = feed
      ? { name: f.name.trim(), description: f.description.trim(), kind: f.kind, source_url: f.url.trim(), refresh_s: f.refresh, items: [] }
      : { name: f.name.trim(), description: f.description.trim(), kind: f.kind, source_url: "", items };
    const r = await run(() => (editing.id ? api.put(`/api/ip-lists/${editing.id}`, body) : api.post("/api/ip-lists", body)), { success: t("savedDeployed") });
    if (r !== undefined) { setEditing(null); done(); }
  };
  const set = (p: Partial<Form>) => setEditing((e) => (e ? { ...e, form: { ...e.form, ...p } } : e));
  const every = (s: number) => (s % 86400 === 0 ? t("listEveryDay", { n: s / 86400 }) : s % 3600 === 0 ? t("listEveryHour", { n: s / 3600 }) : t("listEveryMin", { n: Math.round(s / 60) }));
  const refreshOpts = f && !REFRESH.includes(f.refresh) ? [...REFRESH, f.refresh].sort((a, b) => a - b) : REFRESH;
  const orig = editing?.orig;
  const showStatus = feed && orig?.source_url && orig.source_url === f?.url.trim();

  const addButtons = can && !list.missing && (
    <div className="flex flex-wrap gap-2">
      <Menu.Root>
        <Menu.Trigger asChild>
          <Button><Sparkles className="h-4 w-4" />{t("listAddPreset")}<ChevronDown className="h-3.5 w-3.5 opacity-70" /></Button>
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content align="end" className="max-w-[22rem]">
            {PRESETS.map((p) => (
              <Menu.Item key={p.id} onSelect={() => openPreset(p)} className="flex-col items-start gap-0.5">
                <span className="flex items-center gap-2 font-medium"><Badge tone={p.kind === "ip" ? "info" : "brand"}>{p.kind === "ip" ? t("kindShortIp") : t("kindShortUa")}</Badge>{t(p.label)}</span>
                <span className="text-xs text-fg-subtle">{t(p.desc)}</span>
              </Menu.Item>
            ))}
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
      <Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addIpList")}</Button>
    </div>
  );

  return (
    <>
      <PageHeader title={t("navIpLists")} description={t("ipListsDesc")} actions={addButtons} />
      <Card>
        {list.missing ? <ComingSoon icon={ListChecks} /> : (<>
          <Toolbar><SearchInput className="w-full sm:w-64" value={q} onChange={setQ} placeholder={t("searchIpLists")} /><span className="ml-auto text-xs text-fg-subtle num">{t("nItems", { n: filtered.length })}</span></Toolbar>
          {list.loading ? <TableSkeleton cols={4} /> : list.error && !list.data ? <div className="p-4"><ErrorBox error={list.error} /></div> : !list.data?.length ? (
            <EmptyState icon={ListChecks} title={t("noIpLists")} description={t("noIpListsDesc")} action={can ? <Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addIpList")}</Button> : undefined} />
          ) : !filtered.length ? <EmptyState icon={ListChecks} title={t("noResults")} description={t("noResultsDesc")} /> : (
            <Table>
              <THead><tr><Th>{t("name")}</Th><Th className="text-right">{t("entries")}</Th><Th className="hidden sm:table-cell">{t("usedByRules")}</Th><Th className="hidden md:table-cell">{t("updated")}</Th><Th className="w-12"><span className="sr-only">{t("actions")}</span></Th></tr></THead>
              <tbody>
                {filtered.map((r) => {
                  const upd = r.source_url ? r.fetched_at : r.updated_at;
                  return (
                    <Tr key={r.id}>
                      <Td className="max-w-[24rem]">
                        <div className="flex min-w-0 items-center gap-2">
                          <Badge tone={listKind(r) === "ip" ? "info" : "brand"}>{listKind(r) === "ip" ? t("kindShortIp") : t("kindShortUa")}</Badge>
                          <button type="button" onClick={() => open(r)} className="block min-w-0 truncate text-left font-medium text-fg hover:text-brand">{r.name}</button>
                        </div>
                        {r.source_url && (
                          <div className="mt-0.5 flex items-center gap-1 truncate text-xs text-fg-subtle" title={r.source_url}>
                            <Link2 className="h-3 w-3 shrink-0" aria-hidden />{hostOf(r.source_url)}
                            {r.fetch_error && <span className="ml-1 inline-flex items-center gap-1 text-warn" title={r.fetch_error}><AlertTriangle className="h-3 w-3" aria-hidden />{t("listFetchFailed")}</span>}
                          </div>
                        )}
                        {!r.source_url && r.description && <div className="truncate text-xs text-fg-subtle" title={r.description}>{r.description}</div>}
                      </Td>
                      <Td className="num text-right">{fmtNum(r.items?.length ?? 0)}</Td>
                      <Td className="hidden sm:table-cell">{r.rules ? <Link to="/security-rules" className="text-sm text-brand hover:underline">{t("rulesN", { n: r.rules })}</Link> : <span className="text-xs text-fg-subtle">{t("notUsed")}</span>}</Td>
                      <Td className="hidden text-fg-muted md:table-cell" title={upd ? fmtDateTime(upd, lang) : undefined}>{upd ? relTime(upd, lang) : r.source_url ? <span className="text-xs text-fg-subtle">{t("listNotFetched")}</span> : "–"}</Td>
                      <Td className="text-right"><RowMenu items={[
                        { label: t("edit"), icon: Pencil, onSelect: () => open(r) },
                        { label: t("listRefreshNow"), icon: RefreshCw, hidden: !r.source_url, disabled: refreshing === r.id, onSelect: () => refreshNow(r) },
                        { label: t("delete"), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => setDeleting(r) },
                      ]} /></Td>
                    </Tr>
                  );
                })}
              </tbody>
            </Table>
          )}
        </>)}
      </Card>

      <Sheet open={!!editing} onOpenChange={(o) => !o && setEditing(null)} onSubmit={save} title={ro ? t("viewIpList") : editing?.id ? t("editIpList") : t("addIpList")} description={feed ? t("listUrlHint") : isUa ? t("listUaItemsDesc") : t("ipListSheetDesc")}
        footer={ro ? <Button onClick={() => setEditing(null)}>{t("close")}</Button> : <><Button onClick={() => setEditing(null)}>{t("cancel")}</Button><Button type="submit" variant="primary" loading={busy}>{t("saveAndDeploy")}</Button></>}>
        {f && (<>
          {ro && <div className="px-6 pt-5"><Callout tone="neutral" icon={Eye}>{t("readOnlyNotice")}</Callout></div>}
          {editing?.preset && <div className="px-6 pt-5"><Callout tone="info" icon={Sparkles}>{t("listPresetNote")}</Callout></div>}
          {error != null && !(error instanceof ApiError && error.kind === "validation" && error.fields.every((x) => x.field)) && <div className="px-6 pt-5"><ErrorBox error={error} /></div>}
          <FormSection title={t("secBasic")}>
            <IdField label={t("name")} required error={err("name")}>{(id) => <Input id={id} value={f.name} disabled={ro} maxLength={100} onChange={(e) => set({ name: e.target.value })} placeholder={t("ipListNamePh")} invalid={!!err("name")} />}</IdField>
            <IdField label={t("description")} error={err("description")}>{(id) => <Input id={id} value={f.description} disabled={ro} maxLength={500} onChange={(e) => set({ description: e.target.value })} />}</IdField>
            <IdField label={t("listKind")} error={err("kind")} hint={kindLocked ? t("listKindLocked") : undefined}>{() => (
              ro || kindLocked
                ? <div className="text-sm text-fg">{f.kind === "ip" ? t("listKindIp") : t("listKindUa")}</div>
                : <Segmented label={t("listKind")} value={f.kind} onChange={(v) => set({ kind: v })} options={[{ value: "ip", label: t("listKindIp") }, { value: "user_agent", label: t("listKindUa") }]} />
            )}</IdField>
          </FormSection>
          <FormSection title={t("entries")} description={feed ? undefined : isUa ? t("listUaItemsDesc") : t("ipListItemsDesc")}>
            <IdField label={t("listEntriesSource")}>{() => (
              ro ? <div className="text-sm text-fg">{feed ? t("listSourceUrl") : t("listSourceManual")}</div>
                : <Segmented label={t("listEntriesSource")} value={f.source} onChange={(v) => set({ source: v })} options={[{ value: "manual", label: t("listSourceManual") }, { value: "url", label: t("listSourceUrl") }]} />
            )}</IdField>
            {feed ? (<>
              <IdField label={t("listUrl")} required error={err("source_url")} hint={t("listUrlHint")}>{(id) => (
                <Input id={id} type="url" inputMode="url" className="font-mono text-xs" value={f.url} disabled={ro} maxLength={2048} placeholder="https://example.com/blocklist.txt"
                  onChange={(e) => set({ url: e.target.value })} invalid={!!err("source_url")} />
              )}</IdField>
              <IdField label={t("listRefresh")} error={err("refresh_s")}>{(id) => (
                <Select id={id} value={f.refresh} disabled={ro} onChange={(e) => set({ refresh: Number(e.target.value) })} className="sm:w-56">
                  {refreshOpts.map((s) => <option key={s} value={s}>{every(s)}</option>)}
                </Select>
              )}</IdField>
              {showStatus && orig ? (
                <div className="space-y-2 rounded-lg border border-line bg-surface-2 p-3 text-sm">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="font-medium">{t("listFeedStatus")}</span>
                    {!ro && <Button size="sm" onClick={() => refreshNow(orig)} loading={refreshing === orig.id}><RefreshCw className="h-3.5 w-3.5" />{t("listRefreshNow")}</Button>}
                  </div>
                  <div className="text-fg-muted">
                    {t("listLastFetched")}: {orig.fetched_at ? <span title={fmtDateTime(orig.fetched_at, lang)}>{relTime(orig.fetched_at, lang)}</span> : t("listNotFetched")}
                    {" · "}<span className="num">{t("listFeedEntries", { n: fmtNum(orig.items?.length ?? 0) })}</span>
                  </div>
                  {orig.fetch_error && <Callout tone="warn" icon={AlertTriangle} title={t("listFetchFailed")}><span className="break-all">{orig.fetch_error}</span> {t("listFetchFailedDesc")}<ErrorHint code={orig.fetch_error_hint} className="mt-1.5" /></Callout>}
                  {orig.fetch_warning && <Callout tone="neutral" icon={AlertTriangle} title={t("listFetchWarning")}>{orig.fetch_warning}</Callout>}
                  {(orig.items?.length ?? 0) > 0 && (
                    <Textarea aria-label={t("entries")} rows={8} readOnly spellCheck={false} className="bg-surface font-mono text-xs leading-5" value={(orig.items ?? []).join("\n")} />
                  )}
                </div>
              ) : <p className="text-xs text-fg-subtle">{t("listFeedPending")}</p>}
            </>) : (<>
              <IdField label={isUa ? t("listUaItems") : t("ipListItems")} error={err("items")}
                hint={<span className={cn("num", items.length > max && "text-danger")}>{t("ipListCount", { n: fmtNum(items.length), max: fmtNum(max) })}</span>}>
                {(id) => <Textarea id={id} rows={14} spellCheck={false} readOnly={ro} className={cn("font-mono text-xs leading-5", ro && "bg-surface-2")} value={f.text}
                  onChange={(e) => set({ text: e.target.value })} placeholder={isUa ? "AhrefsBot\nsqlmap\nIndy Library" : "203.0.113.7\n10.0.0.0/8\n2001:db8::/32"} invalid={!!err("items")} />}
              </IdField>
              {!err("items") && <p className="num text-xs text-fg-subtle">{t("ipListCount", { n: fmtNum(items.length), max: fmtNum(max) })}</p>}
              {invalid.length > 0 && (
                <Callout tone="warn" icon={AlertTriangle} title={isUa ? t("listUaInvalid", { n: invalid.length }) : t("ipListInvalid", { n: invalid.length })}>
                  <span className="font-mono text-xs">{invalid.slice(0, 5).join(", ")}{invalid.length > 5 ? " …" : ""}</span>
                </Callout>
              )}
            </>)}
          </FormSection>
        </>)}
      </Sheet>

      <DeleteIpList target={deleting} onClose={() => setDeleting(null)} onDone={done} />
    </>
  );
}

function hostOf(u: string): string {
  try { return new URL(u).host; } catch { return u; }
}

/** Delete confirmation that turns a 409 (still referenced by rules) into a friendly message. */
function DeleteIpList({ target, onClose, onDone }: { target: IpList | null; onClose: () => void; onDone: () => void }) {
  const { t } = useI18n();
  const run = async () => {
    if (!target) return;
    try {
      const r = await api.del(`/api/ip-lists/${target.id}`);
      const v = deployedVersion(r);
      toast.success(v != null ? `${t("deletedDeployed")} · ${t("versionN", { n: v })}` : t("deletedDeployed"));
      onDone();
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) toast.error(t("ipListInUse", { name: target.name }), { description: t("ipListInUseDesc") });
      else if (e instanceof ApiError && e.status === 403) toast.error(t("forbidden"));
      else toast.error(e instanceof ApiError ? e.message : t("requestFailed"));
    }
  };
  return (
    <ConfirmDialog open={!!target} onOpenChange={(o) => !o && onClose()} title={t("deleteIpListTitle")}
      description={target?.rules ? t("ipListInUseWarn", { n: target.rules }) : t("confirmDelete")} confirmLabel={t("delete")} onConfirm={run}>
      {target && <div className="rounded-lg border border-line bg-surface-2 px-3 py-2 text-sm font-medium">{target.name}</div>}
    </ConfirmDialog>
  );
}
