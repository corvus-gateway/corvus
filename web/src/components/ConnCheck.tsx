import { useState } from "react";
import { AlertTriangle, CheckCircle2, PlugZap, XCircle } from "lucide-react";
import { api, ApiError, type ConnCheckResult, type ConnFailKind, type UpstreamError } from "../api";
import { useI18n, type Key } from "../i18n";
import { relTime } from "../lib/utils";
import { Button } from "./ui/button";
import { Badge, Callout, Mono } from "./ui/display";
import { Tip, tipTrigger } from "./ui/overlay";

const TITLE: Record<ConnFailKind, Key> = {
  dns: "ccDnsTitle", refused: "ccRefusedTitle", timeout: "ccTimeoutTitle", unreachable: "ccUnreachableTitle", tls: "ccTlsTitle", other: "ccOtherTitle",
};
const HINT: Record<ConnFailKind, Key> = {
  dns: "ccDnsHint", refused: "ccRefusedHint", timeout: "ccTimeoutHint", unreachable: "ccUnreachableHint", tls: "ccTlsHint", other: "ccOtherHint",
};
const kindOf = (k?: string): ConnFailKind => (k && k in TITLE ? (k as ConnFailKind) : "other");

/** A further backend to check with the forward target (same scheme). */
export interface CheckTarget { host: string; port: number }

/**
 * "Check connection" for a forward target: the control plane tries it the way the gateway
 * reaches it (its DNS resolver without search domains, TCP, TLS for https). With further
 * backends (a pool), every one is checked and the result is shown per backend. The result
 * belongs to the targets it was made for and disappears when the fields change.
 */
export function ConnCheck({ scheme, host, port, extra = [] }: { scheme: string; host: string; port: number; extra?: CheckTarget[] }) {
  const { t } = useI18n();
  const norm = (h: string) => h.trim().toLowerCase();
  const all = [{ host, port }, ...extra];
  const target = `${scheme}://${all.map((x) => `${norm(x.host)}:${x.port}`).join(",")}`;
  const [busy, setBusy] = useState(false);
  const [res, setRes] = useState<{ for: string; r?: ConnCheckResult; err?: string } | null>(null);
  const run = async () => {
    setBusy(true);
    try {
      const body: Record<string, unknown> = { forward_scheme: scheme, forward_host: host, forward_port: port };
      if (extra.length) body.extra_targets = extra.map((x) => ({ forward_host: x.host, forward_port: x.port }));
      const r = await api.post<ConnCheckResult>("/api/proxy-hosts/check", body);
      setRes({ for: target, r });
    } catch (e) {
      const msg = e instanceof ApiError && e.status === 429 ? t("rateLimited") : e instanceof ApiError && e.fields[0] ? e.fields[0].message : e instanceof Error ? e.message : String(e);
      setRes({ for: target, err: msg });
    } finally {
      setBusy(false);
    }
  };
  const cur = res?.for === target ? res : null;
  const r = cur?.r;
  const pool = r?.targets && r.targets.length > 1 ? r.targets : null;
  const mapped = !pool && r && r.target && r.target !== norm(host);
  return (
    <div className="space-y-2">
      <Button size="sm" onClick={run} loading={busy} disabled={all.some((x) => !x.host.trim() || !x.port)}>
        <PlugZap className="h-3.5 w-3.5" aria-hidden />{busy ? t("ccChecking") : t("ccButton")}
      </Button>
      <div aria-live="polite">
        {cur?.err && <p role="alert" className="text-xs text-danger">{cur.err}</p>}
        {pool && (
          <>
            <p className={`text-xs font-medium ${r!.ok ? "text-ok" : "text-warn"}`}>{t("ccPoolSummary", { ok: pool.filter((x) => x.ok).length, n: pool.length })}</p>
            <ul className="mt-1.5 space-y-1">
              {pool.map((x, i) => (
                <li key={i} className="flex items-start gap-1.5 text-xs">
                  {x.ok ? <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-ok" aria-hidden /> : <XCircle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-danger" aria-hidden />}
                  <span className="min-w-0">
                    <Mono className="break-all">{norm(all[i]?.host ?? x.target)}:{all[i]?.port}</Mono>
                    <span className="text-fg-subtle"> · {x.ok ? `${x.time_ms} ms` : t(TITLE[kindOf(x.kind)])}</span>
                    {!x.ok && x.message && <Mono className="block break-all text-[11px] text-fg-subtle">{x.message}</Mono>}
                  </span>
                </li>
              ))}
            </ul>
            {!r!.ok && <p className="mt-1.5 text-xs text-fg-subtle">{t(HINT[kindOf(r!.kind)])}</p>}
          </>
        )}
        {!pool && r?.ok && (
          <p className="flex items-center gap-1.5 text-xs text-ok">
            <CheckCircle2 className="h-3.5 w-3.5 shrink-0" aria-hidden />
            {t("ccOk", { addr: r.addresses?.join(", ") ?? r.address ?? r.target, ms: r.time_ms })}
          </p>
        )}
        {!pool && r && !r.ok && (
          <Callout tone="warn" icon={AlertTriangle} title={t(TITLE[kindOf(r.kind)])}>
            <p>{t(HINT[kindOf(r.kind)])}</p>
            {r.message && <Mono className="mt-1.5 block break-all text-[11px] text-fg-subtle">{r.message}</Mono>}
          </Callout>
        )}
        {mapped && <p className="mt-1 text-xs text-fg-subtle">{t("ccMapped", { target: r.target })}</p>}
      </div>
    </div>
  );
}

/**
 * How an upstream failure reads: for a backend pool, one failing backend is covered by
 * the others (nginx tries the next one and skips the failing one for 30 s); only "all
 * down" means visitors got an error page.
 */
function poolState(err: UpstreamError) {
  const pool = (err.backends ?? 0) > 1;
  return { pool, partial: pool && !err.all_down, allDown: pool && !!err.all_down };
}

/** Small warning next to a host whose upstream failed recently (gateway log). */
export function UpstreamBadge({ err }: { err: UpstreamError }) {
  const { t, lang } = useI18n();
  const { partial, allDown } = poolState(err);
  const reason = allDown ? t("ueAllDownTitle") : t(TITLE[kindOf(err.kind)]);
  return (
    <Tip label={<>
      <span className="block">{t(partial ? "ueTipBackend" : "ueTip", { ago: relTime(err.at, lang), reason })}</span>
      {err.upstream && <span className="mt-0.5 block font-mono">{err.upstream}</span>}
      {partial && <span className="mt-1 block">{t("ueTipOthers")}</span>}
      <span className="mt-1 block opacity-80">{t("ueTipAction")}</span>
    </>}>
      <span tabIndex={0} className={tipTrigger}><Badge tone="warn" icon={AlertTriangle}>{t(partial ? "ueBadgeBackend" : "ueBadge")}</Badge></span>
    </Tip>
  );
}

/** The latest upstream failure inside the host's edit sheet: what visitors saw and why. */
export function UpstreamCallout({ err }: { err: UpstreamError }) {
  const { t, lang } = useI18n();
  const kind = kindOf(err.kind);
  const { partial, allDown } = poolState(err);
  const ago = relTime(err.at, lang);
  return (
    <Callout tone="warn" icon={AlertTriangle} title={partial ? t("ueSheetBackendTitle", { ago }) : t("ueSheetTitle", { ago })}>
      <p className="font-medium">{allDown ? t("ueAllDownTitle") : t(TITLE[kind])}{err.upstream ? ` · ${err.upstream}` : ""}</p>
      <p className="mt-0.5">{allDown ? t("ueAllDownHint") : partial ? `${t(HINT[kind])} ${t("ueTipOthers")}` : t(HINT[kind])}</p>
      <Mono className="mt-1.5 block break-all text-[11px] text-fg-subtle">{err.message}</Mono>
    </Callout>
  );
}
