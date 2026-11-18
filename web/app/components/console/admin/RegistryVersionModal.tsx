"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
import {
  ArrowLeft,
  ExternalLink,
  FileText,
  Loader2,
  Package as PackageIcon,
  RefreshCw,
  RotateCcw,
  Terminal,
  Trash2,
} from "lucide-react";
import { formatBytes } from "../../../console/lib/format";
import type { RegistryArtifact, RegistryRepository } from "../../../console/lib/types";
import { AddonRichDescription } from "../servers/AddonMarkdown";
import {
  Badge,
  Button,
  CopyButton,
  IconButton,
  Input,
  LinkButton,
  type MenuItem,
  Modal,
  Mono,
  MoreMenu,
  Select,
} from "../ui";

const PAGE_SIZE = 15;

export interface PackageGroup {
  key: string;
  repository: string;
  packageId: string;
  repo?: RegistryRepository;
  isGamePkg: boolean;
  isLocalUpload: boolean;
  appName: string;
  ownerAppName?: string;
  appLabel: string;
  title: string;
  subtitle: string;
  iconUrl?: string;
  availableVersions?: string[];
  recommendedVersion?: string;
  artifacts: RegistryArtifact[];
  artifactNames: string[];
  activeArtifact?: RegistryArtifact;
  failedArtifact?: RegistryArtifact;
  readyCount: number;
  totalSizeBytes: number;
  latestVersion: string;
  upstreamNewerVersion?: string;
  overallPhase: "Syncing" | "Failed" | "Ready" | "Local";
  searchHaystack: string;
}

export function isFloatingVersionAlias(ver?: string): boolean {
  const low = (ver || "").trim().toLowerCase();
  return low === "" || low === "latest" || low === "stable";
}

export function formatAddonVersion(ver?: string): string {
  const v = (ver || "").trim();
  if (!v) return "最新版本";
  if (/^\d{10}$/.test(v)) {
    const d = new Date(Number(v) * 1000);
    if (!Number.isNaN(d.getTime())) {
      const y = d.getFullYear();
      const m = String(d.getMonth() + 1).padStart(2, "0");
      const day = String(d.getDate()).padStart(2, "0");
      return `${y}-${m}-${day} 构建`;
    }
  }
  return v;
}

export function humanizeSyncError(raw?: string): string {
  const s = (raw || "").trim();
  if (!s) return "同步上游失败，请检查代理设置后重试";
  if (/no space left|disk full|insufficient storage|quota/i.test(s)) {
    return "对象存储空间不足，请清理磁盘空间后重试";
  }
  if (/upload part|put s3 blob|internalerror|<\?xml|<error>|http\s+5\d\d/i.test(s)) {
    return "对象存储写入异常，请检查存储空间或稍后重试";
  }
  if (/http\s+404|not found/i.test(s)) return "上游未找到该资源版本";
  if (/timeout|deadline|timed out/i.test(s)) return "连接上游超时，请检查代理或重试";
  if (/steamcmd|app_update|workshop|download/i.test(s)) return "Steam 上游拉取失败，请稍后重试";
  return s.length > 72 ? `${s.slice(0, 72)}...` : s;
}

export function formatByteValue(bytes?: number): string {
  if (bytes === undefined || bytes === null || !Number.isFinite(bytes) || bytes <= 0) {
    return "-";
  }
  return formatBytes(bytes);
}

export function artifactStageShortLabel(art: RegistryArtifact): string {
  switch (art.status?.stage) {
    case "Queued":
      return "排队中";
    case "Resolving":
      return "解析中";
    case "Downloading":
      return "下载中";
    case "Archiving":
      return "打包中";
    case "Uploading":
      return "上传中";
    case "Completed":
      return "已就绪";
    case "Failed":
      return "拉取失败";
    default:
      break;
  }
  const phase = art.status?.phase;
  if (phase === "Pending") return "排队中";
  if (phase === "Syncing") return "拉取中";
  if (phase === "Ready") return "已就绪";
  if (phase === "Failed") return "拉取失败";
  return "等待中";
}

function formatDateTime(raw?: string): string {
  if (!raw) return "-";
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return "-";
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  return `${y}-${m}-${day} ${hh}:${mm}`;
}

function formatDatePart(raw?: string): string {
  if (!raw) return "";
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return "";
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function formatBuildShort(build?: string): string {
  const b = (build || "").trim();
  if (!b) return "";
  return b.length > 20 ? b.slice(0, 10) : b;
}

function buildVersionSubtitle(
  group: PackageGroup,
  art: RegistryArtifact,
  versionTitle: string,
  isFailed: boolean,
): string {
  if (isFailed) {
    return humanizeSyncError(art.status?.message);
  }
  const parts: string[] = [];
  const shortBuild = formatBuildShort(art.status?.build);
  if (shortBuild) {
    parts.push(`Build ${shortBuild}`);
  }
  const pubDate = formatDatePart(art.status?.publishedAt);
  if (pubDate) {
    parts.push(`发布于 ${pubDate}`);
  }
  const gvs = art.spec.gameVersions || [];
  if (gvs.length > 0) {
    parts.push(`MC ${gvs[0]}${gvs.length > 1 ? ` +${gvs.length - 1}` : ""}`);
  }
  const modIds = art.status?.modIds || [];
  if (modIds.length > 0) {
    parts.push(`ModID: ${modIds.slice(0, 2).join(", ")}${modIds.length > 2 ? ` +${modIds.length - 2}` : ""}`);
  }
  const deps = art.status?.dependencies || [];
  if (deps.length > 0) {
    parts.push(`${deps.length} 项前置`);
  }
  if (!group.isGamePkg && versionTitle !== art.spec.version && /^\d{10}$/.test(art.spec.version)) {
    parts.push(`修订 ${art.spec.version}`);
  }
  if (parts.length === 0) {
    const fmt = art.status?.descriptor?.archiveFormat || art.spec.archiveFormat;
    if (fmt) parts.push(fmt);
  }
  return parts.join(" · ");
}

function emptyChangelogHint(group: PackageGroup, art: RegistryArtifact): string {
  const src = (art.spec.sourceURL || "").trim();
  if (src.startsWith("steamcmd://")) {
    return "SteamCMD 专用服仅分发 Depot 二进制与 Build 清单，不包含纯文本更新日志；可点击右上角前往 Steam 官方公告查看变更动态。";
  }
  if (group.repo?.spec.format === "workshop_mod") {
    return "该创意工坊模组此版本未填写文本更新说明（作者可在 Steam 创意工坊上传时留空）；可点击右上角前往创意工坊改动记录页核对。";
  }
  if (group.isLocalUpload) {
    return "该版本为本地上传安装包，未附带上游发布说明。";
  }
  return "上游发布源未返回此版本的文字更新日志，可点击右上角前往上游发布页核对。";
}

export function RegistryVersionModal({
  open,
  group,
  busyCoord,
  hydratingCoord,
  onClose,
  onPullGameVersion,
  onPullLatestAddon,
  onRefreshArtifact,
  onHydrateArtifact,
  onOpenLogs,
  onRequestClean,
}: {
  open: boolean;
  group: PackageGroup | null;
  busyCoord: string | null;
  hydratingCoord?: string | null;
  onClose: () => void;
  onPullGameVersion: (group: PackageGroup, version: string) => Promise<void> | void;
  onPullLatestAddon?: (group: PackageGroup) => Promise<void> | void;
  onRefreshArtifact: (artifact: RegistryArtifact) => void;
  onHydrateArtifact: (artifactName: string) => Promise<void> | void;
  onOpenLogs: (artifactName: string) => void;
  onRequestClean: (artifactNames: string[], label: string) => void;
}) {
  const [page, setPage] = useState(1);
  const [pullVersion, setPullVersion] = useState("");
  const [customVersion, setCustomVersion] = useState("");
  const [pullBusy, setPullBusy] = useState(false);
  const [checkingAddon, setCheckingAddon] = useState(false);
  const [detailCoord, setDetailCoord] = useState<string | null>(null);

  const availableVersions = useMemo(
    () => group?.availableVersions || [],
    [group?.availableVersions],
  );
  const existingVersions = useMemo(
    () => new Set((group?.artifacts || []).map((a) => a.spec.version)),
    [group?.artifacts],
  );
  const defaultPullVersion = useMemo(() => {
    const uncached = availableVersions.find((v) => !existingVersions.has(v));
    if (uncached) return uncached;
    if (group?.recommendedVersion && availableVersions.includes(group.recommendedVersion)) {
      return group.recommendedVersion;
    }
    return availableVersions[0] || "";
  }, [availableVersions, existingVersions, group?.recommendedVersion]);

  useEffect(() => {
    setPage(1);
    setPullVersion(defaultPullVersion);
    setCustomVersion("");
    setDetailCoord(null);
  }, [group?.key, defaultPullVersion]);

  useEffect(() => {
    if (!open) {
      setDetailCoord(null);
    }
  }, [open]);

  const artifactCount = group?.artifacts.length ?? 0;
  const totalPages = Math.max(1, Math.ceil(artifactCount / PAGE_SIZE));
  useEffect(() => {
    setPage((prev) => Math.min(prev, totalPages));
  }, [totalPages]);

  const pageArtifacts = useMemo(() => {
    if (!group) return [];
    const safePage = Math.min(page, totalPages);
    const start = (safePage - 1) * PAGE_SIZE;
    return group.artifacts.slice(start, start + PAGE_SIZE);
  }, [group, page, totalPages]);

  const detailArtifact = useMemo(
    () =>
      detailCoord && group
        ? group.artifacts.find((a) => a.metadata.name === detailCoord) || null
        : null,
    [detailCoord, group],
  );

  if (!group) {
    return null;
  }

  function openVersionDetail(art: RegistryArtifact, isLocal: boolean) {
    setDetailCoord(art.metadata.name);
    if (
      !isLocal &&
      (!art.status?.publishedAt || !art.status?.changelog || !art.status?.upstreamURL)
    ) {
      void onHydrateArtifact(art.metadata.name);
    }
  }

  const groupSizeLabel =
    group.totalSizeBytes > 0 ? formatBytes(group.totalSizeBytes) : "0 B";

  const modalDescription = group.isGamePkg
    ? `共缓存 ${group.artifacts.length} 个版本 · 合计占用 ${groupSizeLabel}`
    : `${group.appLabel} · 共缓存 ${group.artifacts.length} 个版本 · 合计占用 ${groupSizeLabel}`;

  const modalTitle = (
    <span className="flex min-w-0 items-center gap-3">
      {group.iconUrl ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={group.iconUrl}
          alt=""
          className="h-10 w-10 shrink-0 rounded-lg object-cover ring-1 ring-[#E3E6F2]"
        />
      ) : (
        <span className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[#F4F6FC] text-[#5B6075] ring-1 ring-[#E3E6F2]">
          <PackageIcon className="h-4 w-4" />
        </span>
      )}
      <span className="min-w-0">
        <span className="block truncate">{group.title}</span>
        <span className="mt-0.5 block text-xs font-normal text-[#5B6075]">
          {modalDescription}
        </span>
      </span>
    </span>
  );

  async function handleInlinePull(e: FormEvent) {
    e.preventDefault();
    if (!group) return;
    const targetVer = (pullVersion === "__custom__" ? customVersion : pullVersion).trim();
    if (!targetVer) return;
    setPullBusy(true);
    try {
      await onPullGameVersion(group, targetVer);
      if (pullVersion === "__custom__") {
        setCustomVersion("");
      }
    } finally {
      setPullBusy(false);
    }
  }

  const subheader = detailArtifact ? (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        icon={<ArrowLeft className="h-4 w-4" />}
        onClick={() => setDetailCoord(null)}
      >
        返回版本列表
      </Button>
      <div className="flex items-center gap-2">
        {detailArtifact.status?.upstreamURL && (
          <a
            href={detailArtifact.status.upstreamURL}
            target="_blank"
            rel="noreferrer"
            className="inline-flex h-8 items-center gap-1.5 rounded-full border border-[#E3E6F2] bg-white px-3 text-xs font-medium text-[#3D4258] transition hover:border-[#2B3AE8]/40 hover:text-[#2B3AE8]"
          >
            <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
            查看上游发布页
          </a>
        )}
      </div>
    </div>
  ) : group.isGamePkg && !group.isLocalUpload ? (
      <form
        onSubmit={(e) => void handleInlinePull(e)}
        className="flex flex-wrap items-center justify-between gap-3"
      >
        <span className="text-xs font-medium text-[#5B6075]">拉取其他游戏版本</span>
        <div className="flex flex-1 flex-wrap items-center justify-end gap-2">
          {availableVersions.length > 0 && pullVersion !== "__custom__" ? (
            <div className="w-56">
              <Select
                aria-label="选择游戏版本号"
                value={pullVersion}
                onChange={(e) => setPullVersion(e.target.value)}
              >
                {availableVersions.map((v) => (
                  <option key={v} value={v}>
                    {existingVersions.has(v) ? `${v}（已缓存）` : v}
                  </option>
                ))}
                <option value="__custom__">手动输入其他版本号...</option>
              </Select>
            </div>
          ) : (
            <div className="flex items-center gap-2">
              <div className="w-48">
                <Input
                  aria-label="输入游戏版本号"
                  value={availableVersions.length > 0 ? customVersion : pullVersion}
                  onChange={(e) =>
                    availableVersions.length > 0
                      ? setCustomVersion(e.target.value)
                      : setPullVersion(e.target.value)
                  }
                  placeholder="例如 latest 或 1.21.4"
                  autoCorrect="off"
                  autoCapitalize="none"
                  spellCheck={false}
                />
              </div>
              {availableVersions.length > 0 && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => setPullVersion(defaultPullVersion)}
                >
                  返回
                </Button>
              )}
            </div>
          )}
          <Button
            type="submit"
            variant="primary"
            size="sm"
            loading={pullBusy}
            disabled={
              pullVersion === "__custom__" ? !customVersion.trim() : !pullVersion.trim()
            }
          >
            拉取
          </Button>
        </div>
      </form>
    ) : !group.isGamePkg && !group.isLocalUpload && onPullLatestAddon ? (
      <div className="flex flex-wrap items-center justify-between gap-3">
        {group.upstreamNewerVersion ? (
          <>
            <span className="text-xs font-medium text-[#3D4258]">
              检测到上游新版本{" "}
              <span className="text-[#2B3AE8]">
                {formatAddonVersion(group.upstreamNewerVersion)}
              </span>
            </span>
            <Button
              type="button"
              variant="primary"
              size="sm"
              loading={checkingAddon}
              disabled={Boolean(group.activeArtifact)}
              onClick={async () => {
                setCheckingAddon(true);
                try {
                  await onPullLatestAddon(group);
                } finally {
                  setCheckingAddon(false);
                }
              }}
            >
              拉取新版本
            </Button>
          </>
        ) : (
          <>
            <span className="text-xs text-[#5B6075]">
              检查创意工坊或上游源是否发布了新版本
            </span>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              icon={<RefreshCw className="h-3.5 w-3.5" />}
              loading={checkingAddon}
              disabled={Boolean(group.activeArtifact)}
              onClick={async () => {
                setCheckingAddon(true);
                try {
                  await onPullLatestAddon(group);
                } finally {
                  setCheckingAddon(false);
                }
              }}
            >
              检查更新
            </Button>
          </>
        )}
      </div>
    ) : group.isLocalUpload && group.ownerAppName ? (
      <div className="flex flex-wrap items-center justify-between gap-3">
        <span className="text-xs text-[#5B6075]">本地上传安装包由应用版本配置管理</span>
        <LinkButton
          href={`/console/admin/runtimes?app=${encodeURIComponent(group.ownerAppName)}&tab=versions`}
          variant="secondary"
          size="sm"
        >
          前往版本管理
        </LinkButton>
      </div>
    ) : undefined;

  const canCleanAll = group.artifacts.length > 1 && !group.isLocalUpload;

  const footer = detailArtifact ? (
    <div className="flex w-full flex-wrap items-center justify-between gap-3">
      <div className="flex items-center gap-2">
        {!(group.repo?.spec.mode === "Hosted" || Boolean(detailArtifact.spec.descriptor && !detailArtifact.spec.sourceURL)) && (
          <Button
            type="button"
            variant="secondary"
            size="sm"
            icon={<Terminal className="h-3.5 w-3.5" />}
            onClick={() => onOpenLogs(detailArtifact.metadata.name)}
          >
            查看日志
          </Button>
        )}
      </div>
      <Button type="button" variant="secondary" onClick={onClose}>
        关闭
      </Button>
    </div>
  ) : (
    <div className="flex w-full flex-wrap items-center justify-between gap-3">
      <div className="flex items-center gap-2">
        {canCleanAll && (
          <Button
            type="button"
            variant="danger"
            size="sm"
            onClick={() => onRequestClean(group.artifactNames, group.title)}
          >
            清理全部 {group.artifacts.length} 个版本
          </Button>
        )}
        {group.artifacts.length > PAGE_SIZE && (
          <div className="flex items-center gap-2 text-xs text-[#5B6075]">
            <span>
              第 {Math.min(page, totalPages)} / {totalPages} 页
            </span>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={page <= 1}
              onClick={() => setPage((p) => Math.max(1, p - 1))}
            >
              上一页
            </Button>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            >
              下一页
            </Button>
          </div>
        )}
      </div>
      <Button type="button" variant="secondary" onClick={onClose}>
        关闭
      </Button>
    </div>
  );

  return (
    <Modal
      open={open}
      onClose={onClose}
      width="max-w-3xl h-[min(640px,84dvh)]"
      title={modalTitle}
      subheader={subheader}
      footer={footer}
    >
      {detailArtifact ? (
        (() => {
          const dSize =
            detailArtifact.status?.descriptor?.sizeBytes ??
            detailArtifact.spec.descriptor?.sizeBytes;
          const dDigest =
            detailArtifact.status?.descriptor?.digest ?? detailArtifact.spec.descriptor?.digest;
          const dFmt =
            detailArtifact.status?.descriptor?.archiveFormat ||
            detailArtifact.spec.archiveFormat ||
            "-";
          const dVerTitle = group.isGamePkg
            ? detailArtifact.spec.version
            : formatAddonVersion(detailArtifact.spec.version);
          const isHydrating = hydratingCoord === detailArtifact.metadata.name;
          const changelog = (detailArtifact.status?.changelog || "").trim();

          return (
            <div className="space-y-5">
              <dl className="grid grid-cols-1 gap-x-6 gap-y-3 rounded-2xl bg-[#F7F8FC] px-5 py-4 text-xs sm:grid-cols-2">
                <div>
                  <dt className="text-[#8A90A6]">版本号</dt>
                  <dd className="mt-1 flex flex-wrap items-center gap-2 font-medium text-[#14161F]">
                    <span>{dVerTitle}</span>
                    {dVerTitle !== detailArtifact.spec.version && (
                      <Mono className="text-[11px] text-[#5B6075]">
                        {detailArtifact.spec.version}
                      </Mono>
                    )}
                  </dd>
                </div>
                <div>
                  <dt className="text-[#8A90A6]">构建标识</dt>
                  <dd className="mt-1 text-[#14161F]">
                    {detailArtifact.status?.build ? (
                      <Mono>{detailArtifact.status.build}</Mono>
                    ) : (
                      "-"
                    )}
                  </dd>
                </div>
                <div>
                  <dt className="text-[#8A90A6]">上游发布时间</dt>
                  <dd className="mt-1 font-mono tabular-nums text-[#14161F]">
                    {formatDateTime(detailArtifact.status?.publishedAt)}
                  </dd>
                </div>
                <div>
                  <dt className="text-[#8A90A6]">镜像同步时间</dt>
                  <dd className="mt-1 font-mono tabular-nums text-[#14161F]">
                    {formatDateTime(
                      detailArtifact.status?.syncedAt || detailArtifact.metadata.creationTimestamp,
                    )}
                  </dd>
                </div>
                <div>
                  <dt className="text-[#8A90A6]">归档格式与大小</dt>
                  <dd className="mt-1 tabular-nums text-[#14161F]">
                    {dFmt} · {formatByteValue(dSize)}
                  </dd>
                </div>
                <div>
                  <dt className="text-[#8A90A6]">内容摘要 (SHA-256)</dt>
                  <dd className="mt-1 flex items-center gap-2 text-[#14161F]">
                    {dDigest ? (
                      <>
                        <Mono className="truncate">{dDigest.slice(0, 20)}...</Mono>
                        <CopyButton text={dDigest} />
                      </>
                    ) : (
                      "-"
                    )}
                  </dd>
                </div>
                {((detailArtifact.spec.gameVersions?.length ?? 0) > 0 ||
                  (detailArtifact.spec.loaders?.length ?? 0) > 0) && (
                  <div className="sm:col-span-2">
                    <dt className="text-[#8A90A6]">适配游戏版本与加载器</dt>
                    <dd className="mt-1 text-[#14161F]">
                      {[
                        ...(detailArtifact.spec.gameVersions || []),
                        ...(detailArtifact.spec.loaders || []),
                      ].join(" · ")}
                    </dd>
                  </div>
                )}
                {((detailArtifact.status?.modIds?.length ?? 0) > 0 ||
                  (detailArtifact.status?.mapFolders?.length ?? 0) > 0) && (
                  <div className="sm:col-span-2">
                    <dt className="text-[#8A90A6]">模组标识 / 地图目录</dt>
                    <dd className="mt-1 flex flex-wrap gap-1.5 text-[#14161F]">
                      {(detailArtifact.status?.modIds || []).map((id) => (
                        <Mono
                          key={`mod-${id}`}
                          className="rounded bg-white px-1.5 py-0.5 text-[11px] ring-1 ring-[#E3E6F2]"
                        >
                          Mod: {id}
                        </Mono>
                      ))}
                      {(detailArtifact.status?.mapFolders || []).map((mf) => (
                        <Mono
                          key={`map-${mf}`}
                          className="rounded bg-white px-1.5 py-0.5 text-[11px] ring-1 ring-[#E3E6F2]"
                        >
                          Map: {mf}
                        </Mono>
                      ))}
                    </dd>
                  </div>
                )}
                {(detailArtifact.status?.dependencies?.length ?? 0) > 0 && (
                  <div className="sm:col-span-2">
                    <dt className="text-[#8A90A6]">前置依赖</dt>
                    <dd className="mt-1 text-[#14161F]">
                      {(detailArtifact.status?.dependencies || [])
                        .map((dep) => dep.title || dep.package)
                        .join("、")}
                    </dd>
                  </div>
                )}
              </dl>

              <section aria-label="版本更新日志">
                <h3 className="mb-2.5 text-xs font-semibold text-[#3D4258]">更新日志</h3>
                {isHydrating && !changelog ? (
                  <div className="flex items-center gap-2 rounded-2xl bg-[#F7F8FC] px-5 py-4 text-xs text-[#5B6075]">
                    <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
                    正在从上游同步版本说明...
                  </div>
                ) : changelog ? (
                  <AddonRichDescription content={changelog} />
                ) : (
                  <p className="rounded-2xl bg-[#F7F8FC] px-5 py-4 text-xs leading-relaxed text-[#8A90A6]">
                    {emptyChangelogHint(group, detailArtifact)}
                  </p>
                )}
              </section>
            </div>
          );
        })()
      ) : (
      <div
        tabIndex={0}
        role="region"
        aria-label={`${group.title} 版本列表`}
        className="-mx-6 -my-4 overflow-x-auto [scrollbar-gutter:stable] focus:outline-none"
      >
        <table className="w-full min-w-[660px] table-fixed text-left text-sm">
          <colgroup>
            <col className="w-[34%]" />
            <col className="w-[21%]" />
            <col className="w-[18%]" />
            <col className="w-[13%]" />
            <col className="w-[14%]" />
          </colgroup>
          <thead className="sticky top-0 z-10 bg-[#FAFBFD]">
            <tr className="border-b border-[#EEF0F7] text-[12px] font-semibold text-[#5B6075]">
              <th className="px-6 py-3">版本号</th>
              <th className="px-4 py-3">进度 / 大小</th>
              <th className="px-4 py-3">更新时间</th>
              <th className="px-4 py-3">状态</th>
              <th className="px-6 py-3 text-right">操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#EEF0F7]">
            {pageArtifacts.map((art) => {
              const coord = art.metadata.name;
              const phase = art.status?.phase || "Pending";
              const isSyncing = phase === "Pending" || phase === "Syncing";
              const isFailed = phase === "Failed";
              const isLocalUpload =
                group.repo?.spec.mode === "Hosted" ||
                Boolean(art.spec.descriptor && !art.spec.sourceURL);
              const progress = typeof art.status?.progress === "number" ? art.status.progress : 0;
              const processedBytes = art.status?.processedBytes ?? 0;
              const totalBytes = art.status?.totalBytes ?? 0;
              const speedBytesPerSec = art.status?.speedBytesPerSec ?? 0;
              const isIndeterminate =
                isSyncing &&
                (art.status?.stage === "Queued" ||
                  art.status?.stage === "Resolving" ||
                  art.status?.stage === "Archiving" ||
                  (progress <= 0 && totalBytes <= 0));
              const sizeBytes =
                art.status?.descriptor?.sizeBytes ?? art.spec.descriptor?.sizeBytes;
              const versionTitle = group.isGamePkg
                ? art.spec.version
                : formatAddonVersion(art.spec.version);
              const versionSubtitle = buildVersionSubtitle(group, art, versionTitle, isFailed);
              const stageText = artifactStageShortLabel(art);
              const telemetryDetail =
                speedBytesPerSec > 0
                  ? `${formatByteValue(speedBytesPerSec)}/s`
                  : totalBytes > 0
                    ? `${formatByteValue(processedBytes)} / ${formatByteValue(totalBytes)}`
                    : `${progress}%`;
              const busy = busyCoord === coord;
              const canChannelSync =
                !isLocalUpload &&
                !isSyncing &&
                !isFailed &&
                group.isGamePkg &&
                isFloatingVersionAlias(art.spec.version);
              const overflowItems: MenuItem[] = [];
              if (canChannelSync) {
                overflowItems.push({
                  label: "检查更新",
                  icon: <RefreshCw className="h-4 w-4" />,
                  disabled: busy,
                  onSelect: () => onRefreshArtifact(art),
                });
              }
              if (!isLocalUpload) {
                overflowItems.push({
                  label: "查看日志",
                  icon: <Terminal className="h-4 w-4" />,
                  onSelect: () => onOpenLogs(coord),
                });
              }
              overflowItems.push({
                label: "清理缓存",
                icon: <Trash2 className="h-4 w-4" />,
                danger: true,
                disabled: busy,
                onSelect: () => onRequestClean([coord], `${group.title} · ${art.spec.version}`),
              });

              return (
                <tr key={coord} className="transition-colors hover:bg-[#F7F8FC]/60">
                  <td className="px-6 py-3.5">
                    <div className="min-w-0">
                      <button
                        type="button"
                        onClick={() => openVersionDetail(art, isLocalUpload)}
                        className="block max-w-full truncate text-left font-semibold text-[#14161F] hover:text-[#2B3AE8]"
                        title={`查看 ${versionTitle} 详情与更新日志`}
                      >
                        {versionTitle}
                      </button>
                      {versionSubtitle && (
                        <div
                          className={`mt-0.5 truncate text-xs ${isFailed ? "text-[#B42318]" : "text-[#8A90A6]"}`}
                          title={versionSubtitle}
                        >
                          {versionSubtitle}
                        </div>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3.5 tabular-nums text-[#5B6075]">
                    {isSyncing ? (
                      <div className="flex flex-col gap-1 pr-2">
                        {isIndeterminate ? (
                          <progress
                            max={100}
                            className="gp-progress w-full"
                            aria-label="同步进度"
                          />
                        ) : (
                          <progress
                            max={100}
                            value={Math.min(100, Math.max(0, progress))}
                            className="gp-progress w-full"
                            aria-label="同步进度"
                          />
                        )}
                        <span className="truncate text-[11px] text-[#8A90A6]">
                          {stageText} · {telemetryDetail}
                        </span>
                      </div>
                    ) : (
                      formatByteValue(sizeBytes)
                    )}
                  </td>
                  <td className="px-4 py-3.5 font-mono text-xs tabular-nums text-[#5B6075]">
                    {formatDateTime(art.status?.syncedAt || art.metadata.creationTimestamp)}
                  </td>
                  <td className="px-4 py-3.5">
                    {isLocalUpload ? (
                      <Badge tone="neutral">本地上传</Badge>
                    ) : phase === "Ready" ? (
                      <Badge tone="success">已就绪</Badge>
                    ) : isFailed ? (
                      <Badge tone="danger">拉取失败</Badge>
                    ) : (
                      <Badge tone="accent">
                        <span className="inline-flex items-center gap-1">
                          <Loader2 className="h-3 w-3 animate-spin" />
                          {stageText}
                        </span>
                      </Badge>
                    )}
                  </td>
                  <td className="px-6 py-3.5 text-right">
                    <div className="inline-flex items-center justify-end gap-1 whitespace-nowrap">
                      {!isLocalUpload && isFailed && (
                        <IconButton
                          label={`重试拉取 ${versionTitle}`}
                          disabled={busy}
                          className="h-8 w-8 bg-transparent"
                          onClick={() => onRefreshArtifact(art)}
                        >
                          <RotateCcw className="h-4 w-4" aria-hidden="true" />
                        </IconButton>
                      )}
                      <IconButton
                        label="版本详情与更新日志"
                        className="h-8 w-8 bg-transparent"
                        onClick={() => openVersionDetail(art, isLocalUpload)}
                      >
                        <FileText className="h-4 w-4" aria-hidden="true" />
                      </IconButton>
                      <MoreMenu
                        label={`更多操作 (${versionTitle})`}
                        className="h-8 w-8 bg-transparent"
                        items={overflowItems}
                      />
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      )}
    </Modal>
  );
}
