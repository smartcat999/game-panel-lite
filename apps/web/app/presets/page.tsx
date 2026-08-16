"use client";

import Link from "next/link";
import { Box, Cpu, Gamepad2, Pencil, Plus, SlidersHorizontal, Trash2, X } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { ProviderConfigEditor } from "@/components/provider-config-editor";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { PageHeader } from "@/components/page-header";
import { SelectionBox } from "@/components/selection-box";
import { Badge, Button, Input, ToastNotice } from "@/components/ui";
import { deleteConfigPreset, deleteConfigPresets, listConfigPresets, listGames, listModPacks, updateConfigPreset } from "@/lib/api";
import { gameFilterOptions } from "@/lib/game-filters";
import { localizeRelativeTime, useI18n, type Locale, type MessageKey } from "@/lib/i18n";
import { providerFilterOptions } from "@/lib/provider-filters";
import { createDefaultProviderConfigPayload, restoreProviderConfigDefaults, updateProviderConfigPayload, type ProviderConfigPayload } from "@/lib/provider-config";
import { providerOptionLabel } from "@/lib/provider-option-label";
import type { ConfigPreset, GameCatalogEntry, ModPack, ProviderCatalog, ProviderConfigField } from "@/lib/types";

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
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [selectedPreset, setSelectedPreset] = useState<ConfigPreset | null>(null);
  const [editingPreset, setEditingPreset] = useState<ConfigPreset | null>(null);
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

  const removeMany = useMutation({
    mutationFn: (ids: string[]) => deleteConfigPresets(ids),
    onSuccess: async (result) => {
      setBatchDeleteOpen(false);
      setSelectedIds(new Set(result.failed.map((item) => item.id)));
      setErrorMessage(result.failed.length > 0 ? t("configurationPresetBatchDeletePartial", { succeeded: result.succeeded.length, failed: result.failed.length }) : "");
      setSuccessMessage(result.succeeded.length > 0 ? t("configurationPresetsDeleted", { count: result.succeeded.length }) : "");
      await client.invalidateQueries({ queryKey: ["config-presets"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableDeleteConfigurationPreset"));
    }
  });

  const update = useMutation({
    mutationFn: ({ id, input }: { id: string; input: Parameters<typeof updateConfigPreset>[1] }) => updateConfigPreset(id, input),
    onSuccess: async (preset) => {
      setEditingPreset(null);
      setSelectedPreset(preset);
      setErrorMessage("");
      setSuccessMessage(t("configurationPresetUpdated"));
      await client.invalidateQueries({ queryKey: ["config-presets"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableUpdateConfigurationPreset"));
    }
  });

  return (
    <>
      <PageHeader title={t("configurationPresets")} description={t("configurationPresetsPageDescription")} />
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
          { label: t("filterRunMode"), options: providerFilters, value: providerFilter, onChange: (value) => setProviderFilter(value) }
        ]}
        onClear={() => {
          setGameFilter("all");
          setProviderFilter("all");
          setSearch("");
        }}
        onSearchChange={setSearch}
        search={search}
        searchPlaceholder={t("searchConfigurationPresets")}
      />

      <div className="flex min-h-11 items-center justify-between border-x border-t border-panel-line bg-slate-950/25 px-4 text-sm">
        <span className={selectedIds.size > 0 ? "text-slate-300" : "text-slate-500"}>
          {selectedIds.size > 0 ? t("selectedConfigurationPresets", { count: selectedIds.size }) : t("configurationPresetSelectionHint")}
        </span>
        <Button className="h-8 px-2.5 text-xs" disabled={selectedIds.size === 0} variant="danger" onClick={() => setBatchDeleteOpen(true)}>
          <Trash2 aria-hidden="true" className="size-3.5" />{t("deleteSelectedConfigurationPresets")}
        </Button>
      </div>

      {filteredPresets.length > 0 ? (
        <PresetResourceTable
          context={context}
          locale={locale}
          presets={filteredPresets}
          selectedIds={selectedIds}
          onEdit={setEditingPreset}
          onInspect={setSelectedPreset}
          onSelectionChange={setSelectedIds}
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
      <ConfirmDialog
        open={batchDeleteOpen}
        eyebrow={t("destructiveAction")}
        title={t("deleteSelectedConfigurationPresets")}
        description={t("confirmBatchDeleteConfigurationPresets", { count: selectedIds.size })}
        cancelLabel={t("cancel")}
        confirmLabel={removeMany.isPending ? t("actionWorking") : t("delete")}
        busy={removeMany.isPending}
        onCancel={() => setBatchDeleteOpen(false)}
        onConfirm={() => removeMany.mutate(Array.from(selectedIds))}
      />
      {selectedPreset ? (
        <PresetDetailsDrawer
          context={context}
          locale={locale}
          preset={selectedPreset}
          t={t}
          onClose={() => setSelectedPreset(null)}
          onEdit={() => {
            setEditingPreset(selectedPreset);
            setSelectedPreset(null);
          }}
          onDelete={() => {
            setSelectedPreset(null);
            setPendingDelete(selectedPreset);
          }}
        />
      ) : null}
      {editingPreset ? (
        <PresetEditDrawer
          context={context}
          pending={update.isPending}
          preset={editingPreset}
          t={t}
          onClose={() => setEditingPreset(null)}
          onSave={(input) => update.mutate({ id: editingPreset.id, input })}
        />
      ) : null}
    </>
  );
}

function PresetResourceTable({
  context,
  locale,
  presets,
  selectedIds,
  onEdit,
  onInspect,
  onSelectionChange,
  t
}: {
  context: ReturnType<typeof buildPresetContext>;
  locale: Locale;
  presets: ConfigPreset[];
  selectedIds: Set<string>;
  onEdit: (preset: ConfigPreset) => void;
  onInspect: (preset: ConfigPreset) => void;
  onSelectionChange: (ids: Set<string>) => void;
  t: (key: MessageKey, values?: Record<string, string | number>) => string;
}) {
  const visibleIds = presets.map((preset) => preset.id);
  const selectedVisibleCount = visibleIds.filter((id) => selectedIds.has(id)).length;
  const allSelected = visibleIds.length > 0 && selectedVisibleCount === visibleIds.length;
  const toggleAll = () => {
    const next = new Set(selectedIds);
    if (allSelected) visibleIds.forEach((id) => next.delete(id));
    else visibleIds.forEach((id) => next.add(id));
    onSelectionChange(next);
  };
  const toggleOne = (id: string) => {
    const next = new Set(selectedIds);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    onSelectionChange(next);
  };
  return (
    <div className="overflow-hidden rounded-b-lg border border-panel-line bg-panel-card">
      <div className="hidden overflow-x-auto md:block">
        <table className="w-full min-w-[900px] border-collapse text-left text-sm">
          <thead className="bg-slate-950/45 text-xs font-medium text-slate-500">
            <tr>
              <th className="w-11 px-4 py-3"><SelectionBox checked={allSelected} indeterminate={selectedVisibleCount > 0 && !allSelected} label={t("selectAll")} onChange={toggleAll} /></th>
              <th className="px-4 py-3">{t("configurationPreset")}</th>
              <th className="px-3 py-3">{t("gameAndMode")}</th>
              <th className="px-3 py-3">{t("gameVersion")}</th>
              <th className="px-3 py-3">{t("runtimeResources")}</th>
              <th className="px-3 py-3">{t("configurationPresetContents")}</th>
              <th className="px-3 py-3">{t("modified")}</th>
              <th className="px-4 py-3 text-right">{t("actions")}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-panel-line">
            {presets.map((preset) => {
              const meta = presetMeta(preset, context);
              const modIds = preset.modIds ?? [];
              const configuredFields = configuredPresetFieldCount(preset);
              return (
                <tr className={selectedIds.has(preset.id) ? "group bg-panel-green/[0.06] transition-colors hover:bg-panel-green/[0.09]" : "group transition-colors hover:bg-slate-800/35"} key={preset.id}>
                  <td className="px-4 py-3"><SelectionBox checked={selectedIds.has(preset.id)} label={t("selectConfigurationPreset", { name: preset.name })} onChange={() => toggleOne(preset.id)} /></td>
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
                    <p>{t("configurationItemsCount", { count: configuredFields })}</p>
                    <p className="mt-0.5 text-xs text-slate-500">
                      {modIds.length > 0 ? [meta.modPackName, t("selectedModsCount", { count: modIds.length })].filter(Boolean).join(" · ") : t("noModsIncluded")}
                    </p>
                  </td>
                  <td className="px-3 py-3 text-slate-400">{localizeRelativeTime(preset.updatedAt, locale)}</td>
                  <td className="px-4 py-3">
                    <div className="flex justify-end gap-2">
                      <Button className="h-8 px-2.5 text-xs" variant="secondary" onClick={() => onEdit(preset)}><Pencil aria-hidden="true" className="size-3.5" />{t("edit")}</Button>
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
            <div className={selectedIds.has(preset.id) ? "bg-panel-green/[0.06] px-4 py-4" : "px-4 py-4"} key={preset.id}>
              <div className="flex items-start justify-between gap-3">
                <div className="flex min-w-0 items-start gap-3">
                  <span className="mt-1"><SelectionBox checked={selectedIds.has(preset.id)} label={t("selectConfigurationPreset", { name: preset.name })} onChange={() => toggleOne(preset.id)} /></span>
                  <button className="min-w-0 text-left" onClick={() => onInspect(preset)} type="button">
                    <p className="truncate font-medium text-slate-100">{preset.name}</p>
                    <p className="mt-1 truncate text-xs text-slate-500">{meta.gameName} · {meta.providerName}</p>
                  </button>
                </div>
                <Badge className="shrink-0 bg-slate-800 text-slate-300">{preset.version || t("recommended")}</Badge>
              </div>
              <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-400">
                <span>{formatCpuLimit(preset.cpuLimitCores, t)} · {formatMemoryLimit(preset.memoryLimitMb, t)}</span>
                <span>{modIds.length > 0 ? t("selectedModsCount", { count: modIds.length }) : t("none")}</span>
                <span>{localizeRelativeTime(preset.updatedAt, locale)}</span>
              </div>
              <div className="mt-3 flex justify-end gap-2">
                <Button className="h-8 px-2.5 text-xs" variant="secondary" onClick={() => onEdit(preset)}><Pencil aria-hidden="true" className="size-3.5" />{t("edit")}</Button>
                <Link className="inline-flex h-8 items-center gap-1.5 rounded-md border border-panel-green/35 bg-panel-green/10 px-2.5 text-xs font-medium text-panel-green" href={`/servers/new?presetId=${encodeURIComponent(preset.id)}`}><Plus className="size-3.5" />{t("createServer")}</Link>
              </div>
            </div>
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
  onEdit,
  onDelete
}: {
  context: ReturnType<typeof buildPresetContext>;
  locale: Locale;
  preset: ConfigPreset;
  t: (key: MessageKey, values?: Record<string, string | number>) => string;
  onClose: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const meta = presetMeta(preset, context);
  const modIds = preset.modIds ?? [];
  const mods = modIds.length > 0 ? [meta.modPackName, t("selectedModsCount", { count: modIds.length })].filter(Boolean).join(" · ") : t("none");
  const entries = presetConfigEntries(preset, meta.provider, locale, t);
  const visibleEntries = entries;
  return (
    <div className="fixed inset-0 z-50 bg-slate-950/60" onMouseDown={(event) => event.target === event.currentTarget && onClose()} role="presentation">
      <aside aria-label={t("configurationPreset")} className="ml-auto flex h-full w-full max-w-2xl flex-col border-l border-panel-line bg-panel-card shadow-2xl">
        <div className="flex items-start justify-between gap-4 border-b border-panel-line px-5 py-4">
          <div className="min-w-0">
            <p className="text-xs text-slate-500">{t("configurationPreset")}</p>
            <h2 className="mt-1 truncate text-lg font-semibold text-white">{preset.name}</h2>
          </div>
          <button aria-label={t("cancel")} className="rounded-md p-2 text-slate-400 hover:bg-slate-800 hover:text-white" onClick={onClose} type="button"><X aria-hidden="true" className="size-5" /></button>
        </div>
        <div className="flex-1 overflow-y-auto px-5 py-5">
          <div className="grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-panel-line bg-panel-line sm:grid-cols-4">
            <PresetSummary icon={<Gamepad2 aria-hidden="true" />} label={t("gameAndMode")} value={`${meta.gameName} · ${meta.providerName}`} />
            <PresetSummary icon={<Cpu aria-hidden="true" />} label={t("runtimeResources")} value={`${formatCpuLimit(preset.cpuLimitCores, t)} · ${formatMemoryLimit(preset.memoryLimitMb, t)}`} />
            <PresetSummary icon={<SlidersHorizontal aria-hidden="true" />} label={t("configurationItems")} value={t("itemsCount", { count: entries.length })} />
            <PresetSummary icon={<Box aria-hidden="true" />} label={t("modsTitle")} value={modIds.length > 0 ? t("itemsCount", { count: modIds.length }) : t("none")} />
          </div>

          <section className="mt-6">
            <div className="flex items-center justify-between gap-3 border-b border-panel-line pb-2">
              <div>
                <h3 className="text-sm font-semibold text-slate-100">{t("savedConfiguration")}</h3>
                <p className="mt-0.5 text-xs text-slate-500">{t("configurationSnapshotHint")}</p>
              </div>
              <span className="text-xs text-slate-500">{t("itemsCount", { count: visibleEntries.length })}</span>
            </div>
            {visibleEntries.length > 0 ? (
              <dl className="divide-y divide-panel-line">
                {visibleEntries.slice(0, 12).map((entry) => <ConfigDrawerRow key={entry.name} entry={entry} />)}
              </dl>
            ) : <p className="py-4 text-sm text-slate-500">{t("noSavedConfigurationItems")}</p>}
            {visibleEntries.length > 12 ? (
              <details className="border-t border-panel-line py-3 text-sm">
                <summary className="cursor-pointer text-panel-green">{t("showRemainingItems", { count: visibleEntries.length - 12 })}</summary>
                <dl className="mt-2 divide-y divide-panel-line">
                  {visibleEntries.slice(12).map((entry) => <ConfigDrawerRow key={entry.name} entry={entry} />)}
                </dl>
              </details>
            ) : null}
          </section>

          <section className="mt-6 border-t border-panel-line pt-4">
            <h3 className="text-sm font-semibold text-slate-100">{t("presetMetadata")}</h3>
            <dl className="mt-2 divide-y divide-panel-line">
              <DrawerRow label={t("gameVersion")} value={preset.version || t("recommended")} />
              <DrawerRow label={t("modsTitle")} value={mods} />
              <DrawerRow label={t("created")} value={localizeRelativeTime(preset.createdAt, locale)} />
              <DrawerRow label={t("modified")} value={localizeRelativeTime(preset.updatedAt, locale)} />
            </dl>
          </section>
        </div>
        <div className="flex items-center justify-between gap-3 border-t border-panel-line px-5 py-4">
          <Button className="text-red-200 hover:bg-red-500/15" variant="ghost" onClick={onDelete}><Trash2 aria-hidden="true" />{t("delete")}</Button>
          <div className="flex gap-2">
            <Button variant="secondary" onClick={onEdit}><Pencil aria-hidden="true" className="size-4" />{t("edit")}</Button>
            <Link className="inline-flex h-10 items-center gap-2 rounded-md bg-panel-green px-4 text-sm font-semibold text-slate-950 hover:bg-panel-green/90" href={`/servers/new?presetId=${encodeURIComponent(preset.id)}`}>
              <Plus aria-hidden="true" className="size-4" />{t("createServerFromPreset")}
            </Link>
          </div>
        </div>
      </aside>
    </div>
  );
}

function ConfigDrawerRow({ entry }: { entry: PresetConfigEntry }) {
  return (
    <div className="grid grid-cols-[minmax(0,1.35fr)_minmax(7rem,0.65fr)] gap-5 py-3">
      <div className="min-w-0">
        <dt className="text-sm font-medium text-slate-300">{entry.label}</dt>
        {entry.help ? <p className="mt-1 text-xs leading-5 text-slate-500">{entry.help}</p> : null}
        <code className="mt-1 block truncate text-[11px] text-slate-600" title={entry.name}>{entry.name}</code>
      </div>
      <dd className="self-center break-words text-right text-sm font-medium text-slate-100">{entry.value}</dd>
    </div>
  );
}

function PresetEditDrawer({ context, pending, preset, t, onClose, onSave }: {
  context: ReturnType<typeof buildPresetContext>;
  pending: boolean;
  preset: ConfigPreset;
  t: (key: MessageKey, values?: Record<string, string | number>) => string;
  onClose: () => void;
  onSave: (input: Parameters<typeof updateConfigPreset>[1]) => void;
}) {
  const meta = presetMeta(preset, context);
  const provider = meta.provider;
  const [name, setName] = useState(preset.name);
  const [version, setVersion] = useState(preset.version || provider?.recommendedVersion || provider?.versions[0] || "");
  const [cpuLimitCores, setCpuLimitCores] = useState(preset.cpuLimitCores ?? 0);
  const [memoryLimitMb, setMemoryLimitMb] = useState(preset.memoryLimitMb ?? 0);
  const [config, setConfig] = useState<ProviderConfigPayload>(() => createDefaultProviderConfigPayload(provider, preset.configPayload ?? preset.config ?? {}));
  const canSave = name.trim().length > 0 && Boolean(provider) && !pending;

  return (
    <div className="fixed inset-0 z-50 bg-slate-950/60" onMouseDown={(event) => event.target === event.currentTarget && onClose()} role="presentation">
      <aside aria-label={t("editConfigurationPreset")} className="ml-auto flex h-full w-full max-w-3xl flex-col border-l border-panel-line bg-panel-card shadow-2xl">
        <div className="flex items-start justify-between gap-4 border-b border-panel-line px-5 py-4">
          <div className="min-w-0">
            <p className="text-xs text-slate-500">{t("configurationPreset")}</p>
            <h2 className="mt-1 truncate text-lg font-semibold text-white">{t("editConfigurationPreset")}</h2>
          </div>
          <button aria-label={t("cancel")} className="rounded-md p-2 text-slate-400 hover:bg-slate-800 hover:text-white" onClick={onClose} type="button"><X aria-hidden="true" className="size-5" /></button>
        </div>
        <div className="flex-1 overflow-y-auto px-5 py-5">
          <section>
            <h3 className="text-sm font-semibold text-slate-100">{t("presetBasicInformation")}</h3>
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              <label className="text-xs font-medium text-slate-400">
                {t("configurationPresetName")}
                <Input className="mt-2 w-full" value={name} onChange={(event) => setName(event.target.value)} />
              </label>
              <label className="text-xs font-medium text-slate-400">
                {t("gameVersion")}
                <select className="mt-2 h-10 w-full rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-100 outline-none focus:border-panel-green" value={version} onChange={(event) => setVersion(event.target.value)}>
                  {(provider?.versions ?? []).map((item) => <option key={item} value={item}>{item}</option>)}
                </select>
              </label>
            </div>
            <p className="mt-3 text-xs text-slate-500">{meta.gameName} · {meta.providerName}</p>
          </section>

          <section className="mt-6 border-t border-panel-line pt-5">
            <h3 className="text-sm font-semibold text-slate-100">{t("runtimeResources")}</h3>
            <p className="mt-1 text-xs text-slate-500">{t("presetResourceEditHint")}</p>
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              <label className="text-xs font-medium text-slate-400">{t("cpuLimit")}<Input className="mt-2 w-full" min={0} step={0.25} type="number" value={cpuLimitCores} onChange={(event) => setCpuLimitCores(Number(event.target.value))} /></label>
              <label className="text-xs font-medium text-slate-400">{t("memoryLimitMb")}<Input className="mt-2 w-full" min={0} step={128} type="number" value={memoryLimitMb} onChange={(event) => setMemoryLimitMb(Number(event.target.value))} /></label>
            </div>
          </section>

          <section className="mt-6 border-t border-panel-line pt-5">
            <div>
              <h3 className="text-sm font-semibold text-slate-100">{t("gameConfiguration")}</h3>
              <p className="mt-1 text-xs text-slate-500">{t("configurationSnapshotHint")}</p>
            </div>
            {provider ? (
              <ProviderConfigEditor
                disabled={pending}
                fieldHelp={(field) => field.help ?? ""}
                fieldLabel={(field) => field.label || formatKey(field.name)}
                fields={provider.configSchema}
                onChange={(field, value) => setConfig((current) => updateProviderConfigPayload(current, field, value))}
                onRestoreDefaults={(fields) => setConfig((current) => restoreProviderConfigDefaults(current, fields))}
                payload={config}
                providerKey={provider.key}
              />
            ) : <p className="mt-3 text-sm text-panel-gold">{t("providerUnavailable")}</p>}
          </section>
        </div>
        <div className="flex justify-end gap-2 border-t border-panel-line px-5 py-4">
          <Button disabled={pending} variant="secondary" onClick={onClose}>{t("cancel")}</Button>
          <Button disabled={!canSave} onClick={() => onSave({
            name: name.trim(),
            providerKey: preset.providerKey,
            config,
            version,
            resources: { cpuLimitCores, memoryLimitMb },
            modPackId: preset.modPackId,
            modIds: preset.modIds ?? []
          })}>{pending ? t("actionWorking") : t("saveSettings")}</Button>
        </div>
      </aside>
    </div>
  );
}

function DrawerRow({ label, value }: { label: string; value: string }) {
  return <div className="grid grid-cols-[minmax(8rem,0.9fr)_minmax(0,1.3fr)] gap-4 py-3 text-sm"><dt className="truncate text-slate-500" title={label}>{label}</dt><dd className="break-words text-right text-slate-100">{value}</dd></div>;
}

function PresetSummary({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return (
    <div className="min-w-0 bg-slate-950/35 p-3">
      <div className="flex items-center gap-1.5 text-xs text-slate-500">{<span className="[&>svg]:size-3.5">{icon}</span>}{label}</div>
      <p className="mt-1.5 line-clamp-2 text-sm font-medium text-slate-100" title={value}>{value}</p>
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
    modPackName: modPack?.name ?? "",
    provider
  };
}

function configuredPresetFieldCount(preset: ConfigPreset) {
  return flattenPresetPayload(preset.configPayload ?? preset.config ?? {}).length;
}

type PresetConfigEntry = { name: string; label: string; help: string; value: string; changed: boolean };

function presetConfigEntries(
  preset: ConfigPreset,
  provider: ProviderCatalog | undefined,
  locale: Locale,
  t: (key: MessageKey, values?: Record<string, string | number>) => string
): PresetConfigEntry[] {
  const payload = preset.configPayload ?? preset.config ?? {};
  const schema = new Map((provider?.configSchema ?? []).map((field) => [field.name.toLowerCase(), field]));
  return flattenPresetPayload(payload).map(({ name, value }) => {
    const field = schema.get(name.toLowerCase());
    return {
      name,
      label: field?.label || formatKey(name),
      help: field?.help ?? t("configurationParameterFallbackHelp"),
      value: field?.type === "password" ? "••••••••" : formatPresetValue(value, locale, field, t),
      changed: field ? !presetValuesEqual(value, field.default) : true
    };
  }).sort((left, right) => Number(right.changed) - Number(left.changed) || left.label.localeCompare(right.label));
}

function flattenPresetPayload(payload: Record<string, unknown>, prefix = ""): Array<{ name: string; value: unknown }> {
  return Object.entries(payload).flatMap(([key, value]) => {
    const name = prefix ? `${prefix}.${key}` : key;
    if (typeof value === "object" && value !== null && !Array.isArray(value)) {
      return flattenPresetPayload(value as Record<string, unknown>, name);
    }
    return [{ name, value }];
  });
}

function formatPresetValue(
  value: unknown,
  locale: Locale,
  field: ProviderConfigField | undefined,
  t: (key: MessageKey, values?: Record<string, string | number>) => string
) {
  if (typeof value === "boolean") return value ? (locale === "zh" ? "已启用" : "Enabled") : (locale === "zh" ? "已禁用" : "Disabled");
  if (field?.type === "select") {
    const option = field.options?.find((item) => item.value === String(value));
    if (option) return providerOptionLabel(field, option.value, option.label, t);
  }
  if (value === null || value === undefined || value === "") return "—";
  if (Array.isArray(value)) return value.length > 0 ? value.map(String).join("、") : "—";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function presetValuesEqual(left: unknown, right: unknown) {
  return JSON.stringify(left ?? null) === JSON.stringify(right ?? null);
}

function formatKey(value: string) {
  return value
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .split(/[.\-_\s]+/)
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
