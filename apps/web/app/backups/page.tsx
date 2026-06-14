"use client";

import { Archive, Download, MoveRight, RotateCcw, Trash2 } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";
import { createBackup, deleteBackup, downloadBackupFile, listBackups, listServers, migrateBackup, restoreBackup } from "@/lib/api";
import { saveBlob } from "@/lib/download";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import type { Backup } from "@/lib/types";

export default function BackupsPage() {
  const { locale, t } = useI18n();
  const client = useQueryClient();
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, retry: false });
  const [selectedServerId, setSelectedServerId] = useState("");
  const [targetServerId, setTargetServerId] = useState("");
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [pendingRestore, setPendingRestore] = useState<Backup | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Backup | null>(null);
  const [downloadingBackupId, setDownloadingBackupId] = useState("");
  const quickCreateHandledRef = useRef(false);
  const servers = serversQuery.data ?? [];
  const backups = backupsQuery.data ?? [];
  const activeServerId = selectedServerId || servers[0]?.id || "";
  const activeTargetServerId = targetServerId || servers[0]?.id || "";
  const serverNameById = useMemo(() => new Map(servers.map((server) => [server.id, server.name])), [servers]);
  const runningServerIds = useMemo(() => new Set(servers.filter((server) => server.status === "running").map((server) => server.id)), [servers]);

  const create = useMutation({
    mutationFn: createBackup,
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("backupCreated"));
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableCreateBackup"));
    }
  });
  const restore = useMutation({
    mutationFn: restoreBackup,
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("backupRestored"));
      setPendingRestore(null);
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableRestoreBackup"));
    }
  });
  const remove = useMutation({
    mutationFn: deleteBackup,
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("backupDeleted"));
      setPendingDelete(null);
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableDeleteBackup"));
    }
  });
  const migrate = useMutation({
    mutationFn: ({ id, instanceId }: { id: string; instanceId: string }) => migrateBackup(id, instanceId),
    onSuccess: async () => {
      setErrorMessage("");
      setSuccessMessage(t("backupMigrated"));
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (error) => {
      setSuccessMessage("");
      setErrorMessage(error instanceof Error ? error.message : t("unableMigrateBackup"));
    }
  });

  const downloadBackup = async (backup: Backup) => {
    setDownloadingBackupId(backup.id);
    setErrorMessage("");
    setSuccessMessage("");
    try {
      const blob = await downloadBackupFile(backup.id);
      saveBlob(blob, backup.name);
      setSuccessMessage(t("downloadStarted"));
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : t("unableDownloadBackup"));
    } finally {
      setDownloadingBackupId("");
    }
  };

  useEffect(() => {
    if (quickCreateHandledRef.current || typeof window === "undefined" || !activeServerId) return;
    const url = new URL(window.location.href);
    if (url.searchParams.get("action") !== "create") return;
    quickCreateHandledRef.current = true;
    url.searchParams.delete("action");
    window.history.replaceState(null, "", `${url.pathname}${url.search}${url.hash}`);
    create.mutate(activeServerId);
  }, [activeServerId, create]);

  return (
    <>
      <PageHeader
        title={t("backupsTitle")}
        description={t("backupsDescription")}
        action={
          <div className="flex flex-wrap items-center gap-2">
            <select
              className="h-10 rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-100 outline-none focus:border-panel-green"
              value={activeServerId}
              onChange={(event) => setSelectedServerId(event.target.value)}
              disabled={servers.length === 0}
            >
              {servers.length === 0 ? <option>{t("noApiServers")}</option> : servers.map((server) => <option key={server.id} value={server.id}>{server.name}</option>)}
            </select>
            <Button variant="gold" onClick={() => activeServerId && create.mutate(activeServerId)} disabled={!activeServerId || create.isPending}>
              <Archive aria-hidden="true" />
              {create.isPending ? t("backingUp") : t("backupNow")}
            </Button>
          </div>
        }
      />
      {(serversQuery.isError || backupsQuery.isError) && <p className="mb-4 text-sm text-panel-gold">{t("apiBackupsUnavailable")}</p>}
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
      <Card className="overflow-hidden">
        <table className="w-full text-left text-sm">
          <thead className="bg-slate-950/50 text-xs text-slate-400">
            <tr>{[t("backupName"), t("server"), t("world"), t("type"), t("size"), t("created"), t("actions")].map((head) => <th key={head} className="px-4 py-3 font-medium">{head}</th>)}</tr>
          </thead>
          <tbody className="divide-y divide-panel-line">
            {backups.map((backup) => (
              <tr key={backup.id}>
                <td className="px-4 py-4">{backup.name}</td>
                <td className="px-4 py-4 text-slate-300">{backup.instanceId ? serverNameById.get(backup.instanceId) ?? backup.instanceId : backup.server}</td>
                <td className="px-4 py-4 text-slate-300">{backup.world}</td>
                <td className="px-4 py-4 text-slate-300">{backup.type === "Auto" ? t("typeAuto") : t("typeManual")}</td>
                <td className="px-4 py-4 text-slate-300">{backup.size}</td>
                <td className="px-4 py-4 text-slate-300">{localizeRelativeTime(backup.created, locale)}</td>
                <td className="px-4 py-4">
                  <div className="flex gap-2">
                    <Button
                      variant="secondary"
                      aria-label={t("restore")}
                      onClick={() => setPendingRestore(backup)}
                      disabled={restore.isPending || backupsQuery.isError || Boolean(backup.instanceId && runningServerIds.has(backup.instanceId))}
                      title={backup.instanceId && runningServerIds.has(backup.instanceId) ? t("restoreRequiresStopped") : undefined}
                    >
                      <RotateCcw aria-hidden="true" />
                    </Button>
                    <Button
                      variant="secondary"
                      aria-label={t("download")}
                      onClick={() => void downloadBackup(backup)}
                      disabled={backupsQuery.isError || downloadingBackupId === backup.id}
                    >
                      <Download aria-hidden="true" />
                    </Button>
                    <Button
                      variant="secondary"
                      aria-label={t("migrate")}
                      onClick={() => activeTargetServerId && migrate.mutate({ id: backup.id, instanceId: activeTargetServerId })}
                      disabled={!activeTargetServerId || migrate.isPending || backupsQuery.isError || serversQuery.isError}
                    >
                      <MoveRight aria-hidden="true" />
                    </Button>
                    <Button
                      variant="danger"
                      aria-label={t("delete")}
                      onClick={() => setPendingDelete(backup)}
                      disabled={remove.isPending || backupsQuery.isError}
                    >
                      <Trash2 aria-hidden="true" />
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
            {!backupsQuery.isLoading && backups.length === 0 && (
              <tr>
                <td className="px-4 py-8 text-center text-slate-400" colSpan={7}>{t("noBackupsYet")}</td>
              </tr>
            )}
          </tbody>
        </table>
      </Card>
      <ConfirmDialog
        open={Boolean(pendingRestore)}
        eyebrow={t("destructiveAction")}
        title={t("restoreBackupConfirm", { name: pendingRestore?.name ?? "" })}
        description={t("confirmRestoreBackupDescription", { name: pendingRestore?.name ?? "" })}
        detail={pendingRestore ? (
          <>
            <span className="text-slate-500">{t("backupName")}: </span>
            <span className="font-medium text-white">{pendingRestore.name}</span>
          </>
        ) : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={restore.isPending ? t("actionWorking") : t("restore")}
        confirmVariant="gold"
        busy={restore.isPending}
        onCancel={() => setPendingRestore(null)}
        onConfirm={() => pendingRestore && restore.mutate(pendingRestore.id)}
      />
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        eyebrow={t("destructiveAction")}
        title={t("deleteBackupConfirm", { name: pendingDelete?.name ?? "" })}
        description={t("confirmDeleteBackupDescription", { name: pendingDelete?.name ?? "" })}
        detail={pendingDelete ? (
          <>
            <span className="text-slate-500">{t("backupName")}: </span>
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
