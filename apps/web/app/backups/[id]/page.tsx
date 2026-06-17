"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, ArrowRight, Download, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PageHeader } from "@/components/page-header";
import { Button, Card } from "@/components/ui";
import { deleteBackup, downloadBackupFile, listBackups, listServers } from "@/lib/api";
import { saveBlob } from "@/lib/download";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";

export default function BackupDetailPage() {
  const { locale, t } = useI18n();
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const client = useQueryClient();
  const id = params.id;
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, retry: false });
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, retry: false });
  const [pendingDelete, setPendingDelete] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const backup = useMemo(() => (backupsQuery.data ?? []).find((item) => item.id === id), [backupsQuery.data, id]);
  const server = useMemo(() => (backup?.instanceId ? (serversQuery.data ?? []).find((item) => item.id === backup.instanceId) : undefined), [backup?.instanceId, serversQuery.data]);

  const remove = useMutation({
    mutationFn: deleteBackup,
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["backups"] });
      router.push("/backups");
    },
    onError: (error) => setErrorMessage(error instanceof Error ? error.message : t("unableDeleteBackup"))
  });

  const download = async () => {
    if (!backup) return;
    setDownloading(true);
    setErrorMessage("");
    try {
      const blob = await downloadBackupFile(backup.id);
      saveBlob(blob, backup.name);
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : t("unableDownloadBackup"));
    } finally {
      setDownloading(false);
    }
  };

  if (backupsQuery.isLoading) {
    return <p className="text-sm text-slate-400">{t("loading")}</p>;
  }

  if (backupsQuery.isError || !backup) {
    return (
      <>
        <BackLink />
        <Card className="p-6">
          <p className="text-sm text-panel-gold">{backupsQuery.isError ? t("apiBackupsUnavailable") : t("backupNotFound")}</p>
        </Card>
      </>
    );
  }

  const serverName = server?.name ?? backup.server;

  return (
    <>
      <BackLink />
      <PageHeader title={backup.name} description={t("backupDetailDescription")} />
      {errorMessage && <p className="mb-4 text-sm text-panel-gold">{errorMessage}</p>}
      <div className="grid gap-4 xl:grid-cols-[1fr_320px]">
        <div className="space-y-4">
          <Card className="p-4">
            <h2 className="font-semibold">{t("backupName")}</h2>
            <div className="mt-4 grid gap-3 md:grid-cols-2">
              <DetailTile label={t("server")} value={serverName} />
              <DetailTile label={t("world")} value={backup.world} />
              <DetailTile label={t("type")} value={backup.type === "Auto" ? t("typeAuto") : t("typeManual")} />
              <DetailTile label={t("size")} value={backup.size} />
              <DetailTile label={t("created")} value={localizeRelativeTime(backup.created, locale)} />
              <DetailTile label={t("backupName")} value={backup.name} />
            </div>
          </Card>
        </div>
        <div className="space-y-4">
          <Card className="p-4">
            <h2 className="font-semibold">{t("relatedServers")}</h2>
            {backup.instanceId ? (
              <Link href={`/servers/${backup.instanceId}`} className="mt-4 flex items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/35 px-3 py-3 transition hover:border-panel-green/50 hover:bg-slate-900/60">
                <span className="truncate text-sm font-medium text-slate-100">{serverName}</span>
                <ArrowRight aria-hidden="true" className="size-4 shrink-0 text-slate-500" />
              </Link>
            ) : (
              <p className="mt-4 text-sm text-slate-500">{t("notInUse")}</p>
            )}
          </Card>
          <Card className="p-4">
            <h2 className="font-semibold">{t("actions")}</h2>
            <div className="mt-4 grid gap-2">
              <Button variant="secondary" onClick={() => void download()} disabled={downloading}>
                <Download aria-hidden="true" />
                {downloading ? t("downloading") : t("download")}
              </Button>
              <Button variant="danger" onClick={() => setPendingDelete(true)} disabled={remove.isPending}>
                <Trash2 aria-hidden="true" />
                {t("delete")}
              </Button>
            </div>
          </Card>
        </div>
      </div>

      <ConfirmDialog
        open={pendingDelete}
        eyebrow={t("destructiveAction")}
        title={t("deleteBackupConfirm", { name: backup.name })}
        description={t("confirmDeleteBackupDescription", { name: backup.name })}
        detail={<DetailLine label={t("backupName")} value={backup.name} />}
        cancelLabel={t("cancel")}
        confirmLabel={remove.isPending ? t("actionWorking") : t("delete")}
        confirmVariant="danger"
        busy={remove.isPending}
        onCancel={() => setPendingDelete(false)}
        onConfirm={() => remove.mutate(backup.id)}
      />
    </>
  );
}

function BackLink() {
  const { t } = useI18n();
  return (
    <Link href="/backups" className="mb-4 inline-flex items-center gap-2 text-sm font-medium text-slate-400 transition hover:text-white">
      <ArrowLeft aria-hidden="true" className="size-4" />
      {t("backToBackups")}
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
