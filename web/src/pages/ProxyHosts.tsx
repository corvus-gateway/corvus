import { useEffect, useMemo, useState } from "react";
import { AlertCircle, AlertTriangle, Loader2, ArrowRight, Copy, Globe, Lock, Pencil, Plus, ShieldAlert, ShieldCheck, Trash2, Users, Zap, Gauge, KeyRound, ExternalLink, UserPlus, X, Siren, ShieldOff } from "lucide-react";
import { toast } from "sonner";
import { api, type AccessList, type Certificate, type ClientIPMode, type ProxyHost, type RateLimit, type SecurityHeaders, type UnderAttackStatus, type Waitroom } from "../api";
import { UnderAttackFields, uaConfig, UA_DEFAULTS } from "../components/UnderAttackFields";
import { LocationsFields } from "../components/LocationsFields";
import { ErrorBox, PageHeader, Toolbar, useSubmit } from "../components/common";
import { Button, IconButton } from "../components/ui/button";
import { Badge, Callout, Card, EmptyState, Mono, TableSkeleton } from "../components/ui/display";
import { Field, FormSection, IdField, Input, NumberInput, Segmented, Select, SearchInput, Switch, SwitchRow, TagInput, Textarea, fieldErr } from "../components/ui/form";
import { ConfirmDialog, RowMenu, Sheet, Tip } from "../components/ui/overlay";
import { servesConsole } from "../locales/consolehost.i18n";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n } from "../i18n";
import { useDnsProviders } from "../hooks/useDnsProviders";
import { DnsFields, dnsRequest, newDnsChoice, type DnsChoice } from "../components/DnsFields";
import { useApp } from "../lib/app";
import { cn, relTime } from "../lib/utils";

interface AuthUser { username: string; password: string; existing?: boolean }
interface BasicAuthForm { enabled: boolean; users: AuthUser[] }
const authFrom = (h?: ProxyHost): BasicAuthForm => {
  const users = (h?.basic_auth_users ?? []).map((u) => ({ username: u, password: "", existing: true }));
  return { enabled: users.length > 0, users };
};

interface CertReq { email: string; dns: boolean; dnsc: DnsChoice; staging: boolean; agree: boolean }

type Form = Omit<ProxyHost, "id" | "created_at" | "updated_at">;

const blank: Form = {
  domains: [], forward_scheme: "http", forward_host: "", forward_port: 80, websocket: false, certificate_id: null,
  ssl_forced: false, http2: true, waitroom_id: null, waitroom_path: "/", advanced_config: "", enabled: true,
  client_ip: "direct", rate_limit_id: null, access_list_id: null, block_exploits: true,
  body_inspection: "log", body_max_kb: 1024, security_headers: "off",
  max_upload_mb: null, upstream_timeout_s: null, gzip: true,
  under_attack: "off", under_attack_config: { ...UA_DEFAULTS }, locations: [],
};

function toForm(h: ProxyHost): Form {
  const { id: _i, created_at: _c, updated_at: _u, ...rest } = h;
  return { ...blank, ...rest, under_attack: rest.under_attack ?? "off", under_attack_config: uaConfig(rest.under_attack_config), locations: rest.locations ?? [] };
}

export default function ProxyHosts() {
  const { t, lang } = useI18n();
  const { can, isAdmin } = useCan();
  const hosts = useResource<ProxyHost[]>("/api/proxy-hosts", { poll: 15000 });
  const [certPoll, setCertPoll] = useState(0);
  const certs = useResource<Certificate[]>("/api/certificates", { poll: certPoll });
  useEffect(() => { setCertPoll(certs.data?.some((c) => c.status === "pending") ? 3000 : 0); }, [certs.data]);
  const providers = useDnsProviders();
  const { user } = useApp();
  const rooms = useResource<Waitroom[]>("/api/waitrooms");
  const limits = useResource<RateLimit[]>("/api/rate-limits");
  const lists = useResource<AccessList[]>("/api/access-lists");
  const ua = useResource<UnderAttackStatus[]>("/api/under-attack", { poll: 10000 });
  const uaOf = (id: number | null | undefined) => ua.data?.find((s) => s.host_id === id);

  const [q, setQ] = useState("");
  const [editing, setEditing] = useState<{ id: number | null; form: Form; req: CertReq | null; auth: BasicAuthForm } | null>(null);
  const [deleting, setDeleting] = useState<ProxyHost | null>(null);
  const { busy, error, setError, run } = useSubmit();
  const rowAction = useSubmit();

  const certName = (id: number | null) => certs.data?.find((c) => c.id === id)?.name;
  const roomName = (id: number | null) => rooms.data?.find((r) => r.id === id)?.name;
  const limitName = (id?: number | null) => limits.data?.find((r) => r.id === id)?.name;

  // Fields introduced after the base API are hidden when the backend does not return them.
  const sample = hosts.data?.[0];
  const has = (k: string) => !sample || k in sample;
  const showClientIp = has("client_ip");
  const showExploits = has("block_exploits");
  const showBody = has("body_inspection");
  const showHeaders = has("security_headers");
  const showUA = has("under_attack");
  const showLocations = has("locations");
  const showTuning = has("gzip");
  const showLimits = !limits.missing && has("rate_limit_id");
  const showLists = !lists.missing && has("access_list_id");

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase();
    if (!s) return hosts.data ?? [];
    return (hosts.data ?? []).filter((h) => h.domains.some((d) => d.toLowerCase().includes(s)) || `${h.forward_host}:${h.forward_port}`.includes(s));
  }, [hosts.data, q]);

  const open = (h?: ProxyHost) => { setError(null); setEditing({ id: h?.id ?? null, form: h ? toForm(h) : { ...blank }, req: null, auth: authFrom(h) }); };
  const newReq = (): CertReq => ({ email: user.email, dns: false, dnsc: newDnsChoice(providers[0]), staging: false, agree: false });

  const save = async () => {
    if (!editing) return;
    const { id, form, req, auth } = editing;
    const body: Record<string, unknown> = { ...form };
    delete body.basic_auth_users;
    delete body.access_list_id;
    body.basic_auth = { enabled: auth.enabled, users: auth.enabled ? auth.users.map(({ username, password }) => ({ username: username.trim(), password })) : [] };
    if (req) {
      const dns = wildcard || req.dns;
      body.certificate_id = null;
      body.certificate_request = { email: req.email, dns_challenge: dns, staging: req.staging, ...(dns ? dnsRequest(req.dnsc) : {}) };
    }
    if (!showClientIp) { delete body.client_ip; }
    if (!showExploits) { delete body.block_exploits; }
    if (!showBody) { delete body.body_inspection; delete body.body_max_kb; }
    if (!showHeaders) { delete body.security_headers; }
    if (!showTuning) { delete body.max_upload_mb; delete body.upstream_timeout_s; delete body.gzip; }
    if (!showUA) { delete body.under_attack; delete body.under_attack_config; }
    if (!showLocations) { delete body.locations; }
    if (!showLimits) { delete body.rate_limit_id; }
    if (!showLists) { delete body.basic_auth; }
    const r = await run(() => (id ? api.put(`/api/proxy-hosts/${id}`, body) : api.post("/api/proxy-hosts", body)), { success: req ? undefined : t("savedDeployed") });
    if (r !== undefined) { if (req) toast.success(t("savedIssuing")); setEditing(null); certs.reload(); hosts.reload(); window.dispatchEvent(new Event("corvus:refresh")); }
  };

  const [disabling, setDisabling] = useState<ProxyHost | null>(null);
  const toggle = async (h: ProxyHost, enabled: boolean) => {
    if (!enabled && servesConsole(h.domains) && disabling?.id !== h.id) { setDisabling(h); return; }
    const body = { ...toForm(h), enabled };
    const r = await rowAction.run(() => api.put(`/api/proxy-hosts/${h.id}`, body), { success: enabled ? t("hostEnabled") : t("hostDisabled") });
    if (r !== undefined) { hosts.reload(); window.dispatchEvent(new Event("corvus:refresh")); }
  };

  const deactivate = async (id: number) => {
    const r = await rowAction.run(() => api.post(`/api/proxy-hosts/${id}/under-attack/deactivate`), { success: t("uaDeactivated") });
    if (r !== undefined) { ua.reload(); window.dispatchEvent(new Event("corvus:refresh")); }
  };

  const remove = async () => {
    if (!deleting) return;
    const r = await rowAction.run(() => api.del(`/api/proxy-hosts/${deleting.id}`), { success: t("deletedDeployed") });
    if (r !== undefined) { hosts.reload(); window.dispatchEvent(new Event("corvus:refresh")); }
  };

  const f = editing?.form;
  const editingConsole = !!editing?.id && servesConsole((hosts.data ?? []).find((x) => x.id === editing.id)?.domains ?? []);
  const set = (patch: Partial<Form>) => setEditing((e) => (e ? { ...e, form: { ...e.form, ...patch } } : e));
  const req = editing?.req ?? null;
  const setReq = (patch: Partial<CertReq>) => setEditing((e) => (e && e.req ? { ...e, req: { ...e.req, ...patch } } : e));
  const wildcard = !!f?.domains.some((d) => d.startsWith("*."));
  const useDns = wildcard || !!req?.dns;
  const hasCert = !!f?.certificate_id || !!req;
  const noDomain = f && f.domains.length === 0;

  return (
    <>
      <PageHeader title={t("navHosts")} description={t("hostsDesc")} actions={can && <Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addHost")}</Button>} />
      <Card>
        <Toolbar>
          <SearchInput className="w-full sm:w-72" value={q} onChange={setQ} placeholder={t("searchHosts")} />
          <span className="ml-auto text-xs text-fg-subtle num">{t("nItems", { n: filtered.length })}</span>
        </Toolbar>
        {hosts.loading ? <TableSkeleton cols={5} /> : hosts.error && !hosts.data ? <div className="p-4"><ErrorBox error={hosts.error} /></div> :
          !hosts.data?.length ? (
            <EmptyState icon={Globe} title={t("noHosts")} description={t("noHostsDesc")} action={<Button variant="primary" onClick={() => open()}><Plus className="h-4 w-4" />{t("addHost")}</Button>} />
          ) : !filtered.length ? (
            <EmptyState icon={Globe} title={t("noResults")} description={t("noResultsDesc")} />
          ) : (
            <Table>
              <THead><tr>
                <Th>{t("domains")}</Th><Th className="hidden md:table-cell">{t("forwardTo")}</Th><Th className="hidden sm:table-cell">{t("sslCol")}</Th><Th className="hidden lg:table-cell">{t("protection")}</Th>
                <Th>{t("status")}</Th><Th className="w-12"><span className="sr-only">{t("actions")}</span></Th>
              </tr></THead>
              <tbody>
                {filtered.map((h) => {
                  const cn_ = certName(h.certificate_id);
                  const uaSt = uaOf(h.id);
                  return (
                    <Tr key={h.id}>
                      <Td>
                        <div className="flex items-center gap-2">
                          <a href={`${h.certificate_id ? "https" : "http"}://${h.domains[0]?.replace(/^\*\./, "")}`} target="_blank" rel="noreferrer noopener" className="group inline-flex items-center gap-1.5 font-medium text-fg hover:text-brand">
                            {h.domains[0]}<ExternalLink className="h-3 w-3 opacity-0 transition group-hover:opacity-70" aria-hidden />
                          </a>
                          {h.domains.length > 1 && <Tip label={h.domains.slice(1).join(", ")}><span><Badge>+{h.domains.length - 1}</Badge></span></Tip>}
                          {uaSt?.active && <Tip label={uaSt.reason || t("uaOn")}><span><Badge tone="danger" icon={Siren}>{t("uaBadge")}</Badge></span></Tip>}
                          {servesConsole(h.domains) && <Tip label={t("consoleHostTitle")}><span><Badge tone="info">{t("consoleHostBadge")}</Badge></span></Tip>}
                        </div>
                        <div className="mt-0.5 flex items-center gap-2 text-xs text-fg-subtle">
                          {cn_ ? <span className="inline-flex items-center gap-1"><Lock className="h-3 w-3 text-ok" aria-hidden />{cn_}</span> : <span>{t("noTls")}</span>}
                          <span className="hidden sm:inline">· {relTime(h.updated_at, lang)}</span>
                        </div>
                      </Td>
                      <Td className="hidden md:table-cell">
                        <Mono className="inline-flex items-center gap-1.5 text-fg-muted"><ArrowRight className="h-3 w-3 text-fg-subtle" aria-hidden />{h.forward_scheme}://{h.forward_host}:{h.forward_port}</Mono>
                      </Td>
                      <Td className="hidden sm:table-cell"><SslBadge cert={certs.data?.find((c) => c.id === h.certificate_id)} /></Td>
                      <Td className="hidden lg:table-cell">
                        <div className="flex flex-wrap items-center gap-1.5">
                          {h.ssl_forced && <Badge tone="ok" icon={Lock}>HTTPS</Badge>}
                          {h.websocket && <Badge icon={Zap}>WS</Badge>}
                          {h.waitroom_id != null && <Badge tone="brand" icon={Users}>{roomName(h.waitroom_id) ?? t("navRooms")}</Badge>}
                          {h.rate_limit_id != null && <Badge tone="info" icon={Gauge}>{limitName(h.rate_limit_id) ?? "Rate limit"}</Badge>}
                          {(h.basic_auth_users?.length ?? 0) > 0 && <Badge tone="info" icon={KeyRound}>{t("basicAuthBadge")}</Badge>}
                          {h.block_exploits && <Badge tone="warn" icon={ShieldAlert}>{t("exploitShort")}</Badge>}
                          {h.under_attack === "auto" && !uaSt?.active && <Badge icon={Siren}>{t("uaAutoArmed")}</Badge>}
                          {h.security_headers && h.security_headers !== "off" && <Badge tone="ok" icon={ShieldCheck}>{t("headersShort")} · {h.security_headers === "strict" ? t("shStrict") : t("shBasic")}</Badge>}
                          {h.client_ip && h.client_ip !== "direct" && <Badge>{h.client_ip === "cloudflare" ? "Cloudflare" : t("trustedProxiesShort")}</Badge>}
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
                          ...(can && uaSt?.active && uaSt.mode === "auto" ? [{ label: t("uaDeactivate"), icon: ShieldOff, onSelect: () => deactivate(h.id) }] : []),
                          { label: t("delete"), icon: Trash2, danger: true, separatorBefore: true, onSelect: () => setDeleting(h) },
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
        title={editing?.id ? t("editHost") : t("addHost")} description={t("hostSheetDesc")}
        footer={<><Button onClick={() => setEditing(null)}>{t("cancel")}</Button><Button type="submit" variant="primary" loading={busy} disabled={!!noDomain || (!!req && !req.agree)}>{t("saveAndDeploy")}</Button></>}>
        {f && (<>
          {error && <div className="px-6 pt-5"><ErrorBox error={error} /></div>}
          {editingConsole && <div className="px-6 pt-5"><Callout tone="warn" icon={AlertTriangle} title={t("consoleHostTitle")}>{t("consoleHostEdit")}</Callout></div>}
          <FormSection title={t("secDomains")}>
            <Field label={t("domains")} required hint={t("domainsHint")} error={fieldErr(error, "domains")}>
              <TagInput label={t("domains")} value={f.domains} onChange={(v) => set({ domains: v })} placeholder="shop.example.com, *.example.com" invalid={!!fieldErr(error, "domains")} />
            </Field>
          </FormSection>

          <FormSection title={t("secForward")}>
            <Field label={t("scheme")}>
              <Segmented label={t("scheme")} value={f.forward_scheme as "http" | "https"} onChange={(v) => set({ forward_scheme: v, forward_port: f.forward_port === 80 && v === "https" ? 443 : f.forward_port === 443 && v === "http" ? 80 : f.forward_port })}
                options={[{ value: "http", label: "http" }, { value: "https", label: "https" }]} />
            </Field>
            <div className="grid grid-cols-[1fr_7rem] gap-3">
              <IdField label={t("forwardHost")} required error={fieldErr(error, "forward_host")}>{(id) => <Input id={id} className="font-mono" value={f.forward_host} onChange={(e) => set({ forward_host: e.target.value })} placeholder="app / 10.0.0.12" invalid={!!fieldErr(error, "forward_host")} />}</IdField>
              <IdField label={t("forwardPort")} required error={fieldErr(error, "forward_port")}>{(id) => <NumberInput id={id} min={1} max={65535} value={f.forward_port} onChange={(v) => set({ forward_port: v })} />}</IdField>
            </div>
            <SwitchRow label={t("websocket")} description={t("websocketDesc")} checked={f.websocket} onChange={(v) => set({ websocket: v })} />
          </FormSection>

          {showLocations && <LocationsFields value={f.locations ?? []} onChange={(v) => set({ locations: v })} error={error} isAdmin={isAdmin} />}

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
            {showHeaders && (
              <IdField label={t("secHeaders")} hint={t("secHeadersHint")} error={fieldErr(error, "security_headers")}>
                {(id) => (
                  <div className="space-y-2">
                    <Select id={id} value={f.security_headers ?? "off"} onChange={(e) => set({ security_headers: e.target.value as SecurityHeaders })}>
                      <option value="off">{t("shOff")}</option>
                      <option value="basic">{t("shBasic")}</option>
                      <option value="strict">{t("shStrict")}</option>
                    </Select>
                    <p className="text-xs leading-5 text-fg-subtle">{f.security_headers === "strict" ? t("shStrictDesc") : f.security_headers === "basic" ? t("shBasicDesc") : t("shOffDesc")}</p>
                  </div>
                )}
              </IdField>
            )}
          </FormSection>

          {showClientIp && (
            <FormSection title={t("secClientIp")} description={t("clientIpDesc")}>
              <Segmented label={t("secClientIp")} value={(f.client_ip ?? "direct") as ClientIPMode} onChange={(v) => set({ client_ip: v })}
                options={[{ value: "direct", label: t("ipDirect") }, { value: "cloudflare", label: "Cloudflare" }, { value: "trusted_proxies", label: t("ipTrusted") }]} />
              <p className="text-xs leading-5 text-fg-subtle">{f.client_ip === "cloudflare" ? t("ipCloudflareDesc") : f.client_ip === "trusted_proxies" ? t("ipTrustedDesc") : t("ipDirectDesc")}</p>
            </FormSection>
          )}

          {(showLimits || showLists || showExploits) && (
            <FormSection title={t("secProtection")}>
              {showLimits && (
                <IdField label={t("rateLimitPolicy")} error={fieldErr(error, "rate_limit_id")}>
                  {(id) => (
                    <Select id={id} value={f.rate_limit_id ?? ""} onChange={(e) => set({ rate_limit_id: e.target.value ? Number(e.target.value) : null })}>
                      <option value="">{t("noneUnlimited")}</option>
                      {limits.data?.map((r) => <option key={r.id} value={r.id}>{r.name} · {r.requests_per_second} r/s</option>)}
                    </Select>
                  )}
                </IdField>
              )}
              {showLists && editing && (
                <div className="space-y-3">
                  <SwitchRow label={t("basicAuthProtect")} description={t("basicAuthProtectDesc")} checked={editing.auth.enabled}
                    onChange={(v) => setEditing({ ...editing, auth: { enabled: v, users: v && !editing.auth.users.length ? [{ username: "", password: "" }] : editing.auth.users } })} />
                  {editing.auth.enabled && (
                    <div className="space-y-2 pl-1">
                      {editing.auth.users.map((u, i) => {
                        const setU = (p: Partial<AuthUser>) => setEditing({ ...editing, auth: { ...editing.auth, users: editing.auth.users.map((x, j) => (j === i ? { ...x, ...p } : x)) } });
                        return (
                          <div key={i} className="flex items-center gap-2">
                            <Input aria-label={t("username")} placeholder={t("username")} value={u.username} autoComplete="off" onChange={(e) => setU({ username: e.target.value })} />
                            <Input aria-label={t("password")} type="password" autoComplete="new-password" placeholder={u.existing && !u.password ? t("keepPassword") : t("password")}
                              value={u.password} onChange={(e) => setU({ password: e.target.value })} />
                            <IconButton label={t("remove")} onClick={() => setEditing({ ...editing, auth: { ...editing.auth, users: editing.auth.users.filter((_, j) => j !== i) } })}><X className="h-4 w-4" /></IconButton>
                          </div>
                        );
                      })}
                      <Button size="sm" onClick={() => setEditing({ ...editing, auth: { ...editing.auth, users: [...editing.auth.users, { username: "", password: "" }] } })}>
                        <UserPlus className="h-3.5 w-3.5" />{t("addUser")}
                      </Button>
                      {fieldErr(error, "basic_auth") && <p className="text-xs text-danger">{fieldErr(error, "basic_auth")}</p>}
                    </div>
                  )}
                </div>
              )}
              {showExploits && <SwitchRow label={t("blockExploits")} description={t("blockExploitsDesc")} checked={!!f.block_exploits} onChange={(v) => set({ block_exploits: v })} />}
              {showBody && (
                <Field label={t("bodyInspection")} hint={t("bodyInspectionDesc")} error={fieldErr(error, "body_inspection")}>
                  <Segmented label={t("bodyInspection")} value={f.body_inspection ?? "off"} onChange={(v) => set({ body_inspection: v as "off" | "log" | "block" })}
                    options={[{ value: "off", label: t("bodyOff") }, { value: "log", label: t("bodyLog") }, { value: "block", label: t("bodyBlock") }]} />
                </Field>
              )}
              {showBody && (f.body_inspection ?? "off") !== "off" && (
                <IdField label={t("bodyMaxKb")} hint={t("bodyMaxKbHint")} error={fieldErr(error, "body_max_kb")}>
                  {(id) => <NumberInput id={id} min={16} max={8192} value={f.body_max_kb ?? 1024} onChange={(v) => set({ body_max_kb: v })} />}
                </IdField>
              )}
            </FormSection>
          )}

          {showUA && (
            <UnderAttackFields mode={f.under_attack ?? "off"} cfg={uaConfig(f.under_attack_config)} error={error}
              onChange={(mode, cfg) => set({ under_attack: mode, under_attack_config: cfg })}
              status={editing?.id ? uaOf(editing.id) : undefined} onDeactivate={() => editing?.id && deactivate(editing.id)} busy={rowAction.busy} can={can} />
          )}

          <FormSection title={t("secWaitroom")}>
            <IdField label={t("waitroom")} error={fieldErr(error, "waitroom_id")}>
              {(id) => (
                <Select id={id} value={f.waitroom_id ?? ""} onChange={(e) => set({ waitroom_id: e.target.value ? Number(e.target.value) : null })}>
                  <option value="">{t("noneNoWaiting")}</option>
                  {rooms.data?.map((r) => <option key={r.id} value={r.id}>{r.name}{r.enabled ? "" : ` (${t("disabled")})`}</option>)}
                </Select>
              )}
            </IdField>
            {f.waitroom_id != null && (
              <IdField label={t("waitroomPath")} hint={t("waitroomPathHint")} error={fieldErr(error, "waitroom_path")}>
                {(id) => <Input id={id} className="font-mono" value={f.waitroom_path} onChange={(e) => set({ waitroom_path: e.target.value })} placeholder="/" invalid={!!fieldErr(error, "waitroom_path")} />}
              </IdField>
            )}
          </FormSection>

          <FormSection title={t("secAdvanced")}>
            {showTuning && (<>
              <IdField label={t("maxUpload")} hint={t("maxUploadHint")} error={fieldErr(error, "max_upload_mb")}>
                {(id) => <NumberInput id={id} min={0} max={1048576} suffix="MB" placeholder="100" value={f.max_upload_mb ?? NaN} onChange={(v) => set({ max_upload_mb: Number.isFinite(v) ? v : null })} />}
              </IdField>
              <IdField label={t("upstreamTimeout")} hint={t("upstreamTimeoutHint")} error={fieldErr(error, "upstream_timeout_s")}>
                {(id) => <NumberInput id={id} min={1} max={86400} suffix="s" placeholder={f.websocket ? "3600" : "60"} value={f.upstream_timeout_s ?? NaN} onChange={(v) => set({ upstream_timeout_s: Number.isFinite(v) ? v : null })} />}
              </IdField>
              <SwitchRow label={t("gzip")} description={t("gzipDesc")} checked={f.gzip ?? true} onChange={(v) => set({ gzip: v })} />
            </>)}
            <IdField label={t("advanced")} hint={isAdmin ? t("advancedHint") : t("advancedAdminOnly")} error={fieldErr(error, "advanced_config")}>
              {(id) => <Textarea id={id} rows={7} spellCheck={false} readOnly={!isAdmin} className={cn("font-mono text-xs leading-5", !isAdmin && "bg-surface-2 text-fg-muted")} value={f.advanced_config} onChange={(e) => set({ advanced_config: e.target.value })} placeholder={"# proxy_set_header X-Foo bar;"} invalid={!!fieldErr(error, "advanced_config")} />}
            </IdField>
          </FormSection>

          <FormSection title={t("status")}>
            <SwitchRow label={t("enabled")} description={t("hostEnabledDesc")} checked={f.enabled} onChange={(v) => set({ enabled: v })} />
          </FormSection>
        </>)}
      </Sheet>

      <ConfirmDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)} title={t("deleteHostTitle")} confirmLabel={t("delete")}
        description={t("confirmDelete")} onConfirm={remove}>
        {deleting && <div className="rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-xs">{deleting.domains.join(", ")}</div>}
        {deleting && servesConsole(deleting.domains) && <Callout tone="danger" icon={AlertTriangle} className="mt-3">{t("consoleHostDelete")}</Callout>}
      </ConfirmDialog>

      <ConfirmDialog open={!!disabling} onOpenChange={(o) => !o && setDisabling(null)} title={t("consoleHostDisableTitle")} confirmLabel={t("disable")}
        description={t("consoleHostDisable")} onConfirm={async () => { if (disabling) await toggle(disabling, false); }}>
        {disabling && <div className="rounded-lg border border-line bg-surface-2 px-3 py-2 font-mono text-xs">{disabling.domains.join(", ")}</div>}
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
