"use client";

import Link from "next/link";
import { Bookmark, Cpu, MemoryStick, Plus, Trash2 } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { PageHeader } from "@/components/page-header";
import { Badge, Button, Card } from "@/components/ui";
import { deleteConfigPreset, listConfigPresets, listGames, listModPacks } from "@/lib/api";
import { gameFilterOptions } from "@/lib/game-filters";
import { localizeRelativeTime, useI18n, type MessageKey } from "@/lib/i18n";
import { providerFilterOptions } from "@/lib/provider-filters";
import type { ConfigPreset, GameCatalogEntry, ModPack, ProviderCatalog } from "@/lib/types";

type PresetGameFilter = "all" | string;
type PresetProviderFilter = "all" | string;

export default function PresetsPage() {
  const { locale, t } = useI18n();
  const client = useQueryClient();
  const presetsQuery = useQuery({ queryKey: ["config-presets"], queryFn: listConfigPresets, retry: false });
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false, staleTime: 5 * 60 * 1000 });
  const modPacksQuery = useQuery({ queryKey: ["mod-packs"], queryFn: listModPacks, retry: false });
  const [search, setSearch] = useState("");
  const [gameFilter, setGameFilter] = useState<PresetGameFilter>("all");
  const [providerFilter, setProviderFilter] = useState<PresetProviderFilter>("all");
  const [pendingDelete, setPendingDelete] = useState<ConfigPreset | null>(null);
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const presets = presetsQuery.data ?? [];
  const games = gamesQuery.data ?? [];
  const modPacks = modPacksQuery.data ?? [];
  const gameFilters = useMemo(
    () => gameFilterOptions(games, t("filterAll"), presets.map((preset) => preset.gameKey), t),
    [games, presets, t]
  );
  const providerFilters = useMemo(
    () => providerFilterOptions(games, t("filterAll"), presets.map((preset) => preset.providerKey), gameFilter),
    [gameFilter, games, presets, t]
  );
  useEffect(() => {
    if (providerFilter !== "all" && !providerFilters.some((option) => option.key === providerFilter)) {
      setProviderFilter("all");
    }
  }, [providerFilter, providerFilters]);
  const context = useMemo(() => buildPresetContext(games, modPacks), [games, modPacks]);
  const filteredPresets = useMemo(() => {
    const term = search.trim().toLowerCase();
    return presets
      .filter((preset) => {
        const meta = presetMeta(preset, context);
        const matchesGame = gameFilter === "all" || preset.gameKey === gameFilter;
        const matchesProvider = providerFilter === "all" || preset.providerKey === providerFilter;
        const matchesSearch = !term || [preset.name, meta.gameName, meta.providerName, meta.modPackName, preset.version ?? ""].some((value) => value.toLowerCase().includes(term));
        return matchesGame && matchesProvider && matchesSearch;
      })
      .sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime());
  }, [context, gameFilter, presets, providerFilter, search]);
  const activeFilterChips = [
    search.trim(),
    gameFilter !== "all" ? filterOptionLabel(gameFilters, gameFilter, t) : "",
    providerFilter !== "all" ? filterOptionLabel(providerFilters, providerFilter, t) : ""
  ].filter(Boolean);

  const remove = useMutation({
    mutationFn: deleteConfigPreset,
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("configurationPresetDeleted"));
      setPendingDelete(null);
      await client.invalidateQueries({ queryKey: ["config-presets"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableDeleteConfigurationPreset"));
    }
  });

  return (
    <>
      <PageHeader title={t("configurationPresets")} />
      {(presetsQuery.isError || gamesQuery.isError) && <p className="mb-4 text-sm text-panel-gold">{t("apiConfigurationPresetsUnavailable")}</p>}
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      {successMessage && <p className="mb-4 text-sm text-panel-green">{successMessage}</p>}

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
        resultLabel={t("configurationPresetFilterSummary", { shown: filteredPresets.length, total: presets.length })}
        search={search}
        searchPlaceholder={t("searchConfigurationPresets")}
      />

      {filteredPresets.length > 0 ? (
        <div className="grid gap-4 xl:grid-cols-2">
          {filteredPresets.map((preset) => {
            const meta = presetMeta(preset, context);
            return (
              <Card key={preset.id} className="group p-4 transition hover:border-panel-green/40">
                <div className="flex items-start justify-between gap-4">
                  <div className="flex min-w-0 items-start gap-3">
                    <span className="flex size-10 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/70 text-panel-green">
                      <Bookmark aria-hidden="true" className="size-5" />
                    </span>
                    <div className="min-w-0">
                      <h2 className="truncate font-semibold text-white">{preset.name}</h2>
                      <p className="mt-1 truncate text-sm text-slate-400">{meta.gameName} · {meta.providerName}</p>
                    </div>
                  </div>
                  <Badge className="shrink-0 bg-slate-800 text-slate-300">{preset.version || t("recommended")}</Badge>
                </div>

                <div className="mt-4 grid gap-2 sm:grid-cols-2">
                  <DetailTile label={t("game")} value={meta.gameName} />
                  <DetailTile label={t("serverType")} value={meta.providerName} />
                  <DetailTile label={t("gameVersion")} value={preset.version || t("recommended")} />
                  <DetailTile label={t("modPack")} value={meta.modPackName || t("none")} />
                </div>

                <div className="mt-4 flex flex-wrap gap-2 text-xs text-slate-400">
                  <span className="inline-flex items-center gap-1 rounded border border-panel-line bg-slate-950/45 px-2 py-1">
                    <Cpu aria-hidden="true" className="size-3.5 text-panel-green" />
                    {t("cpuLimit")}: {formatCpuLimit(preset.cpuLimitCores, t)}
                  </span>
                  <span className="inline-flex items-center gap-1 rounded border border-panel-line bg-slate-950/45 px-2 py-1">
                    <MemoryStick aria-hidden="true" className="size-3.5 text-panel-green" />
                    {t("memoryLimit")}: {formatMemoryLimit(preset.memoryLimitMb, t)}
                  </span>
                  <span className="inline-flex items-center rounded border border-panel-line bg-slate-950/45 px-2 py-1">
                    {t("modified")}: {localizeRelativeTime(preset.updatedAt, locale)}
                  </span>
                </div>

                <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-panel-line pt-3">
                  <Link
                    className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-panel-green/40 bg-panel-green/10 px-3 text-sm font-medium text-panel-green transition hover:bg-panel-green/15 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
                    href={`/servers/new?presetId=${encodeURIComponent(preset.id)}`}
                  >
                    <Plus aria-hidden="true" className="size-4" />
                    {t("createServerFromPreset")}
                  </Link>
                  <Button
                    className="h-9 text-red-200 hover:bg-red-500/15"
                    variant="ghost"
                    onClick={() => setPendingDelete(preset)}
                    disabled={remove.isPending || presetsQuery.isError}
                    aria-label={t("delete")}
                  >
                    <Trash2 aria-hidden="true" className="size-4" />
                  </Button>
                </div>
              </Card>
            );
          })}
        </div>
      ) : null}

      {!presetsQuery.isLoading && filteredPresets.length === 0 && (
        <p className="mt-4 text-sm text-slate-400">{presets.length === 0 ? t("noConfigurationPresetsYet") : t("noConfigurationPresetsMatch")}</p>
      )}

      <ConfirmDialog
        open={Boolean(pendingDelete)}
        eyebrow={t("destructiveAction")}
        title={t("deleteConfigurationPresetConfirm", { name: pendingDelete?.name ?? "" })}
        description={t("confirmDeleteConfigurationPresetDescription", { name: pendingDelete?.name ?? "" })}
        detail={pendingDelete ? (
          <>
            <span className="text-slate-500">{t("configurationPreset")}: </span>
            <span className="font-medium text-white">{pendingDelete.name}</span>
          </>
        ) : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={remove.isPending ? t("actionWorking") : t("delete")}
        busy={remove.isPending}
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => pendingDelete && remove.mutate(pendingDelete.id)}
      />
    </>
  );
}

function DetailTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md border border-panel-line bg-slate-950/45 px-3 py-2">
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 truncate text-sm font-medium text-slate-100">{value}</p>
    </div>
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

function buildPresetContext(games: GameCatalogEntry[], modPacks: ModPack[]) {
  return {
    games: new Map(games.map((game) => [game.key, game])),
    providers: new Map(games.flatMap((game) => game.providers.map((provider) => [provider.key, provider] as const))),
    modPacks: new Map(modPacks.map((pack) => [pack.id, pack]))
  };
}

function presetMeta(
  preset: ConfigPreset,
  context: {
    games: Map<string, GameCatalogEntry>;
    providers: Map<string, ProviderCatalog>;
    modPacks: Map<string, ModPack>;
  }
) {
  const game = context.games.get(preset.gameKey);
  const provider = context.providers.get(preset.providerKey);
  const modPack = preset.modPackId ? context.modPacks.get(preset.modPackId) : undefined;
  return {
    gameName: game?.name ?? formatKey(preset.gameKey),
    providerName: provider?.name ?? formatKey(preset.providerKey),
    modPackName: modPack?.name ?? ""
  };
}

function formatKey(value: string) {
  return value
    .split("-")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function formatCpuLimit(value: number | undefined, t: (key: MessageKey, values?: Record<string, string | number>) => string) {
  return value && value > 0 ? t("cpuCoresValue", { cores: value }) : t("unlimited");
}

function formatMemoryLimit(value: number | undefined, t: (key: MessageKey, values?: Record<string, string | number>) => string) {
  return value && value > 0 ? t("memoryGbValue", { gb: value / 1024 }) : t("unlimited");
}
