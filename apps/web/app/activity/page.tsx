"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import {
  Activity,
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Cpu,
  Database,
  ExternalLink,
  Gauge,
  Info,
  MemoryStick,
  RadioTower,
  Search,
  Server,
  Users
} from "lucide-react";
import { useMemo, useState } from "react";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { PageHeader } from "@/components/page-header";
import { Card } from "@/components/ui";
import { getObservabilityMetrics, listActivity, listGames, listServers, prometheusMetricsUrl } from "@/lib/api";
import { formatActivityEvent } from "@/lib/activity-display";
import { gameFilterOptions } from "@/lib/game-filters";
import {
  createMonitoringModel,
  eventTypeOptions,
  monitoringMockModel,
  severityOptions,
  shouldUseMonitoringMock,
  type MonitoringEvent,
  type MonitoringEventType,
  type MonitoringHealth,
  type MonitoringServerRow,
  type MonitoringSeverity,
  type MonitoringSeverityFilter
} from "@/lib/monitoring";
import { useTimeSeries, type SeriesPoint } from "@/lib/sparkline";
import { cn } from "@/lib/utils";
import { useI18n, type MessageKey } from "@/lib/i18n";

type ActivityGameFilter = "all" | string;

export default function ActivityPage() {
  const { locale, t } = useI18n();
  const activityQuery = useQuery({ queryKey: ["activity"], queryFn: listActivity, retry: false });
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false, staleTime: 5 * 60 * 1000 });
  const metricsQuery = useQuery({ queryKey: ["observability-metrics"], queryFn: getObservabilityMetrics, retry: false, refetchInterval: 5000 });
  const [search, setSearch] = useState("");
  const [gameFilter, setGameFilter] = useState<ActivityGameFilter>("all");
  const [eventTypeFilter, setEventTypeFilter] = useState<MonitoringEventType>("all");
  const [severityFilter, setSeverityFilter] = useState<MonitoringSeverityFilter>("all");

  const events = activityQuery.data ?? [];
  const servers = serversQuery.data ?? [];
  const games = gamesQuery.data ?? [];
  const displayByEventId = useMemo(
    () => new Map(events.map((event) => [event.id, formatActivityEvent(event, locale)])),
    [events, locale]
  );
  const monitoring = useMemo(() => {
    if (shouldUseMonitoringMock(metricsQuery.data, servers, events)) {
      return monitoringMockModel();
    }
    return createMonitoringModel({
      activity: events,
      displayByEventId,
      games,
      metrics: metricsQuery.data,
      servers
    });
  }, [displayByEventId, events, games, metricsQuery.data, servers]);

  const cpuSeries = useTimeSeries(monitoring.trends.cpuPercent, 60);
  const memorySeries = useTimeSeries(monitoring.trends.memoryMb, 60);
  const playerSeries = useTimeSeries(monitoring.trends.playerCount, 60);
  const eventSeries = useTimeSeries(monitoring.trends.eventCount, 60);
  const serverById = useMemo(() => new Map(servers.map((server) => [server.id, server])), [servers]);
  const gameFilters = useMemo(
    () => gameFilterOptions(games, t("filterAll"), events.map((event) => event.instanceId ? serverById.get(event.instanceId)?.gameKey : undefined)),
    [events, games, serverById, t]
  );
  const activeFilterChips = [
    search.trim(),
    gameFilter !== "all" ? filterOptionLabel(gameFilters, gameFilter, t) : "",
    eventTypeFilter !== "all" ? t(eventTypeLabelKey(eventTypeFilter)) : "",
    severityFilter !== "all" ? t(severityLabelKey(severityFilter)) : ""
  ].filter(Boolean);
  const filteredEvents = useMemo(() => {
    const term = search.trim().toLowerCase();
    return monitoring.events.filter((event) => {
      const sourceServer = events.find((item) => item.id === event.id)?.instanceId;
      const sourceGame = sourceServer ? serverById.get(sourceServer)?.gameKey : undefined;
      const matchesGame = gameFilter === "all" || sourceGame === gameFilter;
      const matchesType = eventTypeFilter === "all" || event.kind === eventTypeFilter;
      const matchesSeverity = severityFilter === "all" || event.severity === severityFilter;
      const matchesSearch = !term || event.searchText.includes(term);
      return matchesGame && matchesType && matchesSeverity && matchesSearch;
    });
  }, [eventTypeFilter, events, gameFilter, monitoring.events, search, serverById, severityFilter]);
  const topServerRows = monitoring.serverRows.slice(0, 8);

  return (
    <>
      <PageHeader
        title={t("monitoringTitle")}
        description={t("monitoringDescription")}
        action={
          <div className="flex flex-wrap justify-end gap-2">
            <TechBadge label={t("monitoringDatasource")} value="Prometheus" />
            <TechBadge label={t("monitoringRange")} value="15m" />
            <TechBadge label={t("monitoringRefresh")} value="5s" />
            <a
              className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-panel-line bg-slate-900/70 px-3 text-xs font-medium text-slate-300 transition hover:bg-slate-800 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
              href={prometheusMetricsUrl()}
              rel="noreferrer"
              target="_blank"
            >
              <RadioTower aria-hidden="true" className="size-3.5 text-panel-green" />
              {t("monitoringEndpoint")}
            </a>
          </div>
        }
      />
      {(activityQuery.isError || serversQuery.isError || gamesQuery.isError || metricsQuery.isError) && <p className="mb-4 text-sm text-panel-gold">{t("apiActivityUnavailable")}</p>}

      <section className="mb-5 grid gap-3 xl:grid-cols-5">
        <KpiCard icon={<Server aria-hidden="true" className="size-4" />} label={t("kpiTotalServers")} note={t("kpiTotalServersNote")} value={monitoring.kpis.totalServers} />
        <KpiCard icon={<CheckCircle2 aria-hidden="true" className="size-4" />} label={t("kpiRunning")} note={t("kpiRunningNote")} tone="success" value={monitoring.kpis.runningServers} />
        <KpiCard icon={<AlertTriangle aria-hidden="true" className="size-4" />} label={t("kpiIssues")} note={monitoring.kpis.issues > 0 ? t("kpiIssuesAction") : t("kpiIssuesClear")} tone={monitoring.kpis.issues > 0 ? "warning" : "success"} value={monitoring.kpis.issues} />
        <KpiCard icon={<Users aria-hidden="true" className="size-4" />} label={t("kpiOnlinePlayers")} note={t("playersOnlineHint", { count: monitoring.kpis.onlinePlayers, capacity: monitoring.kpis.playerCapacity })} value={monitoring.kpis.onlinePlayers} />
        <KpiCard icon={<Gauge aria-hidden="true" className="size-4" />} label={t("kpiResourceUsage")} note={t("kpiResourceUsageNote")} tone={monitoring.kpis.resourceUsagePercent > 75 ? "warning" : "success"} value={`${monitoring.kpis.resourceUsagePercent}%`} />
      </section>

      <section className="mb-5 grid gap-4 xl:grid-cols-[360px_minmax(0,1fr)]">
        <HealthOverview health={monitoring.health} />
        <div className="grid gap-4 md:grid-cols-2">
          <TrendCard
            color="#7bd978"
            current={`${monitoring.trends.cpuPercent.toFixed(1)}%`}
            currentValue={monitoring.trends.cpuPercent}
            emptyLabel={t("monitoringNoCpuData")}
            icon={<Cpu aria-hidden="true" className="size-4" />}
            limitLabel={t("chartCpuLimit")}
            max={100}
            note={t("trendCpuNote")}
            series={cpuSeries}
            title={t("trendCpuTitle")}
            unit="%"
          />
          <TrendCard
            color="#a78bfa"
            current={`${Math.round(monitoring.trends.memoryMb)} MB`}
            currentValue={monitoring.trends.memoryMb}
            emptyLabel={t("monitoringNoMemoryData")}
            icon={<MemoryStick aria-hidden="true" className="size-4" />}
            limitLabel={t("chartMemoryLimit", { limit: `${Math.round(monitoring.trends.memoryLimitMb)} MB` })}
            max={monitoring.trends.memoryLimitMb}
            note={t("trendMemoryNote")}
            series={memorySeries}
            title={t("trendMemoryTitle")}
            unit="MB"
          />
          <TrendCard
            color="#7bd978"
            current={`${monitoring.trends.playerCount}`}
            currentValue={monitoring.trends.playerCount}
            emptyLabel={t("monitoringNoPlayerData")}
            icon={<Users aria-hidden="true" className="size-4" />}
            limitLabel={t("chartPlayerCapacity", { capacity: monitoring.kpis.playerCapacity })}
            max={Math.max(1, monitoring.kpis.playerCapacity)}
            note={t("trendPlayersNote")}
            series={playerSeries}
            title={t("trendPlayersTitle")}
            unit=""
          />
          <TrendCard
            color="#f4c95d"
            current={`${monitoring.trends.eventCount}`}
            currentValue={monitoring.trends.eventCount}
            emptyLabel={t("monitoringNoEventData")}
            icon={<Activity aria-hidden="true" className="size-4" />}
            limitLabel={t("chartEventScale", { limit: Math.max(10, monitoring.trends.eventCount) })}
            max={Math.max(10, monitoring.trends.eventCount)}
            note={t("trendEventsNote")}
            series={eventSeries}
            title={t("trendEventsTitle")}
            unit=""
          />
        </div>
      </section>

      <section className="mb-5 overflow-hidden rounded-lg border border-panel-line bg-slate-950/35">
        <SectionHeader title={t("serverLoadTitle")} description={t("serverLoadDescription")} />
        <div className="hidden grid-cols-[minmax(220px,1.2fr)_110px_110px_150px_150px_100px_120px_90px] border-b border-panel-line px-4 py-2 text-xs font-medium text-slate-500 xl:grid">
          <span>{t("monitoringTableServer")}</span>
          <span>{t("monitoringTableGame")}</span>
          <span>{t("monitoringTableStatus")}</span>
          <span>{t("monitoringTableCpu")}</span>
          <span>{t("monitoringTableMemory")}</span>
          <span>{t("monitoringTablePlayers")}</span>
          <span>{t("monitoringTableLastActive")}</span>
          <span>{t("monitoringTableAction")}</span>
        </div>
        <div className="divide-y divide-panel-line">
          {topServerRows.length > 0 ? (
            topServerRows.map((server) => <ServerLoadRow key={server.id} server={server} />)
          ) : (
            <EmptyBlock icon={<Server aria-hidden="true" className="size-5" />} label={t("noServersYet")} />
          )}
        </div>
      </section>

      <section>
        <SectionHeader title={t("recentAlertsTitle")} description={t("recentAlertsDescription")} className="mb-3 rounded-lg border border-panel-line bg-slate-950/35" />
        <ResourceFilterBar
          activeChips={activeFilterChips}
          clearLabel={t("clearFilters")}
          density="compact"
          filters={[
            { label: t("filterGame"), options: gameFilters, value: gameFilter, onChange: (value) => setGameFilter(value) },
            { label: t("filterType"), options: eventTypeOptions.map((key) => ({ key, labelKey: eventTypeLabelKey(key) })), value: eventTypeFilter, onChange: (value) => setEventTypeFilter(value as MonitoringEventType) },
            { label: t("filterSeverity"), options: severityOptions.map((key) => ({ key, labelKey: severityLabelKey(key) })), value: severityFilter, onChange: (value) => setSeverityFilter(value as MonitoringSeverityFilter) }
          ]}
          onClear={() => {
            setGameFilter("all");
            setEventTypeFilter("all");
            setSeverityFilter("all");
            setSearch("");
          }}
          onSearchChange={setSearch}
          resultLabel={t("filteredResultsCount", { count: filteredEvents.length })}
          search={search}
          searchPlaceholder={t("searchMonitoringEvents")}
        />
        <Card className="overflow-hidden">
          {filteredEvents.length === 0 ? (
            <EmptyBlock icon={<Search aria-hidden="true" className="size-5" />} label={activityQuery.isLoading ? t("loading") : t("noResultsMatchFilters")} />
          ) : (
            <div className="divide-y divide-panel-line">
              {filteredEvents.map((event) => <EventRow key={event.id} event={event} />)}
            </div>
          )}
        </Card>
      </section>
    </>
  );
}

function TechBadge({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex h-9 items-center gap-2 rounded-md border border-panel-line bg-slate-900/55 px-3 text-xs">
      <span className="text-slate-500">{label}</span>
      <span className="font-mono font-medium text-slate-200">{value}</span>
    </span>
  );
}

function KpiCard({ icon, label, note, tone = "neutral", value }: { icon: React.ReactNode; label: string; note: string; tone?: "neutral" | "success" | "warning"; value: string | number }) {
  return (
    <Card className="p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-xs font-medium text-slate-500">{label}</p>
          <p className="mt-2 font-mono text-2xl font-semibold text-slate-100">{value}</p>
        </div>
        <span className={cn("flex size-8 items-center justify-center rounded-md border", toneClass(tone))}>{icon}</span>
      </div>
      <p className="mt-3 truncate text-xs text-slate-500">{note}</p>
    </Card>
  );
}

function HealthOverview({ health }: { health: MonitoringHealth }) {
  const { t } = useI18n();
  return (
    <Card className="p-4">
      <div className="mb-4 flex items-start justify-between gap-3">
        <div>
          <h2 className="font-semibold text-slate-100">{t("healthOverviewTitle")}</h2>
          <p className="mt-1 text-xs text-slate-500">{t("healthOverviewDescription")}</p>
        </div>
        <span className={cn("rounded-md border px-2 py-1 text-xs font-medium", health.overall === "healthy" ? "border-panel-green/30 bg-panel-green/10 text-panel-green" : "border-panel-gold/30 bg-panel-gold/10 text-panel-gold")}>
          {t(health.overall === "healthy" ? "healthHealthy" : health.overall === "critical" ? "healthCritical" : "healthWarning")}
        </span>
      </div>
      <div className="space-y-3">
        <HealthRow label={t("healthOverall")} value={t(health.overall === "healthy" ? "healthHealthy" : health.overall === "critical" ? "healthCritical" : "healthWarning")} severity={health.overall === "healthy" ? "success" : "warning"} />
        <HealthRow label={t("healthPrometheusConnected")} value={health.prometheusConnected ? t("connected") : t("unavailable")} severity={health.prometheusConnected ? "success" : "warning"} />
        <HealthRow label={t("healthDockerRuntime")} value={t(health.dockerRuntime === "healthy" ? "available" : health.dockerRuntime === "degraded" ? "healthDegraded" : "unavailable")} severity={health.dockerRuntime === "healthy" ? "success" : "warning"} />
        <HealthRow label={t("healthLastSync")} value={health.lastSyncLabel} severity="info" />
        <HealthRow label={t("healthFailedTargets")} value={String(health.failedTargets)} severity={health.failedTargets > 0 ? "warning" : "success"} />
      </div>
      <div className="mt-4 rounded-md border border-panel-line bg-slate-950/45 p-3 text-xs text-slate-500">
        <div className="flex items-center justify-between gap-3">
          <span>{t("monitoringDatasource")}</span>
          <span className="font-mono text-slate-300">Prometheus</span>
        </div>
        <div className="mt-2 flex items-center justify-between gap-3">
          <span>{t("monitoringEndpoint")}</span>
          <span className="font-mono text-slate-300">/metrics</span>
        </div>
      </div>
    </Card>
  );
}

function HealthRow({ label, severity, value }: { label: string; severity: MonitoringSeverity; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-sm text-slate-400">{label}</span>
      <span className="inline-flex items-center gap-2 text-sm font-medium text-slate-200">
        <span className={cn("size-2 rounded-full", dotClass(severity))} />
        {value}
      </span>
    </div>
  );
}

function TrendCard({
  color,
  current,
  currentValue,
  emptyLabel,
  icon,
  limitLabel,
  max,
  note,
  series,
  title,
  unit
}: {
  color: string;
  current: string;
  currentValue: number;
  emptyLabel: string;
  icon: React.ReactNode;
  limitLabel: string;
  max: number;
  note: string;
  series: SeriesPoint[];
  title: string;
  unit: string;
}) {
  const { t } = useI18n();
  const chartSeries = series.length >= 2 ? series : seedSeries(currentValue);
  const chart = buildChart(chartSeries, max);
  const isEmpty = chartSeries.length === 0;
  const recentSamples = chartSeries.slice(-5);
  return (
    <Card className="h-[260px] p-4">
      <div className="mb-3 flex items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2 text-sm font-semibold text-slate-100">
            <span style={{ color }}>{icon}</span>
            {title}
          </div>
          <p className="mt-1 text-xs text-slate-500">{note}</p>
        </div>
        <p className="font-mono text-xl font-semibold text-slate-100">{current}</p>
      </div>
      <div className="relative h-40 overflow-hidden rounded-md border border-panel-line bg-slate-950/35">
        {isEmpty ? (
          <EmptyChart label={emptyLabel} />
        ) : (
          <svg className="h-full w-full" role="img" viewBox="0 0 640 176">
            {chart.yTicks.map((tick) => (
              <g key={tick.value}>
                <line x1="52" x2="616" y1={tick.y} y2={tick.y} stroke="rgba(148,163,184,0.14)" strokeWidth="1" />
                <text x="12" y={tick.y + 4} fill="#64748b" fontSize="11" fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace">{formatSampleValue(tick.value, unit)}</text>
              </g>
            ))}
            <line x1="52" x2="616" y1={chart.limitY} y2={chart.limitY} stroke="rgba(244,201,93,0.65)" strokeDasharray="4 4" strokeWidth="1.25" />
            <text x="530" y={Math.max(14, chart.limitY - 6)} fill="#f4c95d" fontSize="11" fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace">{t("chartUpperLimit")}</text>
            <path d={chart.area} fill={color} opacity="0.08" />
            <path d={chart.line} fill="none" stroke={color} strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" vectorEffect="non-scaling-stroke" />
            {chart.points.map((point, index) => (
              <g key={`${point.ts}-${index}`}>
                <circle cx={point.x} cy={point.y} r="3" fill="#0b111a" stroke={color} strokeWidth="2">
                  <title>{`${formatSampleTime(point.ts)} · ${formatSampleValue(point.value, unit)}`}</title>
                </circle>
              </g>
            ))}
            {chart.xTicks.map((tick) => (
              <text key={tick.ts} x={tick.x} y="164" textAnchor={tick.anchor} fill="#64748b" fontSize="11" fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace">{formatSampleTime(tick.ts)}</text>
            ))}
          </svg>
        )}
      </div>
      <div className="mt-2 flex items-center justify-between gap-3 text-[11px] text-slate-500">
        <span className="truncate">{limitLabel}</span>
        <span className="font-mono">{t("chartSamples")}: {recentSamples.map((sample) => formatSampleValue(sample.value, unit)).join(" / ")}</span>
      </div>
    </Card>
  );
}

function ServerLoadRow({ server }: { server: MonitoringServerRow }) {
  const { t } = useI18n();
  return (
    <div className={cn("grid gap-3 px-4 py-3 xl:grid-cols-[minmax(220px,1.2fr)_110px_110px_150px_150px_100px_120px_90px] xl:items-center", server.severity === "error" && "bg-panel-gold/5")}>
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className={cn("size-2 rounded-full", dotClass(server.severity))} />
          <p className="truncate font-medium text-slate-100">{server.name}</p>
        </div>
        <p className="mt-1 truncate text-xs text-slate-500">{server.providerLabel} · {server.version}</p>
      </div>
      <span className="text-sm text-slate-300">{server.gameLabel}</span>
      <StatusBadge status={server.status} severity={server.severity} />
      <LoadBar value={`${server.cpuPercent.toFixed(1)}%`} percent={server.cpuPercent} />
      <LoadBar value={`${Math.round(server.memoryMb)} MB`} percent={server.memoryPercent} tone="purple" />
      <span className="font-mono text-sm text-slate-300">{server.playersOnline}/{server.maxPlayers}</span>
      <span className="text-sm text-slate-500">{server.lastActive}</span>
      <Link className="inline-flex w-fit items-center gap-1 rounded border border-panel-line px-2 py-1 text-xs font-medium text-slate-300 transition hover:bg-slate-900" href={server.actionHref}>
        {t("view")}
        <ExternalLink aria-hidden="true" className="size-3" />
      </Link>
    </div>
  );
}

function StatusBadge({ severity, status }: { severity: MonitoringSeverity; status: string }) {
  return (
    <span className={cn("w-fit rounded px-2 py-0.5 text-xs font-medium", severity === "error" ? "bg-panel-gold/15 text-panel-gold" : severity === "success" ? "bg-panel-green/15 text-panel-green" : "bg-slate-800 text-slate-400")}>
      {status}
    </span>
  );
}

function LoadBar({ percent, tone = "green", value }: { percent: number; tone?: "green" | "purple"; value: string }) {
  return (
    <div>
      <div className="mb-1 flex justify-between gap-2 text-xs">
        <span className="font-mono text-slate-300">{value}</span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-slate-800">
        <div className={cn("h-full rounded-full", tone === "purple" ? "bg-panel-purple" : percent > 80 ? "bg-panel-gold" : "bg-panel-green")} style={{ width: `${Math.max(3, Math.min(percent, 100))}%` }} />
      </div>
    </div>
  );
}

function EventRow({ event }: { event: MonitoringEvent }) {
  return (
    <div className={cn("grid gap-3 px-4 py-3 md:grid-cols-[minmax(0,1fr)_150px_120px_110px] md:items-center", event.severity === "error" && "bg-panel-gold/5")}>
      <div className="flex min-w-0 gap-3">
        <span className={cn("mt-1 flex size-7 shrink-0 items-center justify-center rounded-md border", toneClass(event.severity))}>{severityIcon(event.severity)}</span>
        <div className="min-w-0">
          <p className="truncate font-medium text-slate-100">{event.title}</p>
          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-slate-500">
            <span>{event.serverName}</span>
            <span>·</span>
            <span>{event.operator}</span>
          </div>
        </div>
      </div>
      <StatusBadge status={event.typeLabel} severity={event.severity} />
      <span className="text-xs text-slate-500">{event.timestamp}</span>
      <span className="font-mono text-xs text-slate-500">{event.rawType}</span>
    </div>
  );
}

function SectionHeader({ className, description, title }: { className?: string; description: string; title: string }) {
  return (
    <div className={cn("border-b border-panel-line px-4 py-3", className)}>
      <h2 className="text-sm font-semibold text-slate-100">{title}</h2>
      <p className="mt-1 text-xs text-slate-500">{description}</p>
    </div>
  );
}

function EmptyBlock({ icon, label }: { icon: React.ReactNode; label: string }) {
  return (
    <div className="flex min-h-36 items-center justify-center p-6 text-center">
      <div>
        <span className="mx-auto flex size-10 items-center justify-center rounded-md border border-panel-line bg-slate-950/50 text-slate-500">{icon}</span>
        <p className="mt-3 text-sm text-slate-400">{label}</p>
      </div>
    </div>
  );
}

function EmptyChart({ label }: { label: string }) {
  return (
    <div className="flex h-full items-center justify-center">
      <div className="text-center">
        <Database aria-hidden="true" className="mx-auto size-5 text-slate-600" />
        <p className="mt-2 text-xs text-slate-500">{label}</p>
      </div>
    </div>
  );
}

function buildChart(series: SeriesPoint[], max: number) {
  const width = 564;
  const height = 112;
  const left = 52;
  const top = 18;
  const bottom = top + height;
  const pointsSource = series.length >= 2 ? series : [{ value: 0, ts: 0 }, { value: series[0]?.value ?? 0, ts: 1 }];
  const first = pointsSource[0]!.ts;
  const last = pointsSource[pointsSource.length - 1]!.ts;
  const points = pointsSource.map((point, index) => {
    const x = left + ((point.ts - first) / (last - first || 1)) * width;
    const y = bottom - Math.min(1, Math.max(0, point.value / Math.max(1, max))) * height;
    return {
      ts: point.ts,
      value: point.value,
      x: Number.isFinite(x) ? x : left + index / Math.max(1, pointsSource.length - 1) * width,
      y
    };
  });
  const line = points.map((point, index) => `${index === 0 ? "M" : "L"} ${point.x.toFixed(1)} ${point.y.toFixed(1)}`).join(" ");
  const yTicks = [max, max * 0.66, max * 0.33, 0].map((value) => ({
    value,
    y: bottom - Math.min(1, Math.max(0, value / Math.max(1, max))) * height
  }));
  const xTickSource = [points[0], points[Math.floor((points.length - 1) / 2)], points[points.length - 1]].filter(Boolean);
  return {
    area: `${line} L ${left + width} ${bottom} L ${left} ${bottom} Z`,
    last: points[points.length - 1]!,
    limitY: top,
    line,
    points,
    xTicks: xTickSource.map((point, index) => ({
      anchor: index === 0 ? "start" : index === xTickSource.length - 1 ? "end" : "middle" as "start" | "middle" | "end",
      ts: point!.ts,
      x: point!.x
    })),
    yTicks
  };
}

function seedSeries(value: number): SeriesPoint[] {
  const now = Date.now();
  return Array.from({ length: 8 }, (_, index) => ({
    ts: now - (7 - index) * 120000,
    value: Math.max(0, value + value * 0.06 * Math.sin(index * 0.9))
  }));
}

function formatSampleTime(ts: number) {
  return new Date(ts).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function formatSampleValue(value: number, unit: string) {
  const rounded = unit === "%" ? value.toFixed(1) : Number.isInteger(value) ? String(value) : value.toFixed(0);
  return unit ? `${rounded}${unit === "MB" ? " MB" : unit}` : rounded;
}

function toneClass(tone: MonitoringSeverity | "neutral") {
  if (tone === "error" || tone === "warning") return "border-panel-gold/30 bg-panel-gold/10 text-panel-gold";
  if (tone === "success") return "border-panel-green/30 bg-panel-green/10 text-panel-green";
  return "border-panel-line bg-slate-900 text-slate-400";
}

function dotClass(severity: MonitoringSeverity) {
  if (severity === "error" || severity === "warning") return "bg-panel-gold";
  if (severity === "success") return "bg-panel-green";
  return "bg-slate-500";
}

function severityIcon(severity: MonitoringSeverity) {
  if (severity === "error") return <AlertCircle aria-hidden="true" className="size-4" />;
  if (severity === "warning") return <AlertTriangle aria-hidden="true" className="size-4" />;
  if (severity === "success") return <CheckCircle2 aria-hidden="true" className="size-4" />;
  return <Info aria-hidden="true" className="size-4" />;
}

function eventTypeLabelKey(type: MonitoringEventType): MessageKey {
  const keys: Record<MonitoringEventType, MessageKey> = {
    all: "filterAll",
    backup: "activityTypeBackup",
    failure: "activityTypeFailure",
    lifecycle: "activityTypeLifecycle",
    mods: "activityTypeMods",
    other: "activityTypeOther",
    player: "activityTypePlayer",
    world: "activityTypeWorld"
  };
  return keys[type];
}

function severityLabelKey(severity: MonitoringSeverityFilter): MessageKey {
  const keys: Record<MonitoringSeverityFilter, MessageKey> = {
    all: "filterAll",
    error: "severityError",
    info: "severityInfo",
    success: "severitySuccess",
    warning: "severityWarning"
  };
  return keys[severity];
}

function filterOptionLabel<T extends string>(
  options: readonly { key: T; labelKey?: MessageKey; label?: string }[],
  value: T,
  t: (key: MessageKey) => string
) {
  const option = options.find((item) => item.key === value);
  return option?.labelKey ? t(option.labelKey) : option?.label ?? value;
}
