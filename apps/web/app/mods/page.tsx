"use client";

import Image from "next/image";
import Link from "next/link";
import { Check, Clock3, Compass, Download, ExternalLink, Library, Package, Trash2, Upload, Users, X } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useRef, useState, type ReactNode } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { SelectionBox } from "@/components/selection-box";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { Badge, Button, Card, Input } from "@/components/ui";
import { createModPack, createModPackFromWorkshopCollection, deleteGlobalMod, deleteGlobalMods, deleteModPack, deleteModPacks, getDockerStatus, importGlobalWorkshopMods, importRecommendedMod, listGames, listGlobalMods, listModPacks, listRecommendedMods, previewWorkshopCollection, previewWorkshopItems, uploadGlobalMod } from "@/lib/api";
import { gameFilterOptionsForKeys } from "@/lib/game-filters";
import { localizeRelativeTime, useI18n, type MessageKey } from "@/lib/i18n";
import { dstModScopeFromTags, modDisplayName, modSourceLabel } from "@/lib/mod-display";
import { filterModResources, modGameFilterKeys } from "@/lib/mod-filters";
import { providerDisplayName } from "@/lib/provider-display";
import { cn } from "@/lib/utils";
import { parseWorkshopIds } from "@/lib/workshop-input";
import { GameAssetsSubNav } from "@/components/sub-nav";
import type { GameCatalogEntry, ModFile, ModPack, ProviderKey, RecommendedMod, WorkshopPreview } from "@/lib/types";

type ModsView = "discover" | "library" | "packs";
type ModGameFilter = "all" | string;
type DependencyImportPlan = {
  primaryIds: string[];
  dependencyIds: string[];
  dependencyNames: string[];
  providerKey: ProviderKey;
};
export default function ModsPage() {
  const { locale, t } = useI18n();
  const globalInputRef = useRef<HTMLInputElement>(null);
  const client = useQueryClient();
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [pendingDelete, setPendingDelete] = useState<ModFile | null>(null);
  const [pendingPackDelete, setPendingPackDelete] = useState<ModPack | null>(null);
  const [pendingBulkDelete, setPendingBulkDelete] = useState<"library" | "packs" | null>(null);
  const [selectedLibraryIds, setSelectedLibraryIds] = useState<Set<string>>(new Set());
  const [selectedModPackIds, setSelectedModPackIds] = useState<Set<string>>(new Set());
  const [activeView, setActiveView] = useState<ModsView>("discover");
  const [gameFilter, setGameFilter] = useState<ModGameFilter>("all");
  const [search, setSearch] = useState("");
  const [workshopDialogOpen, setWorkshopDialogOpen] = useState(false);
  const [packDialogOpen, setPackDialogOpen] = useState(false);
  const [packImportDialogOpen, setPackImportDialogOpen] = useState(false);
  const [packName, setPackName] = useState("");
  const [packDescription, setPackDescription] = useState("");
  const [selectedPackModIds, setSelectedPackModIds] = useState<string[]>([]);
  const [packCollectionValue, setPackCollectionValue] = useState("");
  const [packCollectionProviderKey, setPackCollectionProviderKey] = useState<ProviderKey>("terraria-tmodloader");
  const [packCollectionPreview, setPackCollectionPreview] = useState<WorkshopPreview | null>(null);
  const [packCollectionName, setPackCollectionName] = useState("");
  const [packCollectionDescription, setPackCollectionDescription] = useState("");
  const [workshopSource, setWorkshopSource] = useState<"collection" | "ids">("collection");
  const [workshopCollectionValue, setWorkshopCollectionValue] = useState("");
  const [workshopPreview, setWorkshopPreview] = useState<WorkshopPreview | null>(null);
  const [selectedWorkshopIds, setSelectedWorkshopIds] = useState<string[]>([]);
  const [workshopIdsText, setWorkshopIdsText] = useState("");
  const [workshopProviderKey, setWorkshopProviderKey] = useState<ProviderKey>("terraria-tmodloader");
  const globalModsQuery = useQuery({ queryKey: ["global-mods"], queryFn: listGlobalMods, retry: false });
  const modPacksQuery = useQuery({ queryKey: ["mod-packs"], queryFn: listModPacks, retry: false });
  const recommendedModsQuery = useQuery({ queryKey: ["recommended-mods"], queryFn: listRecommendedMods, retry: false });
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false, staleTime: 5 * 60 * 1000 });
  const dockerStatusQuery = useQuery({ queryKey: ["docker-status"], queryFn: getDockerStatus, retry: false, refetchInterval: 5000 });
  const workshopUnsupported = isArmArchitecture(dockerStatusQuery.data?.architecture);

  const globalUpload = useMutation({
    mutationFn: async (files: File[]) => {
      const uploaded: ModFile[] = [];
      const failed: string[] = [];
      for (const file of files) {
        try {
          uploaded.push(await uploadGlobalMod(file));
        } catch (error) {
          const reason = error instanceof Error ? error.message : t("unableUploadMod");
          failed.push(`${file.name}: ${reason}`);
        }
      }
      return { uploaded, failed };
    },
    onSuccess: async ({ uploaded, failed }) => {
      setErrorMessage("");
      setSuccessMessage(uploaded.length > 0 ? t("modsUploadedSummary", { count: uploaded.length }) : "");
      if (failed.length > 0) {
        setErrorMessage(t("modsUploadFailedSummary", { count: failed.length, names: failed.slice(0, 3).join("；") }));
      }
      await client.invalidateQueries({ queryKey: ["global-mods"] });
      if (globalInputRef.current) globalInputRef.current.value = "";
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableUploadMod"));
    }
  });
  const removeGlobal = useMutation({
    mutationFn: (modId: string) => deleteGlobalMod(modId),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("modDeleted"));
      setPendingDelete(null);
      await client.invalidateQueries({ queryKey: ["global-mods"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableDeleteMod"));
    }
  });
  const createPack = useMutation({
    mutationFn: () => createModPack({ name: packName, description: packDescription, modIds: selectedPackModIds }),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("modPackCreated"));
      setPackName("");
      setPackDescription("");
      setSelectedPackModIds([]);
      setSelectedLibraryIds(new Set());
      setPackDialogOpen(false);
      await client.invalidateQueries({ queryKey: ["mod-packs"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableCreateModPack"));
    }
  });
  const removePack = useMutation({
    mutationFn: (packId: string) => deleteModPack(packId),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("modPackDeleted"));
      setPendingPackDelete(null);
      await client.invalidateQueries({ queryKey: ["mod-packs"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableDeleteModPack"));
    }
  });
  const bulkRemove = useMutation({
    mutationFn: ({ kind, ids }: { kind: "library" | "packs"; ids: string[] }) => (
      kind === "library" ? deleteGlobalMods(ids) : deleteModPacks(ids)
    ),
    onSuccess: async (result, variables) => {
      setPendingBulkDelete(null);
      if (variables.kind === "library") {
        setSelectedLibraryIds(new Set());
        await client.invalidateQueries({ queryKey: ["global-mods"] });
      } else {
        setSelectedModPackIds(new Set());
        await client.invalidateQueries({ queryKey: ["mod-packs"] });
      }
      setErrorMessage(result.failed.length > 0 ? t("bulkModDeletePartial", { succeeded: result.succeeded.length, failed: result.failed.length }) : "");
      setSuccessMessage(result.succeeded.length > 0
        ? t(variables.kind === "library" ? "bulkModsRemoved" : "bulkModPacksDeleted", { count: result.succeeded.length })
        : "");
    },
    onError: (error) => {
      setPendingBulkDelete(null);
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("batchActionFailed"));
    }
  });
  const packCollectionPreviewMutation = useMutation({
    mutationFn: (value?: string) => previewWorkshopCollection({
      value: value ?? packCollectionValue,
      providerKey: packCollectionProviderKey
    }),
    onSuccess: (preview) => {
      setErrorMessage("");
      setPackCollectionPreview(preview);
      setPackCollectionName((current) => current.trim() || preview.collectionName?.trim() || t("steamCollectionDefaultPackName", { id: preview.collectionId }));
    },
    onError: (error) => {
      setPackCollectionPreview(null);
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unablePreviewWorkshopCollection"));
    }
  });
  const createPackFromCollection = useMutation({
    mutationFn: () => {
      if (!packCollectionPreview) throw new Error(t("unablePreviewWorkshopCollection"));
      return createModPackFromWorkshopCollection({
        name: packCollectionName,
        description: packCollectionDescription,
        providerKey: packCollectionProviderKey,
        previewId: packCollectionPreview.previewId,
        workshopIds: packCollectionPreview.items.filter((item) => item.selectable).map((item) => item.workshopId)
      });
    },
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("steamCollectionModPackCreated"));
      setPackImportDialogOpen(false);
      setPackCollectionValue("");
      setPackCollectionPreview(null);
      setPackCollectionName("");
      setPackCollectionDescription("");
      await Promise.all([
        client.invalidateQueries({ queryKey: ["global-mods"] }),
        client.invalidateQueries({ queryKey: ["mod-packs"] }),
        client.invalidateQueries({ queryKey: ["recommended-mods"] })
      ]);
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableCreateModPack"));
    }
  });
  const workshopImport = useMutation({
    mutationFn: ({ ids, providerKey, previewId }: { ids: string[]; providerKey: ProviderKey; previewId?: string }) => importGlobalWorkshopMods(ids, providerKey, previewId),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("workshopModsImported"));
      setWorkshopDialogOpen(false);
      setWorkshopIdsText("");
      setWorkshopCollectionValue("");
      setWorkshopPreview(null);
      setSelectedWorkshopIds([]);
      await client.invalidateQueries({ queryKey: ["global-mods"] });
      await client.invalidateQueries({ queryKey: ["recommended-mods"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableImportWorkshopMods"));
    }
  });
  const workshopCollectionPreview = useMutation({
    mutationFn: (value?: string) => previewWorkshopCollection({ value: value ?? workshopCollectionValue, providerKey: workshopProviderKey }),
    onSuccess: (preview) => {
      setErrorMessage("");
      setWorkshopPreview(preview);
      setSelectedWorkshopIds(preview.items.filter((item) => item.selectable && item.status === "new").map((item) => item.workshopId));
    },
    onError: (error) => {
      setWorkshopPreview(null);
      setSelectedWorkshopIds([]);
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unablePreviewWorkshopCollection"));
    }
  });
  const workshopItemsPreview = useMutation({
    mutationFn: ({ ids, providerKey }: { ids: string[]; providerKey: ProviderKey }) => previewWorkshopItems({ workshopIds: ids, providerKey }),
    onSuccess: (preview) => {
      setErrorMessage("");
      setWorkshopPreview(preview);
      setSelectedWorkshopIds(preview.items.filter((item) => item.selectable && item.status === "new").map((item) => item.workshopId));
    },
    onError: (error) => {
      setWorkshopPreview(null);
      setSelectedWorkshopIds([]);
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unablePreviewWorkshopItems"));
    }
  });
  const recommendedImport = useMutation({
    mutationFn: importRecommendedMod,
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("workshopModsImported"));
      await client.invalidateQueries({ queryKey: ["global-mods"] });
      await client.invalidateQueries({ queryKey: ["recommended-mods"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableImportWorkshopMods"));
    }
  });
  const globalMods = globalModsQuery.data ?? [];
  const modPacks = modPacksQuery.data ?? [];
  const recommendedMods = recommendedModsQuery.data ?? [];
  const modGameKeys = modGameFilterKeys(gamesQuery.data ?? [], [...globalMods, ...modPacks, ...recommendedMods]);
  const gameFilters = gameFilterOptionsForKeys(gamesQuery.data ?? [], t("filterAll"), modGameKeys, t);
  const filteredGlobalMods = filterModResources(globalMods, gameFilter);
  const filteredModPacks = filterModResources(modPacks, gameFilter);
  const filteredRecommendedMods = filterModResources(recommendedMods, gameFilter);
  const searchTerm = search.trim().toLowerCase();
  const searchedGlobalMods = useMemo(
    () => filteredGlobalMods.filter((mod) => modMatchesSearch(mod, searchTerm)),
    [filteredGlobalMods, searchTerm]
  );
  const searchedModPacks = useMemo(
    () => filteredModPacks.filter((pack) => !searchTerm || [pack.name, pack.description ?? "", ...pack.mods.map((mod) => modDisplayName(mod, locale))].some((value) => value.toLowerCase().includes(searchTerm))),
    [filteredModPacks, locale, searchTerm]
  );
  const searchedRecommendedMods = useMemo(
    () => filteredRecommendedMods.filter((mod) => modMatchesSearch(mod, searchTerm)),
    [filteredRecommendedMods, searchTerm]
  );
  const selectedPackModCount = selectedPackModIds.length;
  const selectedPackDependencies = dependencyNamesForSelectedMods(globalMods, selectedPackModIds);
  const workshopIds = parseWorkshopIds(workshopIdsText);
  const requestWorkshopPreview = (ids: string[], providerKey: ProviderKey = workshopProviderKey) => {
    if (workshopUnsupported) {
      setSuccessMessage("");
      setErrorMessage(t("workshopArmUnsupported"));
      return;
    }
    const plan = buildDependencyImportPlan(ids, providerKey, recommendedMods, globalMods);
    workshopItemsPreview.mutate({ ids: [...plan.primaryIds, ...plan.dependencyIds], providerKey });
  };
  const togglePackMod = (modId: string) => {
    setSelectedPackModIds((current) => current.includes(modId) ? current.filter((id) => id !== modId) : [...current, modId]);
  };
  const clearTableSelection = () => {
    setSelectedLibraryIds(new Set());
    setSelectedModPackIds(new Set());
  };
  const changeView = (view: ModsView) => {
    clearTableSelection();
    setActiveView(view);
  };
  const selectedLibraryMods = globalMods.filter((item) => selectedLibraryIds.has(item.id));
  const selectedModPacks = modPacks.filter((item) => selectedModPackIds.has(item.id));

  return (
    <>
      <PageHeader title={t("modsTitle")} />
      <GameAssetsSubNav />
      <input
        ref={globalInputRef}
        className="hidden"
        type="file"
        accept=".tmod"
        multiple
        onChange={(event) => {
          const files = Array.from(event.target.files ?? []);
          if (files.length > 0) globalUpload.mutate(files);
        }}
      />
      {(globalModsQuery.isError || modPacksQuery.isError || recommendedModsQuery.isError) && (
        <p className="mb-4 text-sm text-panel-gold">{t("modsApiUnavailable")}</p>
      )}
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      {successMessage && <p className="mb-4 text-sm text-panel-green">{successMessage}</p>}

      {/* 现代化一体式模组控制台工具栏 (Unified Mods Toolbar) */}
      <div className="mt-3 mb-4 rounded-xl border border-panel-line bg-panel-card p-3 shadow-xs space-y-3">
        {/* Track 1: 视图切换 Segmented Pills 与 视图提示 */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 pb-2.5 border-b border-panel-line/60">
          <div className="flex items-center gap-1.5 overflow-x-auto scrollbar-none py-0.5">
            <button
              type="button"
              onClick={() => changeView("discover")}
              className={cn(
                "flex items-center gap-2 rounded-lg px-3 py-1.5 text-xs font-medium transition shrink-0",
                activeView === "discover"
                  ? "bg-slate-800 text-panel-green border border-panel-green/40 shadow-xs font-bold"
                  : "bg-slate-950/60 text-slate-400 hover:text-slate-200 hover:bg-slate-900 border border-slate-800/80"
              )}
            >
              <Compass className="size-3.5" />
              <span>{t("discoverMods")}</span>
              <span className={cn(
                "rounded-full px-1.5 py-0.2 text-[10px] font-mono",
                activeView === "discover" ? "bg-panel-green/20 text-panel-green" : "bg-slate-800 text-slate-400"
              )}>
                {searchedRecommendedMods.length}
              </span>
            </button>
            <button
              type="button"
              onClick={() => changeView("library")}
              className={cn(
                "flex items-center gap-2 rounded-lg px-3 py-1.5 text-xs font-medium transition shrink-0",
                activeView === "library"
                  ? "bg-slate-800 text-panel-green border border-panel-green/40 shadow-xs font-bold"
                  : "bg-slate-950/60 text-slate-400 hover:text-slate-200 hover:bg-slate-900 border border-slate-800/80"
              )}
            >
              <Library className="size-3.5" />
              <span>{t("modLibrary")}</span>
              <span className={cn(
                "rounded-full px-1.5 py-0.2 text-[10px] font-mono",
                activeView === "library" ? "bg-panel-green/20 text-panel-green" : "bg-slate-800 text-slate-400"
              )}>
                {searchedGlobalMods.length}
              </span>
            </button>
            <button
              type="button"
              onClick={() => changeView("packs")}
              className={cn(
                "flex items-center gap-2 rounded-lg px-3 py-1.5 text-xs font-medium transition shrink-0",
                activeView === "packs"
                  ? "bg-slate-800 text-panel-green border border-panel-green/40 shadow-xs font-bold"
                  : "bg-slate-950/60 text-slate-400 hover:text-slate-200 hover:bg-slate-900 border border-slate-800/80"
              )}
            >
              <Package className="size-3.5" />
              <span>{t("modPacks")}</span>
              <span className={cn(
                "rounded-full px-1.5 py-0.2 text-[10px] font-mono",
                activeView === "packs" ? "bg-panel-green/20 text-panel-green" : "bg-slate-800 text-slate-400"
              )}>
                {searchedModPacks.length}
              </span>
            </button>
          </div>

          <p className="hidden sm:block truncate text-xs text-slate-400 font-medium">
            {activeView === "discover" ? t("discoverModsHint") : activeView === "library" ? t("modLibraryHint") : t("modPacksHint")}
          </p>
        </div>

        {/* Track 2: 动作按钮与搜索过滤 */}
        <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-3">
          {/* Action Group */}
          <div className="flex flex-wrap items-center gap-2 min-h-9">
            {activeView === "discover" && (
              <Button
                className="h-9 px-3.5 bg-panel-green text-slate-950 font-bold hover:bg-emerald-400 shadow-md shadow-emerald-950/20 transition flex items-center gap-2 text-xs"
                onClick={() => setWorkshopDialogOpen(true)}
                disabled={workshopImport.isPending || workshopItemsPreview.isPending}
              >
                <Download className="size-3.5 stroke-[2.5]" />
                <span>{t("importFromSteam")}</span>
              </Button>
            )}

            {activeView === "library" && (
              selectedLibraryIds.size > 0 ? (
                <div className="flex flex-wrap items-center gap-1.5 bg-slate-950/80 border border-emerald-500/30 rounded-lg p-1 animate-in fade-in zoom-in-95 duration-150">
                  <span className="px-2 text-xs font-mono font-bold text-panel-green">
                    {t("selectedModsCount", { count: selectedLibraryIds.size })}
                  </span>
                  <Button
                    className="h-7 px-2.5 text-xs font-medium"
                    variant="secondary"
                    onClick={() => {
                      setSelectedPackModIds(Array.from(selectedLibraryIds));
                      setPackDialogOpen(true);
                    }}
                  >
                    <Package className="size-3 text-sky-400" />
                    {t("createModPack")}
                  </Button>
                  <Button
                    className="h-7 px-2.5 text-xs font-medium"
                    variant="danger"
                    onClick={() => setPendingBulkDelete("library")}
                    disabled={bulkRemove.isPending}
                  >
                    <Trash2 className="size-3" />
                    {t("removeSelectedMods")}
                  </Button>
                  <button
                    type="button"
                    onClick={clearTableSelection}
                    className="px-2 text-xs text-slate-400 hover:text-slate-200 transition underline underline-offset-2"
                  >
                    {locale === "zh" ? "取消勾选" : "Deselect"}
                  </button>
                </div>
              ) : (
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    className="h-9 px-3.5 bg-panel-green text-slate-950 font-bold hover:bg-emerald-400 shadow-md shadow-emerald-950/20 transition flex items-center gap-2 text-xs"
                    onClick={() => setWorkshopDialogOpen(true)}
                    disabled={workshopImport.isPending || workshopItemsPreview.isPending}
                  >
                    <Download className="size-3.5 stroke-[2.5]" />
                    <span>{t("importFromSteam")}</span>
                  </Button>
                  <Button
                    className="h-9 px-3 text-xs"
                    variant="secondary"
                    onClick={() => globalInputRef.current?.click()}
                    disabled={globalUpload.isPending}
                  >
                    <Upload className="size-3.5" />
                    <span>{globalUpload.isPending ? t("uploading") : t("uploadMod")}</span>
                  </Button>
                </div>
              )
            )}

            {activeView === "packs" && (
              selectedModPackIds.size > 0 ? (
                <div className="flex flex-wrap items-center gap-1.5 bg-slate-950/80 border border-emerald-500/30 rounded-lg p-1 animate-in fade-in zoom-in-95 duration-150">
                  <span className="px-2 text-xs font-mono font-bold text-panel-green">
                    {t("selectedModPacksCount", { count: selectedModPackIds.size })}
                  </span>
                  <Button
                    className="h-7 px-2.5 text-xs font-medium"
                    variant="danger"
                    onClick={() => setPendingBulkDelete("packs")}
                    disabled={bulkRemove.isPending}
                  >
                    <Trash2 className="size-3" />
                    {t("deleteSelectedModPacks")}
                  </Button>
                  <button
                    type="button"
                    onClick={clearTableSelection}
                    className="px-2 text-xs text-slate-400 hover:text-slate-200 transition underline underline-offset-2"
                  >
                    {locale === "zh" ? "取消勾选" : "Deselect"}
                  </button>
                </div>
              ) : (
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    className="h-9 px-3.5 bg-panel-green text-slate-950 font-bold hover:bg-emerald-400 shadow-md shadow-emerald-950/20 transition flex items-center gap-2 text-xs"
                    onClick={() => setPackDialogOpen(true)}
                  >
                    <Package className="size-3.5 stroke-[2.5]" />
                    <span>{t("createModPack")}</span>
                  </Button>
                  <Button
                    className="h-9 px-3 text-xs"
                    variant="secondary"
                    onClick={() => setPackImportDialogOpen(true)}
                    disabled={workshopUnsupported}
                    title={workshopUnsupported ? t("workshopArmUnsupported") : undefined}
                  >
                    <Download className="size-3.5" />
                    <span>{t("importFromSteam")}</span>
                  </Button>
                </div>
              )
            )}
          </div>

          {/* Search & Filter Bar */}
          <div className="flex min-w-0 flex-1 items-center gap-2 lg:max-w-md lg:justify-end">
            <ResourceFilterBar
              clearLabel={t("clearFilters")}
              density="compact"
              filters={[{
                label: t("filterGame"),
                options: gameFilters,
                value: gameFilter,
                onChange: (value) => {
                  clearTableSelection();
                  setGameFilter(value);
                }
              }]}
              onClear={() => {
                clearTableSelection();
                setGameFilter("all");
                setSearch("");
              }}
              onSearchChange={(value) => {
                clearTableSelection();
                setSearch(value);
              }}
              search={search}
              searchPlaceholder={t("searchMods")}
            />
          </div>
        </div>
      </div>

      {activeView === "discover" ? (
        <section className="mt-4">
          <div className="grid gap-3 2xl:grid-cols-2">
            {searchedRecommendedMods.map((item) => (
              <RecommendedModCard
                key={recommendedModKey(item)}
                item={item}
                locale={locale}
                busy={workshopImport.isPending || workshopItemsPreview.isPending || recommendedImport.isPending || (isWorkshopRecommended(item) && workshopUnsupported)}
                disabledReason={recommendedModDisabledReason(item, workshopUnsupported, t)}
                onAdd={() => {
                  if (isWorkshopRecommended(item) && item.workshopId) {
                    const providerKey = item.providerKey ?? "terraria-tmodloader";
                    setWorkshopSource("ids");
                    setWorkshopProviderKey(providerKey);
                    setWorkshopIdsText(item.workshopId);
                    setWorkshopDialogOpen(true);
                    requestWorkshopPreview([item.workshopId], providerKey);
                    return;
                  }
                  recommendedImport.mutate({ providerKey: item.providerKey, externalId: item.externalId, workshopId: item.workshopId });
                }}
              />
            ))}
          </div>
        </section>
      ) : activeView === "library" ? (
        <section className="mt-4">
          <div>
            {searchedGlobalMods.length > 0 ? (
              <LibraryModTable
                games={gamesQuery.data ?? []}
                items={searchedGlobalMods}
                locale={locale}
                deleting={removeGlobal.isPending || bulkRemove.isPending}
                onDelete={setPendingDelete}
                onSelectionChange={setSelectedLibraryIds}
                selectedIds={selectedLibraryIds}
              />
            ) : null}
            {!globalModsQuery.isLoading && searchedGlobalMods.length === 0 && (
              <Card className="flex min-h-44 items-center justify-center border-dashed p-6 text-center text-slate-400">
                <div>
                  <Package aria-hidden="true" className="mx-auto" />
                  <p className="mt-2 text-sm">{t("noGlobalMods")}</p>
                </div>
              </Card>
            )}
          </div>
        </section>
      ) : (
        <section className="mt-4">
          <div>
            {searchedModPacks.length > 0 ? (
              <ModPackTable
                games={gamesQuery.data ?? []}
                packs={searchedModPacks}
                locale={locale}
                deleting={removePack.isPending || bulkRemove.isPending}
                onDelete={setPendingPackDelete}
                onSelectionChange={setSelectedModPackIds}
                selectedIds={selectedModPackIds}
              />
            ) : null}
            {!modPacksQuery.isLoading && searchedModPacks.length === 0 && (
              <Card className="flex min-h-44 items-center justify-center border-dashed p-6 text-center text-slate-400">
                <div>
                  <Package aria-hidden="true" className="mx-auto" />
                  <p className="mt-2 text-sm">{t("noModPacks")}</p>
                </div>
              </Card>
            )}
          </div>
        </section>
      )}

      {workshopDialogOpen && (
        <DialogShell
          title={t("importFromSteam")}
          onClose={() => {
            setWorkshopDialogOpen(false);
            setWorkshopPreview(null);
          }}
        >
          <div className="mb-4 grid grid-cols-2 gap-2 rounded-md bg-slate-950/60 p-1">
            <button
              type="button"
              className={cn("rounded px-3 py-2 text-sm transition", workshopSource === "collection" ? "bg-slate-800 text-white" : "text-slate-400 hover:text-white")}
              onClick={() => {
                setWorkshopSource("collection");
                setWorkshopPreview(null);
              }}
            >
              {t("steamCollection")}
            </button>
            <button
              type="button"
              className={cn("rounded px-3 py-2 text-sm transition", workshopSource === "ids" ? "bg-slate-800 text-white" : "text-slate-400 hover:text-white")}
              onClick={() => {
                setWorkshopSource("ids");
                setWorkshopPreview(null);
              }}
            >
              {t("workshopIds")}
            </button>
          </div>
          <label className="mb-3 grid gap-1.5 text-sm text-slate-300">
            <span className="text-xs font-medium text-slate-400">{t("filterGame")}</span>
            <select
              className="rounded-md border border-panel-line bg-slate-950 px-3 py-2 text-sm text-slate-100 outline-none focus:border-panel-green"
              value={workshopProviderKey}
              onChange={(event) => {
                setWorkshopProviderKey(event.target.value as ProviderKey);
                setWorkshopPreview(null);
                setSelectedWorkshopIds([]);
              }}
              disabled={workshopImport.isPending || workshopCollectionPreview.isPending || workshopItemsPreview.isPending}
            >
              <option value="terraria-tmodloader">tModLoader</option>
              <option value="dont-starve-together">Don't Starve Together</option>
            </select>
          </label>
          {workshopUnsupported && (
            <p className="mb-3 rounded-md border border-panel-gold/25 bg-panel-gold/5 px-3 py-2 text-xs leading-5 text-panel-gold">
              {t("workshopArmPreviewOnly")}
            </p>
          )}
          {workshopSource === "collection" ? (
            <>
              {!workshopPreview ? (
                <form
                  className="grid gap-2"
                  onSubmit={(event) => {
                    event.preventDefault();
                    workshopCollectionPreview.mutate(undefined);
                  }}
                >
                  <Input
                    aria-label={t("steamCollectionPlaceholder")}
                    placeholder={t("steamCollectionPlaceholder")}
                    value={workshopCollectionValue}
                    onChange={(event) => setWorkshopCollectionValue(event.target.value)}
                    onPaste={(event) => {
                      const value = event.clipboardData.getData("text").trim();
                      if (!value) return;
                      event.preventDefault();
                      setWorkshopCollectionValue(value);
                      workshopCollectionPreview.mutate(value);
                    }}
                    disabled={workshopCollectionPreview.isPending}
                  />
                  <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
                    <p className="text-slate-500">{t("steamCollectionHint")}</p>
                    <a
                      href={steamWorkshopURL(workshopProviderKey, "collections")}
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex items-center gap-1 text-slate-400 transition hover:text-panel-green"
                    >
                      {t("browseSteamCollections")}
                      <ExternalLink aria-hidden="true" className="size-3" />
                    </a>
                  </div>
                  {workshopCollectionPreview.isError && (
                    <p className="rounded-md border border-panel-gold/25 bg-panel-gold/5 px-3 py-2 text-xs leading-5 text-panel-gold">
                      {workshopCollectionPreview.error instanceof Error ? workshopCollectionPreview.error.message : t("unablePreviewWorkshopCollection")}
                    </p>
                  )}
                  <Button
                    type="submit"
                    variant="secondary"
                    className="justify-self-end"
                    disabled={workshopCollectionPreview.isPending || workshopCollectionValue.trim().length === 0}
                  >
                    <Download aria-hidden="true" />
                    {workshopCollectionPreview.isPending ? t("resolvingCollection") : t("previewCollection")}
                  </Button>
                </form>
              ) : (
                <WorkshopImportPreview
                  preview={workshopPreview}
                  selectedIds={selectedWorkshopIds}
                  busy={workshopImport.isPending}
                  unsupported={workshopUnsupported}
                  onBack={() => setWorkshopPreview(null)}
                  onCancel={() => setWorkshopDialogOpen(false)}
                  onToggle={(id) => setSelectedWorkshopIds((current) => current.includes(id) ? current.filter((value) => value !== id) : [...current, id])}
                  onImport={() => workshopImport.mutate({ ids: selectedWorkshopIds, providerKey: workshopProviderKey, previewId: workshopPreview.previewId })}
                />
              )}
            </>
          ) : (
            workshopPreview ? (
              <WorkshopImportPreview
                preview={workshopPreview}
                selectedIds={selectedWorkshopIds}
                busy={workshopImport.isPending}
                unsupported={workshopUnsupported}
                onBack={() => setWorkshopPreview(null)}
                onCancel={() => setWorkshopDialogOpen(false)}
                onToggle={(id) => setSelectedWorkshopIds((current) => current.includes(id) ? current.filter((value) => value !== id) : [...current, id])}
                onImport={() => workshopImport.mutate({ ids: selectedWorkshopIds, providerKey: workshopProviderKey, previewId: workshopPreview.previewId })}
              />
            ) : (
              <form
                onSubmit={(event) => {
                  event.preventDefault();
                  requestWorkshopPreview(workshopIds, workshopProviderKey);
                }}
              >
                <div className="grid gap-2">
                  <textarea
                    aria-label={t("workshopIdsPlaceholder")}
                    className="min-h-24 w-full resize-none rounded-md border border-panel-line bg-slate-950 px-3 py-2 text-sm text-slate-100 outline-none placeholder:text-slate-500 focus:border-panel-green"
                    placeholder={t("workshopIdsPlaceholder")}
                    value={workshopIdsText}
                    onChange={(event) => setWorkshopIdsText(event.target.value)}
                    disabled={workshopItemsPreview.isPending}
                  />
                  <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
                    <p className="text-slate-500">{t("workshopIdHint")}</p>
                    <a
                      href={steamWorkshopURL(workshopProviderKey, "items")}
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex items-center gap-1 text-slate-400 transition hover:text-panel-green"
                    >
                      {t("browseSteamWorkshop")}
                      <ExternalLink aria-hidden="true" className="size-3" />
                    </a>
                  </div>
                  {workshopItemsPreview.isError ? (
                    <p className="rounded-md border border-panel-gold/25 bg-panel-gold/5 px-3 py-2 text-xs leading-5 text-panel-gold">
                      {workshopItemsPreview.error instanceof Error ? workshopItemsPreview.error.message : t("unablePreviewWorkshopItems")}
                    </p>
                  ) : null}
                </div>
                <div className="mt-4 flex justify-end gap-2">
                  <Button variant="ghost" onClick={() => setWorkshopDialogOpen(false)} disabled={workshopItemsPreview.isPending}>{t("cancel")}</Button>
                  <Button type="submit" variant="secondary" disabled={workshopItemsPreview.isPending || workshopIds.length === 0 || workshopUnsupported} title={workshopUnsupported ? t("workshopArmUnsupported") : undefined}>
                    <Download aria-hidden="true" />
                    {workshopItemsPreview.isPending ? t("resolvingWorkshopItems") : t("previewWorkshopItems")}
                  </Button>
                </div>
              </form>
            )
          )}
        </DialogShell>
      )}

      {packImportDialogOpen && (
        <DialogShell
          title={t("importFromSteam")}
          description={t("importPackFromSteamHint")}
          onClose={() => {
            setPackImportDialogOpen(false);
            setPackCollectionPreview(null);
          }}
        >
          {!packCollectionPreview ? (
            <form
              className="grid gap-4"
              onSubmit={(event) => {
                event.preventDefault();
                packCollectionPreviewMutation.mutate(undefined);
              }}
            >
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-slate-400">{t("filterGame")}</span>
                <select
                  className="rounded-md border border-panel-line bg-slate-950 px-3 py-2 text-sm text-slate-100 outline-none focus:border-panel-green"
                  value={packCollectionProviderKey}
                  onChange={(event) => {
                    setPackCollectionProviderKey(event.target.value as ProviderKey);
                    setPackCollectionPreview(null);
                  }}
                  disabled={packCollectionPreviewMutation.isPending}
                >
                  <option value="terraria-tmodloader">tModLoader</option>
                  <option value="dont-starve-together">Don't Starve Together</option>
                </select>
              </label>
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-slate-400">{t("steamCollection")}</span>
                <Input
                  aria-label={t("steamCollectionPlaceholder")}
                  placeholder={t("steamCollectionPlaceholder")}
                  value={packCollectionValue}
                  onChange={(event) => setPackCollectionValue(event.target.value)}
                  onPaste={(event) => {
                    const value = event.clipboardData.getData("text").trim();
                    if (!value) return;
                    event.preventDefault();
                    setPackCollectionValue(value);
                    packCollectionPreviewMutation.mutate(value);
                  }}
                  disabled={packCollectionPreviewMutation.isPending}
                />
              </label>
              <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
                <p className="text-slate-500">{t("steamCollectionPackOnlyHint")}</p>
                <a
                  href={steamWorkshopURL(packCollectionProviderKey, "collections")}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-1 text-slate-400 transition hover:text-panel-green"
                >
                  {t("browseSteamCollections")}
                  <ExternalLink aria-hidden="true" className="size-3" />
                </a>
              </div>
              <div className="flex justify-end gap-2">
                <Button variant="ghost" onClick={() => setPackImportDialogOpen(false)} disabled={packCollectionPreviewMutation.isPending}>{t("cancel")}</Button>
                <Button type="submit" variant="secondary" disabled={packCollectionPreviewMutation.isPending || packCollectionValue.trim() === ""}>
                  <Download aria-hidden="true" />
                  {packCollectionPreviewMutation.isPending ? t("resolvingCollection") : t("previewCollection")}
                </Button>
              </div>
            </form>
          ) : (
            <div>
              <div className="flex flex-wrap items-start justify-between gap-3 border-b border-panel-line pb-3">
                <div>
                  <p className="font-medium text-slate-100">{packCollectionPreview.collectionName || t("steamCollectionNumber", { id: packCollectionPreview.collectionId })}</p>
                  <p className="mt-1 text-xs text-slate-500">
                    {t("workshopPreviewSummary", {
                      total: packCollectionPreview.summary.total,
                      added: packCollectionPreview.summary.new,
                      existing: packCollectionPreview.summary.inLibrary
                    })}
                  </p>
                </div>
                <Button variant="ghost" onClick={() => setPackCollectionPreview(null)} disabled={createPackFromCollection.isPending}>{t("changeCollection")}</Button>
              </div>
              <div className="mt-4 grid gap-3 sm:grid-cols-2">
                <label className="grid gap-1.5">
                  <span className="text-xs font-medium text-slate-400">{t("modPackName")}</span>
                  <Input value={packCollectionName} onChange={(event) => setPackCollectionName(event.target.value)} placeholder={t("modPackName")} disabled={createPackFromCollection.isPending} />
                </label>
                <label className="grid gap-1.5">
                  <span className="text-xs font-medium text-slate-400">{t("modPackDescription")}</span>
                  <Input value={packCollectionDescription} onChange={(event) => setPackCollectionDescription(event.target.value)} placeholder={t("modPackDescription")} disabled={createPackFromCollection.isPending} />
                </label>
              </div>
              <div className="mt-4 max-h-64 divide-y divide-panel-line overflow-y-auto rounded-md border border-panel-line bg-slate-950/35">
                {packCollectionPreview.items.map((item) => (
                  <div key={item.workshopId} className={cn("flex items-center gap-3 px-3 py-2.5", !item.selectable && "opacity-50")}>
                    <span className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded bg-slate-900">
                      {item.previewUrl ? <Image src={item.previewUrl} alt="" width={40} height={40} className="size-full object-cover" unoptimized /> : <Package aria-hidden="true" className="size-4 text-slate-500" />}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium text-slate-100">{item.title || item.workshopId}</span>
                      <span className="mt-0.5 block text-xs text-slate-500">{item.size} · {workshopPreviewStatusLabel(item.status, t)}</span>
                    </span>
                  </div>
                ))}
              </div>
              {packCollectionPreview.summary.unavailable > 0 ? (
                <p className="mt-3 text-xs leading-5 text-panel-gold">{t("collectionUnavailableModsSkipped", { count: packCollectionPreview.summary.unavailable })}</p>
              ) : null}
              <div className="mt-4 flex justify-end gap-2">
                <Button variant="ghost" onClick={() => setPackImportDialogOpen(false)} disabled={createPackFromCollection.isPending}>{t("cancel")}</Button>
                <Button
                  variant="secondary"
                  onClick={() => createPackFromCollection.mutate()}
                  disabled={createPackFromCollection.isPending || packCollectionName.trim() === "" || !packCollectionPreview.items.some((item) => item.selectable)}
                >
                  <Package aria-hidden="true" />
                  {createPackFromCollection.isPending ? t("actionWorking") : t("createPackFromCollection")}
                </Button>
              </div>
            </div>
          )}
        </DialogShell>
      )}

      {packDialogOpen && (
        <DialogShell
          title={t("createModPack")}
          description={t("modPacksHint")}
          onClose={() => setPackDialogOpen(false)}
        >
          <div className="grid gap-3">
            <label className="grid gap-1.5">
              <span className="text-xs font-medium text-slate-500">{t("modPackName")}</span>
              <Input value={packName} onChange={(event) => setPackName(event.target.value)} placeholder={t("modPackName")} />
            </label>
            <label className="grid gap-1.5">
              <span className="text-xs font-medium text-slate-500">{t("modPackDescription")}</span>
              <Input value={packDescription} onChange={(event) => setPackDescription(event.target.value)} placeholder={t("modPackDescription")} />
            </label>
          </div>
          <div className="mt-4 rounded-md border border-panel-line bg-slate-950/45">
            <div className="flex items-center justify-between border-b border-panel-line px-3 py-2">
              <span className="text-sm font-medium text-white">{t("modLibrary")}</span>
              <span className="text-xs text-slate-500">{t("selectedForPack", { count: selectedPackModCount })}</span>
            </div>
            {selectedPackDependencies.length > 0 ? (
              <div className="border-b border-panel-line bg-panel-gold/10 px-3 py-2 text-xs text-panel-gold">
                {t("packWillIncludeDependencies", { names: selectedPackDependencies.join(", ") })}
              </div>
            ) : null}
            <div className="max-h-64 space-y-2 overflow-y-auto p-3">
              {globalMods.map((mod) => {
                const selected = selectedPackModIds.includes(mod.id);
                return (
                  <button
                    key={mod.id}
                    type="button"
                    className={cn(
                      "flex w-full items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/60 px-3 py-2 text-left transition hover:border-panel-green/35",
                      selected && "border-panel-green/60 bg-panel-green/10"
                    )}
                    onClick={() => togglePackMod(mod.id)}
                  >
                      <span className="min-w-0">
                        <span className="block truncate text-sm font-medium text-white">{modDisplayName(mod, locale)}</span>
                        <span className="mt-0.5 block truncate text-xs text-slate-500">{mod.size} · {localizeRelativeTime(mod.created, locale)}</span>
                        {mod.dependencies && mod.dependencies.length > 0 ? (
                          <span className="mt-1 block truncate text-xs text-panel-gold">
                            {t("dependencies")}: {mod.dependencies.join(", ")}
                          </span>
                        ) : null}
                      </span>
                    {selected && <Check aria-hidden="true" className="size-4 shrink-0 text-panel-green" />}
                  </button>
                );
              })}
              {!globalModsQuery.isLoading && globalMods.length === 0 && <p className="px-1 py-4 text-center text-sm text-slate-500">{t("noGlobalMods")}</p>}
            </div>
          </div>
          <div className="mt-4 flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setPackDialogOpen(false)} disabled={createPack.isPending}>{t("cancel")}</Button>
            <Button
              variant="secondary"
              onClick={() => createPack.mutate()}
              disabled={createPack.isPending || packName.trim() === "" || selectedPackModCount === 0}
            >
              <Package aria-hidden="true" />
              {createPack.isPending ? t("actionWorking") : t("createModPack")}
            </Button>
          </div>
        </DialogShell>
      )}

      <ConfirmDialog
        open={Boolean(pendingDelete)}
        eyebrow={t("destructiveAction")}
        title={t("deleteModConfirm", { name: pendingDelete ? modDisplayName(pendingDelete, locale) : "" })}
        description={t("confirmDeleteModDescription", { name: pendingDelete ? modDisplayName(pendingDelete, locale) : "" })}
        detail={pendingDelete ? (
          <>
            <span className="text-slate-500">{t("modsTitle")}: </span>
            <span className="font-medium text-white">{modDisplayName(pendingDelete, locale)}</span>
          </>
        ) : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={removeGlobal.isPending ? t("actionWorking") : t("delete")}
        busy={removeGlobal.isPending}
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => {
          if (!pendingDelete) return;
          removeGlobal.mutate(pendingDelete.id);
        }}
      />
      <ConfirmDialog
        open={Boolean(pendingPackDelete)}
        eyebrow={t("destructiveAction")}
        title={t("deleteModPackConfirm", { name: pendingPackDelete?.name ?? "" })}
        description={t("confirmDeleteModPackDescription")}
        detail={pendingPackDelete ? (
          <>
            <span className="text-slate-500">{t("modPacks")}: </span>
            <span className="font-medium text-white">{pendingPackDelete.name}</span>
          </>
        ) : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={removePack.isPending ? t("actionWorking") : t("delete")}
        busy={removePack.isPending}
        onCancel={() => setPendingPackDelete(null)}
        onConfirm={() => pendingPackDelete && removePack.mutate(pendingPackDelete.id)}
      />
      <ConfirmDialog
        open={Boolean(pendingBulkDelete)}
        eyebrow={t("destructiveAction")}
        title={pendingBulkDelete === "library"
          ? t("bulkRemoveModsConfirm", { count: selectedLibraryIds.size })
          : t("bulkDeleteModPacksConfirm", { count: selectedModPackIds.size })}
        description={pendingBulkDelete === "library" ? t("bulkRemoveModsDescription") : t("confirmDeleteModPackDescription")}
        detail={pendingBulkDelete === "library" ? (
          <span>{selectedLibraryMods.slice(0, 6).map((item) => modDisplayName(item, locale)).join("、")}{selectedLibraryMods.length > 6 ? ` +${selectedLibraryMods.length - 6}` : ""}</span>
        ) : pendingBulkDelete === "packs" ? (
          <span>{selectedModPacks.slice(0, 6).map((item) => item.name).join("、")}{selectedModPacks.length > 6 ? ` +${selectedModPacks.length - 6}` : ""}</span>
        ) : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={bulkRemove.isPending ? t("actionWorking") : t("delete")}
        busy={bulkRemove.isPending}
        onCancel={() => setPendingBulkDelete(null)}
        onConfirm={() => {
          if (pendingBulkDelete === "library") bulkRemove.mutate({ kind: "library", ids: Array.from(selectedLibraryIds) });
          if (pendingBulkDelete === "packs") bulkRemove.mutate({ kind: "packs", ids: Array.from(selectedModPackIds) });
        }}
      />
    </>
  );
}


function WorkshopImportPreview({
  busy,
  onBack,
  onCancel,
  onImport,
  onToggle,
  preview,
  selectedIds,
  unsupported
}: {
  busy: boolean;
  onBack: () => void;
  onCancel: () => void;
  onImport: () => void;
  onToggle: (id: string) => void;
  preview: WorkshopPreview;
  selectedIds: string[];
  unsupported: boolean;
}) {
  const { t } = useI18n();
  const title = preview.collectionName
    || (preview.collectionId ? t("steamCollectionNumber", { id: preview.collectionId }) : t("workshopItemsPreviewTitle"));
  return (
    <div>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-md border border-panel-line bg-slate-950/45 px-3 py-2.5">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-slate-100">{title}</p>
          <p className="mt-0.5 text-xs text-slate-500">
            {t("workshopPreviewSummary", {
              total: preview.summary.total,
              added: preview.summary.new,
              existing: preview.summary.inLibrary
            })}
          </p>
        </div>
        <Button variant="ghost" onClick={onBack} disabled={busy}>{preview.collectionId ? t("changeCollection") : t("changeWorkshopItems")}</Button>
      </div>
      <div className="max-h-80 space-y-2 overflow-y-auto pr-1">
        {preview.items.map((item) => {
          const checked = selectedIds.includes(item.workshopId);
          return (
            <button
              key={item.workshopId}
              type="button"
              className={cn(
                "flex w-full items-center gap-3 rounded-md border px-3 py-2 text-left transition",
                checked ? "border-panel-green/50 bg-panel-green/5" : "border-panel-line bg-slate-950/35",
                !item.selectable && "cursor-not-allowed opacity-55"
              )}
              disabled={!item.selectable || busy}
              onClick={() => onToggle(item.workshopId)}
            >
              <span className={cn("flex size-5 shrink-0 items-center justify-center rounded border", checked ? "border-panel-green bg-panel-green text-slate-950" : "border-slate-600")}>
                {checked && <Check aria-hidden="true" className="size-3" />}
              </span>
              <span className="flex size-11 shrink-0 items-center justify-center overflow-hidden rounded bg-slate-900">
                {item.previewUrl ? <Image src={item.previewUrl} alt="" width={44} height={44} className="size-full object-cover" unoptimized /> : <Package aria-hidden="true" className="size-5 text-slate-500" />}
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium text-slate-100">{item.title || item.workshopId}</span>
                <span className="mt-0.5 block text-xs text-slate-500">{item.size} · {workshopPreviewStatusLabel(item.status, t)}</span>
              </span>
            </button>
          );
        })}
      </div>
      {preview.summary.unavailable > 0 ? (
        <p className="mt-3 text-xs leading-5 text-panel-gold">{t("workshopUnavailableItemsSkipped", { count: preview.summary.unavailable })}</p>
      ) : null}
      <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
        <span className="text-xs text-slate-500">{t("workshopIdsSelected", { count: selectedIds.length })}</span>
        <div className="flex gap-2">
          <Button variant="ghost" onClick={onCancel} disabled={busy}>{t("cancel")}</Button>
          <Button variant="secondary" onClick={onImport} disabled={busy || selectedIds.length === 0 || unsupported}>
            <Download aria-hidden="true" />
            {busy ? t("actionWorking") : t("confirmWorkshopImport")}
          </Button>
        </div>
      </div>
    </div>
  );
}

function DialogShell({ children, description, onClose, title }: { children: ReactNode; description?: string; onClose: () => void; title: string }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/75 px-4 py-8">
      <div className="w-full max-w-2xl rounded-lg border border-panel-line bg-panel-card shadow-2xl shadow-black/30" role="dialog" aria-modal="true" aria-label={title}>
        <div className="flex items-start justify-between gap-4 border-b border-panel-line p-4">
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-white">{title}</h2>
            {description ? <p className="mt-1 text-sm text-slate-500">{description}</p> : null}
          </div>
          <button
            type="button"
            className="flex size-8 shrink-0 items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/50"
            onClick={onClose}
            aria-label="Close"
          >
            <X aria-hidden="true" className="size-4" />
          </button>
        </div>
        <div className="p-4">{children}</div>
      </div>
    </div>
  );
}

function steamWorkshopURL(providerKey: ProviderKey, section: "collections" | "items") {
  const appID = providerKey === "dont-starve-together" ? "322330" : "1281930";
  if (section === "collections") {
    return `https://steamcommunity.com/workshop/browse/?appid=${appID}&section=collections`;
  }
  return `https://steamcommunity.com/app/${appID}/workshop/`;
}

function LibraryModTable({
  deleting,
  games,
  items,
  locale,
  onDelete,
  onSelectionChange,
  selectedIds
}: {
  deleting: boolean;
  games: GameCatalogEntry[];
  items: ModFile[];
  locale: string;
  onDelete: (item: ModFile) => void;
  onSelectionChange: (ids: Set<string>) => void;
  selectedIds: Set<string>;
}) {
  const { t } = useI18n();
  const selectedVisibleCount = items.filter((item) => selectedIds.has(item.id)).length;
  const allSelected = items.length > 0 && selectedVisibleCount === items.length;
  const toggleAll = () => {
    const next = new Set(selectedIds);
    if (allSelected) items.forEach((item) => next.delete(item.id));
    else items.forEach((item) => next.add(item.id));
    onSelectionChange(next);
  };
  const toggleItem = (id: string) => {
    const next = new Set(selectedIds);
    if (next.has(id)) next.delete(id); else next.add(id);
    onSelectionChange(next);
  };
  return (
    <div className="overflow-hidden rounded-lg border border-panel-line bg-panel-card">
      <div className="hidden overflow-x-auto md:block">
        <table className="w-full min-w-[860px] border-collapse text-left text-sm">
          <thead className="bg-slate-950/45 text-xs font-medium text-slate-500">
            <tr>
              <th className="w-11 px-4 py-3"><SelectionBox checked={allSelected} indeterminate={selectedVisibleCount > 0 && !allSelected} label={t("selectAll")} onChange={toggleAll} /></th>
              <th className="px-4 py-3">{t("modsTitle")}</th>
              <th className="px-3 py-3">{t("modSource")}</th>
              <th className="px-3 py-3">{t("modVersion")}</th>
              <th className="px-3 py-3">{t("dependencies")}</th>
              <th className="px-3 py-3">{t("size")}</th>
              <th className="px-3 py-3">{t("modified")}</th>
              <th className="px-4 py-3 text-right">{t("actions")}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-panel-line">
            {items.map((item) => {
              const sourceURL = item.workshopId ? `https://steamcommunity.com/sharedfiles/filedetails/?id=${item.workshopId}` : "";
              const catalog = modCatalogLabel(item.gameKey, item.providerKey, games, t);
              return (
                <tr className={cn("group hover:bg-slate-800/35", selectedIds.has(item.id) && "bg-panel-green/[0.06]")} key={item.id}>
                  <td className="w-11 px-4 py-3"><SelectionBox checked={selectedIds.has(item.id)} label={t("selectMod", { name: modDisplayName(item, locale) })} onChange={() => toggleItem(item.id)} /></td>
                  <td className="px-4 py-3">
                    <div className="flex min-w-0 items-center gap-3">
                      <span className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-md border border-panel-line bg-slate-950/55">
                        {item.previewUrl ? <Image src={item.previewUrl} alt="" className="size-full object-cover" width={40} height={40} unoptimized /> : <Package aria-hidden="true" className="size-4 text-slate-500" />}
                      </span>
                      <div className="min-w-0">
                        <Link href={`/mods/${item.id}`} className="block max-w-72 truncate font-medium text-slate-100 group-hover:text-panel-green">{modDisplayName(item, locale)}</Link>
                        <p className="mt-0.5 max-w-72 truncate text-xs text-slate-500">{catalog || item.fileName}</p>
                      </div>
                    </div>
                  </td>
                  <td className="px-3 py-3 text-slate-300">
                    {sourceURL ? <a className="inline-flex items-center gap-1 hover:text-panel-green" href={sourceURL} target="_blank" rel="noreferrer">{modSourceLabel(item, locale)}<ExternalLink aria-hidden="true" className="size-3" /></a> : modSourceLabel(item, locale)}
                  </td>
                  <td className="px-3 py-3 font-mono text-xs text-slate-300">{item.modVersion || item.tmodVersion || "—"}</td>
                  <td className="px-3 py-3 text-slate-400">{item.dependencies?.length ? t("itemsCount", { count: item.dependencies.length }) : t("none")}</td>
                  <td className="px-3 py-3 text-slate-300">{item.size}</td>
                  <td className="px-3 py-3 text-slate-400">{item.updatedAtSteam ? formatWorkshopUpdated(item.updatedAtSteam, locale) : localizeRelativeTime(item.created, locale === "zh" ? "zh" : "en")}</td>
                  <td className="px-4 py-3 text-right">
                    <Button variant="danger" className="h-8 px-2.5" aria-label={t("removeFromModLibrary")} onClick={() => onDelete(item)} disabled={deleting}><Trash2 aria-hidden="true" className="size-3.5" /></Button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <div className="grid gap-3 p-3 md:hidden">
        {items.map((item) => (
          <div className={cn("relative rounded-lg", selectedIds.has(item.id) && "ring-1 ring-panel-green/60")} key={item.id}>
            <div className="absolute left-3 top-3 z-10"><SelectionBox checked={selectedIds.has(item.id)} label={t("selectMod", { name: modDisplayName(item, locale) })} onChange={() => toggleItem(item.id)} /></div>
            <div className="pl-7"><LibraryModCard item={item} locale={locale} deleting={deleting} onDelete={() => onDelete(item)} /></div>
          </div>
        ))}
      </div>
    </div>
  );
}

function ModPackTable({
  deleting,
  games,
  locale,
  onDelete,
  onSelectionChange,
  selectedIds,
  packs
}: {
  deleting: boolean;
  games: GameCatalogEntry[];
  locale: string;
  onDelete: (pack: ModPack) => void;
  onSelectionChange: (ids: Set<string>) => void;
  selectedIds: Set<string>;
  packs: ModPack[];
}) {
  const { t } = useI18n();
  const selectedVisibleCount = packs.filter((pack) => selectedIds.has(pack.id)).length;
  const allSelected = packs.length > 0 && selectedVisibleCount === packs.length;
  const toggleAll = () => {
    const next = new Set(selectedIds);
    if (allSelected) packs.forEach((pack) => next.delete(pack.id));
    else packs.forEach((pack) => next.add(pack.id));
    onSelectionChange(next);
  };
  const toggleItem = (id: string) => {
    const next = new Set(selectedIds);
    if (next.has(id)) next.delete(id); else next.add(id);
    onSelectionChange(next);
  };
  return (
    <div className="overflow-hidden rounded-lg border border-panel-line bg-panel-card">
      <div className="hidden overflow-x-auto md:block">
        <table className="w-full min-w-[760px] border-collapse text-left text-sm">
          <thead className="bg-slate-950/45 text-xs font-medium text-slate-500">
            <tr>
              <th className="w-11 px-4 py-3"><SelectionBox checked={allSelected} indeterminate={selectedVisibleCount > 0 && !allSelected} label={t("selectAll")} onChange={toggleAll} /></th>
              <th className="px-4 py-3">{t("modPackName")}</th>
              <th className="px-3 py-3">{t("gameAndMode")}</th>
              <th className="px-3 py-3">{t("modsTitle")}</th>
              <th className="px-3 py-3">{t("created")}</th>
              <th className="px-4 py-3 text-right">{t("actions")}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-panel-line">
            {packs.map((pack) => {
              const catalog = modCatalogLabel(pack.gameKey, pack.providerKey, games, t);
              return <tr className={cn("group hover:bg-slate-800/35", selectedIds.has(pack.id) && "bg-panel-green/[0.06]")} key={pack.id}>
                <td className="w-11 px-4 py-3"><SelectionBox checked={selectedIds.has(pack.id)} label={t("selectModPack", { name: pack.name })} onChange={() => toggleItem(pack.id)} /></td>
                <td className="px-4 py-3">
                  <Link href={`/mods/packs/${pack.id}`} className="block max-w-80 truncate font-medium text-slate-100 group-hover:text-panel-green">{pack.name}</Link>
                  <p className="mt-0.5 max-w-80 truncate text-xs text-slate-500">{pack.description || pack.mods.slice(0, 3).map((mod) => modDisplayName(mod, locale)).join("、")}</p>
                </td>
                <td className="px-3 py-3 text-slate-300">
                  <p>{catalog || "—"}</p>
                </td>
                <td className="px-3 py-3">
                  <p className="text-slate-200">{t("itemsCount", { count: pack.mods.length })}</p>
                  <p className="mt-0.5 max-w-96 truncate text-xs text-slate-500">{pack.mods.slice(0, 4).map((mod) => modDisplayName(mod, locale)).join("、")}{pack.mods.length > 4 ? ` +${pack.mods.length - 4}` : ""}</p>
                </td>
                <td className="px-3 py-3 text-slate-400">{localizeRelativeTime(pack.created, locale === "zh" ? "zh" : "en")}</td>
                <td className="px-4 py-3 text-right"><Button variant="danger" className="h-8 px-2.5" aria-label={t("delete")} onClick={() => onDelete(pack)} disabled={deleting}><Trash2 aria-hidden="true" className="size-3.5" /></Button></td>
              </tr>;
            })}
          </tbody>
        </table>
      </div>
      <div className="divide-y divide-panel-line md:hidden">
        {packs.map((pack) => (
          <div className={cn("p-4", selectedIds.has(pack.id) && "bg-panel-green/[0.06]")} key={pack.id}>
            <div className="flex items-start justify-between gap-3">
              <div className="flex min-w-0 items-start gap-3"><SelectionBox checked={selectedIds.has(pack.id)} label={t("selectModPack", { name: pack.name })} onChange={() => toggleItem(pack.id)} /><div className="min-w-0"><Link href={`/mods/packs/${pack.id}`} className="truncate font-medium text-slate-100">{pack.name}</Link><p className="mt-1 truncate text-xs text-slate-500">{pack.description || t("itemsCount", { count: pack.mods.length })}</p></div></div>
              <Button variant="danger" className="h-8 px-2.5" aria-label={t("delete")} onClick={() => onDelete(pack)} disabled={deleting}><Trash2 aria-hidden="true" className="size-3.5" /></Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function modCatalogLabel(
  gameKey: string | undefined,
  providerKey: ProviderKey | undefined,
  games: GameCatalogEntry[],
  t: (key: MessageKey) => string
) {
  const game = games.find((entry) => entry.key === gameKey || entry.providers.some((provider) => provider.key === providerKey));
  const provider = game?.providers.find((entry) => entry.key === providerKey);
  const gameName = game?.name || gameKey || "";
  const providerName = providerDisplayName(providerKey, provider?.name || providerKey || "", t);
  if (gameName && providerName && gameName !== providerName) return `${gameName} · ${providerName}`;
  return providerName || gameName;
}

function ModMetadataStrip({ item }: { item: ModFile }) {
  const { locale, t } = useI18n();
  const displayName = modDisplayName(item, locale);
  const entries = [
    item.modName && item.modName !== displayName ? [t("internalModName"), item.modName] : null,
    item.modVersion ? [t("modVersion"), item.modVersion] : null,
    item.tmodVersion ? [t("tmodVersion"), item.tmodVersion] : null,
    item.dependencies && item.dependencies.length > 0 ? [t("dependencies"), item.dependencies.join(", ")] : null
  ].filter(Boolean) as [string, string][];
  if (entries.length === 0) return null;
  return (
    <div className="grid gap-2 px-4 pb-4 sm:grid-cols-2">
      {entries.map(([label, value]) => (
        <div key={label} className="min-w-0 rounded-md border border-panel-line bg-slate-950/40 px-3 py-2">
          <p className="text-xs text-slate-500">{label}</p>
          <p className="mt-1 truncate text-sm font-medium text-slate-100" title={value}>{value}</p>
        </div>
      ))}
    </div>
  );
}

function LibraryModCard({
  deleting,
  item,
  locale,
  onDelete
}: {
  deleting: boolean;
  item: ModFile;
  locale: string;
  onDelete: () => void;
}) {
  const { t } = useI18n();
  const sourceURL = item.workshopId
    ? `https://steamcommunity.com/sharedfiles/filedetails/?id=${item.workshopId}`
    : "";
  const description = sanitizeWorkshopDescription(item.description ?? "");
  const hasWorkshopStats = Boolean(item.subscriptions || item.updatedAtSteam);

  return (
    <Card className="overflow-hidden p-0 transition hover:border-panel-green/25">
      <div className="flex gap-4 p-4">
        <div className="flex size-20 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-panel-line bg-slate-950/55">
          {item.previewUrl ? (
            <Image src={item.previewUrl} alt="" className="size-full object-cover" width={80} height={80} unoptimized />
          ) : (
            <Package aria-hidden="true" className="size-5 text-slate-500" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <Link href={`/mods/${item.id}`} className="rounded-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/50">
                <h3 className="truncate font-semibold text-white transition hover:text-panel-green">{modDisplayName(item, locale)}</h3>
              </Link>
              <p className="mt-1 truncate text-xs text-slate-500">
                {item.workshopId
                  ? `${locale === "zh" ? "创意工坊" : "Workshop"} ${item.workshopId}`
                  : modSourceLabel(item, locale)}
              </p>
            </div>
            <Badge className="shrink-0 bg-slate-800 text-slate-300">{modSourceLabel(item, locale)}</Badge>
          </div>
          <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-400">
            {item.subscriptions ? (
              <StatPill icon={<Users aria-hidden="true" className="size-3.5" />} label={`${item.subscriptions.toLocaleString()} ${locale === "zh" ? "订阅" : "subs"}`} />
            ) : null}
            {item.updatedAtSteam ? (
              <StatPill icon={<Clock3 aria-hidden="true" className="size-3.5" />} label={formatWorkshopUpdated(item.updatedAtSteam, locale)} />
            ) : null}
            <StatPill icon={<Package aria-hidden="true" className="size-3.5" />} label={item.size} />
            {!hasWorkshopStats ? (
              <span className="inline-flex items-center rounded bg-slate-900 px-2 py-1 text-slate-500">
                {localizeRelativeTime(item.created, locale === "zh" ? "zh" : "en")}
              </span>
            ) : null}
          </div>
        </div>
      </div>
      {description ? (
        <div className="mx-4 mb-3 h-10 overflow-hidden">
          <p className="line-clamp-2 text-sm leading-5 text-slate-400" title={description}>{description}</p>
        </div>
      ) : null}
      {item.tags && item.tags.length > 0 ? (
        <div className="flex min-h-6 items-center gap-2 overflow-hidden px-4 pb-4">
          {item.tags.slice(0, 3).map((tag) => (
            <span key={tag} className="max-w-40 shrink-0 truncate rounded bg-slate-900 px-2 py-1 text-xs text-slate-300" title={tag}>{tag}</span>
          ))}
          {item.tags.length > 3 ? <span className="shrink-0 text-xs text-slate-500">+{item.tags.length - 3}</span> : null}
        </div>
      ) : null}
      <ModMetadataStrip item={item} />
      <div className="flex min-h-12 flex-wrap items-center justify-between gap-2 border-t border-panel-line px-4 py-2.5">
        {sourceURL ? (
          <a
            href={sourceURL}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1.5 text-xs text-slate-400 transition hover:text-panel-green"
          >
            {locale === "zh" ? "打开 Steam 工坊" : "Open Steam Workshop"}
            <ExternalLink aria-hidden="true" className="size-3.5" />
          </a>
        ) : <span />}
        <div className="ml-auto flex items-center gap-1">
          <Button
            variant="danger"
            className="h-8 gap-1.5 px-2.5 py-0 text-xs"
            aria-label={locale === "zh" ? `从模组库移除 ${modDisplayName(item, locale)}` : `Remove ${modDisplayName(item, locale)} from library`}
            onClick={onDelete}
            disabled={deleting}
          >
            <Trash2 aria-hidden="true" className="size-3.5" />
            {t("removeFromModLibrary")}
          </Button>
        </div>
      </div>
    </Card>
  );
}

function RecommendedModCard({
  busy,
  disabledReason,
  item,
  locale,
  onAdd
}: {
  item: RecommendedMod;
  locale: string;
  busy: boolean;
  disabledReason: string;
  onAdd: () => void;
}) {
  return (
    <Card className="overflow-hidden p-0 transition hover:border-panel-green/25">
      <div className="flex gap-4 p-4">
        <div className="flex size-24 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-panel-line bg-slate-950/55">
          {item.previewUrl ? <Image src={item.previewUrl} alt={item.title} className="size-full object-cover" width={96} height={96} unoptimized /> : <Package aria-hidden="true" className="size-6 text-slate-500" />}
        </div>
        <div className="min-w-0 flex-1">
          <div className="min-w-0">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <h3 className="truncate text-base font-semibold text-white">{item.title}</h3>
                <p className="mt-1 text-xs text-slate-500">{recommendedSourceLabel(item, locale)}</p>
              </div>
              <span className="rounded bg-slate-900 px-2 py-1 text-[11px] font-medium text-slate-300">
                #{item.rank}
              </span>
            </div>
            <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-400">
              <StatPill icon={<Users aria-hidden="true" className="size-3.5" />} label={`${(item.subscriptions ?? 0).toLocaleString()} ${locale === "zh" ? "订阅" : "subs"}`} />
              <StatPill icon={<Clock3 aria-hidden="true" className="size-3.5" />} label={formatWorkshopUpdated(item.timeUpdated, locale)} />
              <StatPill icon={<Package aria-hidden="true" className="size-3.5" />} label={item.size} />
            </div>
          </div>
          <p className="mt-3 line-clamp-3 text-sm text-slate-400">{sanitizeWorkshopDescription(item.description || item.title)}</p>
          {item.dependencies && item.dependencies.length > 0 ? (
            <p className="mt-3 truncate text-xs text-panel-gold">
              {locale === "zh" ? "依赖" : "Dependencies"}: {item.dependencies.join(", ")}
            </p>
          ) : null}
          <div className="mt-3 flex flex-wrap gap-2">
            {(item.tags ?? []).slice(0, 4).map((tag) => (
              <span key={tag} className="rounded bg-slate-900 px-2 py-1 text-xs text-slate-300">{tag}</span>
            ))}
          </div>
        </div>
      </div>
      <div className="flex items-center justify-between gap-3 border-t border-panel-line px-4 py-3 text-xs text-slate-500">
        {recommendedSourceURL(item) ? (
          <a
            href={recommendedSourceURL(item)}
            target="_blank"
            rel="noreferrer"
            className="truncate text-slate-400 transition hover:text-panel-green"
          >
            {isWorkshopRecommended(item) ? (locale === "zh" ? "打开 Steam 工坊" : "Open Steam Workshop") : (locale === "zh" ? "打开来源页面" : "Open source page")}
          </a>
        ) : (
          <span className="truncate text-slate-500">{recommendedSourceLabel(item, locale)}</span>
        )}
        {item.inLibrary ? (
          <Badge className="bg-panel-green/15 text-panel-green">{locale === "zh" ? "已在模组库" : "In library"}</Badge>
        ) : (
          <Button variant="secondary" onClick={onAdd} disabled={busy || Boolean(disabledReason)} title={disabledReason || undefined}>
            <Download aria-hidden="true" />
            {locale === "zh" ? "加入模组库" : "Add to library"}
          </Button>
        )}
      </div>
    </Card>
  );
}

function recommendedModDisabledReason(item: RecommendedMod, workshopUnsupported: boolean, t: (key: MessageKey) => string) {
  if (dstModScopeFromTags(item.providerKey, item.tags) === "client") return t("dstClientOnlyLibraryBlocked");
  if (isWorkshopRecommended(item) && workshopUnsupported) return t("workshopArmUnsupported");
  return "";
}

function dependencyNamesForSelectedMods(mods: ModFile[], selectedIds: string[]) {
  const selected = new Set(selectedIds);
  const names = new Set<string>();
  for (const mod of mods) {
    if (!selected.has(mod.id)) continue;
    for (const dependency of mod.dependencies ?? []) {
      const dependencyInstalled = mods.some((item) => selected.has(item.id) && modIdentity(item) === dependency);
      if (!dependencyInstalled) names.add(dependency);
    }
  }
  return Array.from(names);
}

function isWorkshopRecommended(item: RecommendedMod) {
  return Boolean(item.workshopId) || item.source === "workshop";
}

function recommendedModKey(item: RecommendedMod) {
  return `${item.providerKey ?? "unknown"}:${item.workshopId ?? item.externalId ?? item.fileName ?? item.title}`;
}

function recommendedSourceURL(item: RecommendedMod) {
  if (item.workshopId) return `https://steamcommunity.com/sharedfiles/filedetails/?id=${item.workshopId}`;
  return item.sourceUrl ?? "";
}

function recommendedSourceLabel(item: RecommendedMod, locale: string) {
  if (item.workshopId) return `${locale === "zh" ? "创意工坊" : "Workshop"} ${item.workshopId}`;
  if (item.providerKey === "palworld") return `${locale === "zh" ? "文件模组" : "File mod"} ${item.fileName ?? ".pak"}`;
  return locale === "zh" ? "推荐模组" : "Recommended mod";
}

function buildDependencyImportPlan(ids: string[], providerKey: ProviderKey, recommendedMods: RecommendedMod[], globalMods: ModFile[]): DependencyImportPlan {
  const primaryIds = Array.from(new Set(ids));
  const primarySet = new Set(primaryIds);
  const providerRecommended = recommendedMods.filter((mod) => mod.providerKey === providerKey && mod.workshopId);
  const providerLibrary = globalMods.filter((mod) => mod.providerKey === providerKey);
  const recommendedByWorkshopID = new Map(providerRecommended.map((mod) => [mod.workshopId ?? "", mod]));
  const recommendedByModName = new Map(providerRecommended.flatMap((mod) => mod.modName ? [[mod.modName, mod] as const] : []));
  const libraryNames = new Set(providerLibrary.map(modIdentity));
  const dependencyIds: string[] = [];
  const dependencyNames: string[] = [];
  const queue = [...primaryIds];
  const seenDependencyNames = new Set<string>();
  while (queue.length > 0) {
    const current = queue.shift();
    if (!current) continue;
    const item = recommendedByWorkshopID.get(current);
    for (const dependencyName of item?.dependencies ?? []) {
      if (libraryNames.has(dependencyName) || seenDependencyNames.has(dependencyName)) continue;
      const dependency = recommendedByModName.get(dependencyName);
      if (!dependency?.workshopId || primarySet.has(dependency.workshopId)) continue;
      seenDependencyNames.add(dependencyName);
      dependencyIds.push(dependency.workshopId);
      dependencyNames.push(dependencyName);
      queue.push(dependency.workshopId);
    }
  }
  return { primaryIds, dependencyIds, dependencyNames, providerKey };
}

function modIdentity(mod: ModFile) {
  return mod.modName || mod.title || mod.fileName.replace(/\.[^.]+$/, "");
}

function isArmArchitecture(architecture: string | undefined) {
  const value = (architecture ?? "").toLowerCase();
  return value.startsWith("arm") || value.includes("aarch64");
}

function StatPill({ icon, label }: { icon: ReactNode; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded bg-slate-900 px-2 py-1">
      <span className="text-slate-500">{icon}</span>
      <span>{label}</span>
    </span>
  );
}

function formatWorkshopUpdated(timestamp: number | undefined, locale: string) {
  if (!timestamp) {
    return locale === "zh" ? "更新时间未知" : "Unknown";
  }
  const diff = Math.max(0, Date.now() - timestamp * 1000);
  const minutes = Math.floor(diff / 60000);
  let value = "Just now";
  if (minutes >= 60 && minutes < 1440) {
    value = `${Math.floor(minutes / 60)} h ago`;
  } else if (minutes >= 1440) {
    value = `${Math.floor(minutes / 1440)} d ago`;
  } else if (minutes >= 1) {
    value = `${minutes} min ago`;
  }
  return locale === "zh" ? `更新 ${localizeRelativeTime(value, "zh")}` : `Updated ${value}`;
}

function sanitizeWorkshopDescription(value: string) {
  return value
    .replace(/\[(\/)?[a-z0-9=:#/.\-_"' ]+\]/gi, "")
    .replace(/https?:\/\/\S+/gi, "")
    .replace(/\s+/g, " ")
    .trim();
}

function workshopPreviewStatusLabel(status: WorkshopPreview["items"][number]["status"], t: (key: MessageKey, values?: Record<string, string | number>) => string) {
  switch (status) {
    case "in_library":
      return t("workshopStatusInLibrary");
    case "in_server":
      return t("workshopStatusInServer");
    case "unavailable":
      return t("workshopStatusUnavailable");
    default:
      return t("workshopStatusNew");
  }
}

function modMatchesSearch(
  item: {
    dependencies?: string[];
    description?: string;
    fileName?: string;
    modName?: string;
    tags?: string[];
    title?: string;
    workshopId?: string;
  },
  term: string
) {
  if (!term) return true;
  return [
    item.title,
    item.modName,
    item.fileName,
    item.workshopId,
    item.description,
    ...(item.tags ?? []),
    ...(item.dependencies ?? [])
  ].some((value) => value && value.toLowerCase().includes(term));
}
