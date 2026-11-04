import { useRef, useState } from "react";
import { toast } from "sonner";
import { AlertTriangle, DatabaseBackup, Download, RotateCcw, Upload, X } from "lucide-react";
import { api, ApiError } from "../api";
import { ErrorBox, useSubmit } from "./common";
import { Button } from "./ui/button";
import { Callout, Card, CardHeader, CodeBlock } from "./ui/display";
import { ConfirmDialog } from "./ui/overlay";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { fmtBytes, fmtDateTime, fmtNum } from "../lib/utils";

interface Summary { version: string; created_at: string; db_engine: string; has_db: boolean; files: number; bytes: number; secrets: number; certs: number }
interface RestoreStatus { state: "" | "uploaded" | "pending"; summary?: Summary; current_engine: string; current_version: string; restart_command?: string }

/** Multipart upload with the console's CSRF header (api.ts only sends JSON). */
async function uploadBackup(file: File): Promise<RestoreStatus> {
  const fd = new FormData();
  fd.append("file", file);
  const res = await fetch("/api/backup/restore", { method: "POST", body: fd, credentials: "same-origin", headers: { "X-Corvus-Request": "1", Accept: "application/json" } });
  let data: any = null;
  try { data = await res.json(); } catch { /* empty or not JSON */ }
  if (!res.ok) throw new ApiError(res.status, data);
  return data as RestoreStatus;
}

/** Backup download and staged restore (admin only; shown on the data retention page). */
export function BackupCard() {
  const { t, lang } = useI18n();
  const status = useResource<RestoreStatus>("/api/backup/restore");
  const fileRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [confirm, setConfirm] = useState(false);
  const up = useSubmit();
  const act = useSubmit();
  const st = status.data;
  const sum = st?.summary;

  const onUpload = async () => {
    if (!file) return;
    const r = await up.run(() => uploadBackup(file), { success: t("restoreChecked"), deployed: false, silentError: true });
    if (r) { setFile(null); if (fileRef.current) fileRef.current.value = ""; status.reload(); }
  };
  const onConfirm = async () => {
    const r = await act.run(() => api.post("/api/backup/restore/confirm"), { success: t("restoreStaged"), deployed: false });
    if (r !== undefined) status.reload();
  };
  const onDiscard = async () => {
    await act.run(() => api.del("/api/backup/restore"), { success: t("restoreDiscarded"), deployed: false });
    status.reload();
  };
  const onDownload = () => { toast.info(t("backupStarted")); };

  return (
    <Card>
      <CardHeader icon={DatabaseBackup} title={t("backupTitle")} description={t("backupDesc")} />
      <div className="space-y-5 p-4 pt-2">
        <Callout tone="warn" icon={AlertTriangle} title={t("backupSecretsTitle")}>{t("backupSecretsWarn")}</Callout>
        {st?.current_engine === "postgres" && <Callout tone="info">{t("backupPgNote")}</Callout>}
        <p className="text-xs text-fg-subtle">{t("backupExcluded")}</p>
        <div>
          <a href="/api/backup" download onClick={onDownload}
            className="inline-flex h-8 items-center gap-2 rounded-[3px] border border-transparent bg-brand-solid px-3.5 text-sm text-on-brand hover:brightness-90 dark:hover:brightness-110">
            <Download className="h-4 w-4" />{t("backupDownload")}
          </a>
        </div>

        <div className="border-t border-line pt-4">
          <h3 className="text-sm font-semibold">{t("restoreTitle")}</h3>
          <p className="mt-1 text-xs text-fg-subtle">{t("restoreHow")}</p>

          {status.error != null && !st ? <ErrorBox error={status.error} className="mt-3" /> : st?.state === "pending" ? (
            <div className="mt-3 space-y-3">
              <Callout tone="ok" icon={RotateCcw} title={t("restorePending")}>{t("restorePendingDesc")}</Callout>
              <CodeBlock code={st.restart_command ?? "docker compose restart"} />
              <Button variant="danger-ghost" size="sm" loading={act.busy} onClick={onDiscard}><X className="h-3.5 w-3.5" />{t("restoreCancel")}</Button>
            </div>
          ) : st?.state === "uploaded" && sum ? (
            <div className="mt-3 space-y-3">
              <dl className="grid grid-cols-2 gap-x-6 gap-y-2 rounded-lg border border-line bg-surface-2 p-3 text-sm sm:grid-cols-3">
                <div><dt className="text-xs text-fg-subtle">{t("restoreVersion")}</dt><dd className="font-mono text-xs">{sum.version || "?"}</dd></div>
                <div><dt className="text-xs text-fg-subtle">{t("restoreCreated")}</dt><dd>{fmtDateTime(sum.created_at, lang)}</dd></div>
                <div><dt className="text-xs text-fg-subtle">{t("restoreEngine")}</dt><dd>{sum.db_engine === "postgres" ? "PostgreSQL" : "SQLite"}</dd></div>
                <div><dt className="text-xs text-fg-subtle">{t("restoreFiles")}</dt><dd className="num">{fmtNum(sum.files)} · {fmtBytes(sum.bytes)}</dd></div>
                <div><dt className="text-xs text-fg-subtle">{t("restoreCerts")}</dt><dd className="num">{fmtNum(sum.certs)}</dd></div>
                <div><dt className="text-xs text-fg-subtle">{t("restoreSecrets")}</dt><dd className="num">{fmtNum(sum.secrets)}</dd></div>
              </dl>
              {!sum.has_db && <Callout tone="info">{t("restoreNoDb")}</Callout>}
              {sum.version !== st.current_version && <Callout tone="info">{t("restoreVersionDiffers", { v: sum.version || "?", cur: st.current_version })}</Callout>}
              {act.error != null && <ErrorBox error={act.error} />}
              <div className="flex flex-wrap gap-2">
                <Button variant="danger" loading={act.busy} onClick={() => setConfirm(true)}><RotateCcw className="h-4 w-4" />{t("restoreConfirm")}</Button>
                <Button variant="ghost" disabled={act.busy} onClick={onDiscard}>{t("cancel")}</Button>
              </div>
            </div>
          ) : (
            <div className="mt-3 space-y-3">
              <div className="flex flex-wrap items-center gap-2">
                <input ref={fileRef} type="file" accept=".gz,.tgz,application/gzip" aria-label={t("restoreChoose")}
                  onChange={(e) => { setFile(e.target.files?.[0] ?? null); up.setError(null); }}
                  className="max-w-full text-sm file:mr-3 file:h-8 file:rounded-[3px] file:border file:border-brand file:bg-transparent file:px-3 file:text-sm file:text-brand" />
                <Button variant="secondary" loading={up.busy} disabled={!file} onClick={onUpload}><Upload className="h-4 w-4" />{t("restoreUpload")}</Button>
              </div>
              {up.error != null && <ErrorBox error={up.error} />}
            </div>
          )}
        </div>
      </div>
      <ConfirmDialog open={confirm} onOpenChange={setConfirm} title={t("restoreConfirm")} description={t("restoreConfirmWarn")}
        confirmLabel={t("restoreConfirm")} onConfirm={onConfirm} />
    </Card>
  );
}
