"use client";

import Image from "next/image";
import Link from "next/link";
import { Check, Clock3, Compass, Download, Library, Package, Trash2, Upload, Users, X } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState, type ReactNode } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Badge, Button, Card, Input } from "@/components/ui";
import { createModPack, deleteGlobalMod, deleteModPack, getDockerStatus, importGlobalWorkshopMods, listGlobalMods, listModPacks, listRecommendedMods, uploadGlobalMod } from "@/lib/api";
import { localizeRelativeTime, useI18n, type MessageKey } from "@/lib/i18n";
import { modDisplayName, modSourceLabel } from "@/lib/mod-display";
import { cn } from "@/lib/utils";
import type { ModFile, ModPack, RecommendedMod } from "@/lib/types";

type ModsView = "discover" | "library" | "packs";
type ModGameFilter = "all" | "terraria";
type DependencyImportPlan = {
  primaryIds: string[];
  dependencyIds: string[];
  dependencyNames: string[];
};

const gameFilters = [
  { key: "all", labelKey: "filterAll" },
  { key: "terraria", labelKey: "gameTerraria" }
] as const satisfies readonly { key: ModGameFilter; labelKey: MessageKey }[];

export default function ModsPage() {
  const { locale, t } = useI18n();
  const globalInputRef = useRef<HTMLInputElement>(null);
  const client = useQueryClient();
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [pendingDelete, setPendingDelete] = useState<ModFile | null>(null);
  const [pendingPackDelete, setPendingPackDelete] = useState<ModPack | null>(null);
  const [activeView, setActiveView] = useState<ModsView>("discover");
  const [gameFilter, setGameFilter] = useState<ModGameFilter>("all");
  const [workshopDialogOpen, setWorkshopDialogOpen] = useState(false);
  const [packDialogOpen, setPackDialogOpen] = useState(false);
  const [packName, setPackName] = useState("");
  const [packDescription, setPackDescription] = useState("");
  const [selectedPackModIds, setSelectedPackModIds] = useState<string[]>([]);
  const [workshopIdsText, setWorkshopIdsText] = useState("");
  const [pendingDependencyImport, setPendingDependencyImport] = useState<DependencyImportPlan | null>(null);
  const globalModsQuery = useQuery({ queryKey: ["global-mods"], queryFn: listGlobalMods, retry: false });
  const modPacksQuery = useQuery({ queryKey: ["mod-packs"], queryFn: listModPacks, retry: false });
  const recommendedModsQuery = useQuery({ queryKey: ["recommended-mods"], queryFn: listRecommendedMods, retry: false });
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
  const workshopImport = useMutation({
    mutationFn: (ids: string[]) => importGlobalWorkshopMods(ids),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("workshopModsImported"));
      setWorkshopDialogOpen(false);
      setPendingDependencyImport(null);
      setWorkshopIdsText("");
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
  const filteredGlobalMods = gameFilter === "all" || gameFilter === "terraria" ? globalMods : [];
  const filteredModPacks = gameFilter === "all" || gameFilter === "terraria" ? modPacks : [];
  const filteredRecommendedMods = gameFilter === "all" || gameFilter === "terraria" ? recommendedMods : [];
  const selectedPackModCount = selectedPackModIds.length;
  const selectedPackDependencies = dependencyNamesForSelectedMods(globalMods, selectedPackModIds);
  const workshopIds = parseWorkshopIds(workshopIdsText);
  const requestWorkshopImport = (ids: string[]) => {
    if (workshopUnsupported) {
      setSuccessMessage("");
      setErrorMessage(t("workshopArmUnsupported"));
      return;
    }
    const plan = buildDependencyImportPlan(ids, recommendedMods, globalMods);
    if (plan.dependencyIds.length > 0) {
      setPendingDependencyImport(plan);
      return;
    }
    workshopImport.mutate(ids);
  };
  const togglePackMod = (modId: string) => {
    setSelectedPackModIds((current) => current.includes(modId) ? current.filter((id) => id !== modId) : [...current, modId]);
  };

  return (
    <>
      <PageHeader
        title={t("modsTitle")}
        description={t("modsDescription")}
      />
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

      <Card className="mb-4 p-3">
        <div className="flex flex-wrap gap-3">
          <FilterGroup label={t("filterGame")} options={gameFilters} value={gameFilter} onChange={setGameFilter} t={t} />
        </div>
      </Card>

      <div className="mt-6 flex flex-wrap gap-2 border-b border-panel-line pb-3">
        <ViewTab
          active={activeView === "discover"}
          count={filteredRecommendedMods.length}
          icon={<Compass aria-hidden="true" />}
          label={t("discoverMods")}
          onClick={() => setActiveView("discover")}
        />
        <ViewTab
          active={activeView === "library"}
          count={filteredGlobalMods.length}
          icon={<Library aria-hidden="true" />}
          label={t("modLibrary")}
          onClick={() => setActiveView("library")}
        />
        <ViewTab
          active={activeView === "packs"}
          count={filteredModPacks.length}
          icon={<Package aria-hidden="true" />}
          label={t("modPacks")}
          onClick={() => setActiveView("packs")}
        />
      </div>

      {activeView === "discover" ? (
        <section className="mt-5">
          <SectionToolbar
            title={t("discoverMods")}
            hint={t("discoverModsHint")}
            count={filteredRecommendedMods.length}
            actions={(
              <Button variant="secondary" onClick={() => setWorkshopDialogOpen(true)} disabled={workshopImport.isPending || workshopUnsupported} title={workshopUnsupported ? t("workshopArmUnsupported") : undefined}>
                <Download aria-hidden="true" />
                {t("importWorkshopMods")}
              </Button>
            )}
          />
          <div className="mt-4 grid gap-3 2xl:grid-cols-2">
            {filteredRecommendedMods.map((item) => (
              <RecommendedModCard
                key={item.workshopId}
                item={item}
                locale={locale}
                busy={workshopImport.isPending || workshopUnsupported}
                disabledReason={workshopUnsupported ? t("workshopArmUnsupported") : ""}
                onAdd={() => {
                  requestWorkshopImport([item.workshopId]);
                }}
              />
            ))}
          </div>
        </section>
      ) : activeView === "library" ? (
        <section className="mt-5">
          <SectionToolbar
            title={t("modLibrary")}
            hint={t("modLibraryHint")}
            count={filteredGlobalMods.length}
            actions={(
              <>
                <Button variant="secondary" onClick={() => globalInputRef.current?.click()} disabled={globalUpload.isPending}>
                  <Upload aria-hidden="true" />
                  {globalUpload.isPending ? t("uploading") : t("uploadMod")}
                </Button>
              <Button variant="secondary" onClick={() => setWorkshopDialogOpen(true)} disabled={workshopImport.isPending || workshopUnsupported} title={workshopUnsupported ? t("workshopArmUnsupported") : undefined}>
                <Download aria-hidden="true" />
                {t("importWorkshopMods")}
              </Button>
              </>
            )}
          />
          <div className="mt-4 grid gap-3 xl:grid-cols-2">
            {filteredGlobalMods.map((item) => (
              <Card key={item.id} className="p-4 transition hover:border-panel-green/25">
                <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                  <ModIdentity item={item} detail={`${item.size} · ${localizeRelativeTime(item.created, locale)}`} locale={locale} />
                  <Badge className="shrink-0 bg-slate-800 text-slate-300">{modSourceLabel(item, locale)}</Badge>
                </div>
                <ModMetadataStrip item={item} />
                <div className="mt-4 flex justify-end border-t border-panel-line pt-3">
                  <Button variant="danger" onClick={() => setPendingDelete(item)} disabled={removeGlobal.isPending}>
                    <Trash2 aria-hidden="true" />
                  </Button>
                </div>
              </Card>
            ))}
            {!globalModsQuery.isLoading && filteredGlobalMods.length === 0 && (
              <Card className="flex min-h-44 items-center justify-center border-dashed p-6 text-center text-slate-400 xl:col-span-2">
                <div>
                  <Package aria-hidden="true" className="mx-auto" />
                  <p className="mt-2 text-sm">{t("noGlobalMods")}</p>
                </div>
              </Card>
            )}
          </div>
        </section>
      ) : (
        <section className="mt-5">
          <SectionToolbar
            title={t("modPacks")}
            hint={t("modPacksHint")}
            count={filteredModPacks.length}
            actions={(
              <Button variant="secondary" onClick={() => setPackDialogOpen(true)}>
                <Package aria-hidden="true" />
                {t("createModPack")}
              </Button>
            )}
          />
          <div className="mt-4 grid gap-3 xl:grid-cols-2">
            {filteredModPacks.map((pack) => (
              <Card key={pack.id} className="p-4 transition hover:border-panel-green/25">
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0">
                    <Link href={`/mods/packs/${pack.id}`} className="block min-w-0 rounded-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/50 focus-visible:ring-offset-2 focus-visible:ring-offset-panel-card">
                      <h3 className="truncate font-semibold text-white transition hover:text-panel-green">{pack.name}</h3>
                    </Link>
                    <p className="mt-1 truncate text-sm text-slate-500">{pack.description || pack.mods.map((mod) => modDisplayName(mod, locale)).join(", ")}</p>
                  </div>
                  <Badge className="shrink-0 bg-slate-800 text-slate-300">{pack.mods.length}</Badge>
                </div>
                <div className="mt-3 flex flex-wrap gap-2">
                  {pack.mods.slice(0, 6).map((mod) => (
                    <span key={mod.id} className="rounded bg-slate-900 px-2 py-1 text-xs text-slate-300">{modDisplayName(mod, locale)}</span>
                  ))}
                  {pack.mods.length > 6 && <span className="rounded bg-slate-900 px-2 py-1 text-xs text-slate-500">+{pack.mods.length - 6}</span>}
                </div>
                <div className="mt-4 flex justify-end border-t border-panel-line pt-3">
                  <Button variant="danger" onClick={() => setPendingPackDelete(pack)} disabled={removePack.isPending}>
                    <Trash2 aria-hidden="true" />
                  </Button>
                </div>
              </Card>
            ))}
            {!modPacksQuery.isLoading && filteredModPacks.length === 0 && (
              <Card className="flex min-h-44 items-center justify-center border-dashed p-6 text-center text-slate-400 xl:col-span-2">
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
          title={t("importWorkshopMods")}
          description={t("workshopImportLibraryHint")}
          onClose={() => setWorkshopDialogOpen(false)}
        >
          <textarea
            className="min-h-32 w-full resize-none rounded-md border border-panel-line bg-slate-950 px-3 py-2 text-sm text-slate-100 outline-none placeholder:text-slate-600 focus:border-panel-green"
            placeholder={t("workshopIdsPlaceholder")}
            value={workshopIdsText}
            onChange={(event) => setWorkshopIdsText(event.target.value)}
            disabled={workshopImport.isPending}
          />
          <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
            <span className="text-xs text-slate-500">{t("workshopIdsSelected", { count: workshopIds.length })}</span>
            <div className="flex gap-2">
              <Button variant="ghost" onClick={() => setWorkshopDialogOpen(false)} disabled={workshopImport.isPending}>{t("cancel")}</Button>
              <Button variant="secondary" onClick={() => requestWorkshopImport(workshopIds)} disabled={workshopImport.isPending || workshopIds.length === 0 || workshopUnsupported} title={workshopUnsupported ? t("workshopArmUnsupported") : undefined}>
                <Download aria-hidden="true" />
                {workshopImport.isPending ? t("actionWorking") : t("importWorkshopMods")}
              </Button>
            </div>
          </div>
        </DialogShell>
      )}

      {pendingDependencyImport && (
        <DialogShell
          title={t("importDependenciesTitle")}
          description={t("importDependenciesDescription", { names: pendingDependencyImport.dependencyNames.join(", ") })}
          onClose={() => setPendingDependencyImport(null)}
        >
          <div className="rounded-md border border-panel-line bg-slate-950/45 px-3 py-3">
            <p className="text-xs text-slate-500">{t("dependencies")}</p>
            <p className="mt-1 text-sm font-medium text-slate-100">{pendingDependencyImport.dependencyNames.join(", ")}</p>
          </div>
          <div className="mt-4 flex flex-wrap justify-end gap-2">
            <Button variant="ghost" onClick={() => setPendingDependencyImport(null)} disabled={workshopImport.isPending}>{t("cancel")}</Button>
            <Button variant="secondary" onClick={() => workshopImport.mutate(pendingDependencyImport.primaryIds)} disabled={workshopImport.isPending}>
              {t("importOnlySelected")}
            </Button>
            <Button onClick={() => workshopImport.mutate([...pendingDependencyImport.primaryIds, ...pendingDependencyImport.dependencyIds])} disabled={workshopImport.isPending}>
              <Download aria-hidden="true" />
              {workshopImport.isPending ? t("actionWorking") : t("importWithDependencies")}
            </Button>
          </div>
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
    </>
  );
}

function parseWorkshopIds(value: string) {
  const seen = new Set<string>();
  return value
    .split(/[\s,，]+/)
    .map((item) => item.trim())
    .filter((item) => {
      if (!item || seen.has(item)) return false;
      seen.add(item);
      return true;
    });
}

function ViewTab({ active, count, icon, label, onClick }: { active: boolean; count: number; icon: ReactNode; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      className={cn(
        "inline-flex items-center gap-2 rounded-md border px-3 py-2 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-panel-green/50",
        active ? "border-panel-green/50 bg-panel-green/15 text-panel-green" : "border-panel-line bg-slate-950/40 text-slate-300 hover:bg-slate-900"
      )}
      onClick={onClick}
    >
      {icon}
      {label}
      <Badge className={cn(active ? "bg-panel-green/15 text-panel-green" : "bg-slate-800 text-slate-400")}>{count}</Badge>
    </button>
  );
}

function SectionToolbar({ actions, count, hint, title }: { actions: ReactNode; count: number; hint: string; title: string }) {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <h2 className="truncate text-base font-semibold text-white">{title}</h2>
          <Badge className="bg-slate-800 text-slate-300">{count}</Badge>
        </div>
        <p className="mt-1 max-w-2xl text-sm text-slate-500">{hint}</p>
      </div>
      <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>
    </div>
  );
}

function DialogShell({ children, description, onClose, title }: { children: ReactNode; description: string; onClose: () => void; title: string }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/75 px-4 py-8">
      <div className="w-full max-w-2xl rounded-lg border border-panel-line bg-panel-card shadow-2xl shadow-black/30">
        <div className="flex items-start justify-between gap-4 border-b border-panel-line p-4">
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-white">{title}</h2>
            <p className="mt-1 text-sm text-slate-500">{description}</p>
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

function ModIdentity({ detail, item, locale }: { detail: string; item: ModFile; locale: string }) {
  return (
    <div className="flex min-w-0 items-start gap-3">
      <span className="flex size-11 shrink-0 items-center justify-center rounded-lg border border-panel-line bg-slate-950/55 text-slate-400">
        <Package aria-hidden="true" className="size-5" />
      </span>
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-2">
          <Link href={`/mods/${item.id}`} className="min-w-0 rounded-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/50 focus-visible:ring-offset-2 focus-visible:ring-offset-panel-card">
            <h3 className="truncate font-semibold text-white transition hover:text-panel-green">{modDisplayName(item, locale)}</h3>
          </Link>
        </div>
        <p className="mt-1 truncate text-sm text-slate-400">{detail}</p>
      </div>
    </div>
  );
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
    <div className="mt-4 grid gap-2 sm:grid-cols-2">
      {entries.map(([label, value]) => (
        <div key={label} className="min-w-0 rounded-md border border-panel-line bg-slate-950/40 px-3 py-2">
          <p className="text-xs text-slate-500">{label}</p>
          <p className="mt-1 truncate text-sm font-medium text-slate-100" title={value}>{value}</p>
        </div>
      ))}
    </div>
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
                <p className="mt-1 text-xs text-slate-500">{locale === "zh" ? "创意工坊" : "Workshop"} {item.workshopId}</p>
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
        <a
          href={`https://steamcommunity.com/sharedfiles/filedetails/?id=${item.workshopId}`}
          target="_blank"
          rel="noreferrer"
          className="truncate text-slate-400 transition hover:text-panel-green"
        >
          {locale === "zh" ? "打开 Steam 工坊" : "Open Steam Workshop"}
        </a>
        {item.inLibrary ? (
          <Badge className="bg-panel-green/15 text-panel-green">{locale === "zh" ? "已在模组库" : "In library"}</Badge>
        ) : (
          <Button variant="secondary" onClick={onAdd} disabled={busy} title={disabledReason || undefined}>
            <Download aria-hidden="true" />
            {locale === "zh" ? "加入模组库" : "Add to library"}
          </Button>
        )}
      </div>
    </Card>
  );
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

function buildDependencyImportPlan(ids: string[], recommendedMods: RecommendedMod[], globalMods: ModFile[]): DependencyImportPlan {
  const primaryIds = Array.from(new Set(ids));
  const primarySet = new Set(primaryIds);
  const recommendedByWorkshopID = new Map(recommendedMods.map((mod) => [mod.workshopId, mod]));
  const recommendedByModName = new Map(recommendedMods.flatMap((mod) => mod.modName ? [[mod.modName, mod] as const] : []));
  const libraryNames = new Set(globalMods.map(modIdentity));
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
      if (!dependency || primarySet.has(dependency.workshopId)) continue;
      seenDependencyNames.add(dependencyName);
      dependencyIds.push(dependency.workshopId);
      dependencyNames.push(dependencyName);
      queue.push(dependency.workshopId);
    }
  }
  return { primaryIds, dependencyIds, dependencyNames };
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

function FilterGroup<T extends string>({
  label,
  onChange,
  options,
  t,
  value
}: {
  label: string;
  onChange: (value: T) => void;
  options: readonly { key: T; labelKey: MessageKey }[];
  t: (key: MessageKey) => string;
  value: T;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-xs font-medium text-slate-500">{label}</span>
      <div className="flex rounded-md border border-panel-line bg-slate-950/50 p-0.5">
        {options.map((item) => (
          <Button
            key={item.key}
            type="button"
            variant="ghost"
            className={cn("h-8 px-2.5 py-1 text-xs hover:bg-slate-800", value === item.key && "bg-panel-green/10 text-panel-green hover:bg-panel-green/15")}
            onClick={() => onChange(item.key)}
          >
            {t(item.labelKey)}
          </Button>
        ))}
      </div>
    </div>
  );
}
