import { useSearchParams } from "react-router-dom";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Activity, AlertOctagon, Ban, Download, Gauge, Hourglass, ListFilter, Radio, ScrollText, ServerCrash, TimerReset, type LucideIcon } from "lucide-react";
import { getText, type EventKind, type HostTraffic, type LogLine, type PathTraffic, type Timeseries, type TrafficEvent, type TrafficSummary } from "../api";
import { ChartLegend, StackBar, TimeChart, type Series } from "../components/charts";
import { ErrorBox, PageHeader, Toolbar, useSubmit } from "../components/common";
import { Button } from "../components/ui/button";
import { Badge, Card, CardHeader, ComingSoon, EmptyState, Skeleton, TableSkeleton, type Tone } from "../components/ui/display";
import { SearchInput, Segmented, Select, Switch } from "../components/ui/form";
import { Table, THead, Td, Th, Tr } from "../components/ui/table";
import { useResource } from "../hooks/useResource";
import { useI18n, type Key } from "../i18n";
import { cn, download, fmtBytes, fmtClock, fmtCompact, fmtDateTime, fmtMs, fmtNum, toMs } from "../lib/utils";

const RANGES = ["5m", "15m", "1h", "6h", "24h"] as const;
type Range = (typeof RANGES)[number];
type Tab = "overview" | "logs" | "events";

const statusTone = (s: number): Tone => (s >= 500 ? "danger" : s >= 400 ? "warn" : s >= 300 ? "info" : "ok");
const methodCls = "inline-block w-12 font-mono text-2xs font-semibold text-fg-muted";

function useDebounced<T>(v: T, ms = 350): T {
  const [d, setD] = useState(v);
  useEffect(() => { const id = setTimeout(() => setD(v), ms); return () => clearTimeout(id); }, [v, ms]);
  return d;
}

function Stat({ icon: Icon, label, value, sub, tone }: { icon: LucideIcon; label: string; value: ReactNode; sub?: ReactNode; tone?: "danger" | "warn" }) {
  return (
    <Card className="p-4">
      <div className="flex items-center justify-between text-xs font-medium text-fg-subtle"><span>{label}</span><Icon className="h-3.5 w-3.5 opacity-70" aria-hidden /></div>
      <div className={cn("num mt-2 text-xl font-semibold tracking-tight", tone === "danger" && "text-danger", tone === "warn" && "text-warn")}>{value}</div>
      {sub && <div className="mt-0.5 text-xs text-fg-subtle">{sub}</div>}
    </Card>
  );
}

export default function Traffic() {
  const { t, lang } = useI18n();
  // Links (e.g. from an alert) may preselect ?tab=logs&range=1h&host=…&q=…
  const [params] = useSearchParams();
  const [range, setRange] = useState<Range>(() => (RANGES as readonly string[]).includes(params.get("range") ?? "") ? (params.get("range") as Range) : "15m");
  const [host, setHost] = useState(() => params.get("host") ?? "");
  const [tab, setTab] = useState<Tab>(() => (params.get("tab") === "logs" || params.get("tab") === "events" ? params.get("tab") as Tab : "overview"));
  const initialQ = params.get("q") ?? "";
  const [live, setLive] = useState(true);
  const poll = live ? 5000 : 0;
  const qs = `range=${range}${host ? `&host=${encodeURIComponent(host)}` : ""}`;

  const summary = useResource<TrafficSummary>(`/api/traffic/summary?${qs}`, { poll });
  const hostsRes = useResource<HostTraffic[]>(`/api/traffic/hosts?range=${range}`, { poll });

  if (summary.missing) {
    return (<><PageHeader title={t("navTraffic")} description={t("trafficDesc")} /><Card><ComingSoon icon={Activity} /></Card></>);
  }

  const hostOptions = (hostsRes.data ?? []).map((h) => h.host);
  return (
    <>
      <PageHeader title={t("navTraffic")} description={t("trafficDesc")} actions={
        <div className="flex flex-wrap items-center gap-2">
          <div className="w-44"><Select aria-label={t("host")} value={host} onChange={(e) => setHost(e.target.value)} className="!h-8"><option value="">{t("allHosts")}</option>{hostOptions.map((h) => <option key={h} value={h}>{h}</option>)}</Select></div>
          <Segmented className="!w-auto" label={t("range")} value={range} onChange={setRange} options={RANGES.map((r) => ({ value: r, label: r }))} />
          <label className="flex items-center gap-2 rounded-lg border border-line bg-surface px-2.5 py-1.5 text-xs text-fg-muted shadow-card">
            <Radio className={cn("h-3.5 w-3.5", live ? "text-ok" : "text-fg-subtle")} aria-hidden />{t("live")}
            <Switch checked={live} onCheckedChange={setLive} label={t("live")} />
          </label>
        </div>} />

      <div role="tablist" aria-label="traffic views" className="mb-4 flex gap-1 border-b border-line">
        {([["overview", t("tabOverview"), Activity], ["logs", t("tabLogs"), ScrollText], ["events", t("tabEvents"), AlertOctagon]] as const).map(([k, l, I]) => (
          <button key={k} role="tab" aria-selected={tab === k} onClick={() => setTab(k)}
            className={cn("-mb-px flex items-center gap-2 border-b-2 px-3.5 py-2.5 text-sm font-medium transition", tab === k ? "border-brand text-fg" : "border-transparent text-fg-subtle hover:text-fg")}>
            <I className="h-3.5 w-3.5" aria-hidden />{l}
          </button>
        ))}
      </div>

      {tab === "overview" && <Overview qs={qs} poll={poll} summary={summary.data} loading={summary.loading} error={summary.error} hosts={hostsRes.data} hostsLoading={hostsRes.loading} onHost={setHost} />}
      {tab === "logs" && <Logs qs={qs} poll={poll} initialQ={initialQ} />}
      {tab === "events" && <Events qs={qs} poll={poll} />}
      <span className="sr-only">{lang}</span>
    </>
  );
}

function Overview({ qs, poll, summary, loading, error, hosts, hostsLoading, onHost }: {
  qs: string; poll: number; summary: TrafficSummary | null; loading: boolean; error: unknown; hosts: HostTraffic[] | null; hostsLoading: boolean; onHost: (h: string) => void;
}) {
  const { t, lang } = useI18n();
  const ts = useResource<Timeseries>(`/api/traffic/timeseries?${qs}`, { poll });
  const paths = useResource<PathTraffic[]>(`/api/traffic/top-paths?${qs}`, { poll });
  const points = ts.data?.points ?? [];
  const step = ts.data?.step_s || 1;

  const statusSeries: Series[] = [
    { key: "s2xx", label: "2xx", color: "rgb(var(--chart-2))" }, { key: "s3xx", label: "3xx", color: "rgb(var(--chart-1))" },
    { key: "s4xx", label: "4xx", color: "rgb(var(--chart-3))" }, { key: "s5xx", label: "5xx", color: "rgb(var(--chart-4))" },
  ];
  const latSeries: Series[] = [
    { key: "p50", label: "p50", color: "rgb(var(--chart-2))" }, { key: "p95", label: "p95", color: "rgb(var(--chart-3))" }, { key: "p99", label: "p99", color: "rgb(var(--chart-4))" },
  ];
  const statusPts = useMemo(() => points.map((p) => ({ t: p.t, v: { s2xx: p.s2xx, s3xx: p.s3xx, s4xx: p.s4xx, s5xx: p.s5xx } })), [points]);
  const latPts = useMemo(() => points.map((p) => ({ t: p.t, v: { p50: p.p50, p95: p.p95, p99: p.p99 } })), [points]);
  const errPct = summary && summary.requests > 0 ? (summary.s5xx / summary.requests) * 100 : 0;

  return (
    <div className="space-y-5">
      {error && !summary ? <ErrorBox error={error} /> : null}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
        {loading || !summary ? Array.from({ length: 6 }, (_, i) => <Skeleton key={i} className="h-[88px] rounded-xl" />) : (<>
          <Stat icon={Activity} label={t("kpiRequests")} value={fmtCompact(summary.requests)} sub={`${fmtNum(summary.rps, 1)} req/s`} />
          <Stat icon={ServerCrash} label={t("kpiErrorRate")} value={`${errPct.toFixed(2)}%`} tone={errPct >= 5 ? "danger" : errPct >= 1 ? "warn" : undefined} sub={`5xx ${fmtNum(summary.s5xx)} · 4xx ${fmtNum(summary.s4xx)}`} />
          <Stat icon={TimerReset} label="p95" value={fmtMs(summary.p95)} sub={`p50 ${fmtMs(summary.p50)} · p99 ${fmtMs(summary.p99)}`} />
          <Stat icon={Ban} label={t("kpiBlocked")} value={fmtCompact(summary.blocked)} sub={t("blockedHint")} tone={summary.blocked > 0 ? "warn" : undefined} />
          <Stat icon={Hourglass} label={t("queuedRequests")} value={fmtCompact(summary.queued)} sub={t("queuedHint")} />
          <Stat icon={Gauge} label={t("egress")} value={fmtBytes(summary.bytes_out)} sub={`${t("uniqueIps")} ${fmtCompact(summary.unique_ips)}`} />
        </>)}
      </div>

      <div className="grid gap-5 xl:grid-cols-2">
        <Card>
          <CardHeader icon={Activity} title={t("requestsByStatus")} description={t("perStep", { s: step })} actions={<ChartLegend series={statusSeries} />} />
          <div className="px-4 pb-4">{ts.loading ? <Skeleton className="h-[220px]" /> : <TimeChart points={statusPts} series={statusSeries} lang={lang} format={fmtCompact} empty={t("noTrafficYet")} />}</div>
        </Card>
        <Card>
          <CardHeader icon={TimerReset} title={t("latency")} description={t("latencyDesc")} actions={<ChartLegend series={latSeries} />} />
          <div className="px-4 pb-4">{ts.loading ? <Skeleton className="h-[220px]" /> : <TimeChart mode="lines" points={latPts} series={latSeries} lang={lang} format={(n) => fmtMs(n)} empty={t("noTrafficYet")} />}</div>
        </Card>
      </div>

      <div className="grid gap-5 xl:grid-cols-5">
        <Card className="xl:col-span-3">
          <CardHeader icon={ListFilter} title={t("byHost")} />
          {hostsLoading ? <TableSkeleton cols={5} rows={4} /> : !hosts?.length ? <EmptyState icon={Activity} className="py-10" title={t("noTrafficYet")} description={t("noTrafficYetDesc")} /> : (
            <Table>
              <THead><tr><Th>{t("host")}</Th><Th className="text-right">{t("kpiRequests")}</Th><Th className="hidden text-right sm:table-cell">req/s</Th><Th className="text-right">{t("kpiErrorRate")}</Th><Th className="hidden text-right md:table-cell">p95</Th><Th className="hidden text-right lg:table-cell">{t("kpiBlocked")}</Th><Th className="hidden text-right lg:table-cell">{t("egress")}</Th></tr></THead>
              <tbody>
                {hosts.map((h) => {
                  const er = h.requests ? (h.s5xx / h.requests) * 100 : 0;
                  return (
                    <Tr key={h.host} className="cursor-pointer" onClick={() => onHost(h.host)}>
                      <Td className="font-medium">{h.host}</Td>
                      <Td className="num text-right">{fmtNum(h.requests)}</Td>
                      <Td className="num hidden text-right text-fg-muted sm:table-cell">{fmtNum(h.rps, 1)}</Td>
                      <Td className={cn("num text-right", er >= 5 ? "text-danger" : er >= 1 ? "text-warn" : "text-fg-muted")}>{er.toFixed(2)}%</Td>
                      <Td className="num hidden text-right text-fg-muted md:table-cell">{fmtMs(h.p95)}</Td>
                      <Td className="num hidden text-right text-fg-muted lg:table-cell">{fmtNum(h.blocked)}</Td>
                      <Td className="num hidden text-right text-fg-muted lg:table-cell">{fmtBytes(h.bytes_out)}</Td>
                    </Tr>
                  );
                })}
              </tbody>
            </Table>
          )}
        </Card>
        <Card className="xl:col-span-2">
          <CardHeader icon={ScrollText} title={t("topPaths")} />
          {paths.loading ? <TableSkeleton cols={3} rows={4} /> : !paths.data?.length ? <EmptyState icon={ScrollText} className="py-10" title={t("noTrafficYet")} /> : (
            <ul className="divide-y divide-line">
              {paths.data.slice(0, 10).map((p) => {
                const max = Math.max(...paths.data!.map((x) => x.requests), 1);
                return (
                  <li key={p.path} className="relative px-4 py-2.5">
                    <div aria-hidden className="absolute inset-y-0 left-0 bg-brand/[0.06]" style={{ width: `${(p.requests / max) * 100}%` }} />
                    <div className="relative flex items-center justify-between gap-3">
                      <span className="min-w-0 truncate font-mono text-xs" title={p.path}>{p.path}</span>
                      <span className="flex shrink-0 items-center gap-3 text-xs">
                        {p.errors > 0 && <span className="num text-danger">{fmtNum(p.errors)} err</span>}
                        <span className="num text-fg-subtle">{fmtMs(p.p95)}</span>
                        <span className="num w-14 text-right font-medium">{fmtCompact(p.requests)}</span>
                      </span>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </Card>
      </div>
      {summary && <StackBar parts={[
        { label: "2xx", value: summary.s2xx, color: "rgb(var(--chart-2))" }, { label: "3xx", value: summary.s3xx, color: "rgb(var(--chart-1))" },
        { label: "4xx", value: summary.s4xx, color: "rgb(var(--chart-3))" }, { label: "5xx", value: summary.s5xx, color: "rgb(var(--chart-4))" }]} />}
    </div>
  );
}

const STATUS_FILTERS = ["", "2xx", "3xx", "4xx", "5xx", "429"];

function Logs({ qs, poll, initialQ = "" }: { qs: string; poll: number; initialQ?: string }) {
  const { t, lang } = useI18n();
  const [status, setStatus] = useState("");
  const [q, setQ] = useState(initialQ);
  const [limit, setLimit] = useState(200);
  const dq = useDebounced(q);
  const query = `${qs}&limit=${limit}${status ? `&status=${status}` : ""}${dq ? `&q=${encodeURIComponent(dq)}` : ""}`;
  const res = useResource<{ items: LogLine[] }>(`/api/traffic/logs?${query}`, { poll });
  const exp = useSubmit();

  const doExport = async (format: "csv" | "json") => {
    const text = await exp.run(() => getText(`/api/traffic/logs?${query}&limit=1000&format=${format}`), { deployed: false });
    if (text != null) download(`corvus-traffic-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-")}.${format}`, text, format === "csv" ? "text/csv" : "application/json");
  };
  const items = res.data?.items ?? [];

  return (
    <Card>
      <Toolbar>
        <SearchInput className="w-full sm:w-72" value={q} onChange={setQ} placeholder={t("searchLogs")} />
        <div className="w-32"><Select aria-label={t("statusCode")} value={status} onChange={(e) => setStatus(e.target.value)} className="!h-8">{STATUS_FILTERS.map((s) => <option key={s} value={s}>{s || t("allStatus")}</option>)}</Select></div>
        <div className="w-28"><Select aria-label={t("rows")} value={limit} onChange={(e) => setLimit(Number(e.target.value))} className="!h-8">{[100, 200, 500, 1000].map((n) => <option key={n} value={n}>{t("lastN", { n })}</option>)}</Select></div>
        <div className="ml-auto flex items-center gap-2">
          <span className="text-xs text-fg-subtle num">{t("nItems", { n: items.length })}</span>
          <Button size="sm" loading={exp.busy} onClick={() => doExport("csv")}><Download className="h-3.5 w-3.5" />CSV</Button>
          <Button size="sm" loading={exp.busy} onClick={() => doExport("json")}><Download className="h-3.5 w-3.5" />JSON</Button>
        </div>
      </Toolbar>
      {res.loading ? <TableSkeleton cols={6} rows={8} /> : res.error && !res.data ? <div className="p-4"><ErrorBox error={res.error} /></div> : !items.length ? (
        <EmptyState icon={ScrollText} title={t("noLogs")} description={t("noLogsDesc")} />
      ) : (
        <Table>
          <THead><tr><Th>{t("when")}</Th><Th>{t("request")}</Th><Th>{t("status")}</Th><Th className="hidden text-right sm:table-cell">{t("responseTime")}</Th><Th className="hidden text-right md:table-cell">{t("size")}</Th><Th className="hidden lg:table-cell">IP</Th><Th className="hidden xl:table-cell">{t("waitroom")}</Th></tr></THead>
          <tbody>
            {items.map((l, i) => (
              <Tr key={i}>
                <Td className="whitespace-nowrap font-mono text-xs text-fg-muted" title={fmtDateTime(toMs(l.ts), lang)}>{fmtClock(toMs(l.ts))}</Td>
                <Td className="max-w-[28rem]"><div className="flex items-baseline gap-1"><span className={methodCls}>{l.method}</span><span className="truncate font-mono text-xs" title={l.path}>{l.path}</span></div><div className="text-xs text-fg-subtle">{l.host}</div></Td>
                <Td><Badge tone={statusTone(l.status)}>{l.status}</Badge></Td>
                <Td className={cn("num hidden text-right sm:table-cell", l.rt_ms > 1000 ? "text-warn" : "text-fg-muted")}>{fmtMs(l.rt_ms)}</Td>
                <Td className="num hidden text-right text-fg-muted md:table-cell">{fmtBytes(l.bytes)}</Td>
                <Td className="hidden font-mono text-xs text-fg-muted lg:table-cell">{l.ip}</Td>
                <Td className="hidden text-xs xl:table-cell">{l.wr ? <Badge tone="brand">{l.wr}</Badge> : <span className="text-fg-subtle">–</span>}</Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      )}
    </Card>
  );
}

const eventMeta: Record<EventKind, { icon: LucideIcon; tone: Tone; key: Key }> = {
  blocked_rule: { icon: Ban, tone: "danger", key: "evBlockedRule" },
  blocked_body: { icon: Ban, tone: "danger", key: "evBlockedBody" },
  banned: { icon: Ban, tone: "danger", key: "evBanned" },
  exploit_detected: { icon: AlertOctagon, tone: "warn", key: "evExploitDetected" },
  body_detected: { icon: AlertOctagon, tone: "warn", key: "evBodyDetected" },
  rule_logged: { icon: Activity, tone: "info", key: "evRuleLogged" },
  rate_limit_dry_run: { icon: Gauge, tone: "info", key: "evRateLimitDryRun" },
  rate_limited: { icon: Gauge, tone: "warn", key: "evRateLimited" },
  blocked_exploit: { icon: Ban, tone: "danger", key: "evBlockedExploit" },
  server_error: { icon: ServerCrash, tone: "danger", key: "evServerError" },
  waitroom_queue: { icon: Hourglass, tone: "info", key: "evWaitroomQueue" },
  waitroom_fail: { icon: AlertOctagon, tone: "warn", key: "evWaitroomFail" },
};

function Events({ qs, poll }: { qs: string; poll: number }) {
  const { t, lang } = useI18n();
  const res = useResource<TrafficEvent[]>(`/api/traffic/events?${qs}`, { poll });
  const [kind, setKind] = useState<"" | EventKind>("");
  const items = (res.data ?? []).filter((e) => !kind || e.kind === kind);
  return (
    <Card>
      <Toolbar>
        <div className="w-52"><Select aria-label={t("eventType")} value={kind} onChange={(e) => setKind(e.target.value as "" | EventKind)} className="!h-8"><option value="">{t("allEvents")}</option>{(Object.keys(eventMeta) as EventKind[]).map((k) => <option key={k} value={k}>{t(eventMeta[k].key)}</option>)}</Select></div>
        <span className="ml-auto text-xs text-fg-subtle num">{t("nItems", { n: items.length })}</span>
      </Toolbar>
      {res.loading ? <TableSkeleton cols={4} rows={6} /> : res.error && !res.data ? <div className="p-4"><ErrorBox error={res.error} /></div> : !items.length ? (
        <EmptyState icon={AlertOctagon} title={t("noEvents")} description={t("noEventsDesc")} />
      ) : (
        <ol className="relative px-4 py-4 before:absolute before:bottom-6 before:left-[31px] before:top-6 before:w-px before:bg-line">
          {items.map((e, i) => {
            const m = eventMeta[e.kind] ?? { icon: Activity, tone: "neutral" as Tone, key: "evServerError" as const };
            return (
              <li key={i} className="relative flex gap-3 py-2">
                <span className="relative z-10 grid h-8 w-8 shrink-0 place-items-center rounded-full border border-line bg-surface"><m.icon className={cn("h-3.5 w-3.5", m.tone === "danger" ? "text-danger" : m.tone === "warn" ? "text-warn" : "text-info")} aria-hidden /></span>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2"><span className="text-sm font-medium">{eventMeta[e.kind] ? t(m.key) : e.kind}</span>{e.count > 1 && <Badge tone={m.tone}>×{fmtNum(e.count)}</Badge>}</div>
                  <div className="truncate text-xs text-fg-subtle"><span className="font-mono">{e.host}{e.path}</span>{e.ip && <> · <span className="font-mono">{e.ip}</span></>}</div>
                </div>
                <time className="num shrink-0 pt-0.5 text-xs text-fg-subtle" dateTime={new Date(toMs(e.ts)).toISOString()}>{fmtDateTime(toMs(e.ts), lang)}</time>
              </li>
            );
          })}
        </ol>
      )}
    </Card>
  );
}
