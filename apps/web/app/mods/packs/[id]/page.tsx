"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, ArrowRight, Check, Package, Pencil, Trash2, X } from "lucide-react";
import { useMemo, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Button, Card, Input } from "@/components/ui";
import { deleteModPack, listGlobalMods, listModPacks, updateModPack } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { modDisplayName } from "@/lib/mod-display";
import { cn } from "@/lib/utils";
import type { ModFile } from "@/lib/types";

export default function ModPackDetailPage() {
  const { locale, t } = useI18n();
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const client = useQueryClient();
  const id = params.id;
  const packsQuery = useQuery({ queryKey: ["mod-packs"], queryFn: listModPacks, retry: false });
  const globalModsQuery = useQuery({ queryKey: ["global-mods"], queryFn: listGlobalMods, retry: false });
  const [pendingDelete, setPendingDelete] = useState(false);
  const [editing, setEditing] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const [draftName, setDraftName] = useState("");
  const [draftDescription, setDraftDescription] = useState("");
  const [selectedModIds, setSelectedModIds] = useState<string[]>([]);
  const pack = useMemo(() => (packsQuery.data ?? []).find((item) => item.id === id), [id, packsQuery.data]);
  const globalMods = globalModsQuery.data ?? [];
  const selectedMods = useMemo(
    () => globalMods.filter((mod) => selectedModIds.includes(mod.id)),
    [globalMods, selectedModIds]
  );
  const selectedDependencies = useMemo(
    () => dependencyNamesForSelectedMods(globalMods, selectedModIds),
    [globalMods, selectedModIds]
  );
  const availableMods = useMemo(
    () => globalMods.filter((mod) => !selectedModIds.includes(mod.id)),
    [globalMods, selectedModIds]
  );
  const remove = useMutation({
    mutationFn: deleteModPack,
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["mod-packs"] });
      router.push("/mods");
    },
    onError: (error) => setErrorMessage(error instanceof Error ? error.message : t("unableDeleteModPack"))
  });
  const save = useMutation({
    mutationFn: () => updateModPack(id, { name: draftName, description: draftDescription, modIds: selectedModIds }),
    onSuccess: async () => {
      setErrorMessage("");
      setEditing(false);
      await client.invalidateQueries({ queryKey: ["mod-packs"] });
    },
    onError: (error) => setErrorMessage(error instanceof Error ? error.message : t("unableUpdateModPack"))
  });

  const openEdit = () => {
    if (!pack) return;
    setDraftName(pack.name);
    setDraftDescription(pack.description);
    setSelectedModIds(pack.modIds);
    setEditing(true);
  };

  if (packsQuery.isLoading) {
    return <p className="text-sm text-slate-400">{t("loading")}</p>;
  }

  if (packsQuery.isError || !pack) {
    return (
      <>
        <BackLink />
        <Card className="p-6">
          <p className="text-sm text-panel-gold">{packsQuery.isError ? t("modsApiUnavailable") : t("modPackNotFound")}</p>
        </Card>
      </>
    );
  }

  return (
    <>
      <BackLink />
      <PageHeader title={pack.name} description={t("modPackDetailDescription")} />
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      <div className="grid gap-4 xl:grid-cols-[1fr_320px]">
        <div className="space-y-4">
          <Card className="p-4">
            <div className="flex items-start gap-3">
              <span className="flex size-11 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/70 text-panel-green">
                <Package aria-hidden="true" className="size-5" />
              </span>
              <div className="min-w-0">
                <h2 className="truncate text-lg font-semibold text-white">{pack.name}</h2>
                <p className="mt-1 text-sm text-slate-500">{pack.description || t("modPacksHint")}</p>
              </div>
            </div>
            <div className="mt-5 grid gap-3 md:grid-cols-2">
              <DetailTile label={t("modPacks")} value={pack.name} />
              <DetailTile label={t("modsTitle")} value={String(pack.mods.length)} />
              <DetailTile label={t("created")} value={pack.created} />
              <DetailTile label={t("type")} value={t("modPacks")} />
            </div>
          </Card>

          <Card className="p-4">
            <h2 className="font-semibold">{t("modLibrary")}</h2>
            <div className="mt-4 grid gap-2">
              {pack.mods.map((mod) => (
                <Link key={mod.id} href={`/mods/${mod.id}`} className="flex items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/35 px-3 py-3 transition hover:border-panel-green/50 hover:bg-slate-900/60">
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium text-slate-100">{modDisplayName(mod, locale)}</span>
                    <span className="mt-0.5 block truncate text-xs text-slate-500">{mod.size}</span>
                    {mod.dependencies && mod.dependencies.length > 0 ? (
                      <span className="mt-1 block truncate text-xs text-panel-gold">{t("dependencies")}: {mod.dependencies.join(", ")}</span>
                    ) : null}
                  </span>
                  <ArrowRight aria-hidden="true" className="size-4 shrink-0 text-slate-500" />
                </Link>
              ))}
              {pack.mods.length === 0 && <p className="text-sm text-slate-500">{t("noGlobalMods")}</p>}
            </div>
          </Card>
        </div>

        <Card className="h-fit p-4">
          <h2 className="font-semibold">{t("actions")}</h2>
          <Button className="mt-4 w-full" variant="secondary" onClick={openEdit} disabled={save.isPending || globalModsQuery.isError}>
            <Pencil aria-hidden="true" />
            {t("editModPack")}
          </Button>
          <Button className="mt-4 w-full" variant="danger" onClick={() => setPendingDelete(true)} disabled={remove.isPending}>
            <Trash2 aria-hidden="true" />
            {t("delete")}
          </Button>
        </Card>
      </div>

      <ConfirmDialog
        open={pendingDelete}
        eyebrow={t("destructiveAction")}
        title={t("deleteModPackConfirm", { name: pack.name })}
        description={t("confirmDeleteModPackDescription")}
        detail={<DetailLine label={t("modPacks")} value={pack.name} />}
        cancelLabel={t("cancel")}
        confirmLabel={remove.isPending ? t("actionWorking") : t("delete")}
        confirmVariant="danger"
        busy={remove.isPending}
        onCancel={() => setPendingDelete(false)}
        onConfirm={() => remove.mutate(pack.id)}
      />

      {editing && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/75 px-4 py-8">
          <div className="w-full max-w-2xl rounded-lg border border-panel-line bg-panel-card shadow-2xl shadow-black/30">
            <div className="flex items-start justify-between gap-4 border-b border-panel-line p-4">
              <div className="min-w-0">
                <h2 className="text-base font-semibold text-white">{t("editModPack")}</h2>
                <p className="mt-1 text-sm text-slate-500">{t("modPackEditHint")}</p>
              </div>
              <button
                type="button"
                className="flex size-8 shrink-0 items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/50"
                onClick={() => setEditing(false)}
                aria-label="Close"
              >
                <X aria-hidden="true" className="size-4" />
              </button>
            </div>
            <div className="p-4">
              <div className="grid gap-3">
                <label className="grid gap-1.5">
                  <span className="text-xs font-medium text-slate-500">{t("modPackName")}</span>
                  <Input value={draftName} onChange={(event) => setDraftName(event.target.value)} placeholder={t("modPackName")} />
                </label>
                <label className="grid gap-1.5">
                  <span className="text-xs font-medium text-slate-500">{t("modPackDescription")}</span>
                  <Input value={draftDescription} onChange={(event) => setDraftDescription(event.target.value)} placeholder={t("modPackDescription")} />
                </label>
              </div>
              <div className="mt-4 grid gap-4 lg:grid-cols-2">
                <SelectionColumn
                  emptyMessage={t("noModPacks")}
                  items={selectedMods}
                  label={`${t("modPacks")} · ${t("selectedForPack", { count: selectedMods.length })}`}
                  locale={locale}
                  onSelect={(modId) => setSelectedModIds((current) => current.filter((item) => item !== modId))}
                  selected
                />
                <SelectionColumn
                  emptyMessage={t("noGlobalMods")}
                  items={availableMods}
                  label={t("modLibrary")}
                  locale={locale}
                  onSelect={(modId) => setSelectedModIds((current) => [...current, modId])}
                />
              </div>
              {selectedDependencies.length > 0 ? (
                <div className="mt-4 rounded-md border border-panel-gold/25 bg-panel-gold/10 px-3 py-2 text-xs text-panel-gold">
                  {t("packWillIncludeDependencies", { names: selectedDependencies.join(", ") })}
                </div>
              ) : null}
              <div className="mt-4 flex justify-end gap-2">
                <Button variant="ghost" onClick={() => setEditing(false)} disabled={save.isPending}>{t("cancel")}</Button>
                <Button
                  variant="secondary"
                  onClick={() => save.mutate()}
                  disabled={save.isPending || draftName.trim() === "" || selectedModIds.length === 0}
                >
                  <Package aria-hidden="true" />
                  {save.isPending ? t("actionWorking") : t("saveModPack")}
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

function BackLink() {
  const { t } = useI18n();
  return (
    <Link href="/mods" className="mb-4 inline-flex items-center gap-2 text-sm font-medium text-slate-400 transition hover:text-white">
      <ArrowLeft aria-hidden="true" className="size-4" />
      {t("backToMods")}
    </Link>
  );
}

function DetailTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/35 px-3 py-2">
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 truncate text-sm font-medium text-slate-100">{value}</p>
    </div>
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

function SelectionColumn({
  emptyMessage,
  items,
  label,
  locale,
  onSelect,
  selected = false
}: {
  emptyMessage: string;
  items: ModFile[];
  label: string;
  locale: string;
  onSelect: (modId: string) => void;
  selected?: boolean;
}) {
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/45">
      <div className="flex items-center justify-between border-b border-panel-line px-3 py-2">
        <span className="text-sm font-medium text-white">{label}</span>
        <span className="text-xs text-slate-500">{items.length}</span>
      </div>
      <div className="max-h-80 space-y-2 overflow-y-auto p-3">
        {items.map((mod) => (
          <button
            key={mod.id}
            type="button"
            className={cn(
              "flex w-full items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/60 px-3 py-2 text-left transition hover:border-panel-green/35",
              selected && "border-panel-green/60 bg-panel-green/10"
            )}
            onClick={() => onSelect(mod.id)}
          >
            <span className="min-w-0">
              <span className="block truncate text-sm font-medium text-white">{modDisplayName(mod, locale)}</span>
              <span className="mt-0.5 block truncate text-xs text-slate-500">{mod.size}</span>
              {mod.dependencies && mod.dependencies.length > 0 ? (
                <span className="mt-1 block truncate text-xs text-panel-gold">{mod.dependencies.join(", ")}</span>
              ) : null}
            </span>
            {selected ? <X aria-hidden="true" className="size-4 shrink-0 text-slate-400" /> : <Check aria-hidden="true" className="size-4 shrink-0 text-panel-green" />}
          </button>
        ))}
        {items.length === 0 && <p className="px-1 py-4 text-center text-sm text-slate-500">{emptyMessage}</p>}
      </div>
    </div>
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

function modIdentity(mod: ModFile) {
  return mod.modName || mod.title || mod.fileName.replace(/\.[^.]+$/, "");
}
