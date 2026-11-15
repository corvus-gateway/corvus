import { Fragment, useMemo, useState } from "react";
import { ChevronDown, ScrollText } from "lucide-react";
import type { AuditEntry } from "../api";
import { ErrorBox, PageHeader, RefreshError, Toolbar } from "../components/common";
import { Badge, Card, CodeBlock, EmptyState, TableSkeleton } from "../components/ui/display";
import { SearchInput, Select } from "../components/ui/form";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { actionLabel, actionMeta } from "../lib/audit";
import { cn, fmtDateTime, relTime } from "../lib/utils";

const LIMITS = [100, 250, 500, 1000];

export default function Audit() {
  const { t, lang } = useI18n();
  const [limit, setLimit] = useState(250);
  const [q, setQ] = useState("");
  const [cat, setCat] = useState("");
  const [open, setOpen] = useState<number | null>(null);
  const res = useResource<AuditEntry[]>(`/api/audit?limit=${limit}`, { poll: 20000 });

  const cats = useMemo(() => [...new Set((res.data ?? []).map((a) => a.action.split(".")[0]))].sort(), [res.data]);
  const rows = useMemo(() => {
    const s = q.trim().toLowerCase();
    return (res.data ?? []).filter((a) => (!cat || a.action.startsWith(cat + ".")) &&
      (!s || `${a.action} ${actionLabel(a.action, lang)} ${a.email} ${a.target} ${a.ip}`.toLowerCase().includes(s)));
  }, [res.data, q, cat, lang]);

  return (
    <>
      <PageHeader title={t("navAudit")} description={t("auditDesc")} />
      <Card>
        <Toolbar>
          <SearchInput className="w-full sm:w-72" value={q} onChange={setQ} placeholder={t("searchAudit")} />
          <div className="w-40"><Select aria-label={t("category")} value={cat} onChange={(e) => setCat(e.target.value)} className="!h-8"><option value="">{t("allCategories")}</option>{cats.map((c) => <option key={c} value={c}>{c}</option>)}</Select></div>
          <div className="w-32"><Select aria-label={t("rows")} value={limit} onChange={(e) => setLimit(Number(e.target.value))} className="!h-8">{LIMITS.map((l) => <option key={l} value={l}>{t("lastN", { n: l })}</option>)}</Select></div>
          <span className="ml-auto text-xs text-fg-subtle num">{t("nItems", { n: rows.length })}</span>
        </Toolbar>
        <RefreshError res={res} className="m-3" />
        {res.loading ? <TableSkeleton cols={5} rows={8} /> : res.error && !res.data ? <div className="p-4"><ErrorBox error={res.error} /></div> : !rows.length ? (
          <EmptyState icon={ScrollText} title={t("noAudit")} description={res.data?.length ? t("noResultsDesc") : t("noAuditDesc")} />
        ) : (
          <Table>
            <THead><tr><Th>{t("when")}</Th><Th>{t("action")}</Th><Th className="hidden md:table-cell">{t("target")}</Th><Th className="hidden sm:table-cell">{t("who")}</Th><Th className="hidden lg:table-cell">IP</Th><Th className="w-10"><span className="sr-only">{t("details")}</span></Th></tr></THead>
            <tbody>
              {rows.map((a) => {
                const m = actionMeta(a.action);
                const hasDetail = a.detail != null && !(typeof a.detail === "object" && !Object.keys(a.detail as object).length);
                const isOpen = open === a.id;
                return (
                  <Fragment key={a.id}>
                    <Tr className={cn(hasDetail && "cursor-pointer")} onClick={() => hasDetail && setOpen(isOpen ? null : a.id)}>
                      <Td className="whitespace-nowrap"><div className="text-sm">{relTime(a.ts, lang)}</div><div className="num text-xs text-fg-subtle">{fmtDateTime(a.ts, lang)}</div></Td>
                      <Td><div className="flex items-center gap-2"><m.icon className="h-3.5 w-3.5 shrink-0 text-fg-subtle" aria-hidden /><Badge tone={m.tone}>{actionLabel(a.action, lang)}</Badge></div><div className="mt-0.5 font-mono text-2xs text-fg-subtle">{a.action}</div></Td>
                      <Td className="hidden max-w-[16rem] truncate font-mono text-xs text-fg-muted md:table-cell">{a.target || "–"}</Td>
                      <Td className="hidden text-fg-muted sm:table-cell">{a.email || "–"}</Td>
                      <Td className="hidden font-mono text-xs text-fg-subtle lg:table-cell">{a.ip || "–"}</Td>
                      <Td>{hasDetail && <button type="button" aria-expanded={isOpen} aria-label={t("details")} onClick={(e) => { e.stopPropagation(); setOpen(isOpen ? null : a.id); }} className="grid h-6 w-6 place-items-center rounded text-fg-subtle hover:bg-surface-3"><ChevronDown className={cn("h-4 w-4 transition", isOpen && "rotate-180")} /></button>}</Td>
                    </Tr>
                    {isOpen && <tr className="border-b border-line bg-surface-2/40"><td colSpan={6} className="px-4 py-3"><CodeBlock code={JSON.stringify(a.detail, null, 2)} maxHeight="16rem" /></td></tr>}
                  </Fragment>
                );
              })}
            </tbody>
          </Table>
        )}
      </Card>
    </>
  );
}
