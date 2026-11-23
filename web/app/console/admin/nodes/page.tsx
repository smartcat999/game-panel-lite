"use client";

import Link from "next/link";
import { FormEvent, useCallback, useEffect, useId, useMemo, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import {
  Activity,
  ArrowLeft,
  Check,
  Cpu,
  HardDriveDownload,
  LayoutGrid,
  List,
  MemoryStick,
  PauseCircle,
  Pencil,
  PlayCircle,
  RefreshCw,
  Search,
  Server,
  SlidersHorizontal,
  Terminal,
} from "lucide-react";
import { useConsole } from "../../ConsoleProvider";
import { ArtifactLogModal } from "../../../components/console/admin/ArtifactLogModal";
import { openFloatingToolbox } from "../../../components/console/FloatingToolbox";
import {
  endpointOf,
  formatBytes,
  formatCpu,
  formatGB,
  formatRateBytes,
  parseNumericQuantity,
  phaseOf,
  relativeTime,
} from "../../lib/format";
import { tenantServerRoute } from "../../lib/routes";
import { errorMessage } from "../../lib/session";
import { subscribeResourceWatch } from "../../lib/ws";
import type {
  AppItem,
  InstanceItem,
  KubeList,
  NodeItem,
  NodeMetricSample,
  PrewarmJobItem,
  PrewarmNodeStatus,
  RegistryArtifact,
} from "../../lib/types";
import { formatBytesCompact } from "../../../components/console/servers/InfoTabs";
import { gameOf } from "../../../components/console/servers/shared";
import {
  Badge,
  Button,
  Card,
  CardHeader,
  ConfirmDialog,
  CopyButton,
  EmptyState,
  Field,
  IconButton,
  Input,
  LinkButton,
  Meter,
  Modal,
  Mono,
  MoreMenu,
  PageHeader,
  Segmented,
  Select,
  Skeleton,
  StatusPill,
  Textarea,
  focusRing,
} from "../../../components/console/ui";

const NODE_VIEW_KEY = "gp_console_node_view";
type ViewMode = "cards" | "table";
type StatusFilter = "all" | "ready" | "paused";

function humanizePrewarmError(raw?: string): string {
  const s = (raw || "").toLowerCase();
  if (!s.trim()) return "节点预热缓存失败，请检查镜像站配置与节点网络";
  if (s.includes("timeout") || s.includes("deadline")) {
    return "节点下载超时，请在镜像站检查上游代理后重试";
  }
  if (s.includes("404") || s.includes("not found")) {
    return "镜像站中尚未缓存该资源，请先在镜像站完成同步";
  }
  if (s.includes("pull image") || s.includes("docker")) {
    return "基础运行环境拉取失败，请检查节点网络连通性";
  }
  if (s.includes("steamcmd")) {
    return "游戏安装包或模组拉取失败，请检查镜像站代理设置";
  }
  if (s.includes("proxy") || s.includes("connection refused")) {
    return "节点暂时无法连接镜像站，请检查节点网络状态";
  }
  return "节点预热缓存失败，请检查镜像站配置与节点网络";
}
type NodeDetailTab = "overview" | "cache" | "instances" | "settings";
type NodeCacheSubTab = "jobs" | "artifacts" | "images";

function resolveCachedRepoInfo(
  apps: AppItem[],
  repo: string,
): { appName: string; kindLabel: "游戏" | "模组" } | null {
  const cleanRepo = repo.trim();
  if (!cleanRepo) return null;
  for (const app of apps) {
    if (app.serverBinding?.repository === cleanRepo) {
      return { appName: app.displayName || app.metadata.name, kindLabel: "游戏" };
    }
    if (app.addonBinding?.repository === cleanRepo || (app.addonBinding?.additional || []).some((sub) => sub.repository === cleanRepo)) {
      return { appName: app.displayName || app.metadata.name, kindLabel: "模组" };
    }
  }
  return null;
}

function buildFallbackPrewarmArtifact(
  coord: string,
  repository: string,
  pkg: string,
  version: string,
  entry?: PrewarmNodeStatus,
): RegistryArtifact {
  return {
    metadata: { name: coord },
    spec: { repository, package: pkg, version: version || "latest" },
    status: entry
      ? {
          phase:
            entry.phase === "Succeeded"
              ? "Ready"
              : entry.phase === "Failed"
                ? "Failed"
                : entry.phase === "Running"
                  ? "Syncing"
                  : "Pending",
          message: entry.message,
        }
      : undefined,
  };
}

function resolvePrewarmLogArtifact(
  matchedArt: RegistryArtifact | undefined,
  fallback: RegistryArtifact | undefined,
  nodeEntry?: PrewarmNodeStatus,
): RegistryArtifact | null {
  const base = matchedArt || fallback || null;
  if (!base || !nodeEntry) return base;
  if (nodeEntry.phase === "Succeeded") return base;
  const regPhase = matchedArt?.status?.phase;
  if (matchedArt && (regPhase === "Pending" || regPhase === "Syncing" || regPhase === "Failed")) {
    return matchedArt;
  }
  const msg = nodeEntry.message?.trim();
  if (nodeEntry.phase === "Failed") {
    return {
      ...base,
      status: {
        ...base.status,
        phase: "Failed",
        stage: "Failed",
        message: msg || "节点预热失败",
      },
    };
  }
  if (nodeEntry.phase === "Running") {
    return {
      ...base,
      status: {
        ...base.status,
        phase: "Syncing",
        stage: "Downloading",
        progress: 0,
        processedBytes: 0,
        speedBytesPerSec: 0,
        message: msg || "节点正在从镜像站拉取并解压制品...",
      },
    };
  }
  return {
    ...base,
    status: {
      ...base.status,
      phase: "Pending",
      stage: "Queued",
      progress: 0,
      processedBytes: 0,
      speedBytesPerSec: 0,
      message: msg || "镜像站已就绪，等待当前节点前序预热任务完成后开始拉取...",
    },
  };
}

const LIGHTWEIGHT_APP_MAX_BYTES = 2 * 1024 * 1024 * 1024;

interface AppArtifactInfo {
  sizeBytes: number;
  cached: boolean;
  hasMods: boolean;
  pkg: string;
}

function resolveAppArtifactInfo(
  app: AppItem,
  registryArtifacts: RegistryArtifact[],
  artifactMap: Map<string, RegistryArtifact>,
  cachedCoords: string[],
): AppArtifactInfo {
  const repo = app.serverBinding?.repository || "";
  const pkg = app.serverBinding?.package || "";
  const hasMods = Boolean(app.addonBinding?.repository);
  if (!repo || !pkg) {
    return { sizeBytes: 0, cached: false, hasMods, pkg: "" };
  }
  const ver = app.recommendedVersion || app.versions?.[0] || "latest";
  const preferredCoord = `${repo}.${pkg}.${ver}`;
  const exact = artifactMap.get(preferredCoord) || artifactMap.get(`${repo}.${pkg}.latest`);
  const candidates = registryArtifacts.filter(
    (a) => a.spec.repository === repo && a.spec.package === pkg,
  );
  const matched =
    exact ||
    candidates.find(
      (a) => (a.status?.descriptor?.sizeBytes || a.spec.descriptor?.sizeBytes || a.status?.totalBytes || 0) > 0,
    ) ||
    candidates[0];
  const sizeBytes =
    matched?.status?.descriptor?.sizeBytes ||
    matched?.spec.descriptor?.sizeBytes ||
    matched?.status?.totalBytes ||
    0;
  const prefix = `${repo}.${pkg}.`;
  const cached = cachedCoords.some((coord) => coord === preferredCoord || coord.startsWith(prefix));
  return { sizeBytes, cached, hasMods, pkg };
}

function computeNodeFreeDiskBytes(node: NodeItem | null): number {
  if (!node) return 0;
  const capGiB = parseNumericQuantity(
    node.status?.capacity?.storage || node.status?.allocatable?.storage || "0",
  );
  const capBytes = capGiB > 0 ? capGiB * 1024 * 1024 * 1024 : 0;
  const usedBytes = Math.max(0, node.status?.usage?.storageBytes || 0);
  if (capBytes <= 0) return 0;
  return Math.max(0, capBytes - usedBytes);
}

function NodeScopePopoverButton({
  node,
  apps,
  registryArtifacts,
  artifactMap,
  variant = "toolbar",
  onSave,
}: {
  node: NodeItem;
  apps: AppItem[];
  registryArtifacts: RegistryArtifact[];
  artifactMap: Map<string, RegistryArtifact>;
  variant?: "toolbar" | "link";
  onSave: (node: NodeItem, nextAllowedApps: string[]) => Promise<boolean>;
}) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [coords, setCoords] = useState<{
    top?: number;
    bottom?: number;
    left: number;
    width: number;
  } | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  const selectable = useMemo(() => apps.filter((a) => a.published !== false), [apps]);
  const allIds = useMemo(() => selectable.map((a) => a.metadata.name), [selectable]);
  const cachedCoords = useMemo(
    () => node.status?.cachedArtifacts || [],
    [node.status?.cachedArtifacts],
  );

  const sortedInfos = useMemo(() => {
    const list = selectable.map((app) => ({
      app,
      ...resolveAppArtifactInfo(app, registryArtifacts, artifactMap, cachedCoords),
    }));
    return list.sort((a, b) => {
      const sa = a.sizeBytes > 0 ? a.sizeBytes : Number.MAX_SAFE_INTEGER;
      const sb = b.sizeBytes > 0 ? b.sizeBytes : Number.MAX_SAFE_INTEGER;
      return sa - sb;
    });
  }, [selectable, registryArtifacts, artifactMap, cachedCoords]);

  const savedAllowed = node.spec?.allowedApps || [];
  const activeCount =
    savedAllowed.length > 0 ? savedAllowed.length : selectable.length;
  const lightIds = useMemo(
    () =>
      sortedInfos
        .filter((i) => i.sizeBytes > 0 && i.sizeBytes <= LIGHTWEIGHT_APP_MAX_BYTES)
        .map((i) => i.app.metadata.name),
    [sortedInfos],
  );

  const updatePosition = useCallback(() => {
    const el = triggerRef.current;
    if (!el || typeof window === "undefined") return;
    const rect = el.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const panelW = Math.min(348, vw - 16);
    const panelH = 372;
    const spaceBelow = vh - rect.bottom;
    const openUpward = spaceBelow < panelH + 12 && rect.top > spaceBelow;
    const preferredLeft =
      variant === "link"
        ? Math.min(rect.left, vw - panelW - 8)
        : Math.max(8, rect.right - panelW);
    setCoords({
      ...(openUpward
        ? { bottom: Math.max(8, vh - rect.top + 8) }
        : { top: rect.bottom + 8 }),
      left: Math.max(8, preferredLeft),
      width: panelW,
    });
  }, [variant]);

  useEffect(() => {
    if (!open) return;
    updatePosition();
    const onDown = (e: MouseEvent) => {
      const target = e.target as Node;
      if (!triggerRef.current?.contains(target) && !panelRef.current?.contains(target)) {
        if (!saving) setOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !saving) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    window.addEventListener("scroll", updatePosition, true);
    window.addEventListener("resize", updatePosition);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
      window.removeEventListener("scroll", updatePosition, true);
      window.removeEventListener("resize", updatePosition);
    };
  }, [open, saving, updatePosition]);

  const handleToggleOpen = () => {
    if (open) {
      if (!saving) setOpen(false);
      return;
    }
    const orig = node.spec?.allowedApps || [];
    setSelected(orig.length > 0 ? orig : allIds);
    updatePosition();
    setOpen(true);
  };

  const toggleApp = (appName: string) => {
    setSelected((prev) =>
      prev.includes(appName) ? prev.filter((x) => x !== appName) : [...prev, appName],
    );
  };

  const estimatedBytes = sortedInfos
    .filter((i) => selected.includes(i.app.metadata.name))
    .reduce((sum, i) => sum + i.sizeBytes, 0);
  const freeDiskBytes = computeNodeFreeDiskBytes(node);
  const isAllSelected = allIds.length > 0 && selected.length === allIds.length;
  const nextAllowedApps = isAllSelected
    ? []
    : allIds.filter((id) => selected.includes(id));
  const origSorted = [...savedAllowed].sort().join(",");
  const nextSorted = [...nextAllowedApps].sort().join(",");
  const dirty = origSorted !== nextSorted;

  async function handleConfirm() {
    if (selected.length === 0 || !dirty) return;
    setSaving(true);
    try {
      const ok = await onSave(node, nextAllowedApps);
      if (ok) setOpen(false);
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      {variant === "link" ? (
        <button
          ref={triggerRef}
          type="button"
          aria-expanded={open}
          onClick={handleToggleOpen}
          className={`rounded text-xs font-medium text-[#2B3AE8] hover:underline ${focusRing}`}
        >
          配置范围
        </button>
      ) : (
        <button
          ref={triggerRef}
          type="button"
          aria-expanded={open}
          onClick={handleToggleOpen}
          className={`inline-flex h-8 items-center gap-2 rounded-full border px-3.5 text-xs font-medium transition-colors ${
            open
              ? "border-[#2B3AE8] bg-[#EEF1FE] text-[#1F2BD8]"
              : "border-[#E3E6F2] bg-white text-[#14161F] hover:border-[#CBD0E0] hover:bg-[#FAFBFD]"
          } ${focusRing}`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5 shrink-0 text-[#5B6075]" aria-hidden />
          <span>配置游戏范围</span>
          <span className="font-mono text-[#5B6075] tabular-nums">
            {activeCount}/{allIds.length}
          </span>
        </button>
      )}

      {open &&
        coords &&
        typeof document !== "undefined" &&
        createPortal(
          <div
            ref={panelRef}
            role="dialog"
            aria-label={`配置游戏范围 · ${node.metadata.name}`}
            style={{
              ...(coords.top !== undefined ? { top: coords.top } : {}),
              ...(coords.bottom !== undefined ? { bottom: coords.bottom } : {}),
              left: coords.left,
              width: coords.width,
              height: 372,
            }}
            className="fixed z-50 flex flex-col overflow-hidden rounded-2xl border border-[#E3E6F2] bg-white shadow-[0_20px_48px_-16px_rgba(20,22,31,0.22)]"
          >
            <div className="flex shrink-0 items-center justify-between gap-2 border-b border-[#EEF0F7] px-4 py-3">
              <span className="text-[13px] font-semibold text-[#14161F]">可调度与预热游戏</span>
              <span className="font-mono text-xs text-[#5B6075] tabular-nums">
                {estimatedBytes > 0 ? formatBytesCompact(estimatedBytes) : "0 MB"}
                {freeDiskBytes > 0 ? ` / ${formatBytesCompact(freeDiskBytes)}` : ""}
              </span>
            </div>

            <div className="flex shrink-0 items-center justify-between gap-2 border-b border-[#EEF0F7] bg-[#FAFBFD] px-4 py-2 text-xs">
              <span className="text-[#5B6075] tabular-nums">
                已选 {selected.length} / {sortedInfos.length}
              </span>
              <div className="flex items-center gap-2.5">
                <button
                  type="button"
                  onClick={() => setSelected(allIds)}
                  className="font-medium text-[#2B3AE8] hover:underline"
                >
                  全选
                </button>
                <button
                  type="button"
                  disabled={lightIds.length === 0}
                  title={
                    lightIds.length === 0
                      ? "镜像站同步游戏包大小后可用"
                      : "仅选择 2 GB 以内轻量游戏"
                  }
                  onClick={() => setSelected(lightIds)}
                  className="font-medium text-[#2B3AE8] hover:underline disabled:cursor-not-allowed disabled:text-[#A3A8BC] disabled:no-underline"
                >
                  轻量
                </button>
                <button
                  type="button"
                  onClick={() => setSelected([])}
                  className="text-[#5B6075] hover:text-[#14161F]"
                >
                  清空
                </button>
              </div>
            </div>

            <div className="flex-1 divide-y divide-[#F1F3F9] overflow-y-auto">
              {sortedInfos.map(({ app, sizeBytes, cached }) => {
                const checked = selected.includes(app.metadata.name);
                return (
                  <button
                    key={app.metadata.name}
                    type="button"
                    aria-pressed={checked}
                    onClick={() => toggleApp(app.metadata.name)}
                    className="flex h-[35px] w-full items-center justify-between gap-2.5 px-4 text-left text-xs transition-colors hover:bg-[#FAFBFD]"
                  >
                    <span className="flex min-w-0 items-center gap-2.5">
                      <span
                        className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border transition-colors ${
                          checked
                            ? "border-[#2B3AE8] bg-[#2B3AE8] text-white"
                            : "border-[#CBD0E0] bg-white"
                        }`}
                      >
                        {checked && <Check className="h-3 w-3" aria-hidden />}
                      </span>
                      <span
                        className={`truncate ${
                          checked ? "font-medium text-[#14161F]" : "text-[#5B6075]"
                        }`}
                      >
                        {app.displayName || app.metadata.name}
                      </span>
                      {cached && (
                        <span className="shrink-0 rounded bg-[#E7F6EC] px-1.5 py-0.5 text-[10px] font-medium leading-none text-[#1F7A45]">
                          已缓存
                        </span>
                      )}
                    </span>
                    <span className="shrink-0 font-mono text-[11px] text-[#8A90A6] tabular-nums">
                      {sizeBytes > 0 ? formatBytesCompact(sizeBytes) : "待同步"}
                    </span>
                  </button>
                );
              })}
            </div>

            <div className="flex shrink-0 items-center justify-between gap-2 border-t border-[#EEF0F7] bg-white px-4 py-2.5">
              {selected.length === 0 ? (
                <span className="text-xs text-[#B42318]">请至少选择 1 款游戏</span>
              ) : (
                <span />
              )}
              <div className="flex items-center gap-1.5">
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={saving}
                  onClick={() => setOpen(false)}
                >
                  取消
                </Button>
                <Button
                  size="sm"
                  variant="primary"
                  loading={saving}
                  disabled={!dirty || saving || selected.length === 0}
                  onClick={() => void handleConfirm()}
                >
                  保存
                </Button>
              </div>
            </div>
          </div>,
          document.body,
        )}
    </>
  );
}

function defaultNodeCacheSubTab(node: NodeItem, jobs: PrewarmJobItem[]): NodeCacheSubTab {
  const nodeName = node.metadata.name;
  const hasJob = jobs.some((job) => job.status?.nodes?.some((ns) => ns.nodeName === nodeName));
  if (hasJob) return "jobs";
  const hasArtifacts =
    (node.status?.cachedArtifacts?.length || 0) > 0 ||
    (node.status?.cachedBlobs?.length || 0) > 0;
  if (hasArtifacts) return "artifacts";
  if ((node.status?.warmImages?.length || 0) > 0) return "images";
  return "jobs";
}

const NODE_METRIC_WINDOWS: Array<{ value: "15m" | "1h" | "6h"; label: string }> = [
  { value: "15m", label: "15 分钟" },
  { value: "1h", label: "1 小时" },
  { value: "6h", label: "6 小时" },
];

function formatSampleClock(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleTimeString("zh-CN", {
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

const formatCpuCoresSample = (cores: number) => `${Math.max(0, cores).toFixed(2)} 核`;
const readNodeCpuCores = (s: NodeMetricSample) => (s.cpuMillicores || 0) / 1000;
const readNodeMemBytes = (s: NodeMetricSample) => s.memoryBytes || 0;
const readNodeDiskBytes = (s: NodeMetricSample) => s.storageBytes || 0;
const readNodeNetBytes = (s: NodeMetricSample) => (s.networkRxBytesPerSec || 0) + (s.networkTxBytesPerSec || 0);
const formatNodeNetSample = (s: NodeMetricSample) =>
  `↓ ${formatRateBytes(s.networkRxBytesPerSec)} · ↑ ${formatRateBytes(s.networkTxBytesPerSec)}`;

function NodeSparkline({
  samples,
  maxVal,
  color,
  valueOf,
  formatValue,
  formatSample,
  className = "",
}: {
  samples: NodeMetricSample[];
  maxVal: number;
  color: string;
  valueOf: (s: NodeMetricSample) => number;
  formatValue: (v: number) => string;
  formatSample?: (s: NodeMetricSample) => string;
  className?: string;
}) {
  const [hoverIdx, setHoverIdx] = useState<number | null>(null);
  const gradId = useId().replace(/:/g, "");
  const valueOfRef = useRef(valueOf);
  valueOfRef.current = valueOf;

  const w = 360;
  const h = 72;
  const topY = 6;
  const bottomY = 68;
  const plotH = bottomY - topY;

  const points = useMemo(() => {
    if (!samples.length) return [];
    const maxRenderPoints = 120;
    let displaySamples = samples;
    if (samples.length > maxRenderPoints) {
      const step = (samples.length - 1) / (maxRenderPoints - 1);
      displaySamples = [];
      for (let i = 0; i < maxRenderPoints - 1; i++) {
        displaySamples.push(samples[Math.round(i * step)]);
      }
      displaySamples.push(samples[samples.length - 1]);
    }
    const readVal = valueOfRef.current;
    const values = displaySamples.map((s) => Math.max(0, readVal(s)));
    const safeMax = Math.max(maxVal > 0 ? maxVal : 0, ...values, 1);
    return displaySamples.map((s, idx) => {
      const ratioX = displaySamples.length === 1 ? 0.5 : idx / (displaySamples.length - 1);
      const x = ratioX * w;
      const v = Math.max(0, readVal(s));
      const ratioY = Math.min(1, Math.max(0, v / safeMax));
      const y = bottomY - ratioY * plotH;
      return { x, y, ratioX, sample: s, value: v };
    });
  }, [samples, maxVal, plotH]);

  if (points.length === 0) {
    return (
      <div
        className={`flex h-[72px] w-full items-center justify-center rounded-lg bg-[#F7F8FC] text-xs text-[#8A90A6] ${className}`}
      >
        暂无采样数据
      </div>
    );
  }

  const linePath =
    points.length === 1
      ? `M 0 ${points[0].y.toFixed(1)} L ${w} ${points[0].y.toFixed(1)}`
      : points.map((p, idx) => `${idx === 0 ? "M" : "L"} ${p.x.toFixed(1)} ${p.y.toFixed(1)}`).join(" ");
  const areaPath = `${linePath} L ${w} ${bottomY} L 0 ${bottomY} Z`;
  const active = hoverIdx !== null && points[hoverIdx] ? points[hoverIdx] : null;

  return (
    <div
      className={`relative h-[72px] w-full select-none ${className}`}
      onMouseLeave={() => setHoverIdx(null)}
      onMouseMove={(e) => {
        const rect = e.currentTarget.getBoundingClientRect();
        if (rect.width <= 0) return;
        const relX = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
        setHoverIdx(Math.round(relX * (points.length - 1)));
      }}
    >
      <svg
        viewBox={`0 0 ${w} ${h}`}
        preserveAspectRatio="none"
        className="block h-[72px] w-full overflow-visible"
      >
        <defs>
          <linearGradient id={`node-spark-${gradId}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity={0.22} />
            <stop offset="100%" stopColor={color} stopOpacity={0.02} />
          </linearGradient>
        </defs>
        <line
          x1={0}
          y1={topY}
          x2={w}
          y2={topY}
          stroke="#DFE3F2"
          strokeWidth={1}
          strokeDasharray="3 3"
          vectorEffect="non-scaling-stroke"
        />
        <line
          x1={0}
          y1={bottomY - plotH * 0.75}
          x2={w}
          y2={bottomY - plotH * 0.75}
          stroke="#E6E9F4"
          strokeWidth={1}
          strokeDasharray="3 3"
          vectorEffect="non-scaling-stroke"
        />
        <line
          x1={0}
          y1={bottomY - plotH * 0.5}
          x2={w}
          y2={bottomY - plotH * 0.5}
          stroke="#E6E9F4"
          strokeWidth={1}
          strokeDasharray="3 3"
          vectorEffect="non-scaling-stroke"
        />
        <line
          x1={0}
          y1={bottomY - plotH * 0.25}
          x2={w}
          y2={bottomY - plotH * 0.25}
          stroke="#E6E9F4"
          strokeWidth={1}
          strokeDasharray="3 3"
          vectorEffect="non-scaling-stroke"
        />
        <line
          x1={0}
          y1={bottomY}
          x2={w}
          y2={bottomY}
          stroke="#DFE3F2"
          strokeWidth={1}
          vectorEffect="non-scaling-stroke"
        />
        <path d={areaPath} fill={`url(#node-spark-${gradId})`} />
        <path
          d={linePath}
          fill="none"
          stroke={color}
          strokeWidth={2}
          vectorEffect="non-scaling-stroke"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
      {active && (
        <>
          <div
            className="pointer-events-none absolute border-l border-dashed border-[#B8C0E6]"
            style={{
              left: `${(active.ratioX * 100).toFixed(2)}%`,
              top: `${topY}px`,
              height: `${plotH}px`,
            }}
          />
          <div
            className="pointer-events-none absolute h-2.5 w-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 bg-white shadow-xs"
            style={{
              left: `${(active.ratioX * 100).toFixed(2)}%`,
              top: `${active.y.toFixed(1)}px`,
              borderColor: color,
            }}
          />
          <div
            className={`pointer-events-none absolute -top-0.5 z-10 flex items-center gap-1.5 rounded-md border border-[#E3E6F2] bg-white/95 px-2 py-0.5 text-[11px] font-medium text-[#14161F] shadow-sm ${
              active.ratioX > 0.55 ? "left-0" : "right-0"
            }`}
          >
            <span className="font-mono text-[#8A90A6]">{formatSampleClock(active.sample.timestamp)}</span>
            <span className="font-mono font-semibold tabular-nums" style={{ color }}>
              {formatSample ? formatSample(active.sample) : formatValue(active.value)}
            </span>
          </div>
        </>
      )}
    </div>
  );
}

function addressOf(n: NodeItem, type: string) {
  return n.status?.addresses?.find((a) => a.type === type)?.address || "";
}

function NodeStatus({
  phase,
  paused,
  conditions,
}: {
  phase?: string;
  paused?: boolean;
  conditions?: NonNullable<NodeItem["status"]>["conditions"];
}) {
  const ready = (phase || "") === "Ready";
  const clockCond = !ready
    ? conditions?.find((c) => c.type === "ClockSynced" && c.status === "False")
    : undefined;
  const tone = !ready ? "danger" : paused ? "warning" : "success";
  const label = !ready
    ? clockCond
      ? "离线 · 时钟偏差"
      : "离线"
    : paused
      ? "暂停调度"
      : "在线";
  return (
    <span title={clockCond?.message}>
      <Badge tone={tone}>{label}</Badge>
    </span>
  );
}

function Stat({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="text-[13px] text-[#8A90A6]">{label}</div>
      <div className="mt-0.5 truncate text-sm text-[#14161F]">{children}</div>
    </div>
  );
}

interface NodeResourceBudget {
  capVal: number;
  allocVal: number;
  reservedVal: number;
  sysVal: number;
  remainVal: number;
  capText: string;
  allocText: string;
  reservedText: string;
  sysText: string;
  remainText: string;
  allocPct: number;
  sysPct: number;
  tight: boolean;
}

function formatCoresValue(cores: number): string {
  const rounded = Math.round(Math.max(0, cores) * 10) / 10;
  return `${Number.isInteger(rounded) ? rounded : rounded.toFixed(1)} 核`;
}

function formatGBValue(gib: number): string {
  const val = Math.max(0, gib);
  if (val >= 1024) {
    const tb = Math.round((val / 1024) * 10) / 10;
    return `${Number.isInteger(tb) ? tb : tb.toFixed(1)} TB`;
  }
  const rounded = Math.round(val * 10) / 10;
  return `${Number.isInteger(rounded) ? rounded : rounded.toFixed(1)} GB`;
}

function computeNodeResourceBudget(
  capRaw?: string,
  allocRaw?: string,
  reservedRaw?: string,
  unit: "cpu" | "gb" = "gb",
): NodeResourceBudget {
  const capVal = parseNumericQuantity(capRaw || allocRaw);
  const allocVal = allocRaw ? parseNumericQuantity(allocRaw) : capVal;
  const reservedVal = parseNumericQuantity(reservedRaw);
  const sysVal = Math.max(0, capVal - allocVal);
  const remainVal = Math.max(0, allocVal - reservedVal);
  const fmt = unit === "cpu" ? formatCoresValue : formatGBValue;
  const allocPct = capVal > 0 ? Math.min(100, Math.max(0, (reservedVal / capVal) * 100)) : 0;
  const sysPct =
    capVal > 0 && sysVal > 0.01 ? Math.min(100 - allocPct, Math.max(2, (sysVal / capVal) * 100)) : 0;
  const tight = allocVal > 0 && reservedVal > 0 && remainVal / allocVal < 0.1;

  return {
    capVal,
    allocVal,
    reservedVal,
    sysVal,
    remainVal,
    capText: fmt(capVal),
    allocText: fmt(allocVal),
    reservedText: fmt(reservedVal),
    sysText: fmt(sysVal),
    remainText: fmt(remainVal),
    allocPct,
    sysPct,
    tight,
  };
}

function formatBudgetSubline(budget: NodeResourceBudget): string {
  if (budget.capVal <= 0 || (budget.reservedVal <= 0 && budget.sysVal <= 0.01)) return "";
  return `分配 ${budget.reservedText} / ${budget.allocText}${
    budget.sysVal > 0.01 ? ` · 预留 ${budget.sysText}` : ""
  }`;
}

function NodeResourceBudgetStat({
  label,
  usedText,
  budget,
}: {
  label: string;
  usedText: string | null;
  budget: NodeResourceBudget;
}) {
  if (budget.capVal <= 0) {
    return <Stat label={label}>-</Stat>;
  }

  return (
    <div
      className="min-w-0"
      title={`实例已分配 ${budget.reservedText} / 可分配上限 ${budget.allocText} · 可调度余量 ${budget.remainText} · 系统预留 ${budget.sysText}`}
    >
      <div className="flex items-baseline justify-between gap-2">
        <span className="truncate text-[13px] text-[#8A90A6]">{label}</span>
        <span
          className={`shrink-0 text-xs tabular-nums ${
            budget.tight ? "font-medium text-[#D97706]" : "text-[#8A90A6]"
          }`}
        >
          余 {budget.remainText}
        </span>
      </div>
      <div className="mt-0.5 truncate text-sm font-semibold text-[#14161F] tabular-nums">
        {usedText || budget.allocText} / {budget.capText}
      </div>
      <div className="mt-1.5 flex h-1.5 w-full overflow-hidden rounded-full bg-[#EEF0F7]">
        {budget.allocPct > 0 && (
          <div
            className={`h-full rounded-l-full ${budget.tight ? "bg-[#D97706]" : "bg-[#2B3AE8]"}`}
            style={{ width: `${budget.allocPct.toFixed(1)}%` }}
          />
        )}
        <div className="flex-1" />
        {budget.sysPct > 0 && (
          <div
            className="h-full rounded-r-full border-l border-[#D5DAE8]"
            style={{
              width: `${budget.sysPct.toFixed(1)}%`,
              background:
                "repeating-linear-gradient(-45deg, rgba(138,144,166,0.38) 0 3px, rgba(138,144,166,0.12) 3px 6px)",
            }}
          />
        )}
      </div>
      <div className="mt-1 flex items-center justify-between gap-2 text-xs text-[#8A90A6] tabular-nums">
        <span className="truncate">
          分配 {budget.reservedText} / {budget.allocText}
        </span>
        <span className="shrink-0">预留 {budget.sysText}</span>
      </div>
    </div>
  );
}

function InfoList({ rows }: { rows: Array<[string, ReactNode]> }) {
  return (
    <dl className="divide-y divide-[#EEF0F7]">
      {rows.map(([k, v]) => (
        <div key={k} className="flex flex-col gap-1 px-6 py-4 sm:flex-row sm:items-center sm:gap-6">
          <dt className="w-32 shrink-0 text-[13px] text-[#8A90A6]">{k}</dt>
          <dd className="min-w-0 flex-1 break-words text-sm text-[#14161F]">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

function NodeLiveMeters({ node }: { node: NodeItem }) {
  const cap = node.status?.capacity || {};
  const usage = node.status?.usage;
  const ready = (node.status?.phase || "") === "Ready";

  const cpuCap = Math.max(1, parseNumericQuantity(cap.cpu || "1"));
  const memCapGB = Math.max(1, parseNumericQuantity(cap.memory || "1Gi"));
  const memCapBytes = memCapGB * 1024 * 1024 * 1024;

  const cpuUsed = usage?.cpuMillicores !== undefined ? usage.cpuMillicores / 1000 : 0;
  const memUsedBytes = usage?.memoryBytes !== undefined ? usage.memoryBytes : 0;

  const cpuPct = ready ? Math.min(100, Math.max(1, Math.round((cpuUsed / cpuCap) * 100))) : 0;
  const memPct = ready ? Math.min(100, Math.max(1, Math.round((memUsedBytes / memCapBytes) * 100))) : 0;

  const items = [
    {
      key: "cpu",
      icon: <Cpu className="h-3.5 w-3.5 shrink-0 text-[#8A90A6]" aria-hidden />,
      used: ready ? `${cpuUsed.toFixed(2)}` : "0",
      limit: formatCpu(cap.cpu),
      pct: cpuPct,
    },
    {
      key: "mem",
      icon: <MemoryStick className="h-3.5 w-3.5 shrink-0 text-[#8A90A6]" aria-hidden />,
      used: ready ? formatBytesCompact(memUsedBytes) : "0 MB",
      limit: formatGB(cap.memory),
      pct: memPct,
    },
  ];

  return (
    <div className="grid grid-cols-2 gap-3.5">
      {items.map((it) => (
        <Meter
          key={it.key}
          label={it.key === "cpu" ? "CPU 使用率" : "内存使用率"}
          used={it.pct}
          max={100}
          className="min-w-0"
          barClassName="mt-1.5"
          headerLeft={
            <span className="inline-flex min-w-0 items-center gap-1 whitespace-nowrap truncate text-[12px]">
              {it.icon}
              <span className="truncate font-medium tabular-nums text-[#14161F]">{it.used}</span>
            </span>
          }
          headerRight={<span className="shrink-0 whitespace-nowrap text-[11px] tabular-nums text-[#8A90A6]">/ {it.limit}</span>}
        />
      ))}
    </div>
  );
}

export default function AdminNodesPage() {
  const { nodes: ctxNodes, instances: ctxInstances, apps, token, loading, setTenant, apiFetch, toast, reload } =
    useConsole();

  const [liveNodes, setLiveNodes] = useState<NodeItem[]>([]);
  const [clusterInstances, setClusterInstances] = useState<InstanceItem[]>([]);
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [view, setView] = useState<ViewMode>("cards");

  const [selectedNodeName, setSelectedNodeName] = useState<string | null>(null);
  const [detailTab, setDetailTab] = useState<NodeDetailTab>("overview");
  const [cacheSubTab, setCacheSubTab] = useState<NodeCacheSubTab>("jobs");
  const cacheSubTabTouchedRef = useRef(false);
  const [urlHydrated, setUrlHydrated] = useState(false);
  const [prewarmJobs, setPrewarmJobs] = useState<PrewarmJobItem[]>([]);
  const [registryArtifacts, setRegistryArtifacts] = useState<RegistryArtifact[]>([]);
  const [prewarmLogTarget, setPrewarmLogTarget] = useState<{
    coord: string;
    title: string;
    fallback: RegistryArtifact;
    nodeEntry?: PrewarmNodeStatus;
  } | null>(null);
  const [retryingPrewarm, setRetryingPrewarm] = useState(false);

  const [prewarmOpen, setPrewarmOpen] = useState(false);
  const [prewarmNodeName, setPrewarmNodeName] = useState<string | null>(null);
  const [prewarmMode, setPrewarmMode] = useState<"app" | "image">("app");
  const [prewarmAppName, setPrewarmAppName] = useState("");
  const [prewarmVersion, setPrewarmVersion] = useState("");
  const [prewarmImage, setPrewarmImage] = useState("");
  const [prewarmBusy, setPrewarmBusy] = useState(false);

  const nodes = liveNodes.length > 0 ? liveNodes : ctxNodes;
  const instances = clusterInstances.length > 0 ? clusterInstances : ctxInstances;

  const loadClusterSnapshots = useCallback(async () => {
    try {
      const [nRes, pRes, iRes, aRes] = await Promise.all([
        apiFetch("/apis/core.gamepanel.io/v1/nodes?limit=200"),
        apiFetch("/apis/core.gamepanel.io/v1/prewarmjobs?limit=200"),
        apiFetch("/apis/core.gamepanel.io/v1/instances?limit=500"),
        apiFetch("/apis/registry.gamepanel.io/v1/artifacts?limit=500"),
      ]);
      if (nRes.ok) {
        const list = (await nRes.json()) as KubeList<NodeItem>;
        setLiveNodes((list.items || []).filter((x) => !x.metadata.deletionTimestamp));
      }
      if (pRes.ok) {
        const list = (await pRes.json()) as KubeList<PrewarmJobItem>;
        setPrewarmJobs((list.items || []).filter((j) => !j.metadata.deletionTimestamp));
      }
      if (iRes.ok) {
        const list = (await iRes.json()) as KubeList<InstanceItem>;
        setClusterInstances((list.items || []).filter((x) => !x.metadata.deletionTimestamp));
      }
      if (aRes.ok) {
        const list = (await aRes.json()) as KubeList<RegistryArtifact>;
        setRegistryArtifacts((list.items || []).filter((x) => !x.metadata.deletionTimestamp));
      }
    } catch {
      // Best-effort node live state hydration
    }
  }, [apiFetch]);

  useEffect(() => {
    void loadClusterSnapshots();
  }, [loadClusterSnapshots]);

  useEffect(() => {
    if (!token) return;
    const stopNodes = subscribeResourceWatch<NodeItem>({
      path: "/apis/core.gamepanel.io/v1/nodes",
      token,
      onReconnect: () => void loadClusterSnapshots(),
      onEvent: (ev) => {
        const obj = ev.object;
        const name = obj?.metadata?.name;
        if (!name) return;
        setLiveNodes((prev) => {
          if (ev.type === "DELETED") return prev.filter((x) => x.metadata.name !== name);
          const idx = prev.findIndex((x) => x.metadata.name === name);
          if (idx === -1) return [obj, ...prev];
          const next = [...prev];
          next[idx] = obj;
          return next;
        });
      },
    });
    const stopPrewarm = subscribeResourceWatch<PrewarmJobItem>({
      path: "/apis/core.gamepanel.io/v1/prewarmjobs",
      token,
      onEvent: (ev) => {
        const obj = ev.object;
        const name = obj?.metadata?.name;
        if (!name) return;
        setPrewarmJobs((prev) => {
          if (ev.type === "DELETED") return prev.filter((x) => x.metadata.name !== name);
          const idx = prev.findIndex((x) => x.metadata.name === name);
          if (idx === -1) return [obj, ...prev];
          const next = [...prev];
          next[idx] = obj;
          return next;
        });
      },
    });
    const stopInstances = subscribeResourceWatch<InstanceItem>({
      path: "/apis/core.gamepanel.io/v1/instances",
      token,
      onEvent: (ev) => {
        const obj = ev.object;
        const key = `${obj?.metadata?.tenant || ""}/${obj?.metadata?.name || ""}`;
        if (!obj?.metadata?.name) return;
        setClusterInstances((prev) => {
          if (ev.type === "DELETED") {
            return prev.filter((x) => `${x.metadata.tenant || ""}/${x.metadata.name}` !== key);
          }
          const idx = prev.findIndex((x) => `${x.metadata.tenant || ""}/${x.metadata.name}` === key);
          if (idx === -1) return [...prev, obj];
          const next = [...prev];
          next[idx] = obj;
          return next;
        });
      },
    });
    const stopArtifacts = subscribeResourceWatch<RegistryArtifact>({
      path: "/apis/registry.gamepanel.io/v1/artifacts",
      token,
      onEvent: (ev) => {
        const obj = ev.object;
        const name = obj?.metadata?.name;
        if (!name) return;
        setRegistryArtifacts((prev) => {
          if (ev.type === "DELETED") return prev.filter((x) => x.metadata.name !== name);
          const idx = prev.findIndex((x) => x.metadata.name === name);
          if (idx === -1) return [obj, ...prev];
          const next = [...prev];
          next[idx] = obj;
          return next;
        });
      },
    });
    return () => {
      stopNodes();
      stopPrewarm();
      stopInstances();
      stopArtifacts();
    };
  }, [token, loadClusterSnapshots]);

  function syncNodeUrl(nodeName: string | null, tab?: NodeDetailTab | null) {
    if (typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    if (!nodeName) {
      sp.delete("node");
      sp.delete("tab");
    } else {
      sp.set("node", nodeName);
      if (tab && tab !== "overview") sp.set("tab", tab);
      else sp.delete("tab");
    }
    const qs = sp.toString();
    const nextUrl = qs ? `${window.location.pathname}?${qs}` : window.location.pathname;
    window.history.replaceState(window.history.state, "", nextUrl);
  }

  const [toggleTarget, setToggleTarget] = useState<NodeItem | null>(null);
  const [toggleBusy, setToggleBusy] = useState(false);

  const [editTarget, setEditTarget] = useState<NodeItem | null>(null);
  const [editExternalIP, setEditExternalIP] = useState("");
  const [editLabelsText, setEditLabelsText] = useState("");
  const [savingEdit, setSavingEdit] = useState(false);

  const [metricsNode, setMetricsNode] = useState<NodeItem | null>(null);
  const [metricsWindow, setMetricsWindow] = useState<"15m" | "1h" | "6h">("15m");
  const [metricsSamples, setMetricsSamples] = useState<NodeMetricSample[]>([]);
  const [metricsLoading, setMetricsLoading] = useState(false);

  useEffect(() => {
    const saved = localStorage.getItem(NODE_VIEW_KEY);
    if (saved === "cards" || saved === "table") setView(saved);
    if (typeof window !== "undefined") {
      const sp = new URLSearchParams(window.location.search);
      const urlStatus = sp.get("status");
      if (urlStatus === "ready" || urlStatus === "paused") setStatusFilter(urlStatus);
      const urlView = sp.get("view");
      if (urlView === "cards" || urlView === "table") setView(urlView);
    }
  }, []);

  const changeStatusFilter = (next: StatusFilter) => {
    setStatusFilter(next);
    if (typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    if (next === "all") sp.delete("status");
    else sp.set("status", next);
    const qs = sp.toString();
    window.history.replaceState(
      window.history.state,
      "",
      qs ? `${window.location.pathname}?${qs}` : window.location.pathname,
    );
  };

  const changeView = (v: ViewMode) => {
    setView(v);
    localStorage.setItem(NODE_VIEW_KEY, v);
    if (typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    if (v === "cards") sp.delete("view");
    else sp.set("view", v);
    const qs = sp.toString();
    window.history.replaceState(
      window.history.state,
      "",
      qs ? `${window.location.pathname}?${qs}` : window.location.pathname,
    );
  };

  const artifactMap = useMemo(
    () => new Map(registryArtifacts.map((a) => [a.metadata.name, a])),
    [registryArtifacts],
  );

  const selectedNode = useMemo(
    () => nodes.find((n) => n.metadata.name === selectedNodeName) || null,
    [nodes, selectedNodeName],
  );

  const filteredNodes = useMemo(() => {
    const q = query.trim().toLowerCase();
    return nodes.filter((n) => {
      const paused = !!n.spec?.unschedulable;
      const ready = (n.status?.phase || "") === "Ready";
      if (statusFilter === "ready" && (!ready || paused)) return false;
      if (statusFilter === "paused" && !paused) return false;
      if (!q) return true;
      const internal = addressOf(n, "InternalIP").toLowerCase();
      const external = (addressOf(n, "ExternalIP") || n.spec?.externalIP || "").toLowerCase();
      return (
        n.metadata.name.toLowerCase().includes(q) ||
        internal.includes(q) ||
        external.includes(q)
      );
    });
  }, [nodes, query, statusFilter]);

  const loadNodeMetrics = useCallback(
    async (nodeName: string, win: "15m" | "1h" | "6h") => {
      setMetricsLoading(true);
      try {
        const res = await apiFetch(
          `/apis/core.gamepanel.io/v1/nodes/${encodeURIComponent(nodeName)}/metrics?window=${win}`,
        );
        if (!res.ok) return;
        const body = await res.json().catch(() => ({}));
        setMetricsSamples(Array.isArray(body?.samples) ? body.samples : []);
      } finally {
        setMetricsLoading(false);
      }
    },
    [apiFetch],
  );

  const activeMetricsNodeName =
    metricsNode?.metadata.name ||
    (selectedNode && detailTab === "overview" ? selectedNode.metadata.name : null);

  useEffect(() => {
    if (!activeMetricsNodeName) return;
    void loadNodeMetrics(activeMetricsNodeName, metricsWindow);
    const id = window.setInterval(() => {
      void loadNodeMetrics(activeMetricsNodeName, metricsWindow);
    }, 10000);
    return () => window.clearInterval(id);
  }, [activeMetricsNodeName, metricsWindow, loadNodeMetrics]);

  function populateEditFields(node: NodeItem) {
    const ext = node.spec?.externalIP || addressOf(node, "ExternalIP") || "";
    setEditExternalIP(ext);
    const labels = node.metadata.labels || {};
    const lines = Object.entries(labels)
      .map(([k, v]) => `${k}=${v}`)
      .join("\n");
    setEditLabelsText(lines);
  }

  function isNodeSettingsDirty(node: NodeItem | null): boolean {
    if (!node) return false;
    const origExt = (node.spec?.externalIP || addressOf(node, "ExternalIP") || "").trim();
    if (editExternalIP.trim() !== origExt) return true;
    const origLabels = node.metadata.labels || {};
    const origLines = Object.entries(origLabels)
      .map(([k, v]) => `${k.trim()}=${v.trim()}`)
      .sort()
      .join("\n");
    const curLines = editLabelsText
      .split("\n")
      .map((l) => l.trim())
      .filter(Boolean)
      .sort()
      .join("\n");
    return origLines !== curLines;
  }

  async function handleSaveNodeScope(
    targetNode: NodeItem,
    nextAllowedApps: string[],
  ): Promise<boolean> {
    const target =
      nodes.find((n) => n.metadata.name === targetNode.metadata.name) || targetNode;
    try {
      const res = await apiFetch(
        `/apis/core.gamepanel.io/v1/nodes/${encodeURIComponent(target.metadata.name)}`,
        {
          method: "PUT",
          body: JSON.stringify({
            ...target,
            spec: {
              ...target.spec,
              allowedApps: nextAllowedApps,
            },
          }),
        },
      );
      if (!res.ok) throw new Error(await errorMessage(res, "保存游戏调度范围失败"));
      toast("success", `节点 ${target.metadata.name} 的游戏调度与预热范围已更新`);
      await Promise.all([reload(), loadClusterSnapshots()]);
      return true;
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "保存游戏调度范围失败");
      return false;
    }
  }

  function openEditModal(node: NodeItem) {
    setEditTarget(node);
    populateEditFields(node);
  }

  function openNodeDetail(node: NodeItem, initialTab: NodeDetailTab = "overview") {
    cacheSubTabTouchedRef.current = false;
    setSelectedNodeName(node.metadata.name);
    setDetailTab(initialTab);
    setCacheSubTab(defaultNodeCacheSubTab(node, prewarmJobs));
    populateEditFields(node);
    syncNodeUrl(node.metadata.name, initialTab);
    if (typeof window !== "undefined") window.scrollTo({ top: 0, behavior: "smooth" });
  }

  useEffect(() => {
    if (!selectedNode) {
      cacheSubTabTouchedRef.current = false;
      return;
    }
    if (!cacheSubTabTouchedRef.current) {
      setCacheSubTab(defaultNodeCacheSubTab(selectedNode, prewarmJobs));
    }
  }, [selectedNode, prewarmJobs]);

  useEffect(() => {
    if (urlHydrated || nodes.length === 0 || typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    const urlNode = sp.get("node");
    const urlTab = sp.get("tab") as NodeDetailTab | null;
    setUrlHydrated(true);
    if (!urlNode) return;
    const matched = nodes.find((n) => n.metadata.name === urlNode);
    if (matched) {
      const validTab: NodeDetailTab =
        urlTab === "cache" || urlTab === "instances" || urlTab === "settings" ? urlTab : "overview";
      cacheSubTabTouchedRef.current = false;
      setSelectedNodeName(matched.metadata.name);
      setDetailTab(validTab);
      setCacheSubTab(defaultNodeCacheSubTab(matched, prewarmJobs));
      populateEditFields(matched);
    }
  }, [nodes, prewarmJobs, urlHydrated]);

  function openPrewarmModal(nodeName: string) {
    const targetNode = nodes.find((n) => n.metadata.name === nodeName);
    const allowedSet = targetNode?.spec?.allowedApps?.length
      ? new Set(targetNode.spec.allowedApps)
      : null;
    const candidateApps = allowedSet ? apps.filter((a) => allowedSet.has(a.metadata.name)) : apps;
    const firstApp =
      candidateApps.find((a) => Boolean(a.serverBinding?.repository && a.serverBinding?.package)) ||
      candidateApps[0];
    setPrewarmNodeName(nodeName);
    setPrewarmMode("app");
    setPrewarmAppName(firstApp?.metadata.name || "");
    setPrewarmVersion(firstApp?.recommendedVersion || firstApp?.versions?.[0] || "latest");
    setPrewarmImage(firstApp?.image || "");
    setPrewarmOpen(true);
  }

  async function handleTriggerPrewarm(e: FormEvent) {
    e.preventDefault();
    if (!prewarmNodeName) return;
    setPrewarmBusy(true);
    try {
      const targetApp = apps.find((a) => a.metadata.name === prewarmAppName);
      const spec: PrewarmJobItem["spec"] = { nodeNames: [prewarmNodeName] };
      if (prewarmMode === "app") {
        if (!targetApp?.serverBinding?.repository || !targetApp?.serverBinding?.package) {
          throw new Error("所选游戏暂无可预热的游戏安装包");
        }
        spec.artifacts = [
          {
            repository: targetApp.serverBinding.repository,
            package: targetApp.serverBinding.package,
            version: prewarmVersion.trim() || targetApp.recommendedVersion || "latest",
          },
        ];
        if (targetApp.image) spec.images = [targetApp.image];
      } else {
        const img = prewarmImage.trim();
        if (!img) throw new Error("请填写基础运行环境地址");
        spec.images = [img];
      }
      const jobName = `prewarm-${prewarmNodeName}-${Date.now().toString(36)}`.slice(0, 63);
      const res = await apiFetch("/apis/core.gamepanel.io/v1/prewarmjobs", {
        method: "POST",
        body: JSON.stringify({
          apiVersion: "core.gamepanel.io/v1",
          kind: "PrewarmJob",
          metadata: {
            name: jobName,
            ...(prewarmMode === "app" && targetApp
              ? { labels: { "gamepanel.io/app": targetApp.metadata.name } }
              : {}),
          },
          spec,
        }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "下发预热任务失败"));
      toast("success", `已向节点 ${prewarmNodeName} 下发预热任务`);
      setPrewarmOpen(false);
      await loadClusterSnapshots();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "下发预热任务失败");
    } finally {
      setPrewarmBusy(false);
    }
  }

  async function saveNodeConfiguration(target: NodeItem, closeModalAfter: boolean) {
    const nextLabels: Record<string, string> = {};
    for (const rawLine of editLabelsText.split("\n")) {
      const line = rawLine.trim();
      if (!line) continue;
      const eqIdx = line.indexOf("=");
      if (eqIdx <= 0) {
        toast("error", `标签格式错误：「${line}」，请使用 key=value 格式`);
        return;
      }
      const k = line.slice(0, eqIdx).trim();
      const v = line.slice(eqIdx + 1).trim();
      if (!k) {
        toast("error", `标签键不能为空：「${line}」`);
        return;
      }
      nextLabels[k] = v;
    }
    const latest = nodes.find((n) => n.metadata.name === target.metadata.name) || target;
    setSavingEdit(true);
    try {
      const extIP = editExternalIP.trim();
      const res = await apiFetch(`/apis/core.gamepanel.io/v1/nodes/${encodeURIComponent(latest.metadata.name)}`, {
        method: "PUT",
        body: JSON.stringify({
          ...latest,
          metadata: {
            ...latest.metadata,
            labels: nextLabels,
          },
          spec: {
            ...latest.spec,
            externalIP: extIP || undefined,
          },
        }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "保存节点配置失败"));
      toast("success", `节点 ${latest.metadata.name} 配置已更新`);
      if (closeModalAfter) setEditTarget(null);
      await Promise.all([reload(), loadClusterSnapshots()]);
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "保存节点配置失败");
    } finally {
      setSavingEdit(false);
    }
  }

  async function handleSaveNodeEdit(e: FormEvent) {
    e.preventDefault();
    if (!editTarget) return;
    await saveNodeConfiguration(editTarget, true);
  }

  async function handleSaveDetailSettings(e: FormEvent) {
    e.preventDefault();
    if (!selectedNode) return;
    await saveNodeConfiguration(selectedNode, false);
  }

  async function toggleSchedulable() {
    const node = toggleTarget;
    if (!node) return;
    const next = !node.spec?.unschedulable;
    setToggleBusy(true);
    try {
      const res = await apiFetch(`/apis/core.gamepanel.io/v1/nodes/${encodeURIComponent(node.metadata.name)}`, {
        method: "PUT",
        body: JSON.stringify({ ...node, spec: { ...node.spec, unschedulable: next } }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "更新节点调度状态失败，请稍后重试"));
      toast("success", `节点 ${node.metadata.name} 已${next ? "暂停调度" : "恢复调度"}`);
      setToggleTarget(null);
      await reload();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "更新节点调度状态失败");
    } finally {
      setToggleBusy(false);
    }
  }

  const pausing = !toggleTarget?.spec?.unschedulable;

  const readyCount = nodes.filter((n) => (n.status?.phase || "") === "Ready" && !n.spec?.unschedulable).length;

  return (
    <div>
      {!selectedNode ? (
        <>
          <PageHeader
            title="计算节点"
            description={
              nodes.length > 0
                ? `${readyCount} 台可调度 · 共 ${nodes.length} 台计算节点`
                : undefined
            }
          />

          {/* Top Single-Row Filter Bar */}
          {nodes.length > 0 && (
            <div className="mb-6 flex flex-wrap items-center gap-3">
              <Segmented<StatusFilter>
                label="按状态筛选"
                value={statusFilter}
                onChange={changeStatusFilter}
                items={[
                  { value: "all", label: "全部" },
                  { value: "ready", label: "可调度" },
                  { value: "paused", label: "暂停调度" },
                ]}
              />

              <div className="relative ml-auto w-full sm:w-64">
                <Search
                  className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[#8A90A6]"
                  aria-hidden
                />
                <Input
                  type="search"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="搜索节点名称或 IP 地址"
                  aria-label="搜索计算节点"
                  className="pl-9"
                />
              </div>

              <Segmented<ViewMode>
                label="显示方式"
                value={view}
                onChange={changeView}
                items={[
                  {
                    value: "cards",
                    label: (
                      <>
                        <LayoutGrid className="h-4 w-4" aria-hidden />
                        <span className="sr-only">卡片视图</span>
                      </>
                    ),
                  },
                  {
                    value: "table",
                    label: (
                      <>
                        <List className="h-4 w-4" aria-hidden />
                        <span className="sr-only">列表视图</span>
                      </>
                    ),
                  },
                ]}
              />
            </div>
          )}

          {loading && nodes.length === 0 ? (
            <div className="grid gap-4 lg:grid-cols-2">
              {[0, 1].map((i) => (
                <Skeleton key={i} className="h-60 rounded-[20px]" />
              ))}
            </div>
          ) : nodes.length === 0 ? (
            <Card>
              <EmptyState
                image="/landing/features/connect.jpg"
                title="还没有计算节点"
                body="等待本地计算节点服务启动并上报心跳后，将自动显示在此处。"
              />
            </Card>
          ) : (
            <>

              {filteredNodes.length === 0 ? (
                <Card>
                  <EmptyState
                    image={null}
                    title="没有符合条件的计算节点"
                    body="换个关键词，或者切回「全部」看看。"
                  />
                </Card>
              ) : view === "cards" ? (
                /* Original Node Card UI Preserved + Clickable Header to Enter Node Detail */
                <div className="grid gap-4 lg:grid-cols-2">
                  {filteredNodes.map((n) => {
                    const internal = addressOf(n, "InternalIP");
                    const external = addressOf(n, "ExternalIP") || n.spec?.externalIP || "";
                    const cap = n.status?.capacity || {};
                    const alloc = n.status?.allocatable || {};
                    const reserved = n.status?.reserved || {};
                    const usage = n.status?.usage;
                    const paused = !!n.spec?.unschedulable;
                    const hb = relativeTime(n.status?.lastHeartbeatTime);
                    const cpuUsedCores =
                      usage?.cpuMillicores !== undefined ? `${(usage.cpuMillicores / 1000).toFixed(2)} 核` : null;
                    const memUsedStr =
                      usage?.memoryBytes !== undefined && usage.memoryBytes > 0
                        ? formatBytes(usage.memoryBytes)
                        : null;
                    const diskUsedStr =
                      usage?.storageBytes !== undefined && usage.storageBytes > 0
                        ? formatBytes(usage.storageBytes)
                        : null;
                    const cpuBudget = computeNodeResourceBudget(cap.cpu, alloc.cpu, reserved.cpu, "cpu");
                    const memBudget = computeNodeResourceBudget(cap.memory, alloc.memory, reserved.memory, "gb");
                    const diskBudget = computeNodeResourceBudget(cap.storage, alloc.storage, reserved.storage, "gb");
                    return (
                      <Card
                        key={n.metadata.name}
                        className="flex flex-col transition-shadow hover:shadow-[0_20px_40px_-24px_rgba(43,58,232,0.45)]"
                      >
                        <div
                          role="button"
                          tabIndex={0}
                          onClick={() => openNodeDetail(n, "overview")}
                          onKeyDown={(e) => {
                            if (e.key === "Enter" || e.key === " ") {
                              e.preventDefault();
                              openNodeDetail(n, "overview");
                            }
                          }}
                          className="flex cursor-pointer flex-wrap items-center justify-between gap-3 border-b border-[#EEF0F7] px-6 py-5"
                        >
                          <div className="flex min-w-0 items-center gap-3">
                            <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-[#EEF1FF] text-[#2B3AE8]">
                              <Server className="h-5 w-5" />
                            </span>
                            <div className="min-w-0">
                              <h2 className="truncate text-base font-semibold text-[#14161F] hover:text-[#2B3AE8]">
                                {n.metadata.name}
                              </h2>
                              <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-[#8A90A6]">
                                <NodeStatus
                                  phase={n.status?.phase}
                                  paused={paused}
                                  conditions={n.status?.conditions}
                                />
                                <span>
                                  {instances.filter((i) => i.spec?.nodeName === n.metadata.name).length} 台服务器
                                </span>
                                {n.spec?.allowedApps && n.spec.allowedApps.length > 0 && (
                                  <Badge tone="accent">限定 {n.spec.allowedApps.length} 款游戏</Badge>
                                )}
                              </div>
                            </div>
                          </div>
                          <div className="flex flex-wrap items-center gap-3 text-xs text-[#8A90A6]">
                            <span className="inline-flex items-center gap-1.5">
                              <span className="inline-block h-2 w-2 rounded-sm bg-[#2B3AE8]" aria-hidden />
                              实例已分配
                            </span>
                            <span className="inline-flex items-center gap-1.5">
                              <span
                                className="inline-block h-2 w-2 rounded-sm border border-[#D5DAE8] bg-[#EEF0F7]"
                                aria-hidden
                              />
                              可调度余量
                            </span>
                            <span className="inline-flex items-center gap-1.5">
                              <span
                                className="inline-block h-2 w-2 rounded-sm border border-[#D5DAE8]"
                                style={{
                                  background:
                                    "repeating-linear-gradient(-45deg, rgba(138,144,166,0.42) 0 2px, rgba(138,144,166,0.14) 2px 4px)",
                                }}
                                aria-hidden
                              />
                              系统预留
                            </span>
                          </div>
                        </div>

                        <div
                          role="button"
                          tabIndex={-1}
                          onClick={() => openNodeDetail(n, "overview")}
                          className="grid cursor-pointer grid-cols-2 gap-x-6 gap-y-4 px-6 py-5"
                        >
                          <Stat label="内网地址">{internal ? <Mono>{internal}</Mono> : "-"}</Stat>
                          <Stat label="公网地址">{external ? <Mono>{external}</Mono> : "-"}</Stat>
                          <NodeResourceBudgetStat
                            label="CPU（已用 / 总量）"
                            usedText={cpuUsedCores}
                            budget={cpuBudget}
                          />
                          <NodeResourceBudgetStat
                            label="内存（已用 / 总量）"
                            usedText={memUsedStr}
                            budget={memBudget}
                          />
                          <NodeResourceBudgetStat
                            label="存储（已用 / 总量）"
                            usedText={diskUsedStr}
                            budget={diskBudget}
                          />
                          <Stat label="最近心跳">{hb ? `${hb}上报` : "等待首次上报"}</Stat>
                        </div>

                        <div className="mt-auto flex flex-wrap items-center justify-end gap-2 border-t border-[#EEF0F7] px-6 py-3.5">
                          <div className="flex flex-wrap items-center gap-1.5">
                            <IconButton
                              label={`编辑节点配置 ${n.metadata.name}`}
                              onClick={() => openEditModal(n)}
                            >
                              <Pencil className="h-4 w-4" aria-hidden />
                            </IconButton>
                            <IconButton
                              label={`打开节点终端 ${n.metadata.name}`}
                              onClick={() => openFloatingToolbox({ tool: "terminal", node: n.metadata.name })}
                            >
                              <Terminal className="h-4 w-4" aria-hidden />
                            </IconButton>
                            <IconButton
                              label={`查看节点监控 ${n.metadata.name}`}
                              onClick={() => setMetricsNode(n)}
                            >
                              <Activity className="h-4 w-4" aria-hidden />
                            </IconButton>
                            <IconButton
                              label={`${paused ? "恢复调度" : "暂停调度"} ${n.metadata.name}`}
                              onClick={() => setToggleTarget(n)}
                            >
                              {paused ? (
                                <PlayCircle className="h-4 w-4" aria-hidden />
                              ) : (
                                <PauseCircle className="h-4 w-4" aria-hidden />
                              )}
                            </IconButton>
                          </div>
                        </div>
                      </Card>
                    );
                  })}
                </div>
              ) : (
                /* Table View (matching ServerTable in ServerViews.tsx) */
                <Card className="overflow-visible">
                  <div tabIndex={0} role="region" aria-label="计算节点列表" className="overflow-x-auto">
                    <table className="w-full min-w-[720px] text-left text-sm">
                      <thead>
                        <tr className="border-b border-[#EEF0F7] text-[13px] text-[#8A90A6]">
                          <th scope="col" className="px-6 py-3.5 font-medium">节点名称</th>
                          <th scope="col" className="px-4 py-3.5 font-medium">状态</th>
                          <th scope="col" className="px-4 py-3.5 font-medium">节点地址</th>
                          <th scope="col" className="w-80 min-w-[280px] px-4 py-3.5 font-medium">资源使用</th>
                          <th scope="col" className="px-6 py-3.5 text-right font-medium">操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {filteredNodes.map((n) => {
                          const internal = addressOf(n, "InternalIP");
                          const external = addressOf(n, "ExternalIP") || n.spec?.externalIP || "";
                          const addr = external || internal;
                          const paused = !!n.spec?.unschedulable;
                          const hostedCount = instances.filter(
                            (i) => i.spec?.nodeName === n.metadata.name,
                          ).length;

                          return (
                            <tr
                              key={n.metadata.name}
                              onClick={() => openNodeDetail(n, "overview")}
                              className="cursor-pointer border-b border-[#EEF0F7] transition-colors last:border-0 hover:bg-[#F6F8FF]"
                            >
                              <td className="px-6 py-4">
                                <button
                                  type="button"
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    openNodeDetail(n, "overview");
                                  }}
                                  className={`text-left font-semibold text-[#14161F] hover:text-[#2B3AE8] ${focusRing} rounded`}
                                >
                                  {n.metadata.name}
                                </button>
                                <div className="mt-0.5 text-xs text-[#8A90A6]">
                                  运行 {hostedCount} 台服务器
                                </div>
                              </td>
                              <td className="px-4 py-4">
                                <NodeStatus
                                  phase={n.status?.phase}
                                  paused={paused}
                                  conditions={n.status?.conditions}
                                />
                              </td>
                              <td className="px-4 py-4" onClick={(e) => e.stopPropagation()}>
                                {addr ? (
                                  <span className="inline-flex items-center gap-0.5">
                                    <Mono className="text-[#14161F]">{addr}</Mono>
                                    <CopyButton text={addr} label="复制节点地址" />
                                  </span>
                                ) : (
                                  <span className="text-[#8A90A6]">-</span>
                                )}
                              </td>
                              <td className="px-4 py-4">
                                <NodeLiveMeters node={n} />
                              </td>
                              <td className="px-6 py-4 text-right" onClick={(e) => e.stopPropagation()}>
                                <div className="inline-flex items-center justify-end gap-1.5">
                                  <MoreMenu
                                    label={`更多操作 (${n.metadata.name})`}
                                    items={[
                                      {
                                        label: "节点终端",
                                        icon: <Terminal className="h-4 w-4" aria-hidden />,
                                        onSelect: () =>
                                          openFloatingToolbox({ tool: "terminal", node: n.metadata.name }),
                                      },
                                      {
                                        label: "查看监控",
                                        icon: <Activity className="h-4 w-4" aria-hidden />,
                                        onSelect: () => setMetricsNode(n),
                                      },
                                      {
                                        label: paused ? "恢复调度" : "暂停调度",
                                        icon: paused ? (
                                          <PlayCircle className="h-4 w-4 text-[#2B3AE8]" aria-hidden />
                                        ) : (
                                          <PauseCircle className="h-4 w-4" aria-hidden />
                                        ),
                                        onSelect: () => setToggleTarget(n),
                                      },
                                      {
                                        label: "编辑配置",
                                        icon: <Pencil className="h-4 w-4" aria-hidden />,
                                        onSelect: () => openEditModal(n),
                                      },
                                    ]}
                                  />
                                </div>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                </Card>
              )}
            </>
          )}
        </>
      ) : (
        /* Node Detail View */
        (() => {
          const nodeName = selectedNode.metadata.name;
          const internal = addressOf(selectedNode, "InternalIP");
          const external = addressOf(selectedNode, "ExternalIP") || selectedNode.spec?.externalIP || "";
          const paused = !!selectedNode.spec?.unschedulable;
          const nodeInstances = instances.filter(
            (i) => i.spec?.nodeName === nodeName,
          );
          const cachedArtifacts = selectedNode.status?.cachedArtifacts || [];
          const cachedBlobs = selectedNode.status?.cachedBlobs || [];
          const warmImages = selectedNode.status?.warmImages || [];
          const cachedCount = (cachedArtifacts.length || cachedBlobs.length) + warmImages.length;

          const nodePrewarmEntries = prewarmJobs
            .map((job) => {
              const entry = job.status?.nodes?.find((ns) => ns.nodeName === nodeName);
              return entry ? { job, entry } : null;
            })
            .filter(
              (
                x,
              ): x is {
                job: PrewarmJobItem;
                entry: PrewarmNodeStatus;
              } => Boolean(x),
            );

          const runningPrewarmCount = nodePrewarmEntries.filter(
            ({ entry }) => entry.phase === "Pending" || entry.phase === "Running",
          ).length;
          const failedPrewarmEntries = nodePrewarmEntries.filter(
            ({ entry }) => entry.phase === "Failed",
          );

          async function retryFailedPrewarmJobs() {
            if (failedPrewarmEntries.length === 0) return;
            setRetryingPrewarm(true);
            try {
              await Promise.all(
                failedPrewarmEntries.map(async ({ job }) => {
                  const nextNodes = (job.status?.nodes || []).map((ns) =>
                    ns.nodeName === nodeName
                      ? { nodeName: ns.nodeName, phase: "Pending", message: "" }
                      : ns,
                  );
                  const res = await apiFetch(
                    `/apis/core.gamepanel.io/v1/prewarmjobs/${encodeURIComponent(job.metadata.name)}/status`,
                    {
                      method: "PUT",
                      body: JSON.stringify({
                        apiVersion: "core.gamepanel.io/v1",
                        kind: "PrewarmJob",
                        metadata: {
                          name: job.metadata.name,
                          uid: job.metadata.uid,
                          statusVersion: job.metadata.statusVersion,
                        },
                        status: {
                          ...(job.status || {}),
                          nodes: nextNodes,
                        },
                      }),
                    },
                  );
                  if (!res.ok) {
                    throw new Error(await errorMessage(res, "重新触发节点预热失败"));
                  }
                }),
              );
              toast("success", "已重新下发节点预热任务");
              await loadClusterSnapshots();
            } catch (err) {
              toast("error", err instanceof Error ? err.message : "重试预热失败");
            } finally {
              setRetryingPrewarm(false);
            }
          }

          return (
            <div>
              <button
                type="button"
                onClick={() => {
                  setSelectedNodeName(null);
                  syncNodeUrl(null);
                }}
                className={`mb-5 inline-flex items-center gap-1.5 rounded-full text-sm font-medium text-[#5B6075] hover:text-[#2B3AE8] ${focusRing}`}
              >
                <ArrowLeft className="h-4 w-4" aria-hidden />
                计算节点
              </button>

              <PageHeader
                title={
                  <span className="inline-flex flex-wrap items-center gap-3">
                    <span>{selectedNode.metadata.name}</span>
                    <NodeStatus
                      phase={selectedNode.status?.phase}
                      paused={paused}
                      conditions={selectedNode.status?.conditions}
                    />
                  </span>
                }
                description={
                  <span className="inline-flex flex-wrap items-center gap-x-3 gap-y-1">
                    <span>内网：{internal ? <Mono>{internal}</Mono> : "-"}</span>
                    <span className="text-[#CBD0E0]">|</span>
                    <span>公网：{external ? <Mono>{external}</Mono> : "未配置"}</span>
                    <span className="text-[#CBD0E0]">|</span>
                    <span>
                      最近心跳：
                      {selectedNode.status?.lastHeartbeatTime
                        ? `${relativeTime(selectedNode.status.lastHeartbeatTime)}上报`
                        : "等待首次上报"}
                    </span>
                  </span>
                }
                actions={
                  <div className="flex flex-wrap items-center gap-2">
                    <Button
                      variant="secondary"
                      icon={<Terminal className="h-4 w-4" aria-hidden />}
                      onClick={() =>
                        openFloatingToolbox({ tool: "terminal", node: selectedNode.metadata.name })
                      }
                    >
                      节点终端
                    </Button>
                    <Button
                      variant="secondary"
                      icon={paused ? <PlayCircle className="h-4 w-4" aria-hidden /> : <PauseCircle className="h-4 w-4" aria-hidden />}
                      onClick={() => setToggleTarget(selectedNode)}
                    >
                      {paused ? "恢复调度" : "暂停调度"}
                    </Button>
                  </div>
                }
              />

              <div className="mb-6 overflow-x-auto">
                <Segmented<NodeDetailTab>
                  label="节点详情"
                  value={detailTab}
                  onChange={(t) => {
                    setDetailTab(t);
                    syncNodeUrl(selectedNode.metadata.name, t);
                  }}
                  items={[
                    { value: "overview", label: "概览" },
                    { value: "cache", label: "节点预热" },
                    { value: "instances", label: "服务器" },
                    { value: "settings", label: "设置" },
                  ]}
                />
              </div>

              {detailTab === "overview" &&
                (() => {
                  const cap = selectedNode.status?.capacity || {};
                  const alloc = selectedNode.status?.allocatable || {};
                  const reserved = selectedNode.status?.reserved || {};
                  const usage = selectedNode.status?.usage;
                  const ready = (selectedNode.status?.phase || "") === "Ready";
                  const capCpuCores = Math.max(1, parseNumericQuantity(cap.cpu || "1"));
                  const capMemBytes = Math.max(
                    1,
                    parseNumericQuantity(cap.memory || "1Gi") * 1024 * 1024 * 1024,
                  );
                  const capDiskBytes = Math.max(
                    1,
                    parseNumericQuantity(cap.storage || "10Gi") * 1024 * 1024 * 1024,
                  );
                  const latest = metricsSamples[metricsSamples.length - 1];
                  const cpuUsed = latest ? latest.cpuMillicores / 1000 : (usage?.cpuMillicores || 0) / 1000;
                  const memUsed = latest ? (latest.memoryBytes ?? 0) : usage?.memoryBytes || 0;
                  const diskUsed = latest ? (latest.storageBytes ?? 0) : usage?.storageBytes || 0;
                  const netRx = usage?.networkRxBytesPerSec ?? latest?.networkRxBytesPerSec ?? 0;
                  const netTx = usage?.networkTxBytesPerSec ?? latest?.networkTxBytesPerSec ?? 0;
                  const netPeakRate = Math.max(
                    netRx + netTx,
                    ...metricsSamples.map(readNodeNetBytes),
                    0,
                  );
                  const netScaleMax = Math.max(1024, netPeakRate);
                  const cpuBudgetLine = formatBudgetSubline(
                    computeNodeResourceBudget(cap.cpu, alloc.cpu, reserved.cpu, "cpu"),
                  );
                  const memBudgetLine = formatBudgetSubline(
                    computeNodeResourceBudget(cap.memory, alloc.memory, reserved.memory, "gb"),
                  );
                  const diskBudgetLine = formatBudgetSubline(
                    computeNodeResourceBudget(cap.storage, alloc.storage, reserved.storage, "gb"),
                  );
                  const anyBudgetLine = Boolean(cpuBudgetLine || memBudgetLine || diskBudgetLine);
                  const labels = selectedNode.metadata.labels || {};
                  const runtimes =
                    selectedNode.status?.runtimes?.map((r) => r.handler).filter(Boolean).join(", ") || "docker";
                  const clockCond = selectedNode.status?.conditions?.find(
                    (c) => c.type === "ClockSynced",
                  );

                  return (
                    <Card>
                      <CardHeader
                        title="节点监控"
                        description={`共 ${metricsSamples.length} 个采样点${
                          metricsLoading ? " · 刷新中…" : ""
                        }`}
                        actions={
                          <Segmented
                            label="监控时间范围"
                            value={metricsWindow}
                            onChange={setMetricsWindow}
                            items={NODE_METRIC_WINDOWS}
                          />
                        }
                      />
                      <div className="divide-y divide-[#EEF0F7]">
                        <div className="grid grid-cols-1 gap-6 p-6 sm:grid-cols-2 lg:grid-cols-4">
                          <div className="flex flex-col">
                            <div
                              className={`${anyBudgetLine ? "mb-1" : "mb-1.5"} flex items-center justify-between text-xs`}
                            >
                              <span className="font-medium text-[#14161F]">CPU 负载</span>
                              <span className="font-mono text-[#5B6075] tabular-nums">
                                {ready || latest ? `${cpuUsed.toFixed(2)} 核` : "-"} / {formatCpu(cap.cpu)}
                              </span>
                            </div>
                            {anyBudgetLine && (
                              <div
                                className="mb-1.5 h-4 truncate text-[11px] leading-4 text-[#8A90A6] tabular-nums"
                                title={cpuBudgetLine || undefined}
                              >
                                {cpuBudgetLine}
                              </div>
                            )}
                            <NodeSparkline
                              className="mt-auto"
                              samples={metricsSamples}
                              maxVal={capCpuCores}
                              color="#2B3AE8"
                              valueOf={readNodeCpuCores}
                              formatValue={formatCpuCoresSample}
                            />
                          </div>

                          <div className="flex flex-col">
                            <div
                              className={`${anyBudgetLine ? "mb-1" : "mb-1.5"} flex items-center justify-between text-xs`}
                            >
                              <span className="font-medium text-[#14161F]">内存占用</span>
                              <span className="font-mono text-[#5B6075] tabular-nums">
                                {ready || latest ? formatBytes(memUsed) : "-"} / {formatGB(cap.memory)}
                              </span>
                            </div>
                            {anyBudgetLine && (
                              <div
                                className="mb-1.5 h-4 truncate text-[11px] leading-4 text-[#8A90A6] tabular-nums"
                                title={memBudgetLine || undefined}
                              >
                                {memBudgetLine}
                              </div>
                            )}
                            <NodeSparkline
                              className="mt-auto"
                              samples={metricsSamples}
                              maxVal={capMemBytes}
                              color="#0E9384"
                              valueOf={readNodeMemBytes}
                              formatValue={formatBytes}
                            />
                          </div>

                          <div className="flex flex-col">
                            <div
                              className={`${anyBudgetLine ? "mb-1" : "mb-1.5"} flex items-center justify-between text-xs`}
                            >
                              <span className="font-medium text-[#14161F]">存储空间</span>
                              <span className="font-mono text-[#5B6075] tabular-nums">
                                {ready || latest ? formatBytes(diskUsed) : "-"} / {formatGB(cap.storage)}
                              </span>
                            </div>
                            {anyBudgetLine && (
                              <div
                                className="mb-1.5 h-4 truncate text-[11px] leading-4 text-[#8A90A6] tabular-nums"
                                title={diskBudgetLine || undefined}
                              >
                                {diskBudgetLine}
                              </div>
                            )}
                            <NodeSparkline
                              className="mt-auto"
                              samples={metricsSamples}
                              maxVal={capDiskBytes}
                              color="#D97706"
                              valueOf={readNodeDiskBytes}
                              formatValue={formatBytes}
                            />
                          </div>

                          <div className="flex flex-col">
                            <div
                              className={`${anyBudgetLine ? "mb-1" : "mb-1.5"} flex items-center justify-between gap-2 text-xs`}
                            >
                              <span className="whitespace-nowrap font-medium text-[#14161F]">网络速率</span>
                              <span className="whitespace-nowrap font-mono text-[#5B6075] tabular-nums">
                                {ready || latest
                                  ? `↓ ${formatRateBytes(netRx)} · ↑ ${formatRateBytes(netTx)}`
                                  : "-"}
                              </span>
                            </div>
                            {anyBudgetLine && (
                              <div className="mb-1.5 h-4 text-[11px] leading-4 text-[#8A90A6] tabular-nums">
                                {netPeakRate > 0 ? `峰值 ${formatRateBytes(netPeakRate)}` : ""}
                              </div>
                            )}
                            <NodeSparkline
                              className="mt-auto"
                              samples={metricsSamples}
                              maxVal={netScaleMax}
                              color="#0EA5E9"
                              valueOf={readNodeNetBytes}
                              formatValue={formatRateBytes}
                              formatSample={formatNodeNetSample}
                            />
                          </div>
                        </div>

                        <div>
                          <div className="px-6 pt-5 pb-1 text-xs font-medium text-[#8A90A6]">基本信息</div>
                          <InfoList
                            rows={[
                          ["节点名称", <Mono key="name">{selectedNode.metadata.name}</Mono>],
                          [
                            "内网地址",
                            internal ? (
                              <span key="int" className="inline-flex items-center gap-0.5">
                                <Mono>{internal}</Mono>
                                <CopyButton text={internal} label="复制内网地址" />
                              </span>
                            ) : (
                              "-"
                            ),
                          ],
                          [
                            "公网地址",
                            external ? (
                              <span key="ext" className="inline-flex items-center gap-0.5">
                                <Mono>{external}</Mono>
                                <CopyButton text={external} label="复制公网地址" />
                              </span>
                            ) : (
                              <span key="ext-empty" className="text-[#8A90A6]">
                                未配置（可在「设置」中指定）
                              </span>
                            ),
                          ],
                          ["节点运行时", <Mono key="rt">{runtimes}</Mono>],
                          [
                            "可调度游戏",
                            <div key="allowed" className="flex flex-wrap items-center gap-2">
                              {selectedNode.spec?.allowedApps && selectedNode.spec.allowedApps.length > 0 ? (
                                <span className="text-xs text-[#14161F]">
                                  {selectedNode.spec.allowedApps
                                    .map((id) => apps.find((a) => a.metadata.name === id)?.displayName || id)
                                    .join(" · ")}
                                </span>
                              ) : (
                                <span className="text-xs text-[#5B6075]">
                                  全部已发布游戏（默认）
                                </span>
                              )}
                              <NodeScopePopoverButton
                                node={selectedNode}
                                apps={apps}
                                registryArtifacts={registryArtifacts}
                                artifactMap={artifactMap}
                                variant="link"
                                onSave={handleSaveNodeScope}
                              />
                            </div>,
                          ],
                          [
                            "时钟同步",
                            clockCond?.status === "False" ? (
                              <span key="clock" className="text-xs font-medium text-[#B42318]">
                                时钟偏差异常：
                                {clockCond.message ||
                                  "节点与平台时钟偏差超过 30 秒，请执行 timedatectl set-ntp true 同步时钟"}
                              </span>
                            ) : (
                              <span key="clock" className="text-xs font-medium text-[#027A48]">
                                已同步
                              </span>
                            ),
                          ],
                          [
                            "节点预热",
                            <div key="cache" className="flex flex-wrap items-center gap-2">
                              <button
                                type="button"
                                onClick={() => {
                                  setDetailTab("cache");
                                  syncNodeUrl(selectedNode.metadata.name, "cache");
                                }}
                                className={`text-left text-sm font-medium text-[#2B3AE8] hover:underline ${focusRing} rounded`}
                              >
                                {cachedCount > 0
                                  ? `已就绪 ${cachedCount} 项（查看）`
                                  : "未预热（查看）"}
                              </button>
                              {failedPrewarmEntries.length > 0 && (
                                <>
                                  <span className="text-xs text-[#B42318]">
                                    预热异常：{humanizePrewarmError(failedPrewarmEntries[0].entry.message)}
                                  </span>
                                  <Button
                                    type="button"
                                    size="sm"
                                    variant="secondary"
                                    icon={<RefreshCw className="h-3.5 w-3.5" aria-hidden />}
                                    loading={retryingPrewarm}
                                    onClick={() => void retryFailedPrewarmJobs()}
                                  >
                                    重试预热
                                  </Button>
                                </>
                              )}
                              {failedPrewarmEntries.length === 0 && runningPrewarmCount > 0 && (
                                <Badge tone="accent">后台预热中 ({runningPrewarmCount})</Badge>
                              )}
                            </div>,
                          ],
                          [
                            "调度标签",
                            Object.keys(labels).length > 0 ? (
                              <div key="lbls" className="flex flex-wrap gap-1.5">
                                {Object.entries(labels).map(([k, v]) => (
                                  <Mono
                                    key={k}
                                    className="rounded-md bg-[#F2F4FB] px-2 py-0.5 text-xs text-[#5B6075]"
                                  >
                                    {k}={v}
                                  </Mono>
                                ))}
                              </div>
                            ) : (
                              "-"
                            ),
                          ],
                        ]}
                          />
                        </div>
                      </div>
                    </Card>
                  );
                })()}

              {detailTab === "cache" &&
                (() => {
                  const cachedList = cachedArtifacts.length > 0 ? cachedArtifacts : cachedBlobs;
                  const hasAny =
                    nodePrewarmEntries.length > 0 || cachedList.length > 0 || warmImages.length > 0;
                  const freeDiskBytes = computeNodeFreeDiskBytes(selectedNode);

                  return (
                    <Card className="overflow-hidden">
                      <CardHeader
                        title="预热资源"
                        description={[
                          nodePrewarmEntries.length > 0
                            ? `预热任务 ${nodePrewarmEntries.length} 项`
                            : "",
                          cachedCount > 0
                            ? `游戏与模组 ${cachedList.length} 项 · 运行环境 ${warmImages.length} 项`
                            : nodePrewarmEntries.length === 0
                              ? "尚未预热游戏或运行环境"
                              : "",
                          freeDiskBytes > 0
                            ? `节点可用磁盘 ${formatBytesCompact(freeDiskBytes)}`
                            : "",
                        ]
                          .filter(Boolean)
                          .join(" · ")}
                        actions={
                          <>
                            {cacheSubTab === "jobs" && failedPrewarmEntries.length > 0 && (
                              <Button
                                size="sm"
                                variant="secondary"
                                icon={<RefreshCw className="h-3.5 w-3.5" aria-hidden />}
                                loading={retryingPrewarm}
                                onClick={() => void retryFailedPrewarmJobs()}
                              >
                                重试失败任务
                              </Button>
                            )}
                            <LinkButton href="/console/admin/registry" variant="secondary" size="sm">
                              前往镜像站
                            </LinkButton>
                            <Button
                              variant="primary"
                              size="sm"
                              icon={<HardDriveDownload className="h-3.5 w-3.5" aria-hidden />}
                              onClick={() => openPrewarmModal(selectedNode.metadata.name)}
                            >
                              节点预热
                            </Button>
                          </>
                        }
                      />

                      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[#EEF0F7] bg-[#FAFBFD] px-6 py-3">
                        <Segmented<NodeCacheSubTab>
                          size="sm"
                          label="预热资源视图"
                          value={cacheSubTab}
                          onChange={(t) => {
                            cacheSubTabTouchedRef.current = true;
                            setCacheSubTab(t);
                          }}
                          items={[
                            { value: "jobs", label: "预热任务" },
                            { value: "artifacts", label: "游戏与模组" },
                            { value: "images", label: "运行环境" },
                          ]}
                        />

                        <NodeScopePopoverButton
                          node={selectedNode}
                          apps={apps}
                          registryArtifacts={registryArtifacts}
                          artifactMap={artifactMap}
                          variant="toolbar"
                          onSave={handleSaveNodeScope}
                        />
                      </div>

                      {!hasAny ? (
                        <EmptyState
                          image={null}
                          title="暂无预热资源"
                          body="点击右上角「节点预热」可提前同步常用游戏或运行环境。"
                        />
                      ) : cacheSubTab === "jobs" ? (
                        nodePrewarmEntries.length === 0 ? (
                          <EmptyState
                            image={null}
                            title="暂无预热任务"
                            body="点击右上角「节点预热」下发任务后，可在此查看执行进度与结果。"
                          />
                        ) : (
                          <ul className="divide-y divide-[#EEF0F7]">
                            {nodePrewarmEntries.map(({ job, entry }) => {
                              const firstArt = job.spec.artifacts?.[0];
                              const primaryCoord = firstArt
                                ? `${firstArt.repository}.${firstArt.package}.${firstArt.version || "latest"}`
                                : "";
                              const matchedArt = primaryCoord ? artifactMap.get(primaryCoord) : undefined;
                              const targetSummary = [
                                ...(job.spec.artifacts || []).map((a) => {
                                  const resolved = resolveCachedRepoInfo(apps, a.repository);
                                  const ver = `${a.package}@${a.version || "latest"}`;
                                  return resolved ? `${resolved.appName} · ${ver}` : ver;
                                }),
                                ...(job.spec.images || []),
                              ].join(" · ");
                              const active = entry.phase === "Running";
                              const regSyncing = Boolean(
                                matchedArt &&
                                  (matchedArt.status?.phase === "Pending" ||
                                    matchedArt.status?.phase === "Syncing"),
                              );
                              const syncProgress = regSyncing ? (matchedArt?.status?.progress ?? 0) : 0;
                              const liveMessage = regSyncing
                                ? matchedArt?.status?.message?.trim() || "镜像站正在同步制品..."
                                : entry.message?.trim() || "正在节点本地拉取并解压制品...";
                              const tone =
                                entry.phase === "Succeeded"
                                  ? "success"
                                  : entry.phase === "Failed"
                                    ? "danger"
                                    : active
                                      ? "accent"
                                      : "neutral";
                              const phaseLabel =
                                entry.phase === "Succeeded"
                                  ? "已就绪"
                                  : entry.phase === "Failed"
                                    ? "预热失败"
                                    : entry.phase === "Running"
                                      ? "下发中"
                                      : "等待执行";
                              const openJobLog = () => {
                                if (!primaryCoord || !firstArt) return;
                                setPrewarmLogTarget({
                                  coord: primaryCoord,
                                  title: targetSummary || primaryCoord,
                                  fallback: buildFallbackPrewarmArtifact(
                                    primaryCoord,
                                    firstArt.repository,
                                    firstArt.package,
                                    firstArt.version || "latest",
                                    entry,
                                  ),
                                  nodeEntry: entry,
                                });
                              };
                              return (
                                <li
                                  key={job.metadata.name}
                                  className="flex flex-wrap items-center justify-between gap-4 px-6 py-4 transition-colors hover:bg-[#FAFBFD]"
                                >
                                  <div className="min-w-0 flex-1">
                                    {primaryCoord ? (
                                      <button
                                        type="button"
                                        onClick={openJobLog}
                                        className={`block max-w-full truncate text-left text-sm font-medium text-[#14161F] hover:text-[#2B3AE8] ${focusRing}`}
                                      >
                                        {targetSummary || job.metadata.name}
                                      </button>
                                    ) : (
                                      <div className="truncate text-sm font-medium text-[#14161F]">
                                        {targetSummary || job.metadata.name}
                                      </div>
                                    )}
                                    {active && (
                                      <div className="mt-2 flex flex-wrap items-center gap-3">
                                        <div className="w-full max-w-xs">
                                          <progress
                                            className="gp-progress"
                                            max={100}
                                            {...(syncProgress > 0
                                              ? { value: Math.min(100, syncProgress) }
                                              : {})}
                                          />
                                        </div>
                                        {liveMessage && (
                                          <span className="truncate text-xs text-[#5B6075]">
                                            {liveMessage}
                                            {syncProgress > 0 ? ` · ${syncProgress}%` : ""}
                                          </span>
                                        )}
                                      </div>
                                    )}
                                    {entry.phase === "Failed" && (
                                      <div className="mt-1 text-xs text-[#B42318]">
                                        {humanizePrewarmError(entry.message)}
                                      </div>
                                    )}
                                  </div>
                                  <div className="flex items-center gap-2.5">
                                    {entry.completedAt && (
                                      <span className="text-xs text-[#8A90A6]">
                                        {relativeTime(entry.completedAt)}
                                      </span>
                                    )}
                                    {primaryCoord && (
                                      <Button
                                        size="sm"
                                        variant="ghost"
                                        icon={<Terminal className="h-3.5 w-3.5" aria-hidden />}
                                        onClick={openJobLog}
                                      >
                                        查看日志
                                      </Button>
                                    )}
                                    <Badge tone={tone}>{phaseLabel}</Badge>
                                  </div>
                                </li>
                              );
                            })}
                          </ul>
                        )
                      ) : cacheSubTab === "artifacts" ? (
                        cachedList.length === 0 ? (
                          <EmptyState
                            image={null}
                            title="暂无已缓存游戏与模组"
                            body="节点拉取游戏或模组后，已缓存的内容会显示在此。"
                          />
                        ) : (
                          <ul className="divide-y divide-[#EEF0F7]">
                            {cachedList.map((item) => {
                              const parts = item.split(".");
                              const repo = parts[0] || "";
                              const pkg = parts[1] || "";
                              const ver = parts.slice(2).join(".") || "latest";
                              const resolved = resolveCachedRepoInfo(apps, repo);
                              const art = artifactMap.get(item);
                              const openArtifactLog = () => {
                                setPrewarmLogTarget({
                                  coord: item,
                                  title: resolved ? `${resolved.appName} · ${pkg}@${ver}` : item,
                                  fallback: buildFallbackPrewarmArtifact(item, repo, pkg, ver),
                                });
                              };
                              return (
                                <li
                                  key={item}
                                  className="flex items-center justify-between gap-4 px-6 py-4 transition-colors hover:bg-[#FAFBFD]"
                                >
                                  <div className="min-w-0 flex-1">
                                    {resolved && (
                                      <div className="mb-1 flex items-center gap-2">
                                        <span className="truncate text-sm font-medium text-[#14161F]">
                                          {resolved.appName}
                                        </span>
                                        <Badge tone="neutral">{resolved.kindLabel}</Badge>
                                      </div>
                                    )}
                                    <Mono className="block min-w-0 truncate text-xs text-[#5B6075]">
                                      {item}
                                    </Mono>
                                  </div>
                                  <div className="flex shrink-0 items-center gap-1">
                                    {(art || pkg) && (
                                      <IconButton
                                        label="查看日志"
                                        className="h-7 w-7 bg-transparent"
                                        onClick={openArtifactLog}
                                      >
                                        <Terminal className="h-3.5 w-3.5" aria-hidden />
                                      </IconButton>
                                    )}
                                    <CopyButton text={item} label="复制缓存标识" />
                                  </div>
                                </li>
                              );
                            })}
                          </ul>
                        )
                      ) : warmImages.length === 0 ? (
                        <EmptyState
                          image={null}
                          title="暂无已预热运行环境"
                          body="点击右上角「节点预热」同步运行环境镜像后，镜像地址将显示在此。"
                        />
                      ) : (
                        <ul className="divide-y divide-[#EEF0F7]">
                          {warmImages.map((img) => (
                            <li
                              key={img}
                              className="flex items-center justify-between gap-4 px-6 py-4 transition-colors hover:bg-[#FAFBFD]"
                            >
                              <Mono className="min-w-0 truncate text-sm text-[#14161F]">{img}</Mono>
                              <CopyButton text={img} label="复制运行环境地址" />
                            </li>
                          ))}
                        </ul>
                      )}
                    </Card>
                  );
                })()}

              {detailTab === "instances" && (
                <Card>
                  <CardHeader title="服务器" />
                  {nodeInstances.length === 0 ? (
                    <EmptyState
                      image={null}
                      title="暂无服务器"
                      body="当前节点上还没有调度任何游戏服务器。"
                    />
                  ) : (
                    <div tabIndex={0} role="region" aria-label="节点服务器列表" className="overflow-x-auto">
                      <table className="w-full min-w-[680px] text-left text-sm">
                        <thead>
                          <tr className="border-b border-[#EEF0F7] text-[13px] text-[#8A90A6]">
                            <th scope="col" className="px-6 py-3.5 font-medium">服务器名称</th>
                            <th scope="col" className="px-4 py-3.5 font-medium">游戏</th>
                            <th scope="col" className="px-4 py-3.5 font-medium">状态</th>
                            <th scope="col" className="px-4 py-3.5 font-medium">连接地址</th>
                            <th scope="col" className="px-6 py-3.5 font-medium">配置</th>
                          </tr>
                        </thead>
                        <tbody>
                          {nodeInstances.map((inst) => {
                            const game = gameOf(inst, apps);
                            const ep = endpointOf(inst, nodes, apps);
                            const lim = inst.spec?.resources?.limits || {};
                            return (
                              <tr
                                key={`${inst.metadata.tenant}/${inst.metadata.name}`}
                                className="border-b border-[#EEF0F7] last:border-0 hover:bg-[#FAFBFD]"
                              >
                                <td className="px-6 py-4">
                                  <Link
                                    href={
                                      inst.metadata.tenant
                                        ? tenantServerRoute(inst.metadata.tenant, inst.metadata.name)
                                        : "/console"
                                    }
                                    onClick={() => {
                                      if (inst.metadata.tenant) setTenant(inst.metadata.tenant);
                                    }}
                                    className="font-semibold text-[#14161F] hover:text-[#2B3AE8]"
                                  >
                                    {inst.metadata.name}
                                  </Link>
                                  <div className="mt-0.5 text-xs text-[#8A90A6]">
                                    工作区：{inst.metadata.tenant || "default"}
                                  </div>
                                </td>
                                <td className="px-4 py-4 text-[#5B6075]">{game.title}</td>
                                <td className="px-4 py-4">
                                  <StatusPill phase={phaseOf(inst)} />
                                </td>
                                <td className="px-4 py-4">
                                  {ep ? (
                                    <span className="inline-flex items-center gap-0.5">
                                      <Mono className="text-[#14161F]">{ep}</Mono>
                                      <CopyButton text={ep} label="复制连接地址" />
                                    </span>
                                  ) : (
                                    <span className="text-[#8A90A6]">-</span>
                                  )}
                                </td>
                                <td className="px-6 py-4 text-xs text-[#5B6075]">
                                  {formatCpu(lim.cpu)} · {formatGB(lim.memory)} ·{" "}
                                  {formatGB(lim.storage)}
                                </td>
                              </tr>
                            );
                          })}
                        </tbody>
                      </table>
                    </div>
                  )}
                </Card>
              )}

              {detailTab === "settings" && (
                <Card>
                  <CardHeader title="节点配置与状态管理" />
                  <div className="divide-y divide-[#EEF0F7]">
                    <form
                      onSubmit={(e) => void handleSaveDetailSettings(e)}
                      noValidate
                      className="space-y-4 p-6"
                    >
                      <Field label="公网连接地址（选填）" htmlFor="detail-node-ext-ip">
                        <Input
                          id="detail-node-ext-ip"
                          value={editExternalIP}
                          onChange={(e) => setEditExternalIP(e.target.value)}
                          placeholder="留空自动探测，如 play.example.com 或 203.0.113.10"
                          autoComplete="off"
                          className="font-mono"
                        />
                      </Field>
                      <Field label="节点调度标签（每行一个 key=value）" htmlFor="detail-node-labels">
                        <Textarea
                          id="detail-node-labels"
                          rows={4}
                          value={editLabelsText}
                          onChange={(e) => setEditLabelsText(e.target.value)}
                          placeholder={"topology.gamepanel.io/region=default\ntier=standard"}
                          className="font-mono text-xs"
                        />
                      </Field>
                      <div className="flex justify-end pt-2">
                        <Button
                          variant="primary"
                          type="submit"
                          loading={savingEdit}
                          disabled={!isNodeSettingsDirty(selectedNode) || savingEdit}
                        >
                          保存配置
                        </Button>
                      </div>
                    </form>

                    <div className="flex flex-wrap items-center justify-between gap-4 px-6 py-4">
                      <div>
                        <div className="text-sm font-medium text-[#14161F]">
                          {paused ? "恢复节点调度" : "暂停节点调度"}
                        </div>
                        <div className="mt-0.5 text-xs text-[#8A90A6]">
                          暂停后新创建的服务器将不再调度到该节点，已在运行的服务器不受影响。
                        </div>
                      </div>
                      <Button
                        variant="secondary"
                        icon={
                          paused ? (
                            <PlayCircle className="h-4 w-4" aria-hidden />
                          ) : (
                            <PauseCircle className="h-4 w-4" aria-hidden />
                          )
                        }
                        onClick={() => setToggleTarget(selectedNode)}
                      >
                        {paused ? "恢复调度" : "暂停调度"}
                      </Button>
                    </div>
                  </div>
                </Card>
              )}
            </div>
          );
        })()
      )}

      <Modal
        open={!!editTarget}
        onClose={() => !savingEdit && setEditTarget(null)}
        title={`编辑节点配置 · ${editTarget?.metadata.name ?? ""}`}
        footer={
          <>
            <Button variant="ghost" onClick={() => setEditTarget(null)} disabled={savingEdit}>
              取消
            </Button>
            <Button
              variant="primary"
              type="submit"
              form="edit-node-form"
              loading={savingEdit}
              disabled={!isNodeSettingsDirty(editTarget) || savingEdit}
            >
              保存配置
            </Button>
          </>
        }
      >
        <form id="edit-node-form" onSubmit={(e) => void handleSaveNodeEdit(e)} noValidate className="space-y-4">
          <Field label="公网连接地址（选填）" htmlFor="edit-node-ext-ip">
            <Input
              id="edit-node-ext-ip"
              value={editExternalIP}
              onChange={(e) => setEditExternalIP(e.target.value)}
              placeholder="留空自动探测，如 play.example.com 或 203.0.113.10"
              autoComplete="off"
              className="font-mono"
            />
          </Field>
          <Field label="节点调度标签（每行一个 key=value）" htmlFor="edit-node-labels">
            <Textarea
              id="edit-node-labels"
              rows={4}
              value={editLabelsText}
              onChange={(e) => setEditLabelsText(e.target.value)}
              placeholder={"topology.gamepanel.io/region=default\ntier=standard"}
              className="font-mono text-xs"
            />
          </Field>
        </form>
      </Modal>

      <ConfirmDialog
        open={!!toggleTarget}
        title={
          pausing
            ? `暂停调度节点 ${toggleTarget?.metadata.name ?? ""}？`
            : `恢复调度节点 ${toggleTarget?.metadata.name ?? ""}？`
        }
        body={
          pausing
            ? "暂停后新创建的服务器将不再调度到该节点，已在运行的服务器不受影响。"
            : "恢复后新创建的服务器可正常调度到该节点。"
        }
        confirmLabel={pausing ? "暂停调度" : "恢复调度"}
        danger={pausing}
        loading={toggleBusy}
        onConfirm={() => void toggleSchedulable()}
        onClose={() => !toggleBusy && setToggleTarget(null)}
      />

      <Modal
        open={!!metricsNode}
        onClose={() => setMetricsNode(null)}
        title={`节点监控 · ${metricsNode?.metadata.name ?? ""}`}
        width="max-w-2xl"
      >
        {metricsNode && (
          <div className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="text-xs text-[#5B6075]">
                共 {metricsSamples.length} 个采样点
                {metricsLoading ? " · 刷新中…" : ""}
              </div>
              <Segmented
                label="监控时间范围"
                value={metricsWindow}
                onChange={setMetricsWindow}
                items={NODE_METRIC_WINDOWS}
              />
            </div>

            {(() => {
              const capCpuCores = Math.max(1, parseNumericQuantity(metricsNode.status?.capacity?.cpu || "1"));
              const capMemBytes = Math.max(
                1,
                parseNumericQuantity(metricsNode.status?.capacity?.memory || "1Gi") * 1024 * 1024 * 1024,
              );
              const capDiskBytes = Math.max(
                1,
                parseNumericQuantity(metricsNode.status?.capacity?.storage || "10Gi") * 1024 * 1024 * 1024,
              );
              const latest = metricsSamples[metricsSamples.length - 1];
              const nodeUsage = metricsNode.status?.usage;
              const uStorage = latest?.storageBytes ?? nodeUsage?.storageBytes ?? 0;
              const storagePct = capDiskBytes > 0 ? Math.min(100, Math.round((uStorage / capDiskBytes) * 100)) : 0;
              const netRx = nodeUsage?.networkRxBytesPerSec ?? latest?.networkRxBytesPerSec ?? 0;
              const netTx = nodeUsage?.networkTxBytesPerSec ?? latest?.networkTxBytesPerSec ?? 0;
              const netScaleMax = Math.max(1024, netRx + netTx, ...metricsSamples.map(readNodeNetBytes));

              return (
                <div className="space-y-3 rounded-2xl bg-[#F7F8FC] p-4">
                  <div>
                    <div className="mb-1.5 flex items-center justify-between text-xs">
                      <span className="font-medium text-[#14161F]">CPU 负载</span>
                      <span className="font-mono text-[#5B6075] tabular-nums">
                        {latest ? `${(latest.cpuMillicores / 1000).toFixed(2)} 核` : "-"} /{" "}
                        {formatCpu(metricsNode.status?.capacity?.cpu)}
                      </span>
                    </div>
                    <NodeSparkline
                      samples={metricsSamples}
                      maxVal={capCpuCores}
                      color="#2B3AE8"
                      valueOf={readNodeCpuCores}
                      formatValue={formatCpuCoresSample}
                    />
                  </div>

                  <div className="border-t border-[#E6E9F4] pt-3">
                    <div className="mb-1.5 flex items-center justify-between text-xs">
                      <span className="font-medium text-[#14161F]">内存占用</span>
                      <span className="font-mono text-[#5B6075] tabular-nums">
                        {latest ? formatBytes(latest.memoryBytes ?? 0) : "-"} /{" "}
                        {formatGB(metricsNode.status?.capacity?.memory)}
                      </span>
                    </div>
                    <NodeSparkline
                      samples={metricsSamples}
                      maxVal={capMemBytes}
                      color="#0E9384"
                      valueOf={readNodeMemBytes}
                      formatValue={formatBytes}
                    />
                  </div>

                  <div className="border-t border-[#E6E9F4] pt-3">
                    <div className="mb-1.5 flex items-center justify-between text-xs">
                      <span className="font-medium text-[#14161F]">网络速率</span>
                      <span className="font-mono text-[#5B6075] tabular-nums">
                        {latest || nodeUsage
                          ? `↓ ${formatRateBytes(netRx)} · ↑ ${formatRateBytes(netTx)}`
                          : "-"}
                      </span>
                    </div>
                    <NodeSparkline
                      samples={metricsSamples}
                      maxVal={netScaleMax}
                      color="#0EA5E9"
                      valueOf={readNodeNetBytes}
                      formatValue={formatRateBytes}
                      formatSample={formatNodeNetSample}
                    />
                  </div>

                  <div className="flex items-center justify-between gap-3 rounded-xl border border-[#E3E6F2] bg-white px-3.5 py-2.5">
                    <div className="flex items-baseline gap-2">
                      <span className="text-xs font-medium text-[#5B6075]">存储空间</span>
                      <span className="text-xs font-semibold text-[#14161F] tabular-nums">
                        {latest || nodeUsage ? formatBytes(uStorage) : "-"}
                      </span>
                      <span className="text-[11px] text-[#8A90A6] tabular-nums">
                        / {formatGB(metricsNode.status?.capacity?.storage)}
                      </span>
                    </div>
                    <div className="flex items-center gap-2">
                      <Meter label="存储空间" used={uStorage} max={capDiskBytes} hideHeader className="w-28" />
                      <span className="font-mono text-[11px] font-medium text-[#5B6075] tabular-nums">
                        {storagePct}%
                      </span>
                    </div>
                  </div>
                </div>
              );
            })()}
          </div>
        )}
      </Modal>

      <Modal
        open={prewarmOpen}
        onClose={() => !prewarmBusy && setPrewarmOpen(false)}
        title={`节点预热 · ${prewarmNodeName ?? ""}`}
        description="提前同步游戏或运行环境至当前节点"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPrewarmOpen(false)} disabled={prewarmBusy}>
              取消
            </Button>
            <Button variant="primary" type="submit" form="prewarm-node-form" loading={prewarmBusy}>
              开始预热
            </Button>
          </>
        }
      >
        <form id="prewarm-node-form" onSubmit={(e) => void handleTriggerPrewarm(e)} className="space-y-4">
          <Field label="预热类型">
            <Segmented<"app" | "image">
              label="预热类型"
              value={prewarmMode}
              onChange={setPrewarmMode}
              items={[
                { value: "app", label: "游戏" },
                { value: "image", label: "运行环境" },
              ]}
            />
          </Field>

          <div className="grid min-h-[168px]">
            <div
              className={`col-start-1 row-start-1 space-y-4 ${
                prewarmMode === "app" ? "" : "pointer-events-none invisible"
              }`}
            >
              <Field label="选择游戏" htmlFor="prewarm-app">
                <Select
                  id="prewarm-app"
                  value={prewarmAppName}
                  onChange={(e) => {
                    const nextName = e.target.value;
                    setPrewarmAppName(nextName);
                    const matched = apps.find((a) => a.metadata.name === nextName);
                    setPrewarmVersion(matched?.recommendedVersion || matched?.versions?.[0] || "latest");
                    if (matched?.image) setPrewarmImage(matched.image);
                  }}
                >
                  {(() => {
                    const targetNode = nodes.find((n) => n.metadata.name === prewarmNodeName);
                    const allowed = targetNode?.spec?.allowedApps;
                    const list = allowed && allowed.length > 0 ? apps.filter((a) => allowed.includes(a.metadata.name)) : apps;
                    return list.map((a) => (
                      <option key={a.metadata.name} value={a.metadata.name}>
                        {a.displayName || a.metadata.name}
                      </option>
                    ));
                  })()}
                </Select>
              </Field>
              <Field label="游戏版本" htmlFor="prewarm-version">
                {(() => {
                  const selectedApp = apps.find((a) => a.metadata.name === prewarmAppName);
                  const versions = selectedApp?.versions || [];
                  return versions.length > 0 ? (
                    <Select
                      id="prewarm-version"
                      value={prewarmVersion}
                      onChange={(e) => setPrewarmVersion(e.target.value)}
                    >
                      {versions.map((v) => (
                        <option key={v} value={v}>
                          {v}
                        </option>
                      ))}
                    </Select>
                  ) : (
                    <Input
                      id="prewarm-version"
                      value={prewarmVersion}
                      onChange={(e) => setPrewarmVersion(e.target.value)}
                      placeholder="latest"
                      className="font-mono"
                    />
                  );
                })()}
              </Field>
            </div>

            <div
              className={`col-start-1 row-start-1 space-y-4 ${
                prewarmMode === "image" ? "" : "pointer-events-none invisible"
              }`}
            >
              <Field label="运行环境地址" htmlFor="prewarm-image" hint="例如游戏中用到的基础系统运行环境地址">
                <Input
                  id="prewarm-image"
                  value={prewarmImage}
                  onChange={(e) => setPrewarmImage(e.target.value)}
                  placeholder="例如 steamcmd:latest"
                  className="font-mono"
                />
              </Field>
            </div>
          </div>
        </form>
      </Modal>

      <ArtifactLogModal
        open={Boolean(prewarmLogTarget)}
        onClose={() => setPrewarmLogTarget(null)}
        artifact={
          prewarmLogTarget
            ? resolvePrewarmLogArtifact(
                artifactMap.get(prewarmLogTarget.coord),
                prewarmLogTarget.fallback,
                prewarmLogTarget.nodeEntry,
              )
            : null
        }
        title={prewarmLogTarget?.title}
        token={token}
      />
    </div>
  );
}
