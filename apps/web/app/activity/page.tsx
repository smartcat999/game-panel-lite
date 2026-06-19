"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Activity, Cpu, Gauge, HardDrive, MemoryStick, Network, RadioTower, Server, Users } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import {
  ActivityTimeline,
  ChartIcon,
  HealthStatusCard,
  KpiStrip,
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
import type { MonitoringEvent } from "@/features/monitoring/types";
import { useI18n, type MessageKey } from "@/lib/i18n";

type FilterValue = "all" | string;
type MonitoringSection = "overview" | "server-load" | "activity-log";

const severityOptions = ["all", "error", "warning", "success", "info"] as const;
const eventTypeOptions = ["all", "server", "backup", "world", "mod", "player", "settings", "system"] as const;

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

  const events = useMemo(() => filterEvents(eventsQuery.data?.events ?? [], search), [eventsQuery.data?.events, search]);
  const games = useMemo(() => gameOptions(loadQuery.data?.rows ?? []), [loadQuery.data?.rows]);
  const activeChips = [search.trim(), severity !== "all" ? severity : "", eventType !== "all" ? eventType : "", game !== "all" ? game : ""].filter(Boolean);
  const anyError = overviewQuery.isError || metricsQuery.isError || loadQuery.isError || eventsQuery.isError || platformQuery.isError;

  return (
    <>
      <PageHeader
        title={t("monitoringTitle")}
        description={t("monitoringDescription")}
        action={
          <div className="flex flex-wrap justify-end gap-2">
            <TechBadge label={t("monitoringRange")} value="15m" />
            <TechBadge label={t("monitoringRefresh")} value="5s" />
            <SourceBadge connected={overviewQuery.data?.dataSource.connected} />
          </div>
        }
      />
      {anyError ? <p className="mb-4 text-sm text-panel-gold">{t("monitoringApiPartialUnavailable")}</p> : null}

      <div className="space-y-5">
        <MonitoringSectionNav
          active={section}
          counts={{
            "activity-log": eventsQuery.data?.events.length ?? 0,
            overview: overviewQuery.data?.kpis.issues ?? 0,
            "server-load": loadQuery.data?.rows.length ?? 0
          }}
          onChange={setSection}
        />

        {section === "overview" ? (
          <>
            <KpiStrip overview={overviewQuery.data} />
            <HealthStatusCard overview={overviewQuery.data} />

            <section className="grid gap-4 md:grid-cols-2">
              <MetricGroupHeader title={t("serverResourceTitle")} description={t("serverResourceDescription")} />
              <MonitoringChartCard color="#59d46f" icon={<Cpu aria-hidden="true" className="size-4" />} series={metricsQuery.data?.series.cpu} />
              <MonitoringChartCard color="#a873ff" icon={<MemoryStick aria-hidden="true" className="size-4" />} series={metricsQuery.data?.series.memory} />
              <MonitoringChartCard color="#59d46f" icon={<Users aria-hidden="true" className="size-4" />} series={metricsQuery.data?.series.players} />
              <MonitoringChartCard color="#e6b84a" icon={<Activity aria-hidden="true" className="size-4" />} series={metricsQuery.data?.series.events} />
            </section>

            <section className="grid gap-4 md:grid-cols-2">
              <MetricGroupHeader title={t("nodeResourceTitle")} description={t("nodeResourceDescription")} />
              <MonitoringChartCard color="#7dd3fc" icon={<Server aria-hidden="true" className="size-4" />} series={platformQuery.data?.series.nodeCpu} />
              <MonitoringChartCard color="#a873ff" icon={<MemoryStick aria-hidden="true" className="size-4" />} series={platformQuery.data?.series.nodeMemory} />
              <MonitoringChartCard color="#e6b84a" icon={<HardDrive aria-hidden="true" className="size-4" />} series={platformQuery.data?.series.nodeDisk} />
              <MonitoringChartCard color="#59d46f" icon={<Network aria-hidden="true" className="size-4" />} series={platformQuery.data?.series.nodeNetwork} />
            </section>

            <section className="grid gap-4 md:grid-cols-2">
              <MetricGroupHeader title={t("platformTrafficTitle")} description={t("platformTrafficDescription")} />
              <MonitoringChartCard color="#7dd3fc" icon={<ChartIcon type="requests" />} series={platformQuery.data?.series.requests} />
              <MonitoringChartCard color="#ff6b6b" icon={<Gauge aria-hidden="true" className="size-4" />} series={platformQuery.data?.series.latencyP95} />
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
  const tabs: { id: MonitoringSection; label: string; note: string; countLabel: string }[] = [
    { id: "overview", label: t("monitoringNavOverview"), note: t("monitoringNavOverviewNote"), countLabel: t("monitoringNavIssues", { count: counts.overview }) },
    { id: "server-load", label: t("monitoringNavServerLoad"), note: t("monitoringNavServerLoadNote"), countLabel: t("monitoringNavServers", { count: counts["server-load"] }) },
    { id: "activity-log", label: t("monitoringNavActivityLog"), note: t("monitoringNavActivityLogNote"), countLabel: t("monitoringNavEvents", { count: counts["activity-log"] }) }
  ];

  return (
    <div className="rounded-lg border border-panel-line bg-panel-card p-2">
      <div className="grid gap-2 lg:grid-cols-3" role="tablist" aria-label={t("monitoringTitle")}>
        {tabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={active === tab.id}
            className={[
              "rounded-md border px-3 py-3 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-green/40",
              active === tab.id
                ? "border-panel-green/35 bg-panel-green/10 text-slate-100"
                : "border-transparent text-slate-400 hover:border-panel-line hover:bg-slate-950/35 hover:text-slate-200"
            ].join(" ")}
            onClick={() => onChange(tab.id)}
          >
            <span className="flex items-center justify-between gap-3">
              <span className="text-sm font-semibold">{tab.label}</span>
              <span className="rounded border border-panel-line bg-slate-950/45 px-2 py-0.5 font-mono text-xs text-slate-400">{tab.countLabel}</span>
            </span>
            <span className="mt-1 block text-xs text-slate-500">{tab.note}</span>
          </button>
        ))}
      </div>
    </div>
  );
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

function MetricGroupHeader({ description, title }: { description: string; title: string }) {
  return (
    <div className="rounded-lg border border-panel-line bg-slate-950/35 px-4 py-3 md:col-span-2">
      <h2 className="text-sm font-semibold text-slate-100">{title}</h2>
      <p className="mt-1 text-xs text-slate-500">{description}</p>
    </div>
  );
}

function filterEvents(events: MonitoringEvent[], search: string) {
  const term = search.trim().toLowerCase();
  if (!term) return events;
  return events.filter((event) => [event.title, event.message, event.serverName, event.type, event.severity].filter(Boolean).join(" ").toLowerCase().includes(term));
}

function gameOptions(rows: { gameKey: string }[]): { key: string; label?: string; labelKey?: MessageKey }[] {
  const keys = Array.from(new Set(rows.map((row) => row.gameKey).filter(Boolean))).sort();
  return [{ key: "all", labelKey: "filterAll" }, ...keys.map((key) => ({ key, label: key }))];
}
