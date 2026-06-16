"use client";

import Link from "next/link";
import { ArrowRight, Download, Search, Trash2 } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Button, Card, Input } from "@/components/ui";
import { deleteBackup, downloadBackupFile, listBackups, listServers } from "@/lib/api";
import { saveBlob } from "@/lib/download";
import { localizeRelativeTime, useI18n, type MessageKey } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { Backup, Server } from "@/lib/types";

type BackupServerFilter = "all" | string;
type BackupTypeFilter = "all" | Backup["type"];
type BackupServerTypeFilter = "all" | Server["mode"];

const backupTypeFilters = [
  { key: "all", labelKey: "filterAll" },
  { key: "Manual", labelKey: "typeManual" },
  { key: "Auto", labelKey: "typeAuto" }
] as const satisfies readonly { key: BackupTypeFilter; labelKey: MessageKey }[];

const backupServerTypeFilters = [
  { key: "all", labelKey: "filterAll" },
  { key: "vanilla", labelKey: "filterVanilla" },
  { key: "tmodloader", labelKey: "filterModded" }
] as const satisfies readonly { key: BackupServerTypeFilter; labelKey: MessageKey }[];

export default function BackupsPage() {
  const { locale, t } = useI18n();
  const client = useQueryClient();
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, retry: false });
  const [search, setSearch] = useState("");
  const [serverFilter, setServerFilter] = useState<BackupServerFilter>("all");
  const [serverTypeFilter, setServerTypeFilter] = useState<BackupServerTypeFilter>("all");
  const [backupTypeFilter, setBackupTypeFilter] = useState<BackupTypeFilter>("all");
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [pendingDelete, setPendingDelete] = useState<Backup | null>(null);
  const [downloadingBackupId, setDownloadingBackupId] = useState("");
  const servers = serversQuery.data ?? [];
  const backups = backupsQuery.data ?? [];
  const serverNameById = useMemo(() => new Map(servers.map((server) => [server.id, server.name])), [servers]);
  const serverById = useMemo(() => new Map(servers.map((server) => [server.id, server])), [servers]);
  const filteredBackups = useMemo(() => {
    const term = search.trim().toLowerCase();
    return backups.filter((backup) => {
      const server = backup.instanceId ? serverById.get(backup.instanceId) : undefined;
      const serverName = backup.instanceId ? serverNameById.get(backup.instanceId) ?? backup.instanceId : backup.server;
      const matchesSearch = !term || [backup.name, backup.world, backup.type, serverName].some((value) => value.toLowerCase().includes(term));
      const matchesServer = serverFilter === "all" || backup.instanceId === serverFilter;
      const matchesServerType = serverTypeFilter === "all" || server?.mode === serverTypeFilter;
      const matchesBackupType = backupTypeFilter === "all" || backup.type === backupTypeFilter;
      return matchesSearch && matchesServer && matchesServerType && matchesBackupType;
    });
  }, [backupTypeFilter, backups, search, serverById, serverFilter, serverNameById, serverTypeFilter]);

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

  return (
    <>
      <PageHeader
        title={t("backupsTitle")}
        description={t("backupsDescription")}
      />
      {(serversQuery.isError || backupsQuery.isError) && <p className="mb-4 text-sm text-panel-gold">{t("apiBackupsUnavailable")}</p>}
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      {successMessage && <p className="mb-4 text-sm text-panel-green">{successMessage}</p>}
      <Card className="mb-4 p-3">
        <div className="flex flex-col gap-3 2xl:flex-row 2xl:items-center 2xl:justify-between">
          <div className="relative min-w-0 2xl:max-w-sm 2xl:flex-1">
            <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-2.5 size-4 text-slate-500" />
            <Input className="pl-9" placeholder={t("searchBackups")} value={search} onChange={(event) => setSearch(event.target.value)} />
          </div>
          <div className="flex flex-wrap gap-3">
            <label className="flex items-center gap-2">
              <span className="text-xs font-medium text-slate-500">{t("server")}</span>
              <select
                className="h-9 rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-100 outline-none focus:border-panel-green"
                value={serverFilter}
                onChange={(event) => setServerFilter(event.target.value)}
              >
                <option value="all">{t("filterAll")}</option>
                {servers.map((server) => <option key={server.id} value={server.id}>{server.name}</option>)}
              </select>
            </label>
            <FilterGroup label={t("serverType")} options={backupServerTypeFilters} value={serverTypeFilter} onChange={setServerTypeFilter} t={t} />
            <FilterGroup label={t("backupType")} options={backupTypeFilters} value={backupTypeFilter} onChange={setBackupTypeFilter} t={t} />
          </div>
        </div>
        <p className="mt-3 text-xs text-slate-500">{t("backupFilterSummary", { shown: filteredBackups.length, total: backups.length })}</p>
      </Card>
      <div className="grid gap-3">
        {filteredBackups.map((backup) => {
          const serverName = backup.instanceId ? serverNameById.get(backup.instanceId) ?? backup.instanceId : backup.server;
          return (
            <Card key={backup.id} className="p-4 transition hover:border-panel-green/30">
              <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h2 className="truncate font-semibold text-white">{backup.name}</h2>
                    <span className={cn("rounded px-2 py-0.5 text-xs font-medium", backup.type === "Auto" ? "bg-slate-800 text-slate-300" : "bg-panel-gold/15 text-panel-gold")}>
                      {backup.type === "Auto" ? t("typeAuto") : t("typeManual")}
                    </span>
                  </div>
                  <div className="mt-3 grid gap-2 text-sm text-slate-300 sm:grid-cols-2 xl:grid-cols-4">
                    <BackupMeta label={t("server")} value={serverName} />
                    <BackupMeta label={t("world")} value={backup.world} />
                    <BackupMeta label={t("size")} value={backup.size} />
                    <BackupMeta label={t("created")} value={localizeRelativeTime(backup.created, locale)} />
                  </div>
                </div>
                <div className="flex shrink-0 flex-wrap gap-2">
                  {backup.instanceId && (
                    <Link
                      href={`/servers/${backup.instanceId}`}
                      className="inline-flex h-10 items-center justify-center gap-2 rounded-md border border-panel-line bg-slate-900/70 px-3 text-sm font-medium text-slate-100 transition hover:bg-slate-800 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
                    >
                      {t("manageOnServer")}
                      <ArrowRight aria-hidden="true" className="size-4" />
                    </Link>
                  )}
                  <Button
                    variant="secondary"
                    aria-label={t("download")}
                    onClick={() => void downloadBackup(backup)}
                    disabled={backupsQuery.isError || downloadingBackupId === backup.id}
                  >
                    <Download aria-hidden="true" />
                    {downloadingBackupId === backup.id ? t("downloading") : t("download")}
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
              </div>
            </Card>
          );
        })}
      </div>
      {!backupsQuery.isLoading && filteredBackups.length === 0 && <p className="mt-4 text-sm text-slate-400">{backups.length === 0 ? t("noBackupsYet") : t("noBackupsMatch")}</p>}
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

function BackupMeta({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 truncate font-medium text-slate-200">{value}</p>
    </div>
  );
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
