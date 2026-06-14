"use client";

import { Download, MoveRight, Plus, Trash2, Upload } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";
import { deleteWorld, downloadWorldFile, duplicateWorld, importWorld, listServers, listWorlds, migrateWorld } from "@/lib/api";
import { saveBlob } from "@/lib/download";
import { localizeDifficulty, localizeRelativeTime, localizeWorldSize, useI18n } from "@/lib/i18n";
import type { World } from "@/lib/types";

export default function WorldsPage() {
  const { locale, t } = useI18n();
  const inputRef = useRef<HTMLInputElement>(null);
  const client = useQueryClient();
  const query = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, retry: false });
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const [targetServerId, setTargetServerId] = useState("");
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [pendingDelete, setPendingDelete] = useState<World | null>(null);
  const [downloadingWorldId, setDownloadingWorldId] = useState("");
  const worlds = query.data ?? [];
  const servers = serversQuery.data ?? [];
  const activeTargetServerId = targetServerId || servers[0]?.id || "";

  const upload = useMutation({
    mutationFn: (file: File) => importWorld(file),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("worldImported"));
      await client.invalidateQueries({ queryKey: ["worlds"] });
      if (inputRef.current) inputRef.current.value = "";
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableImportWorld"));
    }
  });
  const duplicate = useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => duplicateWorld(id, name),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("worldDuplicated"));
      await client.invalidateQueries({ queryKey: ["worlds"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableDuplicateWorld"));
    }
  });
  const remove = useMutation({
    mutationFn: deleteWorld,
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("worldDeleted"));
      setPendingDelete(null);
      await client.invalidateQueries({ queryKey: ["worlds"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      const message = error instanceof Error ? error.message : "";
      setErrorMessage(message.includes("active world") ? t("unableDeleteActiveWorld") : message || t("unableDeleteWorld"));
    }
  });
  const migrate = useMutation({
    mutationFn: ({ id, instanceId }: { id: string; instanceId: string }) => migrateWorld(id, instanceId),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("worldMigrated"));
      await client.invalidateQueries({ queryKey: ["worlds"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableMigrateWorld"));
    }
  });

  const downloadWorld = async (world: World) => {
    setDownloadingWorldId(world.id);
    setErrorMessage("");
    setSuccessMessage("");
    try {
      const blob = await downloadWorldFile(world.id);
      saveBlob(blob, `${world.name}.wld`);
      setSuccessMessage(t("downloadStarted"));
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : t("unableDownloadWorld"));
    } finally {
      setDownloadingWorldId("");
    }
  };

  return (
    <>
      <PageHeader
        title={t("worldsTitle")}
        description={t("worldsDescription")}
        action={
          <>
            <input
              ref={inputRef}
              className="hidden"
              type="file"
              accept=".wld"
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) upload.mutate(file);
              }}
            />
            <Button variant="secondary" onClick={() => inputRef.current?.click()} disabled={upload.isPending}>
              <Upload aria-hidden="true" />
              {upload.isPending ? t("importing") : t("importWorld")}
            </Button>
          </>
        }
      />
      {query.isError && <p className="mb-4 text-sm text-panel-gold">{t("apiWorldsUnavailable")}</p>}
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      {successMessage && <p className="mb-4 text-sm text-panel-green">{successMessage}</p>}
      <div className="mb-4 flex flex-wrap items-center gap-2 text-sm text-slate-400">
        <span>{t("migrationTarget")}</span>
        <select
          className="h-10 rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-100 outline-none focus:border-panel-green"
          value={activeTargetServerId}
          onChange={(event) => setTargetServerId(event.target.value)}
          disabled={servers.length === 0}
        >
          {servers.length === 0 ? <option>{t("noApiServers")}</option> : servers.map((server) => <option key={server.id} value={server.id}>{server.name}</option>)}
        </select>
      </div>
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {worlds.map((world) => (
          <Card key={world.id} className="p-4">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <h2 className="truncate font-semibold">{world.name}</h2>
                <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-300">
                  <span className="rounded bg-slate-800 px-2 py-1">{localizeWorldSize(world.size, locale)}</span>
                  <span className="rounded bg-slate-800 px-2 py-1">{localizeDifficulty(world.difficulty, locale)}</span>
                </div>
              </div>
              {world.server && <span className="shrink-0 rounded bg-panel-green/15 px-2 py-1 text-xs text-panel-green">{t("inUse")}</span>}
            </div>
            <p className="mt-4 text-sm text-slate-400">{t("modified")}: {localizeRelativeTime(world.modified, locale)}</p>
            <p className="text-sm text-slate-400">{t("usedBy")}: {world.server || t("notInUse")}</p>
            <p className="text-sm text-slate-400">{t("size")}: {world.bytes}</p>
            <div className="mt-4 flex flex-wrap gap-2">
              <Button
                variant="secondary"
                onClick={() => duplicate.mutate({ id: world.id, name: `${world.name} ${t("duplicateSuffix")}` })}
                disabled={duplicate.isPending || query.isError}
              >
                <Plus aria-hidden="true" />
                {t("duplicate")}
              </Button>
              <Button variant="secondary" onClick={() => void downloadWorld(world)} disabled={query.isError || downloadingWorldId === world.id}>
                <Download aria-hidden="true" />
                {downloadingWorldId === world.id ? t("downloading") : t("download")}
              </Button>
              <Button
                variant="secondary"
                onClick={() => activeTargetServerId && migrate.mutate({ id: world.id, instanceId: activeTargetServerId })}
                disabled={!activeTargetServerId || migrate.isPending || query.isError || serversQuery.isError}
              >
                <MoveRight aria-hidden="true" />
                {t("migrate")}
              </Button>
              <Button
                variant="danger"
                onClick={() => setPendingDelete(world)}
                disabled={remove.isPending || query.isError}
              >
                <Trash2 aria-hidden="true" />
                {t("delete")}
              </Button>
            </div>
          </Card>
        ))}
        <Card className="flex min-h-52 items-center justify-center border-dashed p-4 text-slate-400">
          <button className="text-center" type="button" onClick={() => inputRef.current?.click()}>
            <Plus aria-hidden="true" className="mx-auto" />
            <p className="mt-2 text-sm">{t("importNewWorld")}</p>
          </button>
        </Card>
      </div>
      {!query.isLoading && worlds.length === 0 && <p className="mt-4 text-sm text-slate-400">{t("noWorldsYet")}</p>}
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        eyebrow={t("destructiveAction")}
        title={t("deleteWorldConfirm", { name: pendingDelete?.name ?? "" })}
        description={t("confirmDeleteWorldDescription", { name: pendingDelete?.name ?? "" })}
        detail={pendingDelete ? (
          <>
            <span className="text-slate-500">{t("world")}: </span>
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
