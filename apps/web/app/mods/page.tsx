"use client";

import Link from "next/link";
import { Check, Download, Library, Package, Trash2, Upload } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState, type ReactNode } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Badge, Button, Card, Input } from "@/components/ui";
import { createModPack, deleteGlobalMod, deleteModPack, importGlobalWorkshopMods, listGlobalMods, listModPacks, uploadGlobalMod } from "@/lib/api";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { ModFile, ModPack } from "@/lib/types";

export default function ModsPage() {
  const { locale, t } = useI18n();
  const globalInputRef = useRef<HTMLInputElement>(null);
  const client = useQueryClient();
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [pendingDelete, setPendingDelete] = useState<ModFile | null>(null);
  const [pendingPackDelete, setPendingPackDelete] = useState<ModPack | null>(null);
  const [pendingWorkshopImport, setPendingWorkshopImport] = useState(false);
  const [packName, setPackName] = useState("");
  const [packDescription, setPackDescription] = useState("");
  const [selectedPackModIds, setSelectedPackModIds] = useState<string[]>([]);
  const [workshopIdsText, setWorkshopIdsText] = useState("");
  const globalModsQuery = useQuery({ queryKey: ["global-mods"], queryFn: listGlobalMods, retry: false });
  const modPacksQuery = useQuery({ queryKey: ["mod-packs"], queryFn: listModPacks, retry: false });

  const globalUpload = useMutation({
    mutationFn: (file: File) => uploadGlobalMod(file),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("modUploaded"));
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
    mutationFn: () => importGlobalWorkshopMods(parseWorkshopIds(workshopIdsText)),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("workshopModsImported"));
      setPendingWorkshopImport(false);
      setWorkshopIdsText("");
      await client.invalidateQueries({ queryKey: ["global-mods"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableImportWorkshopMods"));
    }
  });
  const globalMods = globalModsQuery.data ?? [];
  const modPacks = modPacksQuery.data ?? [];
  const selectedPackModCount = selectedPackModIds.length;
  const workshopIds = parseWorkshopIds(workshopIdsText);
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
        onChange={(event) => {
          const file = event.target.files?.[0];
          if (file) globalUpload.mutate(file);
        }}
      />
      {(globalModsQuery.isError || modPacksQuery.isError) && (
        <p className="mb-4 text-sm text-panel-gold">{t("modsApiUnavailable")}</p>
      )}
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      {successMessage && <p className="mb-4 text-sm text-panel-green">{successMessage}</p>}

      <section className="mt-6">
        <PanelHeading icon={<Library aria-hidden="true" />} title={t("modLibrary")} hint={t("modLibraryHint")} count={globalMods.length} />
        <Card className="mt-3 p-4">
          <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(22rem,0.8fr)]">
            <div className="rounded-md border border-panel-line bg-slate-950/45 p-4">
              <h3 className="font-semibold text-white">{t("uploadMod")}</h3>
              <p className="mt-1 text-sm text-slate-500">{t("supportedModFiles")}</p>
              <Button className="mt-4" variant="secondary" onClick={() => globalInputRef.current?.click()} disabled={globalUpload.isPending}>
                <Upload aria-hidden="true" />
                {globalUpload.isPending ? t("uploading") : t("uploadMod")}
              </Button>
            </div>
            <div className="rounded-md border border-panel-line bg-slate-950/45 p-4">
              <h3 className="font-semibold text-white">{t("importWorkshopMods")}</h3>
              <p className="mt-1 text-sm text-slate-500">{t("workshopImportLibraryHint")}</p>
              <textarea
                className="mt-4 min-h-24 w-full resize-none rounded-md border border-panel-line bg-slate-950 px-3 py-2 text-sm text-slate-100 outline-none placeholder:text-slate-600 focus:border-panel-green"
                placeholder={t("workshopIdsPlaceholder")}
                value={workshopIdsText}
                onChange={(event) => setWorkshopIdsText(event.target.value)}
                disabled={workshopImport.isPending}
              />
              <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
                <span className="text-xs text-slate-500">{t("workshopIdsSelected", { count: workshopIds.length })}</span>
                <Button variant="secondary" onClick={() => setPendingWorkshopImport(true)} disabled={workshopImport.isPending || workshopIds.length === 0}>
                  <Download aria-hidden="true" />
                  {workshopImport.isPending ? t("actionWorking") : t("importWorkshopMods")}
                </Button>
              </div>
            </div>
          </div>
        </Card>
        <div className="mt-3 grid gap-3 xl:grid-cols-2">
          {globalMods.map((item) => (
            <Card key={item.id} className="p-4 transition hover:border-panel-green/25">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <ModIdentity item={item} detail={`${item.size} · ${localizeRelativeTime(item.created, locale)}`} />
                <Badge className="shrink-0 bg-slate-800 text-slate-300">.tmod</Badge>
              </div>
              <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-panel-line pt-3">
                <span className="text-xs text-slate-500">{t("modLibrary")}</span>
                <div className="flex shrink-0 flex-wrap gap-2">
                  <Button
                    variant="ghost"
                    onClick={() => togglePackMod(item.id)}
                    className={cn(selectedPackModIds.includes(item.id) && "bg-panel-green/10 text-panel-green")}
                  >
                    {selectedPackModIds.includes(item.id) && <Check aria-hidden="true" />}
                    {t("selectForPack")}
                  </Button>
                  <Button variant="danger" onClick={() => setPendingDelete(item)} disabled={removeGlobal.isPending}>
                    <Trash2 aria-hidden="true" />
                  </Button>
                </div>
              </div>
            </Card>
          ))}
          {!globalModsQuery.isLoading && globalMods.length === 0 && (
            <Card className="flex min-h-40 items-center justify-center border-dashed p-6 text-center text-slate-400 xl:col-span-2">
              <div>
                <Package aria-hidden="true" className="mx-auto" />
                <p className="mt-2 text-sm">{t("noGlobalMods")}</p>
              </div>
            </Card>
          )}
        </div>
      </section>

      <section className="mt-6">
        <PanelHeading icon={<Package aria-hidden="true" />} title={t("modPacks")} hint={t("modPacksHint")} count={modPacks.length} />
        <Card className="mt-3 p-4">
          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] lg:items-end">
            <label className="grid gap-1.5">
              <span className="text-xs font-medium text-slate-500">{t("modPackName")}</span>
              <Input value={packName} onChange={(event) => setPackName(event.target.value)} placeholder={t("modPackName")} />
            </label>
            <label className="grid gap-1.5">
              <span className="text-xs font-medium text-slate-500">{t("modPackDescription")}</span>
              <Input value={packDescription} onChange={(event) => setPackDescription(event.target.value)} placeholder={t("modPacksHint")} />
            </label>
            <Button
              variant="secondary"
              onClick={() => createPack.mutate()}
              disabled={createPack.isPending || packName.trim() === "" || selectedPackModCount === 0}
            >
              <Package aria-hidden="true" />
              {createPack.isPending ? t("actionWorking") : t("createModPack")}
            </Button>
          </div>
          <p className="mt-3 text-xs text-slate-500">{t("selectedForPack", { count: selectedPackModCount })}</p>
        </Card>
        <div className="mt-3 grid gap-3 xl:grid-cols-2">
          {modPacks.map((pack) => (
            <Card key={pack.id} className="p-4 transition hover:border-panel-green/25">
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <Link href={`/mods/packs/${pack.id}`} className="block min-w-0 rounded-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/50 focus-visible:ring-offset-2 focus-visible:ring-offset-panel-card">
                    <h3 className="truncate font-semibold text-white transition hover:text-panel-green">{pack.name}</h3>
                  </Link>
                  <p className="mt-1 truncate text-sm text-slate-500">{pack.description || pack.mods.map((mod) => mod.fileName).join(", ")}</p>
                </div>
                <Badge className="shrink-0 bg-slate-800 text-slate-300">{pack.mods.length}</Badge>
              </div>
              <div className="mt-3 flex flex-wrap gap-2">
                {pack.mods.map((mod) => (
                  <span key={mod.id} className="rounded bg-slate-900 px-2 py-1 text-xs text-slate-300">{mod.fileName}</span>
                ))}
              </div>
              <div className="mt-4 flex justify-end border-t border-panel-line pt-3">
                <Button variant="danger" onClick={() => setPendingPackDelete(pack)} disabled={removePack.isPending}>
                  <Trash2 aria-hidden="true" />
                </Button>
              </div>
            </Card>
          ))}
          {!modPacksQuery.isLoading && modPacks.length === 0 && (
            <Card className="flex min-h-32 items-center justify-center border-dashed p-6 text-center text-slate-400 xl:col-span-2">
              <div>
                <Package aria-hidden="true" className="mx-auto" />
                <p className="mt-2 text-sm">{t("noModPacks")}</p>
              </div>
            </Card>
          )}
        </div>
      </section>

      <ConfirmDialog
        open={pendingWorkshopImport}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmWorkshopImportTitle", { count: workshopIds.length })}
        description={t("confirmWorkshopImportLibraryDescription", { count: workshopIds.length })}
        detail={<DetailLine label={t("workshopIds")} value={String(workshopIds.length)} />}
        cancelLabel={t("cancel")}
        confirmLabel={workshopImport.isPending ? t("actionWorking") : t("importWorkshopMods")}
        confirmVariant="gold"
        busy={workshopImport.isPending}
        onCancel={() => setPendingWorkshopImport(false)}
        onConfirm={() => workshopImport.mutate()}
      />
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        eyebrow={t("destructiveAction")}
        title={t("deleteModConfirm", { name: pendingDelete?.fileName ?? "" })}
        description={t("confirmDeleteModDescription", { name: pendingDelete?.fileName ?? "" })}
        detail={pendingDelete ? (
          <>
            <span className="text-slate-500">{t("modsTitle")}: </span>
            <span className="font-medium text-white">{pendingDelete.fileName}</span>
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

function DetailLine({ label, value }: { label: string; value: string }) {
  return (
    <>
      <span className="text-slate-500">{label}: </span>
      <span className="font-medium text-white">{value}</span>
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

function PanelHeading({ count, hint, icon, title }: { count: number; hint: string; icon: ReactNode; title: string }) {
  return (
    <div className="min-w-0">
      <div className="flex items-center gap-2">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/50 text-slate-400">
          {icon}
        </span>
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h2 className="truncate text-base font-semibold text-white">{title}</h2>
            <Badge className="bg-slate-800 text-slate-300">{count}</Badge>
          </div>
          <p className="mt-1 text-sm text-slate-500">{hint}</p>
        </div>
      </div>
    </div>
  );
}

function ModIdentity({ detail, item }: { detail: string; item: ModFile }) {
  return (
    <div className="flex min-w-0 items-start gap-3">
      <span className="flex size-11 shrink-0 items-center justify-center rounded-lg border border-panel-line bg-slate-950/55 text-slate-400">
        <Package aria-hidden="true" className="size-5" />
      </span>
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-2">
          <Link href={`/mods/${item.id}`} className="min-w-0 rounded-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/50 focus-visible:ring-offset-2 focus-visible:ring-offset-panel-card">
            <h3 className="truncate font-semibold text-white transition hover:text-panel-green">{item.fileName}</h3>
          </Link>
        </div>
        <p className="mt-1 truncate text-sm text-slate-400">{detail}</p>
      </div>
    </div>
  );
}
