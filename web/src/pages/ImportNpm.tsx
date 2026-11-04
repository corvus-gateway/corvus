import { useState, type ReactNode } from "react";
import { Lock, AlertTriangle, ArrowRight, CheckCircle2, FileInput, FolderSearch, ListChecks, ShieldCheck, Globe } from "lucide-react";
import { toast } from "sonner";
import { api, deployedVersion, isMissing, type NpmPreview } from "../api";
import { ErrorBox, PageHeader, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Badge, Callout, Card, CardHeader, EmptyState } from "../components/ui/display";
import { Field, Input, fieldErr } from "../components/ui/form";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useCan } from "../lib/rbac";
import { useI18n } from "../i18n";
import { cn } from "../lib/utils";

const DEFAULT_PATH = "/import/npm/data";

function Check({ checked, onChange, label, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; disabled?: boolean }) {
  return (
    <input type="checkbox" aria-label={label} checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)}
      className="h-4 w-4 cursor-pointer rounded border-line-strong accent-[rgb(var(--brand-solid))] disabled:cursor-not-allowed disabled:opacity-40" />
  );
}

function Section({ icon, title, count, selected, onAll, children }: { icon: typeof Globe; title: string; count: number; selected: number; onAll: (v: boolean) => void; children: ReactNode }) {
  const { t } = useI18n();
  return (
    <Card>
      <CardHeader icon={icon} title={<span className="flex items-center gap-2">{title}<Badge>{selected}/{count}</Badge></span>}
        actions={count > 0 ? <><Button size="sm" variant="ghost" onClick={() => onAll(true)}>{t("selectAll")}</Button><Button size="sm" variant="ghost" onClick={() => onAll(false)}>{t("selectNone")}</Button></> : undefined} />
      {count === 0 ? <p className="px-4 pb-4 text-sm text-fg-subtle">{t("importNothing")}</p> : children}
    </Card>
  );
}

export default function ImportNpm() {
  const { isAdmin: admin } = useCan();
  const { t: tt } = useI18n();
  if (!admin) return (<><PageHeader title={tt("navImport")} description={tt("importDesc")} /><Card><EmptyState icon={Lock} title={tt("noPermissionTitle")} description={tt("adminOnlyNotice")} /></Card></>);
  return <ImportNpmInner />;
}

function ImportNpmInner() {
  const { t } = useI18n();
  const [path, setPath] = useState(DEFAULT_PATH);
  const [pv, setPv] = useState<NpmPreview | null>(null);
  const [sel, setSel] = useState<{ h: Set<number>; c: Set<number>; a: Set<number> }>({ h: new Set(), c: new Set(), a: new Set() });
  const [result, setResult] = useState<{ imported: Record<string, number>; skipped: unknown[]; version: number | null } | null>(null);
  const prev = useSubmit();
  const app = useSubmit();
  const missing = isMissing(prev.error);

  const preview = async () => {
    setResult(null);
    const r = await prev.run(() => api.post<NpmPreview>("/api/import/npm/preview", { path }), { deployed: false });
    if (!r) return;
    const norm: NpmPreview = { proxy_hosts: r.proxy_hosts ?? [], certificates: r.certificates ?? [], access_lists: r.access_lists ?? [], warnings: r.warnings ?? [] };
    setPv(norm);
    setSel({ h: new Set(norm.proxy_hosts.filter((x) => !x.conflict).map((x) => x.npm_id)), c: new Set(norm.certificates.map((x) => x.npm_id)), a: new Set(norm.access_lists.map((x) => x.npm_id)) });
  };
  const apply = async () => {
    const r = await app.run(() => api.post<{ imported: Record<string, number>; skipped: unknown[]; version: unknown }>("/api/import/npm/apply", {
      path, proxy_host_ids: [...sel.h], certificate_ids: [...sel.c], access_list_ids: [...sel.a],
    }), { deployed: false });
    if (!r) return;
    const v = deployedVersion(r);
    setResult({ imported: r.imported ?? {}, skipped: r.skipped ?? [], version: v });
    toast.success(v != null ? `${t("importDone")} · ${t("versionN", { n: v })}` : t("importDone"));
    setPv(null);
    window.dispatchEvent(new Event("corvus:refresh"));
  };

  const toggle = (k: "h" | "c" | "a", id: number, v: boolean) => setSel((s) => { const n = new Set(s[k]); if (v) n.add(id); else n.delete(id); return { ...s, [k]: n }; });
  const total = sel.h.size + sel.c.size + sel.a.size;

  return (
    <>
      <PageHeader title={t("navImport")} description={t("importDesc")} />
      <Card className="mb-5">
        <div className="space-y-4 p-4">
          <form className="flex flex-col gap-3 sm:flex-row sm:items-end" onSubmit={(e) => { e.preventDefault(); preview(); }}>
            <Field label={t("npmPath")} hint={t("npmPathHint")} error={fieldErr(prev.error, "path")} className="flex-1" htmlFor="npm-path">
              <Input id="npm-path" className="font-mono" value={path} onChange={(e) => setPath(e.target.value)} placeholder={DEFAULT_PATH} />
            </Field>
            <Button type="submit" variant="primary" loading={prev.busy} disabled={!path.trim()} className="sm:mb-[22px]"><FolderSearch className="h-4 w-4" />{t("preview")}</Button>
          </form>
          {missing ? <Callout tone="warn" title={t("comingSoon")}>{t("comingSoonDesc")}</Callout> : !fieldErr(prev.error, "path") && <ErrorBox error={prev.error} />}
        </div>
      </Card>

      {result && (
        <Card className="border-ok/30">
          <div className="p-5">
            <div className="flex items-center gap-3"><span className="grid h-10 w-10 place-items-center rounded-full bg-ok/15 text-ok"><CheckCircle2 className="h-5 w-5" /></span>
              <div><div className="font-semibold">{t("importDone")}{result.version != null && ` · ${t("versionN", { n: result.version })}`}</div><div className="text-sm text-fg-subtle">{t("importDoneDesc")}</div></div></div>
            <dl className="mt-4 grid grid-cols-3 gap-3 text-center">
              {([["proxy_hosts", t("navHosts")], ["certificates", t("navCerts")], ["access_lists", t("navAccessLists")]] as const).map(([k, l]) => (
                <div key={k} className="rounded-lg border border-line bg-surface-2/50 p-3"><dd className="num text-2xl font-semibold">{result.imported[k] ?? 0}</dd><dt className="text-xs text-fg-subtle">{l}</dt></div>
              ))}
            </dl>
            {result.skipped.length > 0 && <div className="mt-4"><div className="mb-1.5 text-xs font-medium text-fg-subtle">{t("skippedN", { n: result.skipped.length })}</div>
              <ul className="space-y-1 text-xs text-fg-muted">{result.skipped.slice(0, 20).map((s, i) => <li key={i} className="font-mono">{typeof s === "string" ? s : JSON.stringify(s)}</li>)}</ul></div>}
          </div>
        </Card>
      )}

      {!pv && !result && !prev.busy && !prev.error && (
        <Card><EmptyState icon={FileInput} title={t("importEmptyTitle")} description={t("importEmptyDesc")} /></Card>
      )}

      {pv && (
        <div className="space-y-5">
          {pv.warnings.length > 0 && <Callout tone="warn" icon={AlertTriangle} title={t("warningsN", { n: pv.warnings.length })}><ul className="mt-1 list-disc space-y-0.5 pl-4">{pv.warnings.map((w, i) => <li key={i}>{w}</li>)}</ul></Callout>}

          <Section icon={Globe} title={t("navHosts")} count={pv.proxy_hosts.length} selected={sel.h.size} onAll={(v) => setSel((s) => ({ ...s, h: new Set(v ? pv.proxy_hosts.filter((x) => !x.conflict).map((x) => x.npm_id) : []) }))}>
            <Table>
              <THead><tr><Th className="w-10"><span className="sr-only">{t("select")}</span></Th><Th>{t("domains")}</Th><Th className="hidden md:table-cell">{t("forwardTo")}</Th><Th>{t("status")}</Th></tr></THead>
              <tbody>
                {pv.proxy_hosts.map((h) => (
                  <Tr key={h.npm_id} className={cn(h.conflict && "bg-warn/5")}>
                    <Td><Check label={h.domains.join(", ")} checked={sel.h.has(h.npm_id)} onChange={(v) => toggle("h", h.npm_id, v)} /></Td>
                    <Td><div className="font-medium">{h.domains[0]}</div>{h.domains.length > 1 && <div className="text-xs text-fg-subtle">{h.domains.slice(1).join(", ")}</div>}</Td>
                    <Td className="hidden font-mono text-xs text-fg-muted md:table-cell"><span className="inline-flex items-center gap-1.5"><ArrowRight className="h-3 w-3" />{h.forward ?? "–"}</span></Td>
                    <Td>{h.conflict ? <Badge tone="warn" icon={AlertTriangle}>{typeof h.conflict === "string" ? h.conflict : t("conflict")}</Badge> : <Badge tone="ok" dot>{t("importable")}</Badge>}</Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          </Section>

          <Section icon={ShieldCheck} title={t("navCerts")} count={pv.certificates.length} selected={sel.c.size} onAll={(v) => setSel((s) => ({ ...s, c: new Set(v ? pv.certificates.map((x) => x.npm_id) : []) }))}>
            <Table>
              <tbody>
                {pv.certificates.map((c) => (
                  <Tr key={c.npm_id}><Td className="w-10"><Check label={c.name ?? String(c.npm_id)} checked={sel.c.has(c.npm_id)} onChange={(v) => toggle("c", c.npm_id, v)} /></Td>
                    <Td><span className="font-medium">{c.name ?? c.nice_name ?? `#${c.npm_id}`}</span>{c.domains?.length ? <span className="ml-2 font-mono text-xs text-fg-subtle">{c.domains.join(", ")}</span> : null}</Td></Tr>
                ))}
              </tbody>
            </Table>
          </Section>

          <Section icon={ListChecks} title={t("navAccessLists")} count={pv.access_lists.length} selected={sel.a.size} onAll={(v) => setSel((s) => ({ ...s, a: new Set(v ? pv.access_lists.map((x) => x.npm_id) : []) }))}>
            <Table>
              <tbody>
                {pv.access_lists.map((a) => (
                  <Tr key={a.npm_id}><Td className="w-10"><Check label={a.name ?? String(a.npm_id)} checked={sel.a.has(a.npm_id)} onChange={(v) => toggle("a", a.npm_id, v)} /></Td><Td className="font-medium">{a.name ?? `#${a.npm_id}`}</Td></Tr>
                ))}
              </tbody>
            </Table>
          </Section>

          {fieldErr(app.error, "path") ? <ErrorBox error={new Error(fieldErr(app.error, "path"))} /> : <ErrorBox error={app.error} />}
          <div className="sticky bottom-0 -mx-4 flex items-center justify-between gap-3 border-t border-line bg-surface px-4 py-3 sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
            <span className="text-sm text-fg-muted">{t("importSelected", { n: total })}</span>
            <Button variant="primary" loading={app.busy} disabled={total === 0} onClick={apply}><FileInput className="h-4 w-4" />{t("importNow")}</Button>
          </div>
        </div>
      )}
    </>
  );
}
