import type { AlertActive } from "../api";
import { en } from "../locales/en";
import type { Key, TFn } from "../i18n";

/**
 * Title of a firing alert in the console language. The server's `title` is in the
 * notification language (alert settings), so it is only the fallback for a rule
 * the console has no title for.
 */
export function alertTitle(t: TFn, a: Pick<AlertActive, "rule" | "subject" | "title">): string {
  const k = `alt_${a.rule}`;
  return k in en && a.subject !== undefined ? t(k as Key, { s: a.subject }) : a.title;
}
