"use client";

import ReactECharts from "echarts-for-react";
import type { EChartsOption } from "echarts";
import Link from "next/link";
import { Activity, AlertCircle, AlertTriangle, CheckCircle2, ChevronDown, ExternalLink, Info, RadioTower, Server } from "lucide-react";
import { useState } from "react";
import { Card } from "@/components/ui";
import { useI18n, type MessageKey } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { MetricPoint, MetricSeries, MonitoringEvent, MonitoringOverviewResponse, MonitoringRange, PlatformService, RouteMetric, ServerLoadRow } from "./types";

export function KpiStrip({ overview }: { overview?: MonitoringOverviewResponse }) {
  const { t } = useI18n();
  const kpis = overview?.kpis;
  return (
    <section className="grid gap-3 xl:grid-cols-5">
      <KpiCard label={t("kpiTotalServers")} value={kpis?.totalServers ?? 0} note={t("kpiTotalServersNote")} tone="neutral" />
      <KpiCard label={t("kpiRunning")} value={kpis?.runningServers ?? 0} note={t("kpiRunningNote")} tone="success" />
      <KpiCard label={t("kpiIssues")} value={kpis?.issues ?? 0} note={(kpis?.issues ?? 0) > 0 ? t("kpiIssuesAction") : t("kpiIssuesClear")} tone={(kpis?.issues ?? 0) > 0 ? "warning" : "success"} />
      <KpiCard label={t("kpiOnlinePlayers")} value={kpis?.onlinePlayers ?? 0} note={`capacity ${kpis?.onlinePlayers ?? 0} / ${kpis?.playerCapacity ?? 0}`} tone="neutral" />
      <KpiCard label={t("kpiResourceUsage")} value={`${kpis?.resourceUsagePercent ?? 0}%`} note={t("kpiResourceUsageNote")} tone={(kpis?.resourceUsagePercent ?? 0) > 75 ? "warning" : "success"} />
    </section>
  );
}

function KpiCard({ label, note, tone, value }: { label: string; note: string; tone: "neutral" | "success" | "warning"; value: number | string }) {
  return (
    <Card className="p-4">
      <div className="flex items-center justify-between gap-3">
        <p className="truncate text-xs font-medium text-slate-500">{label}</p>
        <span className={cn("size-2 rounded-full", tone === "success" ? "bg-panel-green" : tone === "warning" ? "bg-panel-gold" : "bg-slate-600")} />
      </div>
      <p className="mt-3 font-mono text-3xl font-semibold leading-none text-slate-100">{value}</p>
      <p className="mt-3 truncate text-xs text-slate-500">{note}</p>
    </Card>
  );
}

export function HealthStatusCard({ overview }: { overview?: MonitoringOverviewResponse }) {
  const { t } = useI18n();
  const health = overview?.health;
  const dataSource = overview?.dataSource;
  const overall = health?.overall ?? "unknown";
  return (
    <Card className="p-4">
      <div className="grid gap-4 xl:grid-cols-[220px_1fr] xl:items-center">
        <div>
          <div className="flex items-center gap-2">
            <span className={cn("size-2.5 rounded-full", overall === "healthy" ? "bg-panel-green" : "bg-panel-gold")} />
            <h2 className="font-semibold text-slate-100">{t("healthOverviewTitle")}</h2>
            <span className={cn("rounded-md border px-2 py-0.5 text-xs font-medium", overall === "healthy" ? "border-panel-green/30 bg-panel-green/10 text-panel-green" : "border-panel-gold/30 bg-panel-gold/10 text-panel-gold")}>
              {t(overall === "healthy" ? "healthHealthy" : overall === "critical" ? "healthCritical" : "healthWarning")}
            </span>
          </div>
          <p className="mt-1 text-xs text-slate-500">{t("healthOverviewDescription")}</p>
        </div>
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-5">
          <HealthMetric label={t("healthOverall")} value={healthStatusLabel(overall, t)} ok={overall === "healthy"} />
          <HealthMetric label={t("healthPrometheusConnected")} value={dataSource?.connected ? t("connected") : t("unavailable")} ok={Boolean(dataSource?.connected)} />
          <HealthMetric label={t("healthDockerRuntime")} value={healthStatusLabel(health?.dockerRuntime ?? "unknown", t)} ok={(health?.dockerRuntime ?? "unknown") === "healthy"} />
          <HealthMetric label={t("healthLastSync")} value={health?.lastSync ? formatTime(health.lastSync) : t("none")} ok={Boolean(health?.lastSync)} />
          <HealthMetric label={t("healthFailedTargets")} value={String(health?.failedTargets ?? 0)} ok={(health?.failedTargets ?? 0) === 0} />
        </div>
      </div>
      {dataSource?.error ? <p className="mt-3 rounded-md border border-panel-gold/25 bg-panel-gold/10 px-3 py-2 text-xs text-panel-gold">{dataSource.error}</p> : null}
    </Card>
  );
}

function healthStatusLabel(value: string, t: (key: MessageKey, params?: Record<string, string | number>) => string) {
  switch (value) {
    case "healthy":
      return t("healthHealthy");
    case "warning":
      return t("healthWarning");
    case "critical":
      return t("healthCritical");
    case "degraded":
      return t("healthDegraded");
    case "down":
      return t("healthDown");
    case "unknown":
    default:
      return t("unknown");
  }
}

function HealthMetric({ label, ok, value }: { label: string; ok: boolean; value: string }) {
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/35 px-3 py-2">
      <p className="truncate text-[11px] text-slate-500">{label}</p>
      <span className="mt-1 inline-flex max-w-full items-center gap-2 text-sm font-medium text-slate-200">
        <span className={cn("size-1.5 shrink-0 rounded-full", ok ? "bg-panel-green" : "bg-panel-gold")} />
        <span className="truncate">{value}</span>
      </span>
    </div>
  );
}

export function MonitoringChartCard({ color = "#59d46f", compact = false, icon, range, series }: { color?: string; compact?: boolean; icon?: React.ReactNode; range?: MonitoringRange; series?: MetricSeries }) {
  const { t } = useI18n();
  const points = series?.points ?? [];
  const unit = series?.unit ?? "";
  const current = series?.currentValue;
  const helperText = emptyText(series?.emptyReason, t);
  return (
    <Card className="p-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            {icon ? <span className="text-panel-green">{icon}</span> : null}
            <h3 className="truncate text-sm font-semibold text-slate-100">{metricTitle(series, t)}</h3>
          </div>
          {helperText ? <p className="mt-1 text-xs text-slate-500">{helperText}</p> : null}
        </div>
        <p className="shrink-0 font-mono text-2xl font-semibold text-slate-100">{current == null ? "—" : formatValue(current, unit)}</p>
      </div>
      <div className={cn("mt-4 rounded-md border border-panel-line bg-slate-950/35 p-2", compact ? "h-44" : "h-56")}>
        {points.length > 0 ? <MetricChart color={color} points={points} range={range} series={series} /> : <EmptyMetric reason={series?.emptyReason} />}
      </div>
      <div className={cn("mt-3 grid grid-cols-4 gap-3 text-xs", compact && "hidden 2xl:grid")}>
        <MetricFoot label={t("metricAvg")} value={series?.avg == null ? "—" : formatValue(series.avg, unit)} />
        <MetricFoot label={t("metricPeak")} value={series?.max == null ? "—" : formatValue(series.max, unit)} />
        <MetricFoot label={t("metricSamples")} value={String(points.length)} />
        <MetricFoot label={t("metricLimit")} value={series?.threshold == null ? "—" : formatValue(series.threshold, unit)} />
      </div>
    </Card>
  );
}

function MetricChart({ color, points, range, series }: { color: string; points: MetricPoint[]; range?: MonitoringRange; series?: MetricSeries }) {
  const unit = series?.unit ?? "";
  const type = series?.chartType === "bar" ? "bar" : "line";
  const data = points.map((point) => [point.timestamp, Number(point.value.toFixed(3))]);
  const yAxisMax = metricYAxisMax(points, series?.threshold);
  const xAxisRange = metricXAxisRange(range);
  const option: EChartsOption = {
    backgroundColor: "transparent",
    animation: false,
    grid: { left: 48, right: 16, top: 18, bottom: 30 },
    tooltip: {
      trigger: "axis",
      axisPointer: { type: "cross", lineStyle: { color: "#64748b", width: 1 } },
      backgroundColor: "#111821",
      borderColor: "#202a36",
      textStyle: { color: "#f4f7fb" },
      valueFormatter: (value) => formatValue(Number(value), unit)
    },
    xAxis: {
      type: "time",
      min: xAxisRange?.min,
      max: xAxisRange?.max,
      axisLabel: { color: "#74839a", hideOverlap: true },
      axisLine: { lineStyle: { color: "#2b3544" } },
      axisTick: { show: false },
      splitLine: { show: true, lineStyle: { color: "rgba(100,116,139,0.14)" } }
    },
    yAxis: {
      type: "value",
      min: 0,
      max: yAxisMax,
      axisLabel: { color: "#74839a", formatter: (value: number) => formatValue(value, unit) },
      splitLine: { lineStyle: { color: "rgba(100,116,139,0.18)" } }
    },
    series: [
      {
        type,
        data,
        smooth: type === "line" ? 0.25 : false,
        symbol: "circle",
        symbolSize: 5,
        showSymbol: false,
        lineStyle: { color, width: 2 },
        itemStyle: { color },
        areaStyle: series?.chartType === "area" ? { color: `${color}24` } : undefined,
        markLine: series?.threshold == null ? undefined : {
          silent: true,
          symbol: "none",
          label: { show: false },
          lineStyle: { color: "#94a3b8", type: "dashed", width: 1.2 },
          data: [{ yAxis: series.threshold }]
        }
      }
    ]
  };
  return <ReactECharts option={option} style={{ height: "100%", width: "100%" }} />;
}

function metricYAxisMax(points: MetricPoint[], threshold?: number) {
  const dataMax = Math.max(0, ...points.map((point) => point.value));
  const thresholdMax = threshold == null ? 0 : threshold;
  const max = Math.max(dataMax, thresholdMax);
  if (max <= 0) return undefined;
  if (threshold != null && threshold >= dataMax) {
    return Number((threshold * 1.05).toFixed(3));
  }
  return Number((max * 1.12).toFixed(3));
}

function metricXAxisRange(range?: MonitoringRange) {
  if (!range?.start || !range.end) return undefined;
  const min = new Date(range.start).getTime();
  const max = new Date(range.end).getTime();
  if (!Number.isFinite(min) || !Number.isFinite(max) || min >= max) return undefined;
  return { min, max };
}

function EmptyMetric({ reason }: { reason?: string }) {
  const { t } = useI18n();
  return <div className="flex h-full items-center justify-center text-sm text-slate-500">{emptyText(reason, t) ?? t("monitoringNoSamples")}</div>;
}

function MetricFoot({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-slate-600">{label}</p>
      <p className="mt-1 truncate font-mono text-slate-300">{value}</p>
    </div>
  );
}

function metricTitle(series: MetricSeries | undefined, t: ReturnType<typeof useI18n>["t"]) {
  const key = series?.key;
  if (key === "cpu") return t("metricTitleCpu");
  if (key === "memory") return t("metricTitleMemory");
  if (key === "players") return t("metricTitlePlayers");
  if (key === "events") return t("metricTitleEvents");
  if (key === "uptime") return t("metricTitleUptime");
  if (key === "requests") return t("metricTitleRequests");
  if (key === "errors") return t("metricTitleErrors");
  if (key === "latencyP95") return t("metricTitleLatencyP95");
  if (key === "sse") return t("metricTitleSse");
  if (key === "nodeCpu") return t("metricTitleNodeCpu");
  if (key === "nodeMemory") return t("metricTitleNodeMemory");
  if (key === "nodeDisk") return t("metricTitleNodeDisk");
  if (key === "nodeNetwork") return t("metricTitleNodeNetwork");
  return series?.title ?? t("monitoringMetricFallbackTitle");
}

export function ServerLoadTable({ rows }: { rows: ServerLoadRow[] }) {
  const { t } = useI18n();
  return (
    <section className="overflow-hidden rounded-lg border border-panel-line bg-slate-950/35">
      <SectionHeader title={t("serverLoadTitle")} description={t("serverLoadDescription")} />
      <div className="hidden grid-cols-[minmax(220px,1fr)_96px_96px_140px_140px_88px_104px_96px] gap-3 border-b border-panel-line px-4 py-2 text-xs font-medium text-slate-500 2xl:grid">
        <span>{t("monitoringTableServer")}</span>
        <span>{t("monitoringTableGame")}</span>
        <span>{t("monitoringTableStatus")}</span>
        <span>{t("monitoringTableCpu")}</span>
        <span>{t("monitoringTableMemory")}</span>
        <span>{t("monitoringTablePlayers")}</span>
        <span>{t("monitoringTableLastActive")}</span>
        <span className="text-right">{t("monitoringTableAction")}</span>
      </div>
      <div className="divide-y divide-panel-line">
        {rows.length > 0 ? rows.map((row) => <ServerLoadItem key={row.serverId} row={row} />) : <EmptyRows label={t("monitoringNoServerLoad")} />}
      </div>
    </section>
  );
}

function ServerLoadItem({ row }: { row: ServerLoadRow }) {
  const { t } = useI18n();
  const memoryPercent = row.memoryLimitMb > 0 ? (row.memoryMb / row.memoryLimitMb) * 100 : 0;
  return (
    <div className={cn("grid gap-3 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_auto] 2xl:grid-cols-[minmax(220px,1fr)_96px_96px_140px_140px_88px_104px_96px] 2xl:items-center", row.severity !== "normal" && "bg-panel-gold/5")}>
      <div className="min-w-0">
        <p className="truncate font-medium text-slate-100">{row.serverName}</p>
        <p className="mt-1 truncate text-xs text-slate-500">{row.providerKey} · {row.version || "default"}</p>
      </div>
      <span className="text-sm text-slate-300 sm:col-start-1 2xl:col-auto">{row.gameKey}</span>
      <StatusBadge status={row.status} severity={row.severity} />
      <LoadBar className="sm:col-span-2 2xl:col-span-1" percent={row.cpuPercent} value={`${row.cpuPercent.toFixed(1)}%`} />
      <LoadBar className="sm:col-span-2 2xl:col-span-1" percent={memoryPercent} value={`${Math.round(row.memoryMb)} MB`} tone="purple" />
      <span className="font-mono text-sm text-slate-300">{row.playersOnline}/{row.maxPlayers}</span>
      <span className="text-sm text-slate-500">{formatTime(row.lastActive)}</span>
      <Link className="inline-flex w-fit items-center gap-1 justify-self-start rounded border border-panel-line px-2.5 py-1.5 text-xs font-medium text-slate-300 transition hover:bg-slate-900 sm:col-start-2 sm:row-start-1 sm:justify-self-end 2xl:col-auto 2xl:row-auto" href={`/servers/${row.serverId}`}>
        {t("view")} <ExternalLink aria-hidden="true" className="size-3" />
      </Link>
    </div>
  );
}

export function ActivityTimeline({ events }: { events: MonitoringEvent[] }) {
  const { t } = useI18n();
  return (
    <Card className="overflow-hidden">
      {events.length === 0 ? <EmptyRows label={t("monitoringNoEvents")} /> : (
        <div>
          <div className="hidden grid-cols-[minmax(320px,1.3fr)_minmax(180px,0.55fr)_minmax(180px,0.55fr)_96px] border-b border-panel-line px-4 py-2 text-xs font-medium text-slate-500 lg:grid">
            <span>{t("eventColumnEvent")}</span>
            <span>{t("eventColumnSource")}</span>
            <span>{t("eventColumnType")}</span>
            <span className="text-right">{t("eventColumnTime")}</span>
          </div>
          <div className="divide-y divide-panel-line">
          {events.map((event) => <EventRow key={event.id} event={event} />)}
          </div>
        </div>
      )}
    </Card>
  );
}

export function ActivityOperationTimeline({ events }: { events: MonitoringEvent[] }) {
  const { t } = useI18n();
  const groups = groupActivityOperations(events);
  const latest = groups[0];
  return (
    <div className="space-y-3">
      {events.length === 0 ? (
        <Card className="px-4 py-12 text-center text-sm text-slate-500">{t("monitoringNoEvents")}</Card>
      ) : null}
      {latest ? <CurrentOperationCard group={latest} /> : null}
      {groups.length > 0 ? (
        <Card className="overflow-hidden">
          <div className="border-b border-panel-line px-4 py-3">
            <h3 className="text-sm font-semibold text-slate-100">{t("serverOperationHistory")}</h3>
            <p className="mt-1 text-xs text-slate-500">{t("serverOperationHistoryDescription")}</p>
          </div>
          <div className="divide-y divide-panel-line">
            {groups.map((group, index) => <OperationGroupRow key={group.id} group={group} defaultOpen={index === 0 && group.severity === "error"} />)}
          </div>
        </Card>
      ) : null}
    </div>
  );
}

type ActivityOperationGroup = {
  id: string;
  events: MonitoringEvent[];
  endTime: number;
  severity: MonitoringEvent["severity"];
  startTime: number;
};

function CurrentOperationCard({ group }: { group: ActivityOperationGroup }) {
  const { t } = useI18n();
  const latest = group.events[group.events.length - 1];
  if (!latest) return null;
  const duration = Math.max(0, group.endTime - group.startTime);
  const visibleEvents = group.events.slice(-8);
  return (
    <Card className={cn("p-4", group.severity === "error" && "border-panel-gold/40 bg-panel-gold/5")}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className={cn("flex size-8 shrink-0 items-center justify-center rounded-md border", toneClass(group.severity))}>{severityIcon(group.severity)}</span>
            <div className="min-w-0">
              <h3 className="truncate text-sm font-semibold text-slate-100">{t("serverCurrentOperation")}</h3>
              <p className="mt-1 truncate text-xs text-slate-500">{localizedEventTitle(latest, t)}</p>
            </div>
          </div>
        </div>
        <span className="rounded-md border border-panel-line bg-slate-950/40 px-2 py-1 font-mono text-xs text-slate-400">
          {formatDuration(duration)}
        </span>
      </div>
      <div className="mt-4 grid gap-2">
        {visibleEvents.map((event, index) => {
          const title = localizedEventTitle(event, t);
          const isLatest = index === visibleEvents.length - 1;
          return (
            <div key={event.id} className="grid grid-cols-[20px_88px_minmax(0,1fr)] items-start gap-2 text-xs">
              <span className={cn("mt-1 size-2 rounded-full", isLatest ? "bg-panel-green shadow-[0_0_0_4px_rgba(89,212,111,0.12)]" : event.severity === "error" ? "bg-panel-gold" : "bg-slate-600")} />
              <span className="font-mono text-slate-500">{formatTime(event.timestamp)}</span>
              <span className={cn("truncate", isLatest ? "font-medium text-slate-100" : "text-slate-400")}>{title}</span>
            </div>
          );
        })}
      </div>
    </Card>
  );
}

function OperationGroupRow({ defaultOpen, group }: { defaultOpen: boolean; group: ActivityOperationGroup }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(defaultOpen);
  const latest = group.events[group.events.length - 1];
  if (!latest) return null;
  const duration = Math.max(0, group.endTime - group.startTime);
  return (
    <div>
      <button
        className="grid w-full gap-3 px-4 py-3 text-left transition hover:bg-slate-950/30 sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:items-center"
        type="button"
        onClick={() => setOpen((current) => !current)}
      >
        <span className="flex min-w-0 items-start gap-3">
          <span className={cn("mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border", toneClass(group.severity))}>{severityIcon(group.severity)}</span>
          <span className="min-w-0">
            <span className="block truncate font-medium text-slate-100">{localizedEventTitle(latest, t)}</span>
            <span className="mt-1 block truncate text-xs text-slate-500">
              {formatTime(new Date(group.startTime).toISOString())} - {formatTime(new Date(group.endTime).toISOString())} · {group.events.length} {t("serverOperationEventCount")} · {formatDuration(duration)}
            </span>
          </span>
        </span>
        <SeverityPill severity={group.severity} />
        <ChevronDown aria-hidden="true" className={cn("size-4 text-slate-500 transition", open && "rotate-180")} />
      </button>
      {open ? (
        <div className="border-t border-panel-line bg-slate-950/25 px-4 py-2">
          {group.events.map((event) => <CompactEventRow key={event.id} event={event} />)}
        </div>
      ) : null}
    </div>
  );
}

function CompactEventRow({ event }: { event: MonitoringEvent }) {
  const { t } = useI18n();
  return (
    <div className="grid grid-cols-[96px_20px_minmax(0,1fr)] items-start gap-2 py-2 text-xs">
      <span className="font-mono text-slate-500">{formatTime(event.timestamp)}</span>
      <span className={cn("mt-1 size-2 rounded-full", event.severity === "error" ? "bg-panel-gold" : event.severity === "success" ? "bg-panel-green" : "bg-slate-600")} />
      <span className="min-w-0">
        <span className="block truncate font-medium text-slate-300">{localizedEventTitle(event, t)}</span>
        <span className="mt-0.5 block truncate text-slate-600">{event.message}</span>
      </span>
    </div>
  );
}

function groupActivityOperations(events: MonitoringEvent[]): ActivityOperationGroup[] {
  const map = new Map<string, MonitoringEvent[]>();
  for (const event of events) {
    const operationId = event.metadata?.operationId || event.id;
    const list = map.get(operationId) ?? [];
    list.push(event);
    map.set(operationId, list);
  }
  return Array.from(map.entries()).map(([id, items]) => {
    const sorted = [...items].sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());
    const times = sorted.map((event) => new Date(event.timestamp).getTime()).filter((value) => !Number.isNaN(value));
    const severity: MonitoringEvent["severity"] = sorted.some((event) => event.severity === "error") ? "error" : sorted.some((event) => event.severity === "warning") ? "warning" : sorted.some((event) => event.severity === "success") ? "success" : "info";
    return {
      id,
      events: sorted,
      severity,
      startTime: times[0] ?? 0,
      endTime: times[times.length - 1] ?? 0
    };
  }).sort((a, b) => b.endTime - a.endTime);
}

function EventRow({ event }: { event: MonitoringEvent }) {
  const { t } = useI18n();
  const severity = event.severity === "error" ? "critical" : event.severity === "warning" ? "warning" : "normal";
  const title = localizedEventTitle(event, t);
  const typeLabel = localizedEventGroup(event.type, t);
  const operator = event.operator === "system" ? t("eventSourceSystem") : event.operator;
  const source = event.serverName || event.serverId || t("eventSourceSystem");
  const eventTime = formatTime(event.timestamp);
  return (
    <div className={cn("grid gap-3 px-4 py-3 lg:grid-cols-[minmax(320px,1.3fr)_minmax(180px,0.55fr)_minmax(150px,0.45fr)_112px] lg:items-center", event.severity === "error" && "bg-panel-gold/5")}>
      <div className="flex min-w-0 items-start gap-3">
        <span className={cn("mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border", toneClass(event.severity))}>{severityIcon(event.severity)}</span>
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <p className="truncate font-medium text-slate-100">{title}</p>
            <SeverityPill severity={event.severity} />
          </div>
          <p className="mt-1 line-clamp-1 text-xs text-slate-500">
            <span className="font-mono text-slate-400">{eventTime}</span>
            <span className="px-1.5 text-slate-700">·</span>
            {t("eventMessageServer", { server: source, event: title })}
          </p>
        </div>
      </div>
      <div className="min-w-0 text-sm">
        {event.serverId ? (
          <Link className="block truncate font-medium text-slate-300 transition hover:text-panel-green" href={`/servers/${event.serverId}`}>{source}</Link>
        ) : (
          <span className="block truncate font-medium text-slate-300">{t("eventSourceSystem")}</span>
        )}
        <span className="mt-1 block truncate text-xs text-slate-500">{operator}</span>
      </div>
      <div className="flex min-w-0 items-center gap-2">
        <StatusBadge status={typeLabel} severity={severity} />
      </div>
      <span className="font-mono text-xs text-slate-500 lg:text-right">{eventTime}</span>
    </div>
  );
}

const eventTitleKeys: Record<string, MessageKey> = {
  "backup.created": "eventTitleBackupCreated",
  "backup.deleted": "eventTitleBackupDeleted",
  "backup.restored": "eventTitleBackupRestored",
  "mod.assigned": "eventTitleModAssigned",
  "mod.deleted": "eventTitleModDeleted",
  "mod.updated": "eventTitleModUpdated",
  "mod.uploaded": "eventTitleModUploaded",
  "mod.workshop_imported": "eventTitleModImported",
  "player.banned": "eventTitlePlayerBanned",
  "player.kicked": "eventTitlePlayerKicked",
  "player.whitelist.removed": "eventTitlePlayerWhitelistRemoved",
  "player.whitelisted": "eventTitlePlayerWhitelisted",
  "save.snapshot.created": "eventTitleWorldSnapshotCreated",
  "save.snapshot.restored": "eventTitleBackupRestored",
  "server.config.updated": "eventTitleServerConfigUpdated",
  "server.container.create.failed": "eventTitleServerContainerCreateFailed",
  "server.container.create.started": "eventTitleServerContainerCreateStarted",
  "server.container.create.succeeded": "eventTitleServerContainerCreateSucceeded",
  "server.container.inspect.failed": "eventTitleServerContainerInspectFailed",
  "server.container.remove.failed": "eventTitleServerContainerRemoveFailed",
  "server.container.remove.started": "eventTitleServerContainerRemoveStarted",
  "server.container.remove.succeeded": "eventTitleServerContainerRemoveSucceeded",
  "server.container.start.failed": "eventTitleServerContainerStartFailed",
  "server.container.start.started": "eventTitleServerContainerStartStarted",
  "server.container.start.succeeded": "eventTitleServerContainerStartSucceeded",
  "server.container.stop.failed": "eventTitleServerContainerStopFailed",
  "server.container.stop.started": "eventTitleServerContainerStopStarted",
  "server.container.stop.succeeded": "eventTitleServerContainerStopSucceeded",
  "server.created": "eventTitleServerCreated",
  "server.delete.queued": "eventTitleServerDeleteQueued",
  "server.deleted": "eventTitleServerDeleted",
  "server.image.load.failed": "eventTitleServerImageLoadFailed",
  "server.image.load.started": "eventTitleServerImageLoadStarted",
  "server.image.load.succeeded": "eventTitleServerImageLoadSucceeded",
  "server.reconcile.failed": "eventTitleServerReconcileFailed",
  "server.restart.failed": "eventTitleServerRestartFailed",
  "server.restart.container.created": "eventTitleServerContainerCreated",
  "server.restart.container.prepare": "eventTitleServerContainerPreparing",
  "server.restart.container.ready": "eventTitleServerContainerReady",
  "server.restart.runtime.starting": "eventTitleServerRuntimeStarting",
  "server.restart.queued": "eventTitleServerRestartQueued",
  "server.restarted": "eventTitleServerRestarted",
  "server.runtime.created": "eventTitleServerRuntimeCreated",
  "server.runtime.removed": "eventTitleServerRuntimeRemoved",
  "server.share.disabled": "eventTitleShareDisabled",
  "server.share.enabled": "eventTitleShareEnabled",
  "server.start.failed": "eventTitleServerStartFailed",
  "server.start.container.created": "eventTitleServerContainerCreated",
  "server.start.container.prepare": "eventTitleServerContainerPreparing",
  "server.start.container.ready": "eventTitleServerContainerReady",
  "server.start.runtime.starting": "eventTitleServerRuntimeStarting",
  "server.start.queued": "eventTitleServerStartQueued",
  "server.started": "eventTitleServerStarted",
  "server.stop.queued": "eventTitleServerStopQueued",
  "server.stopped": "eventTitleServerStopped",
  "settings.locale": "eventTitleSettingsLocale",
  "settings.publicHost": "eventTitleSettingsUpdated",
  "world.assigned": "eventTitleWorldAssigned",
  "world.deleted": "eventTitleWorldDeleted",
  "world.imported": "eventTitleWorldImported",
  "world.snapshot.created": "eventTitleWorldSnapshotCreated"
};

function localizedEventTitle(event: MonitoringEvent, t: ReturnType<typeof useI18n>["t"]) {
  const key = eventTitleKeys[event.type];
  return key ? t(key) : event.title;
}

function localizedEventGroup(type: string, t: ReturnType<typeof useI18n>["t"]) {
  if (type.startsWith("server.")) return t("eventGroupServer");
  if (type.startsWith("backup.") || type.startsWith("save.")) return t("eventGroupBackup");
  if (type.startsWith("world.")) return t("eventGroupWorld");
  if (type.startsWith("mod.")) return t("eventGroupMod");
  if (type.startsWith("player.")) return t("eventGroupPlayer");
  if (type.startsWith("settings.")) return t("eventGroupSettings");
  return t("eventGroupSystem");
}

function SeverityPill({ severity }: { severity: MonitoringEvent["severity"] }) {
  return (
    <span className={cn(
      "rounded px-1.5 py-0.5 text-[11px] font-medium",
      severity === "error" ? "bg-panel-gold/15 text-panel-gold" :
        severity === "warning" ? "bg-panel-gold/10 text-panel-gold" :
          severity === "success" ? "bg-panel-green/10 text-panel-green" : "bg-slate-800 text-slate-400"
    )}>
      {severity}
    </span>
  );
}

export function PlatformHealth({ services, topRoutes }: { services: PlatformService[]; topRoutes: RouteMetric[] }) {
  const { t } = useI18n();
  return (
    <section className="grid gap-4 xl:grid-cols-[1fr_1fr]">
      <Card className="p-4">
        <SectionTitle title={t("platformStatusTitle")} description={t("platformStatusDescription")} />
        <div className="mt-4 grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
          {services.map((service, index) => <HealthMetric key={`${service.name}-${service.instance ?? index}`} label={service.name} value={service.status} ok={service.status === "healthy"} />)}
        </div>
      </Card>
      <Card className="p-4">
        <SectionTitle title={t("topRoutesTitle")} description={t("topRoutesDescription")} />
        <div className="mt-4 divide-y divide-panel-line">
          {topRoutes.length > 0 ? topRoutes.map((route) => (
            <div key={`${route.method}-${route.route}`} className="grid grid-cols-[64px_minmax(0,1fr)_112px] gap-3 py-2.5 text-sm">
              <span className="font-mono text-slate-500">{route.method}</span>
              <span className="truncate text-slate-300">{route.route}</span>
              <span className="text-right">
                <span className="block font-mono text-slate-100">{formatRouteRequestCount(route.requestCount)} {t("topRoutesCountUnit")}</span>
                <span className="block font-mono text-[11px] text-slate-500">{route.requestRate.toFixed(2)} {t("topRoutesRateUnit")}</span>
              </span>
            </div>
          )) : <EmptyRows label={t("topRoutesEmpty")} />}
        </div>
      </Card>
    </section>
  );
}

function formatRouteRequestCount(value: number) {
  if (!Number.isFinite(value)) return "0";
  return Math.max(0, Math.round(value)).toLocaleString();
}

function SectionHeader({ description, title }: { description: string; title: string }) {
  return <div className="border-b border-panel-line px-4 py-3"><h2 className="text-sm font-semibold text-slate-100">{title}</h2><p className="mt-1 text-xs text-slate-500">{description}</p></div>;
}

function SectionTitle({ description, title }: { description: string; title: string }) {
  return <div><h2 className="text-sm font-semibold text-slate-100">{title}</h2><p className="mt-1 text-xs text-slate-500">{description}</p></div>;
}

function EmptyRows({ label }: { label: string }) {
  return <div className="flex min-h-28 items-center justify-center p-6 text-sm text-slate-500">{label}</div>;
}

function StatusBadge({ severity, status }: { severity: "normal" | "warning" | "critical"; status: string }) {
  return <span className={cn("w-fit rounded px-2 py-0.5 text-xs font-medium", severity === "critical" ? "bg-panel-gold/15 text-panel-gold" : severity === "warning" ? "bg-panel-gold/10 text-panel-gold" : "bg-slate-800 text-slate-400")}>{status}</span>;
}

function LoadBar({ className, percent, tone = "green", value }: { className?: string; percent: number; tone?: "green" | "purple"; value: string }) {
  return (
    <div className={className}>
      <div className="mb-1 flex justify-between gap-2 text-xs"><span className="font-mono text-slate-300">{value}</span></div>
      <div className="h-1.5 overflow-hidden rounded-full bg-slate-800">
        <div className={cn("h-full rounded-full", tone === "purple" ? "bg-panel-purple" : percent > 80 ? "bg-panel-gold" : "bg-panel-green")} style={{ width: `${Math.max(3, Math.min(percent, 100))}%` }} />
      </div>
    </div>
  );
}

function toneClass(tone: MonitoringEvent["severity"]) {
  if (tone === "error" || tone === "warning") return "border-panel-gold/30 bg-panel-gold/10 text-panel-gold";
  if (tone === "success") return "border-panel-green/30 bg-panel-green/10 text-panel-green";
  return "border-panel-line bg-slate-900 text-slate-400";
}

function severityIcon(severity: MonitoringEvent["severity"]) {
  if (severity === "error") return <AlertCircle aria-hidden="true" className="size-4" />;
  if (severity === "warning") return <AlertTriangle aria-hidden="true" className="size-4" />;
  if (severity === "success") return <CheckCircle2 aria-hidden="true" className="size-4" />;
  return <Info aria-hidden="true" className="size-4" />;
}

function emptyText(reason: string | undefined, t: ReturnType<typeof useI18n>["t"]) {
  if (reason === "prometheus_unconfigured") return t("monitoringPrometheusUnconfigured");
  if (reason === "prometheus_unavailable") return t("monitoringPrometheusUnavailable");
  if (reason === "no_active_samples") return t("monitoringNoActiveSamples");
  if (reason === "server_stopped") return t("monitoringServerStopped");
  if (reason === "no_samples") return t("monitoringNoSamples");
  return undefined;
}

function formatValue(value: number, unit: string) {
  const rounded = Math.abs(value) >= 100 ? Math.round(value) : Number(value.toFixed(1));
  if (unit === "%") return `${rounded}%`;
  if (unit === "MB") return `${rounded} MB`;
  if (unit === "ms") return `${rounded} ms`;
  if (unit === "s") return formatDuration(Number(value.toFixed(0)));
  return unit ? `${rounded} ${unit}` : String(rounded);
}

function formatDuration(value: number) {
  if (value < 1000) return `${value}ms`;
  const seconds = value / 1000;
  if (seconds < 60) return `${seconds.toFixed(seconds < 10 ? 1 : 0)}s`;
  const minutes = Math.floor(seconds / 60);
  const rest = Math.round(seconds % 60);
  return `${minutes}m ${rest}s`;
}

function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  const base = date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  return `${base}.${String(date.getMilliseconds()).padStart(3, "0")}`;
}

export function SourceBadge({ connected }: { connected?: boolean }) {
  const { t } = useI18n();
  return (
    <span className="inline-flex h-9 items-center gap-2 rounded-md border border-panel-line bg-slate-900/55 px-3 text-xs">
      <RadioTower aria-hidden="true" className={cn("size-3.5", connected ? "text-panel-green" : "text-panel-gold")} />
      <span className="text-slate-500">{t("monitoringDatasource")}</span>
      <span className="font-mono font-medium text-slate-200">{connected ? t("connected") : t("unavailable")}</span>
    </span>
  );
}

export function ChartIcon({ type }: { type: "cpu" | "memory" | "players" | "events" | "requests" }) {
  if (type === "events") return <Activity aria-hidden="true" className="size-4" />;
  if (type === "players") return <Server aria-hidden="true" className="size-4" />;
  return <RadioTower aria-hidden="true" className="size-4" />;
}
