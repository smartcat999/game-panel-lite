"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/page-header";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { Card } from "@/components/ui";
import { listGameServers, listGames, listWorlds } from "@/lib/api";
import { showWorldAndBackupFeatures } from "@/lib/feature-flags";
import { gameFilterOptions, gameKeyFromProvider } from "@/lib/game-filters";
import { useI18n, type MessageKey } from "@/lib/i18n";
import { providerFilterOptions } from "@/lib/provider-filters";
import { WorldMigrationHub } from "@/components/world-migration-hub";
import { WorldRadarGrid } from "@/components/world-radar-grid";
import { GameAssetsSubNav } from "@/components/sub-nav";

type WorldGameFilter = "all" | string;
type WorldProviderFilter = "all" | string;

export default function WorldsPage() {
  if (!showWorldAndBackupFeatures) return <HiddenFeaturePage />;
  return <EnabledWorldsPage />;
}

function EnabledWorldsPage() {
  const { t } = useI18n();
  const query = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, retry: false });
  const serversQuery = useQuery({ queryKey: ["game-servers"], queryFn: listGameServers, retry: false });
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false, staleTime: 5 * 60 * 1000 });

  const [gameFilter, setGameFilter] = useState<WorldGameFilter>("all");
  const [providerFilter, setProviderFilter] = useState<WorldProviderFilter>("all");
  const [search, setSearch] = useState("");

  const worlds = query.data ?? [];
  const servers = serversQuery.data ?? [];

  const gameFilters = useMemo(
    () => gameFilterOptions(gamesQuery.data ?? [], t("filterAll"), worlds.map((world) => world.gameKey ?? gameKeyFromProvider(world.providerKey)), t),
    [gamesQuery.data, t, worlds]
  );
  const providerFilters = useMemo(
    () => providerFilterOptions(gamesQuery.data ?? [], t("filterAll"), worlds.map((world) => world.providerKey), gameFilter),
    [gameFilter, gamesQuery.data, t, worlds]
  );

  useEffect(() => {
    if (providerFilter !== "all" && !providerFilters.some((option) => option.key === providerFilter)) {
      setProviderFilter("all");
    }
  }, [providerFilter, providerFilters]);

  const filteredWorlds = useMemo(() => {
    const term = search.trim().toLowerCase();
    return worlds.filter((world) => {
      const matchesSearch = !term || [world.name, world.size, world.bytes].some((value) => value?.toLowerCase().includes(term));
      const worldGame = world.gameKey ?? gameKeyFromProvider(world.providerKey);
      const matchesGame = gameFilter === "all" || worldGame === gameFilter;
      const matchesProvider = providerFilter === "all" || world.providerKey === providerFilter;
      return matchesSearch && matchesGame && matchesProvider;
    });
  }, [gameFilter, providerFilter, search, worlds]);

  const activeFilterChips = [
    search.trim(),
    gameFilter !== "all" ? filterOptionLabel(gameFilters, gameFilter, t) : "",
    providerFilter !== "all" ? filterOptionLabel(providerFilters, providerFilter, t) : ""
  ].filter(Boolean);

  return (
    <>
      <PageHeader title={t("worldsTitle")} />
      <GameAssetsSubNav />

      {/* 1. World Migration Hub */}
      <WorldMigrationHub servers={servers} />

      {/* 2. Filter Bar */}
      <ResourceFilterBar
        activeChips={activeFilterChips}
        clearLabel={t("clearFilters")}
        density="compact"
        filters={[
          { label: t("filterGame"), options: gameFilters, value: gameFilter, onChange: (value) => setGameFilter(value) },
          { label: t("filterType"), options: providerFilters, value: providerFilter, onChange: (value) => setProviderFilter(value) }
        ]}
        onClear={() => {
          setGameFilter("all");
          setProviderFilter("all");
          setSearch("");
        }}
        onSearchChange={setSearch}
        search={search}
        searchPlaceholder={t("searchWorlds")}
      />

      {query.isError && <p className="mb-4 text-sm text-panel-gold">{t("apiWorldsUnavailable")}</p>}

      {/* 3. World Radar Grid */}
      <WorldRadarGrid worlds={filteredWorlds} servers={servers} />
    </>
  );
}

function HiddenFeaturePage() {
  return (
    <Card className="p-6">
      <h1 className="text-xl font-semibold text-white">Page not found</h1>
      <p className="mt-2 text-sm text-slate-400">The requested GamePanel Lite page does not exist.</p>
      <Link className="mt-4 inline-flex text-sm font-medium text-panel-green hover:underline" href="/dashboard">
        Back to dashboard
      </Link>
    </Card>
  );
}

function filterOptionLabel<T extends string>(
  options: readonly { key: T; labelKey?: MessageKey; label?: string }[],
  value: T,
  t: (key: MessageKey, params?: Record<string, string | number>) => string
) {
  const option = options.find((item) => item.key === value);
  if (!option) return value;
  return option.labelKey ? t(option.labelKey) : option.label ?? value;
}
