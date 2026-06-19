"use client";

import { useQuery } from "@tanstack/react-query";
import { Activity as ActivityIcon, AlertTriangle, Cpu, MemoryStick, RadioTower, Server, Users } from "lucide-react";
import { useMemo, useState } from "react";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { PageHeader } from "@/components/page-header";
import { Card } from "@/components/ui";
import { getObservabilityMetrics, listActivity, listGames, listServers, prometheusMetricsUrl, type ObservabilityServerMetric } from "@/lib/api";
import { formatActivityEvent } from "@/lib/activity-display";
import { gameFilterOptions } from "@/lib/game-filters";
import { localizeRelativeTime, useI18n, type MessageKey } from "@/lib/i18n";
import { useTimeSeries, type SeriesPoint } from "@/lib/sparkline";
import { cn } from "@/lib/utils";

type ActivityGameFilter = "all" | string;

export default function ActivityPage() {
  const { locale, t } = useI18n();
  const query = useQuery({ queryKey: ["activity"], queryFn: listActivity, retry: false });
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false, staleTime: 5 * 60 * 1000 });
  const metricsQuery = useQuery({ queryKey: ["observability-metrics"], queryFn: getObservabilityMetrics, retry: false, refetchInterval: 5000 });
  const [search, setSearch] = useState("");
  const [gameFilter, setGameFilter] = useState<ActivityGameFilter>("all");
  const events = query.data ?? [];
  const servers = serversQuery.data ?? [];
  const metrics = metricsQuery.data;
  const host = metrics?.host;
  const cpuSeries = useTimeSeries(host?.totalCpuPercent, 60);
  const memorySeries = useTimeSeries(host?.totalMemoryMb, 60);
  const totalPlayers = metrics?.servers.reduce((sum, server) => sum + server.playersOnline, 0) ?? 0;
  const playerCapacity = metrics?.servers.reduce((sum, server) => sum + server.maxPlayers, 0) ?? 0;
  const runningServers = metrics?.servers.filter((server) => server.status === "running").length ?? 0;
  const totalServers = metrics?.servers.length ?? 0;
  const memoryLimit = Math.max(1024, host?.memoryLimitMb ?? 1024);
  const topServers = (metrics?.servers ?? []).slice(0, 5);
  const serverById = useMemo(() => new Map(servers.map((server) => [server.id, server])), [servers]);
  const serverNameById = useMemo(() => new Map(servers.map((server) => [server.id, server.name])), [servers]);
  const gameFilters = useMemo(
    () => gameFilterOptions(gamesQuery.data ?? [], t("filterAll"), events.map((event) => event.instanceId ? serverById.get(event.instanceId)?.gameKey : undefined)),
    [events, gamesQuery.data, serverById, t]
  );
  const filteredEvents = useMemo(() => {
    const term = search.trim().toLowerCase();
    return events.filter((event) => {
      const server = event.instanceId ? serverById.get(event.instanceId) : undefined;
      const matchesGame = gameFilter === "all" || server?.gameKey === gameFilter;
      if (!matchesGame) return false;
      if (!term) return true;
      const serverName = event.instanceId ? serverNameById.get(event.instanceId) ?? event.instanceId : "";
      return [event.message, event.type, serverName].some((value) => value.toLowerCase().includes(term));
    });
  }, [events, gameFilter, search, serverById, serverNameById]);
  const activeFilterChips = [
    search.trim(),
    gameFilter !== "all" ? filterOptionLabel(gameFilters, gameFilter, t) : ""
  ].filter(Boolean);
  return (
    <>
      <PageHeader
        title={t("activityTitle")}
        description={t("activityObservabilityDescription")}
        action={
          <a
            className="inline-flex h-10 items-center justify-center gap-2 rounded-md border border-panel-line bg-slate-900/70 px-3 text-sm font-medium text-slate-100 transition hover:bg-slate-800 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
            href={prometheusMetricsUrl()}
            rel="noreferrer"
            target="_blank"
          >
            <RadioTower aria-hidden="true" className="size-4" />
            {t("prometheusEndpoint")}
          </a>
        }
      />
      {(query.isError || serversQuery.isError || gamesQuery.isError || metricsQuery.isError) && <p className="mb-4 text-sm text-panel-gold">{t("apiActivityUnavailable")}</p>}

      <section className="mb-5 overflow-hidden rounded-md border border-panel-line bg-[#090d13]">
        <div className="flex flex-col gap-3 border-b border-panel-line bg-[#0b111a] px-4 py-3 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <div className="flex items-center gap-2">
              <span className={cn("size-2 rounded-full", metricsQuery.isError ? "bg-panel-gold" : "bg-panel-green")} />
              <h2 className="text-sm font-semibold text-slate-100">{t("metricsOverviewTitle")}</h2>
              <span className="rounded border border-panel-line px-1.5 py-0.5 font-mono text-[11px] text-slate-500">GamePanel</span>
            </div>
            <p className="mt-1 text-xs text-slate-500">{t("monitoringDashboardDescription")}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <DashboardBadge label={t("monitoringDatasource")} value="Prometheus" />
            <DashboardBadge label={t("monitoringRange")} value="15m" />
            <DashboardBadge label={t("monitoringRefresh")} value="5s" />
            <a
              className="inline-flex h-8 items-center gap-2 rounded border border-panel-line bg-slate-950/50 px-2.5 text-xs font-medium text-slate-300 transition hover:bg-slate-900"
              href={prometheusMetricsUrl()}
              rel="noreferrer"
              target="_blank"
            >
              <RadioTower aria-hidden="true" className="size-3.5 text-panel-green" />
              /metrics
            </a>
          </div>
        </div>

        <div className="grid border-b border-panel-line sm:grid-cols-2 xl:grid-cols-4">
          <StatCell icon={<Server aria-hidden="true" className="size-4" />} label={t("metricRunningServers")} value={`${runningServers}/${totalServers}`} />
          <StatCell icon={<Users aria-hidden="true" className="size-4" />} label={t("metricOnlinePlayers")} value={`${totalPlayers}/${playerCapacity}`} />
          <StatCell icon={<RadioTower aria-hidden="true" className="size-4" />} label="Docker containers" value={`${host?.runningContainers ?? 0}`} />
          <StatCell icon={<AlertTriangle aria-hidden="true" className="size-4" />} label={t("activityFailures")} tone="gold" value={`${metrics?.activity.failures ?? 0}`} />
        </div>

        <div className="grid gap-px bg-panel-line xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_360px]">
          <TimeSeriesPanel
            color="#7bd978"
            current={host?.totalCpuPercent}
            icon={<Cpu aria-hidden="true" className="size-4" />}
            max={100}
            metric="gamepanel_runtime_cpu_percent"
            series={cpuSeries}
            title={t("metricRuntimeCpu")}
            unit="%"
          />
          <TimeSeriesPanel
            color="#a78bfa"
            current={host?.totalMemoryMb}
            icon={<MemoryStick aria-hidden="true" className="size-4" />}
            max={memoryLimit}
            metric="gamepanel_runtime_memory_bytes"
            series={memorySeries}
            subtitle={host?.memoryLimitMb ? t("metricMemoryLimit", { limit: `${host.memoryLimitMb} MB` }) : t("metricNoLimit")}
            title={t("metricRuntimeMemory")}
            unit="MB"
          />
          <EventsPanel items={metrics?.activity.byType ?? []} total={metrics?.activity.total ?? 0} windowHours={metrics?.activity.windowHours ?? 24} />
        </div>
      </section>

      <section className="mb-5 overflow-hidden rounded-md border border-panel-line bg-[#090d13]">
        <div className="flex items-center justify-between border-b border-panel-line bg-[#0b111a] px-4 py-3">
          <div>
          <h2 className="text-sm font-semibold text-slate-100">{t("serverLoadTitle")}</h2>
          <p className="mt-1 text-xs text-slate-500">{t("serverLoadDescription")}</p>
          </div>
          <span className="font-mono text-xs text-slate-500">gamepanel_server_*</span>
        </div>
        <div className="hidden grid-cols-[minmax(0,1fr)_110px_150px_150px_110px] border-b border-panel-line px-4 py-2 text-xs text-slate-500 lg:grid">
          <span>target</span>
          <span>state</span>
          <span>cpu</span>
          <span>memory</span>
          <span>players</span>
        </div>
        <div className="divide-y divide-panel-line">
          {topServers.length > 0 ? (
            topServers.map((server) => <ServerLoadRow key={server.id} server={server} />)
          ) : (
            <div className="p-4 text-sm text-slate-400">{metricsQuery.isLoading ? t("loading") : t("noServersYet")}</div>
          )}
        </div>
      </section>

      <ResourceFilterBar
        activeChips={activeFilterChips}
        clearLabel={t("clearFilters")}
        density="compact"
        filters={[
          { label: t("filterGame"), options: gameFilters, value: gameFilter, onChange: (value) => setGameFilter(value) }
        ]}
        onClear={() => {
          setGameFilter("all");
          setSearch("");
        }}
        onSearchChange={setSearch}
        resultLabel={t("filteredResultsCount", { count: filteredEvents.length })}
        search={search}
        searchPlaceholder={t("searchActivity")}
      />
      <Card className="overflow-hidden">
        {filteredEvents.length === 0 ? (
          <div className="flex min-h-48 items-center justify-center p-6 text-center text-sm text-slate-400">
            {query.isLoading ? t("loading") : events.length === 0 ? t("noActivityYet") : t("noResultsMatchFilters")}
          </div>
        ) : (
          <div className="divide-y divide-panel-line">
            {filteredEvents.map((event) => {
              const display = formatActivityEvent(event, locale);
              const server = event.instanceId ? serverById.get(event.instanceId) : undefined;
              return (
                <div key={event.id} className="flex flex-col gap-3 p-4 sm:flex-row sm:items-start sm:justify-between">
                  <div className="flex min-w-0 items-start gap-3">
                    <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-panel-green/15 text-panel-green">
                      <ActivityIcon aria-hidden="true" className="size-5" />
                    </span>
                    <div className="min-w-0">
                      <p className="font-medium text-white">{display.message}</p>
                      <p className="mt-1 text-xs text-slate-500">{server?.name ?? event.instanceId ?? t("none")}</p>
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-wrap gap-2 text-xs text-slate-400">
                    <span className="rounded bg-slate-800 px-2 py-1">{display.typeLabel}</span>
                    <span className="rounded bg-slate-800 px-2 py-1">{localizeRelativeTime(event.created, locale)}</span>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Card>
    </>
  );
}

function DashboardBadge({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex h-8 items-center gap-2 rounded border border-panel-line bg-slate-950/50 px-2.5 text-xs">
      <span className="text-slate-500">{label}</span>
      <span className="font-mono text-slate-200">{value}</span>
    </span>
  );
}

function StatCell({ icon, label, tone = "green", value }: { icon: React.ReactNode; label: string; tone?: "green" | "gold"; value: string }) {
  return (
    <div className="border-b border-panel-line px-4 py-3 sm:border-r sm:last:border-r-0 xl:border-b-0">
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs text-slate-500">{label}</span>
        <span className={cn("text-slate-500", tone === "gold" ? "text-panel-gold" : "text-panel-green")}>{icon}</span>
      </div>
      <p className="mt-2 font-mono text-2xl font-semibold text-slate-100">{value}</p>
    </div>
  );
}

function TimeSeriesPanel({
  color,
  current,
  icon,
  max,
  metric,
  series,
  subtitle,
  title,
  unit
}: {
  color: string;
  current?: number;
  icon: React.ReactNode;
  max: number;
  metric: string;
  series: SeriesPoint[];
  subtitle?: string;
  title: string;
  unit: string;
}) {
  const chart = buildChart(series, max);
  const display = current === undefined ? "—" : unit === "%" ? `${current.toFixed(1)}${unit}` : `${Math.round(current)} ${unit}`;
  return (
    <div className="bg-[#090d13] p-4">
      <div className="mb-3 flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-medium text-slate-200">
            <span style={{ color }}>{icon}</span>
            <span>{title}</span>
          </div>
          <p className="mt-1 truncate font-mono text-[11px] text-slate-500">{metric}</p>
        </div>
        <div className="text-right">
          <p className="font-mono text-2xl font-semibold text-slate-100">{display}</p>
          {subtitle ? <p className="mt-1 text-xs text-slate-500">{subtitle}</p> : null}
        </div>
      </div>
      <svg className="h-52 w-full overflow-visible" role="img" viewBox="0 0 640 240" preserveAspectRatio="none">
        <g>
          {[0, 1, 2, 3].map((line) => {
            const y = 28 + line * 48;
            const labelValue = Math.round(max - max * line / 3);
            return (
              <g key={line}>
                <line x1="48" x2="624" y1={y} y2={y} stroke="rgba(148,163,184,0.14)" strokeWidth="1" />
                <text x="8" y={y + 4} fill="#64748b" fontSize="11" fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace">{labelValue}</text>
              </g>
            );
          })}
          {[0, 1, 2, 3, 4, 5].map((line) => {
            const x = 48 + line * 115.2;
            return <line key={line} x1={x} x2={x} y1="28" y2="196" stroke="rgba(148,163,184,0.08)" strokeWidth="1" />;
          })}
          <line x1="48" x2="624" y1="196" y2="196" stroke="rgba(148,163,184,0.2)" strokeWidth="1" />
          <line x1="48" x2="48" y1="28" y2="196" stroke="rgba(148,163,184,0.16)" strokeWidth="1" />
          <path d={chart.area} fill={color} opacity="0.08" />
          <path d={chart.line} fill="none" stroke={color} strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" vectorEffect="non-scaling-stroke" />
          <circle cx={chart.last.x} cy={chart.last.y} r="3" fill={color} />
          <text x="48" y="226" fill="#64748b" fontSize="11" fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace">-15m</text>
          <text x="580" y="226" fill="#64748b" fontSize="11" fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace">now</text>
        </g>
      </svg>
      <div className="mt-2 flex items-center gap-2 text-xs text-slate-500">
        <span className="size-2 rounded-full" style={{ backgroundColor: color }} />
        <span className="font-mono">{metric}</span>
      </div>
    </div>
  );
}

function ServerLoadRow({ server }: { server: ObservabilityServerMetric }) {
  const cpuPercent = Math.min(server.cpuPercent / 400 * 100, 100);
  const memoryPercent = server.memoryLimitMb > 0 ? Math.min(server.memoryMb / server.memoryLimitMb * 100, 100) : 0;
  return (
    <div className="grid gap-3 p-4 lg:grid-cols-[minmax(0,1fr)_110px_150px_150px_110px] lg:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="truncate font-medium text-slate-100">{server.name}</p>
        </div>
        <p className="mt-1 truncate text-xs text-slate-500">{server.providerKey} · {server.version || "latest"}</p>
      </div>
      <span className={cn("w-fit rounded px-2 py-0.5 text-xs", server.status === "running" ? "bg-panel-green/15 text-panel-green" : "bg-slate-800 text-slate-400")}>{server.status}</span>
      <LoadBar label="CPU" value={`${server.cpuPercent.toFixed(1)}%`} percent={cpuPercent} />
      <LoadBar label="MEM" value={`${server.memoryMb} MB`} percent={memoryPercent} tone="purple" />
      <div className="flex items-center gap-2 text-sm text-slate-300">
        <Users aria-hidden="true" className="size-4 text-slate-500" />
        <span>{server.playersOnline} / {server.maxPlayers}</span>
      </div>
    </div>
  );
}

function LoadBar({ label, percent, tone = "green", value }: { label: string; percent: number; tone?: "green" | "purple"; value: string }) {
  return (
    <div>
      <div className="mb-1 flex justify-between gap-2 text-xs">
        <span className="text-slate-500">{label}</span>
        <span className="font-mono text-slate-300">{value}</span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-slate-800">
        <div className={cn("h-full rounded-full", tone === "purple" ? "bg-panel-purple" : "bg-panel-green")} style={{ width: `${Math.max(3, Math.min(percent, 100))}%` }} />
      </div>
    </div>
  );
}

function EventsPanel({ items, total, windowHours }: { items: { type: string; count: number }[]; total: number; windowHours: number }) {
  const { t } = useI18n();
  const max = Math.max(1, ...items.map((item) => item.count));
  return (
    <div className="bg-[#090d13] p-4">
      <div className="mb-4 flex items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2 text-sm font-medium text-slate-200">
            <ActivityIcon aria-hidden="true" className="size-4 text-panel-green" />
            <span>{t("activityEventMixDescription", { hours: windowHours })}</span>
          </div>
          <p className="mt-1 font-mono text-[11px] text-slate-500">gamepanel_activity_events_24h_by_type</p>
        </div>
        <p className="font-mono text-2xl font-semibold text-slate-100">{total}</p>
      </div>
      {items.length === 0 ? (
        <p className="text-sm text-slate-400">{t("noActivityYet")}</p>
      ) : (
        <div className="space-y-3">
          {items.map((item) => (
            <div key={item.type}>
              <div className="mb-1 flex justify-between gap-2 text-xs">
                <span className="text-slate-400">{eventTypeLabel(item.type, t)}</span>
                <span className="font-mono text-slate-500">{item.count}</span>
              </div>
              <div className="relative h-2 overflow-hidden rounded-sm bg-slate-900">
                <div className="absolute inset-y-0 left-0 bg-panel-green" style={{ width: `${Math.max(5, item.count / max * 100)}%` }} />
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function buildChart(series: SeriesPoint[], max: number) {
  const width = 576;
  const height = 168;
  const left = 48;
  const top = 28;
  const bottom = top + height;
  const pointsSource = series.length >= 2 ? series : [{ value: 0, ts: 0 }, { value: series[0]?.value ?? 0, ts: 1 }];
  const first = pointsSource[0]!.ts;
  const last = pointsSource[pointsSource.length - 1]!.ts;
  const points = pointsSource.map((point, index) => {
    const x = left + ((point.ts - first) / (last - first || 1)) * width;
    const y = bottom - Math.min(1, Math.max(0, point.value / max)) * height;
    return { x: Number.isFinite(x) ? x : left + index / Math.max(1, pointsSource.length - 1) * width, y };
  });
  const line = points.map((point, index) => `${index === 0 ? "M" : "L"} ${point.x.toFixed(1)} ${point.y.toFixed(1)}`).join(" ");
  return {
    area: `${line} L ${left + width} ${bottom} L ${left} ${bottom} Z`,
    last: points[points.length - 1]!,
    line
  };
}

function eventTypeLabel(type: string, t: (key: MessageKey) => string) {
  const keys: Record<string, MessageKey> = {
    backup: "activityTypeBackup",
    failure: "activityTypeFailure",
    lifecycle: "activityTypeLifecycle",
    other: "activityTypeOther",
    player: "activityTypePlayer"
  };
  return keys[type] ? t(keys[type]) : type;
}

function filterOptionLabel<T extends string>(
  options: readonly { key: T; labelKey?: MessageKey; label?: string }[],
  value: T,
  t: (key: MessageKey) => string
) {
  const option = options.find((item) => item.key === value);
  return option?.labelKey ? t(option.labelKey) : option?.label ?? value;
}
