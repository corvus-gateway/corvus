import { Fragment, useMemo } from "react";
import { AlertTriangle, ListChecks, Plus, Split, X } from "lucide-react";
import type { IpList, RuleField, RuleOperator } from "../api";
import { useI18n } from "../i18n";
import {
  COUNTRIES, FIELDS, LIMITS, METHODS, SCHEMES, changeField, changeOperator, countryName, fieldLabelFor, fieldMeta, flag, isExistsOp,
  isIpOrCidr, isListRef, isSetOp, isStringField, listKind, listName, newRow, opLabelFor, operatorsFor, toGroups, type CondRow, type RuleErrors,
} from "../lib/rules";
import { cn } from "../lib/utils";
import { Button, IconButton } from "./ui/button";
import { Input, Select, TagInput } from "./ui/form";
import { ChipToggles, MultiPicker } from "./ui/picker";

/** Cloudflare-style expression builder over a flat row list (AND within a group, OR between groups). */
export function RuleExpression({ rows, onChange, errors, lists, geoipAvailable, disabled }: {
  rows: CondRow[]; onChange: (rows: CondRow[]) => void; errors: RuleErrors; lists: IpList[] | null; geoipAvailable: boolean | null; disabled?: boolean;
}) {
  const { t } = useI18n();
  const groups = useMemo(() => toGroups(rows), [rows]);
  const lastGroupSize = groups[groups.length - 1]?.length ?? 0;

  const patch = (i: number, p: Partial<CondRow>) => onChange(rows.map((r, j) => (j === i ? { ...r, ...p } : r)));
  const remove = (i: number) => {
    const next = rows.filter((_, j) => j !== i);
    // Removing a group's first row keeps the following row as the group start.
    if (rows[i]?.or && next[i] && !next[i].or) next[i] = { ...next[i], or: true };
    onChange(next.length ? next.map((r, j) => (j === 0 ? { ...r, or: false } : r)) : [newRow()]);
  };

  // Group boundaries for rendering boxes.
  const boxes: number[][] = [];
  rows.forEach((r, i) => { if (i === 0 || r.or) boxes.push([]); boxes[boxes.length - 1].push(i); });

  return (
    <div className="space-y-2">
      {boxes.map((box, bi) => (
        <Fragment key={rows[box[0]].key}>
          {bi > 0 && <Connector or index={box[0]} rows={rows} onChange={onChange} disabled={disabled} />}
          <div role="group" aria-label={t("condGroupN", { n: bi + 1 })} className="rounded-[4px] border border-line bg-surface-2/40 p-3">
            {box.map((i, k) => (
              <Fragment key={rows[i].key}>
                {k > 0 && <Connector index={i} rows={rows} onChange={onChange} disabled={disabled} />}
                <CondEditor row={rows[i]} index={i} onPatch={(p) => patch(i, p)} onRemove={() => remove(i)}
                  error={errors[`row:${i}`]} nameError={errors[`rowName:${i}`]} lists={lists} geoipAvailable={geoipAvailable} disabled={disabled} />
              </Fragment>
            ))}
          </div>
        </Fragment>
      ))}
      {errors.expression && <p role="alert" className="text-xs text-danger">{errors.expression}</p>}
      {!disabled && (
        <div className="flex flex-wrap gap-2 pt-1">
          <Button size="sm" disabled={lastGroupSize >= LIMITS.conds} onClick={() => onChange([...rows, newRow()])}><Plus className="h-3.5 w-3.5" />{t("addAndCond")}</Button>
          <Button size="sm" disabled={groups.length >= LIMITS.groups} onClick={() => onChange([...rows, newRow({}, true)])}><Split className="h-3.5 w-3.5" />{t("addOrGroup")}</Button>
          <span className="self-center text-2xs text-fg-subtle">{t("exprLimits", { g: LIMITS.groups, c: LIMITS.conds })}</span>
        </div>
      )}
    </div>
  );
}

function Connector({ index, rows, onChange, or, disabled }: { index: number; rows: CondRow[]; onChange: (r: CondRow[]) => void; or?: boolean; disabled?: boolean }) {
  const { t } = useI18n();
  const set = (v: boolean) => onChange(rows.map((r, j) => (j === index ? { ...r, or: v } : r)));
  const isOr = !!or;
  return (
    <div className={cn("flex items-center gap-2", isOr ? "py-0.5" : "py-2")}>
      <span aria-hidden className="h-px flex-1 bg-line" />
      <div role="radiogroup" aria-label={t("connector")} className="inline-flex overflow-hidden rounded-[3px] border border-line-strong bg-surface text-2xs font-semibold">
        {([false, true] as const).map((v) => (
          <button key={String(v)} type="button" role="radio" aria-checked={isOr === v} disabled={disabled} onClick={() => isOr !== v && set(v)}
            className={cn("px-2.5 py-1 transition-colors disabled:cursor-default",
              isOr === v ? (v ? "bg-warn/15 text-warn" : "bg-brand-solid text-on-brand") : "text-fg-subtle hover:bg-surface-2 hover:text-fg")}>
            {v ? t("connOr") : t("connAnd")}
          </button>
        ))}
      </div>
      <span aria-hidden className="h-px flex-1 bg-line" />
    </div>
  );
}

function ListChips({ refs, lists, disabled, onRemove }: { refs: string[]; lists: IpList[] | null; disabled?: boolean; onRemove: (ref: string) => void }) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {refs.map((r) => (
        <span key={r} className="inline-flex items-center gap-1 rounded-[3px] border border-brand/30 bg-brand/10 py-0.5 pl-2 pr-1 text-xs text-brand">
          <ListChecks className="h-3 w-3" aria-hidden />${listName(r, lists)}
          {!disabled && <button type="button" aria-label={`remove ${listName(r, lists)}`} onClick={() => onRemove(r)}
            className="grid h-4 w-4 place-items-center rounded hover:bg-brand/15"><X className="h-3 w-3" /></button>}
        </span>
      ))}
    </div>
  );
}

function CondEditor({ row, index, onPatch, onRemove, error, nameError, lists, geoipAvailable, disabled }: {
  row: CondRow; index: number; onPatch: (p: Partial<CondRow>) => void; onRemove: () => void; error?: string; nameError?: string;
  lists: IpList[] | null; geoipAvailable: boolean | null; disabled?: boolean;
}) {
  const { t, lang } = useI18n();
  const c = row.c;
  const meta = fieldMeta(c.field);
  const ops = operatorsFor(c.field);
  const setC = (p: Partial<typeof c>) => onPatch({ c: { ...c, ...p } });
  const n = index + 1;
  const valueLabel = t("condValueN", { n });

  const valueEditor = () => {
    if (isExistsOp(c.operator)) return null;
    switch (meta.kind) {
      case "ualist": {
        const refs = c.value.filter(isListRef);
        const uaLists = (lists ?? []).filter((l) => listKind(l) === "user_agent");
        const avail = uaLists.filter((l) => !refs.includes(`list:${l.id}`));
        return (
          <div className="space-y-2">
            {refs.length > 0 && <ListChips refs={refs} lists={lists} disabled={disabled} onRemove={(r) => setC({ value: c.value.filter((x) => x !== r) })} />}
            {!disabled && (
              <div className="w-full sm:w-64">
                <Select aria-label={t("addUaList")} value="" disabled={!avail.length} invalid={!!error && !refs.length} className="!h-8 text-xs"
                  onChange={(e) => e.target.value && setC({ value: [...refs, `list:${e.target.value}`] })}>
                  <option value="">{uaLists.length ? t("addUaList") : t("noUaListsYet")}</option>
                  {avail.map((l) => <option key={l.id} value={l.id}>{l.name} ({t("nItems", { n: l.items?.length ?? 0 })})</option>)}
                </Select>
              </div>
            )}
          </div>
        );
      }
      case "ip": {
        const refs = c.value.filter(isListRef);
        const ips = c.value.filter((v) => !isListRef(v));
        const ipLists = (lists ?? []).filter((l) => listKind(l) === "ip");
        const avail = ipLists.filter((l) => !refs.includes(`list:${l.id}`));
        return (
          <div className="space-y-2">
            {refs.length > 0 && <ListChips refs={refs} lists={lists} disabled={disabled} onRemove={(r) => setC({ value: c.value.filter((x) => x !== r) })} />}
            <TagInput label={valueLabel} value={ips} onChange={(v) => setC({ value: [...refs, ...v] })} validate={isIpOrCidr} invalid={!!error} disabled={disabled}
              placeholder="203.0.113.7, 10.0.0.0/8, 2001:db8::/32" />
            {!disabled && (
              <div className="w-full sm:w-64">
                <Select aria-label={t("addIpList")} value="" disabled={!avail.length} className="!h-8 text-xs"
                  onChange={(e) => e.target.value && setC({ value: [...refs, `list:${e.target.value}`, ...ips] })}>
                  <option value="">{ipLists.length ? t("addIpList") : t("noIpListsYet")}</option>
                  {avail.map((l) => <option key={l.id} value={l.id}>{l.name} ({t("nItems", { n: l.items?.length ?? 0 })})</option>)}
                </Select>
              </div>
            )}
          </div>
        );
      }
      case "country":
        return (
          <div className="space-y-2">
            <MultiPicker label={valueLabel} value={c.value} onChange={(v) => setC({ value: v })} invalid={!!error} disabled={disabled} placeholder={t("searchCountry")}
              options={COUNTRIES.map((code) => ({ value: code, label: countryName(code, lang), hint: code, prefix: <span aria-hidden className="w-5 text-center">{flag(code)}</span> }))}
              renderChip={(v) => <span title={countryName(v, lang)}>{flag(v)} {v} <span className="text-fg-subtle">{countryName(v, lang)}</span></span>} />
            {geoipAvailable === false && <p className="flex items-center gap-1.5 text-xs text-warn"><AlertTriangle className="h-3.5 w-3.5" aria-hidden />{t("geoipMissingCond")}</p>}
          </div>
        );
      case "method":
        return <ChipToggles label={valueLabel} options={METHODS} value={c.value} onChange={(v) => setC({ value: v })} disabled={disabled} invalid={!!error} />;
      case "scheme":
        return <ChipToggles label={valueLabel} options={SCHEMES} value={c.value} onChange={(v) => setC({ value: v })} disabled={disabled} invalid={!!error} />;
      default:
        return isSetOp(c.operator)
          ? <TagInput lineMode label={valueLabel} value={c.value} onChange={(v) => setC({ value: v })} invalid={!!error} disabled={disabled} placeholder={t("tagLineHint")} />
          : <Input aria-label={valueLabel} className="font-mono text-xs" value={c.value[0] ?? ""} disabled={disabled} invalid={!!error} maxLength={LIMITS.valueLen}
            placeholder={c.operator === "matches" || c.operator === "not_matches" ? "^/(wp-admin|xmlrpc\\.php)" : c.field === "http.path" ? "/admin" : ""}
            onChange={(e) => setC({ value: [e.target.value] })} />;
    }
  };

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-start gap-2">
        <div className="w-full sm:w-40">
          <Select aria-label={t("condFieldN", { n })} value={c.field} disabled={disabled} onChange={(e) => onPatch({ c: changeField(c, e.target.value as RuleField) })}>
            {FIELDS.map((f) => <option key={f.id} value={f.id}>{fieldLabelFor(lang)[f.id]}</option>)}
          </Select>
        </div>
        {c.field === "http.header" && (
          <div className="w-full sm:w-40">
            <Input aria-label={t("headerNameN", { n })} className="font-mono text-xs" placeholder="X-Api-Key" value={c.name ?? ""} disabled={disabled} invalid={!!nameError}
              onChange={(e) => setC({ name: e.target.value })} />
          </div>
        )}
        <div className="min-w-0 flex-1">
          <Select aria-label={t("condOpN", { n })} value={c.operator} disabled={disabled} onChange={(e) => onPatch({ c: changeOperator(c, e.target.value as RuleOperator) })}>
            {ops.map((o) => <option key={o} value={o}>{opLabelFor(lang)[o]}</option>)}
          </Select>
        </div>
        {!disabled && <IconButton label={t("removeCondN", { n })} onClick={onRemove} className="mt-0.5"><X className="h-4 w-4" /></IconButton>}
      </div>
      {valueEditor()}
      {isStringField(c.field) && !isExistsOp(c.operator) && (
        <label className="flex w-fit cursor-pointer items-center gap-2 text-xs text-fg-muted">
          <input type="checkbox" checked={!!c.ci} disabled={disabled} onChange={(e) => setC({ ci: e.target.checked })} className="h-3.5 w-3.5 accent-[rgb(var(--brand-solid))]" />
          {t("caseInsensitive")}
        </label>
      )}
      {nameError && <p role="alert" className="text-xs text-danger">{nameError}</p>}
      {error && <p role="alert" className="text-xs text-danger">{error}</p>}
    </div>
  );
}
