"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Camera, Clock, Download, History, RotateCcw, Trash2 } from "lucide-react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { useToast } from "@/components/toast-context";
import { createBackup, deleteBackup, downloadBackupFile, listBackups, restoreBackup } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { Backup, GameServerResource } from "@/lib/types";

interface ServerTimeMachineProps {
  server: GameServerResource;
}

export function ServerTimeMachine({ server }: ServerTimeMachineProps) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const toast = useToast();
  const client = useQueryClient();

  const [pendingRestore, setPendingRestore] = useState<Backup | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Backup | null>(null);
  const [downloadingId, setDownloadingId] = useState<string | null>(null);

  const backupsQuery = useQuery({
    queryKey: ["backups", server.id],
    queryFn: listBackups,
    retry: false
  });

  const backups = (backupsQuery.data ?? []).filter((b) => b.instanceId === server.id);

  const snapshotMutation = useMutation({
    mutationFn: async () => {
      return await createBackup(server.id);
    },
    onSuccess: async () => {
      toast.success(
        isZh ? "世界快照保存成功！" : "Snapshot created!",
        isZh ? "当前世界数据已安全存档" : "World state is now safely saved."
      );
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (err) => {
      toast.error(isZh ? "保存快照失败" : "Failed to create snapshot", err instanceof Error ? err.message : "");
    }
  });

  const restoreMutation = useMutation({
    mutationFn: async (backupId: string) => {
      return await restoreBackup(backupId);
    },
    onSuccess: async () => {
      toast.success(
        isZh ? "世界备份还原成功！" : "World rollback completed!",
        isZh ? "世界数据已恢复至选定时空状态" : "World restored."
      );
      setPendingRestore(null);
      await client.invalidateQueries({ queryKey: ["game-servers"] });
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (err) => {
      toast.error(isZh ? "还原失败" : "Rollback failed", err instanceof Error ? err.message : "");
    }
  });

  const deleteMutation = useMutation({
    mutationFn: async (backupId: string) => {
      return await deleteBackup(backupId);
    },
    onSuccess: async () => {
      toast.success(isZh ? "备份已删除" : "Backup deleted");
      setPendingDelete(null);
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (err) => {
      toast.error(isZh ? "删除失败" : "Delete failed", err instanceof Error ? err.message : "");
    }
  });

  const handleDownload = async (b: Backup) => {
    try {
      setDownloadingId(b.id);
      await downloadBackupFile(b.id);
      toast.success(isZh ? "开始下载存档文件" : "Download started");
    } catch (err) {
      toast.error(isZh ? "下载失败" : "Download failed", err instanceof Error ? err.message : "");
    } finally {
      setDownloadingId(null);
    }
  };

  return (
    <div className="space-y-6">
      {/* Top Banner: Call-to-action Save Snapshot */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 rounded-xl border border-slate-800 bg-slate-950/60 p-4 sm:p-5">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <History className="size-4 text-panel-green" />
            <h3 className="text-sm font-bold text-white tracking-tight">
              {isZh ? "存档备份与回档管理" : "World Backups & Rollback"}
            </h3>
            <span className="rounded bg-panel-green/15 px-1.5 py-0.5 text-[10px] font-semibold text-panel-green">
              {isZh ? "存档保护" : "Save Protection"}
            </span>
          </div>
          <p className="text-xs text-slate-400">
            {isZh ? "保存当前世界状态，支持随时一键回档还原" : "Create safe world points and restore anytime."}
          </p>
        </div>

        <button
          type="button"
          disabled={snapshotMutation.isPending}
          onClick={() => snapshotMutation.mutate()}
          className="flex items-center justify-center gap-2 rounded-xl border border-panel-green/40 bg-panel-green/15 px-4 py-2.5 text-xs font-bold text-panel-green shadow-xs transition hover:bg-panel-green/25 active:scale-95 disabled:opacity-50 shrink-0"
        >
          <Camera className="size-4" />
          <span>{snapshotMutation.isPending ? (isZh ? "正在保存..." : "Saving...") : (isZh ? "保存当前世界快照" : "Save Snapshot")}</span>
        </button>
      </div>

      {/* Snapshot Cards Timeline */}
      <div className="space-y-3">
        <h4 className="text-xs font-bold uppercase tracking-wider text-slate-400">
          {isZh ? "历史备份列表" : "Historical Backups"} ({backups.length})
        </h4>

        {backupsQuery.isLoading ? (
          <p className="text-xs text-slate-500 py-6 text-center">{isZh ? "正在加载存档列表..." : "Loading snapshots..."}</p>
        ) : backups.length === 0 ? (
          <div className="rounded-xl border border-dashed border-slate-800 bg-slate-950/30 p-8 text-center">
            <p className="text-xs text-slate-500">{isZh ? "暂无任何世界快照备份" : "No backups created yet"}</p>
          </div>
        ) : (
          <div className="grid gap-3">
            {backups.map((b, index) => {
              const isRecent = index === 0;
              const dateStr = new Date(b.createdAt).toLocaleString(isZh ? "zh-CN" : "en-US");
              const sizeMB = (b.sizeBytes / (1024 * 1024)).toFixed(1);

              return (
                <div
                  key={b.id}
                  className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 rounded-xl border border-slate-800/90 bg-slate-900/60 p-4 transition hover:border-slate-700 hover:bg-slate-900/80"
                >
                  <div className="flex items-start gap-3 min-w-0">
                    <div className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-slate-700 bg-slate-950 text-slate-300">
                      <Clock className="size-4 text-sky-400" />
                    </div>
                    <div className="min-w-0 space-y-0.5">
                      <div className="flex items-center gap-2">
                        <p className="text-xs font-bold text-white truncate">{b.name}</p>
                        {isRecent && (
                          <span className="rounded bg-panel-green/20 border border-panel-green/40 px-1.5 py-0.5 text-[9px] font-bold text-panel-green">
                            {isZh ? "最新存档" : "Latest"}
                          </span>
                        )}
                        <span className="rounded bg-slate-800 px-1.5 py-0.5 text-[9px] font-mono text-slate-400">
                          {b.type}
                        </span>
                      </div>
                      <p className="text-[11px] text-slate-400 font-mono">
                        {dateStr} · <strong className="text-slate-300">{sizeMB} MB</strong>
                      </p>
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex items-center gap-2 shrink-0 self-end sm:self-center">
                    <button
                      type="button"
                      disabled={downloadingId === b.id}
                      onClick={() => handleDownload(b)}
                      title={isZh ? "下载存档文件到本地" : "Download file"}
                      className="flex size-8 items-center justify-center rounded-lg border border-slate-800 bg-slate-950/80 text-slate-400 hover:text-white hover:border-slate-700 transition"
                    >
                      <Download className="size-3.5" />
                    </button>

                    <button
                      type="button"
                      onClick={() => setPendingDelete(b)}
                      title={isZh ? "删除该快照" : "Delete snapshot"}
                      className="flex size-8 items-center justify-center rounded-lg border border-slate-800 bg-slate-950/80 text-slate-400 hover:text-rose-400 hover:border-rose-500/40 transition"
                    >
                      <Trash2 className="size-3.5" />
                    </button>

                    <button
                      type="button"
                      onClick={() => setPendingRestore(b)}
                      className="flex items-center gap-1.5 rounded-lg border border-panel-green/40 bg-panel-green px-3 py-1.5 text-xs font-bold text-slate-950 shadow-xs transition hover:bg-panel-green/90 active:scale-95"
                    >
                      <RotateCcw className="size-3.5" />
                      <span>{isZh ? "还原此备份" : "Rollback"}</span>
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Confirm Restore Dialog */}
      <ConfirmDialog
        open={Boolean(pendingRestore)}
        eyebrow={isZh ? "备份还原确认" : "Rollback Confirmation"}
        eyebrowTone="gold"
        title={isZh ? "确认还原世界至此历史备份？" : "Confirm World Rollback?"}
        description={
          isZh
            ? `确定要将世界恢复到【${pendingRestore?.name}】吗？当前世界数据将被替换，服务器将自动重新加载。`
            : `Are you sure you want to restore to ${pendingRestore?.name}? Current game state will be replaced.`
        }
        detail={
          pendingRestore ? (
            <div className="text-xs space-y-1 font-mono text-slate-300">
              <p>{isZh ? "备份名称" : "Snapshot"}: {pendingRestore.name}</p>
              <p>{isZh ? "保存时间" : "Saved At"}: {new Date(pendingRestore.createdAt).toLocaleString()}</p>
            </div>
          ) : null
        }
        cancelLabel={isZh ? "取消" : "Cancel"}
        confirmLabel={isZh ? "确认还原" : "Confirm Rollback"}
        confirmVariant="gold"
        busy={restoreMutation.isPending}
        onConfirm={() => pendingRestore && restoreMutation.mutate(pendingRestore.id)}
        onCancel={() => setPendingRestore(null)}
      />

      {/* Confirm Delete Dialog */}
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        eyebrow={isZh ? "删除操作" : "Delete Confirmation"}
        title={isZh ? "确认删除该世界备份？" : "Confirm Delete Backup?"}
        description={
          isZh
            ? `确定要永久删除备份【${pendingDelete?.name}】吗？删除后将无法通过此快照进行回档。`
            : `Are you sure you want to delete ${pendingDelete?.name}?`
        }
        cancelLabel={isZh ? "取消" : "Cancel"}
        confirmLabel={isZh ? "确认删除" : "Delete"}
        confirmVariant="danger"
        busy={deleteMutation.isPending}
        onConfirm={() => pendingDelete && deleteMutation.mutate(pendingDelete.id)}
        onCancel={() => setPendingDelete(null)}
      />
    </div>
  );
}
