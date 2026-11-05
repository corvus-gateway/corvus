import { useEffect, useMemo, useState } from "react";
import { FileWarning, Globe, RotateCcw, Save } from "lucide-react";
import { api } from "../api";
import { ErrorBox, PageHeader, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Badge, Callout, Card, CardHeader, ComingSoon, Skeleton } from "../components/ui/display";
import { Field, IdField, Input, Segmented, Select, Textarea, fieldErr } from "../components/ui/form";
import { ConfirmDialog } from "../components/ui/overlay";
import { useResource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n, type Key } from "../i18n";

type PageName = "block" | "error";
interface PageView { custom: boolean; html: string; default_html: string }
type UnknownMode = "not_found" | "close" | "redirect";
interface UnknownHosts { mode: UnknownMode; redirect_url: string }
interface PagesResponse { pages: Record<PageName, PageView>; max_bytes: number; placeholders: string[]; unknown_hosts?: UnknownHosts }

const UNKNOWN_MODES: { value: UnknownMode; label: Key }[] = [
  { value: "not_found", label: "unknownNotFound" },
  { value: "close", label: "unknownClose" },
  { value: "redirect", label: "unknownRedirect" },
];

/** What the gateway answers for host names no proxy or redirection host serves. */
function UnknownHostsCard({ value, onSaved }: { value: UnknownHosts; onSaved: () => void }) {
  const { t } = useI18n();
  const { can } = useCan();
  const [mode, setMode] = useState<UnknownMode>(value.mode);
  const [url, setUrl] = useState(value.redirect_url ?? "");
  const save = useSubmit();
  useEffect(() => { setMode(value.mode); setUrl(value.redirect_url ?? ""); }, [value]);
  const dirty = mode !== value.mode || (mode === "redirect" && url.trim() !== (value.redirect_url ?? ""));

  const onSave = async () => {
    const r = await save.run(() => api.put("/api/settings/unknown-hosts", { mode, redirect_url: mode === "redirect" ? url.trim() : "" }), { success: t("savedDeployed") });
    if (r !== undefined) onSaved();
  };

  return (
    <Card>
      <CardHeader icon={Globe} title={t("unknownHosts")} description={t("unknownHostsDesc")} />
      <form className="px-4 pb-4 pt-2" onSubmit={(e) => { e.preventDefault(); onSave(); }}>
        <fieldset disabled={!can} className="min-w-0 space-y-4">
          <ErrorBox error={save.error} />
          <IdField label={t("unknownMode")} error={fieldErr(save.error, "mode")}>
            {(id) => (
              <Select id={id} value={mode} onChange={(e) => { setMode(e.target.value as UnknownMode); save.setError(null); }}>
                {UNKNOWN_MODES.map((m) => <option key={m.value} value={m.value}>{t(m.label)}</option>)}
              </Select>
            )}
          </IdField>
          {mode === "redirect" && (
            <IdField label={t("unknownRedirectUrl")} hint={t("unknownRedirectHint")} error={fieldErr(save.error, "redirect_url")} required>
              {(id) => <Input id={id} className="font-mono" inputMode="url" value={url} onChange={(e) => setUrl(e.target.value)}
                placeholder="https://www.example.com/" invalid={!!fieldErr(save.error, "redirect_url")} />}
            </IdField>
          )}
          {can && <div className="flex justify-end">
            <Button type="submit" variant="primary" loading={save.busy} disabled={!dirty || (mode === "redirect" && !url.trim())}><Save className="h-4 w-4" />{t("saveAndDeploy")}</Button>
          </div>}
        </fieldset>
      </form>
    </Card>
  );
}

const VARS: { name: string; label: Key }[] = [
  { name: "{{STATUS}}", label: "pageVarStatus" },
  { name: "{{RID}}", label: "pageVarRid" },
  { name: "{{HOST}}", label: "pageVarHost" },
  { name: "{{IP}}", label: "pageVarIp" },
  { name: "{{TIME}}", label: "pageVarTime" },
];

const escapeHtml = (s: string) => s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]!);

/** Substitutes sample values the way the gateway does (HTML-escaped; unknown placeholders render empty). */
function sample(html: string, page: PageName): string {
  const v: Record<string, string> = {
    STATUS: page === "block" ? "403" : "502", RID: "4f6c1e0b9a2d4c7e8b3a5f1d2c6e9a0b", HOST: "www.example.com",
    IP: "203.0.113.7", TIME: new Date().toUTCString(),
  };
  return html.replace(/\{\{([A-Z]+)\}\}/g, (_, k: string) => escapeHtml(v[k] ?? ""));
}

const bytes = (s: string) => new TextEncoder().encode(s).length;

export default function CustomPages() {
  const { t } = useI18n();
  const { can } = useCan();
  const res = useResource<PagesResponse>("/api/settings/pages");
  const [page, setPage] = useState<PageName>("block");
  const [html, setHtml] = useState<Record<PageName, string>>({ block: "", error: "" });
  const [confirm, setConfirm] = useState(false);
  const save = useSubmit();

  useEffect(() => {
    if (res.data) setHtml({ block: res.data.pages.block.html, error: res.data.pages.error.html });
  }, [res.data]);

  const view = res.data?.pages[page];
  const max = res.data?.max_bytes ?? 65536;
  const size = bytes(html[page]);
  const tooLarge = size > max;
  const dirty = !!view && html[page] !== view.html;
  const preview = useMemo(() => sample(html[page], page), [html, page]);

  const onSave = async () => {
    const r = await save.run(() => api.put(`/api/settings/pages/${page}`, { html: html[page] }), { success: t("savedDeployed") });
    if (r !== undefined) res.reload();
  };
  const onReset = async () => {
    const r = await save.run(() => api.post(`/api/settings/pages/${page}/reset`), { success: t("pageResetDone") });
    if (r !== undefined) res.reload();
  };

  return (
    <>
      <PageHeader title={t("navPages")} description={t("pagesDesc")} />
      {res.missing ? <Card><ComingSoon icon={FileWarning} /></Card> : res.loading ? (
        <div className="grid gap-5 lg:grid-cols-2"><Skeleton className="h-96 rounded-xl" /><Skeleton className="h-96 rounded-xl" /></div>
      ) : res.error && !res.data ? <ErrorBox error={res.error} /> : view && (
        <div className="space-y-5">
          <Segmented label={t("navPages")} className="max-w-md" value={page} onChange={(p) => { setPage(p); save.setError(null); }}
            options={[{ value: "block", label: t("blockPage") }, { value: "error", label: t("errorPage") }]} />
          <div className="grid items-start gap-5 lg:grid-cols-2">
            <Card>
              <CardHeader icon={FileWarning} title={page === "block" ? t("blockPage") : t("errorPage")}
                description={page === "block" ? t("blockPageDesc") : t("errorPageDesc")}
                actions={view.custom ? <Badge tone="brand">{t("pageCustom")}</Badge> : <Badge>{t("pageBuiltin")}</Badge>} />
              <form className="px-4 pb-4 pt-2" onSubmit={(e) => { e.preventDefault(); onSave(); }}>
                <fieldset disabled={!can} className="min-w-0 space-y-4">
                  {!can && <Callout tone="neutral">{t("readOnlyNotice")}</Callout>}
                  <ErrorBox error={save.error} />
                  <Field label={t("pageHtml")} error={tooLarge ? t("pageTooLarge", { max: Math.floor(max / 1024) }) : fieldErr(save.error, "html")}
                    hint={t("pageBytes", { n: (size / 1024).toFixed(1), max: Math.floor(max / 1024) })}>
                    <Textarea className="h-[28rem] font-mono text-xs leading-5" spellCheck={false} value={html[page]} invalid={tooLarge}
                      onChange={(e) => setHtml((h) => ({ ...h, [page]: e.target.value }))} />
                  </Field>
                  <div>
                    <div className="text-sm font-medium text-fg">{t("pagePlaceholders")}</div>
                    <p className="mb-2 text-xs text-fg-subtle">{t("pagePlaceholdersHint")}</p>
                    <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs">
                      {VARS.map((v) => (<div key={v.name} className="contents">
                        <dt className="font-mono text-fg">{v.name}</dt><dd className="text-fg-muted">{t(v.label)}</dd>
                      </div>))}
                    </dl>
                  </div>
                  {can && <div className="flex flex-wrap justify-end gap-2">
                    {view.custom && <Button type="button" onClick={() => setConfirm(true)} disabled={save.busy}><RotateCcw className="h-4 w-4" />{t("pageReset")}</Button>}
                    <Button type="submit" variant="primary" loading={save.busy} disabled={!dirty || tooLarge || !html[page].trim()}><Save className="h-4 w-4" />{t("saveAndDeploy")}</Button>
                  </div>}
                </fieldset>
              </form>
            </Card>
            <Card>
              <CardHeader title={t("pagePreview")} description={t("pagePreviewHint")} />
              <div className="px-4 pb-4 pt-2">
                {/* sandbox="" : no scripts, forms, navigation or same-origin access */}
                <iframe title={t("pagePreview")} sandbox="" srcDoc={preview} className="h-[34rem] w-full rounded-lg border border-line bg-white" />
              </div>
            </Card>
          </div>
          {res.data?.unknown_hosts && <div className="grid items-start gap-5 lg:grid-cols-2"><UnknownHostsCard value={res.data.unknown_hosts} onSaved={res.reload} /></div>}
        </div>
      )}
      <ConfirmDialog open={confirm} onOpenChange={setConfirm} title={t("pageResetTitle")} description={t("pageResetDesc")}
        confirmLabel={t("pageReset")} onConfirm={onReset} />
    </>
  );
}
