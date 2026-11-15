// Shared by the proxy host and redirection host lists: certificate lookup with polling
// while one is being issued, the SSL badge, and search / sort / paging of long lists.
import { useEffect, useMemo, useState } from "react";
import { AlertCircle, ArrowDown, ArrowUp, ArrowUpDown, Loader2, Lock } from "lucide-react";
import type { Certificate } from "../api";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { Badge } from "./ui/display";
import { Tip, tipTrigger } from "./ui/overlay";
import { Th } from "./ui/table";

/** Certificates by id; polls every 3 s while one is pending so its status updates. */
export function useCertificates() {
  const [poll, setPoll] = useState(0);
  const certs = useResource<Certificate[]>("/api/certificates", { poll });
  useEffect(() => { setPoll(certs.data?.some((c) => c.status === "pending") ? 3000 : 0); }, [certs.data]);
  const byId = useMemo(() => new Map((certs.data ?? []).map((c) => [c.id, c])), [certs.data]);
  return { certs, byId };
}

export function SslBadge({ cert }: { cert?: Certificate }) {
  const { t } = useI18n();
  if (!cert) return <span className="text-xs text-fg-subtle">{t("sslNone")}</span>;
  const label = cert.kind === "acme" ? t("sslLetsEncrypt") : t("sslCustom");
  if (cert.status === "pending") return <Badge tone="info" icon={Loader2} className="[&_svg]:animate-spin">{label} · {t("certStatusPending")}</Badge>;
  if (cert.status === "error") {
    const why = cert.last_error || t("certError");
    return <Tip label={why}><span tabIndex={0} aria-label={`${label} · ${t("certError")}: ${why}`} className={tipTrigger}><Badge tone="danger" icon={AlertCircle}>{label} · {t("certError")}</Badge></span></Tip>;
  }
  return <Badge tone="ok" icon={Lock}>{label}</Badge>;
}

/** "+2" after the first domain: the others in a tooltip, also reachable by keyboard. */
export function MoreDomains({ domains }: { domains: string[] }) {
  const { t } = useI18n();
  if (domains.length < 2) return null;
  const rest = domains.slice(1);
  return (
    <Tip label={rest.join(", ")}>
      <span tabIndex={0} aria-label={`${t("moreDomains", { n: rest.length })}: ${rest.join(", ")}`} className={tipTrigger}>
        <Badge>+{rest.length}</Badge>
      </span>
    </Tip>
  );
}

export type HostSort = { key: "domain" | "updated"; dir: 1 | -1 };
const PAGE_FROM = 100; // lists longer than this are paged

/** Case-insensitive search (the caller's text per host), sorting and client-side paging. */
export function useHostList<T extends { domains: string[]; updated_at: string }>(items: T[] | null, q: string, text: (h: T) => string) {
  const [sort, setSort] = useState<HostSort | null>(null);
  const [limit, setLimit] = useState(PAGE_FROM);
  const [offset, setOffset] = useState(0);
  const s = q.trim().toLowerCase();
  const rows = useMemo(() => {
    const list = s ? (items ?? []).filter((h) => text(h).toLowerCase().includes(s)) : [...(items ?? [])];
    if (sort) {
      const k = sort.key === "domain" ? (h: T) => h.domains[0] ?? "" : (h: T) => h.updated_at ?? "";
      list.sort((a, b) => (sort.key === "domain" ? k(a).localeCompare(k(b)) : k(a) < k(b) ? -1 : k(a) > k(b) ? 1 : 0) * sort.dir);
    }
    return list;
  }, [items, s, sort]);
  useEffect(() => { setOffset(0); }, [s, sort]);
  const paged = rows.length > PAGE_FROM;
  const off = paged && offset >= rows.length ? 0 : offset;
  const toggle = (key: HostSort["key"]) =>
    setSort((cur) => (cur?.key !== key ? { key, dir: key === "updated" ? -1 : 1 } : cur.dir === (key === "updated" ? -1 : 1) ? { key, dir: cur.dir === 1 ? -1 : 1 } : null));
  return {
    rows, shown: paged ? rows.slice(off, off + limit) : rows, sort, toggle,
    pager: paged ? { total: rows.length, limit, offset: off, onOffset: setOffset, onLimit: setLimit, sizes: [50, 100, 200] as const } : null,
  };
}

/** Column header that sorts the list (ascending → descending → original order). */
export function SortTh({ label, k, sort, onSort, className }: { label: string; k: HostSort["key"]; sort: HostSort | null; onSort: (k: HostSort["key"]) => void; className?: string }) {
  const { t } = useI18n();
  const on = sort?.key === k;
  const Icon = !on ? ArrowUpDown : sort!.dir === 1 ? ArrowUp : ArrowDown;
  return (
    <Th className={className} aria-sort={on ? (sort!.dir === 1 ? "ascending" : "descending") : "none"}>
      <button type="button" onClick={() => onSort(k)} title={t("sortBy", { col: label })}
        className="-mx-1 inline-flex items-center gap-1 rounded px-1 hover:bg-surface-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand">
        {label}<Icon className={on ? "h-3.5 w-3.5 text-fg" : "h-3.5 w-3.5 text-fg-subtle"} aria-hidden />
      </button>
    </Th>
  );
}
