import { useCallback } from "react";
import { Lightbulb } from "lucide-react";
import { toast } from "sonner";
import { ApiError } from "../api";
import { useI18n, type Key, type TFn } from "../i18n";
import { cn } from "../lib/utils";

/** Hint codes from the control plane (internal/hints) → sentence and troubleshooting anchor. */
const HINTS: Record<string, { key: Key; anchor: string }> = {
  "acme.rate_limited": { key: "hintAcmeRateLimited", anchor: "acme-rate-limited" },
  "acme.dns_servfail": { key: "hintDnsServfail", anchor: "dns01-servfail" },
  "acme.dns_propagation": { key: "hintDnsPropagation", anchor: "dns01-propagation" },
  "acme.dns_credential": { key: "hintDnsCredential", anchor: "dns01-credential" },
  "acme.dns_lookup": { key: "hintDnsLookup", anchor: "dns01-lookup" },
  "acme.caa": { key: "hintAcmeCaa", anchor: "acme-caa" },
  "acme.rejected_name": { key: "hintAcmeRejectedName", anchor: "acme-rejected-name" },
  "acme.invalid_email": { key: "hintAcmeEmail", anchor: "acme-email" },
  "acme.domain_dns": { key: "hintAcmeDns", anchor: "acme-dns" },
  "acme.port80_unreachable": { key: "hintAcmePort80", anchor: "acme-port-80" },
  "acme.wrong_server": { key: "hintAcmeWrongServer", anchor: "acme-wrong-server" },
  "acme.ca_unreachable": { key: "hintAcmeCaUnreachable", anchor: "acme-ca-unreachable" },
  "deploy.port_in_use": { key: "hintDeployPortInUse", anchor: "deploy-port-in-use" },
  "deploy.advanced_config": { key: "hintDeployAdvanced", anchor: "deploy-advanced-config" },
  "net.dns": { key: "hintNetDns", anchor: "outbound-errors" },
  "net.refused": { key: "hintNetRefused", anchor: "outbound-errors" },
  "net.timeout": { key: "hintNetTimeout", anchor: "outbound-errors" },
  "net.tls": { key: "hintNetTls", anchor: "outbound-errors" },
  "net.blocked": { key: "hintNetBlocked", anchor: "outbound-errors" },
  "net.auth": { key: "hintNetAuth", anchor: "outbound-errors" },
  "net.not_found": { key: "hintNetNotFound", anchor: "outbound-errors" },
  "net.mysql_auth": { key: "hintNetMysqlAuth", anchor: "outbound-errors" },
  "net.mysql_db": { key: "hintNetMysqlDb", anchor: "outbound-errors" },
  "net.oidc_issuer": { key: "hintNetOidcIssuer", anchor: "outbound-errors" },
};

/** Troubleshooting guide URL for a hint code (null when the code is unknown). */
export function hintLink(code: string | undefined, lang: string): string | null {
  const h = code ? HINTS[code] : undefined;
  return h ? `https://corvusgateway.org/${lang === "ko" ? "ko/" : ""}docs/reference/troubleshooting/#${h.anchor}` : null;
}

/** The hint code of an API failure, if any. */
export const hintOf = (e: unknown) => (e instanceof ApiError ? e.hint : undefined);

/** Plain hint sentence (for toasts); "" when the code is unknown. */
export function useHintText() {
  const { t } = useI18n();
  return (code?: string) => (code && HINTS[code] ? t(HINTS[code].key) : "");
}

// Server messages the console knows (conflicts that refuse a change): shown translated.
const IN_USE = /^(?:certificate|access queue|rate limit policy|access list|list|DNS credential) is used by (\d+) (proxy host|redirection host|stream|security rule|certificate)\(s\)$/;
const IN_USE_BY: Record<string, Key> = {
  "proxy host": "navHosts", "redirection host": "navRedirects", stream: "navStreams", "security rule": "navSecurityRules", certificate: "navCerts",
};
const FIXED: Record<string, Key> = {
  "at least one active administrator is required": "errLastAdmin",
  "you cannot delete your own account": "errDeleteSelf",
  "you cannot change your own role or disable your own account": "errSelfRole",
  "change your own password on the account page": "errOwnPassword",
};

/** The message of a failure in the console language when the server's text is a known one. */
export function errorText(t: TFn, e: unknown): string {
  if (!(e instanceof ApiError)) return e instanceof Error ? e.message : t("requestFailed");
  const m = e.message;
  const u = IN_USE.exec(m);
  if (u) return t("errInUse", { n: u[1], what: t(IN_USE_BY[u[2]]) });
  const max = /^at most (\d+) security rules$/.exec(m);
  if (max) return t("errMaxRules", { n: max[1] });
  return FIXED[m] ? t(FIXED[m]) : m;
}

/** A trusted PROXY protocol sources error (network settings, or a stream that accepts the
 *  PROXY protocol) in the console language when it is a known one. */
export function ppTrustedError(t: TFn, error: unknown, field = "proxy_protocol.trusted"): string | undefined {
  if (!(error instanceof ApiError)) return undefined;
  const f = error.fields.find((x) => x.field === field);
  if (!f) return undefined;
  if (f.code === "trusted_required") return t("streamPPNoTrusted");
  if (f.code === "required") return f.message.startsWith("stream #") ? t("ppTrustedUsedByStream") : t("ppTrustedRequired");
  return f.message;
}

/** Shows a failure as a toast: translated message, the hint sentence and a "How to fix" link. */
export function useErrorToast() {
  const { t, lang } = useI18n();
  return useCallback((e: unknown, o: { title?: string; description?: string } = {}) => {
    const code = hintOf(e);
    const hint = code && HINTS[code] ? t(HINTS[code].key) : "";
    const link = hintLink(code, lang);
    const detail = o.description ?? (o.title ? errorText(t, e) : "");
    toast.error(o.title ?? errorText(t, e), {
      description: [detail, hint].filter(Boolean).join("\n") || undefined,
      duration: hint || detail ? 10_000 : undefined,
      action: link ? { label: t("hintHowToFix"), onClick: () => window.open(link, "_blank", "noopener,noreferrer") } : undefined,
    });
  }, [t, lang]);
}

/** One actionable line for an error hint code plus a "How to fix" link; nothing for unknown codes. */
export function ErrorHint({ code, className }: { code?: string; className?: string }) {
  const { t, lang } = useI18n();
  const h = code ? HINTS[code] : undefined;
  if (!h) return null;
  return (
    <div className={cn("flex items-start gap-1.5 text-xs text-fg-muted", className)}>
      <Lightbulb className="mt-0.5 h-3.5 w-3.5 shrink-0 text-warn" aria-hidden />
      <span>
        {t(h.key)}{" "}
        <a href={hintLink(code, lang)!} target="_blank" rel="noopener noreferrer" className="whitespace-nowrap text-brand underline-offset-2 hover:underline">{t("hintHowToFix")} →</a>
      </span>
    </div>
  );
}
