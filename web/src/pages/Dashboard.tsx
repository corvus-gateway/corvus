import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { BellRing, Activity, AlertTriangle, ArrowRight, Clock, Gauge, Globe, History, Hourglass, ShieldAlert, ShieldCheck, Siren, Users, Zap, type LucideIcon } from "lucide-react";
import type { AlertActive, AuditEntry, Certificate, Timeseries, UnderAttackStatus } from "../api";
import { ChartLegend, RadialGauge, Sparkline, TimeChart, type ChartPoint, type Series } from "../components/charts";
import { PageHeader } from "../components/common";
import { Badge, Card, CardHeader, EmptyState, Skeleton, StatusDot, type Tone } from "../components/ui/display";
import { Segmented } from "../components/ui/form";
import { useResource } from "../hooks/useResource";
import { useI18n } from "../i18n";
import { actionLabel, actionMeta } from "../lib/audit";
import { useApp } from "../lib/app";
import { cn, daysUntil, fmtCompact, fmtMs, fmtNum, fmtUptime, relTime } from "../lib/utils";

const RANGES = ["5m", "15m", "1h"] as const;
type Range = (typeof RANGES)[number];

function Kpi({ icon: Icon, label, value, unit, sub, spark, color, tone, loading }: {
  icon: LucideIcon; label: string; value: ReactNode; unit?: string; sub?: ReactNode; spark?: number[]; color: string; tone?: Tone; loading?: boolean;
}) {
  return (
    <Card className="group relative flex flex-col overflow-hidden p-4 transition-colors hover:border-line-strong">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-fg-subtle">{label}</span>
        <Icon className="h-3.5 w-3.5 text-fg-subtle/70" aria-hidden />
      </div>
      <div className="mt-2 flex items-baseline gap-1">
        {loading ? <Skeleton className="h-7 w-20" /> : <span className={cn("num text-2xl font-semibold tracking-tight", tone === "danger" ? "text-danger" : tone === "warn" ? "text-warn" : "text-fg")}>{value}</span>}
        {unit && !loading && <span className="text-xs text-fg-subtle">{unit}</span>}
      </div>
      <div className="mt-0.5 min-h-4 text-xs text-fg-subtle">{sub}</div>
      <div className="-mx-4 -mb-4 mt-2 h-8">{spark && spark.length > 1 && <Sparkline values={spark} color={color} height={32} />}</div>
    </Card>
  );
}

export default function Dashboard() {
  const { t, lang } = useI18n();
  const { overview, overviewError } = useApp();
  const [range, setRange] = useState<Range>("15m");
  const ts = useResource<Timeseries>(`/api/traffic/timeseries?range=${range}`, { poll: 5000 });
  const audit = useResource<AuditEntry[]>("/api/audit?limit=8", { poll: 15000 });
  const certs = useResource<Certificate[]>("/api/certificates", { poll: 60000 });
  const alerts = useResource<AlertActive[]>("/api/alerts/active", { poll: 10000 });
  const underAttack = (useResource<UnderAttackStatus[]>("/api/under-attack", { poll: 10000 }).data ?? []).filter((s) => s.active);

  // Client-side rolling history of queue length (overview is polled anyway).
  const [waitHist, setWaitHist] = useState<number[]>([]);
  const lastOv = useRef<unknown>(null);
  useEffect(() => {
    if (!overview || lastOv.current === overview) return;
    lastOv.current = overview;
    setWaitHist((h) => [...h.slice(-59), overview.rooms.reduce((a, r) => a + (r.available ? r.waiting : 0), 0)]);
  }, [overview]);

  const traffic = overview?.traffic ?? null;
  const points = ts.data?.points ?? [];
  const step = ts.data?.step_s || 1;
  const hasSeries = points.length > 1;
  const waitingNow = overview ? overview.rooms.reduce((a, r) => a + (r.available ? r.waiting : 0), 0) : 0;

  const chartPoints: ChartPoint[] = useMemo(() => points.map((p) => ({ t: p.t, v: { s2xx: p.s2xx, s3xx: p.s3xx, s4xx: p.s4xx, s5xx: p.s5xx } })), [points]);
  const series: Series[] = [
    { key: "s2xx", label: "2xx", color: "rgb(var(--chart-2))" },
    { key: "s3xx", label: "3xx", color: "rgb(var(--chart-1))" },
    { key: "s4xx", label: "4xx", color: "rgb(var(--chart-3))" },
    { key: "s5xx", label: "5xx", color: "rgb(var(--chart-4))" },
  ];
  const errPct = traffic && traffic.requests > 0 ? (traffic.s5xx / traffic.requests) * 100 : 0;
  const spark = {
    rps: points.map((p) => p.requests / step),
    req: points.map((p) => p.requests),
    err: points.map((p) => (p.requests ? (p.s5xx / p.requests) * 100 : 0)),
    p95: points.map((p) => p.p95),
    blocked: points.map((p) => p.blocked),
  };
  const loadingOv = !overview && !overviewError;
  const expiring = (certs.data ?? []).map((c) => ({ c, d: daysUntil(c.not_after) })).filter((x) => x.d !== null && x.d <= 30).sort((a, b) => a.d! - b.d!);
  const trafficKnown = traffic !== null;

  return (
    <div className="space-y-5">
      <PageHeader title={t("navDashboard")} description={t("dashboardDesc")}
        actions={<span className="flex items-center gap-2 text-xs text-fg-subtle"><StatusDot tone="ok" pulse />{t("liveUpdating")}</span>} />

      {!!alerts.data?.length && (
        <Link to="/alerts" className="flex items-center gap-3 rounded-[3px] border border-danger/40 bg-danger/10 px-4 py-2.5 hover:border-danger">
          <BellRing className="h-4 w-4 shrink-0 text-danger" aria-hidden />
          <span className="text-sm font-semibold">{t("activeNowN", { n: alerts.data.length })}</span>
          <span className="min-w-0 flex-1 truncate text-xs text-fg-muted">{alerts.data.map((x) => x.title).join(" · ")}</span>
          <ArrowRight className="h-4 w-4 shrink-0 text-fg-subtle" aria-hidden />
        </Link>
      )}

      {underAttack.length > 0 && (
        <Link to="/proxy-hosts" className="flex items-center gap-3 rounded-[3px] border border-danger/40 bg-danger/10 px-4 py-2.5 hover:border-danger">
          <Siren className="h-4 w-4 shrink-0 text-danger" aria-hidden />
          <span className="text-sm font-semibold">{t("uaDashTitle", { n: underAttack.length })}</span>
          <span className="min-w-0 flex-1 truncate text-xs text-fg-muted">{underAttack.map((s) => `${s.host} (${s.mode === "auto" ? t("uaAuto") : t("uaOn")})`).join(" · ")}</span>
          <ArrowRight className="h-4 w-4 shrink-0 text-fg-subtle" aria-hidden />
        </Link>
      )}

      <div className={cn("grid grid-cols-2 gap-3", trafficKnown ? "md:grid-cols-3 xl:grid-cols-6" : "md:grid-cols-4")}>
        {trafficKnown ? (<>
          <Kpi icon={Zap} label={t("kpiRps")} value={fmtNum(traffic!.rps, traffic!.rps < 10 ? 1 : 0)} unit="req/s" sub={`${t("range")} ${traffic!.range}`} spark={spark.rps} color="rgb(var(--chart-1))" />
          <Kpi icon={Activity} label={t("kpiRequests")} value={fmtCompact(traffic!.requests)} sub={`${t("uniqueIps")} ${fmtCompact(traffic!.unique_ips)}`} spark={spark.req} color="rgb(var(--chart-2))" />
          <Kpi icon={AlertTriangle} label={t("kpiErrorRate")} value={errPct.toFixed(errPct < 10 ? 2 : 1)} unit="%" tone={errPct >= 5 ? "danger" : errPct >= 1 ? "warn" : undefined} sub={`5xx ${fmtNum(traffic!.s5xx)} · 4xx ${fmtNum(traffic!.s4xx)}`} spark={spark.err} color="rgb(var(--chart-4))" />
          <Kpi icon={Clock} label={t("kpiP95")} value={fmtMs(traffic!.p95).replace(" ms", "").replace(" s", "")} unit={traffic!.p95 >= 1000 ? "s" : "ms"} sub={`p50 ${fmtMs(traffic!.p50)} · p99 ${fmtMs(traffic!.p99)}`} spark={spark.p95} color="rgb(var(--chart-5))" />
          <Kpi icon={ShieldAlert} label={t("kpiBlocked")} value={fmtCompact(traffic!.blocked)} sub={t("blockedHint")} spark={spark.blocked} color="rgb(var(--chart-3))" />
          <Kpi icon={Hourglass} label={t("kpiWaiting")} value={fmtNum(waitingNow)} sub={t("waitingHint")} spark={waitHist} color="rgb(var(--chart-1))" />
        </>) : (<>
          <Kpi icon={Globe} label={t("navHosts")} loading={loadingOv} value={overview ? `${overview.proxy_hosts_enabled}/${overview.proxy_hosts}` : "–"} sub={t("enabledOfTotal")} color="rgb(var(--chart-1))" />
          <Kpi icon={Users} label={t("navRooms")} loading={loadingOv} value={fmtNum(overview?.waitrooms)} sub={`${t("kpiWaiting")} ${fmtNum(waitingNow)}`} spark={waitHist} color="rgb(var(--chart-2))" />
          <Kpi icon={ShieldCheck} label={t("navCerts")} loading={loadingOv} value={fmtNum(overview?.certificates)} sub={overview?.certificates_expiring ? t("expiringN", { n: overview.certificates_expiring }) : t("allHealthy")} color="rgb(var(--chart-3))" />
          <Kpi icon={History} label={t("appliedVersionLabel")} loading={loadingOv} value={overview?.active_version ? `#${overview.active_version.id}` : "–"} sub={overview?.active_version ? relTime(overview.active_version.created_at, lang) : undefined} color="rgb(var(--chart-5))" />
        </>)}
      </div>

      <div className="grid gap-5 xl:grid-cols-3">
        <Card className="xl:col-span-2">
          <CardHeader icon={Activity} title={t("liveTraffic")} description={t("liveTrafficDesc")}
            actions={<Segmented className="!w-auto" label={t("range")} value={range} onChange={setRange} options={RANGES.map((r) => ({ value: r, label: r }))} />} />
          <div className="px-4 pb-4">
            {ts.loading ? <Skeleton className="h-[220px]" /> : ts.missing || (!hasSeries && !ts.data) ? (
              <EmptyState icon={Activity} className="py-12" title={ts.missing ? t("comingSoon") : t("noTrafficYet")} description={ts.missing ? t("comingSoonDesc") : t("noTrafficYetDesc")} />
            ) : (<>
              <TimeChart points={chartPoints} series={series} height={220} lang={lang} format={(n) => fmtCompact(n)} empty={t("noTrafficYet")} />
              <div className="mt-2 flex items-center justify-between"><ChartLegend series={series} /><span className="text-xs text-fg-subtle">{t("perStep", { s: step })}</span></div>
            </>)}
          </div>
        </Card>

        <Card>
          <CardHeader icon={Users} title={t("liveRooms")} actions={<Link to="/waitrooms" className="text-xs text-fg-subtle hover:text-fg">{t("viewAll")}</Link>} />
          <div className="px-4 pb-4">
            {loadingOv ? <Skeleton className="h-28" /> : !overview?.rooms.length ? (
              <EmptyState icon={Users} className="py-8" title={t("noRooms")} description={t("noRoomsDesc")} />
            ) : (
              <ul className="space-y-3">
                {overview.rooms.map((r) => (
                  <li key={r.name} className="flex items-center gap-4 rounded-lg border border-line bg-surface-2/40 p-3">
                    <RadialGauge value={r.available ? r.active : 0} max={r.capacity} />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2"><span className="truncate font-mono text-sm font-medium">{r.name}</span>{!r.available && <Badge tone="warn">{t("unavailable")}</Badge>}</div>
                      <dl className="mt-1.5 grid grid-cols-3 gap-2 text-xs">
                        <div><dt className="text-fg-subtle">{t("activeLabel")}</dt><dd className="num font-semibold text-fg">{r.available ? r.active : "–"}</dd></div>
                        <div><dt className="text-fg-subtle">{t("waitingLabel")}</dt><dd className={cn("num font-semibold", r.waiting > 0 ? "text-warn" : "text-fg")}>{r.available ? r.waiting : "–"}</dd></div>
                        <div><dt className="text-fg-subtle">{t("capacityLabel")}</dt><dd className="num font-semibold text-fg">{r.capacity}</dd></div>
                      </dl>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </Card>
      </div>

      <div className="grid gap-5 lg:grid-cols-3">
        <Card>
          <CardHeader icon={Gauge} title={t("systemHealth")} />
          <dl className="divide-y divide-line px-4 pb-2 text-sm">
            {loadingOv ? <div className="space-y-3 py-3"><Skeleton /><Skeleton /><Skeleton /></div> : (<>
              <Row label={t("gateway")}>{overview?.gateway ? <Badge tone={overview.gateway.running ? "ok" : "danger"} dot>{overview.gateway.running ? t("running") : t("stopped")}</Badge> : <span className="text-fg-subtle">–</span>}
                {overview?.gateway?.running && overview.gateway.uptime_s != null && <span className="ml-2 text-xs text-fg-subtle num">{fmtUptime(overview.gateway.uptime_s)}</span>}</Row>
              <Row label={t("cacheDb")}><Badge tone={overview?.redis ? "ok" : "warn"} dot>{overview?.redis ? t("connected") : t("disconnected")}</Badge></Row>
              <Row label={t("appliedVersionLabel")}>{overview?.active_version ? <><span className="num font-medium">#{overview.active_version.id}</span><span className="ml-2 text-xs text-fg-subtle">{relTime(overview.active_version.created_at, lang)}</span></> : <span className="text-fg-subtle">{t("noVersion")}</span>}</Row>
              <Row label={t("navHosts")}><span className="num font-medium">{overview?.proxy_hosts_enabled}</span><span className="text-fg-subtle num"> / {overview?.proxy_hosts}</span></Row>
              <Row label={t("controlPlane")}><span className="num text-fg-muted">v{overview?.version}</span></Row>
            </>)}
          </dl>
          {overview && !overview.redis && <p className="mx-4 mb-4 rounded-lg bg-warn/10 px-3 py-2 text-xs text-fg-muted">{t("redisDown")}</p>}
        </Card>

        <Card>
          <CardHeader icon={History} title={t("recentActivity")} actions={<Link to="/audit" className="text-xs text-fg-subtle hover:text-fg">{t("viewAll")}</Link>} />
          <div className="px-4 pb-3">
            {audit.loading ? <div className="space-y-3 py-2"><Skeleton /><Skeleton /><Skeleton /></div> : audit.missing || !audit.data?.length ? (
              <EmptyState icon={History} className="py-8" title={t("noActivity")} />
            ) : (
              <ol className="relative space-y-3.5 py-1 before:absolute before:bottom-2 before:left-[13px] before:top-2 before:w-px before:bg-line">
                {audit.data.map((a) => {
                  const m = actionMeta(a.action);
                  return (
                    <li key={a.id} className="relative flex gap-3">
                      <span className="relative z-10 grid h-7 w-7 shrink-0 place-items-center rounded-full border border-line bg-surface"><m.icon className="h-3.5 w-3.5 text-fg-subtle" aria-hidden /></span>
                      <div className="min-w-0 flex-1 pt-px">
                        <div className="truncate text-sm font-medium text-fg">{actionLabel(a.action, lang)}</div>
                        <div className="truncate text-xs text-fg-subtle">{[a.target, a.email || a.ip, relTime(a.ts, lang)].filter(Boolean).join(" · ")}</div>
                      </div>
                    </li>
                  );
                })}
              </ol>
            )}
          </div>
        </Card>

        <Card>
          <CardHeader icon={ShieldCheck} title={t("expiringCerts")} actions={<Link to="/certificates" className="text-xs text-fg-subtle hover:text-fg">{t("viewAll")}</Link>} />
          <div className="px-4 pb-4">
            {certs.loading ? <Skeleton className="h-16" /> : !expiring.length ? (
              <EmptyState icon={ShieldCheck} className="py-8" title={t("noExpiring")} description={t("noExpiringDesc")} />
            ) : (
              <ul className="space-y-2">
                {expiring.map(({ c, d }) => (
                  <li key={c.id}>
                    <Link to="/certificates" className="flex items-center justify-between gap-3 rounded-lg border border-line px-3 py-2.5 transition hover:bg-surface-2">
                      <div className="min-w-0"><div className="truncate text-sm font-medium">{c.name}</div><div className="truncate font-mono text-xs text-fg-subtle">{c.domains.join(", ")}</div></div>
                      <Badge tone={d! < 0 ? "danger" : d! <= 7 ? "danger" : "warn"}>{d! < 0 ? t("expired") : t("daysLeft", { n: d! })}</Badge>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </div>
          <div className="px-4 pb-4 pt-0"><Link to="/certificates" className="inline-flex items-center gap-1 text-xs text-brand hover:underline">{t("manageCerts")}<ArrowRight className="h-3 w-3" /></Link></div>
        </Card>
      </div>
    </div>
  );
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return <div className="flex items-center justify-between gap-3 py-2.5"><dt className="text-fg-muted">{label}</dt><dd className="flex items-center text-right">{children}</dd></div>;
}
