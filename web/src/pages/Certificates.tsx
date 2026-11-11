import { useEffect, useMemo, useState } from "react";
import { AlertCircle, Check, CheckCircle2, Cloud, FileKey, Globe, Loader2, RefreshCw, ShieldCheck, Sparkles, Trash2, Upload } from "lucide-react";
import { ApiError, api, isMissing, type Certificate } from "../api";
import { ErrorBox, PageHeader, Toolbar, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Badge, Callout, Card, EmptyState, TableSkeleton } from "../components/ui/display";
import { Field, FormSection, IdField, Input, SearchInput, SwitchRow, TagInput, Textarea, fieldErr } from "../components/ui/form";
import { ConfirmDialog, RowMenu, Sheet, Tip } from "../components/ui/overlay";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource } from "../hooks/useResource";
import { useDnsProviders } from "../hooks/useDnsProviders";
import { DnsFields, dnsReady, dnsRequest, newDnsChoice, useDnsCredentials, type DnsChoice } from "../components/DnsFields";
import { DnsCredentialsCard } from "../components/DnsCredentialsCard";
import { useCan } from "../lib/rbac";
import { localeTag, useI18n } from "../i18n";
import { cn, daysUntil } from "../lib/utils";
import { ErrorHint } from "../components/ErrorHint";

type Challenge = "http-01" | "dns-01";
interface Acme { domains: string[]; email: string; challenge: Challenge; dnsc: DnsChoice; staging: boolean }
const acmeBlank: Acme = { domains: [], email: "", challenge: "http-01", dnsc: { provider: "cloudflare", creds: "", name: "" }, staging: false };

function statusOf(c: Certificate): "valid" | "pending" | "error" {
  return c.status ?? "valid";
}

export default function Certificates() {
  const { t, lang } = useI18n();
  const { can } = useCan();
  const [q, setQ] = useState("");
  const [poll, setPoll] = useState(0);
  const list = useResource<Certificate[]>("/api/certificates", { poll: poll || 30000 });
  const anyPending = list.data?.some((c) => c.status === "pending") ?? false;
  useEffect(() => { setPoll(anyPending ? 3000 : 0); }, [anyPending]);

  // id set: replace that uploaded certificate in place (PUT) instead of uploading a new one.
  const [upload, setUpload] = useState<{ id?: number; name: string; certificate: string; private_key: string } | null>(null);
  const [wizard, setWizard] = useState<{ step: 0 | 1 | 2; form: Acme } | null>(null);
  const [deleting, setDeleting] = useState<Certificate | null>(null);
  const providers = useDnsProviders();
  const creds = useDnsCredentials();
  const up = useSubmit();
  const ac = useSubmit();
  const row = useSubmit();

  const rows = useMemo(() => (list.data ?? []).filter((c) => `${c.name} ${c.domains.join(" ")}`.toLowerCase().includes(q.trim().toLowerCase())), [list.data, q]);
  const acmeMissing = isMissing(ac.error);

  const doUpload = async () => {
    if (!upload) return;
    const { id, ...body } = upload;
    const r = await up.run(() => (id ? api.put(`/api/certificates/${id}`, body) : api.post("/api/certificates", body)),
      id ? { success: t("certReplaced") } : { success: t("certUploaded"), deployed: false });
    if (r !== undefined) { setUpload(null); list.reload(); window.dispatchEvent(new Event("corvus:refresh")); }
  };
  const doAcme = async () => {
    if (!wizard) return;
    const f = wizard.form;
    const body: Record<string, unknown> = { domains: f.domains, email: f.email, challenge: f.challenge, staging: f.staging };
    if (f.challenge === "dns-01") Object.assign(body, dnsRequest(f.dnsc));
    let failed: unknown;
    const r = await ac.run(() => api.post("/api/certificates/acme", body).catch((e) => { failed = e; throw e; }), { success: t("acmeStarted"), deployed: false });
    if (r !== undefined) { setWizard(null); list.reload(); creds.reload(); return; }
    // field errors are shown next to their field: go back to the step that has it
    const fields = failed instanceof ApiError ? failed.fields.map((x) => x.field) : [];
    const step = fields.some((x) => x === "domains" || x === "email") ? 0 : fields.some((x) => x.startsWith("dns_")) ? 1 : null;
    if (step !== null) setWizard((x) => (x ? { ...x, step } : x));
  };
  const renew = async (c: Certificate) => {
    const r = await row.run(() => api.post(`/api/certificates/${c.id}/renew`), { success: t("renewStarted"), deployed: false });
    if (r !== undefined) list.reload();
  };
  const remove = async () => {
    if (!deleting) return;
    const r = await row.run(() => api.del(`/api/certificates/${deleting.id}`), { success: t("certDeleted"), deployed: false });
    if (r !== undefined) { list.reload(); window.dispatchEvent(new Event("corvus:refresh")); }
  };

  const w = wizard;
  const wf = w?.form;
  const wildcard = wf?.domains.some((d) => d.startsWith("*.")) ?? false;
  const step0Ok = !!wf && wf.domains.length > 0 && /.+@.+\..+/.test(wf.email);
  const step1Ok = !!wf && (wf.challenge === "http-01" ? !wildcard : dnsReady(wf.dnsc));
  const setW = (p: Partial<Acme>) => setWizard((x) => (x ? { ...x, form: { ...x.form, ...p } } : x));
  const steps = [t("acmeStepDomains"), t("acmeStepChallenge"), t("acmeStepReview")];

  return (
    <>
      <PageHeader title={t("navCerts")} description={t("certsDesc")} actions={can && <>
        <Button onClick={() => { up.setError(null); setUpload({ name: "", certificate: "", private_key: "" }); }}><Upload className="h-4 w-4" />{t("uploadCert")}</Button>
        <Button variant="primary" onClick={() => { ac.setError(null); setWizard({ step: 0, form: { ...acmeBlank } }); }}><Sparkles className="h-4 w-4" />{t("issueAcme")}</Button>
      </>} />
      <Card>
        <Toolbar><SearchInput className="w-full sm:w-64" value={q} onChange={setQ} /><span className="ml-auto text-xs text-fg-subtle num">{t("nItems", { n: rows.length })}</span></Toolbar>
        {list.loading ? <TableSkeleton cols={5} /> : list.error && !list.data ? <div className="p-4"><ErrorBox error={list.error} /></div> : !list.data?.length ? (
          <EmptyState icon={ShieldCheck} title={t("noCerts")} description={t("noCertsDesc")} action={<div className="flex gap-2"><Button onClick={() => setUpload({ name: "", certificate: "", private_key: "" })}><Upload className="h-4 w-4" />{t("uploadCert")}</Button><Button variant="primary" onClick={() => setWizard({ step: 0, form: { ...acmeBlank } })}><Sparkles className="h-4 w-4" />{t("issueAcme")}</Button></div>} />
        ) : !rows.length ? <EmptyState icon={ShieldCheck} title={t("noResults")} description={t("noResultsDesc")} /> : (
          <Table>
            <THead><tr><Th>{t("name")}</Th><Th className="hidden md:table-cell">{t("domains")}</Th><Th>{t("status")}</Th><Th className="hidden sm:table-cell">{t("expires")}</Th><Th className="hidden lg:table-cell">{t("autoRenew")}</Th><Th className="w-12"><span className="sr-only">{t("actions")}</span></Th></tr></THead>
            <tbody>
              {rows.map((c) => {
                const st = statusOf(c);
                const d = daysUntil(c.not_after);
                const soon = st === "valid" && d !== null && d <= 30;
                // a failed renewal keeps the still-valid certificate in use ("valid") with its error
                const renewErr = st === "valid" && c.kind === "acme" && !!c.last_error;
                return (
                  <Tr key={c.id}>
                    <Td>
                      <div className="flex items-center gap-2 font-medium">{c.name}</div>
                      <div className="mt-0.5 flex items-center gap-1.5"><Badge className="!h-4 !px-1.5 !text-2xs" tone={c.kind === "acme" ? "brand" : "neutral"}>{c.kind === "acme" ? "ACME" : c.kind === "selfsigned" ? t("selfSigned") : t("custom")}</Badge>{c.challenge && <span className="text-2xs text-fg-subtle">{c.challenge}</span>}</div>
                    </Td>
                    <Td className="hidden md:table-cell"><div className="flex max-w-xs flex-wrap gap-1">{c.domains.slice(0, 3).map((x) => <span key={x} className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-xs text-fg-muted">{x}</span>)}{c.domains.length > 3 && <Badge>+{c.domains.length - 3}</Badge>}</div></Td>
                    <Td>
                      {st === "pending" ? <Badge tone="info" icon={Loader2} className="[&_svg]:animate-spin">{t("certPending")}</Badge>
                        : st === "error" ? <Tip label={c.last_error || t("certError")}><span><Badge tone="danger" icon={AlertCircle}>{t("certError")}</Badge></span></Tip>
                          : renewErr ? <Tip label={t("renewFailedTip", { e: c.last_error! })}><span><Badge tone={d !== null && d <= 7 ? "danger" : "warn"} icon={AlertCircle}>{t("renewFailed")}</Badge></span></Tip>
                          : soon ? <Badge tone={d! <= 7 ? "danger" : "warn"} dot>{d! < 0 ? t("expired") : t("expiringSoon")}</Badge>
                            : <Badge tone="ok" dot>{t("certValid")}</Badge>}
                      {(st === "error" || renewErr) && c.last_error && (c.last_error_hint
                        // known cause: the actionable line (the raw error stays in the badge tooltip)
                        ? <ErrorHint code={c.last_error_hint} className="mt-1 max-w-[20rem]" />
                        : <div className={cn("mt-1 max-w-[16rem] truncate text-xs", st === "error" ? "text-danger" : "text-warn")} title={c.last_error}>{c.last_error}</div>)}
                    </Td>
                    <Td className="hidden sm:table-cell">
                      {c.not_after && d !== null ? <><div className="num text-sm">{new Date(c.not_after).toLocaleDateString(localeTag(lang))}</div>
                        <div className={cn("text-xs num", soon ? (d! <= 7 ? "text-danger" : "text-warn") : "text-fg-subtle")}>{d! < 0 ? t("expired") : t("daysLeft", { n: d! })}</div></> : <span className="text-fg-subtle">–</span>}
                    </Td>
                    <Td className="hidden lg:table-cell">{c.kind === "acme" ? (c.auto_renew ? <Badge tone="ok" icon={Check}>{t("on")}</Badge> : <Badge>{t("off")}</Badge>) : <span className="text-fg-subtle">–</span>}</Td>
                    <Td className="text-right"><RowMenu items={[
                      { label: t("renewNow"), icon: RefreshCw, hidden: c.kind !== "acme", disabled: st === "pending", onSelect: () => renew(c) },
                      { label: t("replaceCertAction"), icon: Upload, hidden: c.kind !== "custom", onSelect: () => { up.setError(null); setUpload({ id: c.id, name: c.name, certificate: "", private_key: "" }); } },
                      { label: t("delete"), icon: Trash2, danger: true, separatorBefore: c.kind === "acme" || c.kind === "custom", onSelect: () => setDeleting(c) },
                    ]} /></Td>
                  </Tr>
                );
              })}
            </tbody>
          </Table>
        )}
      </Card>

      <DnsCredentialsCard list={creds} providers={providers} />

      <Sheet open={!!upload} onOpenChange={(o) => !o && setUpload(null)} onSubmit={doUpload} title={upload?.id ? t("replaceCertTitle") : t("uploadCert")} description={upload?.id ? t("replaceCertDesc") : t("uploadCertDesc")}
        footer={<><Button onClick={() => setUpload(null)}>{t("cancel")}</Button><Button type="submit" variant="primary" loading={up.busy}>{upload?.id ? t("replaceCertAction") : t("upload")}</Button></>}>
        {upload && <>
          {up.error && <div className="px-6 pt-5"><ErrorBox error={up.error} /></div>}
          <FormSection title={t("secBasic")}>
            <IdField label={t("name")} required error={fieldErr(up.error, "name")}>{(id) => <Input id={id} value={upload.name} onChange={(e) => setUpload({ ...upload, name: e.target.value })} placeholder="wildcard-example-com" invalid={!!fieldErr(up.error, "name")} />}</IdField>
          </FormSection>
          <FormSection title={t("secPem")}>
            <IdField label={t("certPem")} required error={fieldErr(up.error, "certificate")}>{(id) => <Textarea id={id} rows={8} spellCheck={false} className="font-mono text-xs leading-5" value={upload.certificate} onChange={(e) => setUpload({ ...upload, certificate: e.target.value })} placeholder={"-----BEGIN CERTIFICATE-----"} invalid={!!fieldErr(up.error, "certificate")} />}</IdField>
            <IdField label={t("keyPem")} required hint={t("keyPemHint")} error={fieldErr(up.error, "private_key")}>{(id) => <Textarea id={id} rows={6} spellCheck={false} autoComplete="off" className="font-mono text-xs leading-5" value={upload.private_key} onChange={(e) => setUpload({ ...upload, private_key: e.target.value })} placeholder={"-----BEGIN PRIVATE KEY-----"} invalid={!!fieldErr(up.error, "private_key")} />}</IdField>
          </FormSection>
        </>}
      </Sheet>

      <Sheet open={!!w} onOpenChange={(o) => !o && setWizard(null)} title={t("acmeWizardTitle")} description={t("acmeWizardDesc")}
        footer={w && <>
          {w.step > 0 && <Button className="mr-auto" onClick={() => setWizard({ ...w, step: (w.step - 1) as 0 | 1 })}>{t("back")}</Button>}
          <Button onClick={() => setWizard(null)}>{t("cancel")}</Button>
          {w.step < 2 ? <Button variant="primary" disabled={w.step === 0 ? !step0Ok : !step1Ok} onClick={() => setWizard({ ...w, step: (w.step + 1) as 1 | 2 })}>{t("next")}</Button>
            : <Button variant="primary" loading={ac.busy} onClick={doAcme}><Sparkles className="h-4 w-4" />{t("issueNow")}</Button>}
        </>}>
        {w && wf && <>
          <ol className="flex items-center gap-2 border-b border-line px-6 py-3" aria-label="steps">
            {steps.map((s, i) => (
              <li key={s} className="flex flex-1 items-center gap-2 last:flex-none" aria-current={i === w.step ? "step" : undefined}>
                <span className={cn("grid h-5 w-5 shrink-0 place-items-center rounded-full text-2xs font-semibold", i < w.step ? "bg-brand-solid text-white" : i === w.step ? "border-2 border-brand text-brand" : "border border-line-strong text-fg-subtle")}>
                  {i < w.step ? <Check className="h-3 w-3" /> : i + 1}
                </span>
                <span className={cn("truncate text-xs font-medium", i === w.step ? "text-fg" : "text-fg-subtle")}>{s}</span>
                {i < steps.length - 1 && <span aria-hidden className="h-px flex-1 bg-line" />}
              </li>
            ))}
          </ol>
          {ac.error && w.step === 2 && <div className="px-6 pt-5">{acmeMissing ? <Callout tone="warn" title={t("comingSoon")}>{t("comingSoonDesc")}</Callout> : <ErrorBox error={ac.error} />}</div>}
          {w.step === 0 && (
            <FormSection title={t("acmeStepDomains")}>
              <Field label={t("domains")} required hint={t("acmeDomainsHint")} error={fieldErr(ac.error, "domains")}>
                <TagInput label={t("domains")} value={wf.domains} onChange={(v) => setW({ domains: v })} placeholder="example.com, *.example.com" />
              </Field>
              <IdField label={t("acmeEmail")} required hint={t("acmeEmailHint")} error={fieldErr(ac.error, "email")}>{(id) => <Input id={id} type="email" value={wf.email} onChange={(e) => setW({ email: e.target.value })} placeholder="ops@example.com" />}</IdField>
            </FormSection>
          )}
          {w.step === 1 && (
            <FormSection title={t("acmeStepChallenge")} description={t("acmeChallengeDesc")}>
              <div role="radiogroup" aria-label={t("acmeStepChallenge")} className="grid gap-3">
                {([
                  { v: "http-01" as const, icon: Globe, title: "HTTP-01", desc: t("http01Desc") },
                  { v: "dns-01" as const, icon: Cloud, title: t("dns01Title"), desc: t("dns01Desc") },
                ]).map((o) => (
                  <button key={o.v} type="button" role="radio" aria-checked={wf.challenge === o.v} onClick={() => setW(o.v === "dns-01" && !wf.dnsc.creds ? { challenge: o.v, dnsc: { ...newDnsChoice(providers[0]), credential: wf.dnsc.credential } } : { challenge: o.v })}
                    className={cn("flex items-start gap-3 rounded-xl border p-4 text-left transition", wf.challenge === o.v ? "border-brand bg-brand/5 ring-1 ring-brand/40" : "border-line hover:border-line-strong hover:bg-surface-2")}>
                    <span className={cn("grid h-9 w-9 shrink-0 place-items-center rounded-lg", wf.challenge === o.v ? "bg-brand/15 text-brand" : "bg-surface-3 text-fg-muted")}><o.icon className="h-4 w-4" /></span>
                    <span><span className="block text-sm font-semibold">{o.title}</span><span className="mt-0.5 block text-xs leading-5 text-fg-muted">{o.desc}</span></span>
                  </button>
                ))}
              </div>
              {wf.challenge === "http-01" && wildcard && <Callout tone="warn">{t("wildcardNeedsDns")}</Callout>}
              {wf.challenge === "dns-01" && (
                <DnsFields providers={providers} value={wf.dnsc} onChange={(dnsc) => setW({ dnsc })} fieldError={(f) => fieldErr(ac.error, f)} />
              )}
              <SwitchRow label={t("acmeStaging")} description={t("acmeStagingDesc")} checked={wf.staging} onChange={(v) => setW({ staging: v })} />
            </FormSection>
          )}
          {w.step === 2 && (
            <FormSection title={t("acmeStepReview")} description={t("acmeReviewDesc")}>
              <dl className="divide-y divide-line rounded-xl border border-line text-sm">
                <div className="flex justify-between gap-4 px-4 py-2.5"><dt className="text-fg-subtle">{t("domains")}</dt><dd className="flex flex-wrap justify-end gap-1">{wf.domains.map((d) => <span key={d} className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-xs">{d}</span>)}</dd></div>
                <div className="flex justify-between gap-4 px-4 py-2.5"><dt className="text-fg-subtle">{t("acmeEmail")}</dt><dd>{wf.email}</dd></div>
                <div className="flex justify-between gap-4 px-4 py-2.5"><dt className="text-fg-subtle">{t("acmeStepChallenge")}</dt><dd>{wf.challenge === "http-01" ? "HTTP-01" : `DNS-01 · ${wf.dnsc.credential ? creds.data?.find((c) => c.id === wf.dnsc.credential)?.name ?? "" : providers.find((p) => p.id === wf.dnsc.provider)?.name ?? wf.dnsc.provider}`}</dd></div>
                <div className="flex justify-between gap-4 px-4 py-2.5"><dt className="text-fg-subtle">{t("acmeStaging")}</dt><dd>{wf.staging ? <Badge tone="warn">staging</Badge> : <Badge tone="ok">production</Badge>}</dd></div>
              </dl>
              <Callout tone="info" icon={FileKey}>{t("acmeAsyncNote")}</Callout>
              {wf.challenge === "http-01" && <Callout tone="neutral" icon={CheckCircle2}>{t("http01Precheck")}</Callout>}
            </FormSection>
          )}
        </>}
      </Sheet>

      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={t("deleteCertTitle")} description={t("deleteCertDesc")} confirmLabel={t("delete")} onConfirm={remove}>
        {deleting && <div className="rounded-lg border border-line bg-surface-2 px-3 py-2 text-sm font-medium">{deleting.name}</div>}
      </ConfirmDialog>
    </>
  );
}
