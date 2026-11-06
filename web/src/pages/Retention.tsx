import { useEffect, useRef, useState } from "react";
import { Archive, Download, Save } from "lucide-react";
import { api } from "../api";
import { BackupCard } from "../components/BackupCard";
import { ErrorBox, PageHeader, Toolbar, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Callout, Card, EmptyState, TableSkeleton } from "../components/ui/display";
import { Field, IdField, NumberInput, Segmented, fieldErr } from "../components/ui/form";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n } from "../i18n";
import { fmtBytes, fmtDateTime, fmtNum } from "../lib/utils";

interface Settings { log_mode: "off" | "notable" | "all"; log_days: number; log_max_mb: number; stats_days: number; events_days: number }
interface Usage { db_bytes: number; log_bytes: number; log_files: number; dropped_lines: number }
interface ArchiveFile { name: string; bytes: number; modified: string }

export default function Retention() {
  const { t, lang } = useI18n();
  const { isAdmin } = useCan();
  const res = useResource<{ settings: Settings; usage?: Usage }>("/api/settings/retention", { poll: 60000 });
  const files = useResource<ArchiveFile[]>("/api/traffic/archives", { poll: 60000 });
  const [s, setS] = useState<Settings | null>(null);
  const save = useSubmit();

  // refreshes replace the form only while it has no unsaved edits
  const synced = useRef("");
  useEffect(() => {
    if (!res.data) return;
    const next = res.data.settings;
    setS((cur) => (cur === null || JSON.stringify(cur) === synced.current ? next : cur));
    synced.current = JSON.stringify(next);
  }, [res.data]);
  const dirty = !!s && !!res.data && JSON.stringify(s) !== JSON.stringify(res.data.settings);
  const onSave = async () => {
    if (!s) return;
    const r = await save.run(() => api.put("/api/settings/retention", s), { success: t("retentionSaved") });
    if (r !== undefined) { res.reload(); files.reload(); }
  };
  const u = res.data?.usage;

  return (
    <>
      <PageHeader title={t("navRetention")} description={t("retentionDesc")} />
      <div className="space-y-6">
        <Card>
          <form onSubmit={(e) => { e.preventDefault(); void onSave(); }}>
            <fieldset disabled={!isAdmin} className="space-y-5 p-5">
              {res.error && !res.data ? <ErrorBox error={res.error} /> : !s ? <TableSkeleton cols={3} rows={3} /> : (
                <>
                  <Callout tone="info">{t("retentionHow")}</Callout>
                  <Field label={t("logMode")} hint={t("logModeDesc")} error={fieldErr(save.error, "retention")}>
                    <Segmented label={t("logMode")} value={s.log_mode} onChange={(v) => setS({ ...s, log_mode: v as Settings["log_mode"] })}
                      options={[{ value: "off", label: t("logModeOff") }, { value: "notable", label: t("logModeNotable") }, { value: "all", label: t("logModeAll") }]} />
                  </Field>
                  <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                    <IdField label={t("logDays")}>{(id) => <NumberInput id={id} min={1} max={365} value={s.log_days} onChange={(v) => setS({ ...s, log_days: v })} />}</IdField>
                    <IdField label={t("logMaxMb")} hint={t("logMaxHint")}>{(id) => <NumberInput id={id} min={50} max={1048576} value={s.log_max_mb} onChange={(v) => setS({ ...s, log_max_mb: v })} />}</IdField>
                    <IdField label={t("statsDays")}>{(id) => <NumberInput id={id} min={1} max={400} value={s.stats_days} onChange={(v) => setS({ ...s, stats_days: v })} />}</IdField>
                    <IdField label={t("eventsDays")}>{(id) => <NumberInput id={id} min={1} max={400} value={s.events_days} onChange={(v) => setS({ ...s, events_days: v })} />}</IdField>
                  </div>
                  {save.error != null && !fieldErr(save.error, "retention") && <ErrorBox error={save.error} />}
                  {isAdmin && <div className="flex justify-end"><Button type="submit" variant="primary" loading={save.busy} disabled={!dirty}><Save className="h-4 w-4" />{t("save")}</Button></div>}
                </>
              )}
            </fieldset>
          </form>
        </Card>

        {u && (
          <Card>
            <div className="grid gap-px bg-line sm:grid-cols-2">
              <div className="bg-surface p-5"><div className="text-xs text-fg-subtle">{t("dbSize")}</div><div className="mt-1 text-xl font-semibold num">{fmtBytes(u.db_bytes)}</div></div>
              <div className="bg-surface p-5"><div className="text-xs text-fg-subtle">{t("logSize")}</div><div className="mt-1 text-xl font-semibold num">{fmtBytes(u.log_bytes)}</div>
                <div className="text-xs text-fg-subtle">{t("logFilesN", { n: u.log_files })}{s ? ` · ${t("logLimit", { mb: fmtNum(s.log_max_mb) })}` : ""}</div></div>
            </div>
            {u.dropped_lines > 0 && <div className="p-4"><Callout tone="warn">{t("droppedLines", { n: fmtNum(u.dropped_lines) })}</Callout></div>}
          </Card>
        )}

        <Card>
          <Toolbar><h2 className="text-sm font-semibold">{t("archives")}</h2><span className="ml-auto text-xs text-fg-subtle num">{t("nItems", { n: files.data?.length ?? 0 })}</span></Toolbar>
          {files.loading ? <TableSkeleton cols={4} rows={4} /> : files.error && !files.data ? <div className="p-4"><ErrorBox error={files.error} /></div> : !files.data?.length ? (
            <EmptyState icon={Archive} title={t("noArchives")} description={t("noArchivesDesc")} />
          ) : (
            <Table>
              <THead><tr><Th>{t("fileName")}</Th><Th>{t("fileSize")}</Th><Th className="hidden sm:table-cell">{t("fileModified")}</Th><Th className="w-px"><span className="sr-only">{t("download")}</span></Th></tr></THead>
              <tbody>
                {files.data.map((f) => (
                  <Tr key={f.name}>
                    <Td className="font-mono text-xs">{f.name}</Td>
                    <Td className="num">{fmtBytes(f.bytes)}</Td>
                    <Td className="hidden text-fg-muted sm:table-cell">{fmtDateTime(f.modified, lang)}</Td>
                    <Td className="text-right"><a className="inline-flex items-center gap-1 whitespace-nowrap text-sm text-brand hover:underline" href={`/api/traffic/archives/${encodeURIComponent(f.name)}`} download><Download className="h-3.5 w-3.5" />{t("download")}</a></Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>

        {isAdmin && <BackupCard />}
      </div>
    </>
  );
}
