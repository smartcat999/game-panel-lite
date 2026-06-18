"use client";

import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/page-header";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { ServerCard } from "@/components/server-card";
import { listBackups, listGames, listServers } from "@/lib/api";
import { gameFilterOptions } from "@/lib/game-filters";
import { useI18n } from "@/lib/i18n";
import type { MessageKey } from "@/lib/i18n";
import { providerFilterOptions } from "@/lib/provider-filters";
import { filterServers, type ServerGameFilter, type ServerProviderFilter, type ServerStatusFilter } from "@/lib/server-filters";
import { attachLatestBackupTimes } from "@/lib/server-metrics";

const statusFilters = [
  { key: "all", labelKey: "filterAll" },
  { key: "running", labelKey: "filterRunning" },
  { key: "stopped", labelKey: "filterStopped" }
] as const satisfies readonly { key: ServerStatusFilter; labelKey: MessageKey }[];

export default function ServersPage() {
  const query = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false, refetchInterval: 5000 });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, retry: false });
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false, staleTime: 5 * 60 * 1000 });
  const { t } = useI18n();
  const [gameFilter, setGameFilter] = useState<ServerGameFilter>("all");
  const [statusFilter, setStatusFilter] = useState<ServerStatusFilter>("all");
  const [providerFilter, setProviderFilter] = useState<ServerProviderFilter>("all");
  const [search, setSearch] = useState("");
  const servers = useMemo(() => attachLatestBackupTimes(query.data ?? [], backupsQuery.data ?? []), [backupsQuery.data, query.data]);
  const gameFilters = useMemo(
    () => gameFilterOptions(gamesQuery.data ?? [], t("filterAll"), servers.map((server) => server.gameKey)),
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
    return filterServers(servers, { game: gameFilter, provider: providerFilter, query: search, status: statusFilter });
  }, [gameFilter, providerFilter, search, servers, statusFilter]);
  const activeFilterChips = [
    search.trim(),
    gameFilter !== "all" ? filterOptionLabel(gameFilters, gameFilter, t) : "",
    statusFilter !== "all" ? filterOptionLabel(statusFilters, statusFilter, t) : "",
    providerFilter !== "all" ? filterOptionLabel(providerFilters, providerFilter, t) : ""
  ].filter(Boolean);
  return (
    <>
      <PageHeader title={t("serversTitle")} />
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
      {(query.isError || backupsQuery.isError) && <p className="mb-4 text-sm text-panel-gold">{query.isError ? t("apiServersUnavailable") : t("apiBackupsUnavailable")}</p>}
      <div className="grid gap-4 xl:grid-cols-2">
        {filteredServers.map((server) => <ServerCard key={server.id} server={server} />)}
      </div>
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
