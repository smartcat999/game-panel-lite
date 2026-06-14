"use client";

import { Archive, Download, RotateCcw, Trash2 } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";
import { backups as mockBackups } from "@/lib/mock-data";
import { backupDownloadUrl, createBackup, deleteBackup, listBackups, listServers, restoreBackup } from "@/lib/api";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";

export default function BackupsPage() {
  const { locale, t } = useI18n();
  const client = useQueryClient();
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, retry: false });
  const [selectedServerId, setSelectedServerId] = useState("");
  const servers = serversQuery.data ?? [];
  const backups = backupsQuery.data && backupsQuery.data.length > 0 ? backupsQuery.data : mockBackups;
  const activeServerId = selectedServerId || servers[0]?.id || "";
  const serverNameById = useMemo(() => new Map(servers.map((server) => [server.id, server.name])), [servers]);

  const create = useMutation({
    mutationFn: createBackup,
    onSuccess: async () => client.invalidateQueries({ queryKey: ["backups"] }),
    onError: (error) => window.alert(error instanceof Error ? error.message : t("unableCreateBackup"))
  });
  const restore = useMutation({
    mutationFn: restoreBackup,
    onSuccess: async () => client.invalidateQueries({ queryKey: ["backups"] }),
    onError: (error) => window.alert(error instanceof Error ? error.message : t("unableRestoreBackup"))
  });
  const remove = useMutation({
    mutationFn: deleteBackup,
    onSuccess: async () => client.invalidateQueries({ queryKey: ["backups"] }),
    onError: (error) => window.alert(error instanceof Error ? error.message : t("unableDeleteBackup"))
  });

  return (
    <AppShell>
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
      {(serversQuery.isError || backupsQuery.isError) && <p className="mb-4 text-sm text-panel-gold">{t("apiMockBackups")}</p>}
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
                      onClick={() => {
                        if (window.confirm(t("restoreBackupConfirm", { name: backup.name }))) restore.mutate(backup.id);
                      }}
                      disabled={restore.isPending || backupsQuery.isError}
                    >
                      <RotateCcw aria-hidden="true" />
                    </Button>
                    <a href={backupDownloadUrl(backup.id)}>
                      <Button variant="secondary" disabled={backupsQuery.isError}>
                        <Download aria-hidden="true" />
                      </Button>
                    </a>
                    <Button
                      variant="danger"
                      onClick={() => {
                        if (window.confirm(t("deleteBackupConfirm", { name: backup.name }))) remove.mutate(backup.id);
                      }}
                      disabled={remove.isPending || backupsQuery.isError}
                    >
                      <Trash2 aria-hidden="true" />
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </AppShell>
  );
}
