import { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";
import { Cable, Cloud, Network as NetworkIcon, RefreshCw, Save } from "lucide-react";
import { api, type NetworkSettings, type ProxyProtocolSettings } from "../api";
import { ErrorBox, PageHeader, useSubmit } from "../components/common";
import { ppTrustedError } from "../components/ErrorHint";
import { Button } from "../components/ui/button";
import { Badge, Callout, Card, CardHeader, ComingSoon, Skeleton } from "../components/ui/display";
import { Field, IdField, Input, NumberInput, Select, SwitchRow, TagInput, fieldErr } from "../components/ui/form";
import { useResource } from "../hooks/useResource";
import { useCan } from "../lib/rbac";
import { useI18n } from "../i18n";
import { fmtDateTime } from "../lib/utils";

const DEFAULT_HEADERS = ["X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP", "True-Client-IP"];
const PP_DEFAULT: ProxyProtocolSettings = { enabled: false, http_port: 8080, https_port: 8443, trusted: [] };

export default function NetworkPage() {
  const { t, lang } = useI18n();
  const { isAdmin } = useCan();
  const { hash } = useLocation();
  const res = useResource<NetworkSettings>("/api/settings/network");
  const [proxies, setProxies] = useState<string[]>([]);
  const [header, setHeader] = useState("X-Forwarded-For");
  const [custom, setCustom] = useState(false);
  const [mask, setMask] = useState(false);
  const [allow, setAllow] = useState<string[]>([]);
  const [pp, setPP] = useState<ProxyProtocolSettings>(PP_DEFAULT);
  const HEADERS = res.data?.real_ip_headers?.length ? res.data.real_ip_headers : DEFAULT_HEADERS;
  const save = useSubmit();
  const cf = useSubmit();

  useEffect(() => {
    if (!res.data) return;
    setProxies(res.data.trusted_proxies ?? []);
    setHeader(res.data.real_ip_header || "X-Forwarded-For");
    setCustom(!!res.data.real_ip_header && !HEADERS.includes(res.data.real_ip_header));
    setMask(!!res.data.mask_ips);
    setAllow(res.data.admin_allow ?? []);
    setPP(res.data.proxy_protocol ?? PP_DEFAULT);
  }, [res.data]);

  // Links such as /network#proxy-protocol (from the stream form) open at that card.
  const loaded = !!res.data;
  useEffect(() => {
    if (!loaded || !hash) return;
    const el = document.getElementById(hash.slice(1));
    if (el) requestAnimationFrame(() => el.scrollIntoView({ block: "start", behavior: "smooth" }));
  }, [loaded, hash]);

  const dirty = !!res.data && (JSON.stringify(proxies) !== JSON.stringify(res.data.trusted_proxies ?? []) || header !== (res.data.real_ip_header || "X-Forwarded-For") || ("mask_ips" in res.data && mask !== !!res.data.mask_ips) || JSON.stringify(allow) !== JSON.stringify(res.data.admin_allow ?? [])
    || (!!res.data.proxy_protocol && JSON.stringify(pp) !== JSON.stringify(res.data.proxy_protocol)));

  // Empty or out-of-range ports are refused here (JSON would send an empty field as null).
  const badPort = (v: number) => pp.enabled && (!Number.isInteger(v) || v < 1 || v > 65535) ? t("ppPortInvalid") : undefined;
  const httpErr = badPort(pp.http_port) ?? fieldErr(save.error, "proxy_protocol.http_port");
  const httpsErr = badPort(pp.https_port) ?? fieldErr(save.error, "proxy_protocol.https_port");
  const noSources = pp.enabled && !(pp.trusted?.length);
  const trustedErr = (noSources ? t("ppTrustedRequired") : undefined) ?? ppTrustedError(t, save.error);
  const blocked = !!badPort(pp.http_port) || !!badPort(pp.https_port) || noSources;

  const onSave = async () => {
    if (blocked) return;
    const r = await save.run(() => api.put("/api/settings/network", { trusted_proxies: proxies, real_ip_header: header, admin_allow: allow, ...(res.data && "mask_ips" in res.data ? { mask_ips: mask } : {}), ...(res.data?.proxy_protocol ? { proxy_protocol: pp } : {}) }), { success: t("savedDeployed") });
    if (r !== undefined) { res.reload(); window.dispatchEvent(new Event("corvus:refresh")); }
  };
  const onRefresh = async () => {
    const r = await cf.run(() => api.post("/api/settings/network/cloudflare/refresh"), { success: t("cfRefreshed"), deployed: true });
    if (r !== undefined) res.reload();
  };
  // Both forms save the whole page (the API replaces the network settings as one).
  const saveRow = isAdmin && <div className="flex justify-end gap-2">
    <Button type="submit" variant="primary" loading={save.busy} disabled={!dirty || blocked}><Save className="h-4 w-4" />{t("saveAndDeploy")}</Button>
  </div>;

  const c = res.data?.cloudflare;
  return (
    <>
      <PageHeader title={t("navNetwork")} description={t("networkDesc")} />
      {res.missing ? <Card><ComingSoon icon={NetworkIcon} /></Card> : res.loading ? (
        <div className="grid gap-5 lg:grid-cols-2"><Skeleton className="h-72 rounded-xl" /><Skeleton className="h-72 rounded-xl" /></div>
      ) : res.error && !res.data ? <ErrorBox error={res.error} /> : (
        <div className="grid items-start gap-5 lg:grid-cols-2">
          <Card>
            <CardHeader icon={NetworkIcon} title={t("trustedProxies")} description={t("trustedProxiesDesc")} />
            <form className="px-4 pb-4 pt-2" onSubmit={(e) => { e.preventDefault(); onSave(); }}>
            <fieldset disabled={!isAdmin} className="min-w-0 space-y-5">
              {!isAdmin && <Callout tone="neutral">{t("adminOnlyNotice")}</Callout>}
              <ErrorBox error={save.error} />
              <Field label={t("trustedProxyCidrs")} hint={t("trustedProxyHint")} error={fieldErr(save.error, "trusted_proxies")}>
                <TagInput label={t("trustedProxyCidrs")} value={proxies} onChange={setProxies} placeholder="10.0.0.0/8, 172.16.0.0/12" />
              </Field>
              <IdField label={t("realIpHeader")} hint={t("realIpHeaderHint")} error={fieldErr(save.error, "real_ip_header")}>
                {(id) => (<div className="space-y-2">
                  <Select id={id} value={custom ? "__custom" : header} onChange={(e) => { if (e.target.value === "__custom") { setCustom(true); } else { setCustom(false); setHeader(e.target.value); } }}>
                    {HEADERS.map((h) => <option key={h} value={h}>{h}</option>)}
                    <option value="__custom">{t("customHeader")}</option>
                  </Select>
                  {custom && <Input aria-label={t("realIpHeader")} className="font-mono" value={header} onChange={(e) => setHeader(e.target.value)} placeholder="X-Client-IP" />}
                </div>)}
              </IdField>
              {res.data && "mask_ips" in res.data && <SwitchRow label={t("maskIps")} description={t("maskIpsDesc")} checked={mask} onChange={setMask} />}
              <Callout tone="info">{t("trustedProxyWarn")}</Callout>
              <Field label={t("adminAllow")} hint={t("adminAllowHint")} error={fieldErr(save.error, "admin_allow")}>
                <TagInput label={t("adminAllow")} value={allow} onChange={setAllow} placeholder="203.0.113.0/24, 10.0.0.0/8" />
              </Field>
              {saveRow}
            </fieldset>
            </form>
          </Card>

          <Card>
            <CardHeader icon={Cloud} title={t("cloudflareRealIp")} description={t("cloudflareRealIpDesc")}
              actions={isAdmin && <Button size="sm" loading={cf.busy} onClick={onRefresh}><RefreshCw className="h-3.5 w-3.5" />{t("refreshList")}</Button>} />
            <div className="space-y-4 px-4 pb-4 pt-2">
              <ErrorBox error={cf.error} />
              {c ? (<>
                <div className="grid grid-cols-2 gap-3">
                  <div className="rounded-lg border border-line bg-surface-2/50 p-3"><div className="text-xs text-fg-subtle">IPv4 {t("ranges")}</div><div className="num mt-1 text-2xl font-semibold">{c.ipv4}</div></div>
                  <div className="rounded-lg border border-line bg-surface-2/50 p-3"><div className="text-xs text-fg-subtle">IPv6 {t("ranges")}</div><div className="num mt-1 text-2xl font-semibold">{c.ipv6}</div></div>
                </div>
                <dl className="divide-y divide-line rounded-lg border border-line text-sm">
                  <div className="flex items-center justify-between px-3 py-2.5"><dt className="text-fg-muted">{t("listSource")}</dt><dd>{c.source === "remote" ? <Badge tone="ok" dot>{t("sourceRemote")}</Badge> : <Badge>{t("sourceBuiltin")}</Badge>}</dd></div>
                  <div className="flex items-center justify-between px-3 py-2.5"><dt className="text-fg-muted">{t("lastUpdated")}</dt><dd className="num text-fg">{c.updated_at ? fmtDateTime(c.updated_at, lang) : "–"}</dd></div>
                </dl>
              </>) : <p className="text-sm text-fg-subtle">{t("cfNoInfo")}</p>}
              <p className="text-xs leading-5 text-fg-subtle">{t("cfUsageHint")}</p>
            </div>
          </Card>

          {res.data?.proxy_protocol && (
            <Card id="proxy-protocol" className="scroll-mt-4">
              <CardHeader icon={Cable} title={t("ppCardTitle")} description={t("ppCardDesc")} />
              <form className="px-4 pb-4 pt-2" onSubmit={(e) => { e.preventDefault(); onSave(); }}>
              <fieldset disabled={!isAdmin} className="min-w-0 space-y-5">
                <Field label={t("ppTrusted")} hint={t("ppTrustedHint")} error={trustedErr}>
                  <TagInput label={t("ppTrusted")} value={pp.trusted ?? []} onChange={(v) => setPP({ ...pp, trusted: v })} placeholder="172.20.0.5, 10.8.0.0/24" />
                </Field>
                <div className="space-y-4 border-t border-line pt-3">
                  <SwitchRow label={t("ppTitle")} description={t("ppDesc")} checked={pp.enabled} onChange={(v) => setPP({ ...pp, enabled: v })} />
                  {pp.enabled && <>
                    <div className="grid grid-cols-2 gap-3">
                      <IdField label={t("ppHttpPort")} error={httpErr}>
                        {(id) => <NumberInput id={id} min={1} max={65535} value={pp.http_port} onChange={(v) => setPP({ ...pp, http_port: v })} />}
                      </IdField>
                      <IdField label={t("ppHttpsPort")} error={httpsErr}>
                        {(id) => <NumberInput id={id} min={1} max={65535} value={pp.https_port} onChange={(v) => setPP({ ...pp, https_port: v })} />}
                      </IdField>
                    </div>
                    <Callout tone="info">{t("ppHint")}</Callout>
                  </>}
                </div>
                {saveRow}
              </fieldset>
              </form>
            </Card>
          )}
        </div>
      )}
    </>
  );
}
