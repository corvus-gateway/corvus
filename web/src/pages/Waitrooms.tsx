import { useState } from "react";
import { Eye, Globe, Pencil, Plus, Power, PowerOff, Trash2, Users } from "lucide-react";
import { api, type ProxyHost, type Waitroom } from "../api";
import { RadialGauge } from "../components/charts";
import { ErrorBox, PageHeader, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Badge, Callout, Card, EmptyState, Skeleton } from "../components/ui/display";
import { Field, FormSection, IdField, Input, NumberInput, Segmented, SwitchRow, fieldErr } from "../components/ui/form";
import { ConfirmDialog, RowMenu, Sheet } from "../components/ui/overlay";
import { useResource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n } from "../i18n";
import { cn } from "../lib/utils";

type Form = Omit<Waitroom, "id" | "created_at" | "updated_at" | "live">;
const blank: Form = { name: "", enabled: true, capacity: 500, active_ttl: 120, grant_ttl: 30, queue_ttl: 300, refresh: 15, admit_rate: 0, fail_mode: "open", max_queue: 100000, max_new_per_ip: 60, bind_ip: false };
const pick = (w: Waitroom): Form => ({
  name: w.name, enabled: w.enabled, capacity: w.capacity, active_ttl: w.active_ttl, grant_ttl: w.grant_ttl,
  queue_ttl: w.queue_ttl, refresh: w.refresh, admit_rate: w.admit_rate, fail_mode: w.fail_mode,
  max_queue: w.max_queue ?? 100000, max_new_per_ip: w.max_new_per_ip ?? 60, bind_ip: w.bind_ip ?? false,
});

export default function Waitrooms() {
  const { t } = useI18n();
  const { can } = useCan();
  const rooms = useResource<Waitroom[]>("/api/waitrooms", { poll: 4000 });
  const hosts = useResource<ProxyHost[]>("/api/proxy-hosts");
  const [editing, setEditing] = useState<{ id: number | null; form: Form } | null>(null);
  const [deleting, setDeleting] = useState<Waitroom | null>(null);
  const [releasing, setReleasing] = useState<Waitroom | null>(null);
  const { busy, error, setError, run } = useSubmit();
  const row = useSubmit({ toastAll: true });

  const refreshAll = () => { rooms.reload(); window.dispatchEvent(new Event("corvus:refresh")); };
  const open = (w?: Waitroom) => { setError(null); setEditing({ id: w?.id ?? null, form: w ? pick(w) : { ...blank } }); };

  const save = async () => {
    if (!editing) return;
    const r = await run(() => (editing.id ? api.put(`/api/waitrooms/${editing.id}`, editing.form) : api.post("/api/waitrooms", editing.form)), { success: t("savedDeployed") });
    if (r !== undefined) { setEditing(null); refreshAll(); }
  };
  const setEnabled = async (w: Waitroom, enabled: boolean) => {
    const r = await row.run(() => api.put(`/api/waitrooms/${w.id}`, { ...pick(w), enabled }), { success: enabled ? t("roomEnabled") : t("roomReleased") });
    if (r !== undefined) refreshAll();
  };
  const remove = async () => {
    if (!deleting) return;
    const r = await row.run(() => api.del(`/api/waitrooms/${deleting.id}`), { success: t("deletedDeployed") });
    if (r !== undefined) refreshAll();
  };

  // Opens the visitor waiting page with sample numbers (served by the admin API, no real queue needed).
  const preview = (w: Waitroom) => window.open(`/preview/wait/${encodeURIComponent(w.name)}?preview=1`, "_blank", "noopener");

  const f = editing?.form;
  const set = (p: Partial<Form>) => setEditing((e) => (e ? { ...e, form: { ...e.form, ...p } } : e));
  const refreshTooHigh = f && f.refresh > f.active_ttl / 2;

  return (
    <>
      <PageHeader title={t("navRooms")} description={t("roomsDesc")} actions={can && <Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addRoom")}</Button>} />

      {rooms.loading ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-52 rounded-xl" />)}</div>
      ) : rooms.error && !rooms.data ? <ErrorBox error={rooms.error} /> : !rooms.data?.length ? (
        <Card><EmptyState icon={Users} title={t("noRooms")} description={t("noRoomsDescLong")} action={<Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addRoom")}</Button>} /></Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {rooms.data.map((w) => {
            const attached = hosts.data?.filter((h) => h.waitroom_id === w.id) ?? [];
            const live = w.live;
            const ok = live?.available;
            return (
              <Card key={w.id} className={cn("flex flex-col p-4 transition-colors hover:border-line-strong animate-rise", !w.enabled && "opacity-90")}>
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2"><h3 className="truncate font-mono text-sm font-semibold">{w.name}</h3></div>
                    <div className="mt-1.5 flex flex-wrap gap-1.5">
                      {w.enabled ? <Badge tone="ok" dot>{t("roomActive")}</Badge> : <Badge tone="warn" dot>{t("roomReleasedBadge")}</Badge>}
                      <Badge tone={w.fail_mode === "open" ? "neutral" : "danger"}>{w.fail_mode === "open" ? "fail-open" : "fail-closed"}</Badge>
                    </div>
                  </div>
                  <RowMenu items={[
                    { label: t("edit"), icon: Pencil, onSelect: () => open(w) },
                    { label: t("previewWaitPage"), icon: Eye, onSelect: () => preview(w) },
                    { label: t("turnOn"), icon: Power, hidden: w.enabled, onSelect: () => setEnabled(w, true) },
                    { label: t("emergencyOff"), icon: PowerOff, hidden: !w.enabled, onSelect: () => setReleasing(w) },
                    { label: t("delete"), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => setDeleting(w) },
                  ]} />
                </div>

                <div className="mt-4 flex items-center gap-4">
                  <RadialGauge size={84} stroke={8} value={ok ? live!.active : 0} max={w.capacity} sublabel={ok ? t("occupancy") : undefined} label={ok ? undefined : "–"} />
                  <dl className="grid flex-1 grid-cols-3 gap-2 text-xs">
                    <div><dt className="text-fg-subtle">{t("activeLabel")}</dt><dd className="num mt-0.5 text-lg font-semibold">{ok ? live!.active : "–"}</dd></div>
                    <div><dt className="text-fg-subtle">{t("waitingLabel")}</dt><dd className={cn("num mt-0.5 text-lg font-semibold", ok && live!.waiting > 0 && "text-warn")}>{ok ? live!.waiting : "–"}</dd></div>
                    <div><dt className="text-fg-subtle">{t("capacityLabel")}</dt><dd className="num mt-0.5 text-lg font-semibold">{w.capacity}</dd></div>
                  </dl>
                </div>
                {live && !ok && <p className="mt-2 text-xs text-warn">{t("liveUnavailable")}</p>}

                <div className="mt-4 flex items-start gap-2 border-t border-line pt-3 text-xs text-fg-subtle">
                  <Globe className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden />
                  <span className="min-w-0 break-all">{attached.length ? attached.map((h) => h.domains[0] + (h.waitroom_path !== "/" ? h.waitroom_path : "")).join(", ") : t("noAttachedHosts")}</span>
                </div>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  {can && (w.enabled ? (
                    <Button size="sm" variant="secondary" className="text-warn" onClick={() => setReleasing(w)}><PowerOff className="h-3.5 w-3.5" />{t("emergencyOff")}</Button>
                  ) : (
                    <Button size="sm" variant="primary" onClick={() => setEnabled(w, true)}><Power className="h-3.5 w-3.5" />{t("turnOn")}</Button>
                  ))}
                  <Button size="sm" variant="ghost" onClick={() => preview(w)}><Eye className="h-3.5 w-3.5" />{t("previewWaitPage")}</Button>
                  {can && <Button size="sm" variant="ghost" onClick={() => open(w)}>{t("edit")}</Button>}
                </div>
              </Card>
            );
          })}
        </div>
      )}

      <Sheet open={!!editing} onOpenChange={(o) => !o && setEditing(null)} onSubmit={save} title={editing?.id ? t("editRoom") : t("addRoom")} description={t("roomSheetDesc")}
        footer={<><Button onClick={() => setEditing(null)}>{t("cancel")}</Button><Button type="submit" variant="primary" loading={busy}>{t("saveAndDeploy")}</Button></>}>
        {f && (<>
          {error && <div className="px-6 pt-5"><ErrorBox error={error} /></div>}
          <FormSection title={t("secBasic")}>
            <IdField label={t("roomName")} required hint={editing?.id ? t("roomNameImmutable") : t("roomNameHint")} error={fieldErr(error, "name")}>
              {(id) => <Input id={id} className="font-mono" value={f.name} disabled={!!editing?.id} onChange={(e) => set({ name: e.target.value.toLowerCase() })} placeholder="main-sale" invalid={!!fieldErr(error, "name")} />}
            </IdField>
            <SwitchRow label={t("enabled")} description={t("roomDisabledNote")} checked={f.enabled} onChange={(v) => set({ enabled: v })} />
          </FormSection>

          <FormSection title={t("secCapacity")} description={t("secCapacityDesc")}>
            <div className="grid gap-4 sm:grid-cols-2">
              <IdField label={t("capacity")} required error={fieldErr(error, "capacity")}>{(id) => <NumberInput id={id} min={1} value={f.capacity} onChange={(v) => set({ capacity: v })} suffix={t("unitPeople")} />}</IdField>
              <IdField label={t("admitRate")} hint={t("admitRateHint")} error={fieldErr(error, "admit_rate")}>{(id) => <NumberInput id={id} min={0} value={f.admit_rate} onChange={(v) => set({ admit_rate: v })} suffix={t("unitPerSec")} />}</IdField>
            </div>
          </FormSection>

          <FormSection title={t("secTiming")} description={t("secTimingDesc")}>
            <div className="grid gap-4 sm:grid-cols-2">
              <IdField label={t("activeTTL")} hint={t("activeTTLHint")} error={fieldErr(error, "active_ttl")}>{(id) => <NumberInput id={id} min={1} value={f.active_ttl} onChange={(v) => set({ active_ttl: v })} suffix={t("unitSec")} />}</IdField>
              <IdField label={t("refresh")} hint={t("refreshHint", { n: Math.floor((f.active_ttl || 0) / 2) })} error={fieldErr(error, "refresh") ?? (refreshTooHigh ? t("refreshTooHigh") : undefined)}>{(id) => <NumberInput id={id} min={1} value={f.refresh} onChange={(v) => set({ refresh: v })} suffix={t("unitSec")} />}</IdField>
              <IdField label={t("grantTTL")} hint={t("grantTTLHint")} error={fieldErr(error, "grant_ttl")}>{(id) => <NumberInput id={id} min={1} value={f.grant_ttl} onChange={(v) => set({ grant_ttl: v })} suffix={t("unitSec")} />}</IdField>
              <IdField label={t("queueTTL")} hint={t("queueTTLHint")} error={fieldErr(error, "queue_ttl")}>{(id) => <NumberInput id={id} min={1} value={f.queue_ttl} onChange={(v) => set({ queue_ttl: v })} suffix={t("unitSec")} />}</IdField>
            </div>
          </FormSection>

          <FormSection title={t("secAbuse")} description={t("secAbuseDesc")}>
            <div className="grid gap-4 sm:grid-cols-2">
              <IdField label={t("maxQueue")} hint={t("maxQueueHint")} error={fieldErr(error, "max_queue")}>{(id) => <NumberInput id={id} min={0} value={f.max_queue ?? 100000} onChange={(v) => set({ max_queue: v })} suffix={t("unitPeople")} />}</IdField>
              <IdField label={t("maxNewPerIp")} hint={t("maxNewPerIpHint")} error={fieldErr(error, "max_new_per_ip")}>{(id) => <NumberInput id={id} min={0} value={f.max_new_per_ip ?? 60} onChange={(v) => set({ max_new_per_ip: v })} suffix={t("unitPerMin")} />}</IdField>
            </div>
            <SwitchRow label={t("bindIp")} description={t("bindIpDesc")} checked={!!f.bind_ip} onChange={(v) => set({ bind_ip: v })} />
          </FormSection>

          <FormSection title={t("secFailure")}>
            <Field label={t("failMode")}>
              <Segmented label={t("failMode")} value={f.fail_mode} onChange={(v) => set({ fail_mode: v })}
                options={[{ value: "open", label: t("failOpenShort") }, { value: "closed", label: t("failClosedShort") }]} />
            </Field>
            <Callout tone={f.fail_mode === "open" ? "info" : "warn"}>{f.fail_mode === "open" ? t("failOpenDesc") : t("failClosedDesc")}</Callout>
          </FormSection>
        </>)}
      </Sheet>

      <ConfirmDialog open={!!releasing} onOpenChange={(o) => !o && setReleasing(null)} title={t("emergencyOffTitle")} description={t("emergencyOffDesc")}
        confirmLabel={t("emergencyOff")} onConfirm={async () => { if (releasing) await setEnabled(releasing, false); }}>
        {releasing && <div className="rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-xs">{releasing.name}</div>}
      </ConfirmDialog>
      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={t("deleteRoomTitle")} description={t("confirmDelete")} confirmLabel={t("delete")} onConfirm={remove}>
        {deleting && <div className="rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-xs">{deleting.name}</div>}
      </ConfirmDialog>
    </>
  );
}
