import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import * as Dropdown from "@radix-ui/react-dropdown-menu";
import * as Tooltip from "@radix-ui/react-tooltip";
import { AlertTriangle, MoreHorizontal, X, type LucideIcon } from "lucide-react";
import { cn } from "../../lib/utils";
import { useI18n } from "../../i18n";
import { useCan } from "../../lib/rbac";
import { Button, IconButton } from "./button";

/** True while `value` (a form being edited; null = closed) differs from how it was when it opened. */
export function useDirty(value: unknown): boolean {
  const base = useRef<string | null>(null);
  if (value == null) { base.current = null; return false; }
  const now = JSON.stringify(value);
  if (base.current === null) base.current = now;
  return base.current !== now;
}

/** Cancel button for a Sheet footer: closes through the Sheet, so unsaved changes are confirmed. */
export function SheetCancel({ children }: { children?: ReactNode }) {
  const { t } = useI18n();
  return <Dialog.Close asChild><Button>{children ?? t("cancel")}</Button></Dialog.Close>;
}

/** Right-hand slide-over. With `onSubmit` the body + footer are wrapped in a <form>.
 *  `dirty`: closing (Esc, outside click, ×, SheetCancel) asks before discarding changes.
 *  `error`: when it changes to a failure, the first invalid field is scrolled to and focused. */
export function Sheet({ open, onOpenChange, title, description, children, footer, onSubmit, width = "max-w-[34rem]", dirty = false, error }: {
  open: boolean; onOpenChange: (o: boolean) => void; title: ReactNode; description?: ReactNode;
  children: ReactNode; footer?: ReactNode; onSubmit?: () => void; width?: string; dirty?: boolean; error?: unknown;
}) {
  const { t } = useI18n();
  const [confirm, setConfirm] = useState(false);
  const body = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!error || !open) return;
    // After the error has rendered: the first field marked invalid, else the error box at the top.
    const id = requestAnimationFrame(() => {
      const el = body.current?.querySelector<HTMLElement>('[aria-invalid="true"]');
      if (el) {
        el.scrollIntoView({ block: "center", behavior: "smooth" });
        (el.matches("input, select, textarea, button, [tabindex]") ? el : el.querySelector<HTMLElement>("input, select, textarea, button") ?? el).focus({ preventScroll: true });
      } else body.current?.scrollTo({ top: 0, behavior: "smooth" });
    });
    return () => cancelAnimationFrame(id);
  }, [error, open]);
  const change = (o: boolean) => {
    if (!o && dirty) { setConfirm(true); return; }
    onOpenChange(o);
  };
  const inner = (
    <>
      <div ref={body} className="min-h-0 flex-1 overflow-y-auto overscroll-contain">{children}</div>
      {footer && <div className="flex shrink-0 items-center justify-end gap-2 border-t border-line bg-surface px-6 py-3.5">{footer}</div>}
    </>
  );
  return (
    <Dialog.Root open={open} onOpenChange={change}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-black/60 data-[state=open]:animate-fade-in data-[state=closed]:animate-fade-out" />
        <Dialog.Content
          aria-describedby={description ? undefined : undefined}
          className={cn("fixed inset-y-0 right-0 z-50 flex w-full flex-col border-l border-line bg-surface shadow-sheet focus:outline-none data-[state=open]:animate-slide-in-right data-[state=closed]:animate-slide-out-right", width)}>
          <div className="flex shrink-0 items-start justify-between gap-4 border-b border-line px-6 py-4">
            <div className="min-w-0">
              <Dialog.Title className="truncate text-base font-semibold tracking-tight text-fg">{title}</Dialog.Title>
              <Dialog.Description className={cn("mt-0.5 text-xs text-fg-subtle", !description && "sr-only")}>{description ?? String(title)}</Dialog.Description>
            </div>
            <Dialog.Close asChild><IconButton label={t("close")}><X className="h-4 w-4" /></IconButton></Dialog.Close>
          </div>
          {onSubmit ? (
            <form className="flex min-h-0 flex-1 flex-col" onSubmit={(e: FormEvent) => { e.preventDefault(); onSubmit(); }}>{inner}</form>
          ) : inner}
          <ConfirmDialog open={confirm} onOpenChange={setConfirm} title={t("discardTitle")} description={t("discardDesc")}
            confirmLabel={t("discardConfirm")} cancelLabel={t("keepEditing")} onConfirm={() => { setConfirm(false); onOpenChange(false); }} />
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

/** Centered modal for confirmations and short flows. */
export function Modal({ open, onOpenChange, title, description, children, footer, width = "max-w-md" }: {
  open: boolean; onOpenChange: (o: boolean) => void; title: ReactNode; description?: ReactNode; children?: ReactNode; footer?: ReactNode; width?: string;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-black/60 data-[state=open]:animate-fade-in data-[state=closed]:animate-fade-out" />
        <Dialog.Content className={cn("fixed left-1/2 top-1/2 z-50 w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 rounded-[4px] border border-line bg-surface shadow-pop focus:outline-none data-[state=open]:animate-pop-in", width)}>
          <div className="px-5 pt-5">
            <Dialog.Title className="text-base font-semibold tracking-tight text-fg">{title}</Dialog.Title>
            <Dialog.Description className={cn("mt-1.5 text-sm text-fg-muted", !description && "sr-only")}>{description ?? String(title)}</Dialog.Description>
          </div>
          {children && <div className="px-5 pt-3">{children}</div>}
          {footer && <div className="mt-5 flex justify-end gap-2 border-t border-line px-5 py-3">{footer}</div>}
          {!footer && <div className="h-5" />}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

export function ConfirmDialog({ open, onOpenChange, title, description, confirmLabel, cancelLabel, onConfirm, danger = true, children }: {
  open: boolean; onOpenChange: (o: boolean) => void; title: ReactNode; description?: ReactNode; confirmLabel: string; cancelLabel?: string;
  onConfirm: () => void | Promise<void>; danger?: boolean; children?: ReactNode;
}) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const run = async () => {
    setBusy(true);
    try { await onConfirm(); onOpenChange(false); } finally { setBusy(false); }
  };
  return (
    <Modal open={open} onOpenChange={onOpenChange} title={
      <span className="flex items-center gap-2">{danger && <AlertTriangle className="h-4 w-4 text-danger" aria-hidden />}{title}</span>}
      description={description}
      footer={<>
        <Button onClick={() => onOpenChange(false)} disabled={busy}>{cancelLabel ?? t("cancel")}</Button>
        <Button variant={danger ? "danger" : "primary"} loading={busy} onClick={run}>{confirmLabel}</Button>
      </>}>
      {children}
    </Modal>
  );
}

export interface MenuItem {
  label: string; icon?: LucideIcon; onSelect: () => void; danger?: boolean; hidden?: boolean; disabled?: boolean; separatorBefore?: boolean;
}

const menuContent =
  "z-50 min-w-[11rem] overflow-hidden rounded-[3px] border border-line bg-surface py-1 shadow-pop data-[state=open]:animate-pop-in";
const menuItem =
  "flex cursor-pointer select-none items-center gap-2 px-3 py-1.5 text-sm text-fg outline-none data-[disabled]:pointer-events-none data-[disabled]:opacity-40 data-[highlighted]:bg-surface-3/70";

export function RowMenu({ items, label }: { items: MenuItem[]; label?: string }) {
  const { t } = useI18n();
  const { can, isAdmin } = useCan();
  // Viewers get no mutation menus; a menu with nothing visible renders nothing.
  const visible = can || isAdmin ? items.filter((i) => !i.hidden) : [];
  if (!visible.length) return null;
  return (
    <Dropdown.Root>
      <Dropdown.Trigger asChild>
        <IconButton label={label ?? t("actions")} className="data-[state=open]:bg-surface-3"><MoreHorizontal className="h-4 w-4" /></IconButton>
      </Dropdown.Trigger>
      <Dropdown.Portal>
        <Dropdown.Content align="end" sideOffset={4} className={menuContent}>
          {visible.map((i, idx) => (
            <div key={idx}>
              {i.separatorBefore && idx > 0 && <Dropdown.Separator className="my-1 h-px bg-line" />}
              <Dropdown.Item disabled={i.disabled} onSelect={i.onSelect} className={cn(menuItem, i.danger && "text-danger data-[highlighted]:bg-danger/10")}>
                {i.icon && <i.icon className="h-3.5 w-3.5 opacity-70" aria-hidden />}{i.label}
              </Dropdown.Item>
            </div>
          ))}
        </Dropdown.Content>
      </Dropdown.Portal>
    </Dropdown.Root>
  );
}

export const Menu = {
  Root: Dropdown.Root, Trigger: Dropdown.Trigger, Portal: Dropdown.Portal,
  Content: ({ className, ...p }: Dropdown.DropdownMenuContentProps) => <Dropdown.Content sideOffset={6} className={cn(menuContent, className)} {...p} />,
  Item: ({ className, ...p }: Dropdown.DropdownMenuItemProps) => <Dropdown.Item className={cn(menuItem, className)} {...p} />,
  Label: ({ className, ...p }: Dropdown.DropdownMenuLabelProps) => <Dropdown.Label className={cn("px-2.5 py-1.5 text-xs text-fg-subtle", className)} {...p} />,
  Separator: () => <Dropdown.Separator className="my-1 h-px bg-line" />,
  RadioGroup: Dropdown.RadioGroup,
  RadioItem: ({ className, children, ...p }: Dropdown.DropdownMenuRadioItemProps) => (
    <Dropdown.RadioItem className={cn(menuItem, "justify-between data-[state=checked]:font-medium", className)} {...p}>
      {children}
      <Dropdown.ItemIndicator><span className="block h-1.5 w-1.5 rounded-full bg-brand" /></Dropdown.ItemIndicator>
    </Dropdown.RadioItem>
  ),
};

/** Classes for a focusable tooltip trigger (`<span tabIndex={0}>` around a badge whose details are in the tooltip). */
export const tipTrigger = "rounded focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand";

export function Tip({ label, children, side = "top" }: { label: ReactNode; children: ReactNode; side?: "top" | "right" | "bottom" | "left" }) {
  return (
    <Tooltip.Root delayDuration={250}>
      <Tooltip.Trigger asChild>{children}</Tooltip.Trigger>
      <Tooltip.Portal>
        <Tooltip.Content side={side} sideOffset={6}
          className="z-[60] max-w-xs rounded-md border border-line bg-fg px-2 py-1 text-xs text-bg shadow-pop data-[state=delayed-open]:animate-pop-in">
          {label}
        </Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  );
}
export const TooltipProvider = Tooltip.Provider;
