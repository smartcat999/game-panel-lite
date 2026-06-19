"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import {
  Activity,
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Cpu,
  ExternalLink,
  Gauge,
  Info,
  MemoryStick,
  RadioTower,
  Search,
  Server,
  Users
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { PageHeader } from "@/components/page-header";
import { Card } from "@/components/ui";
import { MonitoringChartCard } from "@/components/monitoring-chart-card";
import { getObservabilityMetrics, listActivity, listGames, listServers, prometheusMetricsUrl } from "@/lib/api";
import { formatActivityEvent } from "@/lib/activity-display";
import { gameFilterOptions } from "@/lib/game-filters";
import {
  createMonitoringModel,
  eventTypeOptions,
  severityOptions,
  summarizeTimeSeries,
  type MonitoringEvent,
  type MonitoringEventType,
  type MonitoringHealth,
  type MonitoringServerRow,
  type MonitoringSeverity,
  type MonitoringSeverityFilter,
  type MonitoringTimeSeriesPoint
} from "@/lib/monitoring";
import { cn } from "@/lib/utils";
import { useI18n, type MessageKey } from "@/lib/i18n";

type ActivityGameFilter = "all" | string;

const SERIES_WINDOW_MS = 15 * 60 * 1000;

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
  const [chartPhase, setChartPhase] = useState(0);

  useEffect(() => {
    const timer = window.setInterval(() => setChartPhase((phase) => phase + 1), 5000);
    return () => window.clearInterval(timer);
  }, []);

  const events = activityQuery.data ?? [];
  const servers = serversQuery.data ?? [];
  const games = gamesQuery.data ?? [];
  const displayByEventId = useMemo(
    () => new Map(events.map((event) => [event.id, formatActivityEvent(event, locale)])),
    [events, locale]
  );
  const monitoring = useMemo(() => {
    return createMonitoringModel({
      activity: events,
      displayByEventId,
      games,
      metrics: metricsQuery.data,
      servers
    });
  }, [displayByEventId, events, games, metricsQuery.data, servers]);

  const cpuSeries = useSampledTimeSeries(monitoring.trends.cpuPercent, Boolean(metricsQuery.data), chartPhase);
  const memorySeries = useSampledTimeSeries(monitoring.trends.memoryMb, Boolean(metricsQuery.data), chartPhase);
  const playerSeries = useSampledTimeSeries(monitoring.trends.playerCount, Boolean(metricsQuery.data || serversQuery.data), chartPhase);
  const eventSeries = useSampledTimeSeries(monitoring.trends.eventCount, Boolean(activityQuery.data), chartPhase);
  const cpuSummary = useMemo(() => summarizeTimeSeries(cpuSeries), [cpuSeries]);
  const memorySummary = useMemo(() => summarizeTimeSeries(memorySeries), [memorySeries]);
  const playerSummary = useMemo(() => summarizeTimeSeries(playerSeries), [playerSeries]);
  const eventSummary = useMemo(() => summarizeTimeSeries(eventSeries), [eventSeries]);
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

      <section className="mb-5">
        <HealthOverview health={monitoring.health} />
        <div className="mt-4 grid gap-4 md:grid-cols-2">
          <MonitoringChartCard
            chartType="line"
            color="#7bd978"
            currentValue={`${monitoring.trends.cpuPercent.toFixed(1)}%`}
            data={cpuSeries}
            emptyLabel={t("monitoringNoCpuData")}
            icon={<Cpu aria-hidden="true" className="size-4" />}
            limitLabel={t("chartCpuLimit")}
            subtitle={t("trendCpuNote")}
            summary={cpuSummary}
            threshold={80}
            title={t("trendCpuTitle")}
            unit="%"
          />
          <MonitoringChartCard
            chartType="line"
            color="#a78bfa"
            currentValue={`${Math.round(monitoring.trends.memoryMb)} MB`}
            data={memorySeries}
            emptyLabel={t("monitoringNoMemoryData")}
            icon={<MemoryStick aria-hidden="true" className="size-4" />}
            limitLabel={t("chartMemoryLimit", { limit: `${Math.round(monitoring.trends.memoryLimitMb)} MB` })}
            subtitle={t("trendMemoryNote")}
            summary={memorySummary}
            threshold={monitoring.trends.memoryLimitMb}
            title={t("trendMemoryTitle")}
            unit="MB"
          />
          <MonitoringChartCard
            chartType="line"
            color="#7bd978"
            currentValue={`${monitoring.trends.playerCount}`}
            data={playerSeries}
            emptyLabel={t("monitoringNoPlayerData")}
            icon={<Users aria-hidden="true" className="size-4" />}
            limitLabel={t("chartPlayerCapacity", { capacity: monitoring.kpis.playerCapacity })}
            subtitle={t("trendPlayersNote")}
            summary={playerSummary}
            threshold={monitoring.kpis.playerCapacity}
            title={t("trendPlayersTitle")}
            unit=""
          />
          <MonitoringChartCard
            chartType="bar"
            color="#f4c95d"
            currentValue={`${monitoring.trends.eventCount}`}
            data={eventSeries}
            emptyLabel={t("monitoringNoEventData")}
            icon={<Activity aria-hidden="true" className="size-4" />}
            limitLabel={t("chartEventScale", { limit: Math.max(10, monitoring.trends.eventCount) })}
            subtitle={t("trendEventsNote")}
            summary={eventSummary}
            threshold={Math.max(10, monitoring.trends.eventCount)}
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

function useSampledTimeSeries(value: number, enabled: boolean, tick: number): MonitoringTimeSeriesPoint[] {
  const [series, setSeries] = useState<MonitoringTimeSeriesPoint[]>([]);

  useEffect(() => {
    if (!enabled) {
      setSeries([]);
      return;
    }
    const now = Date.now();
    const point = { timestamp: new Date(now).toISOString(), value };
    setSeries((previous) => {
      const next = [...previous, point].filter((item) => new Date(item.timestamp).getTime() >= now - SERIES_WINDOW_MS);
      const deduped = next.filter((item, index) => index === next.length - 1 || item.timestamp !== next[index + 1]?.timestamp);
      return deduped;
    });
  }, [enabled, tick, value]);

  return series;
}

function KpiCard({ icon, label, note, tone = "neutral", value }: { icon: React.ReactNode; label: string; note: string; tone?: "neutral" | "success" | "warning"; value: string | number }) {
  return (
    <Card className="p-4">
      <div className="flex items-center gap-2">
        <span className={cn("flex size-6 shrink-0 items-center justify-center rounded border", toneClass(tone))}>{icon}</span>
        <p className="min-w-0 truncate text-xs font-medium text-slate-500">{label}</p>
      </div>
      <p className="mt-3 font-mono text-3xl font-semibold leading-none text-slate-100">{value}</p>
      <p className="mt-3 min-h-4 truncate text-xs text-slate-500">{note}</p>
    </Card>
  );
}

function HealthOverview({ health }: { health: MonitoringHealth }) {
  const { t } = useI18n();
  return (
    <Card className="p-4">
      <div className="grid gap-4 xl:grid-cols-[minmax(220px,0.9fr)_minmax(0,2.8fr)_minmax(240px,0.8fr)] xl:items-center">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className={cn("size-2.5 rounded-full", health.overall === "healthy" ? "bg-panel-green" : "bg-panel-gold")} />
            <h2 className="font-semibold text-slate-100">{t("healthOverviewTitle")}</h2>
            <span className={cn("rounded-md border px-2 py-0.5 text-xs font-medium", health.overall === "healthy" ? "border-panel-green/30 bg-panel-green/10 text-panel-green" : "border-panel-gold/30 bg-panel-gold/10 text-panel-gold")}>
              {t(health.overall === "healthy" ? "healthHealthy" : health.overall === "critical" ? "healthCritical" : "healthWarning")}
            </span>
          </div>
          <p className="mt-1 text-xs text-slate-500">{t("healthOverviewDescription")}</p>
        </div>
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-5">
          <HealthMetric label={t("healthOverall")} value={t(health.overall === "healthy" ? "healthHealthy" : health.overall === "critical" ? "healthCritical" : "healthWarning")} severity={health.overall === "healthy" ? "success" : "warning"} />
          <HealthMetric label={t("healthPrometheusConnected")} value={health.prometheusConnected ? t("connected") : t("unavailable")} severity={health.prometheusConnected ? "success" : "warning"} />
          <HealthMetric label={t("healthDockerRuntime")} value={t(health.dockerRuntime === "healthy" ? "available" : health.dockerRuntime === "degraded" ? "healthDegraded" : "unavailable")} severity={health.dockerRuntime === "healthy" ? "success" : "warning"} />
          <HealthMetric label={t("healthLastSync")} value={health.lastSyncLabel} severity="info" />
          <HealthMetric label={t("healthFailedTargets")} value={String(health.failedTargets)} severity={health.failedTargets > 0 ? "warning" : "success"} />
        </div>
        <div className="grid grid-cols-2 gap-2 rounded-md border border-panel-line bg-slate-950/45 p-2 text-xs text-slate-500">
          <HealthMeta label={t("monitoringDatasource")} value="Prometheus" />
          <HealthMeta label={t("monitoringEndpoint")} value="/metrics" />
        </div>
      </div>
    </Card>
  );
}

function HealthMetric({ label, severity, value }: { label: string; severity: MonitoringSeverity; value: string }) {
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/35 px-3 py-2">
      <p className="truncate text-[11px] text-slate-500">{label}</p>
      <span className="mt-1 inline-flex max-w-full items-center gap-2 text-sm font-medium text-slate-200">
        <span className={cn("size-1.5 shrink-0 rounded-full", dotClass(severity))} />
        <span className="truncate">{value}</span>
      </span>
    </div>
  );
}

function HealthMeta({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="truncate text-[11px] text-slate-600">{label}</p>
      <p className="mt-1 truncate font-mono text-slate-300">{value}</p>
    </div>
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
