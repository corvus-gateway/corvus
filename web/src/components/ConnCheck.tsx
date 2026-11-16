import { useState } from "react";
import { AlertTriangle, CheckCircle2, PlugZap } from "lucide-react";
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

/**
 * "Check connection" for a forward target: the control plane tries it the way the gateway
 * reaches it (its DNS resolver without search domains, TCP, TLS for https). The result
 * belongs to the target it was made for and disappears when the fields change.
 */
export function ConnCheck({ scheme, host, port }: { scheme: string; host: string; port: number }) {
  const { t } = useI18n();
  const target = `${scheme}://${host.trim().toLowerCase()}:${port}`;
  const [busy, setBusy] = useState(false);
  const [res, setRes] = useState<{ for: string; r?: ConnCheckResult; err?: string } | null>(null);
  const run = async () => {
    setBusy(true);
    try {
      const r = await api.post<ConnCheckResult>("/api/proxy-hosts/check", { forward_scheme: scheme, forward_host: host, forward_port: port });
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
  const mapped = r && r.target && r.target !== host.trim().toLowerCase();
  return (
    <div className="space-y-2">
      <Button size="sm" onClick={run} loading={busy} disabled={!host.trim() || !port}>
        <PlugZap className="h-3.5 w-3.5" aria-hidden />{busy ? t("ccChecking") : t("ccButton")}
      </Button>
      <div aria-live="polite">
        {cur?.err && <p role="alert" className="text-xs text-danger">{cur.err}</p>}
        {r?.ok && (
          <p className="flex items-center gap-1.5 text-xs text-ok">
            <CheckCircle2 className="h-3.5 w-3.5 shrink-0" aria-hidden />
            {t("ccOk", { addr: r.address ?? r.target, ms: r.time_ms })}
          </p>
        )}
        {r && !r.ok && (
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

/** Small warning next to a host whose upstream failed recently (gateway log). */
export function UpstreamBadge({ err }: { err: UpstreamError }) {
  const { t, lang } = useI18n();
  const reason = t(TITLE[kindOf(err.kind)]);
  return (
    <Tip label={<>
      <span className="block">{t("ueTip", { ago: relTime(err.at, lang), reason })}</span>
      {err.upstream && <span className="mt-0.5 block font-mono">{err.upstream}</span>}
      <span className="mt-1 block opacity-80">{t("ueTipAction")}</span>
    </>}>
      <span tabIndex={0} className={tipTrigger}><Badge tone="warn" icon={AlertTriangle}>{t("ueBadge")}</Badge></span>
    </Tip>
  );
}

/** The latest upstream failure inside the host's edit sheet: what visitors saw and why. */
export function UpstreamCallout({ err }: { err: UpstreamError }) {
  const { t, lang } = useI18n();
  const kind = kindOf(err.kind);
  return (
    <Callout tone="warn" icon={AlertTriangle} title={t("ueSheetTitle", { ago: relTime(err.at, lang) })}>
      <p className="font-medium">{t(TITLE[kind])}{err.upstream ? ` · ${err.upstream}` : ""}</p>
      <p className="mt-0.5">{t(HINT[kind])}</p>
      <Mono className="mt-1.5 block break-all text-[11px] text-fg-subtle">{err.message}</Mono>
    </Callout>
  );
}
