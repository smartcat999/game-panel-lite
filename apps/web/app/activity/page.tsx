"use client";

import { useMemo, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { Activity, AlertTriangle, CheckCircle2, Cpu, ExternalLink, Gauge, HardDrive, MemoryStick, Network, RadioTower, Server, Users } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import {
  ActivityTimeline,
  ChartIcon,
  MonitoringChartCard,
  PlatformHealth,
  ServerLoadTable,
  SourceBadge
} from "@/features/monitoring/components";
import {
  getMonitoringEvents,
  getMonitoringMetrics,
  getMonitoringOverview,
  getPlatformMonitoring,
  getServerLoad
} from "@/features/monitoring/api";
import type { MetricSeries, MonitoringEvent, MonitoringOverviewResponse, PlatformResponse, PlatformService, ServerLoadRow } from "@/features/monitoring/types";
import { isWorldOrBackupEventType } from "@/lib/feature-flags";
import { gameDisplayName } from "@/lib/game-display";
import { useI18n, type MessageKey } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type FilterValue = "all" | string;
type MonitoringSection = "overview" | "resource-trends" | "platform" | "server-load" | "activity-log";

const severityOptions = ["all", "error", "warning", "success", "info"] as const;
const eventTypeOptions = ["all", "server", "mod", "player", "settings", "system"] as const;

export default function ActivityPage() {
  const { t } = useI18n();
  const [search, setSearch] = useState("");
  const [severity, setSeverity] = useState<FilterValue>("all");
  const [eventType, setEventType] = useState<FilterValue>("all");
  const [game, setGame] = useState<FilterValue>("all");
  const [section, setSection] = useState<MonitoringSection>("overview");

  const overviewQuery = useQuery({ queryKey: ["monitoring-overview"], queryFn: getMonitoringOverview, retry: false, refetchInterval: 5000 });
  const metricsQuery = useQuery({ queryKey: ["monitoring-metrics", "15m"], queryFn: () => getMonitoringMetrics("15m", "30s"), retry: false, refetchInterval: 5000 });
  const loadQuery = useQuery({ queryKey: ["monitoring-server-load"], queryFn: getServerLoad, retry: false, refetchInterval: 5000 });
  const eventsQuery = useQuery({ queryKey: ["monitoring-events", severity, eventType, game], queryFn: () => getMonitoringEvents({ severity, type: eventType, game, limit: 100 }), retry: false, refetchInterval: 5000 });
  const platformQuery = useQuery({ queryKey: ["monitoring-platform", "15m"], queryFn: () => getPlatformMonitoring("15m", "30s"), retry: false, refetchInterval: 5000 });

  const visibleEvents = useMemo(() => (eventsQuery.data?.events ?? []).filter((event) => !isWorldOrBackupEventType(event.type)), [eventsQuery.data?.events]);
  const events = useMemo(() => filterEvents(visibleEvents, search), [visibleEvents, search]);
  const games = useMemo(() => gameOptions(loadQuery.data?.rows ?? [], t), [loadQuery.data?.rows, t]);
  const activeChips = [search.trim(), severity !== "all" ? severity : "", eventType !== "all" ? eventType : "", game !== "all" ? game : ""].filter(Boolean);
  const anyError = overviewQuery.isError || metricsQuery.isError || loadQuery.isError || eventsQuery.isError || platformQuery.isError;

  return (
    <>
      <PageHeader
        title={t("monitoringTitle")}
        description={t("monitoringDescription")}
        action={
          <div className="flex flex-wrap justify-end gap-2">
            <SourceBadge connected={overviewQuery.data?.dataSource.connected} />
          </div>
        }
      />
      {anyError ? <p className="mb-4 text-sm text-panel-gold">{t("monitoringApiPartialUnavailable")}</p> : null}

      <div className="space-y-5">
        <MonitoringSectionNav
          active={section}
          counts={{
            "activity-log": visibleEvents.length,
            overview: overviewQuery.data?.kpis.issues ?? 0,
            platform: platformQuery.data?.services.length ?? 0,
            "resource-trends": 4,
            "server-load": loadQuery.data?.rows.length ?? 0
          }}
          onChange={setSection}
        />

        {section === "overview" ? (
          <CommandCenterOverview
            loadRows={loadQuery.data?.rows ?? []}
            overview={overviewQuery.data}
            platform={platformQuery.data}
          />
        ) : null}

        {section === "resource-trends" ? (
          <section className="grid gap-4 md:grid-cols-2">
            <MetricGroupHeader
              title={t("serverResourceTitle")}
              description={t("serverResourceDescription")}
              meta={
                <>
                  <TechBadge label={t("monitoringRange")} value="15m" />
                  <TechBadge label={t("monitoringRefresh")} value="5s" />
                </>
              }
            />
            <MonitoringChartCard color="#59d46f" icon={<Cpu aria-hidden="true" className="size-4" />} range={metricsQuery.data?.range} series={metricsQuery.data?.series.cpu} />
            <MonitoringChartCard color="#a873ff" icon={<MemoryStick aria-hidden="true" className="size-4" />} range={metricsQuery.data?.range} series={metricsQuery.data?.series.memory} />
            <MonitoringChartCard color="#59d46f" icon={<Users aria-hidden="true" className="size-4" />} range={metricsQuery.data?.range} series={metricsQuery.data?.series.players} />
            <MonitoringChartCard color="#e6b84a" icon={<Activity aria-hidden="true" className="size-4" />} range={metricsQuery.data?.range} series={metricsQuery.data?.series.events} />
          </section>
        ) : null}

        {section === "platform" ? (
          <>
            <section className="grid gap-4 md:grid-cols-2">
              <MetricGroupHeader
                title={t("nodeResourceTitle")}
                description={t("nodeResourceDescription")}
                meta={
                  <>
                    <TechBadge label={t("monitoringRange")} value="15m" />
                    <TechBadge label={t("monitoringRefresh")} value="5s" />
                  </>
                }
              />
              <MonitoringChartCard color="#7dd3fc" icon={<Server aria-hidden="true" className="size-4" />} range={platformQuery.data?.range} series={platformQuery.data?.series.nodeCpu} />
              <MonitoringChartCard color="#a873ff" icon={<MemoryStick aria-hidden="true" className="size-4" />} range={platformQuery.data?.range} series={platformQuery.data?.series.nodeMemory} />
              <MonitoringChartCard color="#e6b84a" icon={<HardDrive aria-hidden="true" className="size-4" />} range={platformQuery.data?.range} series={platformQuery.data?.series.nodeDisk} />
              <MonitoringChartCard color="#59d46f" icon={<Network aria-hidden="true" className="size-4" />} range={platformQuery.data?.range} series={platformQuery.data?.series.nodeNetwork} />
            </section>

            <section className="grid gap-4 md:grid-cols-2">
              <MetricGroupHeader title={t("platformTrafficTitle")} description={t("platformTrafficDescription")} />
              <MonitoringChartCard color="#7dd3fc" icon={<ChartIcon type="requests" />} range={platformQuery.data?.range} series={platformQuery.data?.series.requests} />
              <MonitoringChartCard color="#ff6b6b" icon={<Gauge aria-hidden="true" className="size-4" />} range={platformQuery.data?.range} series={platformQuery.data?.series.latencyP95} />
            </section>

            <PlatformHealth services={platformQuery.data?.services ?? []} topRoutes={platformQuery.data?.topRoutes ?? []} />
          </>
        ) : null}

        {section === "server-load" ? <ServerLoadTable rows={loadQuery.data?.rows ?? []} /> : null}

        {section === "activity-log" ? (
          <section>
            <div className="mb-3 rounded-lg border border-panel-line bg-slate-950/35 px-4 py-3">
              <h2 className="text-sm font-semibold text-slate-100">{t("monitoringActivityLogTitle")}</h2>
              <p className="mt-1 text-xs text-slate-500">{t("recentAlertsDescription")}</p>
            </div>
            <ResourceFilterBar
              activeChips={activeChips}
              clearLabel={t("clearFilters")}
              density="compact"
              filters={[
                { label: t("filterGame"), options: games, value: game, onChange: setGame },
                { label: t("filterType"), options: eventTypeOptions.map((key) => ({ key, label: key === "all" ? t("filterAll") : key })), value: eventType, onChange: setEventType },
                { label: t("filterSeverity"), options: severityOptions.map((key) => ({ key, label: key === "all" ? t("filterAll") : key })), value: severity, onChange: setSeverity }
              ]}
              onClear={() => {
                setGame("all");
                setEventType("all");
                setSeverity("all");
                setSearch("");
              }}
              onSearchChange={setSearch}
              resultLabel={t("filteredResultsCount", { count: events.length })}
              search={search}
              searchPlaceholder={t("searchMonitoringEvents")}
            />
            <ActivityTimeline events={events} />
          </section>
        ) : null}
      </div>
    </>
  );
}

function MonitoringSectionNav({
  active,
  counts,
  onChange
}: {
  active: MonitoringSection;
  counts: Record<MonitoringSection, number>;
  onChange: (section: MonitoringSection) => void;
}) {
  const { t } = useI18n();
  const tabs: { id: MonitoringSection; label: string; countLabel?: string }[] = [
    { id: "overview", label: t("monitoringNavOverview"), countLabel: counts.overview > 0 ? t("monitoringNavIssues", { count: counts.overview }) : undefined },
    { id: "resource-trends", label: t("monitoringNavResourceTrends") },
    { id: "platform", label: t("monitoringNavPlatform") },
    { id: "server-load", label: t("monitoringNavServerLoad") },
    { id: "activity-log", label: t("monitoringNavActivityLog") }
  ];

  return (
    <div className="rounded-lg border border-panel-line bg-panel-card p-1.5">
      <div className="flex flex-wrap gap-1" role="tablist" aria-label={t("monitoringTitle")}>
        {tabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={active === tab.id}
            className={[
              "inline-flex min-h-10 items-center gap-2 rounded-md border px-3 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-panel-green/40",
              active === tab.id
                ? "border-panel-green/35 bg-panel-green/10 text-slate-100"
                : "border-transparent text-slate-400 hover:border-panel-line hover:bg-slate-950/35 hover:text-slate-200"
            ].join(" ")}
            onClick={() => onChange(tab.id)}
          >
            <span>{tab.label}</span>
            {tab.countLabel ? <span className="rounded border border-panel-gold/25 bg-panel-gold/10 px-1.5 py-0.5 font-mono text-[11px] text-panel-gold">{tab.countLabel}</span> : null}
          </button>
        ))}
      </div>
    </div>
  );
}

function CommandCenterOverview({
  loadRows,
  overview,
  platform
}: {
  loadRows: ServerLoadRow[];
  overview?: MonitoringOverviewResponse;
  platform?: PlatformResponse;
}) {
  const { t } = useI18n();
  const issueRows = loadRows.filter((row) => row.severity !== "normal" || row.status === "error" || row.status === "errored").slice(0, 3);
  const services = compactPlatformServices(platform?.services ?? []);
  const failedTargets = overview?.health.failedTargets ?? 0;
  const nodeCpu = platform?.series.nodeCpu;
  const nodeMemory = platform?.series.nodeMemory;

  return (
    <section className="space-y-4">
      <OverviewStatusStrip overview={overview} />
      <OverviewKpis overview={overview} nodeCpu={nodeCpu} nodeMemory={nodeMemory} />

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.1fr)_minmax(360px,0.9fr)]">
        <AttentionPanel failedTargets={failedTargets} rows={issueRows} />
        <CompactPlatformMatrix services={services} />
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <div className="rounded-lg border border-panel-line bg-slate-950/35 px-4 py-3 xl:col-span-2">
          <h2 className="text-sm font-semibold text-slate-100">{t("monitoringOverviewNodeTrendsTitle")}</h2>
          <p className="mt-1 text-xs text-slate-500">{t("monitoringOverviewNodeTrendsDescription")}</p>
        </div>
        <MonitoringChartCard compact color="#7dd3fc" icon={<Server aria-hidden="true" className="size-4" />} range={platform?.range} series={nodeCpu} />
        <MonitoringChartCard compact color="#a873ff" icon={<MemoryStick aria-hidden="true" className="size-4" />} range={platform?.range} series={nodeMemory} />
      </div>
    </section>
  );
}

function OverviewStatusStrip({ overview }: { overview?: MonitoringOverviewResponse }) {
  const { t } = useI18n();
  const issues = overview?.kpis.issues ?? 0;
  const connected = Boolean(overview?.dataSource.connected);
  const overall = overview?.health.overall ?? "unknown";
  const isHealthy = issues === 0 && overall === "healthy";

  return (
    <div className="rounded-lg border border-panel-line bg-panel-card px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span className={cn("flex size-9 shrink-0 items-center justify-center rounded-md border", isHealthy ? "border-panel-green/30 bg-panel-green/10 text-panel-green" : "border-panel-gold/30 bg-panel-gold/10 text-panel-gold")}>
            {isHealthy ? <CheckCircle2 aria-hidden="true" className="size-4" /> : <AlertTriangle aria-hidden="true" className="size-4" />}
          </span>
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold text-slate-100">
              {isHealthy ? t("monitoringOverviewHealthySummary") : t("monitoringOverviewWarningSummary", { count: issues })}
            </p>
            <p className="mt-0.5 text-xs text-slate-500">
              {connected ? t("monitoringOverviewDataConnected") : t("monitoringOverviewDataUnavailable")}
              {" · "}
              {t("monitoringOverviewLastSync", { time: overview?.health.lastSync ? formatOverviewTime(overview.health.lastSync) : t("none") })}
            </p>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <OverviewMetaPill label={t("monitoringRange")} value="15m" />
          <OverviewMetaPill label={t("monitoringRefresh")} value="5s" />
        </div>
      </div>
    </div>
  );
}

function OverviewMetaPill({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex h-8 items-center gap-2 rounded-md border border-panel-line bg-slate-950/35 px-2.5 text-xs">
      <span className="text-slate-500">{label}</span>
      <span className="font-mono font-medium text-slate-200">{value}</span>
    </span>
  );
}

function OverviewKpis({ nodeCpu, nodeMemory, overview }: { nodeCpu?: MetricSeries; nodeMemory?: MetricSeries; overview?: MonitoringOverviewResponse }) {
  const { t } = useI18n();
  const kpis = overview?.kpis;
  const cpu = nodeCpu?.currentValue;
  const memory = nodeMemory?.currentValue;
  const memoryLimit = nodeMemory?.threshold;
  return (
    <div className="grid gap-3 xl:grid-cols-4">
      <CommandKpi
        label={t("monitoringKpiServers")}
        note={t("monitoringKpiServersNote", { running: kpis?.runningServers ?? 0, total: kpis?.totalServers ?? 0 })}
        tone={(kpis?.issues ?? 0) > 0 ? "warning" : "success"}
        value={`${kpis?.runningServers ?? 0} / ${kpis?.totalServers ?? 0}`}
      />
      <CommandKpi
        label={t("monitoringKpiIssues")}
        note={(kpis?.issues ?? 0) > 0 ? t("kpiIssuesAction") : t("kpiIssuesClear")}
        tone={(kpis?.issues ?? 0) > 0 ? "warning" : "success"}
        value={kpis?.issues ?? 0}
      />
      <CommandKpi
        label={t("monitoringKpiPlayers")}
        note={t("monitoringKpiPlayersNote", { online: kpis?.onlinePlayers ?? 0, capacity: kpis?.playerCapacity ?? 0 })}
        tone="neutral"
        value={`${kpis?.onlinePlayers ?? 0} / ${kpis?.playerCapacity ?? 0}`}
      />
      <CommandKpi
        label={t("monitoringKpiNodeResource")}
        note={t("monitoringKpiNodeResourceNote", { cpu: formatPercent(cpu), memory: formatMemoryPair(memory, memoryLimit) })}
        tone={(cpu ?? 0) > 80 || (memoryLimit && memory ? memory / memoryLimit > 0.85 : false) ? "warning" : "success"}
        value={`${formatPercent(cpu)} · ${formatMemoryShort(memory)}`}
      />
    </div>
  );
}

function CommandKpi({ label, note, tone, value }: { label: string; note: string; tone: "neutral" | "success" | "warning"; value: number | string }) {
  return (
    <div className="rounded-lg border border-panel-line bg-panel-card p-4">
      <div className="flex items-center justify-between gap-3">
        <p className="truncate text-xs font-medium text-slate-500">{label}</p>
        <span className={cn("size-2 rounded-full", tone === "success" ? "bg-panel-green" : tone === "warning" ? "bg-panel-gold" : "bg-slate-600")} />
      </div>
      <p className="mt-3 font-mono text-3xl font-semibold leading-none text-slate-100">{value}</p>
      <p className="mt-3 truncate text-xs text-slate-500">{note}</p>
    </div>
  );
}

function AttentionPanel({ failedTargets, rows }: { failedTargets: number; rows: ServerLoadRow[] }) {
  const { t } = useI18n();
  const hasItems = rows.length > 0 || failedTargets > 0;
  return (
    <div className="rounded-lg border border-panel-line bg-panel-card p-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-sm font-semibold text-slate-100">{t("monitoringAttentionTitle")}</h2>
          <p className="mt-1 text-xs text-slate-500">{t("monitoringAttentionDescription")}</p>
        </div>
        {hasItems ? <span className="rounded-md border border-panel-gold/30 bg-panel-gold/10 px-2 py-0.5 text-xs font-medium text-panel-gold">{t("healthWarning")}</span> : null}
      </div>

      <div className="mt-4 space-y-2">
        {rows.map((row) => <AttentionServerRow key={row.serverId} row={row} />)}
        {failedTargets > 0 ? (
          <div className="rounded-md border border-panel-gold/20 bg-panel-gold/5 px-3 py-2">
            <p className="text-sm font-medium text-slate-100">{t("monitoringFailedTargetsTitle", { count: failedTargets })}</p>
            <p className="mt-1 text-xs text-slate-500">{t("monitoringFailedTargetsDescription")}</p>
          </div>
        ) : null}
        {!hasItems ? (
          <div className="rounded-md border border-panel-green/20 bg-panel-green/5 px-3 py-3">
            <p className="text-sm font-medium text-slate-100">{t("monitoringAttentionEmptyTitle")}</p>
            <p className="mt-1 text-xs text-slate-500">{t("monitoringAttentionEmptyDescription")}</p>
          </div>
        ) : null}
      </div>
    </div>
  );
}

function AttentionServerRow({ row }: { row: ServerLoadRow }) {
  const { t } = useI18n();
  return (
    <div className="grid gap-3 rounded-md border border-panel-line bg-slate-950/35 px-3 py-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <p className="truncate text-sm font-medium text-slate-100">{row.serverName}</p>
          <span className="rounded border border-panel-gold/25 bg-panel-gold/10 px-1.5 py-0.5 text-[11px] text-panel-gold">{row.status}</span>
        </div>
        <p className="mt-1 truncate text-xs text-slate-500">
          {row.gameKey} · CPU {row.cpuPercent.toFixed(1)}% · {Math.round(row.memoryMb)} MB · {row.playersOnline}/{row.maxPlayers}
        </p>
      </div>
      <Link className="inline-flex w-fit items-center gap-1 rounded border border-panel-line px-2.5 py-1.5 text-xs font-medium text-slate-300 transition hover:bg-slate-900 hover:text-panel-green" href={`/servers/${row.serverId}`}>
        {t("view")} <ExternalLink aria-hidden="true" className="size-3" />
      </Link>
    </div>
  );
}

function CompactPlatformMatrix({ services }: { services: PlatformService[] }) {
  const { t } = useI18n();
  return (
    <div className="rounded-lg border border-panel-line bg-panel-card p-4">
      <h2 className="text-sm font-semibold text-slate-100">{t("monitoringPlatformCompactTitle")}</h2>
      <p className="mt-1 text-xs text-slate-500">{t("monitoringPlatformCompactDescription")}</p>
      <div className="mt-4 grid gap-2 sm:grid-cols-2">
        {services.length > 0 ? services.map((service) => <CompactServicePill key={service.name} service={service} />) : (
          <div className="rounded-md border border-panel-line bg-slate-950/35 px-3 py-3 text-sm text-slate-500 sm:col-span-2">{t("monitoringNoSamples")}</div>
        )}
      </div>
    </div>
  );
}

function CompactServicePill({ service }: { service: PlatformService }) {
  const { t } = useI18n();
  const healthy = service.status === "healthy";
  return (
    <div className={cn("rounded-md border px-3 py-2", healthy ? "border-panel-line bg-slate-950/35" : "border-panel-gold/25 bg-panel-gold/5")}>
      <div className="flex items-center justify-between gap-3">
        <span className="truncate text-sm font-medium text-slate-200">{platformServiceLabel(service.name)}</span>
        <span className={cn("size-2 rounded-full", healthy ? "bg-panel-green" : "bg-panel-gold")} />
      </div>
      <p className="mt-1 truncate text-xs text-slate-500">{healthy ? t("healthHealthy") : healthStatusValue(service.status, t)}</p>
      {!healthy && service.lastError ? <p className="mt-1 line-clamp-1 text-xs text-panel-gold">{service.lastError}</p> : null}
    </div>
  );
}

function compactPlatformServices(services: PlatformService[]) {
  const rank = (status: string) => status === "down" ? 3 : status === "degraded" ? 2 : status === "healthy" ? 0 : 1;
  const byName = new Map<string, PlatformService>();
  for (const service of services) {
    const current = byName.get(service.name);
    if (!current || rank(service.status) > rank(current.status)) {
      byName.set(service.name, service);
    }
  }
  const preferred = ["gamepanel-api", "gamepanel-exporter", "prometheus", "cadvisor", "node-exporter"];
  return [
    ...preferred.map((name) => byName.get(name)).filter(Boolean),
    ...Array.from(byName.values()).filter((service) => !preferred.includes(service.name))
  ] as PlatformService[];
}

function platformServiceLabel(name: string) {
  const labels: Record<string, string> = {
    cadvisor: "cAdvisor",
    "gamepanel-api": "API",
    "gamepanel-exporter": "Exporter",
    "node-exporter": "Node Exporter",
    prometheus: "Prometheus"
  };
  return labels[name] ?? name;
}

function healthStatusValue(value: string, t: ReturnType<typeof useI18n>["t"]) {
  if (value === "healthy") return t("healthHealthy");
  if (value === "degraded") return t("healthDegraded");
  if (value === "down") return t("healthDown");
  if (value === "warning") return t("healthWarning");
  if (value === "critical") return t("healthCritical");
  return value;
}

function TechBadge({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex h-9 items-center gap-2 rounded-md border border-panel-line bg-slate-900/55 px-3 text-xs">
      <RadioTower aria-hidden="true" className="size-3.5 text-slate-500" />
      <span className="text-slate-500">{label}</span>
      <span className="font-mono font-medium text-slate-200">{value}</span>
    </span>
  );
}

function MetricGroupHeader({ description, meta, title }: { description: string; meta?: ReactNode; title: string }) {
  return (
    <div className="rounded-lg border border-panel-line bg-slate-950/35 px-4 py-3 md:col-span-2">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold text-slate-100">{title}</h2>
          <p className="mt-1 text-xs text-slate-500">{description}</p>
        </div>
        {meta ? <div className="flex flex-wrap gap-2">{meta}</div> : null}
      </div>
    </div>
  );
}

function formatOverviewTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function formatPercent(value: number | null | undefined) {
  if (value == null || !Number.isFinite(value)) return "0%";
  return `${Math.round(value)}%`;
}

function formatMemoryShort(value: number | null | undefined) {
  if (value == null || !Number.isFinite(value)) return "0 MB";
  if (value >= 1024) return `${(value / 1024).toFixed(1)} GB`;
  return `${Math.round(value)} MB`;
}

function formatMemoryPair(value: number | null | undefined, limit: number | null | undefined) {
  const used = formatMemoryShort(value);
  if (limit == null || !Number.isFinite(limit) || limit <= 0) return used;
  return `${used} / ${formatMemoryShort(limit)}`;
}

function filterEvents(events: MonitoringEvent[], search: string) {
  const term = search.trim().toLowerCase();
  if (!term) return events;
  return events.filter((event) => [event.title, event.message, event.serverName, event.type, event.severity].filter(Boolean).join(" ").toLowerCase().includes(term));
}

function gameOptions(rows: { gameKey: string }[], t: (key: MessageKey) => string): { key: string; label?: string; labelKey?: MessageKey }[] {
  const keys = Array.from(new Set(rows.map((row) => row.gameKey).filter(Boolean))).sort();
  return [{ key: "all", labelKey: "filterAll" }, ...keys.map((key) => ({ key, label: gameDisplayName(key, key, t) }))];
}
