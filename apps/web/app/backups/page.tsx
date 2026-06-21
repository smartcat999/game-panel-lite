"use client";

import Link from "next/link";
import { Clock, Download, Trash2 } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { ResourceFilterBar } from "@/components/resource-filter-bar";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";
import { deleteBackup, downloadBackupFile, listBackups, listGameServers, listGames } from "@/lib/api";
import { saveBlob } from "@/lib/download";
import { showWorldAndBackupFeatures } from "@/lib/feature-flags";
import { gameFilterOptions } from "@/lib/game-filters";
import { localizeRelativeTime, useI18n, type MessageKey } from "@/lib/i18n";
import { providerFilterOptions } from "@/lib/provider-filters";
import { cn } from "@/lib/utils";
import type { Backup } from "@/lib/types";

type BackupTypeFilter = "all" | Backup["type"];
type BackupGameFilter = "all" | string;
type BackupProviderFilter = "all" | string;

const backupTypeFilters = [
  { key: "all", labelKey: "filterAll" },
  { key: "Manual", labelKey: "typeManual" },
  { key: "Auto", labelKey: "typeAuto" }
] as const satisfies readonly { key: BackupTypeFilter; labelKey: MessageKey }[];

export default function BackupsPage() {
  if (!showWorldAndBackupFeatures) return <HiddenFeaturePage />;
  return <EnabledBackupsPage />;
}

function EnabledBackupsPage() {
  const { locale, t } = useI18n();
  const client = useQueryClient();
  const serversQuery = useQuery({ queryKey: ["game-servers"], queryFn: listGameServers, retry: false });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, retry: false });
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, retry: false, staleTime: 5 * 60 * 1000 });
  const [search, setSearch] = useState("");
  const [gameFilter, setGameFilter] = useState<BackupGameFilter>("all");
  const [providerFilter, setProviderFilter] = useState<BackupProviderFilter>("all");
  const [backupTypeFilter, setBackupTypeFilter] = useState<BackupTypeFilter>("all");
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [pendingDelete, setPendingDelete] = useState<Backup | null>(null);
  const [downloadingBackupId, setDownloadingBackupId] = useState("");
  const servers = serversQuery.data ?? [];
  const backups = backupsQuery.data ?? [];
  const serverNameById = useMemo(() => new Map(servers.map((server) => [server.id, server.name])), [servers]);
  const serverById = useMemo(() => new Map(servers.map((server) => [server.id, server])), [servers]);
  const backupGameFilters = useMemo(
    () => gameFilterOptions(gamesQuery.data ?? [], t("filterAll"), backups.map((backup) => backup.gameKey ?? serverById.get(backup.instanceId ?? "")?.gameKey), t),
    [backups, gamesQuery.data, serverById, t]
  );
  const providerFilters = useMemo(
    () => providerFilterOptions(gamesQuery.data ?? [], t("filterAll"), servers.map((server) => server.providerKey), gameFilter),
    [gameFilter, gamesQuery.data, servers, t]
  );
  useEffect(() => {
    if (providerFilter !== "all" && !providerFilters.some((option) => option.key === providerFilter)) {
      setProviderFilter("all");
    }
  }, [providerFilter, providerFilters]);
  const filteredBackups = useMemo(() => {
    const term = search.trim().toLowerCase();
    return backups.filter((backup) => {
      const server = backup.instanceId ? serverById.get(backup.instanceId) : undefined;
      const serverName = backup.instanceId ? serverNameById.get(backup.instanceId) ?? backup.instanceId : backup.server;
      const matchesSearch = !term || [backup.name, backup.world, backup.type, serverName].some((value) => value.toLowerCase().includes(term));
      const backupGame = backup.gameKey ?? server?.gameKey;
      const matchesGame = gameFilter === "all" || backupGame === gameFilter;
      const matchesProvider = providerFilter === "all" || server?.providerKey === providerFilter;
      const matchesBackupType = backupTypeFilter === "all" || backup.type === backupTypeFilter;
      return matchesSearch && matchesGame && matchesProvider && matchesBackupType;
    }).sort(sortBackupsNewestFirst);
  }, [backupTypeFilter, backups, gameFilter, providerFilter, search, serverById, serverNameById]);
  const activeFilterChips = [
    search.trim(),
    gameFilter !== "all" ? filterOptionLabel(backupGameFilters, gameFilter, t) : "",
    providerFilter !== "all" ? filterOptionLabel(providerFilters, providerFilter, t) : "",
    backupTypeFilter !== "all" ? filterOptionLabel(backupTypeFilters, backupTypeFilter, t) : ""
  ].filter(Boolean);

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
      <PageHeader title={t("backupsTitle")} />
      {(serversQuery.isError || backupsQuery.isError) && <p className="mb-4 text-sm text-panel-gold">{t("apiBackupsUnavailable")}</p>}
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      {successMessage && <p className="mb-4 text-sm text-panel-green">{successMessage}</p>}
      <ResourceFilterBar
        activeChips={activeFilterChips}
        clearLabel={t("clearFilters")}
        density="compact"
        filters={[
          { label: t("filterGame"), options: backupGameFilters, value: gameFilter, onChange: (value) => setGameFilter(value) },
          { label: t("serverType"), options: providerFilters, value: providerFilter, onChange: (value) => setProviderFilter(value) },
          { label: t("backupType"), options: backupTypeFilters, value: backupTypeFilter, onChange: (value) => setBackupTypeFilter(value as BackupTypeFilter) }
        ]}
        onClear={() => {
          setGameFilter("all");
          setProviderFilter("all");
          setBackupTypeFilter("all");
          setSearch("");
        }}
        onSearchChange={setSearch}
        resultLabel={t("backupFilterSummary", { shown: filteredBackups.length, total: backups.length })}
        search={search}
        searchPlaceholder={t("searchBackups")}
      />
      {filteredBackups.length > 0 ? (
        <Card className="overflow-hidden">
          <div className="divide-y divide-panel-line">
            {filteredBackups.map((backup) => {
              const serverName = backup.instanceId ? serverNameById.get(backup.instanceId) ?? backup.instanceId : backup.server;
              return (
                <div key={backup.id} className="grid gap-3 px-4 py-3 transition hover:bg-slate-900/40 xl:grid-cols-[minmax(0,1fr)_10rem_auto] xl:items-center">
                  <div className="min-w-0">
                    <div className="flex min-w-0 flex-wrap items-center gap-2">
                      <Link href={`/backups/${backup.id}`} className="min-w-0 rounded-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/50 focus-visible:ring-offset-2 focus-visible:ring-offset-panel-card">
                        <h2 className="truncate font-semibold text-white transition hover:text-panel-green">{backup.name}</h2>
                      </Link>
                      <span className={cn("shrink-0 rounded px-2 py-0.5 text-xs font-medium", backup.type === "Auto" ? "bg-slate-800 text-slate-300" : "bg-panel-gold/15 text-panel-gold")}>
                        {backup.type === "Auto" ? t("typeAuto") : t("typeManual")}
                      </span>
                    </div>
                    <div className="mt-2 flex min-w-0 flex-wrap gap-x-5 gap-y-1 text-sm text-slate-400">
                      <BackupMeta href={backup.instanceId ? `/servers/${backup.instanceId}` : undefined} label={t("server")} value={serverName} />
                      <BackupMeta label={t("world")} value={backup.world} />
                      <BackupMeta label={t("size")} value={backup.size} />
                    </div>
                  </div>
                  <div className="flex items-center gap-2 text-sm text-slate-400 xl:justify-end">
                    <Clock aria-hidden="true" className="size-4 text-slate-500" />
                    <div className="min-w-0 xl:text-right">
                      <p className="font-medium text-slate-200">{localizeRelativeTime(backup.created, locale)}</p>
                      <p className="text-xs text-slate-500">{formatBackupDate(backup.createdAt, locale)}</p>
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-wrap gap-2 lg:justify-end">
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
              );
            })}
          </div>
        </Card>
      ) : null}
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

function HiddenFeaturePage() {
  return (
    <Card className="p-6">
      <h1 className="text-xl font-semibold text-white">Page not found</h1>
      <p className="mt-2 text-sm text-slate-400">The requested GamePanel Lite page does not exist.</p>
      <Link className="mt-4 inline-flex text-sm font-medium text-panel-green hover:underline" href="/dashboard">
        Back to dashboard
      </Link>
    </Card>
  );
}

function BackupMeta({ href, label, value }: { href?: string; label: string; value: string }) {
  return (
    <span className="min-w-0">
      <span className="text-xs text-slate-500">{label}: </span>
      {href ? (
        <Link href={href} className="font-medium text-panel-green hover:underline">{value}</Link>
      ) : (
        <span className="font-medium text-slate-200">{value}</span>
      )}
    </span>
  );
}

function sortBackupsNewestFirst(a: Backup, b: Backup) {
  return new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime();
}

function formatBackupDate(value: string, locale: string) {
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  }).format(new Date(value));
}

function filterOptionLabel<T extends string>(
  options: readonly { key: T; labelKey?: MessageKey; label?: string }[],
  value: T,
  t: (key: MessageKey) => string
) {
  const option = options.find((item) => item.key === value);
  return option?.labelKey ? t(option.labelKey) : option?.label ?? value;
}
