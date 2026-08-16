"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Activity, CheckCircle2, CircleAlert, Container, Plus, Server } from "lucide-react";
import { useState } from "react";
import { ResourceTrendChart, ServerStatusKpis } from "@/components/dashboard-charts";
import { PageHeader } from "@/components/page-header";
import { ServerResourceTable } from "@/components/server-resource-table";
import { Button } from "@/components/ui";
import { getPlatformMonitoring } from "@/features/monitoring/api";
import type { MetricSeries } from "@/features/monitoring/types";
import { formatActivityEvent } from "@/lib/activity-display";
import { isWorldOrBackupEventType } from "@/lib/feature-flags";
import { gameServerStatus } from "@/lib/game-server-resource";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import { getApiHealth, getDockerStatus, getObservabilityMetrics, getSettings, listActivity, listGameServers } from "@/lib/api";
import { cn } from "@/lib/utils";

type DashboardRange = "1h" | "6h" | "24h" | "168h";
type DashboardMetric = "nodeCpu" | "nodeMemory" | "nodeNetwork";

const rangeOptions: { label: string; value: DashboardRange; step: string }[] = [
  { label: "1h", value: "1h", step: "1m" },
  { label: "6h", value: "6h", step: "5m" },
  { label: "24h", value: "24h", step: "15m" },
  { label: "7d", value: "168h", step: "1h" }
];

export default function DashboardPage() {
  const { locale, t } = useI18n();
  const [range, setRange] = useState<DashboardRange>("1h");
  const [metricKey, setMetricKey] = useState<DashboardMetric>("nodeCpu");
  const step = rangeOptions.find((item) => item.value === range)?.step ?? "1m";
  const serversQuery = useQuery({ queryKey: ["game-servers"], queryFn: listGameServers, retry: false, refetchInterval: 10000 });
  const activityQuery = useQuery({ queryKey: ["activity"], queryFn: listActivity, retry: false, refetchInterval: 30000 });
  const metricsQuery = useQuery({ queryKey: ["observability-metrics"], queryFn: getObservabilityMetrics, retry: false, refetchInterval: 10000 });
  const platformQuery = useQuery({
    queryKey: ["monitoring-platform", range, step],
    queryFn: () => getPlatformMonitoring(range, step),
    retry: false,
    refetchInterval: 30000
  });
  const apiQuery = useQuery({ queryKey: ["api-health"], queryFn: getApiHealth, retry: false, refetchInterval: 30000 });
  const dockerQuery = useQuery({ queryKey: ["docker-status"], queryFn: getDockerStatus, retry: false, refetchInterval: 30000 });
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false, staleTime: 5 * 60 * 1000 });

  const servers = serversQuery.data ?? [];
  const activity = (activityQuery.data ?? []).filter((event) => !isWorldOrBackupEventType(event.type));
  const running = servers.filter((server) => gameServerStatus(server) === "running");
  const unhealthy = servers.filter((server) => gameServerStatus(server) === "errored");
  const stopped = servers.filter((server) => gameServerStatus(server) === "stopped").length;
  const statusData = [
    { color: "#94a3b8", label: t("dashboardTotalInstances"), value: servers.length },
    { color: "#59d46f", label: t("statusRunning"), value: running.length },
    { color: "#64748b", label: t("statusStopped"), value: stopped },
    { color: "#f87171", label: t("statusErrored"), value: unhealthy.length }
  ];
  const series = platformQuery.data?.series[metricKey];
  const featuredServers = [...servers].sort(serverPriority).slice(0, 5);
  const hasDataError = serversQuery.isError || activityQuery.isError || metricsQuery.isError;

  return (
    <>
      <PageHeader
        title={t("dashboardTitle")}
        description={t("dashboardInfrastructureDescription")}
        action={
          <Link href="/servers/new">
            <Button className="h-10 px-4"><Plus aria-hidden="true" className="size-4" />{t("createServer")}</Button>
          </Link>
        }
      />

      {hasDataError ? <p className="mb-4 text-sm text-panel-gold" role="alert">{t("apiDataUnavailable")}</p> : null}

      <section className="border-y border-panel-line py-5" aria-labelledby="dashboard-summary-title">
        <div className="mb-4">
          <h2 className="text-base font-semibold text-slate-100" id="dashboard-summary-title">{t("dashboardOperationsOverview")}</h2>
          <p className="mt-1 text-xs text-slate-500">{t("dashboardOperationsOverviewDescription")}</p>
        </div>
        <div className="min-w-0">
          <h3 className="mb-3 text-sm font-medium text-slate-300">{t("dashboardServerStatus")}</h3>
          <ServerStatusKpis data={statusData} hint={t("dashboardInstanceStatusHint")} />
        </div>
      </section>

      <section className="border-b border-panel-line py-6" aria-labelledby="resource-trend-title">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
          <div>
            <h2 id="resource-trend-title" className="text-base font-semibold text-slate-100">{t("dashboardResourceTrend")}</h2>
            <p className="mt-1 text-xs text-slate-500">{t("dashboardResourceTrendDescription")}</p>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <div className="inline-flex rounded-md border border-panel-line bg-slate-950/35 p-0.5" aria-label={t("dashboardResourceMetric")}>
              <MetricButton active={metricKey === "nodeCpu"} onClick={() => setMetricKey("nodeCpu")}>CPU</MetricButton>
              <MetricButton active={metricKey === "nodeMemory"} onClick={() => setMetricKey("nodeMemory")}>{t("memory")}</MetricButton>
              <MetricButton active={metricKey === "nodeNetwork"} onClick={() => setMetricKey("nodeNetwork")}>{t("dashboardNetwork")}</MetricButton>
            </div>
            <div className="inline-flex items-center" aria-label={t("monitoringRange")}>
              {rangeOptions.map((item) => (
                <button
                  className={cn("rounded px-2.5 py-1.5 text-xs text-slate-500 transition hover:text-slate-200 focus:outline-none focus:ring-2 focus:ring-panel-green/50", range === item.value && "bg-slate-800 text-slate-100")}
                  key={item.value}
                  onClick={() => setRange(item.value)}
                  type="button"
                >
                  {item.label}
                </button>
              ))}
            </div>
          </div>
        </div>
        <div className="mt-3 flex items-center justify-between gap-4">
          <span className="text-xs text-slate-500">{metricTitle(metricKey, t)}</span>
          <span className="font-mono text-sm font-semibold text-slate-200">{formatCurrentMetric(series)}</span>
        </div>
        <ResourceTrendChart emptyLabel={platformQuery.isLoading ? t("loading") : t("monitoringNoSamples")} series={series} />
      </section>

      <section className="border-b border-panel-line py-6" aria-labelledby="dashboard-servers-title">
        <div className="mb-3 flex items-center justify-between gap-4">
          <h2 id="dashboard-servers-title" className="text-base font-semibold text-slate-100">{t("dashboardServerInstances")}</h2>
          <Link className="text-sm font-medium text-panel-green hover:text-panel-green/80" href="/servers">{t("dashboardViewAll")}</Link>
        </div>
        {featuredServers.length > 0 ? (
          <ServerResourceTable
            flat
            limit={5}
            metrics={metricsQuery.data?.servers}
            publicHost={settingsQuery.data?.publicHost}
            servers={featuredServers}
            showVersion={false}
          />
        ) : (
          <div className="py-8 text-center text-sm text-slate-500">
            <p>{serversQuery.isLoading ? t("loading") : t("noServersYet")}</p>
            {!serversQuery.isLoading ? <Link className="mt-2 inline-block text-panel-green" href="/servers/new">{t("createServer")}</Link> : null}
          </div>
        )}
      </section>

      <div className="grid border-b border-panel-line xl:grid-cols-[minmax(0,2fr)_minmax(280px,1fr)] xl:divide-x xl:divide-panel-line">
        <section className="min-w-0 py-5 xl:pr-6" aria-labelledby="recent-events-title">
          <div className="mb-3 flex items-center justify-between gap-4">
            <h2 id="recent-events-title" className="text-base font-semibold text-slate-100">{t("dashboardRecentEvents")}</h2>
            <Link className="text-sm text-slate-400 hover:text-panel-green" href="/activity">{t("dashboardViewAll")}</Link>
          </div>
          {activity.length === 0 ? (
            <p className="py-5 text-sm text-slate-500">{activityQuery.isLoading ? t("loading") : t("noActivityYet")}</p>
          ) : (
            <div className="divide-y divide-panel-line">
              {activity.slice(0, 5).map((event) => {
                const display = formatActivityEvent(event, locale);
                return (
                  <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-4 py-3" key={event.id}>
                    <div className="min-w-0">
                      <p className="truncate text-sm text-slate-200">{display.typeLabel}</p>
                      <p className="mt-0.5 truncate text-xs text-slate-500">{display.message}</p>
                    </div>
                    <time className="whitespace-nowrap text-xs text-slate-500">{localizeRelativeTime(event.created, locale)}</time>
                  </div>
                );
              })}
            </div>
          )}
        </section>

        <section className="min-w-0 border-t border-panel-line py-5 xl:border-t-0 xl:pl-6" aria-labelledby="system-status-title">
          <h2 id="system-status-title" className="text-base font-semibold text-slate-100">{t("dashboardSystemStatus")}</h2>
          <div className="mt-3 divide-y divide-panel-line">
            <SystemStatusRow icon={<Server aria-hidden="true" />} label={t("dashboardPanelService")} loading={apiQuery.isLoading} healthy={apiQuery.data?.status === "ok"} />
            <SystemStatusRow icon={<Container aria-hidden="true" />} label={t("docker")} loading={dockerQuery.isLoading} healthy={Boolean(dockerQuery.data?.available)} />
            <SystemStatusRow icon={<Activity aria-hidden="true" />} label={t("dashboardMetricsService")} loading={platformQuery.isLoading} healthy={Boolean(platformQuery.data?.dataSource.connected)} />
          </div>
          <Link className="mt-4 inline-flex text-sm text-slate-400 hover:text-panel-green" href="/activity">{t("dashboardOpenMonitoring")}</Link>
        </section>
      </div>
    </>
  );
}

function MetricButton({ active, children, onClick }: { active: boolean; children: React.ReactNode; onClick: () => void }) {
  return (
    <button
      className={cn("rounded px-3 py-1.5 text-xs text-slate-500 transition hover:text-slate-200 focus:outline-none focus:ring-2 focus:ring-panel-green/50", active && "bg-slate-800 text-slate-100")}
      onClick={onClick}
      type="button"
    >
      {children}
    </button>
  );
}

function SystemStatusRow({ healthy, icon, label, loading }: { healthy: boolean; icon: React.ReactNode; label: string; loading: boolean }) {
  const { t } = useI18n();
  const statusLabel = loading ? t("dockerCheckingShort") : healthy ? t("dashboardStatusNormal") : t("unavailable");
  return (
    <div className="flex items-center justify-between gap-4 py-3 text-sm">
      <span className="flex items-center gap-2 text-slate-300"><span className="text-slate-500 [&>svg]:size-4">{icon}</span>{label}</span>
      <span className={cn("inline-flex items-center gap-2", healthy ? "text-panel-green" : loading ? "text-slate-500" : "text-panel-gold")}>
        {healthy ? <CheckCircle2 aria-hidden="true" className="size-4" /> : <CircleAlert aria-hidden="true" className="size-4" />}
        {statusLabel}
      </span>
    </div>
  );
}

function metricTitle(key: DashboardMetric, t: ReturnType<typeof useI18n>["t"]) {
  if (key === "nodeMemory") return t("metricTitleNodeMemory");
  if (key === "nodeNetwork") return t("metricTitleNodeNetwork");
  return t("metricTitleNodeCpu");
}

function formatCurrentMetric(series?: MetricSeries) {
  const value = series?.currentValue;
  if (value === null || value === undefined || !Number.isFinite(value)) return "—";
  if (series?.unit === "%") return `${value.toFixed(1)}%`;
  if (series?.unit === "MB") return value >= 1024 ? `${(value / 1024).toFixed(1)} GB` : `${value.toFixed(0)} MB`;
  if (series?.unit === "MB/s") return `${value.toFixed(2)} MB/s`;
  return `${value.toFixed(1)} ${series?.unit ?? ""}`.trim();
}

function serverPriority(left: Parameters<typeof gameServerStatus>[0], right: Parameters<typeof gameServerStatus>[0]) {
  const rank: Record<string, number> = { errored: 0, running: 1, creating: 2, starting: 2, stopping: 2, restarting: 2, stopped: 3 };
  return (rank[gameServerStatus(left)] ?? 4) - (rank[gameServerStatus(right)] ?? 4);
}
