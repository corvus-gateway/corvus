import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { en } from "./locales/en";
import type { ko } from "./locales/ko";
import type { RuleLabels } from "./lib/rules";
import type { AuditLabels } from "./lib/audit";

/** Supported console languages. English is the default and the fallback for missing keys. */
export const LANGS = ["en", "ko", "zh", "ja", "ru", "fr", "de", "es"] as const;
export type Lang = (typeof LANGS)[number];
export type Key = keyof typeof ko;

/** Native language names for pickers. */
export const LANG_NAMES: Record<Lang, string> = {
  en: "English", ko: "한국어", zh: "简体中文", ja: "日本語", ru: "Русский", fr: "Français", de: "Deutsch", es: "Español",
};

/** BCP 47 tags for Intl (dates, numbers, region names). */
export const LOCALE_TAG: Record<Lang, string> = {
  en: "en-US", ko: "ko-KR", zh: "zh-CN", ja: "ja-JP", ru: "ru-RU", fr: "fr-FR", de: "de-DE", es: "es-ES",
};
export const localeTag = (l: Lang) => LOCALE_TAG[l] ?? "en-US";

// Number formatting follows the console language (set by I18nProvider on render, so
// plain helpers such as fmtNum need no language argument).
let numLocale = "en-US";
export const numberLocale = () => numLocale;

/** A console language: its strings, plus rule and audit labels for languages other than ko/en. */
export interface Locale { dict: Partial<Record<Key, string>>; rules?: RuleLabels; audit?: AuditLabels }

// English is built in (it is the fallback); the other languages are separate chunks loaded
// when chosen, so the first page load does not download every translation.
const loaders: Record<Exclude<Lang, "en">, () => Promise<Locale>> = {
  ko: () => import("./locales/ko").then((m) => ({ dict: m.ko })),
  zh: () => import("./locales/zh").then((m) => ({ dict: m.zh, rules: m.zhRuleLabels, audit: m.zhAuditLabels })),
  ja: () => import("./locales/ja").then((m) => ({ dict: m.ja, rules: m.jaRuleLabels, audit: m.jaAuditLabels })),
  ru: () => import("./locales/ru").then((m) => ({ dict: m.ru, rules: m.ruRuleLabels, audit: m.ruAuditLabels })),
  fr: () => import("./locales/fr").then((m) => ({ dict: m.fr, rules: m.frRuleLabels, audit: m.frAuditLabels })),
  de: () => import("./locales/de").then((m) => ({ dict: m.de, rules: m.deRuleLabels, audit: m.deAuditLabels })),
  es: () => import("./locales/es").then((m) => ({ dict: m.es, rules: m.esRuleLabels, audit: m.esAuditLabels })),
};
const loaded: Partial<Record<Lang, Locale>> = { en: { dict: en } };

/** The loaded locale of a language (undefined until loadLang resolved). */
export const localeOf = (l: Lang): Locale | undefined => loaded[l];

/** Loads a language's strings; resolves to false when the chunk could not be loaded. */
export async function loadLang(l: Lang): Promise<boolean> {
  if (loaded[l]) return true;
  try {
    loaded[l] = await loaders[l as Exclude<Lang, "en">]();
    return true;
  } catch {
    return false;
  }
}

export const isLang = (v: unknown): v is Lang => typeof v === "string" && (LANGS as readonly string[]).includes(v);

export function initialLang(): Lang {
  try {
    const saved = localStorage.getItem("corvus.lang");
    if (isLang(saved)) return saved;
  } catch { /* storage unavailable */ }
  return "en";
}

export type TFn = (k: Key, vars?: Record<string, string | number>) => string;
const Ctx = createContext<{ lang: Lang; setLang: (l: Lang) => void; t: TFn }>(null!);

export function I18nProvider({ children }: { children: ReactNode }) {
  // main.tsx loads the saved language before the first render; English if that failed.
  const [lang, setLangState] = useState<Lang>(() => { const l = initialLang(); return loaded[l] ? l : "en"; });
  numLocale = localeTag(lang);
  useEffect(() => { document.documentElement.lang = lang; }, [lang]);
  const setLang = useCallback((l: Lang) => {
    try { localStorage.setItem("corvus.lang", l); } catch { /* ignore */ }
    void loadLang(l).then((ok) => ok && setLangState(l));
  }, []);
  const t = useCallback<TFn>((k, vars) => {
    const s = loaded[lang]?.dict[k] ?? en[k] ?? k;
    return vars ? s.replace(/\{(\w+)\}/g, (_, n) => String(vars[n] ?? "")) : s;
  }, [lang]);
  const value = useMemo(() => ({ lang, setLang, t }), [lang, setLang, t]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export const useI18n = () => useContext(Ctx);
