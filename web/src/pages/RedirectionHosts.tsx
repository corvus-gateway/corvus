import { useEffect, useMemo, useState } from "react";
import { AlertCircle, ArrowRight, Copy, ExternalLink, Loader2, Lock, Pencil, Plus, ShieldAlert, Signpost, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, type Certificate, type RedirectionHost, type RedirectScheme } from "../api";
import { NoteLine, NotesSection, notesMatch } from "../components/NotesField";
import { ErrorBox, PageHeader, Toolbar, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Badge, Card, EmptyState, Mono, TableSkeleton } from "../components/ui/display";
import { Field, FormSection, IdField, Input, Segmented, Select, SearchInput, Switch, SwitchRow, TagInput, Textarea, fieldErr } from "../components/ui/form";
import { ConfirmDialog, RowMenu, Sheet, Tip } from "../components/ui/overlay";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n, type Key } from "../i18n";
import { useDnsProviders } from "../hooks/useDnsProviders";
import { DnsFields, dnsRequest, newDnsChoice, type DnsChoice } from "../components/DnsFields";
import { useApp } from "../lib/app";
import { cn, relTime } from "../lib/utils";

interface CertReq { email: string; dns: boolean; dnsc: DnsChoice; staging: boolean; agree: boolean }

type Form = Omit<RedirectionHost, "id" | "created_at" | "updated_at">;

const blank: Form = {
  domains: [], forward_scheme: "auto", forward_domain: "", http_code: 301, preserve_path: true, certificate_id: null,
  ssl_forced: false, http2: true, block_exploits: true, advanced_config: "", enabled: true, notes: "",
};

const CODES: { code: number; label: Key }[] = [
  { code: 301, label: "code301" }, { code: 302, label: "code302" }, { code: 303, label: "code303" }, { code: 307, label: "code307" }, { code: 308, label: "code308" },
];

function toForm(h: RedirectionHost): Form {
  const { id: _i, created_at: _c, updated_at: _u, ...rest } = h;
  return { ...blank, ...rest };
}

/** The redirect as nginx answers it, e.g. "301 → https://example.com/…". */
function target(h: Pick<RedirectionHost, "forward_scheme" | "forward_domain" | "preserve_path">) {
  const scheme = h.forward_scheme === "auto" ? "$scheme" : h.forward_scheme;
  return `${scheme}://${h.forward_domain}${h.preserve_path ? "/…" : ""}`;
}

export default function RedirectionHosts() {
  const { t, lang } = useI18n();
  const { can, isAdmin } = useCan();
  const hosts = useResource<RedirectionHost[]>("/api/redirection-hosts", { poll: 15000 });
  const [certPoll, setCertPoll] = useState(0);
  const certs = useResource<Certificate[]>("/api/certificates", { poll: certPoll });
  useEffect(() => { setCertPoll(certs.data?.some((c) => c.status === "pending") ? 3000 : 0); }, [certs.data]);
  const providers = useDnsProviders();
  const { user } = useApp();

  const [q, setQ] = useState("");
  const [editing, setEditing] = useState<{ id: number | null; form: Form; req: CertReq | null } | null>(null);
  const [deleting, setDeleting] = useState<RedirectionHost | null>(null);
  const { busy, error, setError, run } = useSubmit();
  const rowAction = useSubmit();

  const certName = (id: number | null) => certs.data?.find((c) => c.id === id)?.name;

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase();
    if (!s) return hosts.data ?? [];
    return (hosts.data ?? []).filter((h) => h.domains.some((d) => d.toLowerCase().includes(s)) || h.forward_domain.toLowerCase().includes(s) || notesMatch(h.notes, s));
  }, [hosts.data, q]);

  const open = (h?: RedirectionHost) => { setError(null); setEditing({ id: h?.id ?? null, form: h ? toForm(h) : { ...blank }, req: null }); };
  const newReq = (): CertReq => ({ email: user.email, dns: false, dnsc: newDnsChoice(providers[0]), staging: false, agree: false });

  const f = editing?.form;
  const set = (patch: Partial<Form>) => setEditing((e) => (e ? { ...e, form: { ...e.form, ...patch } } : e));
  const req = editing?.req ?? null;
  const setReq = (patch: Partial<CertReq>) => setEditing((e) => (e && e.req ? { ...e, req: { ...e.req, ...patch } } : e));
  const wildcard = !!f?.domains.some((d) => d.startsWith("*."));
  const useDns = wildcard || !!req?.dns;
  const hasCert = !!f?.certificate_id || !!req;
  const incomplete = !f || f.domains.length === 0 || !f.forward_domain.trim();

  const save = async () => {
    if (!editing) return;
    const { id, form, req } = editing;
    const body: Record<string, unknown> = { ...form };
    if (req) {
      const dns = wildcard || req.dns;
      body.certificate_id = null;
      body.certificate_request = { email: req.email, dns_challenge: dns, staging: req.staging, ...(dns ? dnsRequest(req.dnsc) : {}) };
    }
    const r = await run(() => (id ? api.put(`/api/redirection-hosts/${id}`, body) : api.post("/api/redirection-hosts", body)), { success: req ? undefined : t("savedDeployed") });
    if (r !== undefined) { if (req) toast.success(t("savedIssuing")); setEditing(null); certs.reload(); hosts.reload(); window.dispatchEvent(new Event("corvus:refresh")); }
  };

  const toggle = async (h: RedirectionHost, enabled: boolean) => {
    const r = await rowAction.run(() => api.put(`/api/redirection-hosts/${h.id}`, { ...toForm(h), enabled }), { success: enabled ? t("hostEnabled") : t("hostDisabled") });
    if (r !== undefined) { hosts.reload(); window.dispatchEvent(new Event("corvus:refresh")); }
  };

  const remove = async () => {
    if (!deleting) return;
    const r = await rowAction.run(() => api.del(`/api/redirection-hosts/${deleting.id}`), { success: t("deletedDeployed") });
    if (r !== undefined) { hosts.reload(); window.dispatchEvent(new Event("corvus:refresh")); }
  };

  return (
    <>
      <PageHeader title={t("navRedirects")} description={t("redirectsDesc")} actions={can && <Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addRedirect")}</Button>} />
      <Card>
        <Toolbar>
          <SearchInput className="w-full sm:w-72" value={q} onChange={setQ} placeholder={t("searchRedirects")} />
          <span className="ml-auto text-xs text-fg-subtle num">{t("nItems", { n: filtered.length })}</span>
        </Toolbar>
        {hosts.loading ? <TableSkeleton cols={5} /> : hosts.error && !hosts.data ? <div className="p-4"><ErrorBox error={hosts.error} /></div> :
          !hosts.data?.length ? (
            <EmptyState icon={Signpost} title={t("noRedirects")} description={t("noRedirectsDesc")} action={can ? <Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addRedirect")}</Button> : undefined} />
          ) : !filtered.length ? (
            <EmptyState icon={Signpost} title={t("noResults")} description={t("noResultsDesc")} />
          ) : (
            <Table>
              <THead><tr>
                <Th>{t("domains")}</Th><Th className="hidden md:table-cell">{t("redirectTo")}</Th><Th className="hidden sm:table-cell">{t("sslCol")}</Th><Th className="hidden lg:table-cell">{t("protection")}</Th>
                <Th>{t("status")}</Th><Th className="w-12"><span className="sr-only">{t("actions")}</span></Th>
              </tr></THead>
              <tbody>
                {filtered.map((h) => {
                  const cn_ = certName(h.certificate_id);
                  return (
                    <Tr key={h.id}>
                      <Td>
                        <div className="flex items-center gap-2">
                          <a href={`${h.certificate_id ? "https" : "http"}://${h.domains[0]?.replace(/^\*\./, "")}`} target="_blank" rel="noreferrer noopener" className="group inline-flex items-center gap-1.5 font-medium text-fg hover:text-brand">
                            {h.domains[0]}<ExternalLink className="h-3 w-3 opacity-0 transition group-hover:opacity-70" aria-hidden />
                          </a>
                          {h.domains.length > 1 && <Tip label={h.domains.slice(1).join(", ")}><span><Badge>+{h.domains.length - 1}</Badge></span></Tip>}
                        </div>
                        <div className="mt-0.5 flex items-center gap-2 text-xs text-fg-subtle">
                          {cn_ ? <span className="inline-flex items-center gap-1"><Lock className="h-3 w-3 text-ok" aria-hidden />{cn_}</span> : <span>{t("noTls")}</span>}
                          <span className="hidden sm:inline">· {relTime(h.updated_at, lang)}</span>
                        </div>
                        <NoteLine notes={h.notes} />
                      </Td>
                      <Td className="hidden md:table-cell">
                        <Mono className="inline-flex items-center gap-1.5 text-fg-muted"><Badge>{h.http_code}</Badge><ArrowRight className="h-3 w-3 text-fg-subtle" aria-hidden />{target(h)}</Mono>
                      </Td>
                      <Td className="hidden sm:table-cell"><SslBadge cert={certs.data?.find((c) => c.id === h.certificate_id)} /></Td>
                      <Td className="hidden lg:table-cell">
                        <div className="flex flex-wrap items-center gap-1.5">
                          {h.ssl_forced && <Badge tone="ok" icon={Lock}>HTTPS</Badge>}
                          {h.block_exploits && <Badge tone="warn" icon={ShieldAlert}>{t("exploitShort")}</Badge>}
                        </div>
                      </Td>
                      <Td>
                        <div className="flex items-center gap-2">
                          <Switch checked={h.enabled} onCheckedChange={(v) => toggle(h, v)} label={`${h.domains[0]} ${t("enabled")}`} disabled={rowAction.busy || !can} />
                          <span className="hidden text-xs text-fg-muted xl:inline">{h.enabled ? t("enabled") : t("disabled")}</span>
                        </div>
                      </Td>
                      <Td className="text-right">
                        <RowMenu items={[
                          { label: t("edit"), icon: Pencil, onSelect: () => open(h) },
                          { label: t("copyDomain"), icon: Copy, onSelect: () => { navigator.clipboard?.writeText(h.domains[0]); toast.success(t("copied")); } },
                          ...(can ? [{ label: t("delete"), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => setDeleting(h) }] : []),
                        ]} />
                      </Td>
                    </Tr>
                  );
                })}
              </tbody>
            </Table>
          )}
      </Card>

      <Sheet open={!!editing} onOpenChange={(o) => !o && setEditing(null)} onSubmit={save} width="max-w-[38rem]"
        title={editing?.id ? t("editRedirect") : t("addRedirect")} description={t("redirectSheetDesc")}
        footer={<><Button onClick={() => setEditing(null)}>{t("cancel")}</Button><Button type="submit" variant="primary" loading={busy} disabled={incomplete || !can || (!!req && !req.agree)}>{t("saveAndDeploy")}</Button></>}>
        {f && (<>
          {error && <div className="px-6 pt-5"><ErrorBox error={error} /></div>}
          <FormSection title={t("secDomains")}>
            <Field label={t("domains")} required hint={t("domainsHint")} error={fieldErr(error, "domains")}>
              <TagInput label={t("domains")} value={f.domains} onChange={(v) => set({ domains: v })} placeholder="old.example.com, www.example.com" invalid={!!fieldErr(error, "domains")} />
            </Field>
          </FormSection>

          <FormSection title={t("secRedirect")}>
            <Field label={t("redirectScheme")} hint={t("schemeAutoDesc")} error={fieldErr(error, "forward_scheme")}>
              <Segmented label={t("redirectScheme")} value={f.forward_scheme} onChange={(v) => set({ forward_scheme: v as RedirectScheme })}
                options={[{ value: "auto", label: t("schemeAuto") }, { value: "http", label: "http" }, { value: "https", label: "https" }]} />
            </Field>
            <IdField label={t("redirectTarget")} required hint={t("redirectTargetHint")} error={fieldErr(error, "forward_domain")}>
              {(id) => <Input id={id} className="font-mono" value={f.forward_domain} onChange={(e) => set({ forward_domain: e.target.value })} placeholder="example.com" invalid={!!fieldErr(error, "forward_domain")} />}
            </IdField>
            <IdField label={t("redirectCode")} error={fieldErr(error, "http_code")}>
              {(id) => (
                <Select id={id} value={f.http_code} onChange={(e) => set({ http_code: Number(e.target.value) })}>
                  {CODES.map((c) => <option key={c.code} value={c.code}>{t(c.label)}</option>)}
                </Select>
              )}
            </IdField>
            <SwitchRow label={t("preservePath")} description={t("preservePathDesc")} checked={f.preserve_path} onChange={(v) => set({ preserve_path: v })} />
          </FormSection>

          <FormSection title={t("secTls")}>
            <IdField label={t("certificate")} error={fieldErr(error, "certificate_id")}>
              {(id) => (
                <Select id={id} value={req ? "__new" : f.certificate_id ?? ""} onChange={(e) => {
                  const v = e.target.value;
                  if (v === "__new") setEditing((x) => (x ? { ...x, req: newReq(), form: { ...x.form, certificate_id: null } } : x));
                  else setEditing((x) => (x ? { ...x, req: null, form: { ...x.form, certificate_id: v ? Number(v) : null, ssl_forced: v ? x.form.ssl_forced : false } } : x));
                }}>
                  <option value="">{t("certNone")}</option>
                  {certs.data?.map((c) => <option key={c.id} value={c.id}>{c.name} · {c.domains.slice(0, 2).join(", ")}{c.status === "pending" ? ` (${t("certStatusPending")})` : c.status === "error" ? ` (${t("certError")})` : ""}</option>)}
                  <option value="__new">{t("newCertOption")}</option>
                </Select>
              )}
            </IdField>
            {req && (
              <div className="space-y-4 rounded-xl border border-brand/25 bg-brand/[0.04] p-4">
                <IdField label={t("acmeEmail")} required error={fieldErr(error, "certificate_request.email")}>
                  {(id) => <Input id={id} type="email" value={req.email} onChange={(e) => setReq({ email: e.target.value })} invalid={!!fieldErr(error, "certificate_request.email")} />}
                </IdField>
                <SwitchRow label={t("dnsChallenge")} description={wildcard ? t("dnsChallengeLocked") : t("dnsChallengeDesc")} checked={useDns} disabled={wildcard} onChange={(v) => setReq({ dns: v })} />
                {useDns ? (
                  <DnsFields providers={providers} value={req.dnsc} onChange={(dnsc) => setReq({ dnsc })} fieldError={(f) => fieldErr(error, `certificate_request.${f}`)} />
                ) : <p className="text-xs leading-5 text-fg-subtle">{t("httpChallengeHint")}</p>}
                {fieldErr(error, "certificate_request") && <p role="alert" className="text-xs text-danger">{fieldErr(error, "certificate_request")}</p>}
                <p className="text-xs leading-5 text-fg-subtle">{t("autoRenewNote")}</p>
                <label className="flex cursor-pointer items-center gap-2 text-sm">
                  <input type="checkbox" checked={req.agree} onChange={(e) => setReq({ agree: e.target.checked })} className="h-4 w-4 accent-[rgb(var(--brand-solid))]" />{t("agreeTos")}
                </label>
              </div>
            )}
            <div className="divide-y divide-line">
              <SwitchRow label={t("sslForced")} description={t("sslForcedDesc")} checked={f.ssl_forced} disabled={!hasCert} onChange={(v) => set({ ssl_forced: v })} />
              <SwitchRow label="HTTP/2" description={t("http2Desc")} checked={f.http2} disabled={!hasCert} onChange={(v) => set({ http2: v })} />
            </div>
          </FormSection>

          <FormSection title={t("secProtection")} description={t("redirectProtectionNote")}>
            <SwitchRow label={t("blockExploits")} description={t("blockExploitsDesc")} checked={f.block_exploits} onChange={(v) => set({ block_exploits: v })} />
          </FormSection>

          <FormSection title={t("secAdvanced")}>
            <IdField label={t("advanced")} hint={isAdmin ? t("advancedHint") : t("advancedAdminOnly")} error={fieldErr(error, "advanced_config")}>
              {(id) => <Textarea id={id} rows={5} spellCheck={false} readOnly={!isAdmin} className={cn("font-mono text-xs leading-5", !isAdmin && "bg-surface-2 text-fg-muted")} value={f.advanced_config} onChange={(e) => set({ advanced_config: e.target.value })} placeholder={"# add_header X-Robots-Tag noindex;"} invalid={!!fieldErr(error, "advanced_config")} />}
            </IdField>
          </FormSection>

          <NotesSection value={f.notes ?? ""} onChange={(v) => set({ notes: v })} error={error} disabled={!can} />

          <FormSection title={t("status")}>
            <SwitchRow label={t("enabled")} description={t("redirectEnabledDesc")} checked={f.enabled} onChange={(v) => set({ enabled: v })} />
          </FormSection>
        </>)}
      </Sheet>

      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={t("deleteRedirectTitle")} confirmLabel={t("delete")}
        description={t("confirmDelete")} onConfirm={remove}>
        {deleting && <div className="rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-xs">{deleting.domains.join(", ")} → {target(deleting)}</div>}
      </ConfirmDialog>
    </>
  );
}

function SslBadge({ cert }: { cert?: Certificate }) {
  const { t } = useI18n();
  if (!cert) return <span className="text-xs text-fg-subtle">{t("sslNone")}</span>;
  const label = cert.kind === "acme" ? t("sslLetsEncrypt") : t("sslCustom");
  if (cert.status === "pending") return <Badge tone="info" icon={Loader2} className="[&_svg]:animate-spin">{label} · {t("certStatusPending")}</Badge>;
  if (cert.status === "error") return <Tip label={cert.last_error || t("certError")}><span><Badge tone="danger" icon={AlertCircle}>{label} · {t("certError")}</Badge></span></Tip>;
  return <Badge tone="ok" icon={Lock}>{label}</Badge>;
}
