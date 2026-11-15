import { useState, type HTMLAttributes, type ReactNode } from "react";
import { Check, Copy, type LucideIcon } from "lucide-react";
import { cn, copyText } from "../../lib/utils";
import { useI18n } from "../../i18n";
import { useCan } from "../../lib/rbac";
import { toast } from "sonner";

export type Tone = "neutral" | "ok" | "warn" | "danger" | "info" | "brand";

const toneText: Record<Tone, string> = {
  neutral: "text-fg-muted", ok: "text-ok", warn: "text-warn", danger: "text-danger", info: "text-info", brand: "text-brand",
};
const toneBadge: Record<Tone, string> = {
  neutral: "bg-surface-3/70 text-fg-muted ring-line-strong/70",
  ok: "bg-ok/10 text-ok ring-ok/25", warn: "bg-warn/10 text-warn ring-warn/25",
  danger: "bg-danger/10 text-danger ring-danger/25", info: "bg-info/10 text-info ring-info/25", brand: "bg-brand/10 text-brand ring-brand/25",
};
const toneDot: Record<Tone, string> = {
  neutral: "bg-fg-subtle", ok: "bg-ok", warn: "bg-warn", danger: "bg-danger", info: "bg-info", brand: "bg-brand",
};
export const toneColor = toneText;

export function StatusDot({ tone = "neutral", pulse, className }: { tone?: Tone; pulse?: boolean; className?: string }) {
  return <span aria-hidden className={cn("inline-block h-1.5 w-1.5 shrink-0 rounded-full", toneDot[tone], pulse && "live-dot", className)} />;
}

export function Badge({ tone = "neutral", dot, children, className, icon: Icon }: {
  tone?: Tone; dot?: boolean; children: ReactNode; className?: string; icon?: LucideIcon;
}) {
  return (
    <span className={cn("inline-flex h-5 shrink-0 items-center gap-1.5 whitespace-nowrap rounded-[3px] px-2 text-xs font-medium ring-1 ring-inset", toneBadge[tone], className)}>
      {dot && <StatusDot tone={tone} />}
      {Icon && <Icon className="h-3 w-3" aria-hidden />}
      {children}
    </span>
  );
}

export function Card({ className, children, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("rounded-xl border border-line bg-surface shadow-card", className)} {...rest}>{children}</div>;
}

export function CardHeader({ title, description, actions, icon: Icon, className }: {
  title: ReactNode; description?: ReactNode; actions?: ReactNode; icon?: LucideIcon; className?: string;
}) {
  return (
    <div className={cn("flex items-start justify-between gap-3 px-4 pt-3.5 pb-2", className)}>
      <div className="flex min-w-0 items-center gap-2">
        {Icon && <Icon className="h-4 w-4 shrink-0 text-fg-subtle" aria-hidden />}
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold text-fg">{title}</h3>
          {description && <p className="mt-0.5 text-xs text-fg-subtle">{description}</p>}
        </div>
      </div>
      {actions && <div className="flex shrink-0 items-center gap-1.5">{actions}</div>}
    </div>
  );
}

export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden className={cn("skeleton h-4 w-full", className)} />;
}

export function TableSkeleton({ rows = 5, cols = 5 }: { rows?: number; cols?: number }) {
  return (
    <div role="status" aria-busy="true" className="divide-y divide-line">
      {Array.from({ length: rows }, (_, r) => (
        <div key={r} className="flex items-center gap-6 px-4 py-3.5">
          {Array.from({ length: cols }, (_, c) => <Skeleton key={c} className={cn("h-3.5", c === 0 ? "w-40" : "w-20 flex-1")} />)}
        </div>
      ))}
    </div>
  );
}

export function EmptyState({ icon: Icon, title, description, action, className }: {
  icon: LucideIcon; title: ReactNode; description?: ReactNode; action?: ReactNode; className?: string;
}) {
  const { can } = useCan();
  return (
    <div className={cn("flex flex-col items-center justify-center px-6 py-14 text-center", className)}>
      <div className="mb-4 grid h-12 w-12 place-items-center rounded-full border border-line bg-surface-2 text-fg-subtle">
        <Icon className="h-5 w-5" aria-hidden />
      </div>
      <h3 className="text-sm font-semibold text-fg">{title}</h3>
      {description && <p className="mt-1 max-w-sm text-sm text-fg-subtle">{description}</p>}
      {action && can && <div className="mt-4">{action}</div>}
    </div>
  );
}

/** "Not available yet" placeholder for endpoints the connected backend does not implement. */
export function ComingSoon({ icon, className }: { icon: LucideIcon; className?: string }) {
  const { t } = useI18n();
  return <EmptyState icon={icon} className={className} title={t("comingSoon")} description={t("comingSoonDesc")} />;
}

export function CodeBlock({ code, lines, className, maxHeight = "28rem", copy = true, tone }: {
  code: string; lines?: boolean; className?: string; maxHeight?: string; copy?: boolean; tone?: "danger";
}) {
  const { t } = useI18n();
  const [done, setDone] = useState(false);
  const rows = code.replace(/\n$/, "").split("\n");
  const onCopy = async () => {
    if (await copyText(code)) { setDone(true); toast.success(t("copied")); setTimeout(() => setDone(false), 1500); }
  };
  return (
    <div className={cn("group relative overflow-hidden rounded-lg border", tone === "danger" ? "border-danger/30 bg-danger/5" : "border-line bg-surface-2", className)}>
      {copy && (
        <button type="button" onClick={onCopy} aria-label={t("copy")}
          className="absolute right-2 top-2 z-10 grid h-7 w-7 place-items-center rounded-md border border-line bg-surface text-fg-muted opacity-0 transition hover:text-fg focus-visible:opacity-100 group-hover:opacity-100">
          {done ? <Check className="h-3.5 w-3.5 text-ok" /> : <Copy className="h-3.5 w-3.5" />}
        </button>
      )}
      <div className="overflow-auto" style={{ maxHeight }} tabIndex={0} role="region" aria-label={t("codeRegion")}>
        <pre className="m-0 min-w-max py-3 text-xs leading-5 text-fg">
          {rows.map((r, i) => (
            <div key={i} className="flex">
              {lines && <span aria-hidden className="sticky left-0 w-11 shrink-0 select-none bg-surface-2 pr-3 text-right text-fg-subtle/70">{i + 1}</span>}
              <code className={cn("whitespace-pre pr-4", !lines && "pl-4")}>{r || " "}</code>
            </div>
          ))}
        </pre>
      </div>
    </div>
  );
}

export function Kbd({ children }: { children: ReactNode }) {
  return <kbd className="inline-grid h-5 min-w-5 place-items-center rounded border border-line-strong bg-surface-2 px-1 text-2xs text-fg-subtle">{children}</kbd>;
}

export function Mono({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={cn("font-mono text-xs", className)}>{children}</span>;
}

const calloutCls: Record<Tone, string> = {
  neutral: "border-line bg-surface-2", ok: "border-ok/25 bg-ok/5", warn: "border-warn/30 bg-warn/5",
  danger: "border-danger/30 bg-danger/5", info: "border-info/25 bg-info/5", brand: "border-brand/25 bg-brand/5",
};

/** Inline banner (info / warn / danger / ok) with optional action. */
export function Callout({ tone = "info", icon: Icon, title, children, action, className }: {
  tone?: Tone; icon?: LucideIcon; title?: ReactNode; children?: ReactNode; action?: ReactNode; className?: string;
}) {
  return (
    <div role={tone === "danger" ? "alert" : "status"} className={cn("flex items-start gap-3 rounded-lg border px-3.5 py-3 text-sm", calloutCls[tone], className)}>
      {Icon && <Icon className={cn("mt-0.5 h-4 w-4 shrink-0", toneText[tone])} aria-hidden />}
      <div className="min-w-0 flex-1">
        {title && <div className={cn("font-medium", toneText[tone])}>{title}</div>}
        {children && <div className={cn("text-fg-muted", title && "mt-0.5")}>{children}</div>}
      </div>
      {action}
    </div>
  );
}
