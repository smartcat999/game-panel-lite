"use client";

import { DragEvent, FormEvent, useEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  Clock,
  DatabaseBackup,
  Download,
  Loader2,
  Terminal,
  Trash2,
  Upload,
} from "lucide-react";
import { useConsole } from "../../../console/ConsoleProvider";
import {
  Badge,
  Button,
  Card,
  CardHeader,
  ConfirmDialog,
  EmptyState,
  Field,
  Input,
  Modal,
  Mono,
  MoreMenu,
  Segmented,
  Select,
  Switch,
  focusRing,
  type MenuItem,
} from "../ui";
import { TaskLogModal, formatTaskByteValue } from "../shared/TaskLogModal";
import { addonBindingMeta } from "./shared";
import { formatBytes, relativeTime } from "../../../console/lib/format";
import type { AppItem, InstanceBackupRecord, InstanceItem } from "../../../console/lib/types";
import { type RestoreArchiveOptions, useServerActions, useServerFiles } from "./api";
import { BackupDetailModal } from "./BackupDetailModal";
import { BackupRestoreModal } from "./BackupRestoreModal";
import {
  buildArchivePath,
  buildBackupRowMeta,
  formatBackupDisplayName,
  formatBackupSaveConfirmationSummary,
  formatBackupStageLabel,
  isTerminalBackupPhase,
  splitSubtreeDirs,
} from "./backupUtils";

function formatNextBackupHint(inst: InstanceItem): string {
  const policy = inst.spec.backupPolicy;
  if (!policy?.enabled) return "";
  if (inst.status?.phase !== "Running") return "运行后自动执行";
  const hours = policy.intervalHours && policy.intervalHours > 0 ? policy.intervalHours : 24;
  let lastMs = 0;
  for (const b of inst.status?.backups || []) {
    if (b.completedAt) {
      const t = new Date(b.completedAt).getTime();
      if (!Number.isNaN(t) && t > lastMs) lastMs = t;
    }
  }
  if (!lastMs && inst.status?.startedAt) {
    const t = new Date(inst.status.startedAt).getTime();
    if (!Number.isNaN(t)) lastMs = t;
  }
  if (!lastMs && inst.metadata.creationTimestamp) {
    const t = new Date(inst.metadata.creationTimestamp).getTime();
    if (!Number.isNaN(t)) lastMs = t;
  }
  if (!lastMs) return "即将执行";
  const nextMs = lastMs + hours * 3600 * 1000;
  if (nextMs <= Date.now()) return "即将执行";
  const nextDate = new Date(nextMs);
  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  const hm = `${pad(nextDate.getHours())}:${pad(nextDate.getMinutes())}`;
  const isToday =
    nextDate.getFullYear() === now.getFullYear() &&
    nextDate.getMonth() === now.getMonth() &&
    nextDate.getDate() === now.getDate();
  const tomorrow = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
  const isTomorrow =
    nextDate.getFullYear() === tomorrow.getFullYear() &&
    nextDate.getMonth() === tomorrow.getMonth() &&
    nextDate.getDate() === tomorrow.getDate();
  if (isToday) return `下次今日 ${hm}`;
  if (isTomorrow) return `下次明日 ${hm}`;
  return `下次 ${pad(nextDate.getMonth() + 1)}-${pad(nextDate.getDate())} ${hm}`;
}

export function BackupTab({
  inst,
  app,
  running,
  onOpenConsole,
}: {
  inst: InstanceItem;
  app?: AppItem;
  running?: boolean;
  onOpenConsole?: () => void;
}) {
  const name = inst.metadata.name;
  const isRunning = running ?? inst.status?.phase === "Running";
  const { toast, reloadInstances, quota, token, tenant } = useConsole();
  const { archive, deleteBackup, downloadBackup, uploadArchive } = useServerFiles(name);
  const { updateSpec } = useServerActions();

  const [busy, setBusy] = useState<"backup" | "restore" | "delete" | "download" | "policy" | "import" | null>(null);
  const [actionModalOpen, setActionModalOpen] = useState(false);
  const [actionMode, setActionMode] = useState<"create" | "import">("create");
  const [policyOpen, setPolicyOpen] = useState(false);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [importLabel, setImportLabel] = useState("");
  const [importUploadPct, setImportUploadPct] = useState<number | null>(null);
  const [dragActive, setDragActive] = useState(false);
  const importFileInputRef = useRef<HTMLInputElement | null>(null);
  const [backupLabel, setBackupLabel] = useState("");
  const [detailTargetPath, setDetailTargetPath] = useState<string | null>(null);
  const [restoreTargetPath, setRestoreTargetPath] = useState<string | null>(null);
  const [confirmDeleteTarget, setConfirmDeleteTarget] = useState<string | null>(null);
  const [logTarget, setLogTarget] = useState<InstanceBackupRecord | null>(null);

  const curPolicy = inst.spec.backupPolicy;
  const [policyEnabled, setPolicyEnabled] = useState(Boolean(curPolicy?.enabled));
  const [policyInterval, setPolicyInterval] = useState(String(curPolicy?.intervalHours || 24));
  const [policyRetention, setPolicyRetention] = useState(String(curPolicy?.retentionCount || 5));

  const backups = useMemo(() => inst.status?.backups ?? [], [inst.status?.backups]);
  const hasActiveBackupWork = backups.some((b) =>
    Boolean(!isTerminalBackupPhase(b.phase) || b.lastRestoreResult === "Running"),
  );
  const prevPhasesRef = useRef<Record<string, string>>({});
  const prevRestoreRef = useRef<Record<string, string>>({});

  useEffect(() => {
    const prev = prevPhasesRef.current;
    const prevRestore = prevRestoreRef.current;
    const next: Record<string, string> = {};
    const nextRestore: Record<string, string> = {};
    for (const [oldPath, oldPhase] of Object.entries(prev)) {
      if (oldPhase === "Deleting" && !backups.some((item) => item.path === oldPath)) {
        toast("success", `备份「${formatBackupDisplayName(oldPath, name)}」已删除`);
      }
    }
    for (const b of backups) {
      const phase = b.phase || "Completed";
      const restoreResult = b.lastRestoreResult || "";
      next[b.path] = phase;
      if (restoreResult) nextRestore[b.path] = restoreResult;
      const oldPhase = prev[b.path];
      const oldRestore = prevRestore[b.path];
      const displayName = formatBackupDisplayName(b.path, name);
      if (oldPhase && oldPhase !== "Restoring" && !isTerminalBackupPhase(oldPhase) && phase === "Completed") {
        const sizeText = b.sizeBytes ? `（${formatBytes(b.sizeBytes)}）` : "";
        if (b.saveConfirmation?.status === "Unconfirmed") {
          toast(
            "info",
            `备份「${displayName}」已归档${sizeText}，但存档落盘未确认：${b.saveConfirmation.warning || "请留意存档一致性"}`,
          );
        } else {
          toast(
            "success",
            b.imported
              ? `导入备份「${displayName}」解析完成，可随时恢复`
              : `备份「${displayName}」创建成功${sizeText}`,
          );
        }
      } else if (
        oldPhase &&
        oldPhase !== "Failed" &&
        oldPhase !== "InspectFailed" &&
        (phase === "Failed" || phase === "InspectFailed")
      ) {
        toast(
          "error",
          b.message || (phase === "InspectFailed" ? `导入备份「${displayName}」解析失败` : `备份「${displayName}」失败`),
        );
      }
      if (oldRestore === "Running" && restoreResult === "Succeeded") {
        toast("success", `存档「${displayName}」恢复完成`);
      } else if (oldRestore === "Running" && restoreResult === "Failed") {
        toast("error", b.lastRestoreMessage || b.message || `存档「${displayName}」恢复失败`);
      }
    }
    prevPhasesRef.current = next;
    prevRestoreRef.current = nextRestore;
  }, [backups, name, toast]);

  function openPolicyModal() {
    setPolicyEnabled(Boolean(inst.spec.backupPolicy?.enabled));
    setPolicyInterval(String(inst.spec.backupPolicy?.intervalHours || 24));
    setPolicyRetention(String(inst.spec.backupPolicy?.retentionCount || 5));
    setPolicyOpen(true);
  }

  const policyDirty =
    policyEnabled !== Boolean(inst.spec.backupPolicy?.enabled) ||
    String(policyInterval) !== String(inst.spec.backupPolicy?.intervalHours || 24) ||
    String(policyRetention) !== String(inst.spec.backupPolicy?.retentionCount || 5);

  async function handleSavePolicy(e: FormEvent) {
    e.preventDefault();
    setBusy("policy");
    try {
      const ok = await updateSpec(
        inst,
        {
          backupPolicy: {
            ...inst.spec.backupPolicy,
            enabled: policyEnabled,
            intervalHours: Number(policyInterval) || 24,
            retentionCount: Number(policyRetention) || 5,
          },
        },
        policyEnabled ? "自动备份已开启" : "自动备份已关闭",
      );
      if (ok) setPolicyOpen(false);
    } finally {
      setBusy(null);
    }
  }

  async function runArchive(mode: "backup" | "restore", targetPath: string, opts?: RestoreArchiveOptions) {
    const path = targetPath.trim();
    if (!path) return;
    setBusy(mode);
    try {
      const out = await archive(mode, path, opts);
      const displayName = formatBackupDisplayName(out.path || path, name);
      toast(
        "success",
        mode === "backup"
          ? `已提交备份请求「${displayName}」，正在后台打包上传`
          : `已提交恢复请求「${displayName}」，正在后台下载并恢复`,
      );
      setActionModalOpen(false);
      setBackupLabel("");
      setRestoreTargetPath(null);
      await reloadInstances();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : mode === "backup" ? "备份失败" : "恢复失败");
    } finally {
      setBusy(null);
    }
  }

  async function handleCreateSubmit(e: FormEvent) {
    e.preventDefault();
    await runArchive("backup", buildArchivePath(name, backupLabel));
  }

  async function runDelete(targetPath: string) {
    const path = targetPath.trim();
    if (!path) return;
    setBusy("delete");
    try {
      await deleteBackup(path);
      toast("success", "已提交删除请求");
      await reloadInstances();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "删除备份失败");
    } finally {
      setBusy(null);
      setConfirmDeleteTarget(null);
    }
  }

  async function runDownload(targetPath: string, displayName: string) {
    setBusy("download");
    try {
      await downloadBackup(targetPath, displayName);
      toast("success", `正在下载「${displayName}」`);
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "下载备份失败");
    } finally {
      setBusy(null);
    }
  }

  function applySelectedImportFile(file: File | null) {
    if (!file) {
      if (importFileInputRef.current) importFileInputRef.current.value = "";
      setImportFile(null);
      return;
    }
    if (!/\.(tar\.gz|tgz|zip)$/i.test(file.name)) {
      if (importFileInputRef.current) importFileInputRef.current.value = "";
      toast("error", "请选择 .tar.gz / .tgz / .zip 格式的备份归档");
      return;
    }
    setImportFile(file);
    if (!importLabel.trim()) {
      const rawStem = file.name.replace(/\.(tar\.gz|tgz|zip)$/i, "");
      const stem = rawStem.toLowerCase().startsWith(`${name.toLowerCase()}-`)
        ? rawStem.slice(name.length + 1)
        : rawStem;
      setImportLabel(stem.trim());
    }
  }

  function handleDropImportFile(e: DragEvent<HTMLButtonElement>) {
    e.preventDefault();
    setDragActive(false);
    const f = e.dataTransfer.files?.[0];
    if (f) applySelectedImportFile(f);
  }

  async function handleImportArchive(e: FormEvent) {
    e.preventDefault();
    if (!importFile) {
      toast("error", "请先选择要导入的 .tar.gz / .zip 备份文件");
      return;
    }
    setBusy("import");
    setImportUploadPct(0);
    try {
      const ext: ".tar.gz" | ".zip" = /\.zip$/i.test(importFile.name) ? ".zip" : ".tar.gz";
      const fallbackLabel = importFile.name.replace(/\.(tar\.gz|tgz|zip)$/i, "").trim();
      const label = importLabel.trim() || fallbackLabel || "本地导入存档";
      const out = await uploadArchive(buildArchivePath(name, label, ext), importFile, (pct) => setImportUploadPct(pct));
      const displayName = formatBackupDisplayName(out.path, name);
      toast("success", `已上传「${displayName}」（${formatBytes(out.sizeBytes || importFile.size)}），正在后台解析归档结构`);
      setActionModalOpen(false);
      if (importFileInputRef.current) importFileInputRef.current.value = "";
      setImportFile(null);
      setImportLabel("");
      await reloadInstances();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "导入本地备份失败");
    } finally {
      setImportUploadPct(null);
      setBusy(null);
    }
  }

  const addonNoun = addonBindingMeta(app?.addonBinding).noun;
  const subtreeSummary =
    splitSubtreeDirs(inst.spec.backupPolicy?.subtree || app?.storagePolicy?.backupSubtree).join(", ") || "工作区目录";
  const configFieldCount = app?.configSchema?.length || 0;
  const configSummary =
    configFieldCount > 0
      ? `${configFieldCount} 项配置`
      : (inst.spec.files?.length || 0) > 0
        ? `${inst.spec.files!.length} 个配置文件`
        : "实例配置";
  const hasAddonBinding = Boolean(app?.addonBinding);
  const thirdCellLabel = hasAddonBinding ? `${addonNoun}记录` : "服务端版本";
  const serverVersionText =
    inst.spec.serverArtifact?.version ||
    inst.metadata.annotations?.["core.gamepanel.io/app-version"] ||
    "跟随实例";
  const thirdCellValue = hasAddonBinding
    ? (inst.spec.addons?.length || 0) > 0
      ? `${inst.spec.addons!.length} 个${addonNoun}`
      : `无${addonNoun}`
    : serverVersionText;

  const detailRecord = detailTargetPath ? backups.find((b) => b.path === detailTargetPath) ?? null : null;
  const restoreRecord = restoreTargetPath ? backups.find((b) => b.path === restoreTargetPath) ?? null : null;

  const nextHint = formatNextBackupHint(inst);
  const quotaSummary = `备份 ${quota.backups.used}/${quota.backups.max} 份 · 已用 ${quota.backupStorage.used}/${quota.backupStorage.max} GB`;
  const cardDescription = curPolicy?.enabled
    ? `${quotaSummary} · 每 ${curPolicy.intervalHours || 24} 小时自动备份（保留 ${curPolicy.retentionCount || 5} 份）${nextHint ? ` · ${nextHint}` : ""}`
    : quotaSummary;

  const liveLogTarget = logTarget ? backups.find((b) => b.path === logTarget.path) ?? logTarget : null;
  const logTaskId = liveLogTarget
    ? liveLogTarget.lastRestoreResult === "Running" || liveLogTarget.lastRestoreResult === "Failed"
      ? liveLogTarget.lastRestoreTaskID || liveLogTarget.taskID
      : liveLogTarget.taskID || liveLogTarget.lastRestoreTaskID
    : undefined;
  const logIsRestoring = liveLogTarget?.lastRestoreResult === "Running" || liveLogTarget?.phase === "Restoring";
  const logIsFailed =
    liveLogTarget?.phase === "Failed" ||
    liveLogTarget?.phase === "InspectFailed" ||
    liveLogTarget?.lastRestoreResult === "Failed";
  const logIsActive = Boolean(
    liveLogTarget && (!isTerminalBackupPhase(liveLogTarget.phase) || liveLogTarget.lastRestoreResult === "Running"),
  );
  const logProgress = liveLogTarget?.progress;
  const logDoneBytes = liveLogTarget?.processedBytes;
  const logTotalBytes = liveLogTarget?.totalBytes;
  const logSpeed = liveLogTarget?.speedBytesPerSec;
  const logByteSummary = (() => {
    if (logIsActive) {
      if (logTotalBytes && logTotalBytes > 0) {
        return `${formatTaskByteValue(logDoneBytes || 0)} / ${formatTaskByteValue(logTotalBytes)}`;
      }
      if (logDoneBytes && logDoneBytes > 0) {
        return `已处理 ${formatTaskByteValue(logDoneBytes)}`;
      }
      return undefined;
    }
    const finalBytes = liveLogTarget?.sizeBytes || logTotalBytes || logDoneBytes;
    return finalBytes && finalBytes > 0 ? formatTaskByteValue(finalBytes) : undefined;
  })();

  return (
    <>
      <Card>
        <CardHeader
          title="备份与恢复"
          description={cardDescription}
          actions={
            <div className="flex flex-wrap items-center gap-2">
              <Button
                size="sm"
                variant="secondary"
                icon={<Clock className="h-3.5 w-3.5" />}
                disabled={busy !== null}
                onClick={openPolicyModal}
              >
                {curPolicy?.enabled ? "自动备份：已开启" : "自动备份"}
              </Button>
              <Button
                size="sm"
                variant="secondary"
                icon={<Upload className="h-3.5 w-3.5" />}
                disabled={busy !== null || hasActiveBackupWork}
                onClick={() => {
                  setActionMode("import");
                  if (importFileInputRef.current) importFileInputRef.current.value = "";
                  setImportFile(null);
                  setImportLabel("");
                  setActionModalOpen(true);
                }}
              >
                上传本地备份
              </Button>
              <Button
                size="sm"
                variant="primary"
                icon={<DatabaseBackup className="h-3.5 w-3.5" />}
                disabled={busy !== null || hasActiveBackupWork}
                onClick={() => {
                  setActionMode("create");
                  setActionModalOpen(true);
                }}
              >
                新建备份
              </Button>
            </div>
          }
        />

        {backups.length === 0 ? (
          <EmptyState image={null} title="还没有备份" body="支持云端即时快照，或上传本地 .tar.gz / .zip 归档。" />
        ) : (
          <ul className="divide-y divide-[#EEF0F7]">
            {backups.map((b) => {
              const displayName = formatBackupDisplayName(b.path, name);
              const isRestoring = b.phase === "Restoring" || b.lastRestoreResult === "Running";
              const isRestoreFailed = !isRestoring && b.lastRestoreResult === "Failed";
              const isDeleting = b.phase === "Deleting";
              const isFailed = b.phase === "Failed";
              const isInspectFailed = b.phase === "InspectFailed";
              const isInspecting = b.phase === "Inspecting";
              const isBackingUp = Boolean(
                b.phase && b.phase !== "Completed" && !isDeleting && !isRestoring && !isFailed && !isInspectFailed && !isInspecting,
              );
              const isActive = isRestoring || isDeleting || isBackingUp || isInspecting;
              const isReady = (!b.phase || b.phase === "Completed") && !isRestoring;
              const isSaveUnconfirmed = isReady && b.saveConfirmation?.status === "Unconfirmed";
              const saveConfirmSummary = isReady ? formatBackupSaveConfirmationSummary(b.saveConfirmation) : null;

              const badgeTone = isDeleting
                ? "warning"
                : isFailed || isInspectFailed || isRestoreFailed
                  ? "danger"
                  : isSaveUnconfirmed
                    ? "warning"
                    : isActive
                      ? "accent"
                      : null;
              const badgeLabel = isDeleting
                ? "删除中"
                : isRestoring
                  ? "恢复中"
                  : isInspecting
                    ? "解析中"
                    : isBackingUp
                      ? b.imported
                        ? "上传中"
                        : "备份中"
                      : isInspectFailed
                        ? "解析失败"
                        : isFailed
                          ? "备份失败"
                          : isRestoreFailed
                            ? "恢复失败"
                            : isSaveUnconfirmed
                              ? saveConfirmSummary?.label || "已归档，存档未确认"
                              : null;

              const activeProgress = b.progress;
              const activeStage =
                b.message ||
                formatBackupStageLabel(b.stage, isRestoring) ||
                (isRestoring
                  ? "正在下载并恢复存档"
                  : isDeleting
                    ? "正在清理备份文件"
                    : isInspecting
                      ? "正在解析归档目录与游戏配置"
                      : "正在打包上传");
              const activeDone = b.processedBytes;
              const activeTotal = b.totalBytes;
              const activeSpeed = b.speedBytesPerSec;
              const byteHint = activeTotal
                ? `${formatTaskByteValue(activeDone || 0)} / ${formatTaskByteValue(activeTotal)}`
                : activeDone
                  ? formatTaskByteValue(activeDone)
                  : "";
              const speedHint = activeSpeed && activeSpeed > 0 ? `${formatTaskByteValue(activeSpeed)}/s` : "";
              const progressMeta = [byteHint, speedHint].filter(Boolean).join(" · ");
              const activeTaskId =
                isRestoring || isRestoreFailed ? b.lastRestoreTaskID || b.taskID : b.taskID || b.lastRestoreTaskID;
              const failureText = isRestoreFailed
                ? b.lastRestoreMessage || b.message
                : isFailed || isInspectFailed
                  ? b.message
                  : undefined;
              const rowMeta = buildBackupRowMeta(b, addonNoun);
              const mobileFallbackMeta = [
                b.sizeBytes ? formatBytes(b.sizeBytes) : "",
                relativeTime(b.completedAt),
              ]
                .filter(Boolean)
                .join(" · ");

              const overflowItems: MenuItem[] = [];
              if (isReady || isInspectFailed) {
                overflowItems.push({
                  label: "下载归档",
                  icon: <Download className="h-4 w-4" />,
                  disabled: busy !== null,
                  onSelect: () => void runDownload(b.path, displayName),
                });
              }
              if (activeTaskId) {
                overflowItems.push({
                  label: "查看日志",
                  icon: <Terminal className="h-4 w-4" />,
                  onSelect: () => setLogTarget(b),
                });
              }
              overflowItems.push({
                label: "删除备份",
                icon: <Trash2 className="h-4 w-4" />,
                danger: true,
                disabled: busy !== null || isDeleting || isRestoring || (b.phase === "Uploading" && !b.imported),
                onSelect: () => setConfirmDeleteTarget(b.path),
              });

              return (
                <li key={b.path} className="group flex items-center gap-3 px-4 py-4 hover:bg-[#F7F8FC]/60 sm:px-6">
                  {isActive ? (
                    <Loader2 className="h-4 w-4 shrink-0 animate-spin text-[#2B3AE8]" aria-hidden />
                  ) : (
                    <DatabaseBackup className="h-4 w-4 shrink-0 text-[#2B3AE8]" aria-hidden />
                  )}

                  <div className="min-w-0 flex-1">
                    <div className="flex min-w-0 items-center gap-2">
                      {isReady ? (
                        <button
                          type="button"
                          onClick={() => setDetailTargetPath(b.path)}
                          className={`min-w-0 truncate rounded text-left text-sm font-medium text-[#14161F] transition-colors hover:text-[#2B3AE8] ${focusRing}`}
                        >
                          {displayName}
                        </button>
                      ) : (
                        <span className="truncate text-sm font-medium text-[#14161F]">{displayName}</span>
                      )}
                      {badgeTone && badgeLabel ? (
                        <span className="shrink-0">
                          <Badge tone={badgeTone}>{badgeLabel}</Badge>
                        </span>
                      ) : b.imported ? (
                        <span className="shrink-0">
                          <Badge tone="neutral">导入</Badge>
                        </span>
                      ) : null}
                    </div>

                    {isActive ? (
                      <div className="mt-1.5 max-w-md space-y-1">
                        <div className="flex items-center justify-between gap-2 text-xs text-[#5B6075]">
                          <span className="truncate">{activeStage}</span>
                          <span className="shrink-0 tabular-nums">
                            {progressMeta
                              ? `${progressMeta}${typeof activeProgress === "number" && activeProgress > 0 ? ` · ${activeProgress}%` : ""}`
                              : typeof activeProgress === "number" && activeProgress > 0
                                ? `${activeProgress}%`
                                : "处理中"}
                          </span>
                        </div>
                        <progress
                          className="gp-progress gp-progress-accent h-1.5 w-full"
                          max={100}
                          {...(typeof activeProgress === "number" && activeProgress > 0
                            ? { value: Math.min(100, activeProgress) }
                            : {})}
                        />
                      </div>
                    ) : failureText ? (
                      <p className="mt-0.5 truncate text-xs text-[#B42318]">{failureText}</p>
                    ) : (
                      <>
                        {isSaveUnconfirmed && saveConfirmSummary?.warning && (
                          <p className="mt-0.5 truncate text-xs text-[#B54708]" title={saveConfirmSummary.warning}>
                            {saveConfirmSummary.warning}
                          </p>
                        )}
                        {(rowMeta || mobileFallbackMeta) && (
                          <p className="mt-0.5 truncate text-xs text-[#8A90A6] tabular-nums">
                            {rowMeta}
                            {mobileFallbackMeta && (
                              <span className="sm:hidden">
                                {rowMeta ? ` · ${mobileFallbackMeta}` : mobileFallbackMeta}
                              </span>
                            )}
                          </p>
                        )}
                      </>
                    )}
                  </div>

                  {!isActive && (
                    <>
                      <span className="hidden w-20 shrink-0 text-right text-[13px] tabular-nums text-[#8A90A6] sm:block">
                        {b.sizeBytes ? formatBytes(b.sizeBytes) : "-"}
                      </span>
                      <span className="hidden w-24 shrink-0 text-right text-[13px] text-[#8A90A6] md:block">
                        {relativeTime(b.completedAt)}
                      </span>
                    </>
                  )}

                  <div className="flex shrink-0 items-center gap-1">
                    {isReady && (
                      <>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="h-8 px-2.5 text-xs"
                          disabled={busy !== null}
                          onClick={() => setDetailTargetPath(b.path)}
                        >
                          详情
                        </Button>
                        <Button
                          size="sm"
                          variant="secondary"
                          className="h-8 px-3 text-xs"
                          disabled={busy !== null || hasActiveBackupWork}
                          onClick={() => setRestoreTargetPath(b.path)}
                        >
                          恢复
                        </Button>
                      </>
                    )}
                    <MoreMenu
                      label={`更多操作 (${displayName})`}
                      className="h-8 w-8 bg-transparent"
                      items={overflowItems}
                    />
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </Card>

      <Modal
        open={actionModalOpen}
        onClose={() => {
          if (busy !== "backup" && busy !== "import") setActionModalOpen(false);
        }}
        title={actionMode === "create" ? "新建备份" : "上传本地备份"}
        footer={
          <>
            <Button
              variant="ghost"
              disabled={busy === "backup" || busy === "import"}
              onClick={() => setActionModalOpen(false)}
            >
              取消
            </Button>
            {actionMode === "create" ? (
              <Button
                variant="primary"
                loading={busy === "backup"}
                onClick={(e) => void handleCreateSubmit(e as unknown as FormEvent)}
              >
                创建备份
              </Button>
            ) : (
              <Button
                variant="primary"
                loading={busy === "import"}
                disabled={!importFile || busy === "import"}
                onClick={(e) => void handleImportArchive(e as unknown as FormEvent)}
              >
                上传并导入
              </Button>
            )}
          </>
        }
      >
        <div className="space-y-4">
          <Segmented
            label="备份方式"
            value={actionMode}
            onChange={(v) => setActionMode(v)}
            items={[
              { value: "create", label: "创建云端备份" },
              { value: "import", label: "上传本地备份" },
            ]}
          />

          <div className="grid min-h-[168px]">
            <form
              className={
                actionMode === "create"
                  ? "col-start-1 row-start-1 space-y-3.5"
                  : "invisible pointer-events-none col-start-1 row-start-1 space-y-3.5"
              }
              aria-hidden={actionMode !== "create"}
              inert={actionMode !== "create"}
              onSubmit={(e) => void handleCreateSubmit(e)}
            >
              {isRunning && (
                <div className="flex items-center justify-between gap-3 rounded-xl bg-[#FFF8EB] px-3.5 py-2.5 text-xs text-[#7A4F01]">
                  <span className="flex min-w-0 items-center gap-2">
                    <AlertTriangle className="h-4 w-4 shrink-0 text-[#B76E00]" aria-hidden />
                    <span className="truncate font-medium text-[#5C3A00]">实例运行中 · 仅归档已落盘存档</span>
                  </span>
                  {onOpenConsole && (
                    <button
                      type="button"
                      onClick={() => {
                        setActionModalOpen(false);
                        onOpenConsole();
                      }}
                      className={`shrink-0 rounded font-medium text-[#B54708] hover:underline ${focusRing}`}
                    >
                      前往控制台
                    </button>
                  )}
                </div>
              )}

              <div className="grid grid-cols-3 gap-px overflow-hidden rounded-2xl bg-[#EEF0F7]">
                <div className="bg-[#F7F8FC] px-3.5 py-2.5" title={subtreeSummary}>
                  <div className="text-[11px] text-[#8A90A6]">存档目录</div>
                  <Mono className="mt-0.5 block truncate text-xs font-medium text-[#14161F]">
                    {subtreeSummary}
                  </Mono>
                </div>
                <div className="bg-[#F7F8FC] px-3.5 py-2.5">
                  <div className="text-[11px] text-[#8A90A6]">配置快照</div>
                  <div className="mt-0.5 truncate text-xs font-medium text-[#14161F] tabular-nums">{configSummary}</div>
                </div>
                <div className="bg-[#F7F8FC] px-3.5 py-2.5">
                  <div className="text-[11px] text-[#8A90A6]">{thirdCellLabel}</div>
                  <div className="mt-0.5 truncate text-xs font-medium text-[#14161F] tabular-nums">
                    {thirdCellValue}
                  </div>
                </div>
              </div>

              <Field label="备份备注" htmlFor="backup-label">
                <Input
                  id="backup-label"
                  placeholder="选填，留空按时间命名"
                  value={backupLabel}
                  onChange={(e) => setBackupLabel(e.target.value)}
                />
              </Field>
            </form>

            <form
              className={
                actionMode === "import"
                  ? "col-start-1 row-start-1 space-y-3.5"
                  : "invisible pointer-events-none col-start-1 row-start-1 space-y-3.5"
              }
              aria-hidden={actionMode !== "import"}
              inert={actionMode !== "import"}
              onSubmit={(e) => void handleImportArchive(e)}
            >
              <div>
                <input
                  ref={importFileInputRef}
                  id="import-archive-file"
                  type="file"
                  accept=".tar.gz,.tgz,.zip,application/gzip,application/x-gzip,application/zip"
                  className="hidden"
                  onChange={(e) => applySelectedImportFile(e.target.files?.[0] || null)}
                />
                <button
                  type="button"
                  onClick={() => importFileInputRef.current?.click()}
                  onDragOver={(e) => {
                    e.preventDefault();
                    setDragActive(true);
                  }}
                  onDragLeave={() => setDragActive(false)}
                  onDrop={handleDropImportFile}
                  className={`flex w-full items-center justify-between gap-3 rounded-2xl border border-dashed px-4 py-4 text-left transition-colors ${focusRing} ${
                    dragActive
                      ? "border-[#2B3AE8] bg-[#EEF1FF]/50"
                      : "border-[#CBD2E8] bg-[#FAFBFD] hover:border-[#2B3AE8]"
                  }`}
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-[#EEF0F7] text-[#2B3AE8]">
                      <Upload className="h-4 w-4" aria-hidden />
                    </div>
                    <div className="min-w-0">
                      {importFile ? (
                        <>
                          <div className="truncate text-sm font-medium text-[#14161F]">{importFile.name}</div>
                          <div className="mt-0.5 text-xs text-[#8A90A6] tabular-nums">{formatBytes(importFile.size)}</div>
                        </>
                      ) : (
                        <>
                          <div className="text-sm font-medium text-[#14161F]">选择或拖入本地备份包</div>
                          <div className="mt-0.5 flex items-center gap-1.5 text-xs text-[#8A90A6]">
                            <Mono>.tar.gz</Mono>
                            <span aria-hidden>·</span>
                            <Mono>.zip</Mono>
                          </div>
                        </>
                      )}
                    </div>
                  </div>
                  <span className="shrink-0 text-xs font-medium text-[#2B3AE8]">
                    {importFile ? "更换" : "浏览"}
                  </span>
                </button>
                {importUploadPct !== null && (
                  <div className="mt-2 space-y-1">
                    <div className="flex justify-between text-xs text-[#5B6075]">
                      <span>正在上传备份文件</span>
                      <span className="tabular-nums">{importUploadPct}%</span>
                    </div>
                    <progress className="gp-progress h-1.5 w-full" value={importUploadPct} max={100} />
                  </div>
                )}
              </div>

              <Field label="显示名称" htmlFor="import-backup-label">
                <Input
                  id="import-backup-label"
                  placeholder="选填，留空使用文件名"
                  value={importLabel}
                  onChange={(e) => setImportLabel(e.target.value)}
                />
              </Field>
            </form>
          </div>
        </div>
      </Modal>

      <Modal
        open={policyOpen}
        onClose={() => {
          if (busy !== "policy") setPolicyOpen(false);
        }}
        title="自动备份"
        footer={
          <>
            <Button variant="ghost" disabled={busy === "policy"} onClick={() => setPolicyOpen(false)}>
              取消
            </Button>
            <Button
              variant="primary"
              loading={busy === "policy"}
              disabled={!policyDirty || busy === "policy"}
              onClick={(e) => void handleSavePolicy(e as unknown as FormEvent)}
            >
              保存
            </Button>
          </>
        }
      >
        <form className="space-y-4" onSubmit={(e) => void handleSavePolicy(e)}>
          <div className="flex items-center justify-between rounded-xl border border-[#E4E7F0] bg-[#F7F8FC] px-4 py-3">
            <span className="text-sm font-medium text-[#14161F]">启用自动备份</span>
            <Switch checked={policyEnabled} onChange={setPolicyEnabled} label="启用自动备份" />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="备份周期" htmlFor="policy-interval">
              <Select
                id="policy-interval"
                value={policyInterval}
                disabled={!policyEnabled}
                onChange={(e) => setPolicyInterval(e.target.value)}
              >
                <option value="6">每 6 小时</option>
                <option value="12">每 12 小时</option>
                <option value="24">每 24 小时</option>
                <option value="72">每 3 天</option>
              </Select>
            </Field>
            <Field label="保留份数" htmlFor="policy-retention">
              <Select
                id="policy-retention"
                value={policyRetention}
                disabled={!policyEnabled}
                onChange={(e) => setPolicyRetention(e.target.value)}
              >
                <option value="3">最近 3 份</option>
                <option value="5">最近 5 份</option>
                <option value="7">最近 7 份</option>
                <option value="10">最近 10 份</option>
              </Select>
            </Field>
          </div>
        </form>
      </Modal>

      <BackupDetailModal
        key={detailRecord?.path || "closed"}
        open={!!detailRecord}
        backup={detailRecord}
        inst={inst}
        app={app}
        canRestore={!hasActiveBackupWork}
        busy={busy !== null}
        onClose={() => setDetailTargetPath(null)}
        onDownload={(path, label) => void runDownload(path, label)}
        onRequestRestore={(target) => {
          setDetailTargetPath(null);
          setRestoreTargetPath(target.path);
        }}
      />

      <BackupRestoreModal
        key={restoreRecord?.path || "closed"}
        open={!!restoreRecord}
        backup={restoreRecord}
        inst={inst}
        app={app}
        isRunning={isRunning}
        busy={busy === "restore"}
        onClose={() => setRestoreTargetPath(null)}
        onInspectDetail={(target) => {
          setRestoreTargetPath(null);
          setDetailTargetPath(target.path);
        }}
        onConfirmRestore={(path, opts) => void runArchive("restore", path, opts)}
      />

      <ConfirmDialog
        open={!!confirmDeleteTarget}
        title="确认删除此备份？"
        body={`将永久删除备份「${formatBackupDisplayName(confirmDeleteTarget || "", name)}」。`}
        confirmLabel="删除"
        danger
        loading={busy === "delete"}
        onConfirm={() => {
          if (confirmDeleteTarget) void runDelete(confirmDeleteTarget);
        }}
        onClose={() => setConfirmDeleteTarget(null)}
      />

      <TaskLogModal
        open={!!liveLogTarget && !!logTaskId}
        onClose={() => setLogTarget(null)}
        wsPath={
          logTaskId && tenant
            ? `/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenant)}/instances/${encodeURIComponent(name)}/log?taskID=${encodeURIComponent(logTaskId)}`
            : ""
        }
        token={token}
        title={liveLogTarget ? formatBackupDisplayName(liveLogTarget.path, name) : "备份任务"}
        badgeTone={logIsFailed ? "danger" : logIsActive ? "accent" : "success"}
        badgeLabel={
          logIsFailed
            ? "失败"
            : logIsRestoring
              ? "恢复中"
              : logIsActive
                ? liveLogTarget?.stage === "Inspecting"
                  ? "解析中"
                  : liveLogTarget?.stage === "Uploading"
                    ? "上传中"
                    : liveLogTarget?.stage === "Archiving" || liveLogTarget?.stage === "PreHook"
                      ? "打包中"
                      : "进行中"
                : "已完成"
        }
        message={
          logIsFailed
            ? liveLogTarget?.lastRestoreMessage || liveLogTarget?.message
            : logIsActive
              ? liveLogTarget?.message || formatBackupStageLabel(liveLogTarget?.stage, logIsRestoring)
              : liveLogTarget?.completedAt
                ? `完成于 ${relativeTime(liveLogTarget.completedAt)}`
                : undefined
        }
        progress={logProgress}
        isIndeterminate={logIsActive && !(typeof logProgress === "number" && logProgress > 0)}
        isActive={logIsActive}
        byteSummary={logByteSummary}
        speedBytesPerSec={logSpeed}
      />
    </>
  );
}
