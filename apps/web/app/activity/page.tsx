"use client";

import { useQuery } from "@tanstack/react-query";
import { Activity as ActivityIcon } from "lucide-react";
import { useMemo, useState } from "react";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { PageHeader } from "@/components/page-header";
import { Card } from "@/components/ui";
import { listActivity, listGames, listServers } from "@/lib/api";
import { formatActivityEvent } from "@/lib/activity-display";
import { gameFilterOptions } from "@/lib/game-filters";
import { localizeRelativeTime, useI18n, type MessageKey } from "@/lib/i18n";

type ActivityGameFilter = "all" | string;

export default function ActivityPage() {
  const { locale, t } = useI18n();
  const query = useQuery({ queryKey: ["activity"], queryFn: listActivity, retry: false });
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false, staleTime: 5 * 60 * 1000 });
  const [search, setSearch] = useState("");
  const [gameFilter, setGameFilter] = useState<ActivityGameFilter>("all");
  const events = query.data ?? [];
  const servers = serversQuery.data ?? [];
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
      <PageHeader title={t("activityTitle")} />
      {(query.isError || serversQuery.isError || gamesQuery.isError) && <p className="mb-4 text-sm text-panel-gold">{t("apiActivityUnavailable")}</p>}
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

function filterOptionLabel<T extends string>(
  options: readonly { key: T; labelKey?: MessageKey; label?: string }[],
  value: T,
  t: (key: MessageKey) => string
) {
  const option = options.find((item) => item.key === value);
  return option?.labelKey ? t(option.labelKey) : option?.label ?? value;
}
