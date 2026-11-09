import { localeTag, numberLocale, type Lang } from "../i18n";
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export const cn = (...v: ClassValue[]) => twMerge(clsx(v));

export function toMs(t: number | string): number {
  if (typeof t === "number") return t > 1e12 ? t : t * 1000;
  const n = Number(t);
  if (!Number.isNaN(n)) return n > 1e12 ? n : n * 1000;
  return Date.parse(t);
}

export function fmtNum(n: number | null | undefined, digits = 0): string {
  if (n == null || Number.isNaN(n)) return "–";
  return n.toLocaleString(numberLocale(), { maximumFractionDigits: digits });
}

export function fmtCompact(n: number | null | undefined): string {
  if (n == null || Number.isNaN(n)) return "–";
  const a = Math.abs(n);
  if (a >= 1e9) return (n / 1e9).toFixed(1).replace(/\.0$/, "") + "B";
  if (a >= 1e6) return (n / 1e6).toFixed(1).replace(/\.0$/, "") + "M";
  if (a >= 1e4) return (n / 1e3).toFixed(1).replace(/\.0$/, "") + "K";
  return fmtNum(n, a < 10 && n % 1 ? 1 : 0);
}

export function fmtBytes(n: number | null | undefined): string {
  if (n == null || Number.isNaN(n)) return "–";
  const u = ["B", "KB", "MB", "GB", "TB"];
  let i = 0; let v = n;
  while (Math.abs(v) >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(i === 0 || v >= 100 ? 0 : 1)} ${u[i]}`;
}

export function fmtMs(n: number | null | undefined): string {
  if (n == null || Number.isNaN(n)) return "–";
  if (n >= 1000) return (n / 1000).toFixed(2) + " s";
  return Math.round(n) + " ms";
}

export function fmtPct(n: number | null | undefined, digits = 2): string {
  if (n == null || Number.isNaN(n)) return "–";
  // error_rate may be a ratio (0..1) or a percent; ratios are always <= 1.
  return n.toFixed(digits) + "%";
}

export function fmtUptime(s: number): string {
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
  if (d) return `${d}d ${h}h`;
  if (h) return `${h}h ${m}m`;
  return `${m}m ${Math.floor(s % 60)}s`;
}

export function relTime(iso: string | undefined, lang: Lang): string {
  if (!iso) return "–";
  const diff = (Date.now() - Date.parse(iso)) / 1000;
  const rtf = new Intl.RelativeTimeFormat(localeTag(lang), { numeric: "auto" });
  const abs = Math.abs(diff);
  const sign = diff > 0 ? -1 : 1;
  if (abs < 45) return rtf.format(0, "second");
  if (abs < 3600) return rtf.format(sign * Math.round(abs / 60), "minute");
  if (abs < 86400) return rtf.format(sign * Math.round(abs / 3600), "hour");
  if (abs < 86400 * 30) return rtf.format(sign * Math.round(abs / 86400), "day");
  return new Date(iso).toLocaleDateString(localeTag(lang));
}

export function fmtDateTime(iso: string | number | undefined, lang: Lang): string {
  if (iso == null || iso === "") return "–";
  const d = typeof iso === "number" ? new Date(toMs(iso)) : new Date(iso);
  if (Number.isNaN(d.getTime())) return "–";
  return d.toLocaleString(localeTag(lang), { dateStyle: "medium", timeStyle: "medium" });
}

export function daysUntil(iso: string | undefined): number | null {
  if (!iso) return null;
  const t = Date.parse(iso);
  return Number.isNaN(t) ? null : Math.floor((t - Date.now()) / 86400000);
}

export function download(name: string, content: string | Blob, type = "text/plain") {
  const blob = content instanceof Blob ? content : new Blob([content], { type });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url; a.download = name; a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export async function copyText(s: string): Promise<boolean> {
  try { await navigator.clipboard.writeText(s); return true; } catch {
    try {
      const ta = document.createElement("textarea");
      ta.value = s; ta.style.position = "fixed"; ta.style.opacity = "0";
      document.body.appendChild(ta); ta.select();
      const ok = document.execCommand("copy"); ta.remove(); return ok;
    } catch { return false; }
  }
}

/** HH:MM(:SS) in 24h, independent of locale quirks (ko-KR renders "13시 3분"). */
export function fmtClock(ms: number, seconds = true): string {
  const d = new Date(ms);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}${seconds ? ":" + p(d.getSeconds()) : ""}`;
}
