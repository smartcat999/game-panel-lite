"use client";

import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { Download, FileText, Folder, Package, SlidersHorizontal } from "lucide-react";
import { useConsole } from "../../../console/ConsoleProvider";
import { formatBytes, relativeTime } from "../../../console/lib/format";
import type { AppItem, InstanceBackupRecord, InstanceItem, RegistryPackage } from "../../../console/lib/types";
import { Badge, Button, Modal, Mono, Segmented, Select, Skeleton, focusRing } from "../ui";
import { useServerFiles, type BackupInspectResult } from "./api";
import { addonBindingMeta } from "./shared";
import {
  formatAddonScopeLabel,
  formatArchiveFormatLabel,
  formatBackupAddonVersion,
  formatBackupDateTime,
  formatBackupDisplayName,
  formatConfigSideValue,
  getLatestBackupInspect,
  setCachedBackupInspect,
} from "./backupUtils";

interface HydratedPackageMeta {
  title?: string;
  iconUrl?: string;
}

function normalizeHydrationPkgKey(pkg: string): string {
  return pkg.trim().replace(/^workshop-/i, "");
}

const hydratedPackageCache: Record<string, HydratedPackageMeta> = {};

function resolveInitialDetailTab(cached: BackupInspectResult | null): "config" | "addons" | "files" {
  if (!cached) return "config";
  if ((cached.configPreview?.length || 0) > 0) return "config";
  if ((cached.addonPreview?.length || 0) > 0) return "addons";
  if ((cached.sourceEntries?.length || 0) > 0) return "files";
  return "config";
}

function DetailAddonThumb({ src, title }: { src?: string; title: string }) {
  const [broken, setBroken] = useState(false);

  useEffect(() => {
    setBroken(false);
  }, [src]);

  if (!src || broken) {
    return (
      <div
        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-[#EEF0F7] text-[#5B6075]"
        aria-hidden
      >
        <Package className="h-4 w-4" />
      </div>
    );
  }

  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={src}
      alt={title}
      loading="lazy"
      fetchPriority="low"
      onError={() => setBroken(true)}
      className="h-9 w-9 shrink-0 rounded-xl bg-[#EEF0F7] object-cover"
    />
  );
}

export function BackupDetailModal({
  open,
  backup,
  inst,
  app,
  canRestore,
  busy,
  onClose,
  onDownload,
  onRequestRestore,
}: {
  open: boolean;
  backup: InstanceBackupRecord | null;
  inst: InstanceItem;
  app?: AppItem;
  canRestore: boolean;
  busy: boolean;
  onClose: () => void;
  onDownload: (path: string, displayName: string) => void;
  onRequestRestore: (backup: InstanceBackupRecord) => void;
}) {
  const { apiFetch } = useConsole();
  const instanceName = inst.metadata.name;
  const { inspectBackup } = useServerFiles(instanceName);

  const backupPath = backup?.path || "";
  const revisionKey = `${inst.metadata.resourceVersion || ""}:${backup?.sha256 || ""}`;
  const initialCached = backupPath ? getLatestBackupInspect(instanceName, backupPath, revisionKey) : null;

  const [activeTab, setActiveTab] = useState<"config" | "addons" | "files">(() =>
    resolveInitialDetailTab(initialCached),
  );
  const [onlyDiffConfig, setOnlyDiffConfig] = useState(false);
  const [configGroupFilter, setConfigGroupFilter] = useState<string>("__all__");
  const [inspectData, setInspectData] = useState<BackupInspectResult | null>(() => initialCached);
  const [inspectPending, setInspectPending] = useState<boolean>(() => Boolean(open && backupPath && !initialCached));
  const [showSkeleton, setShowSkeleton] = useState(false);
  const [inspectError, setInspectError] = useState<string | null>(null);
  const [selectedDir, setSelectedDir] = useState<string>(
    () => initialCached?.sourcePath || initialCached?.recommendedSource || ".",
  );
  const [fileEntries, setFileEntries] = useState<string[]>(() => initialCached?.sourceEntries || []);
  const [fileLoading, setFileLoading] = useState(false);
  const [reloadNonce, setReloadNonce] = useState(0);
  const [hydratedMap, setHydratedMap] = useState<Record<string, HydratedPackageMeta>>(() => ({
    ...hydratedPackageCache,
  }));

  const reqSeqRef = useRef(0);
  const dirSeqRef = useRef(0);
  const timerRef = useRef<number | null>(null);
  const pickedTabRef = useRef(Boolean(initialCached));
  const userPickedDirRef = useRef(false);

  const addonNoun = addonBindingMeta(app?.addonBinding).noun;

  useEffect(() => {
    if (!inspectPending) {
      setShowSkeleton(false);
      return;
    }
    const timer = window.setTimeout(() => setShowSkeleton(true), 120);
    return () => window.clearTimeout(timer);
  }, [inspectPending]);

  useEffect(() => {
    const clearTimer = () => {
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };

    if (!open || !backupPath) {
      reqSeqRef.current += 1;
      dirSeqRef.current += 1;
      clearTimer();
      return;
    }

    const seq = ++reqSeqRef.current;
    dirSeqRef.current += 1;
    clearTimer();
    const cached = reloadNonce === 0 ? getLatestBackupInspect(instanceName, backupPath, revisionKey) : null;
    if (!cached && reloadNonce === 0) {
      pickedTabRef.current = false;
      userPickedDirRef.current = false;
      setActiveTab("config");
      setOnlyDiffConfig(false);
      setConfigGroupFilter("__all__");
      setInspectData(null);
      setInspectError(null);
      setInspectPending(true);
      setSelectedDir(".");
      setFileEntries([]);
      setFileLoading(false);
    } else if (reloadNonce > 0) {
      setInspectError(null);
      setInspectPending(true);
    } else if (cached) {
      setInspectData((prev) => prev ?? cached);
      setInspectError(null);
      setInspectPending(false);
    }

    const poll = async (attempt: number) => {
      try {
        const res = await inspectBackup(backupPath);
        if (reqSeqRef.current !== seq) return;
        setInspectData(res);
        if (res.phase !== "Pending") {
          setCachedBackupInspect(instanceName, backupPath, res, revisionKey);
        }

        if (!userPickedDirRef.current) {
          const initialDir = res.sourcePath || res.recommendedSource || ".";
          setSelectedDir(initialDir);
          setFileEntries(res.sourceEntries || []);
        }

        if (
          !pickedTabRef.current &&
          (res.phase !== "Pending" || (res.configPreview?.length || 0) > 0 || (res.addonPreview?.length || 0) > 0)
        ) {
          pickedTabRef.current = true;
          setActiveTab(resolveInitialDetailTab(res));
        }

        if (res.phase === "Pending") {
          setInspectPending(true);
          if (attempt >= 40) {
            setInspectPending(false);
            setInspectError("归档索引超时，请稍后重试");
            return;
          }
          timerRef.current = window.setTimeout(() => {
            void poll(attempt + 1);
          }, 800);
          return;
        }

        setInspectPending(false);
      } catch (err) {
        if (reqSeqRef.current !== seq) return;
        setInspectPending(false);
        setInspectError(err instanceof Error ? err.message : "加载备份详情失败");
      }
    };

    void poll(0);
    return () => {
      reqSeqRef.current += 1;
      dirSeqRef.current += 1;
      clearTimer();
    };
  }, [open, instanceName, backupPath, revisionKey, reloadNonce, inspectBackup]);

  const addonPreview = useMemo(() => inspectData?.addonPreview ?? [], [inspectData?.addonPreview]);

  useEffect(() => {
    if (!open || addonPreview.length === 0) return;
    const grouped = new Map<string, Array<{ raw: string; norm: string }>>();
    for (const item of addonPreview) {
      const repo = (item.repository || app?.addonBinding?.repository || "").trim();
      const pkg = (item.package || "").trim();
      const normPkg = normalizeHydrationPkgKey(pkg);
      if (!repo || !pkg || !normPkg) continue;
      const cacheKey = `${repo}\u0000${normPkg}`;
      if (cacheKey in hydratedPackageCache) continue;
      const list = grouped.get(repo) || [];
      if (!list.some((x) => x.norm === normPkg)) list.push({ raw: pkg, norm: normPkg });
      grouped.set(repo, list);
    }
    if (grouped.size === 0) return;

    let cancelled = false;
    for (const [repo, list] of grouped.entries()) {
      for (let i = 0; i < list.length; i += 50) {
        const chunk = list.slice(i, i + 50);
        const params = new URLSearchParams({
          repository: repo,
          resolve: chunk.map((x) => x.raw).join(","),
        });
        void apiFetch(`/apis/registry.gamepanel.io/v1/packages?${params.toString()}`)
          .then(async (res) => {
            if (!res.ok || cancelled) return;
            const data = (await res.json().catch(() => ({}))) as { items?: RegistryPackage[] };
            if (cancelled) return;
            const updates: Record<string, HydratedPackageMeta> = {};
            for (const entry of chunk) {
              updates[`${repo}\u0000${entry.norm}`] = {};
            }
            for (const pkgItem of data.items ?? []) {
              const rawId = normalizeHydrationPkgKey(pkgItem.package || "");
              if (!rawId) continue;
              updates[`${repo}\u0000${rawId}`] = {
                title: pkgItem.title || undefined,
                iconUrl: pkgItem.iconUrl || undefined,
              };
            }
            Object.assign(hydratedPackageCache, updates);
            setHydratedMap((prev) => ({ ...prev, ...updates }));
          })
          .catch(() => {
            /* keep raw snapshot/archive metadata when package lookup is unavailable */
          });
      }
    }

    return () => {
      cancelled = true;
    };
  }, [open, addonPreview, app?.addonBinding?.repository, apiFetch]);

  async function handleSelectDir(nextDir: string) {
    if (!backupPath) return;
    const prevDir = selectedDir;
    userPickedDirRef.current = true;
    setSelectedDir(nextDir);
    const seq = ++dirSeqRef.current;
    setFileLoading(true);
    try {
      const res = await inspectBackup(backupPath, { sourcePath: nextDir });
      if (dirSeqRef.current !== seq) return;
      setFileEntries(res.sourceEntries || []);
    } catch {
      if (dirSeqRef.current === seq) setSelectedDir(prevDir);
    } finally {
      if (dirSeqRef.current === seq) setFileLoading(false);
    }
  }

  const configPreview = useMemo(() => inspectData?.configPreview ?? [], [inspectData?.configPreview]);
  const configGroups = useMemo(() => {
    const out: string[] = [];
    for (const item of configPreview) {
      if (item.group && !out.includes(item.group)) out.push(item.group);
    }
    return out;
  }, [configPreview]);
  const diffConfigCount = useMemo(() => configPreview.filter((i) => i.different).length, [configPreview]);
  const filteredConfigItems = useMemo(() => {
    const filtered = configPreview.filter((i) => {
      if (onlyDiffConfig && !i.different) return false;
      if (configGroupFilter !== "__all__" && configGroups.includes(configGroupFilter) && i.group !== configGroupFilter) {
        return false;
      }
      return true;
    });
    if (configGroups.length <= 1 || configGroupFilter !== "__all__") return filtered;
    const rank = new Map(configGroups.map((g, idx) => [g, idx]));
    return [...filtered].sort((a, b) => (rank.get(a.group || "") ?? 999) - (rank.get(b.group || "") ?? 999));
  }, [configPreview, onlyDiffConfig, configGroupFilter, configGroups]);

  const diffAddonCount = useMemo(() => addonPreview.filter((a) => a.different).length, [addonPreview]);
  const archiveDirectories = inspectData?.archiveDirectories ?? [];
  const showAddonsTab = Boolean(
    app?.addonBinding ||
      addonPreview.length > 0 ||
      (backup?.snapshot?.addonCount ?? 0) > 0 ||
      (backup?.inspection?.extractedAddons?.length ?? 0) > 0,
  );

  const tabItems = useMemo(() => {
    const items: Array<{ value: "config" | "addons" | "files"; label: string }> = [{ value: "config", label: "配置" }];
    if (showAddonsTab) items.push({ value: "addons", label: addonNoun });
    items.push({ value: "files", label: "文件" });
    return items;
  }, [showAddonsTab, addonNoun]);

  const displayName = backup ? formatBackupDisplayName(backup.path, instanceName) : "";
  const formatLabel = backup
    ? formatArchiveFormatLabel(backup.path, inspectData?.archiveFormat || backup.inspection?.archiveFormat)
    : "";
  const sizeBytes = backup?.sizeBytes || inspectData?.sizeBytes || 0;
  const fileCount = inspectData?.fileCount ?? backup?.inspection?.fileCount ?? 0;
  const completedAt = backup?.completedAt || inspectData?.completedAt;
  const serverVer = inspectData?.serverVersion || backup?.snapshot?.serverVersion;
  const currentVer =
    inspectData?.currentServerVersion ||
    inst.spec.serverArtifact?.version ||
    inst.metadata.annotations?.["core.gamepanel.io/app-version"];
  const versionChanged = Boolean(serverVer && currentVer && serverVer !== currentVer);

  const titleNode = backup ? (
    <span className="flex min-w-0 items-center gap-2.5">
      <span className="truncate text-[15px]">{displayName}</span>
      <span className="shrink-0">
        <Badge tone="neutral">
          {backup.imported ? "外部导入" : "云端备份"}
          {formatLabel ? ` · ${formatLabel}` : ""}
        </Badge>
      </span>
    </span>
  ) : (
    displayName
  );

  const descriptionNode = backup ? (
    <span className="mt-1 flex flex-wrap items-center gap-x-1.5 text-xs text-[#8A90A6] tabular-nums">
      <span title={relativeTime(completedAt)}>{formatBackupDateTime(completedAt)}</span>
      {sizeBytes > 0 && <span>· {formatBytes(sizeBytes)}</span>}
      {fileCount > 0 && <span>· {fileCount} 个文件</span>}
      {(serverVer || currentVer) && (
        <span>
          · 服务端 <Mono className="text-[#3D4258]">{serverVer || currentVer}</Mono>
          {versionChanged && <span className="ml-1 text-[#B54708]">（当前 {currentVer}）</span>}
        </span>
      )}
    </span>
  ) : undefined;

  const subheaderNode = backup ? (
    <div className="flex h-8 items-center justify-between gap-2.5 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
      <Segmented
        size="sm"
        label="备份详情视图"
        value={activeTab}
        onChange={(v) => setActiveTab(v)}
        items={tabItems}
      />

      {activeTab === "config" && configPreview.length > 0 && (
        <div className="flex shrink-0 items-center gap-2.5">
          <span
            className={
              diffConfigCount > 0
                ? "whitespace-nowrap text-xs font-medium text-[#B54708] tabular-nums"
                : "whitespace-nowrap text-xs text-[#8A90A6] tabular-nums"
            }
          >
            {diffConfigCount > 0 ? `${diffConfigCount} 项与当前不同` : "与当前配置一致"}
          </span>
          {configGroups.length > 1 && (
            <Select
              aria-label="筛选配置分组"
              className="h-7 w-auto min-w-[6.5rem] shrink-0 rounded-full px-3 pr-8 py-0 text-xs"
              value={configGroupFilter}
              onChange={(e) => setConfigGroupFilter(e.target.value)}
            >
              <option value="__all__">全部分组</option>
              {configGroups.map((group) => (
                <option key={group} value={group}>
                  {group}
                </option>
              ))}
            </Select>
          )}
          {diffConfigCount > 0 && diffConfigCount < configPreview.length && (
            <button
              type="button"
              aria-pressed={onlyDiffConfig}
              onClick={() => setOnlyDiffConfig((v) => !v)}
              className={`inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full border px-2.5 text-xs font-medium whitespace-nowrap transition-colors ${focusRing} ${
                onlyDiffConfig
                  ? "border-[#2B3AE8] bg-[#EEF1FF] text-[#2B3AE8]"
                  : "border-[#D5DAEE] bg-white text-[#3D4258] hover:border-[#2B3AE8] hover:text-[#2B3AE8]"
              }`}
            >
              <SlidersHorizontal className="h-3.5 w-3.5" aria-hidden />
              仅看差异
            </button>
          )}
        </div>
      )}

      {activeTab === "addons" && addonPreview.length > 0 && (
        <span className="text-xs text-[#5B6075] tabular-nums">
          共 {addonPreview.length} 个
          {diffAddonCount > 0 && <span className="font-medium text-[#B54708]"> · {diffAddonCount} 个与当前不同</span>}
        </span>
      )}

      {activeTab === "files" && archiveDirectories.length > 1 && (
        <Select
          aria-label="浏览归档目录"
          className="h-7 min-w-0 w-auto max-w-[19rem] truncate rounded-full px-3 pr-8 py-0 text-xs"
          value={selectedDir}
          onChange={(e) => void handleSelectDir(e.target.value)}
        >
          {archiveDirectories.map((dir) => {
            const recTag = inspectData?.recommendedSource && dir === inspectData.recommendedSource ? "（推荐）" : "";
            return (
              <option key={dir} value={dir}>
                {dir === "." ? `归档根目录 (.)${recTag}` : `${dir}${recTag}`}
              </option>
            );
          })}
        </Select>
      )}
    </div>
  ) : undefined;

  return (
    <Modal
      open={open && !!backup}
      onClose={onClose}
      width="max-w-[680px] h-[min(640px,88dvh)]"
      title={titleNode}
      description={descriptionNode}
      subheader={subheaderNode}
      footer={
        backup ? (
          <div className="flex w-full items-center justify-between gap-3">
            <Button
              variant="ghost"
              size="sm"
              icon={<Download className="h-4 w-4" />}
              disabled={busy}
              onClick={() => onDownload(backup.path, displayName)}
            >
              下载归档
            </Button>
            <div className="flex items-center gap-2">
              <Button variant="secondary" onClick={onClose}>
                关闭
              </Button>
              <Button
                variant="primary"
                disabled={!canRestore || busy}
                onClick={() => {
                  onClose();
                  onRequestRestore(backup);
                }}
              >
                恢复此备份
              </Button>
            </div>
          </div>
        ) : undefined
      }
    >
      {backup && (
        <div>
          {activeTab === "config" &&
            (inspectError && configPreview.length === 0 ? (
              <div className="flex items-center justify-between gap-3 rounded-xl bg-[#FEF3F2] px-4 py-3 text-xs text-[#B42318]">
                <span className="min-w-0 break-words">{inspectError}</span>
                <Button size="sm" variant="secondary" onClick={() => setReloadNonce((n) => n + 1)}>
                  重试
                </Button>
              </div>
            ) : inspectPending && configPreview.length === 0 ? (
              showSkeleton ? (
                <div className="divide-y divide-[#EEF0F7]">
                  <div className="flex items-center justify-between py-3">
                    <Skeleton className="h-4 w-24" />
                    <Skeleton className="h-4 w-40" />
                  </div>
                  <div className="flex items-center justify-between py-3">
                    <Skeleton className="h-4 w-24" />
                    <Skeleton className="h-4 w-32" />
                  </div>
                  <div className="flex items-center justify-between py-3">
                    <Skeleton className="h-4 w-24" />
                    <Skeleton className="h-4 w-28" />
                  </div>
                  <div className="flex items-center justify-between py-3">
                    <Skeleton className="h-4 w-24" />
                    <Skeleton className="h-4 w-20" />
                  </div>
                </div>
              ) : (
                <div className="min-h-[240px]" aria-busy="true" aria-label="正在加载备份配置" />
              )
            ) : configPreview.length === 0 ? (
              <div className="py-10 text-center text-xs text-[#8A90A6]">当前备份未包含可展示的结构化游戏配置</div>
            ) : filteredConfigItems.length === 0 ? (
              <div className="py-10 text-center text-xs text-[#8A90A6]">当前筛选条件下没有配置项</div>
            ) : (() => {
              const groupCounts = new Map<string, number>();
              for (const cfgItem of filteredConfigItems) {
                if (cfgItem.group) {
                  groupCounts.set(cfgItem.group, (groupCounts.get(cfgItem.group) ?? 0) + 1);
                }
              }
              return (
                <dl>
                  {filteredConfigItems.map((item, idx) => {
                    const arch = formatConfigSideValue(item, "archive");
                    const cur = formatConfigSideValue(item, "current");
                    const showGroupHeaders = configGroups.length > 1 && configGroupFilter === "__all__";
                    const showGroupHeader =
                      showGroupHeaders &&
                      Boolean(item.group) &&
                      (idx === 0 || filteredConfigItems[idx - 1]?.group !== item.group);
                    const nextGroup = idx < filteredConfigItems.length - 1 ? filteredConfigItems[idx + 1]?.group : "";
                    const endsBeforeNextGroup = showGroupHeaders && Boolean(nextGroup) && nextGroup !== item.group;
                    const showDivider = idx < filteredConfigItems.length - 1 && !endsBeforeNextGroup;
                    const diffBadgeText = item.overriddenByPanel ? `保留当前 · ${cur.text}` : `当前 · ${cur.text}`;
                    return (
                      <Fragment key={`${item.group || ""}:${item.name}`}>
                        {showGroupHeader && (
                          <div
                            className={`-mx-6 flex items-center justify-between border-y border-[#EEF0F7] bg-[#F7F8FC] px-6 py-1.5 text-xs font-semibold text-[#5B6075] ${
                              idx === 0 ? "-mt-4 border-t-0" : "mt-0"
                            }`}
                          >
                            <span>{item.group}</span>
                            <span className="font-normal text-[#8A90A6] tabular-nums">
                              {groupCounts.get(item.group ?? "") ?? 0} 项
                            </span>
                          </div>
                        )}
                        <div
                          className={`flex min-h-[34px] items-center justify-between gap-4 py-1.5 ${
                            showDivider ? "border-b border-[#EEF0F7]" : ""
                          }`}
                        >
                          <dt
                            className="max-w-[48%] shrink-0 text-[13px] font-medium leading-5 text-[#14161F]"
                            title={item.label}
                          >
                            {item.label}
                          </dt>
                          <dd className="flex min-w-0 flex-1 flex-wrap items-center justify-end gap-x-2 gap-y-1 text-right">
                            <span
                              title={arch.text}
                              className={
                                arch.muted
                                  ? "max-w-full break-words text-[13.5px] text-[#8A90A6] tabular-nums"
                                  : "max-w-full break-words text-[13.5px] font-medium text-[#14161F] tabular-nums"
                              }
                            >
                              {arch.text}
                            </span>
                            {item.different && (
                              <Badge tone="warning">
                                <span className="max-w-[360px] truncate tabular-nums" title={diffBadgeText}>
                                  {diffBadgeText}
                                </span>
                              </Badge>
                            )}
                          </dd>
                        </div>
                      </Fragment>
                    );
                  })}
                </dl>
              );
            })())}

          {activeTab === "addons" &&
            (inspectError && addonPreview.length === 0 ? (
              <div className="flex items-center justify-between gap-3 rounded-xl bg-[#FEF3F2] px-4 py-3 text-xs text-[#B42318]">
                <span className="min-w-0 break-words">{inspectError}</span>
                <Button size="sm" variant="secondary" onClick={() => setReloadNonce((n) => n + 1)}>
                  重试
                </Button>
              </div>
            ) : inspectPending && addonPreview.length === 0 ? (
              showSkeleton ? (
                <div className="divide-y divide-[#EEF0F7]">
                  <div className="py-3">
                    <Skeleton className="h-10" />
                  </div>
                  <div className="py-3">
                    <Skeleton className="h-10" />
                  </div>
                  <div className="py-3">
                    <Skeleton className="h-10" />
                  </div>
                </div>
              ) : (
                <div className="min-h-[240px]" aria-busy="true" aria-label={`正在加载${addonNoun}快照`} />
              )
            ) : addonPreview.length === 0 ? (
              <div className="py-10 text-center text-xs text-[#8A90A6]">{`备份时未检测到已安装的${addonNoun}`}</div>
            ) : (
              <ul className="-my-1.5 divide-y divide-[#EEF0F7]">
                {addonPreview.map((item) => {
                  const repo = (item.repository || app?.addonBinding?.repository || "").trim();
                  const hydrated = hydratedMap[`${repo}\u0000${normalizeHydrationPkgKey(item.package || "")}`];
                  const title = hydrated?.title || item.title || item.package;
                  const formattedVer = formatBackupAddonVersion(item.backupVersion);
                  const scopeText =
                    item.scopes && item.scopes.length > 0 ? item.scopes.map(formatAddonScopeLabel).join(" · ") : "";
                  const configText = item.configCount && item.configCount > 0 ? `${item.configCount} 项配置` : "";
                  const extraFacts = [formattedVer, scopeText, configText].filter(Boolean).join(" · ");

                  let statusNode = <span className="shrink-0 text-xs text-[#8A90A6]">已安装</span>;
                  if (item.backupDisabled) {
                    statusNode = <Badge tone="neutral">备份时停用</Badge>;
                  } else if (!item.installedNow) {
                    statusNode = <Badge tone="warning">当前未安装</Badge>;
                  } else if (item.currentDisabled) {
                    statusNode = <Badge tone="neutral">当前已停用</Badge>;
                  } else if (item.different) {
                    statusNode = <Badge tone="accent">版本已变更</Badge>;
                  }

                  return (
                    <li key={item.package} className="flex items-center gap-3 py-3">
                      <DetailAddonThumb src={hydrated?.iconUrl} title={title} />
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm font-medium text-[#14161F]">{title}</div>
                        <div className="mt-0.5 truncate text-xs text-[#8A90A6] tabular-nums">
                          <Mono>{item.package}</Mono>
                          {extraFacts ? ` · ${extraFacts}` : ""}
                        </div>
                      </div>
                      <div className="shrink-0">{statusNode}</div>
                    </li>
                  );
                })}
              </ul>
            ))}

          {activeTab === "files" &&
            (inspectError && fileEntries.length === 0 ? (
              <div className="flex items-center justify-between gap-3 rounded-xl bg-[#FEF3F2] px-4 py-3 text-xs text-[#B42318]">
                <span className="min-w-0 break-words">{inspectError}</span>
                <Button size="sm" variant="secondary" onClick={() => setReloadNonce((n) => n + 1)}>
                  重新索引
                </Button>
              </div>
            ) : (inspectPending && fileEntries.length === 0) || fileLoading ? (
              !fileLoading && !showSkeleton ? (
                <div className="min-h-[240px]" aria-busy="true" aria-label="正在加载归档文件列表" />
              ) : (
                <div className="divide-y divide-[#EEF0F7]">
                  <div className="py-2.5">
                    <Skeleton className="h-5" />
                  </div>
                  <div className="py-2.5">
                    <Skeleton className="h-5" />
                  </div>
                  <div className="py-2.5">
                    <Skeleton className="h-5" />
                  </div>
                  <div className="py-2.5">
                    <Skeleton className="h-5" />
                  </div>
                </div>
              )
            ) : fileEntries.length === 0 ? (
              <div className="py-10 text-center text-xs text-[#8A90A6]">当前目录下没有可预览的文件条目</div>
            ) : (
              <ul className="-my-1.5 divide-y divide-[#EEF0F7]">
                {fileEntries.map((entry) => {
                  const isDir = entry.endsWith("/");
                  return (
                    <li key={entry} className="flex items-center gap-2.5 py-2 text-xs text-[#343849]">
                      {isDir ? (
                        <Folder className="h-3.5 w-3.5 shrink-0 text-[#8A90A6]" aria-hidden />
                      ) : (
                        <FileText className="h-3.5 w-3.5 shrink-0 text-[#8A90A6]" aria-hidden />
                      )}
                      <Mono className="min-w-0 break-all text-[#14161F]">{entry}</Mono>
                    </li>
                  );
                })}
              </ul>
            ))}
        </div>
      )}
    </Modal>
  );
}
