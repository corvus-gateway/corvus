import { useEffect, useState } from "react";
import { ArrowLeftRight, CheckCircle2, FileCode2, History, Rocket, RotateCcw, XCircle } from "lucide-react";
import { api, type DeploymentPage, type Version } from "../api";
import { Pagination } from "../components/Pagination";
import { useApp } from "../lib/app";
import { ErrorBox, PageHeader, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Badge, Card, CodeBlock, EmptyState, Skeleton, type Tone } from "../components/ui/display";
import { Input, Field } from "../components/ui/form";
import { ConfirmDialog, Modal, Sheet } from "../components/ui/overlay";
import { useResource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n } from "../i18n";
import { cn, fmtDateTime, relTime } from "../lib/utils";

const tone: Record<string, Tone> = { active: "ok", failed: "danger", superseded: "neutral" };

export default function Deployments() {
  const { t, lang } = useI18n();
  const { can } = useCan();
  const { overview } = useApp();
  const [limit, setLimit] = useState(20);
  const [offset, setOffset] = useState(0);
  // Polls only the page being viewed.
  const res = useResource<DeploymentPage>(`/api/deployments?limit=${limit}&offset=${offset}`, { poll: 15000 });
  const [files, setFiles] = useState<{ v: Version; names: string[]; files: Record<string, string>; tab: string } | null>(null);
  const [failed, setFailed] = useState<Version | null>(null);
  const [rolling, setRolling] = useState<Version | null>(null);
  const [deploying, setDeploying] = useState<string | null>(null);
  const act = useSubmit();
  const view = useSubmit();

  const reload = () => { setOffset(0); res.reload(); window.dispatchEvent(new Event("corvus:refresh")); };

  const openFiles = async (v: Version) => {
    const r = await view.run(() => api.get<{ names: string[]; files: Record<string, string> }>(`/api/deployments/${v.id}/files`), { deployed: false });
    if (r) {
      const all = r.names?.length ? r.names : Object.keys(r.files);
      const names = [...all].sort((a, b) => Number(b === "nginx.conf") - Number(a === "nginx.conf"));
      setFiles({ v, names, files: r.files, tab: names[0] });
    }
  };
  const rollback = async () => {
    if (!rolling) return;
    const r = await act.run(() => api.post(`/api/deployments/${rolling.id}/rollback`), { success: t("rolledBack") });
    if (r !== undefined) reload();
  };
  const deploy = async () => {
    const r = await act.run(() => api.post("/api/deployments", deploying ? { message: deploying } : {}), { success: t("deployed") });
    if (r !== undefined) { setDeploying(null); reload(); }
  };

  const versions = res.data?.versions ?? [];
  const activeId = versions.find((v) => v.status === "active")?.id ?? overview?.active_version?.id;
  // Older backends omit `total`: infer whether another page may exist.
  const total = res.data?.total ?? offset + versions.length + (versions.length >= limit ? 1 : 0);
  useEffect(() => {
    if (res.data?.total != null && offset > 0 && offset >= res.data.total) setOffset(Math.max(0, Math.floor((res.data.total - 1) / limit) * limit));
  }, [res.data?.total, offset, limit]);
  const pager = (versions.length > 0 || offset > 0) && (
    <Pagination total={total} limit={limit} offset={offset} onOffset={setOffset} onLimit={setLimit} className="mt-4" />
  );

  return (
    <>
      <PageHeader title={t("navDeploy")} description={t("deployDesc")} actions={can && <Button variant="primary" onClick={() => { act.setError(null); setDeploying(""); }}><Rocket className="h-4 w-4" />{t("deployNow")}</Button>} />
      <ErrorBox error={deploying === null ? act.error : null} className="mb-4" />
      {res.loading ? <div className="space-y-3">{[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-20 rounded-xl" />)}</div>
        : res.error && !res.data ? <ErrorBox error={res.error} />
          : !versions.length && offset === 0 ? <Card><EmptyState icon={History} title={t("noDeployments")} description={t("noDeploymentsDesc")} /></Card> : (
            <ol className="relative space-y-3 before:absolute before:bottom-4 before:left-[19px] before:top-4 before:w-px before:bg-line">
              {versions.map((v) => {
                const isActive = v.id === activeId;
                return (
                  <li key={v.id} className="relative flex gap-4 animate-rise">
                    <span className={cn("relative z-10 mt-4 grid h-10 w-10 shrink-0 place-items-center rounded-full border bg-surface",
                      v.status === "failed" ? "border-danger/40 text-danger" : isActive ? "border-ok/40 text-ok" : "border-line text-fg-subtle")}>
                      {v.status === "failed" ? <XCircle className="h-4 w-4" /> : v.rollback_of ? <RotateCcw className="h-4 w-4" /> : isActive ? <CheckCircle2 className="h-4 w-4" /> : <ArrowLeftRight className="h-4 w-4" />}
                    </span>
                    <Card className={cn("min-w-0 flex-1 p-4", isActive && "border-ok/30")}>
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="num text-sm font-semibold">{t("versionN", { n: v.id })}</span>
                            <Badge tone={tone[v.status] ?? "neutral"} dot>{v.status === "active" ? t("statusActive") : v.status === "failed" ? t("statusFailed") : t("statusSuperseded")}</Badge>
                            {v.rollback_of != null && <Badge tone="warn" icon={RotateCcw}>{t("rollbackOf", { n: v.rollback_of })}</Badge>}
                          </div>
                          <p className="mt-1 text-sm text-fg-muted">{v.message || <span className="text-fg-subtle">{t("noMessage")}</span>}</p>
                          <p className="mt-1 text-xs text-fg-subtle" title={fmtDateTime(v.created_at, lang)}>{v.email || "system"} · {relTime(v.created_at, lang)} · {fmtDateTime(v.created_at, lang)}</p>
                        </div>
                        <div className="flex shrink-0 items-center gap-1.5">
                          {v.status === "failed" && <Button size="sm" variant="danger-ghost" onClick={() => setFailed(v)}><XCircle className="h-3.5 w-3.5" />{t("viewFailure")}</Button>}
                          <Button size="sm" loading={view.busy && files?.v.id === v.id} onClick={() => openFiles(v)}><FileCode2 className="h-3.5 w-3.5" />{t("viewFiles")}</Button>
                          {can && v.status !== "failed" && !isActive && <Button size="sm" onClick={() => setRolling(v)}><RotateCcw className="h-3.5 w-3.5" />{t("rollback")}</Button>}
                        </div>
                      </div>
                    </Card>
                  </li>
                );
              })}
            </ol>
          )}
      {!res.loading && pager}

      <Sheet open={!!files} onOpenChange={(o) => !o && setFiles(null)} width="max-w-4xl" title={files ? `${t("versionN", { n: files.v.id })} · ${t("generatedFiles")}` : ""} description={t("generatedFilesDesc")}>
        {files && (
          <div className="flex h-full flex-col">
            <div role="tablist" aria-label="files" className="flex shrink-0 gap-1 overflow-x-auto border-b border-line px-4 pt-3">
              {files.names.map((n) => (
                <button key={n} role="tab" aria-selected={files.tab === n} onClick={() => setFiles({ ...files, tab: n })}
                  className={cn("-mb-px shrink-0 whitespace-nowrap rounded-t-md border-b-2 px-3 py-2 font-mono text-xs transition", files.tab === n ? "border-brand text-fg" : "border-transparent text-fg-subtle hover:text-fg")}>{n}</button>
              ))}
            </div>
            <div className="p-4"><CodeBlock lines code={files.files[files.tab] ?? ""} maxHeight="calc(100dvh - 12rem)" /></div>
          </div>
        )}
      </Sheet>

      <Modal open={!!failed} onOpenChange={(o) => !o && setFailed(null)} width="max-w-2xl" title={failed ? `${t("versionN", { n: failed.id })} · ${t("deployFailed")}` : ""} description={t("deployFailedDesc")}
        footer={<Button onClick={() => setFailed(null)}>{t("close")}</Button>}>
        {failed && <CodeBlock tone="danger" code={failed.test_output || t("noOutput")} maxHeight="22rem" />}
      </Modal>

      <ConfirmDialog open={!!rolling} onOpenChange={(o) => !o && setRolling(null)} danger={false} title={rolling ? t("rollbackTitle", { n: rolling.id }) : ""} description={t("confirmRollback")} confirmLabel={t("rollback")} onConfirm={rollback} />

      <Modal open={deploying !== null} onOpenChange={(o) => !o && setDeploying(null)} title={t("deployNow")} description={t("deployNowDesc")}
        footer={<><Button onClick={() => setDeploying(null)}>{t("cancel")}</Button><Button variant="primary" loading={act.busy} onClick={deploy}><Rocket className="h-4 w-4" />{t("deployNow")}</Button></>}>
        <ErrorBox error={act.error} className="mb-4" />
        <Field label={t("message")} htmlFor="dmsg"><Input id="dmsg" value={deploying ?? ""} onChange={(e) => setDeploying(e.target.value)} placeholder={t("messagePlaceholder")} autoFocus /></Field>
      </Modal>
    </>
  );
}
