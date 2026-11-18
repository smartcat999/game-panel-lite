"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, DatabaseBackup, Loader2, X } from "lucide-react";
import { formatBytes, relativeTime } from "../../../console/lib/format";
import type { AppItem, InstanceBackupRecord, InstanceItem } from "../../../console/lib/types";
import { Button, Field, Input, Modal, Select, focusRing } from "../ui";
import { useServerFiles, type BackupInspectResult, type RestoreArchiveOptions } from "./api";
import {
  filterManagedFilesForTarget,
  formatArchiveFormatLabel,
  formatBackupDisplayName,
  getLatestBackupInspect,
  parseRestoreTargetInput,
  resolveDefaultRestoreTargetPath,
  setCachedBackupInspect,
} from "./backupUtils";

export function BackupRestoreModal({
  open,
  backup,
  inst,
  app,
  isRunning,
  busy,
  onClose,
  onInspectDetail,
  onConfirmRestore,
}: {
  open: boolean;
  backup: InstanceBackupRecord | null;
  inst: InstanceItem;
  app?: AppItem;
  isRunning: boolean;
  busy: boolean;
  onClose: () => void;
  onInspectDetail: (backup: InstanceBackupRecord) => void;
  onConfirmRestore: (path: string, opts?: RestoreArchiveOptions) => void;
}) {
  const instanceName = inst.metadata.name;
  const { inspectBackup } = useServerFiles(instanceName);

  const fallbackTarget = useMemo(
    () => resolveDefaultRestoreTargetPath(inst, app?.storagePolicy?.backupSubtree),
    [inst, app?.storagePolicy?.backupSubtree],
  );

  const backupPath = backup?.path || "";
  const isImported = Boolean(backup?.imported);
  const revisionKey = `${inst.metadata.resourceVersion || ""}:${backup?.sha256 || ""}`;
  const initialCached =
    backupPath && isImported ? getLatestBackupInspect(instanceName, backupPath, revisionKey) : null;

  const [inspectLoading, setInspectLoading] = useState(() =>
    Boolean(open && backupPath && isImported && !initialCached),
  );
  const [inspectError, setInspectError] = useState<string | null>(null);
  const [inspectData, setInspectData] = useState<BackupInspectResult | null>(() => initialCached);
  const [restoreSource, setRestoreSource] = useState(() =>
    initialCached ? initialCached.sourcePath || initialCached.recommendedSource || "" : "",
  );
  const [restoreDest, setRestoreDest] = useState(() => initialCached?.targetPath || fallbackTarget);
  const [defaultRestoreTarget, setDefaultRestoreTarget] = useState(() => initialCached?.targetPath || fallbackTarget);
  const [reloadNonce, setReloadNonce] = useState(0);

  const destInputRef = useRef<HTMLInputElement>(null);
  const reqSeqRef = useRef(0);
  const mapSeqRef = useRef(0);
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    const clearTimer = () => {
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };

    if (!open || !backupPath) {
      reqSeqRef.current += 1;
      mapSeqRef.current += 1;
      clearTimer();
      return;
    }

    const seq = ++reqSeqRef.current;
    mapSeqRef.current += 1;
    const initialMapSeq = mapSeqRef.current;
    clearTimer();
    setInspectError(null);

    if (!isImported) {
      setInspectData(null);
      setRestoreSource("");
      setDefaultRestoreTarget(fallbackTarget);
      setRestoreDest(fallbackTarget);
      setInspectLoading(false);
      return;
    }

    const cached = reloadNonce === 0 ? getLatestBackupInspect(instanceName, backupPath, revisionKey) : null;
    if (!cached && reloadNonce === 0) {
      setInspectData(null);
      setRestoreSource("");
      setDefaultRestoreTarget(fallbackTarget);
      setRestoreDest(fallbackTarget);
      setInspectLoading(true);
    } else if (reloadNonce > 0) {
      setInspectData(null);
      setRestoreSource("");
      setDefaultRestoreTarget(fallbackTarget);
      setRestoreDest(fallbackTarget);
      setInspectLoading(true);
    } else {
      setInspectData((prev) => prev ?? cached);
      setInspectLoading(false);
    }

    const poll = async (attempt: number) => {
      try {
        const res = await inspectBackup(backupPath);
        if (reqSeqRef.current !== seq) return;
        if (res.phase === "Pending") {
          if (attempt >= 40) {
            setInspectError("备份解析超时，请稍后重试");
            setInspectLoading(false);
            return;
          }
          timerRef.current = window.setTimeout(() => {
            void poll(attempt + 1);
          }, 800);
          return;
        }
        setCachedBackupInspect(instanceName, backupPath, res, revisionKey);
        if (mapSeqRef.current === initialMapSeq) {
          setInspectData(res);
          const nextDest = res.targetPath || fallbackTarget;
          const nextSource = res.sourcePath || res.recommendedSource || "";
          setDefaultRestoreTarget(nextDest);
          if (!cached || reloadNonce > 0) {
            setRestoreSource(nextSource);
            setRestoreDest(nextDest);
          }
        }
        setInspectLoading(false);
      } catch (err) {
        if (reqSeqRef.current !== seq) return;
        setInspectError(err instanceof Error ? err.message : "识别备份目录结构失败");
        setInspectLoading(false);
      }
    };

    void poll(0);
    return () => {
      reqSeqRef.current += 1;
      mapSeqRef.current += 1;
      clearTimer();
    };
  }, [open, instanceName, backupPath, isImported, revisionKey, fallbackTarget, reloadNonce, inspectBackup]);

  const parsedDest = useMemo(() => parseRestoreTargetInput(restoreDest), [restoreDest]);

  const refreshMapping = useCallback(
    async (nextSource: string, nextDest: string) => {
      if (!backupPath) return;
      const modalSeq = reqSeqRef.current;
      const mapSeq = ++mapSeqRef.current;
      try {
        const res = await inspectBackup(backupPath, {
          sourcePath: nextSource || undefined,
          targetPath: nextDest || undefined,
        });
        if (reqSeqRef.current !== modalSeq || mapSeqRef.current !== mapSeq) return;
        if (res.phase === "Completed" || !res.phase) {
          setInspectData(res);
          if (nextSource === (res.recommendedSource || "") && nextDest === fallbackTarget) {
            setCachedBackupInspect(instanceName, backupPath, res, revisionKey);
          }
        }
      } catch {
        /* keep current selection even if preview refresh fails transiently */
      }
    },
    [instanceName, backupPath, revisionKey, fallbackTarget, inspectBackup],
  );

  useEffect(() => {
    if (!open || !backupPath || !isImported || inspectLoading || inspectError || !restoreSource) return;
    if (parsedDest.empty || parsedDest.error) return;
    const curSource = inspectData?.sourcePath ?? "";
    const curTarget = inspectData?.targetPath ?? ".";
    if (restoreSource === curSource && parsedDest.normalized === curTarget) return;

    const timer = window.setTimeout(() => {
      void refreshMapping(restoreSource, parsedDest.normalized);
    }, 220);
    return () => window.clearTimeout(timer);
  }, [
    open,
    backupPath,
    isImported,
    inspectLoading,
    inspectError,
    restoreSource,
    parsedDest.empty,
    parsedDest.error,
    parsedDest.normalized,
    inspectData?.sourcePath,
    inspectData?.targetPath,
    refreshMapping,
  ]);

  const restoreSourceOptions = useMemo(() => {
    const out: string[] = [];
    for (const item of [
      inspectData?.recommendedSource,
      ...(inspectData?.candidateSources || []),
      ".",
      ...(inspectData?.archiveDirectories || []),
    ]) {
      if (item && !out.includes(item)) out.push(item);
    }
    return out;
  }, [inspectData?.recommendedSource, inspectData?.candidateSources, inspectData?.archiveDirectories]);

  const activeTarget = parsedDest.empty || parsedDest.error ? defaultRestoreTarget : parsedDest.normalized;
  const effectiveManagedFiles = useMemo(
    () => filterManagedFilesForTarget(inst.spec.files, activeTarget, inspectData?.managedFiles || []),
    [inst.spec.files, activeTarget, inspectData?.managedFiles],
  );

  const displayName = backup ? formatBackupDisplayName(backup.path, instanceName) : "";
  const formatLabel = backup
    ? formatArchiveFormatLabel(backup.path, inspectData?.archiveFormat || backup.inspection?.archiveFormat)
    : "";
  const sizeBytes = backup?.sizeBytes || inspectData?.sizeBytes || 0;
  const fileCount = inspectData?.fileCount ?? backup?.inspection?.fileCount ?? 0;
  const completedRel = relativeTime(backup?.completedAt);
  const sourceMetaLine = [
    formatLabel,
    sizeBytes > 0 ? formatBytes(sizeBytes) : "",
    fileCount > 0 ? `${fileCount} 个文件` : completedRel,
  ]
    .filter(Boolean)
    .join(" · ");

  const mappingAmbiguous = (inspectData?.candidateSources?.length || 0) > 1 || !restoreSource;
  const hasManagedConfig = effectiveManagedFiles.length > 0;
  const isCustomTarget = restoreDest !== defaultRestoreTarget;
  const confirmDisabled =
    busy ||
    parsedDest.empty ||
    Boolean(parsedDest.error) ||
    (isImported && (inspectLoading || Boolean(inspectError) || !restoreSource));

  const subheaderNode = backup ? (
    <div className="flex items-center justify-between gap-3">
      <div className="flex min-w-0 items-center gap-2.5">
        <span
          className="inline-flex h-[34px] w-[34px] shrink-0 items-center justify-center rounded-full bg-[#EEF1FF] text-[#2B3AE8]"
          aria-hidden
        >
          <DatabaseBackup className="h-4 w-4" />
        </span>
        <div className="min-w-0">
          <div className="truncate text-[13px] font-medium text-[#14161F] tabular-nums">{displayName}</div>
          {sourceMetaLine && (
            <div className="mt-0.5 truncate text-xs text-[#8A90A6] tabular-nums">{sourceMetaLine}</div>
          )}
        </div>
      </div>
      <Button
        size="sm"
        variant="ghost"
        disabled={busy}
        className="h-7 shrink-0 px-2.5 text-xs text-[#2B3AE8] hover:text-[#2230D0]"
        onClick={() => {
          onClose();
          onInspectDetail(backup);
        }}
      >
        查看详情
      </Button>
    </div>
  ) : undefined;

  function handleConfirm() {
    if (!backup || confirmDisabled) return;
    if (!isImported) {
      if (parsedDest.normalized === defaultRestoreTarget) {
        onConfirmRestore(backup.path);
        return;
      }
      onConfirmRestore(backup.path, {
        sourcePath: defaultRestoreTarget,
        targetPath: parsedDest.normalized,
      });
      return;
    }
    onConfirmRestore(backup.path, {
      sourcePath: restoreSource,
      targetPath: parsedDest.normalized,
      replaceTarget: parsedDest.normalized === "." ? false : inspectData?.replaceTarget,
    });
  }

  return (
    <Modal
      open={open && !!backup}
      onClose={() => {
        if (!busy) onClose();
      }}
      title="恢复备份"
      width="max-w-[440px]"
      subheader={subheaderNode}
      footer={
        backup ? (
          <>
            <Button variant="ghost" disabled={busy} onClick={onClose}>
              取消
            </Button>
            <Button variant="danger-solid" loading={busy} disabled={confirmDisabled} onClick={handleConfirm}>
              确认恢复
            </Button>
          </>
        ) : undefined
      }
    >
      {backup && (
        <div className="space-y-4">
          {isImported && inspectLoading ? (
            <div className="flex items-center justify-center gap-2.5 py-6 text-xs text-[#5B6075]">
              <Loader2 className="h-4 w-4 animate-spin text-[#2B3AE8]" aria-hidden />
              <span>正在读取归档目录结构...</span>
            </div>
          ) : isImported && inspectError ? (
            <div className="flex items-center justify-between gap-3 rounded-xl bg-[#FEF3F2] px-3.5 py-3 text-xs text-[#B42318]">
              <span className="min-w-0 break-words">{inspectError}</span>
              <Button size="sm" variant="secondary" onClick={() => setReloadNonce((n) => n + 1)}>
                重试
              </Button>
            </div>
          ) : (
            <div className="space-y-4">
              {isImported && mappingAmbiguous && (
                <div className="flex items-center gap-2 rounded-xl bg-[#FFF8EB] px-3 py-2 text-xs text-[#7A4F01]">
                  <AlertTriangle className="h-3.5 w-3.5 shrink-0 text-[#B76E00]" aria-hidden />
                  <span>
                    {(inspectData?.candidateSources?.length || 0) > 1
                      ? "检测到多个候选目录 · 请确认归档源目录"
                      : "未匹配特征目录 · 请手动选择归档源目录"}
                  </span>
                </div>
              )}

              {isImported && (
                <Field label="归档源目录" htmlFor="restore-source-path">
                  <Select
                    id="restore-source-path"
                    className="h-10 text-[13px]"
                    value={restoreSource}
                    onChange={(e) => setRestoreSource(e.target.value)}
                  >
                    {!restoreSource && <option value="">请选择归档源目录...</option>}
                    {restoreSourceOptions.map((dir) => {
                      const isRec = Boolean(inspectData?.recommendedSource && dir === inspectData.recommendedSource);
                      const isCand = Boolean(inspectData?.candidateSources?.includes(dir));
                      const suffix = isRec ? "（推荐）" : isCand ? "（候选）" : "";
                      const label = dir === "." ? `归档根目录 (.)${suffix}` : `${dir}${suffix}`;
                      return (
                        <option key={dir} value={dir}>
                          {label}
                        </option>
                      );
                    })}
                  </Select>
                </Field>
              )}

              <div className="space-y-1.5">
                <div className="flex items-center justify-between gap-2">
                  <label htmlFor="restore-target-path" className="text-[13px] font-medium text-[#3D4258]">
                    实例目标路径
                  </label>
                  {isCustomTarget ? (
                    <button
                      type="button"
                      onClick={() => setRestoreDest(defaultRestoreTarget)}
                      className={`rounded-full px-1 text-xs font-medium text-[#2B3AE8] hover:underline ${focusRing}`}
                    >
                      恢复默认
                    </button>
                  ) : (
                    <span id="restore-target-hint" className="text-xs text-[#8A90A6]">
                      清空可手动输入
                    </span>
                  )}
                </div>

                <div className="relative">
                  <Input
                    ref={destInputRef}
                    id="restore-target-path"
                    name="targetPath"
                    value={restoreDest}
                    onChange={(e) => setRestoreDest(e.target.value)}
                    placeholder={defaultRestoreTarget === "." ? "." : `例如 ${defaultRestoreTarget}（根目录填 .）`}
                    autoCorrect="off"
                    autoCapitalize="none"
                    spellCheck={false}
                    autoComplete="off"
                    aria-invalid={parsedDest.error ? true : false}
                    aria-describedby={
                      parsedDest.error ? "restore-target-error" : !isCustomTarget ? "restore-target-hint" : undefined
                    }
                    className={`h-10 pr-9 font-mono text-[12.5px] ${
                      parsedDest.error ? "border-[#D92D20] focus:border-[#D92D20] focus:ring-[#D92D20]/15" : ""
                    }`}
                  />
                  {restoreDest.length > 0 && (
                    <button
                      type="button"
                      aria-label="清空实例目标路径"
                      title="清空路径"
                      onClick={() => {
                        setRestoreDest("");
                        destInputRef.current?.focus();
                      }}
                      className={`absolute right-2.5 top-1/2 inline-flex h-[22px] w-[22px] -translate-y-1/2 items-center justify-center rounded-full bg-[#EEF0F5] text-[#5B6075] transition-colors hover:bg-[#E3E6F2] hover:text-[#14161F] ${focusRing}`}
                    >
                      <X className="h-3 w-3" aria-hidden />
                    </button>
                  )}
                </div>

                {parsedDest.error && (
                  <p id="restore-target-error" className="text-[13px] text-[#B42318]">
                    {parsedDest.error}
                  </p>
                )}
              </div>

              <div className="border-t border-[#EEF0F7] pt-3 text-xs leading-relaxed text-[#5B6075]">
                将覆盖该路径下未备份进度 ·{" "}
                {isRunning ? "实例运行中将自动停服还原并重启" : "实例已停止直接还原"}
                {hasManagedConfig ? "，面板设置保持不变" : ""}
              </div>
            </div>
          )}
        </div>
      )}
    </Modal>
  );
}
