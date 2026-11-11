import { useCallback, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { Rocket, ShieldAlert } from "lucide-react";
import { ApiError, api, deployedVersion } from "../api";
import { useI18n } from "../i18n";
import { cn } from "../lib/utils";
import { Callout, CodeBlock } from "./ui/display";
import { ErrorHint } from "./ErrorHint";
import { Crumbs } from "./layout/Crumbs";
import { useCan } from "../lib/rbac";
import { Button } from "./ui/button";

/** Page title row with description and primary actions. */
export function PageHeader({ title, description, actions }: { title: ReactNode; description?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="-mx-4 mb-5 flex flex-wrap items-end justify-between gap-3 border-b border-line bg-surface px-4 pb-4 pt-4 sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
      <div className="min-w-0">
        <Crumbs />
        <h1 className="mt-1 text-2xl font-normal tracking-tight text-fg">{title}</h1>
        {description && <p className="mt-1 max-w-3xl text-sm text-fg-muted">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  );
}

export function Toolbar({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("flex flex-wrap items-center gap-2 border-b border-line px-3 py-2.5", className)}>{children}</div>;
}

/** Renders an API failure. For config_rejected, shows the gateway's nginx output in a code block. */
export function ErrorBox({ error, className }: { error: unknown; className?: string }) {
  const { t } = useI18n();
  if (!error) return null;
  const e = error instanceof ApiError ? error : null;
  const general = e ? e.fields.filter((f) => !f.field) : [];
  const title = e?.kind === "config_rejected" ? t("configRejected") : e?.kind === "validation" ? t("validationFailed") : e?.status === 429 ? t("rateLimited") : t("requestFailed");
  const msg = e?.kind === "config_rejected" ? t("configRejectedDesc") : e?.kind === "validation" ? null : (e?.message ?? String(error));
  return (
    <div className={cn("space-y-2", className)}>
      <Callout tone="danger" icon={ShieldAlert} title={title}>
        {msg}
        {general.map((f, i) => <div key={i}>{f.message}</div>)}
        <ErrorHint code={e?.hint} className="mt-1.5" />
      </Callout>
      {e?.output && <CodeBlock code={e.output} tone="danger" maxHeight="14rem" />}
    </div>
  );
}

/** Wraps a mutation: busy flag, error capture, success toast with deployed version.
 *  Validation and "configuration rejected" errors are left to the page's ErrorBox; pass
 *  toastAll for actions that have no ErrorBox (table row switches, delete confirmations,
 *  the redeploy banner), so those failures are not silent. */
export function useSubmit({ toastAll = false }: { toastAll?: boolean } = {}) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const run = useCallback(async <T,>(fn: () => Promise<T>, o: { success?: string; deployed?: boolean; silentError?: boolean } = {}): Promise<T | undefined> => {
    setBusy(true); setError(null);
    try {
      const res = await fn();
      const v = o.deployed === false ? null : deployedVersion(res);
      if (o.success) toast.success(v != null ? `${o.success} · ${t("versionN", { n: v })}` : o.success);
      return res;
    } catch (e) {
      setError(e);
      // 403 is also "wrong current password / code" (invalid_credentials): the ErrorBox
      // shows that message, the permission toast would be misleading
      if (e instanceof ApiError && e.status === 403 && e.kind !== "invalid_credentials") toast.error(t("forbidden"));
      else if (o.silentError) { /* the caller shows it */ }
      else if (e instanceof ApiError && (e.kind === "validation" || e.kind === "config_rejected")) {
        if (toastAll) toast.error(e.kind === "config_rejected" ? t("configRejected") : t("validationFailed"), { description: errorSummary(e), duration: 10_000 });
      } else {
        toast.error(e instanceof ApiError ? e.message : t("requestFailed"));
      }
      return undefined;
    } finally { setBusy(false); }
  }, [t, toastAll]);
  return { busy, error, setError, run };
}

/** Short text for a toast: the field messages, or the last lines of nginx's test output. */
function errorSummary(e: ApiError): string {
  if (e.kind === "config_rejected" && e.output) {
    const lines = e.output.trim().split("\n").filter((l) => !/test failed|syntax is ok/.test(l));
    return (lines.slice(-2).join("\n") || e.message).slice(0, 300);
  }
  const msgs = e.fields.map((f) => f.message).filter(Boolean);
  return (msgs.length ? msgs.join(" · ") : e.message).slice(0, 300);
}

/** "Saved but not yet applied" banner with redeploy action. */
export function DirtyBanner({ onDeployed }: { onDeployed?: () => void }) {
  const { t } = useI18n();
  const { can } = useCan();
  const { busy, run } = useSubmit({ toastAll: true });
  const redeploy = async () => {
    const r = await run(() => api.post("/api/deployments", {}), { success: t("deployed") });
    if (r !== undefined) { onDeployed?.(); window.dispatchEvent(new Event("corvus:refresh")); }
  };
  return (
    <div role="status" className="flex flex-wrap items-center gap-3 border-b border-warn/40 bg-warn/10 px-4 py-2.5 sm:px-6 lg:px-8">
      <Rocket className="h-4 w-4 shrink-0 text-warn" aria-hidden />
      <div className="min-w-0 flex-1">
        <div className="text-sm font-semibold text-fg">{t("dirtyTitle")}</div>
        <div className="text-xs text-fg-muted">{t("dirtyBanner")}</div>
      </div>
      {can && <Button variant="primary" loading={busy} onClick={redeploy}>{t("redeploy")}</Button>}
    </div>
  );
}
