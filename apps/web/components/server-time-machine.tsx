"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Camera, Clock, Download, History, RotateCcw, ShieldCheck, Trash2 } from "lucide-react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { useToast } from "@/components/toast-context";
import {
  createBackup,
  deleteBackup,
  downloadBackupFile,
  getServerBackupPolicy,
  listBackups,
  restoreBackup,
  updateServerBackupPolicy
} from "@/lib/api";
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
  const [filterType, setFilterType] = useState<"all" | "manual" | "auto">("all");

  // Scheduled Auto-Backup Policy State
  const [policyEnabled, setPolicyEnabled] = useState<boolean>(true);
  const [intervalHours, setIntervalHours] = useState<number>(6);
  const [retentionCount, setRetentionCount] = useState<number>(7);
  const [policyDirty, setPolicyDirty] = useState<boolean>(false);

  const policyQuery = useQuery({
    queryKey: ["backup-policy", server.id],
    queryFn: () => getServerBackupPolicy(server.id),
    retry: false
  });

  useEffect(() => {
    if (policyQuery.data) {
      setPolicyEnabled(policyQuery.data.enabled);
      setIntervalHours(policyQuery.data.intervalHours || 6);
      setRetentionCount(policyQuery.data.retentionCount || 7);
      setPolicyDirty(false);
    }
  }, [policyQuery.data]);

  const policyMutation = useMutation({
    mutationFn: async (payload: { enabled: boolean; intervalHours: number; retentionCount: number }) => {
      return await updateServerBackupPolicy(server.id, payload);
    },
    onSuccess: async () => {
      toast.success(isZh ? "自动备份策略已保存" : "Backup policy saved");
      setPolicyDirty(false);
      await client.invalidateQueries({ queryKey: ["backup-policy", server.id] });
    },
    onError: (err) => {
      toast.error(isZh ? "保存策略失败" : "Failed to save policy", err instanceof Error ? err.message : "");
    }
  });

  const backupsQuery = useQuery({
    queryKey: ["backups", server.id],
    queryFn: listBackups,
    retry: false
  });

  const allBackups = (backupsQuery.data ?? []).filter((b) => b.instanceId === server.id);
  const backups = allBackups.filter((b) => {
    if (filterType === "manual") return b.type === "Manual";
    if (filterType === "auto") return b.type === "Auto" || (b.type as string) === "Scheduled";
    return true;
  });

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

      {/* Auto-Backup Policy Settings Card */}
      <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-4 sm:p-5 space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <ShieldCheck className="size-4 text-sky-400" />
              <h4 className="text-sm font-bold text-white tracking-tight">
                {isZh ? "定时自动备份策略" : "Scheduled Auto-Backup Policy"}
              </h4>
              <span
                className={`rounded px-1.5 py-0.5 text-[10px] font-semibold transition ${
                  policyEnabled
                    ? "bg-emerald-500/15 text-emerald-400 border border-emerald-500/30"
                    : "bg-slate-800 text-slate-400 border border-slate-700"
                }`}
              >
                {policyEnabled ? (isZh ? "运行中自动备份" : "Active") : (isZh ? "已禁用" : "Disabled")}
              </span>
            </div>
            <p className="text-xs text-slate-400">
              {isZh
                ? "服务器运行期间定时自动创建轻量存档。超期按保留份数自动轮转清理，不占多余磁盘空间。"
                : "Automatically archives game saves while the server runs. Older backups are safely pruned."}
            </p>
          </div>

          {/* Enable Toggle Switch */}
          <div className="flex items-center gap-3 shrink-0">
            <label className="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                checked={policyEnabled}
                onChange={(e) => {
                  setPolicyEnabled(e.target.checked);
                  setPolicyDirty(true);
                }}
                className="sr-only peer"
              />
              <div className="w-11 h-6 bg-slate-800 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-emerald-500"></div>
            </label>
            <span className="text-xs font-semibold text-slate-300">
              {policyEnabled ? (isZh ? "已开启" : "Enabled") : (isZh ? "已关闭" : "Disabled")}
            </span>
          </div>
        </div>

        {/* Policy Controls Grid */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 pt-3 border-t border-slate-900">
          {/* Interval Select */}
          <div className="space-y-1.5">
            <label className="text-[11px] font-medium text-slate-400 flex items-center gap-1.5">
              <Clock className="size-3.5 text-slate-500" />
              {isZh ? "备份频率周期" : "Backup Interval"}
            </label>
            <select
              value={intervalHours}
              disabled={!policyEnabled || policyMutation.isPending}
              onChange={(e) => {
                setIntervalHours(Number(e.target.value));
                setPolicyDirty(true);
              }}
              className="w-full rounded-lg border border-slate-800 bg-slate-900/90 px-3 py-2 text-xs font-medium text-white focus:border-emerald-500 focus:outline-none disabled:opacity-50"
            >
              <option value={2}>{isZh ? "每 2 小时 (高频防炸档)" : "Every 2 Hours"}</option>
              <option value={6}>{isZh ? "每 6 小时 (推荐默认)" : "Every 6 Hours (Recommended)"}</option>
              <option value={12}>{isZh ? "每 12 小时 (半日备份)" : "Every 12 Hours"}</option>
              <option value={24}>{isZh ? "每 24 小时 (每日一次)" : "Every 24 Hours (Daily)"}</option>
            </select>
          </div>

          {/* Retention Select */}
          <div className="space-y-1.5">
            <label className="text-[11px] font-medium text-slate-400 flex items-center gap-1.5">
              <RotateCcw className="size-3.5 text-slate-500" />
              {isZh ? "最大保留份数 (轮转配额)" : "Retention Count"}
            </label>
            <select
              value={retentionCount}
              disabled={!policyEnabled || policyMutation.isPending}
              onChange={(e) => {
                setRetentionCount(Number(e.target.value));
                setPolicyDirty(true);
              }}
              className="w-full rounded-lg border border-slate-800 bg-slate-900/90 px-3 py-2 text-xs font-medium text-white focus:border-emerald-500 focus:outline-none disabled:opacity-50"
            >
              <option value={3}>{isZh ? "保留最近 3 份" : "Keep 3 Backups"}</option>
              <option value={5}>{isZh ? "保留最近 5 份" : "Keep 5 Backups"}</option>
              <option value={7}>{isZh ? "保留最近 7 份 (推荐)" : "Keep 7 Backups (Recommended)"}</option>
              <option value={14}>{isZh ? "保留最近 14 份" : "Keep 14 Backups"}</option>
            </select>
          </div>

          {/* Status info & Save button */}
          <div className="space-y-1.5 sm:col-span-2 lg:col-span-1 flex flex-col justify-end">
            <div className="flex items-center justify-between gap-2">
              <div className="text-[11px] text-slate-400">
                {policyQuery.data?.nextRunAt ? (
                  <span>
                    {isZh ? "下次自动备份: " : "Next: "}
                    <strong className="text-slate-300 font-mono">
                      {new Date(policyQuery.data.nextRunAt).toLocaleTimeString(isZh ? "zh-CN" : "en-US", {
                        hour: "2-digit",
                        minute: "2-digit"
                      })}
                    </strong>
                  </span>
                ) : (
                  <span className="text-slate-500">{isZh ? "当前未排期" : "Not scheduled"}</span>
                )}
              </div>
              {policyDirty && (
                <button
                  type="button"
                  disabled={policyMutation.isPending}
                  onClick={() => {
                    policyMutation.mutate(
                      {
                        enabled: policyEnabled,
                        intervalHours,
                        retentionCount
                      },
                      {
                        onSuccess: () => setPolicyDirty(false)
                      }
                    );
                  }}
                  className="rounded-lg bg-emerald-500 hover:bg-emerald-400 text-slate-950 font-bold px-3 py-1.5 text-xs transition disabled:opacity-50 shadow-xs"
                >
                  {policyMutation.isPending ? (isZh ? "保存中..." : "Saving...") : (isZh ? "保存策略" : "Save Policy")}
                </button>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Snapshot Cards Timeline */}
      <div className="space-y-3">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
          <h4 className="text-xs font-bold uppercase tracking-wider text-slate-400">
            {isZh ? "历史备份列表" : "Historical Backups"} ({backups.length})
          </h4>

          {/* Filter pills */}
          <div className="flex items-center gap-1.5 text-xs">
            <button
              type="button"
              onClick={() => setFilterType("all")}
              className={`px-2.5 py-1 rounded-md text-[11px] font-semibold transition ${
                filterType === "all"
                  ? "bg-slate-800 text-white border border-slate-700"
                  : "text-slate-400 hover:text-white"
              }`}
            >
              {isZh ? "全部" : "All"} ({allBackups.length})
            </button>
            <button
              type="button"
              onClick={() => setFilterType("manual")}
              className={`px-2.5 py-1 rounded-md text-[11px] font-semibold transition ${
                filterType === "manual"
                  ? "bg-emerald-500/20 text-emerald-400 border border-emerald-500/40"
                  : "text-slate-400 hover:text-white"
              }`}
            >
              {isZh ? "手动快照" : "Manual"} ({allBackups.filter((b) => b.type === "Manual").length})
            </button>
            <button
              type="button"
              onClick={() => setFilterType("auto")}
              className={`px-2.5 py-1 rounded-md text-[11px] font-semibold transition ${
                filterType === "auto"
                  ? "bg-sky-500/20 text-sky-400 border border-sky-500/40"
                  : "text-slate-400 hover:text-white"
              }`}
            >
              {isZh ? "自动定时" : "Auto"} ({allBackups.filter((b) => b.type === "Auto" || (b.type as string) === "Scheduled").length})
            </button>
          </div>
        </div>

        {backupsQuery.isLoading ? (
          <p className="text-xs text-slate-500 py-6 text-center">{isZh ? "正在加载存档列表..." : "Loading snapshots..."}</p>
        ) : backups.length === 0 ? (
          <div className="rounded-xl border border-dashed border-slate-800 bg-slate-950/30 p-8 text-center">
            <p className="text-xs text-slate-500">
              {filterType === "all"
                ? isZh ? "暂无任何世界快照备份" : "No backups created yet"
                : isZh ? "该分类下暂无备份" : "No backups in this category"}
            </p>
          </div>
        ) : (
          <div className="grid gap-3">
            {backups.map((b, index) => {
              const isRecent = index === 0 && filterType === "all";
              const isAuto = b.type === "Auto" || (b.type as string) === "Scheduled";
              const dateStr = new Date(b.createdAt).toLocaleString(isZh ? "zh-CN" : "en-US");
              const sizeMB = (b.sizeBytes / (1024 * 1024)).toFixed(1);

              return (
                <div
                  key={b.id}
                  className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 rounded-xl border border-slate-800/90 bg-slate-900/60 p-4 transition hover:border-slate-700 hover:bg-slate-900/80"
                >
                  <div className="flex items-start gap-3 min-w-0">
                    <div
                      className={`flex size-9 shrink-0 items-center justify-center rounded-lg border ${
                        isAuto
                          ? "border-sky-500/40 bg-sky-950/40 text-sky-400"
                          : "border-emerald-500/40 bg-emerald-950/40 text-emerald-400"
                      }`}
                    >
                      {isAuto ? <Clock className="size-4" /> : <Camera className="size-4" />}
                    </div>
                    <div className="min-w-0 space-y-0.5">
                      <div className="flex items-center gap-2 flex-wrap">
                        <p className="text-xs font-bold text-white truncate max-w-xs">{b.name}</p>
                        {isRecent && (
                          <span className="rounded bg-panel-green/20 border border-panel-green/40 px-1.5 py-0.5 text-[9px] font-bold text-panel-green">
                            {isZh ? "最新存档" : "Latest"}
                          </span>
                        )}
                        {isAuto ? (
                          <span className="rounded bg-sky-500/15 border border-sky-500/30 px-1.5 py-0.5 text-[9px] font-bold text-sky-400 flex items-center gap-1 font-mono">
                            <Clock className="size-2.5" />
                            {isZh ? "自动定时" : "Auto"}
                          </span>
                        ) : (
                          <span className="rounded bg-emerald-500/15 border border-emerald-500/30 px-1.5 py-0.5 text-[9px] font-bold text-emerald-400 flex items-center gap-1 font-mono">
                            <Camera className="size-2.5" />
                            {isZh ? "手动快照" : "Manual"}
                          </span>
                        )}
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
