import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { ArrowLeft, ChevronRight, Eye, EyeOff, KeyRound } from "lucide-react";
import { api, type LoginResult, type User } from "../api";
import { Wordmark } from "../components/Logo";
import { ErrorBox, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Field, Input, fieldErr } from "../components/ui/form";
import { LANGS, LANG_NAMES, isLang, useI18n, type Key } from "../i18n";
import { useTheme } from "../lib/theme";
import { cn } from "../lib/utils";

// Enterprise sign-in: calm navy field with a faint hexagonal mesh, a compact square card on the
// right (product name, labelled fields, full-width primary button, grey "Server: …" band).
// No marketing copy.

/** Deep navy field, very low-contrast hex mesh and a vignette (pure SVG, no assets). */
function Backdrop() {
  return (
    <svg aria-hidden className="pointer-events-none absolute inset-0 h-full w-full" preserveAspectRatio="xMidYMid slice">
      <defs>
        <pattern id="bd-hex" width="28" height="49" patternUnits="userSpaceOnUse" patternTransform="scale(2.4)">
          <path fill="#9bb4d6" fillOpacity="0.06" fillRule="evenodd"
            d="M13.99 9.25l13 7.5v15l-13 7.5L1 31.75v-15l12.99-7.5zM3 17.9v12.7l10.99 6.34 11-6.35V17.9l-11-6.34L3 17.9zM0 15l12.98-7.5V0h-2v6.35L0 12.69v2.3zm0 18.5L12.98 41v8h-2v-6.85L0 35.81v-2.3zM15 0v7.5L27.99 15H28v-2.31h-.01L17 6.35V0h-2zm0 49v-8l12.99-7.5H28v2.31h-.01L17 42.15V49h-2z" />
        </pattern>
        <radialGradient id="bd-vignette" cx="0.3" cy="0.4" r="0.85">
          <stop offset="0" stopColor="#000" stopOpacity="0" />
          <stop offset="0.65" stopColor="#000" stopOpacity="0.18" />
          <stop offset="1" stopColor="#000" stopOpacity="0.6" />
        </radialGradient>
        <radialGradient id="bd-mask" cx="0.62" cy="0.38" r="0.7">
          <stop offset="0" stopColor="#fff" /><stop offset="1" stopColor="#fff" stopOpacity="0.25" />
        </radialGradient>
        <mask id="bd-fade"><rect width="100%" height="100%" fill="url(#bd-mask)" /></mask>
      </defs>
      <rect width="100%" height="100%" fill="#0e1520" />
      <rect width="100%" height="100%" fill="url(#bd-hex)" mask="url(#bd-fade)" />
      <rect width="100%" height="100%" fill="url(#bd-vignette)" />
    </svg>
  );
}

function AuthLayout({ children, title }: { children: ReactNode; title?: string }) {
  const { t } = useI18n();
  return (
    <div className="cockpit-auth relative min-h-dvh overflow-hidden bg-[#0e1520]">
      <Backdrop />
      <div className="absolute left-5 top-5 z-10 sm:left-8 sm:top-6">
        <div className="flex items-center gap-2.5">
          <Wordmark className="h-6 w-auto text-white" />
          <span className="text-xs font-semibold tracking-[.12em] text-white/65">AWPG</span>
        </div>
        <p className="mt-1.5 text-[11px] font-medium tracking-[.08em] text-white/50">Active Web Protection Gateway</p>
      </div>
      <div className="relative flex min-h-dvh items-start justify-center px-4 pb-8 pt-24 sm:justify-end sm:px-[8vw] sm:pt-[22vh]">
        <section className="ck-card w-full max-w-[26rem]">
          <div className="px-6 pb-7 pt-8 sm:px-9 sm:pt-9">
            <h1 className="ck-brand">{title ?? "Corvus AWPG"}</h1>
            {children}
          </div>
          <footer className="ck-footer px-6 py-4 sm:px-9">
            <p className="font-semibold">{t("ckServer")}: {location.hostname}</p>
            <p className="mt-1">{t("ckHint")}</p>
          </footer>
        </section>
      </div>
    </div>
  );
}

function Heading({ title, sub }: { title: string; sub?: string }) {
  return (
    <div className="mb-6">
      <h2 className="text-lg font-semibold">{title}</h2>
      {sub && <p className="ck-muted mt-1.5 text-[15px]">{sub}</p>}
    </div>
  );
}

/** Cockpit-like field: bold label, rectangular input with an accent underline on focus. */
function CkField({ id, label, children }: { id: string; label: string; children: ReactNode }) {
  return (
    <div className="mb-4">
      <label htmlFor={id} className="ck-label">{label}</label>
      {children}
    </div>
  );
}

/** "Other options" expander: language and theme (kept out of the way, as in Cockpit). */
function OtherOptions() {
  const { t, lang, setLang } = useI18n();
  const { mode, setMode } = useTheme();
  const [open, setOpen] = useState(false);
  return (
    <div className="mb-6">
      <button type="button" aria-expanded={open} onClick={() => setOpen(!open)} className="ck-link inline-flex items-center gap-1.5">
        <ChevronRight className={cn("h-4 w-4 transition-transform", open && "rotate-90")} aria-hidden />{t("ckOtherOptions")}
      </button>
      {open && (
        <div className="mt-4 grid gap-4 sm:grid-cols-2">
          <div>
            <label htmlFor="ck-lang" className="ck-label">{t("language")}</label>
            <select id="ck-lang" className="ck-input" value={lang} onChange={(e) => isLang(e.target.value) && setLang(e.target.value)}>
              {LANGS.map((l) => <option key={l} value={l} lang={l}>{LANG_NAMES[l]}</option>)}
            </select>
          </div>
          <div>
            <label htmlFor="ck-theme" className="ck-label">{t("theme")}</label>
            <select id="ck-theme" className="ck-input" value={mode} onChange={(e) => setMode(e.target.value as typeof mode)}>
              <option value="system">{t("themeSystem")}</option><option value="light">{t("themeLight")}</option><option value="dark">{t("themeDark")}</option>
            </select>
          </div>
        </div>
      )}
    </div>
  );
}

export function LoginPage({ onDone }: { onDone: (u: User) => void }) {
  const { t } = useI18n();
  const [f, setF] = useState({ email: "", password: "" });
  const [show, setShow] = useState(false);
  const [mfa, setMfa] = useState<string | null>(null);
  const { busy, error, setError, run } = useSubmit();

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const r = await run(() => api.post<LoginResult>("/api/auth/login", f), { deployed: false, silentError: true });
    if (!r) return;
    if ("mfa_required" in r && r.mfa_required) setMfa(r.mfa_token);
    else onDone(r as User);
  };

  if (mfa) {
    return (
      <AuthLayout>
        <MfaStep token={mfa} onDone={onDone} onBack={() => { setMfa(null); setError(null); }} />
      </AuthLayout>
    );
  }
  return (
    <AuthLayout>
      <form onSubmit={submit} noValidate={false}>
        <ErrorBox error={error} className="mb-6" />
        <SsoError />
        <CkField id="email" label={t("email")}>
          <input id="email" className="ck-input" type="email" required autoFocus autoComplete="username" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} />
        </CkField>
        <CkField id="password" label={t("password")}>
          <div className="flex">
            <input id="password" className="ck-input min-w-0 flex-1" type={show ? "text" : "password"} required autoComplete="current-password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} />
            <button type="button" className="ck-eye" aria-label={show ? t("ckHidePassword") : t("ckShowPassword")} aria-pressed={show} onClick={() => setShow(!show)}>
              {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </button>
          </div>
        </CkField>
        <OtherOptions />
        <button type="submit" className="ck-button" disabled={busy}>{busy ? t("ckSigningIn") : t("login")}</button>
        <SsoButton />
      </form>
    </AuthLayout>
  );
}

// ------------------------------------------------------------------ SSO (OIDC)

const SSO_ERRORS: Record<string, Key> = {
  state: "ssoErrState", expired: "ssoErrExpired", disabled: "ssoErrDisabled", provider_unavailable: "ssoErrProvider", config: "ssoErrConfig",
  idp_error: "ssoErrIdp", exchange: "ssoErrExchange", token_invalid: "ssoErrToken", no_email: "ssoErrNoEmail", email_unverified: "ssoErrEmailUnverified",
  domain: "ssoErrDomain", no_role: "ssoErrNoRole", not_provisioned: "ssoErrNotProvisioned", user_disabled: "ssoErrUserDisabled",
  rate_limited: "ssoErrRateLimited", busy: "ssoErrBusy", internal: "ssoErrInternal",
  link_unverified: "ssoErrLinkUnverified", link_conflict: "ssoErrLinkConflict",
};

/** Reads (once) and removes `?sso_error=` left by a failed SSO callback. */
function useSsoError(): string | null {
  const [code] = useState(() => new URLSearchParams(location.search).get("sso_error"));
  useEffect(() => {
    if (code) history.replaceState(null, "", "/");
  }, [code]);
  return code;
}

/** Friendly message for a failed SSO attempt (from the callback redirect). */
function SsoError() {
  const { t } = useI18n();
  const errCode = useSsoError();
  if (!errCode) return null;
  return (
    <div role="alert" className="mb-6 border-l-4 px-3 py-2.5 text-[13.5px]" style={{ borderColor: "#c9190b", background: "#faeae8", color: "#7d1007" }}>
      <p className="font-semibold">{t("ssoErrTitle")}</p>
      <p className="mt-0.5">{t(SSO_ERRORS[errCode] ?? "ssoErrInternal")}</p>
    </div>
  );
}

/** Secondary "Sign in with …" button, shown when SSO is configured. */
function SsoButton() {
  const { t } = useI18n();
  const [cfg, setCfg] = useState<{ enabled: boolean; display_name: string } | null>(null);
  const [going, setGoing] = useState(false);
  useEffect(() => {
    let live = true;
    api.get<{ enabled: boolean; display_name: string }>("/api/auth/sso").then((c) => { if (live) setCfg(c); }).catch(() => { /* older backend: no SSO */ });
    return () => { live = false; };
  }, []);
  const label = !cfg?.display_name || cfg.display_name === "SSO" ? t("ssoButtonDefault") : cfg.display_name;
  if (!cfg?.enabled) return null;
  return (
    <>
      <div className="my-4 flex items-center gap-3 text-xs" style={{ color: "var(--ck-muted)" }} aria-hidden>
        <span className="h-px flex-1" style={{ background: "var(--ck-under)", opacity: 0.4 }} />{t("ssoOr")}<span className="h-px flex-1" style={{ background: "var(--ck-under)", opacity: 0.4 }} />
      </div>
      <a href="/api/auth/oidc/start" onClick={() => setGoing(true)} aria-busy={going}
        className="flex h-[2.4rem] w-full items-center justify-center gap-2 rounded-[3px] border text-[14.5px] font-medium transition-colors hover:bg-black/[.04] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"
        style={{ borderColor: "var(--ck-accent)", color: "var(--ck-accent)", outlineColor: "var(--ck-accent)" }}>
        <KeyRound className="h-4 w-4" aria-hidden />{going ? t("ssoRedirecting") : label}
      </a>
    </>
  );
}

/** Six single-digit boxes with auto-advance, backspace-back and paste support. */
export function CodeInput({ value, onChange, onComplete, disabled, autoFocus }: {
  value: string; onChange: (v: string) => void; onComplete?: (v: string) => void; disabled?: boolean; autoFocus?: boolean;
}) {
  const refs = useRef<(HTMLInputElement | null)[]>([]);
  const digits = Array.from({ length: 6 }, (_, i) => value[i] ?? "");
  useEffect(() => { if (autoFocus) refs.current[0]?.focus(); }, [autoFocus]);
  const set = (next: string) => {
    const v = next.replace(/\D/g, "").slice(0, 6);
    onChange(v);
    if (v.length === 6) onComplete?.(v);
    return v;
  };
  return (
    <div className="flex justify-between gap-2" role="group" aria-label="verification code">
      {digits.map((d, i) => (
        <input key={i} ref={(el) => { refs.current[i] = el; }} value={d} inputMode="numeric" autoComplete={i === 0 ? "one-time-code" : "off"} maxLength={1}
          disabled={disabled} aria-label={`${i + 1}/6`}
          onChange={(e) => {
            const ch = e.target.value.replace(/\D/g, "").slice(-1);
            const arr = [...digits]; arr[i] = ch;
            const v = set(arr.join(""));
            if (ch && i < 5) refs.current[Math.min(i + 1, v.length)]?.focus();
          }}
          onKeyDown={(e) => {
            if (e.key === "Backspace" && !digits[i] && i > 0) { refs.current[i - 1]?.focus(); }
            if (e.key === "ArrowLeft" && i > 0) refs.current[i - 1]?.focus();
            if (e.key === "ArrowRight" && i < 5) refs.current[i + 1]?.focus();
          }}
          onPaste={(e) => { e.preventDefault(); const v = set(e.clipboardData.getData("text")); refs.current[Math.min(v.length, 5)]?.focus(); }}
          onFocus={(e) => e.target.select()}
          className={cn("num h-12 w-full min-w-0 rounded-lg border border-line-strong bg-surface text-center text-lg font-semibold text-fg shadow-card transition",
            "focus:border-brand focus:outline-none focus:ring-2 focus:ring-brand/30 disabled:opacity-50", d && "border-brand/50")} />
      ))}
    </div>
  );
}

function MfaStep({ token, onDone, onBack }: { token: string; onDone: (u: User) => void; onBack: () => void }) {
  const { t } = useI18n();
  const [code, setCode] = useState("");
  const [recovery, setRecovery] = useState(false);
  const [rec, setRec] = useState("");
  const { busy, error, setError, run } = useSubmit();

  const verify = async (c: string) => {
    const u = await run(() => api.post<User>("/api/auth/login/mfa", { mfa_token: token, code: c }), { deployed: false, silentError: true });
    if (u) onDone(u); else setCode("");
  };
  return (
    <>
      <button type="button" onClick={onBack} className="mb-5 inline-flex items-center gap-1.5 text-sm text-fg-subtle hover:text-fg"><ArrowLeft className="h-3.5 w-3.5" />{t("back")}</button>
      <Heading title={t("mfaTitle")} sub={recovery ? t("mfaRecoverySub") : t("mfaSub")} />
      <div className="space-y-4">
        <ErrorBox error={error} />
        {recovery ? (
          <form onSubmit={(e) => { e.preventDefault(); verify(rec.trim()); }} className="space-y-4">
            <Field label={t("recoveryCode")} htmlFor="rec"><Input id="rec" autoFocus className="font-mono" autoComplete="off" value={rec} onChange={(e) => setRec(e.target.value)} placeholder="xxxx-xxxx-xxxx" /></Field>
            <Button type="submit" variant="primary" size="lg" className="w-full justify-center" loading={busy} disabled={!rec.trim()}>{t("verify")}</Button>
          </form>
        ) : (
          <>
            <CodeInput value={code} onChange={(v) => { setCode(v); if (error) setError(null); }} onComplete={verify} disabled={busy} autoFocus />
            <Button variant="primary" size="lg" className="w-full justify-center" loading={busy} disabled={code.length !== 6} onClick={() => verify(code)}>{t("verify")}</Button>
          </>
        )}
        <button type="button" onClick={() => { setRecovery(!recovery); setError(null); }}
          className="mx-auto flex items-center gap-1.5 text-sm text-fg-muted hover:text-fg"><KeyRound className="h-3.5 w-3.5" />{recovery ? t("useAuthenticator") : t("useRecovery")}</button>
      </div>
    </>
  );
}

export function SetupPage({ onDone }: { onDone: (u: User) => void }) {
  const { t } = useI18n();
  const [f, setF] = useState({ token: "", email: "", name: "", password: "" });
  const { busy, error, run } = useSubmit();
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const u = await run(() => api.post<User>("/api/setup", f), { deployed: false, silentError: true });
    if (u) onDone(u);
  };
  const general = !fieldErr(error, "email", "password", "token", "name");
  return (
    <AuthLayout>
      <Heading title={t("setupTitle")} sub={t("setupLead")} />
      <form onSubmit={submit} className="space-y-4">
        {general && <ErrorBox error={error} />}
        <Field label={t("setupToken")} error={fieldErr(error, "token")} htmlFor="token" hint={t("setupTokenHint")}>
          <Input id="token" required autoComplete="off" className="font-mono" value={f.token} onChange={(e) => setF({ ...f, token: e.target.value })} />
        </Field>
        <Field label={t("email")} error={fieldErr(error, "email")} htmlFor="s-email"><Input id="s-email" type="email" required autoComplete="username" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} /></Field>
        <Field label={t("name")} error={fieldErr(error, "name")} htmlFor="s-name"><Input id="s-name" value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} /></Field>
        <Field label={t("password")} error={fieldErr(error, "password")} hint={t("passwordHint")} htmlFor="s-pw">
          <Input id="s-pw" type="password" required minLength={10} autoComplete="new-password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} />
        </Field>
        <Button type="submit" variant="primary" size="lg" className="w-full justify-center" loading={busy}>{t("createAdmin")}</Button>
      </form>
    </AuthLayout>
  );
}
