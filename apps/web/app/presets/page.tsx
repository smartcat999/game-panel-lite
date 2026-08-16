"use client";

import Link from "next/link";
import { Eye, Plus, Trash2, X } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { PageHeader } from "@/components/page-header";
import { Badge, Button, ToastNotice } from "@/components/ui";
import { deleteConfigPreset, listConfigPresets, listGames, listModPacks } from "@/lib/api";
import { gameFilterOptions } from "@/lib/game-filters";
import { localizeRelativeTime, useI18n, type Locale, type MessageKey } from "@/lib/i18n";
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
  const [selectedPreset, setSelectedPreset] = useState<ConfigPreset | null>(null);
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
      {(errorMessage || successMessage) && (
        <div className="pointer-events-none fixed inset-x-4 bottom-4 z-[60] flex justify-end md:inset-x-auto md:bottom-auto md:right-6 md:top-24">
          <ToastNotice
            closeLabel={t("cancel")}
            message={errorMessage || successMessage}
            tone={errorMessage ? "error" : "success"}
            onClose={() => {
              setErrorMessage("");
              setSuccessMessage("");
            }}
          />
        </div>
      )}

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
        <PresetResourceTable
          context={context}
          locale={locale}
          presets={filteredPresets}
          onInspect={setSelectedPreset}
          t={t}
        />
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
      {selectedPreset ? (
        <PresetDetailsDrawer
          context={context}
          locale={locale}
          preset={selectedPreset}
          t={t}
          onClose={() => setSelectedPreset(null)}
          onDelete={() => {
            setSelectedPreset(null);
            setPendingDelete(selectedPreset);
          }}
        />
      ) : null}
    </>
  );
}

function PresetResourceTable({
  context,
  locale,
  presets,
  onInspect,
  t
}: {
  context: ReturnType<typeof buildPresetContext>;
  locale: Locale;
  presets: ConfigPreset[];
  onInspect: (preset: ConfigPreset) => void;
  t: (key: MessageKey, values?: Record<string, string | number>) => string;
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-panel-line bg-panel-card">
      <div className="hidden overflow-x-auto md:block">
        <table className="w-full min-w-[900px] border-collapse text-left text-sm">
          <thead className="bg-slate-950/45 text-xs font-medium text-slate-500">
            <tr>
              <th className="px-4 py-3">{t("configurationPreset")}</th>
              <th className="px-3 py-3">{t("serverType")}</th>
              <th className="px-3 py-3">{t("gameVersion")}</th>
              <th className="px-3 py-3">{t("runtimeResources")}</th>
              <th className="px-3 py-3">{t("modsTitle")}</th>
              <th className="px-3 py-3">{t("modified")}</th>
              <th className="px-4 py-3 text-right">{t("actions")}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-panel-line">
            {presets.map((preset) => {
              const meta = presetMeta(preset, context);
              const modIds = preset.modIds ?? [];
              return (
                <tr className="group transition-colors hover:bg-slate-800/35" key={preset.id}>
                  <td className="px-4 py-3">
                    <button className="max-w-72 truncate text-left font-medium text-slate-100 group-hover:text-panel-green" onClick={() => onInspect(preset)} type="button">
                      {preset.name}
                    </button>
                  </td>
                  <td className="px-3 py-3">
                    <p className="text-slate-200">{meta.gameName}</p>
                    <p className="mt-0.5 text-xs text-slate-500">{meta.providerName}</p>
                  </td>
                  <td className="px-3 py-3"><Badge className="bg-slate-800 text-slate-300">{preset.version || t("recommended")}</Badge></td>
                  <td className="px-3 py-3 font-mono text-xs text-slate-300">
                    {formatCpuLimit(preset.cpuLimitCores, t)} · {formatMemoryLimit(preset.memoryLimitMb, t)}
                  </td>
                  <td className="px-3 py-3 text-slate-300">
                    {modIds.length > 0 ? [meta.modPackName, t("selectedModsCount", { count: modIds.length })].filter(Boolean).join(" · ") : t("none")}
                  </td>
                  <td className="px-3 py-3 text-slate-400">{localizeRelativeTime(preset.updatedAt, locale)}</td>
                  <td className="px-4 py-3">
                    <div className="flex justify-end gap-2">
                      <Button className="h-8 px-2.5 text-xs" variant="ghost" onClick={() => onInspect(preset)}>
                        <Eye aria-hidden="true" className="size-3.5" />{t("viewDetails")}
                      </Button>
                      <Link className="inline-flex h-8 items-center gap-1.5 rounded-md border border-panel-green/35 bg-panel-green/10 px-2.5 text-xs font-medium text-panel-green hover:bg-panel-green/15" href={`/servers/new?presetId=${encodeURIComponent(preset.id)}`}>
                        <Plus aria-hidden="true" className="size-3.5" />{t("createServer")}
                      </Link>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <div className="divide-y divide-panel-line md:hidden">
        {presets.map((preset) => {
          const meta = presetMeta(preset, context);
          const modIds = preset.modIds ?? [];
          return (
            <button className="block w-full px-4 py-4 text-left hover:bg-slate-800/35" key={preset.id} onClick={() => onInspect(preset)} type="button">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate font-medium text-slate-100">{preset.name}</p>
                  <p className="mt-1 truncate text-xs text-slate-500">{meta.gameName} · {meta.providerName}</p>
                </div>
                <Badge className="shrink-0 bg-slate-800 text-slate-300">{preset.version || t("recommended")}</Badge>
              </div>
              <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-400">
                <span>{formatCpuLimit(preset.cpuLimitCores, t)} · {formatMemoryLimit(preset.memoryLimitMb, t)}</span>
                <span>{modIds.length > 0 ? t("selectedModsCount", { count: modIds.length }) : t("none")}</span>
                <span>{localizeRelativeTime(preset.updatedAt, locale)}</span>
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}

function PresetDetailsDrawer({
  context,
  locale,
  preset,
  t,
  onClose,
  onDelete
}: {
  context: ReturnType<typeof buildPresetContext>;
  locale: Locale;
  preset: ConfigPreset;
  t: (key: MessageKey, values?: Record<string, string | number>) => string;
  onClose: () => void;
  onDelete: () => void;
}) {
  const meta = presetMeta(preset, context);
  const modIds = preset.modIds ?? [];
  const mods = modIds.length > 0 ? [meta.modPackName, t("selectedModsCount", { count: modIds.length })].filter(Boolean).join(" · ") : t("none");
  return (
    <div className="fixed inset-0 z-50 bg-slate-950/60" onMouseDown={(event) => event.target === event.currentTarget && onClose()} role="presentation">
      <aside aria-label={t("configurationPreset")} className="ml-auto flex h-full w-full max-w-lg flex-col border-l border-panel-line bg-panel-card shadow-2xl">
        <div className="flex items-start justify-between gap-4 border-b border-panel-line px-5 py-4">
          <div className="min-w-0">
            <p className="text-xs text-slate-500">{t("configurationPreset")}</p>
            <h2 className="mt-1 truncate text-lg font-semibold text-white">{preset.name}</h2>
          </div>
          <button aria-label={t("cancel")} className="rounded-md p-2 text-slate-400 hover:bg-slate-800 hover:text-white" onClick={onClose} type="button"><X aria-hidden="true" className="size-5" /></button>
        </div>
        <div className="flex-1 overflow-y-auto px-5 py-5">
          <dl className="divide-y divide-panel-line border-y border-panel-line">
            <DrawerRow label={t("game")} value={meta.gameName} />
            <DrawerRow label={t("serverType")} value={meta.providerName} />
            <DrawerRow label={t("gameVersion")} value={preset.version || t("recommended")} />
            <DrawerRow label={t("cpuLimit")} value={formatCpuLimit(preset.cpuLimitCores, t)} />
            <DrawerRow label={t("memoryLimit")} value={formatMemoryLimit(preset.memoryLimitMb, t)} />
            <DrawerRow label={t("modsTitle")} value={mods} />
            <DrawerRow label={t("modified")} value={localizeRelativeTime(preset.updatedAt, locale)} />
          </dl>
        </div>
        <div className="flex items-center justify-between gap-3 border-t border-panel-line px-5 py-4">
          <Button className="text-red-200 hover:bg-red-500/15" variant="ghost" onClick={onDelete}><Trash2 aria-hidden="true" />{t("delete")}</Button>
          <Link className="inline-flex h-10 items-center gap-2 rounded-md bg-panel-green px-4 text-sm font-semibold text-slate-950 hover:bg-panel-green/90" href={`/servers/new?presetId=${encodeURIComponent(preset.id)}`}>
            <Plus aria-hidden="true" className="size-4" />{t("createServerFromPreset")}
          </Link>
        </div>
      </aside>
    </div>
  );
}

function DrawerRow({ label, value }: { label: string; value: string }) {
  return <div className="grid grid-cols-[9rem_minmax(0,1fr)] gap-4 py-3 text-sm"><dt className="text-slate-500">{label}</dt><dd className="text-right text-slate-100">{value}</dd></div>;
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
