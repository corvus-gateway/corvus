import { Siren } from "lucide-react";
import type { UnderAttackConfig, UnderAttackMode, UnderAttackStatus } from "../api";
import { useI18n, type Key, type TFn } from "../i18n";
import { fmtDateTime } from "../lib/utils";
import { Button } from "./ui/button";
import { Callout } from "./ui/display";
import { Field, FormSection, IdField, NumberInput, Segmented, Select, fieldErr } from "./ui/form";

// "Under attack" mode of a proxy host: mode select, challenge settings and auto thresholds.

export const UA_DEFAULTS: UnderAttackConfig = {
  mode: "js", difficulty: 16, ttl_s: 3600, bind: "ip", rps: 200, blocked_pct: 50, window_s: 30, duration_s: 900,
};

/** Fills unset values (hosts stored before the mode existed) with the defaults. */
export function uaConfig(c?: Partial<UnderAttackConfig> | null): UnderAttackConfig {
  const x = c ?? {};
  return {
    mode: x.mode === "cookie" ? "cookie" : "js",
    difficulty: x.difficulty || UA_DEFAULTS.difficulty,
    ttl_s: x.ttl_s || UA_DEFAULTS.ttl_s,
    bind: x.bind === "ip_ua" ? "ip_ua" : "ip",
    rps: x.rps || UA_DEFAULTS.rps,
    blocked_pct: x.blocked_pct ?? UA_DEFAULTS.blocked_pct,
    window_s: x.window_s || UA_DEFAULTS.window_s,
    duration_s: x.duration_s || UA_DEFAULTS.duration_s,
  };
}

const TTLS = [300, 1800, 3600, 21600, 86400, 259200, 604800] as const;
const ttlKeys: Record<number, Key> = { 300: "chTtl5m", 1800: "chTtl30m", 3600: "chTtl1h", 21600: "chTtl6h", 86400: "chTtl24h", 259200: "chTtl3d", 604800: "chTtl7d" };
const ttlLabel = (t: TFn, s: number) => (ttlKeys[s] ? t(ttlKeys[s]) : t("chTtlSeconds", { n: s }));

/** Active automatic activation: when it ends and why, with "deactivate now". */
export function UnderAttackActive({ st, onDeactivate, busy, can }: { st: UnderAttackStatus; onDeactivate: () => void; busy?: boolean; can: boolean }) {
  const { t, lang } = useI18n();
  return (
    <Callout tone="danger" icon={Siren} title={t("uaBadge")}
      action={can ? <Button size="sm" onClick={onDeactivate} loading={busy}>{t("uaDeactivate")}</Button> : undefined}>
      <p>{t("uaActiveUntil", { until: fmtDateTime(st.until, lang) })}</p>
      {st.reason && <p className="mt-1">{t("uaReason")}: {st.reason}</p>}
      <p className="mt-1 text-fg-subtle">{t("uaDeactivateHint")}</p>
    </Callout>
  );
}

export function UnderAttackFields({ mode, cfg, onChange, error, status, onDeactivate, busy, can }: {
  mode: UnderAttackMode; cfg: UnderAttackConfig;
  onChange: (mode: UnderAttackMode, cfg: UnderAttackConfig) => void;
  error: unknown; status?: UnderAttackStatus; onDeactivate: () => void; busy?: boolean; can: boolean;
}) {
  const { t } = useI18n();
  const set = (p: Partial<UnderAttackConfig>) => onChange(mode, { ...cfg, ...p });
  const err = (f: string) => fieldErr(error, `under_attack_config.${f}`);
  return (
    <FormSection title={t("secUnderAttack")} description={t("secUnderAttackDesc")}>
      {status?.active && mode === "auto" && status.until && <UnderAttackActive st={status} onDeactivate={onDeactivate} busy={busy} can={can} />}
      <Field label={t("uaMode")} hint={mode === "on" ? t("uaOnDesc") : mode === "auto" ? t("uaAutoDesc") : t("uaOffDesc")} error={fieldErr(error, "under_attack")}>
        <Segmented<UnderAttackMode> label={t("uaMode")} value={mode} onChange={(v) => onChange(v, cfg)}
          options={[{ value: "off", label: t("uaOff") }, { value: "on", label: t("uaOn") }, { value: "auto", label: t("uaAuto") }]} />
      </Field>
      {mode !== "off" && (<>
        <Field label={t("chMode")} hint={cfg.mode === "cookie" ? t("chModeCookieDesc") : t("chModeJsDesc")} error={err("mode")}>
          <Segmented<"js" | "cookie"> label={t("chMode")} value={cfg.mode} onChange={(v) => set({ mode: v })}
            options={[{ value: "js", label: t("chModeJs") }, { value: "cookie", label: t("chModeCookie") }]} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          {cfg.mode === "js" && (
            <IdField label={t("chDifficulty")} hint={t("chDifficultyHint")} error={err("difficulty")}>
              {(id) => (
                <Select id={id} value={cfg.difficulty} onChange={(e) => set({ difficulty: Number(e.target.value) })}>
                  {Array.from({ length: 13 }, (_, i) => 12 + i).map((b) => (
                    <option key={b} value={b}>{b}{b === 16 ? ` (${t("chDefault")})` : b === 12 ? ` (${t("chLight")})` : b === 24 ? ` (${t("chHeavy")})` : ""}</option>
                  ))}
                </Select>
              )}
            </IdField>
          )}
          <IdField label={t("chTtl")} hint={t("chTtlHint")} error={err("ttl_s")}>
            {(id) => (
              <Select id={id} value={cfg.ttl_s} onChange={(e) => set({ ttl_s: Number(e.target.value) })}>
                {(TTLS as readonly number[]).includes(cfg.ttl_s) ? null : <option value={cfg.ttl_s}>{ttlLabel(t, cfg.ttl_s)}</option>}
                {TTLS.map((s) => <option key={s} value={s}>{ttlLabel(t, s)}</option>)}
              </Select>
            )}
          </IdField>
        </div>
        <IdField label={t("chBind")} hint={t("chBindHint")} error={err("bind")}>
          {(id) => (
            <Select id={id} value={cfg.bind} onChange={(e) => set({ bind: e.target.value as "ip" | "ip_ua" })}>
              <option value="ip">{t("chBindIp")}</option><option value="ip_ua">{t("chBindIpUa")}</option>
            </Select>
          )}
        </IdField>
      </>)}
      {mode === "auto" && (
        <div className="grid gap-4 sm:grid-cols-2">
          <IdField label={t("uaRps")} hint={t("uaRpsHint")} error={err("rps")}>
            {(id) => <NumberInput id={id} min={1} max={1000000} value={cfg.rps} suffix="r/s" onChange={(v) => set({ rps: v })} />}
          </IdField>
          <IdField label={t("uaBlocked")} hint={t("uaBlockedHint")} error={err("blocked_pct")}>
            {(id) => <NumberInput id={id} min={0} max={100} value={cfg.blocked_pct} suffix="%" onChange={(v) => set({ blocked_pct: v })} />}
          </IdField>
          <IdField label={t("uaWindow")} hint={t("uaWindowHint")} error={err("window_s")}>
            {(id) => <NumberInput id={id} min={10} max={600} step={10} value={cfg.window_s} suffix="s" onChange={(v) => set({ window_s: v })} />}
          </IdField>
          <IdField label={t("uaDuration")} hint={t("uaDurationHint")} error={err("duration_s")}>
            {(id) => <NumberInput id={id} min={1} max={1440} value={Math.round(cfg.duration_s / 60)} suffix="min" onChange={(v) => set({ duration_s: Number.isFinite(v) ? v * 60 : v })} />}
          </IdField>
        </div>
      )}
    </FormSection>
  );
}
