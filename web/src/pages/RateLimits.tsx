import { useMemo, useState } from "react";
import { Eye, Gauge, Pencil, Plus, Trash2 } from "lucide-react";
import { api, type RateLimit } from "../api";
import { ErrorBox, PageHeader, Toolbar, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Badge, Callout, Card, ComingSoon, EmptyState, TableSkeleton } from "../components/ui/display";
import { FormSection, IdField, Input, NumberInput, SearchInput, Switch, SwitchRow, TagInput, fieldErr } from "../components/ui/form";
import { ConfirmDialog, RowMenu, Sheet } from "../components/ui/overlay";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n } from "../i18n";

type Form = Omit<RateLimit, "id" | "hosts">;
const blank: Form = { name: "", requests_per_second: 10, burst: 20, max_connections: 0, status_code: 429, dry_run: false, exempt_cidrs: [], enabled: true };
const pick = (r: RateLimit): Form => ({ name: r.name, requests_per_second: r.requests_per_second, burst: r.burst, max_connections: r.max_connections, status_code: r.status_code, dry_run: r.dry_run, exempt_cidrs: r.exempt_cidrs ?? [], enabled: r.enabled });

export default function RateLimits() {
  const { t } = useI18n();
  const { can } = useCan();
  const list = useResource<RateLimit[]>("/api/rate-limits", { poll: 20000 });
  const [q, setQ] = useState("");
  const [editing, setEditing] = useState<{ id: number | null; form: Form } | null>(null);
  const [deleting, setDeleting] = useState<RateLimit | null>(null);
  const { busy, error, setError, run } = useSubmit();
  const row = useSubmit({ toastAll: true });

  const filtered = useMemo(() => (list.data ?? []).filter((r) => r.name.toLowerCase().includes(q.trim().toLowerCase())), [list.data, q]);
  const open = (r?: RateLimit) => { setError(null); setEditing({ id: r?.id ?? null, form: r ? pick(r) : { ...blank } }); };
  const done = () => { list.reload(); window.dispatchEvent(new Event("corvus:refresh")); };

  const save = async () => {
    if (!editing) return;
    const r = await run(() => (editing.id ? api.put(`/api/rate-limits/${editing.id}`, editing.form) : api.post("/api/rate-limits", editing.form)), { success: t("savedDeployed") });
    if (r !== undefined) { setEditing(null); done(); }
  };
  const toggle = async (r: RateLimit, enabled: boolean) => {
    const x = await row.run(() => api.put(`/api/rate-limits/${r.id}`, { ...pick(r), enabled }), { success: t("savedDeployed") });
    if (x !== undefined) done();
  };
  const remove = async () => {
    if (!deleting) return;
    const x = await row.run(() => api.del(`/api/rate-limits/${deleting.id}`), { success: t("deletedDeployed") });
    if (x !== undefined) done();
  };

  const f = editing?.form;
  const set = (p: Partial<Form>) => setEditing((e) => (e ? { ...e, form: { ...e.form, ...p } } : e));

  return (
    <>
      <PageHeader title={t("navRateLimits")} description={t("rateLimitsDesc")}
        actions={can && !list.missing && <Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addRateLimit")}</Button>} />
      <Card>
        {list.missing ? <ComingSoon icon={Gauge} /> : (<>
          <Toolbar><SearchInput className="w-full sm:w-64" value={q} onChange={setQ} /><span className="ml-auto text-xs text-fg-subtle num">{t("nItems", { n: filtered.length })}</span></Toolbar>
          {list.loading ? <TableSkeleton cols={5} /> : list.error && !list.data ? <div className="p-4"><ErrorBox error={list.error} /></div> : !list.data?.length ? (
            <EmptyState icon={Gauge} title={t("noRateLimits")} description={t("noRateLimitsDesc")} action={<Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addRateLimit")}</Button>} />
          ) : !filtered.length ? <EmptyState icon={Gauge} title={t("noResults")} description={t("noResultsDesc")} /> : (
            <Table>
              <THead><tr><Th>{t("name")}</Th><Th>{t("rpsLimit")}</Th><Th className="hidden md:table-cell">{t("burst")}</Th><Th className="hidden lg:table-cell">{t("maxConn")}</Th><Th className="hidden md:table-cell">{t("mode")}</Th><Th className="hidden sm:table-cell">{t("navHosts")}</Th><Th>{t("status")}</Th><Th className="w-12"><span className="sr-only">{t("actions")}</span></Th></tr></THead>
              <tbody>
                {filtered.map((r) => (
                  <Tr key={r.id}>
                    <Td><div className="font-medium">{r.name}</div>{r.exempt_cidrs?.length > 0 && <div className="text-xs text-fg-subtle">{t("exemptN", { n: r.exempt_cidrs.length })}</div>}</Td>
                    <Td className="num">{r.requests_per_second} <span className="text-xs text-fg-subtle">r/s</span></Td>
                    <Td className="num hidden md:table-cell">{r.burst}</Td>
                    <Td className="num hidden lg:table-cell">{r.max_connections || <span className="text-fg-subtle">–</span>}</Td>
                    <Td className="hidden md:table-cell">{r.dry_run ? <Badge tone="info" icon={Eye}>{t("dryRunBadge")}</Badge> : <Badge tone="neutral">{t("enforcing", { code: r.status_code })}</Badge>}</Td>
                    <Td className="num hidden sm:table-cell">{r.hosts ?? "–"}</Td>
                    <Td><Switch checked={r.enabled} onCheckedChange={(v) => toggle(r, v)} label={`${r.name} ${t("enabled")}`} disabled={row.busy || !can} /></Td>
                    <Td className="text-right"><RowMenu items={[
                      { label: t("edit"), icon: Pencil, onSelect: () => open(r) },
                      { label: t("delete"), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => setDeleting(r) },
                    ]} /></Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </>)}
      </Card>

      <Sheet open={!!editing} onOpenChange={(o) => !o && setEditing(null)} onSubmit={save} title={editing?.id ? t("editRateLimit") : t("addRateLimit")} description={t("rateLimitSheetDesc")}
        footer={<><Button onClick={() => setEditing(null)}>{t("cancel")}</Button><Button type="submit" variant="primary" loading={busy}>{t("saveAndDeploy")}</Button></>}>
        {f && (<>
          {error && <div className="px-6 pt-5"><ErrorBox error={error} /></div>}
          <FormSection title={t("secBasic")}>
            <IdField label={t("name")} required error={fieldErr(error, "name")}>{(id) => <Input id={id} value={f.name} onChange={(e) => set({ name: e.target.value })} placeholder="api-default" invalid={!!fieldErr(error, "name")} />}</IdField>
          </FormSection>
          <FormSection title={t("secThreshold")} description={t("secThresholdDesc")}>
            <div className="grid gap-4 sm:grid-cols-2">
              <IdField label={t("rpsLimit")} required hint={t("rpsHint")} error={fieldErr(error, "requests_per_second")}>{(id) => <NumberInput id={id} min={1} value={f.requests_per_second} onChange={(v) => set({ requests_per_second: v })} suffix="r/s" />}</IdField>
              <IdField label={t("burst")} hint={t("burstHint")} error={fieldErr(error, "burst")}>{(id) => <NumberInput id={id} min={0} value={f.burst} onChange={(v) => set({ burst: v })} />}</IdField>
              <IdField label={t("maxConnPerIp")} hint={t("maxConnHint")} error={fieldErr(error, "max_connections")}>{(id) => <NumberInput id={id} min={0} value={f.max_connections} onChange={(v) => set({ max_connections: v })} />}</IdField>
              <IdField label={t("statusCode")} hint={t("statusCodeHint")} error={fieldErr(error, "status_code")}>{(id) => <NumberInput id={id} min={400} max={599} value={f.status_code} onChange={(v) => set({ status_code: v })} />}</IdField>
            </div>
          </FormSection>
          <FormSection title={t("secMode")}>
            <SwitchRow label={t("dryRun")} description={t("dryRunDesc")} checked={f.dry_run} onChange={(v) => set({ dry_run: v })} />
            {f.dry_run && <Callout tone="info" icon={Eye}>{t("dryRunNote")}</Callout>}
            <SwitchRow label={t("enabled")} checked={f.enabled} onChange={(v) => set({ enabled: v })} />
          </FormSection>
          <FormSection title={t("secExempt")} description={t("secExemptDesc")}>
            <IdField label={t("exemptCidrs")} error={fieldErr(error, "exempt_cidrs")}>
              {() => <TagInput label={t("exemptCidrs")} value={f.exempt_cidrs} onChange={(v) => set({ exempt_cidrs: v })} placeholder="10.0.0.0/8, 192.168.0.0/16" invalid={!!fieldErr(error, "exempt_cidrs")} />}
            </IdField>
          </FormSection>
        </>)}
      </Sheet>

      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={t("deleteRateLimitTitle")} description={deleting?.hosts ? t("inUseWarn", { n: deleting.hosts }) : t("confirmDelete")} confirmLabel={t("delete")} onConfirm={remove}>
        {deleting && <div className="rounded-lg border border-line bg-surface-2 px-3 py-2 text-sm font-medium">{deleting.name}</div>}
      </ConfirmDialog>
    </>
  );
}
