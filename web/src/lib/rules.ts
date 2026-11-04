// Security-rule metadata, validation and human/expression rendering.
import type { IpList, RuleCondition, RuleField, RuleOperator, SecurityRule } from "../api";
import { localeTag, type Lang } from "../i18n";
import { zhRuleLabels } from "../locales/zh";
import { jaRuleLabels } from "../locales/ja";
import { ruRuleLabels } from "../locales/ru";
import { frRuleLabels } from "../locales/fr";
import { deRuleLabels } from "../locales/de";
import { esRuleLabels } from "../locales/es";

export const LIMITS = { rules: 200, groups: 10, conds: 10, valueLen: 1024, values: 1000, regex: 512, listItems: 50000, uaItems: 10000, uaEntryMin: 3, uaEntryMax: 256 } as const;

export type FieldKind = "ip" | "country" | "method" | "scheme" | "string" | "header" | "ualist";

export const FIELDS: { id: RuleField; kind: FieldKind; exists?: boolean }[] = [
  { id: "ip.src", kind: "ip" },
  { id: "ip.country", kind: "country" },
  { id: "http.host", kind: "string" },
  { id: "http.path", kind: "string" },
  { id: "http.query", kind: "string" },
  { id: "http.method", kind: "method" },
  { id: "http.user_agent", kind: "string", exists: true },
  { id: "http.user_agent_list", kind: "ualist" },
  { id: "http.referer", kind: "string", exists: true },
  { id: "http.header", kind: "header", exists: true },
  { id: "http.scheme", kind: "scheme" },
  { id: "http.body", kind: "string" },
];
export const fieldMeta = (f: RuleField) => FIELDS.find((x) => x.id === f) ?? FIELDS[3];

const SET_OPS: RuleOperator[] = ["in", "not_in"];
const STRING_OPS: RuleOperator[] = ["eq", "ne", "contains", "not_contains", "starts_with", "ends_with", "matches", "not_matches", "in", "not_in"];

export function operatorsFor(f: RuleField): RuleOperator[] {
  const m = fieldMeta(f);
  if (m.kind === "ip" || m.kind === "country" || m.kind === "method" || m.kind === "scheme" || m.kind === "ualist") return SET_OPS;
  if (f === "http.body") return ["contains", "not_contains", "matches", "not_matches"];
  return m.exists ? [...STRING_OPS, "exists", "not_exists"] : STRING_OPS;
}
export const isSetOp = (o: RuleOperator) => o === "in" || o === "not_in";
export const isExistsOp = (o: RuleOperator) => o === "exists" || o === "not_exists";
export const isRegexOp = (o: RuleOperator) => o === "matches" || o === "not_matches";
/** String operators (ci applies). */
export const isStringField = (f: RuleField) => { const k = fieldMeta(f).kind; return k === "string" || k === "header"; };

const fieldLabels: Partial<Record<Lang, Record<RuleField, string>>> = {
  ko: {
    "ip.src": "IP 주소", "ip.country": "국가", "http.host": "호스트", "http.path": "경로", "http.query": "쿼리 문자열",
    "http.method": "메서드", "http.user_agent": "User-Agent", "http.user_agent_list": "User-Agent 목록", "http.referer": "Referer", "http.header": "요청 헤더", "http.body": "요청 본문", "http.scheme": "스킴",
  },
  en: {
    "ip.src": "IP address", "ip.country": "Country", "http.host": "Host", "http.path": "Path", "http.query": "Query string",
    "http.method": "Method", "http.user_agent": "User-Agent", "http.user_agent_list": "User-Agent list", "http.referer": "Referer", "http.header": "Request header", "http.body": "Request body", "http.scheme": "Scheme",
  },
};

const opLabels: Partial<Record<Lang, Record<RuleOperator, string>>> = {
  ko: {
    in: "다음 중 하나", not_in: "다음 중 하나가 아님", eq: "같음", ne: "같지 않음", contains: "포함", not_contains: "포함하지 않음",
    starts_with: "다음으로 시작", ends_with: "다음으로 끝남", matches: "정규식 일치", not_matches: "정규식 불일치", exists: "있음", not_exists: "없음",
  },
  en: {
    in: "is in", not_in: "is not in", eq: "equals", ne: "does not equal", contains: "contains", not_contains: "does not contain",
    starts_with: "starts with", ends_with: "ends with", matches: "matches regex", not_matches: "does not match regex", exists: "exists", not_exists: "does not exist",
  },
};

/** Short field/operator labels for the compact table summary. */
const shortFields: Partial<Record<Lang, Partial<Record<RuleField, string>>>> = {
  ko: { "ip.src": "IP", "http.query": "쿼리", "http.user_agent": "UA", "http.user_agent_list": "UA", "http.header": "헤더" },
  en: { "ip.src": "IP", "http.query": "Query", "http.user_agent": "UA", "http.user_agent_list": "UA", "http.header": "Header" },
};
const shortOps: Partial<Record<Lang, Record<RuleOperator, string>>> = {
  ko: { in: "∈", not_in: "∉", eq: "=", ne: "≠", contains: "포함", not_contains: "미포함", starts_with: "시작", ends_with: "끝", matches: "~", not_matches: "!~", exists: "있음", not_exists: "없음" },
  en: { in: "∈", not_in: "∉", eq: "=", ne: "≠", contains: "contains", not_contains: "lacks", starts_with: "starts", ends_with: "ends", matches: "~", not_matches: "!~", exists: "exists", not_exists: "missing" },
};

/** Labels for languages other than ko/en come from the locale files (RuleLabels). */
export interface RuleLabels {
  field: Record<RuleField, string>;
  op: Record<RuleOperator, string>;
  shortField?: Partial<Record<RuleField, string>>;
  shortOp?: Record<RuleOperator, string>;
  or: string;
  unknownCountry: string;
}
const extra: Partial<Record<Lang, RuleLabels>> = { zh: zhRuleLabels, ja: jaRuleLabels, ru: ruRuleLabels, fr: frRuleLabels, de: deRuleLabels, es: esRuleLabels };
const pick = <T,>(m: Partial<Record<Lang, T>>, lang: Lang): T => (m[lang] ?? m.en) as T;
export const fieldLabelFor = (lang: Lang): Record<RuleField, string> => extra[lang]?.field ?? pick(fieldLabels, lang);
export const opLabelFor = (lang: Lang): Record<RuleOperator, string> => extra[lang]?.op ?? pick(opLabels, lang);
const shortFieldFor = (lang: Lang, f: RuleField): string =>
  (extra[lang] ? extra[lang]!.shortField?.[f] : pick(shortFields, lang)[f]) ?? fieldLabelFor(lang)[f];
const shortOpFor = (lang: Lang): Record<RuleOperator, string> => extra[lang]?.shortOp ?? pick(shortOps, lang);
const orWord = (lang: Lang) => extra[lang]?.or ?? (lang === "ko" ? " 또는 " : " or ");

export const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "CONNECT", "TRACE"];
export const SCHEMES = ["http", "https"];
export const STATUS_CODES = [403, 401, 404, 410, 429, 444] as const;

// ISO 3166-1 alpha-2 (plus XX = undetermined).
export const COUNTRIES = (
  "KR US JP CN TW HK SG VN TH PH ID MY IN RU DE GB FR NL IT ES CA AU BR MX UA PL TR IR KP " +
  "AD AE AF AG AI AL AM AO AQ AR AS AT AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BS BT BV BW BY BZ " +
  "CC CD CF CG CH CI CK CL CM CO CR CU CV CW CX CY CZ DJ DK DM DO DZ EC EE EG EH ER ET FI FJ FK FM FO GA GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY " +
  "HM HN HR HT HU IE IL IM IO IQ IS JE JM JO KE KG KH KI KM KN KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MZ " +
  "NA NC NE NF NG NI NO NP NR NU NZ OM PA PE PF PG PK PM PN PR PS PT PW PY QA RE RO RS RW SA SB SC SD SE SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ " +
  "TC TD TF TG TJ TK TL TM TN TO TT TV TZ UG UM UY UZ VA VC VE VG VI VU WF WS XK YE YT ZA ZM ZW XX"
).split(" ");

const dn: Partial<Record<Lang, Intl.DisplayNames | null>> = {};
export function countryName(code: string, lang: Lang): string {
  if (code === "XX") return extra[lang]?.unknownCountry ?? (lang === "ko" ? "판별 불가" : "Unknown");
  if (dn[lang] === undefined) {
    try { dn[lang] = new Intl.DisplayNames([localeTag(lang)], { type: "region" }); } catch { dn[lang] = null; }
  }
  try { return dn[lang]?.of(code) ?? code; } catch { return code; }
}
export function flag(code?: string): string {
  if (!code || !/^[A-Z]{2}$/.test(code) || code === "XX") return "";
  return String.fromCodePoint(...[...code].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
}

// ---- IP validation -------------------------------------------------------
const v4 = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/;
function isV6(raw: string) {
  if (!/^[0-9a-fA-F:.]+$/.test(raw) || !raw.includes(":")) return false;
  let s = raw;
  const lc = s.lastIndexOf(":");
  const last = s.slice(lc + 1);
  if (last.includes(".")) { if (!v4.test(last)) return false; s = s.slice(0, lc + 1) + "0:0"; }
  const halves = s.split("::");
  if (halves.length > 2) return false;
  const parts = halves.flatMap((h) => (h ? h.split(":") : []));
  if (!parts.every((p) => /^[0-9a-fA-F]{1,4}$/.test(p))) return false;
  return halves.length === 2 ? parts.length < 8 : parts.length === 8;
}
export function isIpOrCidr(raw: string): boolean {
  const s = raw.trim();
  const [ip, bits, extra] = s.split("/");
  if (extra !== undefined || !ip) return false;
  const six = ip.includes(":");
  if (six ? !isV6(ip) : !v4.test(ip)) return false;
  if (bits === undefined) return true;
  if (!/^\d{1,3}$/.test(bits)) return false;
  return Number(bits) <= (six ? 128 : 32);
}
export const isListRef = (v: string) => /^list:\d+$/.test(v);
export const listKind = (l: IpList) => l.kind ?? "ip";
/** User-Agent list entry: plain substring (no patterns), 3-256 bytes, no control characters. */
export const isUaEntry = (v: string) => { const n = new TextEncoder().encode(v).length; return n >= LIMITS.uaEntryMin && n <= LIMITS.uaEntryMax && !/[\u0000-\u001f\u007f]/.test(v); };

// ---- editor model --------------------------------------------------------
/** Flat editor row; `or` = this row starts a new OR group (ignored for the first row). */
export interface CondRow { key: number; c: RuleCondition; or: boolean }
let seq = 0;
export const newRow = (c?: Partial<RuleCondition>, or = false): CondRow => ({
  key: ++seq, or, c: { field: "ip.src", operator: "in", value: [], ...c },
});
export function toRows(groups: RuleCondition[][]): CondRow[] {
  const out: CondRow[] = [];
  groups.forEach((g, gi) => g.forEach((c, ci) => out.push({ key: ++seq, or: gi > 0 && ci === 0, c: { ...c, value: [...(c.value ?? [])] } })));
  return out.length ? out : [newRow()];
}
export function toGroups(rows: CondRow[]): RuleCondition[][] {
  const groups: RuleCondition[][] = [];
  rows.forEach((r, i) => {
    if (i === 0 || r.or) groups.push([]);
    groups[groups.length - 1].push(cleanCond(r.c));
  });
  return groups;
}
function cleanCond(c: RuleCondition): RuleCondition {
  const out: RuleCondition = { field: c.field, operator: c.operator, value: isExistsOp(c.operator) ? [] : c.value.map((v) => v.trim()).filter((v) => v !== "") };
  if (c.field === "http.header") out.name = (c.name ?? "").trim();
  if (isStringField(c.field) && !isExistsOp(c.operator) && c.ci) out.ci = true;
  return out;
}
/** Adjusts operator/value when the field changes so the condition stays valid. */
export function changeField(c: RuleCondition, field: RuleField): RuleCondition {
  const ops = operatorsFor(field);
  const sameKind = fieldMeta(field).kind === fieldMeta(c.field).kind || (isStringField(field) && isStringField(c.field));
  const operator = ops.includes(c.operator) ? c.operator : ops[0];
  return { field, operator, value: sameKind ? c.value : [], ci: isStringField(field) ? c.ci : undefined, name: field === "http.header" ? c.name ?? "" : undefined };
}
export function changeOperator(c: RuleCondition, operator: RuleOperator): RuleCondition {
  let value = c.value;
  if (isExistsOp(operator)) value = [];
  else if (!isSetOp(operator) && value.length > 1) value = value.slice(0, 1);
  return { ...c, operator, value };
}

export type RuleErrors = Record<string, string>; // keys: name, description, status_code, host_ids, expression, row:<i>, rowName:<i>

export interface ValidateMsgs {
  required: string; noConditions: string; tooManyGroups: string; tooManyConds: string; valueRequired: string; oneValue: string;
  tooManyValues: string; valueTooLong: string; badIp: string; headerName: string; regexTooLong: string; badRegex: string; hostsRequired: string;
}

export function validateRule(form: { name: string; scope: string; host_ids: number[] }, rows: CondRow[], m: ValidateMsgs): RuleErrors {
  const e: RuleErrors = {};
  if (!form.name.trim()) e.name = m.required;
  if (form.scope === "hosts" && !form.host_ids.length) e.host_ids = m.hostsRequired;
  if (!rows.length) e.expression = m.noConditions;
  const groups = toGroups(rows);
  if (groups.length > LIMITS.groups) e.expression = m.tooManyGroups;
  else if (groups.some((g) => g.length > LIMITS.conds)) e.expression = m.tooManyConds;
  rows.forEach((r, i) => {
    const c = cleanCond(r.c);
    if (c.field === "http.header" && !/^[A-Za-z0-9!#$%&'*+.^_`|~-]+$/.test(c.name ?? "")) e[`rowName:${i}`] = m.headerName;
    if (isExistsOp(c.operator)) return;
    let msg: string | undefined;
    if (!c.value.length) msg = m.valueRequired;
    else if (!isSetOp(c.operator) && c.value.length !== 1) msg = m.oneValue;
    else if (c.value.length > LIMITS.values) msg = m.tooManyValues;
    else if (c.value.some((v) => v.length > LIMITS.valueLen)) msg = m.valueTooLong;
    else if (c.field === "ip.src" && c.value.some((v) => !isListRef(v) && !isIpOrCidr(v))) msg = m.badIp;
    else if (c.field === "http.user_agent_list" && c.value.some((v) => !isListRef(v))) msg = m.valueRequired;
    else if (isRegexOp(c.operator)) {
      if (c.value[0].length > LIMITS.regex) msg = m.regexTooLong;
      else { try { new RegExp(c.value[0].replace(/^\(\?i\)/, "")); } catch { msg = m.badRegex; } }
    }
    if (msg) e[`row:${i}`] = msg;
  });
  return e;
}

/** Maps server field errors (e.g. `expression.groups[1][0].value`, `expression.groups.1.0.name`) onto editor keys. */
export function mapServerErrors(fields: { field: string; message: string }[], rows: CondRow[]): RuleErrors {
  const e: RuleErrors = {};
  // index of the first row of each group
  const starts: number[] = [];
  rows.forEach((r, i) => { if (i === 0 || r.or) starts.push(i); });
  const add = (k: string, msg: string) => { e[k] = e[k] ? `${e[k]} / ${msg}` : msg; };
  for (const f of fields) {
    if (!f.field) continue;
    const m = /groups\D*(\d+)\D+(\d+)(?:\]?\.?(\w+))?/.exec(f.field);
    if (m) {
      const idx = (starts[Number(m[1])] ?? -1) + Number(m[2]);
      if (idx >= 0 && idx < rows.length) { add(m[3] === "name" ? `rowName:${idx}` : `row:${idx}`, f.message); continue; }
    }
    if (f.field.startsWith("expression")) { add("expression", f.message); continue; }
    if (f.field === "scope" || f.field.startsWith("host_ids")) { add("host_ids", f.message); continue; }
    add(f.field.split(/[.[]/)[0], f.message);
  }
  return e;
}

// ---- rendering -----------------------------------------------------------
const cfField: Record<RuleField, string> = {
  "ip.src": "ip.src", "ip.country": "ip.src.country", "http.host": "http.host", "http.path": "http.request.uri.path",
  "http.query": "http.request.uri.query", "http.method": "http.request.method", "http.user_agent": "http.user_agent", "http.user_agent_list": "http.user_agent",
  "http.referer": "http.referer", "http.header": "http.request.headers", "http.body": "http.request.body.raw", "http.scheme": "http.request.scheme",
};
const q = (s: string) => JSON.stringify(s);

function exprCond(c: RuleCondition, lists: IpList[] | null | undefined): string {
  let f = cfField[c.field];
  if (c.field === "http.header") f = `${f}[${q((c.name || "…").toLowerCase())}]`;
  if (isExistsOp(c.operator)) return c.operator === "exists" ? `exists(${f})` : `not exists(${f})`;
  if (c.field === "http.user_agent_list") {
    // any entry is a case-insensitive substring of the User-Agent
    const refs = c.value.map((v) => `$${listName(v, lists)}`).join(" ");
    const e = `any(lower(${f}) contains {${refs}})`;
    return c.operator === "not_in" ? `not ${e}` : e;
  }
  const ci = !!c.ci && isStringField(c.field);
  const fv = ci && !isRegexOp(c.operator) ? `lower(${f})` : f;
  const val = (v: string) => (c.field === "ip.src" ? (isListRef(v) ? `$${listName(v, lists)}` : v) : q(ci && !isRegexOp(c.operator) ? v.toLowerCase() : v));
  const v0 = c.value[0] ?? "";
  switch (c.operator) {
    case "in": case "not_in": {
      const body = c.value.length ? `{${c.value.map(val).join(" ")}}` : "{}";
      const one = c.field === "ip.src" && c.value.length === 1 && isListRef(v0) ? val(v0) : body;
      return c.operator === "in" ? `${fv} in ${one}` : `not ${fv} in ${one}`;
    }
    case "eq": return `${fv} eq ${val(v0)}`;
    case "ne": return `${fv} ne ${val(v0)}`;
    case "contains": return `${fv} contains ${val(v0)}`;
    case "not_contains": return `not ${fv} contains ${val(v0)}`;
    case "starts_with": return `starts_with(${fv}, ${val(v0)})`;
    case "ends_with": return `ends_with(${fv}, ${val(v0)})`;
    case "matches": return `${fv} matches ${q((ci ? "(?i)" : "") + v0)}`;
    case "not_matches": return `not ${fv} matches ${q((ci ? "(?i)" : "") + v0)}`;
    default: return f;
  }
}

/** Cloudflare-like expression text: `(a and b) or (c)`. */
export function expressionText(groups: RuleCondition[][], lists?: IpList[] | null): string {
  const gs = groups.filter((g) => g.length);
  if (!gs.length) return "";
  const parts = gs.map((g) => g.map((c) => exprCond(c, lists)).join(" and "));
  return gs.length === 1 && gs[0].length === 1 ? parts[0] : parts.map((p) => `(${p})`).join(" or ");
}

export function listName(v: string, lists: IpList[] | null | undefined): string {
  const id = Number(v.slice(5));
  return lists?.find((l) => l.id === id)?.name ?? `#${id}`;
}

function summaryVal(c: RuleCondition, lists: IpList[] | null | undefined): string {
  const show = c.value.slice(0, 2).map((v) => ((c.field === "ip.src" || c.field === "http.user_agent_list") && isListRef(v) ? listName(v, lists) : v));
  return show.join(", ") + (c.value.length > 2 ? ` +${c.value.length - 2}` : "");
}

/** Compact human text, e.g. `IP ∈ 사무실 · 경로 시작 /admin`. */
export function summarize(groups: RuleCondition[][], lang: Lang, lists?: IpList[] | null): string {
  const or = orWord(lang);
  return groups.filter((g) => g.length).map((g) => g.map((c) => {
    const fl = c.field === "http.header" ? `${shortFieldFor(lang, c.field)}(${c.name || "?"})` : shortFieldFor(lang, c.field);
    const op = shortOpFor(lang)[c.operator];
    return isExistsOp(c.operator) ? `${fl} ${op}` : `${fl} ${op} ${summaryVal(c, lists)}`;
  }).join(" · ")).join(or);
}

export const usesCountry = (r: SecurityRule) => r.expression?.groups?.some((g) => g.some((c) => c.field === "ip.country"));
