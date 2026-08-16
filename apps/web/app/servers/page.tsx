"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { Plus } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/page-header";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { ServerResourceTable } from "@/components/server-resource-table";
import { Button } from "@/components/ui";
import { getObservabilityMetrics, getSettings, listGameServers, listGames } from "@/lib/api";
import { gameFilterOptions } from "@/lib/game-filters";
import { useI18n } from "@/lib/i18n";
import type { MessageKey } from "@/lib/i18n";
import { providerFilterOptions } from "@/lib/provider-filters";
import { filterGameServers, type ServerGameFilter, type ServerProviderFilter, type ServerStatusFilter } from "@/lib/server-filters";

const statusFilters = [
  { key: "all", labelKey: "filterAll" },
  { key: "running", labelKey: "filterRunning" },
  { key: "stopped", labelKey: "filterStopped" }
] as const satisfies readonly { key: ServerStatusFilter; labelKey: MessageKey }[];

export default function ServersPage() {
  const query = useQuery({ queryKey: ["game-servers"], queryFn: listGameServers, retry: false, refetchInterval: 5000 });
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false, staleTime: 5 * 60 * 1000 });
  const metricsQuery = useQuery({ queryKey: ["observability-metrics"], queryFn: getObservabilityMetrics, retry: false, refetchInterval: 5000 });
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false, staleTime: 5 * 60 * 1000 });
  const { t } = useI18n();
  const [gameFilter, setGameFilter] = useState<ServerGameFilter>("all");
  const [statusFilter, setStatusFilter] = useState<ServerStatusFilter>("all");
  const [providerFilter, setProviderFilter] = useState<ServerProviderFilter>("all");
  const [search, setSearch] = useState("");
  const servers = query.data ?? [];
  const gameFilters = useMemo(
    () => gameFilterOptions(gamesQuery.data ?? [], t("filterAll"), servers.map((server) => server.gameKey), t),
    [gamesQuery.data, servers, t]
  );
  const providerFilters = useMemo(
    () => providerFilterOptions(gamesQuery.data ?? [], t("filterAll"), servers.map((server) => server.providerKey), gameFilter),
    [gameFilter, gamesQuery.data, servers, t]
  );
  useEffect(() => {
    if (providerFilter !== "all" && !providerFilters.some((option) => option.key === providerFilter)) {
      setProviderFilter("all");
    }
  }, [providerFilter, providerFilters]);
  useEffect(() => {
    setSearch(new URLSearchParams(window.location.search).get("search") ?? "");
  }, []);
  const filteredServers = useMemo(() => {
    return filterGameServers(servers, { game: gameFilter, provider: providerFilter, query: search, status: statusFilter });
  }, [gameFilter, providerFilter, search, servers, statusFilter]);
  const activeFilterChips = [
    search.trim(),
    gameFilter !== "all" ? filterOptionLabel(gameFilters, gameFilter, t) : "",
    statusFilter !== "all" ? filterOptionLabel(statusFilters, statusFilter, t) : "",
    providerFilter !== "all" ? filterOptionLabel(providerFilters, providerFilter, t) : ""
  ].filter(Boolean);
  return (
    <>
      <PageHeader
        title={t("serversTitle")}
        action={<Link href="/servers/new"><Button className="h-10 px-4"><Plus aria-hidden="true" className="size-4" />{t("createServer")}</Button></Link>}
      />
      <ResourceFilterBar
        activeChips={activeFilterChips}
        clearLabel={t("clearFilters")}
        density="compact"
        filters={[
          { label: t("filterGame"), options: gameFilters, value: gameFilter, onChange: (value) => setGameFilter(value) },
          { label: t("filterStatus"), options: statusFilters, value: statusFilter, onChange: (value) => setStatusFilter(value as ServerStatusFilter) },
          { label: t("filterType"), options: providerFilters, value: providerFilter, onChange: (value) => setProviderFilter(value) }
        ]}
        onClear={() => {
          setGameFilter("all");
          setStatusFilter("all");
          setProviderFilter("all");
          setSearch("");
        }}
        onSearchChange={setSearch}
        resultLabel={t("searchResultsCount", { count: filteredServers.length })}
        search={search}
        searchPlaceholder={t("searchServers")}
      />
      {query.isError && <p className="mb-4 text-sm text-panel-gold">{t("apiServersUnavailable")}</p>}
      {filteredServers.length > 0 ? <ServerResourceTable servers={filteredServers} metrics={metricsQuery.data?.servers} publicHost={settingsQuery.data?.publicHost} /> : null}
      {filteredServers.length === 0 && <p className="mt-6 text-sm text-slate-400">{query.isLoading ? t("loading") : t("noServersMatch")}</p>}
    </>
  );
}

function filterOptionLabel<T extends string>(
  options: readonly { key: T; labelKey?: MessageKey; label?: string }[],
  value: T,
  t: (key: MessageKey) => string
) {
  const option = options.find((item) => item.key === value);
  return option?.labelKey ? t(option.labelKey) : option?.label ?? value;
}
