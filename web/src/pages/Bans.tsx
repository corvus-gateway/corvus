import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Ban, Lock, Plus, Save, ShieldOff } from "lucide-react";
import { api } from "../api";
import { ErrorBox, PageHeader, Toolbar, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Badge, Callout, Card, EmptyState, TableSkeleton } from "../components/ui/display";
import { Field, FormSection, IdField, Input, NumberInput, Select, SwitchRow, TagInput, fieldErr } from "../components/ui/form";
import { ConfirmDialog, Sheet } from "../components/ui/overlay";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n } from "../i18n";
import { fmtDateTime, relTime } from "../lib/utils";

interface IPBan { id: number; ip: string; reason: string; source: "auto" | "manual"; offenses: number; created_at: string; expires_at: string | null }
interface AutoBan { enabled: boolean; threshold: number; window_s: number; duration_s: number; max_duration_s: number; permanent_after: number; include_scans: boolean; scan_threshold: number; exempt: string[];
  bad_behaviour: boolean; bad_behaviour_codes: number[]; bad_behaviour_threshold: number; bad_behaviour_window_s: number }

const DURATIONS: { v: number; k: "dur15m" | "dur1h" | "dur24h" | "dur7d" | "durForever" }[] = [
  { v: 900, k: "dur15m" }, { v: 3600, k: "dur1h" }, { v: 86400, k: "dur24h" }, { v: 7 * 86400, k: "dur7d" }, { v: 0, k: "durForever" },
];

export default function Bans() {
  const { t, lang } = useI18n();
  const { can } = useCan();
  const bans = useResource<IPBan[]>("/api/bans", { poll: 15000 });
  const settings = useResource<AutoBan>("/api/settings/autoban");
  const [s, setS] = useState<AutoBan | null>(null);
  const [adding, setAdding] = useState<{ ip: string; duration: number; reason: string } | null>(null);
  const [lifting, setLifting] = useState<IPBan | null>(null);
  const saveSettings = useSubmit();
  const addBan = useSubmit();
  const lift = useSubmit();
  const perm = useSubmit();
  const makePermanent = async (b: IPBan) => {
    const r = await perm.run(() => api.post(`/api/bans/${b.id}/permanent`, {}), { success: t("madePermanentOk") });
    if (r !== undefined) bans.reload();
  };

  useEffect(() => { if (settings.data) setS(settings.data); }, [settings.data]);
  const dirty = !!s && !!settings.data && JSON.stringify(s) !== JSON.stringify(settings.data);

  const onSave = async () => {
    if (!s) return;
    const r = await saveSettings.run(() => api.put("/api/settings/autoban", s), { success: t("savedOk") });
    if (r !== undefined) settings.reload();
  };
  const onAdd = async () => {
    if (!adding) return;
    const r = await addBan.run(() => api.post("/api/bans", { ip: adding.ip.trim(), duration_s: adding.duration, reason: adding.reason }), { success: t("bannedOk") });
    if (r !== undefined) { setAdding(null); bans.reload(); }
  };
  const onLift = async () => {
    if (!lifting) return;
    const r = await lift.run(() => api.del(`/api/bans/${lifting.id}`), { success: t("unbannedOk") });
    if (r !== undefined) { setLifting(null); bans.reload(); }
  };

  const minutes = (sec: number) => Math.round(sec / 60);

  return (
    <>
      <PageHeader title={t("navBans")} description={t("bansDesc")}
        actions={can && <Button variant="primary" onClick={() => { addBan.setError(null); setAdding({ ip: "", duration: 3600, reason: "" }); }}><Plus className="h-4 w-4" />{t("addBan")}</Button>} />
      <div className="space-y-6">
        <Card>
          <form onSubmit={(e) => { e.preventDefault(); void onSave(); }}>
            <fieldset disabled={!can} className="space-y-4 p-5">
              <h2 className="text-sm font-semibold">{t("autoBanTitle")}</h2>
              {settings.error && !settings.data ? <ErrorBox error={settings.error} /> : !s ? <TableSkeleton cols={3} rows={2} /> : (
                <>
                  <SwitchRow label={t("autoBanEnable")} description={t("autoBanEnableDesc")} checked={s.enabled} onChange={(v) => setS({ ...s, enabled: v })} />
                  {s.enabled && (
                    <>
                      <div className="grid gap-4 sm:grid-cols-3">
                        <IdField label={t("banThreshold")}>{(id) => <NumberInput id={id} min={3} max={100000} value={s.threshold} onChange={(v) => setS({ ...s, threshold: v })} />}</IdField>
                        <IdField label={t("banWindow")}>{(id) => <NumberInput id={id} min={1} max={1440} value={minutes(s.window_s)} onChange={(v) => setS({ ...s, window_s: v * 60 })} />}</IdField>
                        <IdField label={t("banDuration")}>{(id) => <NumberInput id={id} min={1} max={1440} value={minutes(s.duration_s)} onChange={(v) => setS({ ...s, duration_s: v * 60 })} />}</IdField>
                      </div>
                      <div className="grid gap-4 sm:grid-cols-3">
                        <IdField label={t("banMaxDuration")}>{(id) => <NumberInput id={id} min={1} max={30} value={Math.max(1, Math.round((s.max_duration_s || 604800) / 86400))} onChange={(v) => setS({ ...s, max_duration_s: v * 86400 })} />}</IdField>
                        <IdField label={t("banPermanentAfter")} hint={t("banPermanentAfterHint")}>{(id) => <NumberInput id={id} min={0} max={100} value={s.permanent_after ?? 0} onChange={(v) => setS({ ...s, permanent_after: v === 1 ? 2 : v })} />}</IdField>
                      </div>
                      <SwitchRow label={t("banIncludeScans")} description={t("banIncludeScansDesc")} checked={s.include_scans} onChange={(v) => setS({ ...s, include_scans: v })} />
                      {s.include_scans && (
                        <div className="sm:w-1/3">
                          <IdField label={t("banScanThreshold")}>{(id) => <NumberInput id={id} min={20} max={100000} value={s.scan_threshold} onChange={(v) => setS({ ...s, scan_threshold: v })} />}</IdField>
                        </div>
                      )}
                      <SwitchRow label={t("banBadBehaviour")} description={t("banBadBehaviourDesc")} checked={!!s.bad_behaviour} onChange={(v) => setS({ ...s, bad_behaviour: v })} />
                      {s.bad_behaviour && (
                        <div className="grid gap-4 sm:grid-cols-3">
                          <IdField label={t("banBbThreshold")}>{(id) => <NumberInput id={id} min={3} max={100000} value={s.bad_behaviour_threshold || 10} onChange={(v) => setS({ ...s, bad_behaviour_threshold: v })} />}</IdField>
                          <IdField label={t("banBbWindow")}>{(id) => <NumberInput id={id} min={10} max={3600} value={s.bad_behaviour_window_s || 60} onChange={(v) => setS({ ...s, bad_behaviour_window_s: v })} />}</IdField>
                          <Field label={t("banBbCodes")}>
                            <TagInput label={t("banBbCodes")} value={(s.bad_behaviour_codes ?? []).map(String)} validate={(v) => /^[45]\d\d$/.test(v.trim())}
                              onChange={(v) => setS({ ...s, bad_behaviour_codes: [...new Set(v.map((x) => Number(x.trim())))].filter((n) => n >= 400 && n <= 599) })} placeholder="404, 403" />
                          </Field>
                        </div>
                      )}
                      <Field label={t("banExempt")} hint={t("banExemptHint")} error={fieldErr(saveSettings.error, "autoban")}>
                        <TagInput label={t("banExempt")} value={s.exempt} onChange={(v) => setS({ ...s, exempt: v })} placeholder="203.0.113.0/24, 10.0.0.0/8" />
                      </Field>
                      <Callout tone="warn">{t("banNatWarn")}</Callout>
                    </>
                  )}
                  {saveSettings.error != null && !fieldErr(saveSettings.error, "autoban") && <ErrorBox error={saveSettings.error} />}
                  {can && <div className="flex justify-end"><Button type="submit" variant="primary" loading={saveSettings.busy} disabled={!dirty}><Save className="h-4 w-4" />{t("save")}</Button></div>}
                </>
              )}
            </fieldset>
          </form>
        </Card>

        <Card>
          <Toolbar><h2 className="text-sm font-semibold">{t("activeBans")}</h2><span className="ml-auto text-xs text-fg-subtle num">{t("nItems", { n: bans.data?.length ?? 0 })}</span></Toolbar>
          {perm.error != null && <div className="px-4 pb-3"><ErrorBox error={perm.error} /></div>}
          {bans.loading ? <TableSkeleton cols={6} rows={4} /> : bans.error && !bans.data ? <div className="p-4"><ErrorBox error={bans.error} /></div> : !bans.data?.length ? (
            <EmptyState icon={Ban} title={t("noBans")} description={t("noBansDesc")} />
          ) : (
            <Table>
              <THead><tr><Th>{t("banIp")}</Th><Th className="hidden md:table-cell">{t("banReason")}</Th><Th>{t("banSource")}</Th><Th className="hidden sm:table-cell">{t("banOffenses")}</Th><Th className="hidden sm:table-cell">{t("banSince")}</Th><Th>{t("banExpires")}</Th><Th className="w-12"><span className="sr-only">{t("actions")}</span></Th></tr></THead>
              <tbody>
                {bans.data.map((b) => (
                  <Tr key={b.id}>
                    <Td className="font-mono text-sm">{b.ip}</Td>
                    <Td className="hidden max-w-[22rem] truncate text-xs text-fg-muted md:table-cell" title={b.reason}>{b.reason}</Td>
                    <Td><Badge tone={b.source === "auto" ? "warn" : "neutral"}>{b.source === "auto" ? t("banSourceAuto") : t("banSourceManual")}</Badge></Td>
                    <Td className="num hidden sm:table-cell">{b.offenses}</Td>
                    <Td className="hidden text-fg-muted sm:table-cell" title={fmtDateTime(b.created_at, lang)}>{relTime(b.created_at, lang)}</Td>
                    <Td className="text-fg-muted" title={b.expires_at ? fmtDateTime(b.expires_at, lang) : undefined}>{b.expires_at ? fmtDateTime(b.expires_at, lang) : t("banUntilRemoved")}</Td>
                    <Td>{can && <div className="flex flex-wrap justify-end gap-1.5">
                      {b.expires_at && <Button size="sm" variant="ghost" loading={perm.busy} onClick={() => void makePermanent(b)}><Lock className="h-3.5 w-3.5" />{t("makePermanent")}</Button>}
                      <Button size="sm" onClick={() => { lift.setError(null); setLifting(b); }}><ShieldOff className="h-3.5 w-3.5" />{t("unban")}</Button>
                    </div>}</Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      </div>

      <Sheet open={!!adding} onOpenChange={(o) => !o && setAdding(null)} onSubmit={onAdd} title={t("addBan")} description={t("bansDesc")}
        footer={<><Button onClick={() => setAdding(null)}>{t("cancel")}</Button><Button type="submit" variant="primary" loading={addBan.busy}>{t("addBan")}</Button></>}>
        {adding && (
          <FormSection title={t("addBan")}>
            {addBan.error != null && !fieldErr(addBan.error, "ip") && <ErrorBox error={addBan.error} />}
            <IdField label={t("banIp")} required error={fieldErr(addBan.error, "ip")}>{(id) => <Input id={id} className="font-mono" value={adding.ip} onChange={(e) => setAdding({ ...adding, ip: e.target.value })} placeholder="203.0.113.7" invalid={!!fieldErr(addBan.error, "ip")} />}</IdField>
            <IdField label={t("banDurationLabel")}>{(id) => (
              <Select id={id} value={adding.duration} onChange={(e) => setAdding({ ...adding, duration: Number(e.target.value) })}>
                {DURATIONS.map((d) => <option key={d.v} value={d.v}>{t(d.k)}</option>)}
              </Select>
            )}</IdField>
            <IdField label={t("banReason")}>{(id) => <Input id={id} value={adding.reason} maxLength={200} onChange={(e) => setAdding({ ...adding, reason: e.target.value })} />}</IdField>
          </FormSection>
        )}
      </Sheet>
      <ConfirmDialog open={!!lifting} onOpenChange={(o) => !o && setLifting(null)} title={t("unban")} description={lifting ? t("unbanConfirm", { ip: lifting.ip }) : ""}
        confirmLabel={t("unban")} onConfirm={onLift} danger={false} />
    </>
  );
}

// keep toast import used for future inline actions (tree-shaken)
void toast;
