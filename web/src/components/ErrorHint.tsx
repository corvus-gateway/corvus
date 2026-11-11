import { Lightbulb } from "lucide-react";
import { ApiError } from "../api";
import { useI18n, type Key } from "../i18n";
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
  return h ? `https://corvusgateway.org/${lang === "ko" ? "ko/" : ""}reference/troubleshooting/#${h.anchor}` : null;
}

/** The hint code of an API failure, if any. */
export const hintOf = (e: unknown) => (e instanceof ApiError ? e.hint : undefined);

/** Plain hint sentence (for toasts); "" when the code is unknown. */
export function useHintText() {
  const { t } = useI18n();
  return (code?: string) => (code && HINTS[code] ? t(HINTS[code].key) : "");
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
