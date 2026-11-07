import { ChevronRight, Plus, X } from "lucide-react";
import type { ProxyLocation } from "../api";
import { useI18n } from "../i18n";
import { cn } from "../lib/utils";
import { Button, IconButton } from "./ui/button";
import { IdField, Input, NumberInput, Select, Textarea, fieldErr } from "./ui/form";

const blankLocation: ProxyLocation = { path: "", forward_scheme: "http", forward_host: "", forward_port: 80, forward_path: "", advanced_config: "" };

/** Custom locations of a proxy host: a small collapsed section (open when the host has any). */
export function LocationsFields({ value, onChange, error, isAdmin }: {
  value: ProxyLocation[]; onChange: (v: ProxyLocation[]) => void; error: unknown; isAdmin: boolean;
}) {
  const { t } = useI18n();
  const setAt = (i: number, p: Partial<ProxyLocation>) => onChange(value.map((l, j) => (j === i ? { ...l, ...p } : l)));
  const err = (i: number, f: string) => fieldErr(error, `locations[${i}].${f}`);
  return (
    <section className="border-b border-line px-6 py-5">
      <details open={value.length > 0} className="group">
        <summary className="flex cursor-pointer list-none items-center gap-2 text-sm font-semibold text-fg [&::-webkit-details-marker]:hidden">
          <ChevronRight className="h-4 w-4 text-fg-subtle transition-transform group-open:rotate-90" aria-hidden />
          {t("secLocations")}
          {value.length > 0 && <span className="text-xs font-normal text-fg-subtle">{t("locationsCount", { n: value.length })}</span>}
        </summary>
        <div className="mt-4 space-y-4">
          <p className="text-xs leading-5 text-fg-subtle">{t("locationsDesc")}</p>
          {fieldErr(error, "locations") && <p role="alert" className="text-xs text-danger">{fieldErr(error, "locations")}</p>}
          {value.map((l, i) => (
            <div key={i} className="space-y-3 rounded-xl border border-line p-4">
              <div className="flex items-start gap-2">
                <div className="flex-1">
                  <IdField label={t("locPath")} required error={err(i, "path")}>
                    {(id) => <Input id={id} className="font-mono" value={l.path} onChange={(e) => setAt(i, { path: e.target.value })} placeholder="/api" invalid={!!err(i, "path")} />}
                  </IdField>
                </div>
                <IconButton label={t("remove")} className="mt-6" onClick={() => onChange(value.filter((_, j) => j !== i))}><X className="h-4 w-4" /></IconButton>
              </div>
              <div className="grid grid-cols-[6rem_1fr_6rem] gap-3">
                <IdField label={t("scheme")} error={err(i, "forward_scheme")}>
                  {(id) => (
                    <Select id={id} value={l.forward_scheme} onChange={(e) => {
                      const s = e.target.value as "http" | "https";
                      setAt(i, { forward_scheme: s, forward_port: l.forward_port === 80 && s === "https" ? 443 : l.forward_port === 443 && s === "http" ? 80 : l.forward_port });
                    }}>
                      <option value="http">http</option>
                      <option value="https">https</option>
                    </Select>
                  )}
                </IdField>
                <IdField label={t("forwardHost")} required error={err(i, "forward_host")}>
                  {(id) => <Input id={id} className="font-mono" value={l.forward_host} onChange={(e) => setAt(i, { forward_host: e.target.value })} placeholder="api / 10.0.0.12" invalid={!!err(i, "forward_host")} />}
                </IdField>
                <IdField label={t("forwardPort")} required error={err(i, "forward_port")}>
                  {(id) => <NumberInput id={id} min={1} max={65535} value={l.forward_port} onChange={(v) => setAt(i, { forward_port: v })} />}
                </IdField>
              </div>
              <IdField label={t("locForwardPath")} hint={t("locForwardPathHint")} error={err(i, "forward_path")}>
                {(id) => <Input id={id} className="font-mono" value={l.forward_path} onChange={(e) => setAt(i, { forward_path: e.target.value })} placeholder="/v2/" invalid={!!err(i, "forward_path")} />}
              </IdField>
              {(isAdmin || l.advanced_config) && (
                <IdField label={t("locAdvanced")} error={err(i, "advanced_config")}>
                  {(id) => <Textarea id={id} rows={2} spellCheck={false} readOnly={!isAdmin} className={cn("font-mono text-xs leading-5", !isAdmin && "bg-surface-2 text-fg-muted")}
                    value={l.advanced_config} onChange={(e) => setAt(i, { advanced_config: e.target.value })} placeholder="# proxy_read_timeout 300s;" invalid={!!err(i, "advanced_config")} />}
                </IdField>
              )}
            </div>
          ))}
          <Button size="sm" onClick={() => onChange([...value, { ...blankLocation, path: value.length ? "" : "/api" }])}>
            <Plus className="h-3.5 w-3.5" />{t("addLocation")}
          </Button>
        </div>
      </details>
    </section>
  );
}
