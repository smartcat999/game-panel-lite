"use client";

import { useQuery } from "@tanstack/react-query";
import { Activity as ActivityIcon, AlertTriangle, Cpu, Gauge, MemoryStick, RadioTower, Users } from "lucide-react";
import { useMemo, useState } from "react";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { PageHeader } from "@/components/page-header";
import { Card } from "@/components/ui";
import { getObservabilityMetrics, listActivity, listGames, listServers, prometheusMetricsUrl, type ObservabilityServerMetric } from "@/lib/api";
import { formatActivityEvent } from "@/lib/activity-display";
import { gameFilterOptions } from "@/lib/game-filters";
import { localizeRelativeTime, useI18n, type MessageKey } from "@/lib/i18n";
import { Sparkline, useTimeSeries } from "@/lib/sparkline";
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

      <section className="mb-5 grid gap-3 md:grid-cols-4">
        <MetricTile
          icon={<Cpu aria-hidden="true" className="size-4" />}
          label={t("metricRuntimeCpu")}
          value={host ? `${host.totalCpuPercent.toFixed(1)}%` : "—"}
          hint={t("metricScrapeHint")}
          sparkline={<Sparkline data={cpuSeries} width={94} height={28} color="#7bd978" max={400} />}
        />
        <MetricTile
          icon={<MemoryStick aria-hidden="true" className="size-4" />}
          label={t("metricRuntimeMemory")}
          value={host ? `${host.totalMemoryMb} MB` : "—"}
          hint={host?.memoryLimitMb ? t("metricMemoryLimit", { limit: `${host.memoryLimitMb} MB` }) : t("metricNoLimit")}
          sparkline={<Sparkline data={memorySeries} width={94} height={28} color="#a78bfa" max={memoryLimit} />}
          tone="purple"
        />
        <MetricTile
          icon={<Gauge aria-hidden="true" className="size-4" />}
          label={t("metricRunningServers")}
          value={`${runningServers} / ${metrics?.servers.length ?? 0}`}
          hint={t("metricDockerContainers", { count: host?.runningContainers ?? 0 })}
        />
        <MetricTile
          icon={<Users aria-hidden="true" className="size-4" />}
          label={t("metricOnlinePlayers")}
          value={`${totalPlayers}`}
          hint={t("playersOnlineHint", { count: totalPlayers, capacity: playerCapacity })}
        />
      </section>

      <section className="mb-5 grid gap-4 xl:grid-cols-[minmax(0,1.25fr)_minmax(300px,0.75fr)]">
        <Card className="overflow-hidden">
          <div className="border-b border-panel-line px-4 py-4">
            <h2 className="font-semibold">{t("serverLoadTitle")}</h2>
            <p className="mt-1 text-xs text-slate-500">{t("serverLoadDescription")}</p>
          </div>
          <div className="divide-y divide-panel-line">
            {topServers.length > 0 ? (
              topServers.map((server) => <ServerLoadRow key={server.id} server={server} />)
            ) : (
              <div className="p-4 text-sm text-slate-400">{metricsQuery.isLoading ? t("loading") : t("noServersYet")}</div>
            )}
          </div>
        </Card>

        <Card className="overflow-hidden">
          <div className="border-b border-panel-line px-4 py-4">
            <h2 className="font-semibold">{t("activityEventMixTitle")}</h2>
            <p className="mt-1 text-xs text-slate-500">{t("activityEventMixDescription", { hours: metrics?.activity.windowHours ?? 24 })}</p>
          </div>
          <div className="space-y-4 p-4">
            <div className="grid grid-cols-2 gap-3">
              <EventCount icon={<ActivityIcon aria-hidden="true" className="size-4" />} label={t("activityEventsTotal")} value={metrics?.activity.total ?? 0} />
              <EventCount icon={<AlertTriangle aria-hidden="true" className="size-4" />} label={t("activityFailures")} value={metrics?.activity.failures ?? 0} tone="gold" />
            </div>
            <EventMixBars items={metrics?.activity.byType ?? []} />
          </div>
        </Card>
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

function MetricTile({
  hint,
  icon,
  label,
  sparkline,
  tone = "green",
  value
}: {
  hint: string;
  icon: React.ReactNode;
  label: string;
  sparkline?: React.ReactNode;
  tone?: "green" | "purple";
  value: string;
}) {
  const toneClass = tone === "purple" ? "text-panel-purple bg-panel-purple/10 border-panel-purple/25" : "text-panel-green bg-panel-green/10 border-panel-green/25";
  return (
    <Card className="p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <span className={cn("mb-3 flex size-8 items-center justify-center rounded-md border", toneClass)}>{icon}</span>
          <p className="text-xs text-slate-500">{label}</p>
          <p className="mt-1 font-mono text-xl font-semibold text-slate-100">{value}</p>
          <p className="mt-1 text-xs text-slate-500">{hint}</p>
        </div>
        {sparkline ? <div className="shrink-0 pt-1">{sparkline}</div> : null}
      </div>
    </Card>
  );
}

function ServerLoadRow({ server }: { server: ObservabilityServerMetric }) {
  const cpuPercent = Math.min(server.cpuPercent / 400 * 100, 100);
  const memoryPercent = server.memoryLimitMb > 0 ? Math.min(server.memoryMb / server.memoryLimitMb * 100, 100) : 0;
  return (
    <div className="grid gap-3 p-4 lg:grid-cols-[minmax(0,1fr)_140px_140px_120px] lg:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="truncate font-medium text-slate-100">{server.name}</p>
          <span className={cn("rounded px-2 py-0.5 text-xs", server.status === "running" ? "bg-panel-green/15 text-panel-green" : "bg-slate-800 text-slate-400")}>{server.status}</span>
        </div>
        <p className="mt-1 truncate text-xs text-slate-500">{server.providerKey} · {server.version || "latest"}</p>
      </div>
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

function EventCount({ icon, label, tone = "green", value }: { icon: React.ReactNode; label: string; tone?: "green" | "gold"; value: number }) {
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/35 p-3">
      <div className={cn("mb-2 flex size-7 items-center justify-center rounded", tone === "gold" ? "bg-panel-gold/15 text-panel-gold" : "bg-panel-green/15 text-panel-green")}>{icon}</div>
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 font-mono text-lg font-semibold text-slate-100">{value}</p>
    </div>
  );
}

function EventMixBars({ items }: { items: { type: string; count: number }[] }) {
  const { t } = useI18n();
  const max = Math.max(1, ...items.map((item) => item.count));
  if (items.length === 0) {
    return <p className="text-sm text-slate-400">{t("noActivityYet")}</p>;
  }
  return (
    <div className="space-y-2">
      {items.map((item) => (
        <div key={item.type}>
          <div className="mb-1 flex justify-between gap-2 text-xs">
            <span className="text-slate-400">{eventTypeLabel(item.type, t)}</span>
            <span className="font-mono text-slate-500">{item.count}</span>
          </div>
          <div className="h-1.5 overflow-hidden rounded-full bg-slate-800">
            <div className="h-full rounded-full bg-panel-green" style={{ width: `${Math.max(5, item.count / max * 100)}%` }} />
          </div>
        </div>
      ))}
    </div>
  );
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
