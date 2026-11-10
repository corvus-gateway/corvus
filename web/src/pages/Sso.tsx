import { useEffect, useMemo, useState } from "react";
import { Copy, Fingerprint, Lock, Save, SearchCheck, UsersRound } from "lucide-react";
import { toast } from "sonner";
import { api } from "../api";
import { ErrorBox, PageHeader, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Callout, Card, CardHeader, ComingSoon, EmptyState, Skeleton } from "../components/ui/display";
import { Field, Input, Select, SwitchRow, TagInput, fieldErr } from "../components/ui/form";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { useCan } from "../lib/rbac";
import { copyText } from "../lib/utils";

/** GET/PUT /api/settings/sso. */
interface SsoSettings {
  enabled: boolean; display_name: string; issuer: string; client_id: string; client_secret_set: boolean;
  scopes: string[]; allowed_domains: string[]; role_claim: string; admin_values: string[]; operator_values: string[];
  default_role: "viewer" | "deny"; auto_provision: boolean; sync_role: boolean; trust_email: boolean; redirect_uri: string;
}
interface Discovery { issuer: string; authorization_endpoint: string; token_endpoint: string; userinfo_endpoint?: string; jwks_uri: string }

interface Form {
  enabled: boolean; display_name: string; issuer: string; client_id: string; scopes: string;
  allowed_domains: string[]; role_claim: string; admin_values: string[]; operator_values: string[];
  default_role: "viewer" | "deny"; auto_provision: boolean; sync_role: boolean; trust_email: boolean;
}

const toForm = (s: SsoSettings): Form => ({
  enabled: s.enabled, display_name: s.display_name, issuer: s.issuer, client_id: s.client_id, scopes: (s.scopes ?? []).join(" "),
  allowed_domains: s.allowed_domains ?? [], role_claim: s.role_claim, admin_values: s.admin_values ?? [], operator_values: s.operator_values ?? [],
  default_role: s.default_role === "deny" ? "deny" : "viewer", auto_provision: s.auto_provision, sync_role: s.sync_role, trust_email: !!s.trust_email,
});

export default function SsoPage() {
  const { t } = useI18n();
  const { isAdmin } = useCan();
  const res = useResource<SsoSettings>(isAdmin ? "/api/settings/sso" : null);
  const [f, setF] = useState<Form | null>(null);
  const [secret, setSecret] = useState("");
  const [clearSecret, setClearSecret] = useState(false);
  const [disc, setDisc] = useState<Discovery | null>(null);
  const save = useSubmit();
  const test = useSubmit();

  useEffect(() => { if (res.data) setF(toForm(res.data)); }, [res.data]);
  const dirty = useMemo(() => !!f && !!res.data && (JSON.stringify(f) !== JSON.stringify(toForm(res.data)) || secret !== "" || clearSecret), [f, res.data, secret, clearSecret]);

  if (!isAdmin) {
    return (<><PageHeader title={t("navSso")} description={t("ssoDesc")} />
      <Card><EmptyState icon={Lock} title={t("noPermissionTitle")} description={t("noPermissionDesc")} /></Card></>);
  }

  const set = <K extends keyof Form>(k: K, v: Form[K]) => setF((p) => (p ? { ...p, [k]: v } : p));
  const onSave = async () => {
    if (!f) return;
    const body = { ...f, scopes: f.scopes.split(/[\s,]+/).filter(Boolean), client_secret: secret, clear_client_secret: clearSecret };
    const r = await save.run(() => api.put<SsoSettings>("/api/settings/sso", body), { deployed: false, success: t("ssoSaved") });
    if (r !== undefined) { setSecret(""); setClearSecret(false); res.reload(); }
  };
  const onTest = async () => {
    if (!f) return;
    setDisc(null);
    const r = await test.run(() => api.post<{ ok: boolean; discovery: Discovery }>("/api/settings/sso/test", { issuer: f.issuer.trim() }), { deployed: false, success: t("ssoDiscoveryOk"), silentError: true });
    if (r) setDisc(r.discovery);
  };
  const copyRedirect = async () => { if (res.data && await copyText(res.data.redirect_uri)) toast.success(t("copied")); };
  const err = (k: string) => fieldErr(save.error, k);
  const testErr = fieldErr(test.error, "issuer");

  return (
    <>
      <PageHeader title={t("navSso")} description={t("ssoDesc")} />
      {res.missing ? <Card><ComingSoon icon={Fingerprint} /></Card> : res.loading || (!f && !res.error) ? (
        <div className="grid gap-5 lg:grid-cols-2"><Skeleton className="h-96 rounded-xl" /><Skeleton className="h-96 rounded-xl" /></div>
      ) : res.error && !res.data ? <ErrorBox error={res.error} /> : f && res.data && (
        <form className="space-y-5" onSubmit={(e) => { e.preventDefault(); onSave(); }}>
          <ErrorBox error={save.error} />
          <div className="grid items-start gap-5 lg:grid-cols-2">
            <Card>
              <CardHeader icon={Fingerprint} title={t("ssoProvider")} description={t("ssoProviderDesc")} />
              <div className="space-y-5 px-4 pb-4 pt-2">
                <SwitchRow label={t("ssoEnabled")} description={t("ssoEnabledDesc")} checked={f.enabled} onChange={(v) => set("enabled", v)} />
                <Field label={t("ssoDisplayName")} hint={t("ssoDisplayNameHint")} error={err("display_name")} htmlFor="sso-name">
                  <Input id="sso-name" maxLength={64} value={f.display_name} onChange={(e) => set("display_name", e.target.value)} placeholder={t("ssoButtonDefault")} invalid={!!err("display_name")} />
                </Field>
                <Field label={t("ssoIssuer")} hint={t("ssoIssuerHint")} error={err("issuer") ?? testErr} required htmlFor="sso-issuer">
                  <div className="flex gap-2">
                    <Input id="sso-issuer" className="min-w-0 flex-1 font-mono" inputMode="url" autoComplete="off" spellCheck={false} value={f.issuer}
                      onChange={(e) => { set("issuer", e.target.value); setDisc(null); }} placeholder="https://idp.example.com/realms/corp" invalid={!!(err("issuer") ?? testErr)} />
                    <Button onClick={onTest} loading={test.busy} disabled={!f.issuer.trim()}><SearchCheck className="h-4 w-4" />{t("ssoTestDiscovery")}</Button>
                  </div>
                </Field>
                {disc && (
                  <dl className="divide-y divide-line rounded-lg border border-line text-xs">
                    {([["authorization", disc.authorization_endpoint], ["token", disc.token_endpoint], ["jwks", disc.jwks_uri], ["userinfo", disc.userinfo_endpoint]] as const)
                      .filter(([, v]) => !!v).map(([k, v]) => (
                        <div key={k} className="flex gap-3 px-3 py-2"><dt className="w-24 shrink-0 text-fg-muted">{k}</dt><dd className="min-w-0 break-all font-mono text-fg">{v}</dd></div>
                      ))}
                  </dl>
                )}
                <Field label={t("ssoClientId")} error={err("client_id")} required htmlFor="sso-client">
                  <Input id="sso-client" className="font-mono" autoComplete="off" spellCheck={false} value={f.client_id} onChange={(e) => set("client_id", e.target.value)} invalid={!!err("client_id")} />
                </Field>
                <Field label={t("ssoClientSecret")} hint={t("ssoSecretHint")} error={err("client_secret")} htmlFor="sso-secret">
                  <Input id="sso-secret" type="password" autoComplete="new-password" value={secret} disabled={clearSecret}
                    onChange={(e) => setSecret(e.target.value)} placeholder={res.data.client_secret_set ? t("ssoSecretSet") : ""} invalid={!!err("client_secret")} />
                </Field>
                {res.data.client_secret_set && (
                  <label className="flex items-center gap-2 text-sm text-fg-muted">
                    <input type="checkbox" checked={clearSecret} onChange={(e) => { setClearSecret(e.target.checked); if (e.target.checked) setSecret(""); }} />
                    {t("ssoClearSecret")}
                  </label>
                )}
                <Field label={t("ssoScopes")} hint={t("ssoScopesHint")} error={err("scopes")} htmlFor="sso-scopes">
                  <Input id="sso-scopes" className="font-mono" spellCheck={false} value={f.scopes} onChange={(e) => set("scopes", e.target.value)} placeholder="openid email profile" invalid={!!err("scopes")} />
                </Field>
                <Field label={t("ssoRedirectUri")} hint={t("ssoRedirectUriHint")} htmlFor="sso-redirect">
                  <div className="flex gap-2">
                    <Input id="sso-redirect" readOnly className="min-w-0 flex-1 font-mono" value={res.data.redirect_uri} onFocus={(e) => e.target.select()} />
                    <Button onClick={copyRedirect} aria-label={t("copy")}><Copy className="h-4 w-4" />{t("copy")}</Button>
                  </div>
                </Field>
              </div>
            </Card>

            <Card>
              <CardHeader icon={UsersRound} title={t("ssoAccess")} description={t("ssoAccessDesc")} />
              <div className="space-y-5 px-4 pb-4 pt-2">
                <Field label={t("ssoAllowedDomains")} hint={t("ssoAllowedDomainsHint")} error={err("allowed_domains")}>
                  <TagInput label={t("ssoAllowedDomains")} value={f.allowed_domains} onChange={(v) => set("allowed_domains", v)} placeholder="example.com" invalid={!!err("allowed_domains")} />
                </Field>
                <Field label={t("ssoRoleClaim")} hint={t("ssoRoleClaimHint")} error={err("role_claim")} htmlFor="sso-claim">
                  <Input id="sso-claim" className="font-mono" spellCheck={false} value={f.role_claim} onChange={(e) => set("role_claim", e.target.value)} placeholder="groups" invalid={!!err("role_claim")} />
                </Field>
                {f.role_claim.trim() !== "" && (<>
                  <Field label={t("ssoAdminValues")} hint={t("ssoValuesHint")} error={err("admin_values")}>
                    <TagInput lineMode label={t("ssoAdminValues")} value={f.admin_values} onChange={(v) => set("admin_values", v)} placeholder="corvus-admins" invalid={!!err("admin_values")} />
                  </Field>
                  <Field label={t("ssoOperatorValues")} error={err("operator_values")}>
                    <TagInput lineMode label={t("ssoOperatorValues")} value={f.operator_values} onChange={(v) => set("operator_values", v)} placeholder="corvus-operators" invalid={!!err("operator_values")} />
                  </Field>
                </>)}
                <Field label={t("ssoDefaultRole")} error={err("default_role")} htmlFor="sso-default">
                  <Select id="sso-default" value={f.default_role} onChange={(e) => set("default_role", e.target.value === "deny" ? "deny" : "viewer")}>
                    <option value="viewer">{t("ssoDefaultViewer")}</option>
                    <option value="deny">{t("ssoDefaultDeny")}</option>
                  </Select>
                </Field>
                <SwitchRow label={t("ssoAutoProvision")} description={t("ssoAutoProvisionDesc")} checked={f.auto_provision} onChange={(v) => set("auto_provision", v)} />
                <SwitchRow label={t("ssoSyncRole")} description={t("ssoSyncRoleDesc")} checked={f.sync_role} onChange={(v) => set("sync_role", v)} disabled={f.role_claim.trim() === ""} />
                <SwitchRow label={t("ssoTrustEmail")} description={t("ssoTrustEmailDesc")} checked={f.trust_email} onChange={(v) => set("trust_email", v)} />
                <Callout tone="info">{t("ssoMfaNote")}</Callout>
                <Callout tone="neutral">{t("ssoLocalNote")}</Callout>
              </div>
            </Card>
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" disabled={!dirty || save.busy} onClick={() => { setF(toForm(res.data!)); setSecret(""); setClearSecret(false); save.setError(null); }}>{t("cancel")}</Button>
            <Button type="submit" variant="primary" loading={save.busy} disabled={!dirty}><Save className="h-4 w-4" />{t("save")}</Button>
          </div>
        </form>
      )}
    </>
  );
}
