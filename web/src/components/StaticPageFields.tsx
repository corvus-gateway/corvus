import { useEffect, useState } from "react";
import { api, type PageTemplate, type ProxyHost, type StaticPage } from "../api";
import { useI18n, type Key } from "../i18n";
import { IdField, Input, Select, Textarea, fieldErr } from "./ui/form";

/** Custom HTML limit (validate.MaxPageHTMLBytes). */
const MAX_HTML = 256 * 1024;
export const PAGE_STATUSES = [200, 404, 410, 503] as const;

export const PAGE_TEMPLATES: { value: PageTemplate; label: Key }[] = [
  { value: "coming_soon", label: "spTplComingSoon" },
  { value: "maintenance", label: "spTplMaintenance" },
  { value: "parked", label: "spTplParked" },
  { value: "custom", label: "spTplCustom" },
];

export const blankPage: StaticPage = { template: "coming_soon", status: 200, title: "", message: "", html: "" };

/** The page of a host form: the stored one, or the default for a host that never had one. */
export const pageOf = (p?: StaticPage): StaticPage => (p && p.template ? { ...blankPage, ...p } : { ...blankPage });

/** "Maintenance (503)" for the host list. */
export function pageLabel(t: (k: Key) => string, p?: StaticPage): string {
  const tpl = PAGE_TEMPLATES.find((x) => x.value === p?.template);
  return `${t(tpl?.label ?? "spTplComingSoon")} (${p?.status || 200})`;
}

/** Text of a host the list search matches: domains, forward target and backends,
 *  custom locations (path and forward host), the page template and notes. */
export function hostSearchText(h: ProxyHost, t: (k: Key) => string): string {
  return [
    h.domains.join(" "),
    h.content === "page" ? pageLabel(t, h.page) : `${h.forward_scheme}://${h.forward_host}:${h.forward_port}`,
    (h.extra_targets ?? []).map((x) => `${x.forward_host}:${x.forward_port}`).join(" "),
    (h.locations ?? []).map((l) => `${l.path} ${l.forward_scheme}://${l.forward_host}:${l.forward_port}`).join(" "),
    h.notes ?? "",
  ].join(" ");
}

/** "Serve a page": template, status, texts or HTML, and a preview from the control plane. */
export function StaticPageFields({ value, onChange, error, host }: { value: StaticPage; onChange: (p: StaticPage) => void; error: unknown; host: string }) {
  const { t } = useI18n();
  const set = (p: Partial<StaticPage>) => onChange({ ...value, ...p });
  const custom = value.template === "custom";
  const size = new Blob([value.html]).size;
  const tooLarge = size > MAX_HTML;
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-[1fr_12rem] gap-3">
        <IdField label={t("spTemplate")} error={fieldErr(error, "page.template")}>
          {(id) => (
            <Select id={id} value={value.template} onChange={(e) => {
              const tpl = e.target.value as PageTemplate;
              // maintenance answers 503 unless another status was chosen on purpose
              const status = tpl === "maintenance" && value.status === 200 ? 503 : value.template === "maintenance" && value.status === 503 ? 200 : value.status;
              set({ template: tpl, status });
            }}>
              {PAGE_TEMPLATES.map((o) => <option key={o.value} value={o.value}>{t(o.label)}</option>)}
            </Select>
          )}
        </IdField>
        <IdField label={t("spStatus")} error={fieldErr(error, "page.status")}>
          {(id) => (
            <Select id={id} value={value.status} onChange={(e) => set({ status: Number(e.target.value) })}>
              {PAGE_STATUSES.map((s) => <option key={s} value={s}>{s} {STATUS_TEXT[s]}</option>)}
            </Select>
          )}
        </IdField>
      </div>
      <p className="-mt-2 text-xs leading-5 text-fg-subtle">{t("spStatusHint")}</p>
      {custom ? (
        <IdField label={t("pageHtml")} required hint={tooLarge ? undefined : `${t("spHtmlHint")} ${t("pageBytes", { n: (size / 1024).toFixed(1), max: MAX_HTML / 1024 })}`}
          error={tooLarge ? t("pageTooLarge", { max: MAX_HTML / 1024 }) : fieldErr(error, "page.html")}>
          {(id) => <Textarea id={id} rows={12} spellCheck={false} className="font-mono text-xs leading-5" value={value.html} invalid={tooLarge || !!fieldErr(error, "page.html")}
            onChange={(e) => set({ html: e.target.value })} placeholder={"<!doctype html>\n<title>…</title>\n<h1>…</h1>"} />}
        </IdField>
      ) : (<>
        <IdField label={t("spTitle")} error={fieldErr(error, "page.title")}>
          {(id) => <Input id={id} maxLength={200} value={value.title} onChange={(e) => set({ title: e.target.value })} invalid={!!fieldErr(error, "page.title")} />}
        </IdField>
        <IdField label={t("spMessage")} hint={t("spTextHint")} error={fieldErr(error, "page.message")}>
          {(id) => <Textarea id={id} rows={3} maxLength={2000} value={value.message} onChange={(e) => set({ message: e.target.value })} invalid={!!fieldErr(error, "page.message")} />}
        </IdField>
      </>)}
      {!tooLarge && <PagePreview page={value} host={host} />}
    </div>
  );
}

const STATUS_TEXT: Record<number, string> = { 200: "OK", 404: "Not Found", 410: "Gone", 503: "Service Unavailable" };

/** Rendered by the control plane (same template as the gateway), shown in a sandboxed frame. */
function PagePreview({ page, host }: { page: StaticPage; host: string }) {
  const { t, lang } = useI18n();
  const [html, setHtml] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const key = JSON.stringify([page, host, lang]);
  useEffect(() => {
    if (page.template === "custom" && !page.html.trim()) { setHtml(""); setFailed(false); return; }
    let live = true;
    const timer = window.setTimeout(() => {
      api.post<{ html: string }>("/api/proxy-hosts/page-preview", { page, host, lang })
        .then((r) => { if (live) { setHtml(r.html); setFailed(false); } })
        .catch(() => { if (live) setFailed(true); });
    }, 300);
    return () => { live = false; window.clearTimeout(timer); };
  }, [key]);
  return (
    <div className="space-y-1.5">
      <div className="text-sm font-medium text-fg">{t("pagePreview")}</div>
      <p className="text-xs text-fg-subtle">{t("spPreviewHint")}</p>
      {failed ? <p role="alert" className="text-xs text-danger">{t("spPreviewFailed")}</p> : (
        // sandbox="": no scripts, forms, navigation or same-origin access
        <iframe title={t("pagePreview")} sandbox="" srcDoc={html ?? ""} className="h-72 w-full rounded-lg border border-line bg-white" />
      )}
    </div>
  );
}
