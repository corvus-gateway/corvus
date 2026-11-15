import { createContext, forwardRef, useContext, useId, useState, type InputHTMLAttributes, type KeyboardEvent, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes } from "react";
import * as SwitchPrimitive from "@radix-ui/react-switch";
import { ChevronDown, Search, X } from "lucide-react";
import { ApiError } from "../../api";
import { cn } from "../../lib/utils";
import { useI18n } from "../../i18n";

export function fieldErr(error: unknown, ...fields: string[]): string | undefined {
  if (!(error instanceof ApiError)) return undefined;
  const m = error.fields.filter((f) => fields.includes(f.field)).map((f) => f.message);
  return m.length ? m.join(" / ") : undefined;
}

// A Field tells its controls whether it shows an error and which hint/error text describes
// them, so invalid inputs are marked and announced without wiring every control by hand.
const FieldCtx = createContext<{ invalid: boolean; describedBy?: string } | null>(null);
function useFieldAria(invalid?: boolean) {
  const c = useContext(FieldCtx);
  return { "aria-invalid": (invalid ?? c?.invalid) || undefined, "aria-describedby": c?.describedBy };
}

const control =
  "w-full rounded-[3px] border border-line border-b-line-strong bg-surface text-sm text-fg placeholder:text-fg-subtle transition-colors " +
  "hover:border-b-brand focus:border-b-brand focus:shadow-[inset_0_-1px_0_0_rgb(var(--brand))] focus:outline-none focus-visible:ring-0 disabled:cursor-not-allowed disabled:bg-surface-2 disabled:text-fg-subtle " +
  "aria-[invalid=true]:border-b-danger aria-[invalid=true]:shadow-[inset_0_-1px_0_0_rgb(var(--danger))]";

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean }>(
  function Input({ className, invalid, ...rest }, ref) {
    return <input ref={ref} {...useFieldAria(invalid)} className={cn(control, "h-9 px-3", className)} {...rest} />;
  },
);

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement> & { invalid?: boolean }>(
  function Textarea({ className, invalid, ...rest }, ref) {
    return <textarea ref={ref} {...useFieldAria(invalid)} className={cn(control, "px-3 py-2", className)} {...rest} />;
  },
);

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement> & { invalid?: boolean }>(
  function Select({ className, invalid, children, ...rest }, ref) {
    return (
      <div className="relative">
        <select ref={ref} {...useFieldAria(invalid)} className={cn(control, "h-9 appearance-none pl-3 pr-9", className)} {...rest}>{children}</select>
        <ChevronDown aria-hidden className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-fg-subtle" />
      </div>
    );
  },
);

export function Field({ label, hint, error, required, children, className, htmlFor }: {
  label: ReactNode; hint?: ReactNode; error?: string; required?: boolean; children: ReactNode; className?: string; htmlFor?: string;
}) {
  const id = useId();
  const descId = hint || error ? `${id}-desc` : undefined;
  return (
    <div className={cn("space-y-1.5", className)}>
      <label htmlFor={htmlFor} className="flex items-center gap-1 text-sm font-semibold text-fg">
        {label}{required && <span aria-hidden className="text-danger">*</span>}
      </label>
      <FieldCtx.Provider value={{ invalid: !!error, describedBy: descId }}>{children}</FieldCtx.Provider>
      {hint && !error && <p id={descId} className="text-xs leading-4 text-fg-subtle">{hint}</p>}
      {error && <p id={descId} role="alert" className="text-xs leading-4 text-danger">{error}</p>}
    </div>
  );
}

/** A Field whose child control gets an auto-generated id wired to the label. */
export function IdField(props: Omit<Parameters<typeof Field>[0], "htmlFor" | "children"> & { children: (id: string) => ReactNode }) {
  const id = useId();
  const { children, ...rest } = props;
  return <Field htmlFor={id} {...rest}>{children(id)}</Field>;
}

export function Switch({ checked, onCheckedChange, label, disabled, id, describedBy }: {
  checked: boolean; onCheckedChange: (v: boolean) => void; label: string; disabled?: boolean; id?: string; describedBy?: string;
}) {
  return (
    <SwitchPrimitive.Root
      id={id} checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} aria-label={label} aria-describedby={describedBy}
      className="relative h-[18px] w-8 shrink-0 rounded-full border border-line-strong bg-surface-3 transition-colors data-[state=checked]:border-brand-solid data-[state=checked]:bg-brand-solid disabled:opacity-50"
    >
      <SwitchPrimitive.Thumb className="block h-3.5 w-3.5 translate-x-px rounded-full bg-fg-muted transition-transform duration-100 data-[state=checked]:translate-x-[15px] data-[state=checked]:bg-on-brand" />
    </SwitchPrimitive.Root>
  );
}

export function SwitchRow({ label, description, checked, onChange, disabled }: {
  label: ReactNode; description?: ReactNode; checked: boolean; onChange: (v: boolean) => void; disabled?: boolean;
}) {
  const id = useId();
  return (
    <div className="flex items-start justify-between gap-4 py-2.5">
      <label htmlFor={id} className="min-w-0 flex-1 cursor-pointer">
        <div className="text-sm font-medium text-fg">{label}</div>
        {description && <div id={`${id}-desc`} className="mt-0.5 text-xs leading-4 text-fg-subtle">{description}</div>}
      </label>
      <Switch id={id} describedBy={description ? `${id}-desc` : undefined} checked={checked} onCheckedChange={onChange} disabled={disabled} label={typeof label === "string" ? label : ""} />
    </div>
  );
}

export function Segmented<T extends string>({ value, onChange, options, label, className }: {
  value: T; onChange: (v: T) => void; options: { value: T; label: ReactNode; disabled?: boolean }[]; label: string; className?: string;
}) {
  return (
    <div role="radiogroup" aria-label={label} className={cn("inline-flex w-full divide-x divide-line-strong overflow-hidden rounded-[3px] border border-line-strong bg-surface", className)}>
      {options.map((o) => (
        <button key={o.value} type="button" role="radio" aria-checked={value === o.value} disabled={o.disabled}
          onClick={() => onChange(o.value)}
          className={cn("flex-1 shrink-0 whitespace-nowrap px-3 py-1.5 text-sm font-medium transition-colors disabled:opacity-40",
            value === o.value ? "bg-brand-solid text-on-brand" : "text-fg-muted hover:bg-surface-2 hover:text-fg")}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

/** Chip input: Enter / comma / space / blur commits; Backspace on empty removes the last chip. */
export function TagInput({ value, onChange, placeholder, invalid, label, validate, lineMode, renderChip, disabled }: {
  value: string[]; onChange: (v: string[]) => void; placeholder?: string; invalid?: boolean; label: string; validate?: (v: string) => boolean;
  /** Values may contain spaces/commas: only Enter, blur and pasted newlines commit. */
  lineMode?: boolean; renderChip?: (v: string) => ReactNode; disabled?: boolean;
}) {
  const [draft, setDraft] = useState("");
  const aria = useFieldAria(invalid);
  invalid = !!aria["aria-invalid"];
  const sep = lineMode ? /\r?\n+/ : /[\s,;]+/;
  const commit = (raw: string) => {
    const parts = raw.split(sep).map((s) => s.trim()).filter(Boolean);
    if (!parts.length) return;
    const next = [...value];
    for (const p of parts) if (!next.includes(p)) next.push(p);
    onChange(next); setDraft("");
  };
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" || (!lineMode && (e.key === "," || e.key === " ")) || e.key === "Tab" && draft) {
      if (draft.trim()) { e.preventDefault(); commit(draft); }
      else if (e.key === "Enter") e.preventDefault();
    } else if (e.key === "Backspace" && !draft && value.length) onChange(value.slice(0, -1));
  };
  return (
    <div aria-invalid={invalid || undefined}
      className={cn("flex min-h-9 flex-wrap items-center gap-1.5 rounded-[3px] border border-line border-b-line-strong bg-surface px-2 py-1.5 transition focus-within:border-b-brand focus-within:shadow-[inset_0_-1px_0_0_rgb(var(--brand))]",
        invalid && "border-danger")}>
      {value.map((v) => (
        <span key={v} className={cn("group inline-flex items-center gap-1 rounded-[3px] border border-line bg-surface-2 py-0.5 pl-2 pr-1 font-mono text-xs text-fg",
          validate && !validate(v) && "border-danger/50 text-danger")}>
          {renderChip ? renderChip(v) : v}
          {!disabled && <button type="button" aria-label={`remove ${v}`} onClick={() => onChange(value.filter((x) => x !== v))}
            className="grid h-4 w-4 place-items-center rounded text-fg-subtle hover:bg-surface-3 hover:text-fg"><X className="h-3 w-3" /></button>}
        </span>
      ))}
      <input aria-label={label} aria-invalid={aria["aria-invalid"]} aria-describedby={aria["aria-describedby"]} disabled={disabled} value={draft} onChange={(e) => setDraft(e.target.value)} onKeyDown={onKey} onBlur={() => commit(draft)}
        onPaste={(e) => { const x = e.clipboardData.getData("text"); if ((lineMode ? /\n/ : /[\s,;]/).test(x)) { e.preventDefault(); commit(x); } }}
        placeholder={value.length ? "" : placeholder}
        className="min-w-[8rem] flex-1 bg-transparent px-1 py-0.5 font-mono text-xs text-fg placeholder:font-sans placeholder:text-fg-subtle focus:outline-none" />
    </div>
  );
}

export function SearchInput({ value, onChange, placeholder, className }: {
  value: string; onChange: (v: string) => void; placeholder?: string; className?: string;
}) {
  const { t } = useI18n();
  return (
    <div className={cn("relative", className)}>
      <Search aria-hidden className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-fg-subtle" />
      <input type="search" value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder ?? t("search")} aria-label={placeholder ?? t("search")}
        className={cn(control, "h-8 pl-8 pr-8 [&::-webkit-search-cancel-button]:hidden")} />
      {value && (
        <button type="button" onClick={() => onChange("")} aria-label={t("clear")}
          className="absolute right-1.5 top-1/2 grid h-5 w-5 -translate-y-1/2 place-items-center rounded text-fg-subtle hover:bg-surface-3 hover:text-fg"><X className="h-3 w-3" /></button>
      )}
    </div>
  );
}

export function FormSection({ title, description, children }: { title: ReactNode; description?: ReactNode; children: ReactNode }) {
  return (
    <section className="space-y-4 border-b border-line px-6 py-5 last:border-b-0">
      <div>
        <h3 className="text-sm font-semibold text-fg">{title}</h3>
        {description && <p className="mt-1 text-xs text-fg-subtle">{description}</p>}
      </div>
      {children}
    </section>
  );
}

export function NumberInput({ value, onChange, min, max, step, suffix, ...rest }: {
  value: number; onChange: (v: number) => void; min?: number; max?: number; step?: number; suffix?: string;
} & Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange" | "min" | "max" | "step">) {
  return (
    <div className="relative">
      <Input type="number" inputMode="numeric" min={min} max={max} step={step} value={Number.isFinite(value) ? value : ""}
        onChange={(e) => onChange(e.target.value === "" ? NaN : Number(e.target.value))} className={cn("num", suffix && "pr-10")} {...rest} />
      {suffix && <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-fg-subtle">{suffix}</span>}
    </div>
  );
}
