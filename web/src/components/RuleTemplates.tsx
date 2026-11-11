// Virtual-patch templates (create a security rule from a CVE template) and the managed
// rule bundle status card.
import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { FileUp, PackageCheck, RefreshCw, Save } from "lucide-react";
import { api, type IpList, type ProxyHost } from "../api";
import { ErrorBox, useSubmit } from "./common";
import { ErrorHint } from "./ErrorHint";
import { Button } from "./ui/button";
import { Badge, Callout, Card, TableSkeleton } from "./ui/display";
import { Field, FormSection, IdField, Input, Segmented, Select, SwitchRow, fieldErr } from "./ui/form";
import { Sheet } from "./ui/overlay";
import { MultiPicker } from "./ui/picker";
import { useResource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n, type Lang } from "../i18n";
import { cn, fmtDateTime } from "../lib/utils";

interface Template {
  id: string; cves: string[]; severity: string; products: string;
  title: Record<string, string>; description: Record<string, string>; false_positives: Record<string, string>;
  needs_exempt: boolean;
}
interface BundleStatus {
  serial: number; version: string; source: "builtin" | "installed"; signatures: number; templates: number;
  installed_at?: string; checked_at?: string; error?: string; error_hint?: string; settings: { url: string; auto_update: boolean };
}

const pickLang = (m: Record<string, string> | undefined, lang: Lang) => (m ? m[lang] ?? m.en ?? "" : "");

export function TemplateSheet({ open, onClose, hosts, lists, onCreated }: {
  open: boolean; onClose: () => void; hosts: ProxyHost[] | null; lists: IpList[] | null; onCreated: () => void;
}) {
  const { t, lang } = useI18n();
  const res = useResource<Template[]>(open ? "/api/rule-templates" : null);
  const [sel, setSel] = useState<string>("");
  const [action, setAction] = useState<"log" | "block">("log");
  const [scope, setScope] = useState<"all" | "hosts">("all");
  const [hostIds, setHostIds] = useState<string[]>([]);
  const [exempt, setExempt] = useState<string>("");
  const sub = useSubmit();
  useEffect(() => { if (open) { setSel(""); setAction("log"); setScope("all"); setHostIds([]); setExempt(""); sub.setError(null); } }, [open]); // eslint-disable-line react-hooks/exhaustive-deps
  const tpl = useMemo(() => res.data?.find((x) => x.id === sel), [res.data, sel]);

  const create = async () => {
    if (!tpl) return;
    const r = await sub.run(() => api.post("/api/security-rules/from-template", {
      template_id: tpl.id, action, status_code: 403, scope, host_ids: scope === "hosts" ? hostIds.map(Number) : [],
      exempt_list_id: tpl.needs_exempt && exempt ? Number(exempt) : 0,
    }), { success: t("tplCreated") });
    if (r !== undefined) { onCreated(); onClose(); }
  };

  return (
    <Sheet open={open} onOpenChange={(o) => !o && onClose()} onSubmit={create} width="max-w-3xl" title={t("templatesTitle")} description={t("templatesDesc")}
      footer={<><Button onClick={onClose}>{t("cancel")}</Button><Button type="submit" variant="primary" loading={sub.busy} disabled={!tpl}>{t("tplApply")}</Button></>}>
      <FormSection title={t("tplSelect")}>
        {res.loading ? <TableSkeleton cols={2} rows={5} /> : res.error && !res.data ? <ErrorBox error={res.error} /> : (
          <ul className="max-h-[22rem] space-y-2 overflow-y-auto pr-1" role="listbox" aria-label={t("tplSelect")}>
            {(res.data ?? []).map((x) => (
              <li key={x.id}>
                <button type="button" role="option" aria-selected={sel === x.id} onClick={() => setSel(x.id)}
                  className={cn("w-full rounded-md border px-3 py-2.5 text-left transition", sel === x.id ? "border-brand bg-brand/5" : "border-line hover:border-fg-subtle")}>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium">{pickLang(x.title, lang)}</span>
                    <Badge tone={x.severity === "critical" ? "danger" : "warn"}>{x.severity === "critical" ? t("tplSevCritical") : t("tplSevHigh")}</Badge>
                    {x.cves.map((c) => <span key={c} className="font-mono text-2xs text-fg-subtle">{c}</span>)}
                  </div>
                  <p className="mt-1 text-xs text-fg-muted">{pickLang(x.description, lang)}</p>
                  <p className="mt-1 text-2xs text-fg-subtle">{t("tplProducts")}: {x.products}</p>
                </button>
              </li>
            ))}
          </ul>
        )}
      </FormSection>
      {tpl && (
        <FormSection title={t("secThen")}>
          {sub.error != null && <ErrorBox error={sub.error} />}
          <Callout tone="info"><b>{t("tplFalsePositives")}:</b> {pickLang(tpl.false_positives, lang)}</Callout>
          <Field label={t("secThen")}>
            <Segmented label={t("secThen")} value={action} onChange={(v) => setAction(v as "log" | "block")}
              options={[{ value: "log", label: t("actLog") }, { value: "block", label: t("actBlock") }]} />
          </Field>
          <Field label={t("scope")} error={fieldErr(sub.error, "host_ids")}>
            <Segmented label={t("scope")} value={scope} onChange={(v) => setScope(v as "all" | "hosts")}
              options={[{ value: "all", label: t("allHosts") }, { value: "hosts", label: t("selectedHosts") }]} />
          </Field>
          {scope === "hosts" && (
            <MultiPicker label={t("selectedHosts")} value={hostIds} onChange={setHostIds}
              options={(hosts ?? []).map((h) => ({ value: String(h.id), label: h.domains[0] ?? `#${h.id}` }))} />
          )}
          {tpl.needs_exempt && (
            <IdField label={t("tplExemptList")} hint={t("tplExemptHint")}>{(id) => (
              <Select id={id} value={exempt} onChange={(e) => setExempt(e.target.value)}>
                <option value="">{t("tplExemptNone")}</option>
                {(lists ?? []).filter((l) => (l.kind ?? "ip") === "ip").map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
              </Select>
            )}</IdField>
          )}
        </FormSection>
      )}
    </Sheet>
  );
}

export function ManagedRulesCard() {
  const { t, lang } = useI18n();
  const { isAdmin } = useCan();
  const res = useResource<BundleStatus>("/api/managed-rules", { poll: 120000 });
  const [url, setUrl] = useState("");
  const [auto, setAuto] = useState(false);
  const [bundle, setBundle] = useState<File | null>(null);
  const [sig, setSig] = useState<File | null>(null);
  const save = useSubmit();
  const check = useSubmit();
  const upload = useSubmit();
  // refreshes replace the fields only while they have no unsaved edits
  const synced = useRef<{ url: string; auto: boolean } | null>(null);
  useEffect(() => {
    if (!res.data) return;
    const next = { url: res.data.settings.url ?? "", auto: !!res.data.settings.auto_update };
    const prev = synced.current;
    setUrl((cur) => (!prev || cur === prev.url ? next.url : cur));
    setAuto((cur) => (!prev || cur === prev.auto ? next.auto : cur));
    synced.current = next;
  }, [res.data]);
  const d = res.data;
  const dirty = !!d && (url !== (d.settings.url ?? "") || auto !== !!d.settings.auto_update);

  const onSave = async () => { const r = await save.run(() => api.put("/api/settings/managed-rules", { url: url.trim(), auto_update: auto }), { success: t("mrSaved") }); if (r !== undefined) res.reload(); };
  const onCheck = async () => { const r = await check.run(() => api.post("/api/managed-rules/check", {}), { success: t("mrUpToDate") }); if (r !== undefined) res.reload(); };
  const onUpload = async () => {
    if (!bundle || !sig) return;
    const body = { bundle: await bundle.text(), signature: await sig.text() };
    const r = await upload.run(() => api.post("/api/managed-rules/upload", body), { success: t("mrUploaded") });
    if (r !== undefined) { setBundle(null); setSig(null); res.reload(); }
  };

  return (
    <Card className="mt-6">
      <div className="flex items-start gap-3 p-5">
        <PackageCheck className="mt-0.5 h-5 w-5 shrink-0 text-fg-subtle" aria-hidden />
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-semibold">{t("managedRules")}</h2>
          <p className="mt-0.5 text-xs text-fg-subtle">{t("managedRulesDesc")}</p>
          {res.error && !d ? <div className="mt-3"><ErrorBox error={res.error} /></div> : !d ? <div className="mt-3"><TableSkeleton cols={4} rows={1} /></div> : (
            <>
              <dl className="mt-4 grid grid-cols-2 gap-4 text-sm sm:grid-cols-5">
                <div><dt className="text-xs text-fg-subtle">{t("mrVersion")}</dt><dd className="font-mono">{d.version}</dd></div>
                <div><dt className="text-xs text-fg-subtle">{t("mrSource")}</dt><dd><Badge tone={d.source === "installed" ? "ok" : "neutral"}>{d.source === "installed" ? t("mrInstalled") : t("mrBuiltin")}</Badge></dd></div>
                <div><dt className="text-xs text-fg-subtle">{t("mrSignatures")}</dt><dd className="num">{d.signatures}</dd></div>
                <div><dt className="text-xs text-fg-subtle">{t("mrTemplates")}</dt><dd className="num">{d.templates}</dd></div>
                <div><dt className="text-xs text-fg-subtle">{t("mrChecked")}</dt><dd className="text-fg-muted">{d.checked_at ? fmtDateTime(d.checked_at, lang) : "–"}</dd></div>
              </dl>
              {d.error && <div className="mt-3"><Callout tone="warn">{d.error}<ErrorHint code={d.error_hint} className="mt-1.5" /></Callout></div>}
              {isAdmin && (
                <div className="mt-5 grid gap-5 lg:grid-cols-2">
                  <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); void onSave(); }}>
                    <Field label={t("mrUrl")} hint={t("mrUrlHint")} error={fieldErr(save.error, "url")}>
                      <Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://rules.example.com/corvus/bundle.json" className="font-mono" />
                    </Field>
                    <SwitchRow label={t("mrAuto")} checked={auto} onChange={setAuto} />
                    {(save.error != null && !fieldErr(save.error, "url")) || check.error != null ? <ErrorBox error={save.error ?? check.error} /> : null}
                    <div className="flex gap-2">
                      <Button type="submit" size="sm" variant="primary" loading={save.busy} disabled={!dirty}><Save className="h-3.5 w-3.5" />{t("save")}</Button>
                      <Button type="button" size="sm" loading={check.busy} disabled={!d.settings.url} onClick={onCheck}><RefreshCw className="h-3.5 w-3.5" />{t("mrCheckNow")}</Button>
                    </div>
                  </form>
                  <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); void onUpload(); }}>
                    <p className="text-xs text-fg-subtle">{t("mrUploadHint")}</p>
                    <Field label={t("mrBundleFile")}><input type="file" accept=".json,application/json" onChange={(e) => setBundle(e.target.files?.[0] ?? null)} className="block w-full text-xs" /></Field>
                    <Field label={t("mrSigFile")}><input type="file" accept=".sig,text/plain" onChange={(e) => setSig(e.target.files?.[0] ?? null)} className="block w-full text-xs" /></Field>
                    {upload.error != null && <ErrorBox error={upload.error} />}
                    <Button type="submit" size="sm" loading={upload.busy} disabled={!bundle || !sig}><FileUp className="h-3.5 w-3.5" />{t("mrUpload")}</Button>
                  </form>
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </Card>
  );
}

void toast;
