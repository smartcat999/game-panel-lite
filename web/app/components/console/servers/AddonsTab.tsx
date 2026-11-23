"use client";

import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  ArrowLeft,
  ArrowUpCircle,
  Check,
  ChevronLeft,
  ChevronRight,
  ExternalLink,
  Link2,
  Loader2,
  Package,
  Plus,
  RotateCcw,
  Search,
  Settings2,
  SlidersHorizontal,
  Terminal,
  Trash2,
  X,
} from "lucide-react";
import { AddonConfigModal } from "./AddonConfigModal";
import { useConsole } from "../../../console/ConsoleProvider";
import { formatBytes, phaseOf, relativeTime } from "../../../console/lib/format";
import { errorMessage } from "../../../console/lib/session";
import { subscribeResourceWatch } from "../../../console/lib/ws";
import type {
  AppAddonBinding,
  AppItem,
  InstanceAddon,
  InstanceItem,
  RegistryArtifact,
  RegistryPackage,
} from "../../../console/lib/types";

interface ListResponse<T> {
  metadata?: {
    totalCount?: number;
  };
  items?: T[];
}
import {
  Badge,
  Button,
  Card,
  ConfirmDialog,
  EmptyState,
  IconButton,
  Input,
  Modal,
  Mono,
  MoreMenu,
  Segmented,
  Select,
  Skeleton,
  Switch,
  focusRing,
  type MenuItem,
} from "../ui";
import { ArtifactLogModal } from "../admin/ArtifactLogModal";
import { DownloadProxyModal } from "../admin/DownloadProxyModal";
import { AddonRichDescription } from "./AddonMarkdown";
import { useServerActions } from "./api";
import {
  addonBindingMeta,
  INSTANCE_APP_VERSION_ANNOTATION,
  INSTANCE_RESTART_REQUESTED_AT_ANNOTATION,
} from "./shared";

type SortMode = "downloads" | "trend" | "toprated" | "newest" | "updated";

function parseEnvMap(raw: unknown): Record<string, string> {
  if (Array.isArray(raw)) {
    return Object.fromEntries(
      (raw as Array<{ name: string; value: string }>).map((e) => [e.name, e.value]),
    );
  }
  return { ...((raw as Record<string, string>) || {}) };
}

function normalizePackageId(raw: string, isSteam: boolean): string {
  const trimmed = raw.trim();
  if (!trimmed) return "";
  if (isSteam && /^workshop-\d+$/i.test(trimmed)) {
    return trimmed.replace(/^workshop-/i, "");
  }
  return trimmed;
}

function looksLikeResolveInput(input: string, isSteam: boolean): boolean {
  const trimmed = input.trim();
  if (!trimmed) return false;
  if (
    /^https?:\/\//i.test(trimmed) ||
    trimmed.includes("steamcommunity.com/") ||
    trimmed.includes("modrinth.com/") ||
    trimmed.includes("thunderstore.io/") ||
    trimmed.includes("umod.org/") ||
    /^workshop-\d+$/i.test(trimmed)
  ) {
    return true;
  }
  if (isSteam && /^\d{6,}(?:\s*[,;\s]\s*\d{6,})*$/.test(trimmed)) {
    return true;
  }
  return false;
}

function formatCompactCount(n?: number): string {
  if (n === undefined || n === null || n <= 0) return "-";
  if (n >= 100_000_000) return `${(n / 100_000_000).toFixed(1)} 亿`;
  if (n >= 10_000) return `${(n / 10_000).toFixed(1)} 万`;
  return n.toLocaleString("zh-CN");
}

function formatAddonVersion(version: string | undefined): string {
  const v = (version || "").trim();
  if (!v || v === "1" || v.toLowerCase() === "latest") {
    return "默认版本";
  }
  if (/^\d{9,11}$/.test(v)) {
    const sec = Number(v);
    if (Number.isFinite(sec) && sec > 946684800) {
      const d = new Date(sec * 1000);
      const yyyy = d.getFullYear();
      const mm = String(d.getMonth() + 1).padStart(2, "0");
      const dd = String(d.getDate()).padStart(2, "0");
      return `${yyyy}-${mm}-${dd}`;
    }
  }
  return v;
}

function canonicalAddonConfig(cfg?: Record<string, string>): string {
  if (!cfg) return "";
  const entries = Object.entries(cfg).sort(([a], [b]) => a.localeCompare(b));
  return entries.length > 0 ? JSON.stringify(entries) : "";
}

function serializeAddons(list: InstanceAddon[]): string {
  return JSON.stringify(
    list.map((a) => [
      a.repository,
      a.package,
      a.version,
      a.installMode || "",
      a.targetDir || "",
      a.disabled ? "1" : "0",
      canonicalAddonConfig(a.config),
    ]),
  );
}

function distinctSummary(title: string | undefined, summary: string | undefined): string {
  const cleanSummary = (summary || "").trim();
  if (!cleanSummary) return "";
  const cleanTitle = (title || "").trim();
  if (cleanTitle && cleanSummary.toLowerCase() === cleanTitle.toLowerCase()) {
    return "";
  }
  return cleanSummary;
}

function AddonThumb({
  src,
  title,
  size = "h-11 w-11",
  inModal = false,
}: {
  src?: string;
  title: string;
  size?: string;
  inModal?: boolean;
}) {
  const [broken, setBroken] = useState(false);

  useEffect(() => {
    setBroken(false);
  }, [src]);

  if (!src || broken) {
    return (
      <div
        className={`flex ${size} shrink-0 items-center justify-center rounded-xl border border-[#E3E6F2] bg-[#F5F7FD] text-[#5B6075]`}
        aria-hidden
      >
        <Package className="h-5 w-5" />
      </div>
    );
  }

  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={src}
      alt={title}
      loading="lazy"
      {...(inModal ? { fetchPriority: "low" as const } : {})}
      onError={() => setBroken(true)}
      className={`${size} shrink-0 rounded-xl border border-[#E3E6F2] bg-[#F5F7FD] object-cover`}
    />
  );
}

function GalleryPreviewImage({ src, alt }: { src: string; alt: string }) {
  const [broken, setBroken] = useState(false);
  useEffect(() => {
    setBroken(false);
  }, [src]);
  if (broken) return null;
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={src}
      alt={alt}
      loading="lazy"
      fetchPriority="low"
      onError={() => setBroken(true)}
      className="h-36 w-60 shrink-0 rounded-xl border border-[#E3E6F2] bg-[#F5F7FD] object-cover"
    />
  );
}

function AddonDetailBody({
  item,
  loading,
  noun,
  idLabel,
  hideTitle = false,
  installedIds,
  pinnedVersion,
  versionOptions,
  compatibilityWarning,
  onSelectVersion,
}: {
  item: RegistryPackage | null;
  loading: boolean;
  noun: string;
  idLabel: string;
  hideTitle?: boolean;
  installedIds?: ReadonlySet<string>;
  pinnedVersion?: string;
  versionOptions?: Array<{ value: string; label: string }>;
  compatibilityWarning?: string;
  onSelectVersion?: (nextVersion: string) => void;
}) {
  if (loading && !item) {
    return (
      <div className="flex min-h-full items-center justify-center py-14 text-sm text-[#5B6075]">
        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
        正在加载{noun}详情…
      </div>
    );
  }
  if (!item) {
    return (
      <div className="flex min-h-full items-center justify-center py-10 text-center text-sm text-[#5B6075]">
        暂无法获取该{noun}的详情。
      </div>
    );
  }

  const gallery = (item.galleryUrls || []).filter((u) => u && u !== item.iconUrl);

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <AddonThumb src={item.iconUrl} title={item.title} size="h-14 w-14" inModal />
        <div className="min-w-0 flex-1 space-y-1.5">
          {!hideTitle && <h3 className="text-base font-semibold text-[#14161F]">{item.title}</h3>}
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[#5B6075]">
            {item.author && <span>作者：{item.author}</span>}
            <span>
              {idLabel}：<Mono className="text-[#14161F]">{item.package}</Mono>
            </span>
          </div>
          {item.tags && item.tags.length > 0 && (
            <div className="flex flex-wrap gap-1.5 pt-0.5">
              {item.tags.slice(0, 6).map((tag) => (
                <span
                  key={tag}
                  className="rounded-full bg-[#F1F3FB] px-2.5 py-0.5 text-[11px] text-[#3D4258]"
                >
                  {tag}
                </span>
              ))}
            </div>
          )}
        </div>
      </div>

      {compatibilityWarning && (
        <div
          role="status"
          className="flex items-center gap-2 rounded-2xl border border-[#FDE6B6] bg-[#FFFBEB] px-4 py-3 text-xs text-[#92400E]"
        >
          <AlertTriangle className="h-4 w-4 shrink-0 text-[#D97706]" aria-hidden />
          <span>{compatibilityWarning}</span>
        </div>
      )}

      {pinnedVersion && versionOptions && versionOptions.length > 0 && onSelectVersion && (
        <div className="flex flex-col gap-2 rounded-2xl bg-[#F7F8FC] px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
          <span className="text-xs font-medium text-[#14161F]">当前版本</span>
          <div className="w-full sm:w-72">
            <Select
              aria-label="选择当前版本"
              value={pinnedVersion}
              onChange={(e) => onSelectVersion(e.target.value)}
            >
              {versionOptions.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label}
                </option>
              ))}
            </Select>
          </div>
        </div>
      )}

      {gallery.length > 0 && (
        <div className="flex gap-2.5 overflow-x-auto pb-1">
          {gallery.slice(0, 5).map((imgUrl, idx) => (
            <GalleryPreviewImage
              key={imgUrl}
              src={imgUrl}
              alt={`${item.title} 预览图 ${idx + 1}`}
            />
          ))}
        </div>
      )}

      {(item.modIds?.length || item.mapFolders?.length) && (
        <dl className="grid grid-cols-1 gap-2 rounded-2xl bg-[#F7F8FC] px-4 py-3 text-xs sm:grid-cols-2">
          {item.modIds && item.modIds.length > 0 && (
            <div>
              <dt className="text-[#8A90A6]">模组标识</dt>
              <dd className="mt-0.5 font-mono text-[#14161F]">{item.modIds.join("; ")}</dd>
            </div>
          )}
          {item.mapFolders && item.mapFolders.length > 0 && (
            <div>
              <dt className="text-[#8A90A6]">地图目录</dt>
              <dd className="mt-0.5 font-mono text-[#14161F]">{item.mapFolders.join("; ")}</dd>
            </div>
          )}
        </dl>
      )}

      {item.dependencies && item.dependencies.length > 0 && (
        <div className="rounded-2xl bg-[#F7F8FC] px-4 py-3 text-xs">
          <div className="font-medium text-[#3D4258]">所需前置{noun}</div>
          <div className="mt-1.5 flex flex-wrap gap-2">
            {item.dependencies.map((dep) => {
              const isInstalled = installedIds?.has(dep.package);
              return (
                <span
                  key={dep.package}
                  className="inline-flex items-center gap-1.5 rounded-full bg-white px-3 py-1 text-[#14161F]"
                >
                  <span>{dep.title || dep.package}</span>
                  {isInstalled && <span className="font-medium text-[#1F7A45]">✓ 已安装</span>}
                </span>
              );
            })}
          </div>
        </div>
      )}

      <AddonRichDescription content={item.description || item.summary || ""} />
    </div>
  );
}

export function AddonsTab({
  inst,
  app,
  running,
}: {
  inst: InstanceItem;
  app: AppItem;
  running: boolean;
}) {
  const { apiFetch, toast, token, isAdmin } = useConsole();
  const { busy, updateSpec } = useServerActions();
  const isUpdating = busy[inst.metadata.name] === "update";

  const binding: AppAddonBinding | undefined = app.addonBinding;
  const repoName = binding?.repository || "";
  const { isSteam, noun, idLabel, marketLabel, browseActionLabel } = addonBindingMeta(binding);

  const instanceEnvMap = useMemo(() => parseEnvMap(inst.spec.env), [inst.spec.env]);
  const appVersion =
    inst.spec.serverArtifact?.version ||
    inst.metadata.annotations?.[INSTANCE_APP_VERSION_ANNOTATION] ||
    instanceEnvMap.VERSION ||
    app.recommendedVersion ||
    app.versions?.[0] ||
    "";
  const displayVersion =
    appVersion && appVersion.toLowerCase() !== "latest" ? appVersion : "";

  const savedAddons = useMemo<InstanceAddon[]>(
    () => (inst.spec.addons ? inst.spec.addons.map((a) => ({ ...a })) : []),
    [inst.spec.addons],
  );
  const savedSig = useMemo(() => serializeAddons(savedAddons), [savedAddons]);

  const [stagedAddons, setStagedAddons] = useState<InstanceAddon[]>(savedAddons);
  const lastSyncedSigRef = useRef<string>(`${inst.metadata.name}::${savedSig}`);

  useEffect(() => {
    const nextSig = `${inst.metadata.name}::${savedSig}`;
    if (lastSyncedSigRef.current === nextSig) return;
    lastSyncedSigRef.current = nextSig;
    setStagedAddons(savedAddons);
  }, [inst.metadata.name, savedAddons, savedSig]);

  const savedMap = useMemo(() => {
    const m = new Map<string, InstanceAddon>();
    for (const a of savedAddons) {
      m.set(a.package, a);
    }
    return m;
  }, [savedAddons]);

  const stagedMap = useMemo(() => {
    const m = new Map<string, InstanceAddon>();
    for (const a of stagedAddons) {
      m.set(a.package, a);
    }
    return m;
  }, [stagedAddons]);

  const dirty = useMemo(
    () => serializeAddons(stagedAddons) !== savedSig,
    [stagedAddons, savedSig],
  );

  const changedCount = useMemo(() => {
    let diff = 0;
    for (const a of stagedAddons) {
      const prev = savedMap.get(a.package);
      if (
        !prev ||
        prev.version !== a.version ||
        Boolean(prev.disabled) !== Boolean(a.disabled) ||
        canonicalAddonConfig(prev.config) !== canonicalAddonConfig(a.config)
      ) {
        diff++;
      }
    }
    for (const a of savedAddons) {
      if (!stagedMap.has(a.package)) diff++;
    }
    return Math.max(diff, dirty ? 1 : 0);
  }, [stagedAddons, savedAddons, savedMap, stagedMap, dirty]);

  const phase = phaseOf(inst);
  const pendingRestart =
    running &&
    ((inst.status?.observedGeneration ?? 0) < (inst.metadata.generation ?? 0) ||
      phase === "Starting" ||
      phase === "Restarting");

  // Hydration cache keyed by package ID
  const [hydratedMap, setHydratedMap] = useState<Record<string, RegistryPackage>>({});
  const [hydrating, setHydrating] = useState(false);
  const hydratedMapRef = useRef<Record<string, RegistryPackage>>({});
  hydratedMapRef.current = hydratedMap;

  // Package IDs whose latestVersion/availableVersions/dependencies were explicitly resolved for the active displayVersion
  const [versionHydratedIds, setVersionHydratedIds] = useState<Set<string>>(() => new Set());
  const versionHydratedIdsRef = useRef<Set<string>>(new Set());
  versionHydratedIdsRef.current = versionHydratedIds;
  const [stagingIds, setStagingIds] = useState<Set<string>>(() => new Set());

  // Artifacts in repository keyed by `${package}@${version}` and grouped by package
  const [repoArtifacts, setRepoArtifacts] = useState<RegistryArtifact[]>([]);
  const [retryingCoord, setRetryingCoord] = useState<string | null>(null);
  const [confirmRetryArt, setConfirmRetryArt] = useState<RegistryArtifact | null>(null);
  const [logArtifactId, setLogArtifactId] = useState<string | null>(null);

  // Installed table filter & inline resolve
  const cardHeadingId = useId();
  const filterInputId = useId();
  const [filterQuery, setFilterQuery] = useState("");
  const [resolvingInput, setResolvingInput] = useState(false);

  // Browse / Import modal state
  const browseHeadingId = useId();
  const browseSearchId = useId();
  const [browseOpen, setBrowseOpen] = useState(false);
  const [browseSort, setBrowseSort] = useState<SortMode>("downloads");
  const [browseQuery, setBrowseQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");
  const [resolvedMode, setResolvedMode] = useState(false);
  const [browsePage, setBrowsePage] = useState(1);
  const [catalogItems, setCatalogItems] = useState<RegistryPackage[] | null>(null);
  const [catalogTotal, setCatalogTotal] = useState(0);
  const [catalogLoading, setCatalogLoading] = useState(false);
  const [catalogError, setCatalogError] = useState<string | null>(null);
  const [catalogReloadSeq, setCatalogReloadSeq] = useState(0);
  const [proxyModalOpen, setProxyModalOpen] = useState(false);
  const lastCatalogKeyRef = useRef("");

  // Detail & version switch modal state
  const [detailTargetId, setDetailTargetId] = useState<string | null>(null);
  const [detailItem, setDetailItem] = useState<RegistryPackage | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailArtifacts, setDetailArtifacts] = useState<RegistryArtifact[]>([]);

  const sortOptions: Array<{ value: SortMode; label: string }> = useMemo(
    () =>
      isSteam
        ? [
            { value: "downloads", label: "最多订阅" },
            { value: "trend", label: "近期热门" },
            { value: "toprated", label: "最高评分" },
            { value: "updated", label: "最近更新" },
          ]
        : [
            { value: "downloads", label: "最多下载" },
            { value: "trend", label: "近期热门" },
            { value: "newest", label: "最新发布" },
            { value: "updated", label: "最近更新" },
          ],
    [isSteam],
  );

  const mergeHydratedPackages = useCallback(
    (items?: RegistryPackage[], versionResolved = false) => {
      if (!items || items.length === 0) return;
      const nextMap = { ...hydratedMapRef.current };
      const nextResolved = new Set(versionHydratedIdsRef.current);
      for (const item of items) {
        const id = normalizePackageId(item.package, isSteam);
        if (!id) continue;
        const existing = nextMap[id];
        if (!versionResolved && existing && nextResolved.has(id)) {
          nextMap[id] = {
            ...item,
            latestVersion: existing.latestVersion,
            availableVersions: existing.availableVersions,
            dependencies: existing.dependencies,
            sizeBytes: existing.sizeBytes || item.sizeBytes,
            description: existing.description || item.description,
          };
        } else {
          nextMap[id] = item;
        }
        if (versionResolved) {
          nextResolved.add(id);
        }
      }
      hydratedMapRef.current = nextMap;
      setHydratedMap(nextMap);
      if (versionResolved) {
        versionHydratedIdsRef.current = nextResolved;
        setVersionHydratedIds(nextResolved);
      }
    },
    [isSteam],
  );

  // Reset version-sensitive metadata when repository or active game version changes
  useEffect(() => {
    hydratedMapRef.current = {};
    versionHydratedIdsRef.current = new Set();
    setHydratedMap({});
    setVersionHydratedIds(new Set());
    setCatalogItems(null);
  }, [displayVersion, repoName]);

  // Hydrate installed package metadata via registry packages resolve
  const stagedPackageKey = useMemo(
    () => stagedAddons.map((a) => a.package).join(","),
    [stagedAddons],
  );

  useEffect(() => {
    if (!repoName || stagedAddons.length === 0) return;
    const missing = stagedAddons
      .map((a) => a.package)
      .filter((pkg) => pkg && !versionHydratedIdsRef.current.has(pkg));
    if (missing.length === 0) return;

    let cancelled = false;
    setHydrating(true);
    const params = new URLSearchParams({
      repository: repoName,
      resolve: missing.join(","),
    });
    if (displayVersion) params.set("gameVersion", displayVersion);

    void apiFetch(`/apis/registry.gamepanel.io/v1/packages?${params.toString()}`)
      .then(async (res) => {
        if (!res.ok || cancelled) return;
        const data = (await res.json()) as ListResponse<RegistryPackage>;
        if (!cancelled && data.items) {
          mergeHydratedPackages(data.items, true);
        }
      })
      .catch(() => {
        /* fallback to saved addon title/package */
      })
      .finally(() => {
        if (!cancelled) setHydrating(false);
      });

    return () => {
      cancelled = true;
    };
  }, [apiFetch, displayVersion, mergeHydratedPackages, repoName, stagedAddons, stagedPackageKey]);

  // Load artifacts for the bound repository to show sync state & cached sizes
  const loadRepoArtifacts = useCallback(async () => {
    if (!repoName) return;
    try {
      const params = new URLSearchParams({
        fieldSelector: `repository=${repoName}`,
        limit: "200",
      });
      const res = await apiFetch(`/apis/registry.gamepanel.io/v1/artifacts?${params.toString()}`);
      if (!res.ok) return;
      const data = (await res.json()) as ListResponse<RegistryArtifact>;
      setRepoArtifacts(data.items || []);
    } catch {
      /* ignore background status refresh error */
    }
  }, [apiFetch, repoName]);

  const retryFailedArtifact = useCallback(
    async (art: RegistryArtifact) => {
      setRetryingCoord(art.metadata.name);
      try {
        const res = await apiFetch(
          `/apis/registry.gamepanel.io/v1/artifacts/${encodeURIComponent(art.metadata.name)}`,
          {
            method: "PATCH",
            body: JSON.stringify({
              metadata: {
                annotations: {
                  "registry.gamepanel.io/retry-requested-at": new Date().toISOString(),
                },
              },
            }),
          },
        );
        if (!res.ok) throw new Error(await errorMessage(res, "重新触发同步失败"));
        toast("success", `已重新加入同步队列`);
        await loadRepoArtifacts();
      } catch (err) {
        toast("error", err instanceof Error ? err.message : "重试失败");
      } finally {
        setRetryingCoord(null);
      }
    },
    [apiFetch, loadRepoArtifacts, toast],
  );

  useEffect(() => {
    if (!repoName || stagedAddons.length === 0) return;
    void loadRepoArtifacts();
  }, [loadRepoArtifacts, repoName, stagedAddons.length, savedSig]);

  const artifactMap = useMemo(() => {
    const m = new Map<string, RegistryArtifact>();
    for (const art of repoArtifacts) {
      m.set(`${art.spec.package}@${art.spec.version}`, art);
    }
    return m;
  }, [repoArtifacts]);

  useEffect(() => {
    if (!repoName) return;
    const unsub = subscribeResourceWatch<RegistryArtifact>({
      path: "/apis/registry.gamepanel.io/v1/artifacts",
      token,
      onReconnect: loadRepoArtifacts,
      onEvent: (ev) => {
        const obj = ev.object;
        if (!obj?.metadata?.name || obj.spec?.repository !== repoName) return;
        setRepoArtifacts((prev) => {
          if (ev.type === "DELETED") {
            return prev.filter((item) => item.metadata.name !== obj.metadata.name);
          }
          const idx = prev.findIndex((item) => item.metadata.name === obj.metadata.name);
          if (idx >= 0) {
            const next = prev.slice();
            next[idx] = obj;
            return next;
          }
          return [...prev, obj];
        });
      },
    });
    return () => unsub();
  }, [loadRepoArtifacts, repoName, token]);

  const [configTargetPkg, setConfigTargetPkg] = useState<string | null>(null);

  const logArtifact = useMemo(
    () => (logArtifactId ? (repoArtifacts.find((a) => a.metadata.name === logArtifactId) ?? null) : null),
    [logArtifactId, repoArtifacts],
  );

  // Browse modal catalog loader (supports keyword search AND direct URL / collection resolve)
  useEffect(() => {
    if (!browseOpen || !repoName) return;
    const controller = new AbortController();
    const fetchKey = `${repoName}|${browseSort}|${browsePage}|${submittedQuery}|${displayVersion || ""}`;
    if (lastCatalogKeyRef.current !== fetchKey) {
      lastCatalogKeyRef.current = fetchKey;
      setCatalogError(null);
    }
    setCatalogLoading(true);

    const isResolve = looksLikeResolveInput(submittedQuery, isSteam);
    setResolvedMode(isResolve);

    const limit = 16;
    const params = new URLSearchParams({ repository: repoName });
    if (displayVersion) params.set("gameVersion", displayVersion);

    if (isResolve) {
      params.set("resolve", submittedQuery.trim());
    } else {
      params.set("sort", browseSort);
      params.set("limit", String(limit));
      params.set("offset", String((browsePage - 1) * limit));
      if (submittedQuery.trim()) {
        params.set("q", submittedQuery.trim());
      }
    }

    void apiFetch(`/apis/registry.gamepanel.io/v1/packages?${params.toString()}`, {
      signal: controller.signal,
    })
      .then(async (res) => {
        if (!res.ok) {
          throw new Error(await errorMessage(res, `加载${marketLabel}失败`));
        }
        const data = (await res.json()) as ListResponse<RegistryPackage>;
        if (controller.signal.aborted) return;
        const items = data.items || [];
        setCatalogError(null);
        setCatalogItems(items);
        setCatalogTotal(data.metadata?.totalCount ?? items.length);
        mergeHydratedPackages(items, isResolve);
      })
      .catch((err) => {
        if (
          controller.signal.aborted ||
          (err instanceof DOMException && err.name === "AbortError")
        ) {
          return;
        }
        setCatalogError(err instanceof Error ? err.message : `加载${marketLabel}失败`);
      })
      .finally(() => {
        if (!controller.signal.aborted) setCatalogLoading(false);
      });

    return () => {
      controller.abort();
    };
  }, [
    apiFetch,
    browseOpen,
    browsePage,
    browseSort,
    displayVersion,
    isSteam,
    marketLabel,
    mergeHydratedPackages,
    repoName,
    submittedQuery,
    catalogReloadSeq,
  ]);

  // Load package detail & cached artifact versions when detailTargetId is opened
  useEffect(() => {
    if (!detailTargetId || !repoName) {
      setDetailItem(null);
      setDetailArtifacts([]);
      return;
    }
    const cached = hydratedMapRef.current[detailTargetId];
    setDetailItem(cached ?? null);

    let cancelled = false;
    setDetailLoading(true);
    const coord = `${repoName}.${detailTargetId}`;
    const detailParams = new URLSearchParams();
    if (displayVersion) detailParams.set("gameVersion", displayVersion);
    const detailQuery = detailParams.toString();

    void Promise.all([
      apiFetch(
        `/apis/registry.gamepanel.io/v1/packages/${encodeURIComponent(coord)}${detailQuery ? `?${detailQuery}` : ""}`,
      ),
      apiFetch(
        `/apis/registry.gamepanel.io/v1/artifacts?fieldSelector=${encodeURIComponent(
          `repository=${repoName},package=${detailTargetId}`,
        )}&limit=50`,
      ),
    ])
      .then(async ([pkgRes, artRes]) => {
        if (cancelled) return;
        if (pkgRes.ok) {
          const pkg = (await pkgRes.json()) as RegistryPackage;
          setDetailItem(pkg);
          mergeHydratedPackages([pkg], true);
        }
        if (artRes.ok) {
          const artList = (await artRes.json()) as ListResponse<RegistryArtifact>;
          setDetailArtifacts(artList.items || []);
        }
      })
      .catch(() => {
        /* keep cached detail if available */
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [apiFetch, detailTargetId, displayVersion, mergeHydratedPackages, repoName]);

  const stagePackages = useCallback(
    async (incoming: RegistryPackage[]): Promise<number> => {
      if (!binding || !repoName || incoming.length === 0) return 0;
      const incomingIds = Array.from(
        new Set(incoming.map((p) => normalizePackageId(p.package, isSteam)).filter(Boolean)),
      );
      if (incomingIds.length === 0) return 0;

      setStagingIds((prev) => {
        const next = new Set(prev);
        for (const id of incomingIds) next.add(id);
        return next;
      });

      try {
        mergeHydratedPackages(incoming);

        // Search cards carry bare metadata without version lists/dependencies; resolve exact compatible version first.
        let resolvedIncomingMap: Record<string, RegistryPackage> = {};
        const needPreResolveIds = !isSteam
          ? incomingIds.filter((id) => !versionHydratedIdsRef.current.has(id))
          : [];
        if (needPreResolveIds.length > 0) {
          try {
            const params = new URLSearchParams({
              repository: repoName,
              resolve: needPreResolveIds.join(","),
            });
            if (displayVersion) params.set("gameVersion", displayVersion);
            const res = await apiFetch(`/apis/registry.gamepanel.io/v1/packages?${params.toString()}`);
            if (!res.ok) {
              throw new Error(await errorMessage(res, `解析${noun}版本失败`));
            }
            const list = (await res.json()) as ListResponse<RegistryPackage>;
            if (list.items) {
              mergeHydratedPackages(list.items, true);
              resolvedIncomingMap = Object.fromEntries(
                list.items.map((it) => [normalizePackageId(it.package, isSteam), it]),
              );
            }
          } catch (err) {
            toast("error", err instanceof Error ? err.message : `解析${noun}适配版本失败`);
            return 0;
          }
        }

        const resolvedItems = incoming.map((item) => {
          const cleanId = normalizePackageId(item.package, isSteam);
          return resolvedIncomingMap[cleanId] || hydratedMapRef.current[cleanId] || item;
        });

        const compatibleItems: RegistryPackage[] = [];
        for (const item of resolvedItems) {
          const cleanId = normalizePackageId(item.package, isSteam);
          if (!isSteam && displayVersion && !item.latestVersion?.trim()) {
            toast("error", `「${item.title || cleanId}」暂无适配 ${displayVersion} 的版本`);
            continue;
          }
          compatibleItems.push(item);
        }
        if (compatibleItems.length === 0) return 0;

        // Resolve any missing dependency versions for the active gameVersion
        const depIds: string[] = [];
        for (const pkg of compatibleItems) {
          for (const dep of pkg.dependencies || []) {
            const cleanDep = normalizePackageId(dep.package, isSteam);
            if (
              cleanDep &&
              !versionHydratedIdsRef.current.has(cleanDep) &&
              !compatibleItems.some((i) => normalizePackageId(i.package, isSteam) === cleanDep)
            ) {
              depIds.push(cleanDep);
            }
          }
        }
        let resolvedDeps: Record<string, RegistryPackage> = {};
        let depsFetchFailed = false;
        if (depIds.length > 0) {
          try {
            const params = new URLSearchParams({
              repository: repoName,
              resolve: Array.from(new Set(depIds)).join(","),
            });
            if (displayVersion) params.set("gameVersion", displayVersion);
            const res = await apiFetch(`/apis/registry.gamepanel.io/v1/packages?${params.toString()}`);
            if (res.ok) {
              const list = (await res.json()) as ListResponse<RegistryPackage>;
              if (list.items) {
                mergeHydratedPackages(list.items, true);
                resolvedDeps = Object.fromEntries(
                  list.items.map((it) => [normalizePackageId(it.package, isSteam), it]),
                );
              }
            } else {
              depsFetchFailed = true;
            }
          } catch {
            depsFetchFailed = true;
          }
        }

        let autoAddedDeps = 0;
        let stagedMainCount = 0;
        const skippedDeps: string[] = [];
        const existingNow = new Set(stagedMap.keys());
        for (const item of compatibleItems) {
          const cleanId = normalizePackageId(item.package, isSteam);
          for (const dep of item.dependencies || []) {
            const depId = normalizePackageId(dep.package, isSteam);
            if (!depId || existingNow.has(depId)) continue;
            const depMeta = resolvedDeps[depId] || hydratedMapRef.current[depId];
            if (!isSteam && displayVersion && !depMeta?.latestVersion?.trim() && !depsFetchFailed) {
              skippedDeps.push(depMeta?.title || dep.title || depId);
              continue;
            }
            existingNow.add(depId);
            autoAddedDeps++;
          }
          if (cleanId && !existingNow.has(cleanId)) {
            existingNow.add(cleanId);
            stagedMainCount++;
          }
        }

        setStagedAddons((prev) => {
          const next = [...prev];
          const existing = new Set(next.map((a) => a.package));

          for (const item of compatibleItems) {
            for (const dep of item.dependencies || []) {
              const depId = normalizePackageId(dep.package, isSteam);
              if (depId && !existing.has(depId)) {
                const depMeta = resolvedDeps[depId] || hydratedMapRef.current[depId];
                const depVer = depMeta?.latestVersion?.trim();
                if (!isSteam && displayVersion && !depVer && !depsFetchFailed) {
                  continue;
                }
                existing.add(depId);
                next.push({
                  repository: repoName,
                  package: depId,
                  version: depVer || (isSteam ? "1" : "latest"),
                  title: depMeta?.title || dep.title || depId,
                  installMode: binding.installMode,
                  targetDir: binding.targetDir,
                });
              }
            }
            const cleanId = normalizePackageId(item.package, isSteam);
            if (cleanId && !existing.has(cleanId)) {
              existing.add(cleanId);
              next.push({
                repository: repoName,
                package: cleanId,
                version: item.latestVersion?.trim() || (isSteam ? "1" : "latest"),
                title: item.title || cleanId,
                installMode: binding.installMode,
                targetDir: binding.targetDir,
              });
            }
          }
          return next;
        });

        if (autoAddedDeps > 0) {
          toast("info", `已自动补齐 ${autoAddedDeps} 项前置依赖${noun}`);
        }
        if (skippedDeps.length > 0 && displayVersion) {
          toast("error", `前置依赖「${skippedDeps[0]}」暂无适配 ${displayVersion} 的版本`);
        }
        return stagedMainCount + autoAddedDeps;
      } finally {
        setStagingIds((prev) => {
          const next = new Set(prev);
          for (const id of incomingIds) next.delete(id);
          return next;
        });
      }
    },
    [apiFetch, binding, displayVersion, isSteam, mergeHydratedPackages, noun, repoName, stagedMap, toast],
  );

  const updateAddonVersion = useCallback((pkgId: string, nextVersion: string) => {
    if (!nextVersion) return;
    setStagedAddons((prev) =>
      prev.map((a) => (a.package === pkgId ? { ...a, version: nextVersion } : a)),
    );
  }, []);

  const removeAddon = useCallback((pkgId: string) => {
    setStagedAddons((prev) => prev.filter((a) => a.package !== pkgId));
  }, []);

  const toggleAddonEnabled = useCallback((pkgId: string, enabled: boolean) => {
    setStagedAddons((prev) =>
      prev.map((a) => (a.package === pkgId ? { ...a, disabled: !enabled } : a)),
    );
  }, []);

  const handleDirectResolveAndAdd = useCallback(
    async (rawInput: string): Promise<boolean> => {
      const trimmed = rawInput.trim();
      if (!trimmed || !repoName || resolvingInput) return false;
      setResolvingInput(true);
      try {
        const params = new URLSearchParams({
          repository: repoName,
          resolve: trimmed,
        });
        if (displayVersion) params.set("gameVersion", displayVersion);
        const res = await apiFetch(`/apis/registry.gamepanel.io/v1/packages?${params.toString()}`);
        if (!res.ok) {
          throw new Error(await errorMessage(res, `解析${noun}链接失败`));
        }
        const data = (await res.json()) as ListResponse<RegistryPackage>;
        const items = data.items || [];
        if (items.length === 0) {
          toast("error", `未找到对应的${noun}，请检查链接或编号是否正确`);
          return false;
        }
        mergeHydratedPackages(items, true);
        const added = await stagePackages(items);
        if (added <= 0) {
          return false;
        }
        toast(
          "success",
          items.length > 1
            ? `已解析并加入 ${items.length} 项${noun}，点击「保存更改」生效`
            : `已加入「${items[0].title || items[0].package}」，点击「保存更改」生效`,
        );
        return true;
      } catch (err) {
        toast("error", err instanceof Error ? err.message : `解析${noun}失败`);
        return false;
      } finally {
        setResolvingInput(false);
      }
    },
    [apiFetch, displayVersion, mergeHydratedPackages, noun, repoName, resolvingInput, stagePackages, toast],
  );

  const handleSave = useCallback(async () => {
    if (!dirty || isUpdating) return;
    const ok = await updateSpec(
      inst,
      { addons: stagedAddons },
      running
        ? `已保存${noun}配置，下载完成后将自动生效`
        : `已保存${noun}配置，启动服务器时自动加载`,
      running ? { [INSTANCE_RESTART_REQUESTED_AT_ANNOTATION]: new Date().toISOString() } : undefined,
    );
    if (ok) {
      void loadRepoArtifacts();
      setDetailTargetId(null);
      setBrowseOpen(false);
    }
  }, [dirty, inst, isUpdating, loadRepoArtifacts, noun, running, stagedAddons, updateSpec]);

  const handleSaveSchemaConfig = useCallback(
    async (pkgId: string, nextConfig: Record<string, string> | undefined): Promise<boolean> => {
      const nextAddons = stagedAddons.map((a) => {
        if (a.package !== pkgId) return a;
        if (!nextConfig || Object.keys(nextConfig).length === 0) {
          const { config: _omit, ...rest } = a;
          void _omit;
          return rest;
        }
        return { ...a, config: nextConfig };
      });
      const hasOtherStaged =
        stagedAddons.some((a) => {
          const prev = savedMap.get(a.package);
          if (!prev) return true;
          if (a.package === pkgId) {
            return prev.version !== a.version || Boolean(prev.disabled) !== Boolean(a.disabled);
          }
          return (
            prev.version !== a.version ||
            Boolean(prev.disabled) !== Boolean(a.disabled) ||
            canonicalAddonConfig(prev.config) !== canonicalAddonConfig(a.config)
          );
        }) || savedAddons.some((a) => !stagedMap.has(a.package));

      setStagedAddons(nextAddons);
      if (hasOtherStaged) {
        toast("info", `已暂存${noun}配置，请点击「保存更改」一并生效`);
        return true;
      }
      const ok = await updateSpec(
        inst,
        { addons: nextAddons },
        running
          ? `已保存${noun}配置，重启服务器后生效`
          : `已保存${noun}配置，启动服务器时自动加载`,
        running ? { [INSTANCE_RESTART_REQUESTED_AT_ANNOTATION]: new Date().toISOString() } : undefined,
      );
      if (ok) {
        void loadRepoArtifacts();
      }
      return ok;
    },
    [inst, loadRepoArtifacts, noun, running, savedAddons, savedMap, stagedAddons, stagedMap, toast, updateSpec],
  );

  const filterKeyword = filterQuery.trim().toLowerCase();

  const visibleAddons = useMemo(() => {
    if (!filterKeyword) return stagedAddons;
    return stagedAddons.filter((addon) => {
      const meta = hydratedMap[addon.package];
      if (addon.package.toLowerCase().includes(filterKeyword)) return true;
      if (addon.title?.toLowerCase().includes(filterKeyword)) return true;
      if (meta?.title?.toLowerCase().includes(filterKeyword)) return true;
      if (meta?.author?.toLowerCase().includes(filterKeyword)) return true;
      return false;
    });
  }, [filterKeyword, hydratedMap, stagedAddons]);

  const closeBrowseModal = useCallback(() => {
    setDetailTargetId(null);
    setBrowseOpen(false);
  }, []);

  const detailVersionOptions = useMemo(() => {
    if (!detailTargetId) return [];
    const staged = stagedMap.get(detailTargetId);
    const seen = new Set<string>();
    const opts: Array<{ value: string; label: string }> = [];
    const availList = detailItem?.availableVersions || [];
    const availSet = new Set(availList);
    const versionChecked = Boolean(
      !isSteam && displayVersion && versionHydratedIds.has(detailTargetId),
    );
    const isConcreteVersion = (v?: string) =>
      Boolean(v && v !== "1" && v.toLowerCase() !== "latest");

    const pushOpt = (v: string | undefined, suffix: string) => {
      const clean = (v || "").trim();
      if (!clean || seen.has(clean)) return;
      seen.add(clean);
      const formatted = formatAddonVersion(clean);
      const baseLabel = /^\d{4}-\d{2}-\d{2}$/.test(formatted) ? `${formatted} 更新` : formatted;
      opts.push({
        value: clean,
        label: suffix ? `${baseLabel}（${suffix}）` : baseLabel,
      });
    };

    if (staged?.version) {
      const isIncompatible =
        versionChecked && isConcreteVersion(staged.version) && !availSet.has(staged.version);
      const suffix = isIncompatible
        ? `当前 · 不兼容 ${displayVersion}`
        : detailItem?.latestVersion && detailItem.latestVersion === staged.version
          ? "当前 · 最新"
          : "当前";
      pushOpt(staged.version, suffix);
    }
    if (detailItem?.latestVersion) {
      pushOpt(detailItem.latestVersion, "最新");
    }
    for (const ver of availList) {
      pushOpt(ver, !isSteam && displayVersion ? `适配 ${displayVersion}` : "");
    }
    for (const art of detailArtifacts) {
      if (art.status?.phase !== "Ready") continue;
      const gvs = art.spec.gameVersions || [];
      if (!isSteam && displayVersion && gvs.length > 0 && !gvs.includes(displayVersion)) {
        continue;
      }
      pushOpt(art.spec.version, "已缓存");
    }
    return opts;
  }, [
    detailArtifacts,
    detailItem?.availableVersions,
    detailItem?.latestVersion,
    detailTargetId,
    displayVersion,
    isSteam,
    stagedMap,
    versionHydratedIds,
  ]);

  const addonCompatibility = useCallback(
    (addon: InstanceAddon) => {
      const meta = hydratedMap[addon.package];
      const ready =
        !isSteam && Boolean(displayVersion) && versionHydratedIds.has(addon.package) && Boolean(meta);
      if (!ready) {
        return { incompatible: false, fixVersion: undefined as string | undefined };
      }
      const v = (addon.version || "").trim();
      if (!v || v === "1" || v.toLowerCase() === "latest") {
        return { incompatible: false, fixVersion: undefined };
      }
      const avail = meta?.availableVersions || [];
      if (avail.includes(v)) {
        return { incompatible: false, fixVersion: undefined };
      }
      const fixVersion = meta?.latestVersion?.trim() || undefined;
      return { incompatible: true, fixVersion };
    },
    [displayVersion, hydratedMap, isSteam, versionHydratedIds],
  );

  const { incompatibleCount, fixableUpdates } = useMemo(() => {
    let count = 0;
    const updates = new Map<string, string>();
    for (const addon of stagedAddons) {
      const { incompatible, fixVersion } = addonCompatibility(addon);
      if (!incompatible) continue;
      count++;
      if (fixVersion) {
        updates.set(addon.package, fixVersion);
      }
    }
    return { incompatibleCount: count, fixableUpdates: updates };
  }, [addonCompatibility, stagedAddons]);

  const switchAllToCompatible = useCallback(() => {
    if (fixableUpdates.size === 0) return;
    setStagedAddons((prev) =>
      prev.map((a) => {
        const nextVer = fixableUpdates.get(a.package);
        return nextVer ? { ...a, version: nextVer } : a;
      }),
    );
    toast(
      "success",
      `已切换 ${fixableUpdates.size} 项${noun}至适配 ${displayVersion} 的版本，点击「保存更改」生效`,
    );
  }, [displayVersion, fixableUpdates, noun, toast]);

  if (!binding || !repoName) return null;

  const totalPages = Math.max(1, Math.ceil((catalogTotal || 0) / 16));
  const inBrowseDetail = browseOpen && Boolean(detailTargetId);
  const isSteamUnconfigured =
    isSteam &&
    !submittedQuery &&
    !catalogLoading &&
    !catalogError &&
    catalogItems !== null &&
    catalogItems.length === 0;
  const isResolveDraft = looksLikeResolveInput(browseQuery, isSteam);
  const detailStaged = detailTargetId ? stagedMap.get(detailTargetId) : undefined;
  const detailVersionChecked = Boolean(
    !detailLoading &&
      !isSteam &&
      displayVersion &&
      detailTargetId &&
      versionHydratedIds.has(detailTargetId) &&
      detailItem,
  );
  const detailNoCompatibleVersion = Boolean(detailVersionChecked && !detailItem?.latestVersion?.trim());
  const detailIncompatiblePinned = Boolean(
    detailVersionChecked &&
      detailStaged?.version &&
      detailStaged.version !== "1" &&
      detailStaged.version.toLowerCase() !== "latest" &&
      !(detailItem?.availableVersions || []).includes(detailStaged.version),
  );
  const detailCompatWarning = detailNoCompatibleVersion
    ? `该${noun}暂无适配游戏版本 ${displayVersion} 的发布包。`
    : detailIncompatiblePinned
      ? `当前固定版本 ${formatAddonVersion(detailStaged?.version)} 不兼容游戏版本 ${displayVersion}，请在上方下拉列表切换兼容版本。`
      : undefined;

  const detailMetaLine = detailItem
    ? [
        detailItem.updatedAt ? `更新于 ${relativeTime(detailItem.updatedAt)}` : null,
        detailItem.sizeBytes ? formatBytes(detailItem.sizeBytes) : null,
        detailItem.downloads
          ? `${formatCompactCount(detailItem.downloads)} ${isSteam ? "订阅" : "下载"}`
          : null,
      ]
        .filter(Boolean)
        .join(" · ")
    : "";

  return (
    <>
      <Card className="overflow-hidden">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[#EEF0F7] px-6 py-4">
          <div className="flex items-baseline gap-2">
            <h2 id={cardHeadingId} className="text-base font-semibold text-[#14161F]">
              {noun}
            </h2>
            {stagedAddons.length > 0 && (
              <span className="text-xs font-medium tabular-nums text-[#8A90A6]">
                {stagedAddons.length}
              </span>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {stagedAddons.length > 0 && (
              <search className="w-48 sm:w-60">
                <form
                  aria-labelledby={cardHeadingId}
                  onSubmit={(e) => {
                    e.preventDefault();
                    const next = filterQuery.trim();
                    if (looksLikeResolveInput(next, isSteam)) {
                      void handleDirectResolveAndAdd(next).then((ok) => {
                        if (ok) setFilterQuery("");
                      });
                    }
                  }}
                  className="relative"
                >
                  <label htmlFor={filterInputId} className="sr-only">
                    搜索已安装{noun}
                  </label>
                  <Search
                    className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[#8A90A6]"
                    aria-hidden
                  />
                  <Input
                    id={filterInputId}
                    name="addon-filter"
                    type="search"
                    value={filterQuery}
                    onChange={(e) => setFilterQuery(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && (e.nativeEvent.isComposing || e.keyCode === 229)) {
                        e.preventDefault();
                      }
                    }}
                    autoCorrect="off"
                    autoCapitalize="none"
                    spellCheck={false}
                    enterKeyHint="search"
                    placeholder={`搜索已安装${noun}…`}
                    className="h-8 rounded-full border-[#DDE2F2] bg-[#F7F8FC] pl-8 pr-7 text-xs focus:bg-white [&::-webkit-search-cancel-button]:hidden"
                  />
                  {filterQuery && (
                    <button
                      type="button"
                      aria-label="清空搜索"
                      title="清空搜索"
                      onClick={() => setFilterQuery("")}
                      className={`absolute right-2 top-1/2 inline-flex h-4 w-4 -translate-y-1/2 items-center justify-center rounded-full text-[#8A90A6] transition-colors hover:bg-[#E7EAF5] hover:text-[#14161F] ${focusRing}`}
                    >
                      <X className="h-3 w-3" />
                    </button>
                  )}
                  <button type="submit" className="sr-only">
                    搜索
                  </button>
                </form>
              </search>
            )}

            {dirty && (
              <>
                <Button
                  variant="ghost"
                  size="sm"
                  icon={<RotateCcw className="h-3.5 w-3.5" />}
                  disabled={isUpdating}
                  onClick={() => setStagedAddons(savedAddons)}
                >
                  撤销
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  loading={isUpdating}
                  onClick={() => void handleSave()}
                >
                  保存更改 ({changedCount})
                </Button>
              </>
            )}

            <Button
              variant={dirty ? "secondary" : "primary"}
              size="sm"
              icon={<Plus className="h-3.5 w-3.5" />}
              onClick={() => {
                setDetailTargetId(null);
                setBrowseOpen(true);
              }}
            >
              {browseActionLabel}
            </Button>
          </div>
        </div>

        {incompatibleCount > 0 && displayVersion && (
          <div
            role="status"
            className="flex flex-wrap items-center justify-between gap-3 border-b border-[#FDE6B6] bg-[#FFFBEB] px-6 py-3 text-xs text-[#92400E]"
          >
            <div className="flex items-center gap-2">
              <AlertTriangle className="h-4 w-4 shrink-0 text-[#D97706]" aria-hidden />
              <span>
                检测到 {incompatibleCount} 项{noun}当前固定版本不适配游戏版本{" "}
                <Mono className="font-semibold text-[#92400E]">{displayVersion}</Mono>
                {fixableUpdates.size < incompatibleCount
                  ? `（其中 ${incompatibleCount - fixableUpdates.size} 项暂无 ${displayVersion} 适配版本）`
                  : "，建议切换至适配版本后保存。"}
              </span>
            </div>
            {fixableUpdates.size > 0 && (
              <Button
                size="sm"
                variant="secondary"
                disabled={isUpdating}
                onClick={switchAllToCompatible}
              >
                一键切换适配版本 ({fixableUpdates.size})
              </Button>
            )}
          </div>
        )}

        {stagedAddons.length === 0 ? (
          <EmptyState
            title={`暂未安装${noun}`}
            action={
              <Button
                variant="primary"
                size="sm"
                icon={<Plus className="h-3.5 w-3.5" />}
                onClick={() => {
                  setDetailTargetId(null);
                  setBrowseOpen(true);
                }}
              >
                {browseActionLabel}
              </Button>
            }
          />
        ) : visibleAddons.length === 0 ? (
          <div className="px-6 py-12 text-center">
            <p className="text-sm font-medium text-[#14161F]">
              未找到匹配的已安装{noun}「{filterQuery.trim()}」
            </p>
            <p className="mt-1 text-xs text-[#5B6075]">
              可清空筛选词，或前往{marketLabel}搜索添加。
            </p>
            <div className="mt-4 flex justify-center gap-2">
              <Button size="sm" variant="ghost" onClick={() => setFilterQuery("")}>
                清空筛选
              </Button>
              <Button
                size="sm"
                variant="secondary"
                onClick={() => {
                  const q = filterQuery.trim();
                  setFilterQuery("");
                  setBrowseQuery(q);
                  setSubmittedQuery(q);
                  setBrowsePage(1);
                  setDetailTargetId(null);
                  setBrowseOpen(true);
                }}
              >
                去{marketLabel}搜索
              </Button>
            </div>
          </div>
        ) : (
          <div
            role="region"
            aria-labelledby={cardHeadingId}
            tabIndex={0}
            className={`overflow-x-auto ${focusRing}`}
          >
            <table className="w-full min-w-[640px] text-left text-sm">
              <thead>
                <tr className="border-b border-[#EEF0F7] text-[13px] text-[#8A90A6]">
                  <th className="px-6 py-3.5 font-medium">名称</th>
                  <th className="px-4 py-3.5 font-medium">当前版本</th>
                  <th className="hidden px-4 py-3.5 font-medium sm:table-cell">大小</th>
                  <th className="px-4 py-3.5 font-medium">状态</th>
                  <th className="w-28 px-6 py-3.5 text-right font-medium">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#EEF0F7]">
                {visibleAddons.map((addon) => {
                  const meta = hydratedMap[addon.package];
                  const title = meta?.title || addon.title || (hydrating ? `加载中 (${addon.package})…` : addon.package);
                  const savedEntry = savedMap.get(addon.package);
                  const isRowStaged =
                    !savedEntry ||
                    savedEntry.version !== addon.version ||
                    Boolean(savedEntry.disabled) !== Boolean(addon.disabled) ||
                    canonicalAddonConfig(savedEntry.config) !== canonicalAddonConfig(addon.config);

                  const art = artifactMap.get(`${addon.package}@${addon.version}`);
                  const artPhase = art?.status?.phase;
                  const sizeBytes =
                    art?.status?.descriptor?.sizeBytes ||
                    art?.spec?.descriptor?.sizeBytes ||
                    meta?.sizeBytes ||
                    0;

                  const hasNewerUpstream = Boolean(
                    meta?.latestVersion &&
                      meta.latestVersion !== "1" &&
                      meta.latestVersion !== addon.version,
                  );
                  const compat = addonCompatibility(addon);

                  const subtitle = [
                    meta?.author?.trim(),
                    distinctSummary(title, meta?.summary),
                  ]
                    .filter(Boolean)
                    .join(" · ");

                  let statusBadge: { tone: "neutral" | "accent" | "success" | "warning" | "danger"; label: string };
                  if (isRowStaged) {
                    statusBadge = { tone: "warning", label: "待保存" };
                  } else if (addon.disabled) {
                    statusBadge = { tone: "neutral", label: "已停用" };
                  } else if (artPhase === "Failed") {
                    statusBadge = { tone: "danger", label: "下载失败" };
                  } else if (artPhase === "Pending" || artPhase === "Syncing") {
                    const isInspectingCached =
                      Boolean(art?.status?.descriptor?.digest) && art?.status?.stage !== "Downloading";
                    statusBadge = { tone: "warning", label: isInspectingCached ? "解析中" : "下载中" };
                  } else if (pendingRestart) {
                    statusBadge = { tone: "warning", label: "待重启" };
                  } else {
                    statusBadge = { tone: "success", label: "已安装" };
                  }

                  const overflowItems: MenuItem[] = [];
                  if (isAdmin && art && artPhase === "Failed" && !isRowStaged) {
                    overflowItems.push({
                      label: "重新下载",
                      icon: (
                        <RotateCcw
                          className={`h-4 w-4 ${retryingCoord === art.metadata.name ? "animate-spin" : ""}`}
                        />
                      ),
                      disabled: retryingCoord === art.metadata.name,
                      onSelect: () => setConfirmRetryArt(art),
                    });
                  }
                  overflowItems.push({
                    label: `${noun}配置`,
                    icon: <Settings2 className="h-4 w-4" />,
                    onSelect: () => setConfigTargetPkg(addon.package),
                  });
                  overflowItems.push({
                    label: "版本与详情",
                    icon: <SlidersHorizontal className="h-4 w-4" />,
                    onSelect: () => setDetailTargetId(addon.package),
                  });
                  if (art) {
                    overflowItems.push({
                      label: "查看日志",
                      icon: <Terminal className="h-4 w-4" />,
                      onSelect: () => setLogArtifactId(art.metadata.name),
                    });
                  }
                  overflowItems.push({
                    label: "移除",
                    icon: <Trash2 className="h-4 w-4" />,
                    danger: true,
                    disabled: isUpdating,
                    onSelect: () => removeAddon(addon.package),
                  });

                  return (
                    <tr key={addon.package} className="transition-colors hover:bg-[#FAFBFD]">
                      <td className="px-6 py-4">
                        <div className="flex items-center gap-3.5">
                          <AddonThumb src={meta?.iconUrl} title={title} />
                          <div className="min-w-0">
                            <button
                              type="button"
                              onClick={() => setDetailTargetId(addon.package)}
                              className={`block max-w-[320px] truncate text-left font-medium text-[#14161F] hover:text-[#2B3AE8] ${focusRing}`}
                            >
                              {title}
                            </button>
                            <div className="mt-0.5 max-w-[340px] truncate text-xs text-[#5B6075]">
                              {subtitle || <Mono className="text-[11px] text-[#8A90A6]">{addon.package}</Mono>}
                            </div>
                          </div>
                        </div>
                      </td>

                      <td className="px-4 py-4">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="tabular-nums text-[13px] text-[#14161F]">
                            {formatAddonVersion(addon.version)}
                          </span>
                          {compat.incompatible && displayVersion ? (
                            compat.fixVersion ? (
                              <>
                                <Badge tone="warning">不兼容 {displayVersion}</Badge>
                                <button
                                  type="button"
                                  onClick={() => updateAddonVersion(addon.package, compat.fixVersion!)}
                                  title={`切换至适配 ${displayVersion} 的版本 ${formatAddonVersion(compat.fixVersion)}`}
                                  className={`inline-flex items-center gap-1 rounded-full bg-[#FEF3C7] px-2.5 py-0.5 text-xs font-medium text-[#92400E] transition-colors hover:bg-[#FDE68A] ${focusRing}`}
                                >
                                  <ArrowUpCircle className="h-3.5 w-3.5" />
                                  切换适配版本
                                </button>
                              </>
                            ) : (
                              <Badge tone="danger">不兼容 {displayVersion}</Badge>
                            )
                          ) : (
                            hasNewerUpstream &&
                            meta?.latestVersion && (
                              <button
                                type="button"
                                onClick={() => updateAddonVersion(addon.package, meta.latestVersion!)}
                                title={`更新至新版本 ${formatAddonVersion(meta.latestVersion)}`}
                                className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium text-[#2B3AE8] transition-colors hover:bg-[#EEF1FF] ${focusRing}`}
                              >
                                <ArrowUpCircle className="h-3.5 w-3.5" />
                                更新
                              </button>
                            )
                          )}
                        </div>
                      </td>

                      <td className="hidden px-4 py-4 tabular-nums text-[13px] text-[#5B6075] sm:table-cell">
                        {sizeBytes > 0 ? formatBytes(sizeBytes) : "-"}
                      </td>

                      <td className="px-4 py-4">
                        <Badge tone={statusBadge.tone}>{statusBadge.label}</Badge>
                      </td>

                      <td className="px-6 py-4 text-right">
                        <div className="inline-flex items-center justify-end gap-2">
                          <Switch
                            checked={!addon.disabled}
                            onChange={(next) => toggleAddonEnabled(addon.package, next)}
                            label={addon.disabled ? `启用 ${title}` : `停用 ${title}`}
                            disabled={isUpdating}
                          />
                          <MoreMenu
                            label={`更多操作 (${title})`}
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
      </Card>

      {/* Browse & Link/Collection Import Modal */}
      <Modal
        open={browseOpen}
        onClose={closeBrowseModal}
        width={isSteamUnconfigured && !inBrowseDetail ? "max-w-xl" : "max-w-4xl h-[min(700px,85dvh)]"}
        title={browseActionLabel}
        description={
          displayVersion ? `${app.displayName} · 适配版本 ${displayVersion}` : app.displayName
        }
        subheader={
          inBrowseDetail ? (
            <div className="flex flex-wrap items-center justify-between gap-2">
              <button
                type="button"
                onClick={() => setDetailTargetId(null)}
                className={`inline-flex items-center gap-1.5 rounded-full text-sm font-medium text-[#5B6075] hover:text-[#2B3AE8] ${focusRing}`}
              >
                <ArrowLeft className="h-4 w-4" />
                返回列表
              </button>
              {detailMetaLine && (
                <span className="text-xs tabular-nums text-[#8A90A6]">{detailMetaLine}</span>
              )}
            </div>
          ) : isSteamUnconfigured ? undefined : (
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <Segmented<SortMode>
                label="排序方式"
                value={browseSort}
                onChange={(v) => {
                  setBrowseSort(v);
                  setBrowsePage(1);
                }}
                items={sortOptions}
              />
              <search className="w-full sm:w-[360px]">
                <span id={browseHeadingId} className="sr-only">
                  搜索或粘贴链接解析{noun}
                </span>
                <form
                  aria-labelledby={browseHeadingId}
                  onSubmit={(e) => {
                    e.preventDefault();
                    setSubmittedQuery(browseQuery.trim());
                    setBrowsePage(1);
                  }}
                  className="flex h-10 w-full items-center gap-1.5 rounded-full border border-[#DDE2F2] bg-[#F7F8FC] pl-3.5 pr-1 transition-[border-color,background-color,box-shadow] hover:border-[#B8C0E6] focus-within:border-[#2B3AE8] focus-within:bg-white focus-within:ring-4 focus-within:ring-[#2B3AE8]/15"
                >
                  <label htmlFor={browseSearchId} className="sr-only">
                    {isSteam ? "搜索模组名称，或粘贴工坊单品与合集链接" : `搜索${noun}名称，或粘贴${noun}链接`}
                  </label>
                  {isResolveDraft ? (
                    <Link2 className="h-4 w-4 shrink-0 text-[#2B3AE8]" aria-hidden />
                  ) : (
                    <Search className="h-4 w-4 shrink-0 text-[#8A90A6]" aria-hidden />
                  )}
                  <Input
                    variant="bare"
                    id={browseSearchId}
                    name="q"
                    type="search"
                    value={browseQuery}
                    onChange={(e) => setBrowseQuery(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && (e.nativeEvent.isComposing || e.keyCode === 229)) {
                        e.preventDefault();
                      }
                    }}
                    autoCorrect="off"
                    autoCapitalize="none"
                    spellCheck={false}
                    enterKeyHint="search"
                    placeholder={
                      isSteam
                        ? "搜索名称，或粘贴单品 / 合集链接…"
                        : `搜索${noun}名称，或粘贴链接…`
                    }
                    className="flex-1 text-[13px] [&::-webkit-search-cancel-button]:hidden"
                  />
                  {(browseQuery || submittedQuery) && (
                    <button
                      type="button"
                      aria-label="清空搜索"
                      title="清空搜索"
                      onClick={() => {
                        setBrowseQuery("");
                        if (submittedQuery !== "") {
                          setSubmittedQuery("");
                          setBrowsePage(1);
                        }
                      }}
                      className={`inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[#8A90A6] transition-colors hover:bg-[#E7EAF5] hover:text-[#14161F] ${focusRing}`}
                    >
                      <X className="h-3.5 w-3.5" />
                    </button>
                  )}
                  <button
                    type="submit"
                    disabled={catalogLoading && catalogItems !== null}
                    className={`inline-flex h-8 shrink-0 items-center justify-center gap-1.5 rounded-full bg-white px-3.5 text-[13px] font-medium shadow-[0_2px_6px_-2px_rgba(20,22,31,0.14)] transition-[color,transform] hover:text-[#2B3AE8] active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-60 ${
                      isResolveDraft ? "text-[#2B3AE8]" : "text-[#14161F]"
                    } ${focusRing}`}
                  >
                    {catalogLoading && catalogItems !== null && (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    )}
                    {isResolveDraft ? "解析" : "搜索"}
                  </button>
                </form>
              </search>
            </div>
          )
        }
        footer={
          inBrowseDetail && detailTargetId ? (
            <div className="flex w-full flex-wrap items-center justify-between gap-3">
              <div>
                {detailItem?.sourceUrl && (
                  <a
                    href={detailItem.sourceUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className={`inline-flex items-center gap-1.5 text-xs font-medium text-[#5B6075] hover:text-[#2B3AE8] ${focusRing}`}
                  >
                    <ExternalLink className="h-3.5 w-3.5" />
                    查看{isSteam ? "工坊主页" : `${noun}主页`}
                  </a>
                )}
              </div>
              <div className="flex items-center gap-2.5">
                <Button variant="ghost" onClick={() => setDetailTargetId(null)}>
                  返回列表
                </Button>
                {stagedMap.has(detailTargetId) ? (
                  <Button variant="danger" onClick={() => removeAddon(detailTargetId)}>
                    移除
                  </Button>
                ) : (
                  <Button
                    variant={dirty ? "secondary" : "primary"}
                    icon={<Plus className="h-4 w-4" />}
                    loading={stagingIds.has(detailTargetId)}
                    disabled={
                      detailLoading ||
                      stagingIds.has(detailTargetId) ||
                      detailNoCompatibleVersion
                    }
                    onClick={() => {
                      if (detailItem) {
                        void stagePackages([detailItem]);
                      }
                    }}
                  >
                    添加
                  </Button>
                )}
                {dirty && (
                  <Button
                    variant="primary"
                    loading={isUpdating}
                    onClick={() => void handleSave()}
                  >
                    保存更改 ({changedCount})
                  </Button>
                )}
              </div>
            </div>
          ) : (
            <div className="flex w-full flex-wrap items-center justify-between gap-3">
              <div className="flex items-center gap-2 text-xs text-[#5B6075]">
                {!resolvedMode && !catalogError && catalogTotal > 0 && (
                  <>
                    <IconButton
                      label="上一页"
                      disabled={browsePage <= 1 || catalogLoading}
                      onClick={() => setBrowsePage((p) => Math.max(1, p - 1))}
                      className="h-8 w-8"
                    >
                      <ChevronLeft className="h-4 w-4" />
                    </IconButton>
                    <span className="tabular-nums">
                      第 {browsePage} / {totalPages} 页（共 {catalogTotal.toLocaleString("zh-CN")} 项）
                    </span>
                    <IconButton
                      label="下一页"
                      disabled={browsePage >= totalPages || catalogLoading}
                      onClick={() => setBrowsePage((p) => p + 1)}
                      className="h-8 w-8"
                    >
                      <ChevronRight className="h-4 w-4" />
                    </IconButton>
                  </>
                )}
              </div>
              <div className="flex items-center gap-2.5">
                <Button variant="ghost" onClick={closeBrowseModal}>
                  关闭
                </Button>
                {dirty && (
                  <Button
                    variant="primary"
                    loading={isUpdating}
                    onClick={() => void handleSave()}
                  >
                    保存更改 ({changedCount})
                  </Button>
                )}
              </div>
            </div>
          )
        }
      >
        {inBrowseDetail ? (
          <AddonDetailBody
            item={detailItem}
            loading={detailLoading}
            noun={noun}
            idLabel={idLabel}
            installedIds={new Set(stagedAddons.map((a) => a.package))}
            pinnedVersion={detailStaged?.version}
            versionOptions={detailVersionOptions}
            compatibilityWarning={detailCompatWarning}
            onSelectVersion={
              detailTargetId ? (v) => updateAddonVersion(detailTargetId, v) : undefined
            }
          />
        ) : catalogLoading && catalogItems === null ? (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {Array.from({ length: 6 }).map((_, idx) => (
              <Skeleton key={idx} className="h-[104px] w-full rounded-2xl" />
            ))}
          </div>
        ) : catalogError ? (
          <div className="flex min-h-full flex-col items-center justify-center px-6 py-12 text-center">
            <p className="text-sm font-medium text-[#14161F]">
              {isSteam ? "暂时无法加载创意工坊列表" : `暂时无法加载${noun}列表`}
            </p>
            <p className="mt-1 max-w-[36ch] break-words text-xs leading-relaxed text-[#5B6075]">
              {catalogError}
            </p>
            <div className="mt-4 flex flex-wrap items-center justify-center gap-2">
              <Button
                variant="secondary"
                size="sm"
                loading={catalogLoading}
                icon={<RotateCcw className="h-3.5 w-3.5" />}
                onClick={() => setCatalogReloadSeq((n) => n + 1)}
              >
                重新加载
              </Button>
              {submittedQuery && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    setBrowseQuery("");
                    setSubmittedQuery("");
                    setBrowsePage(1);
                  }}
                >
                  清空搜索
                </Button>
              )}
            </div>
          </div>
        ) : !catalogItems?.length ? (
          <div className={isSteam && !submittedQuery ? "py-1" : "flex min-h-full flex-col items-center justify-center text-center"}>
            {isSteam && !submittedQuery ? (
              <div className="space-y-4">
                <div className="rounded-2xl bg-[#F7F8FC] p-4 sm:p-5">
                  <div className="text-sm font-semibold text-[#14161F]">
                    粘贴创意工坊链接或数字 ID 添加
                  </div>
                  <p className="mt-1 text-xs leading-relaxed text-[#5B6075]">
                    支持单品链接、合集链接或纯数字 ID（如 2804469978），无需配置密钥即可直接解析安装
                  </p>
                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      if (browseQuery.trim()) {
                        setSubmittedQuery(browseQuery.trim());
                        setBrowsePage(1);
                      }
                    }}
                    className="mt-3.5 flex items-center gap-2"
                  >
                    <Input
                      value={browseQuery}
                      onChange={(e) => setBrowseQuery(e.target.value)}
                      placeholder="粘贴工坊链接或数字 ID，如 2804469978"
                      className="min-w-0 flex-1"
                    />
                    <Button type="submit" variant="primary" className="shrink-0">
                      解析
                    </Button>
                  </form>
                </div>

                <div className="flex flex-col gap-2.5 rounded-2xl border border-[#E3E6F2] bg-white px-4 py-3 text-xs text-[#5B6075] sm:flex-row sm:items-center sm:justify-between">
                  <span>未配置 Steam Web API 密钥时暂不支持关键词搜索与热门榜单浏览。</span>
                  {isAdmin && (
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      className="shrink-0 self-start sm:self-auto"
                      onClick={() => setProxyModalOpen(true)}
                    >
                      立即配置密钥
                    </Button>
                  )}
                </div>
              </div>
            ) : (
              <>
                <p className="text-sm text-[#5B6075]">未找到匹配的{noun}，请更换关键词或链接重试。</p>
                {submittedQuery && (
                  <Button
                    size="sm"
                    variant="ghost"
                    className="mt-3"
                    onClick={() => {
                      setBrowseQuery("");
                      setSubmittedQuery("");
                      setBrowsePage(1);
                    }}
                  >
                    返回列表
                  </Button>
                )}
              </>
            )}
          </div>
        ) : (
          <div className="space-y-3">
            {resolvedMode && catalogItems.length > 1 && (
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-2xl bg-[#F7F8FC] px-4 py-2.5 text-xs text-[#3D4258]">
                <span>已解析到 {catalogItems.length} 项{noun}</span>
                <Button
                  size="sm"
                  variant="secondary"
                  icon={<Plus className="h-3.5 w-3.5" />}
                  onClick={() => void stagePackages(catalogItems)}
                >
                  全部添加
                </Button>
              </div>
            )}

            <div
              className={`grid grid-cols-1 gap-3 transition-opacity duration-150 sm:grid-cols-2 ${
                catalogLoading ? "pointer-events-none opacity-55" : "opacity-100"
              }`}
            >
              {catalogItems.map((item) => {
                const cleanId = normalizePackageId(item.package, isSteam);
                const added = stagedMap.has(cleanId);
                const cardSummary = distinctSummary(item.title, item.summary);
                return (
                  <div
                    key={item.package}
                    className="flex min-h-[104px] items-start justify-between gap-3 rounded-2xl border border-[#EEF0F7] bg-[#FAFBFD] p-3.5 transition-colors hover:border-[#D5DAEE] hover:bg-white"
                  >
                    <div className="flex min-w-0 flex-1 items-start gap-3">
                      <AddonThumb
                        src={item.iconUrl}
                        title={item.title}
                        size="h-12 w-12"
                        inModal
                      />
                      <div className="min-w-0 flex-1">
                        <button
                          type="button"
                          onClick={() => setDetailTargetId(cleanId)}
                          className={`block max-w-full truncate text-left text-sm font-semibold text-[#14161F] hover:text-[#2B3AE8] ${focusRing}`}
                        >
                          {item.title}
                        </button>
                        {cardSummary && (
                          <p className="mt-0.5 line-clamp-2 text-xs leading-relaxed text-[#5B6075]">
                            {cardSummary}
                          </p>
                        )}
                        <div className="mt-1.5 flex flex-wrap items-center gap-x-2.5 gap-y-0.5 text-[11px] tabular-nums text-[#8A90A6]">
                          {item.author && <span>{item.author}</span>}
                          {item.downloads ? (
                            <span>
                              {formatCompactCount(item.downloads)} {isSteam ? "订阅" : "下载"}
                            </span>
                          ) : null}
                          {item.sizeBytes ? <span>{formatBytes(item.sizeBytes)}</span> : null}
                        </div>
                      </div>
                    </div>

                    <div className="shrink-0 pt-0.5">
                      {added ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          icon={<Check className="h-3.5 w-3.5 text-[#1F9D55]" />}
                          onClick={() => removeAddon(cleanId)}
                        >
                          已添加
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          variant="secondary"
                          icon={<Plus className="h-3.5 w-3.5" />}
                          loading={stagingIds.has(cleanId)}
                          disabled={stagingIds.has(cleanId)}
                          onClick={() => void stagePackages([item])}
                        >
                          添加
                        </Button>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </Modal>

      {/* Standalone Detail & Version Switch Modal (opened from installed table) */}
      <Modal
        open={!browseOpen && Boolean(detailTargetId)}
        onClose={() => setDetailTargetId(null)}
        width="max-w-2xl h-[min(660px,84dvh)]"
        title={detailItem?.title || detailStaged?.title || detailTargetId || `${noun}详情`}
        description={detailMetaLine || undefined}
        footer={
          detailTargetId ? (
            <div className="flex w-full flex-wrap items-center justify-between gap-3">
              <div>
                {detailItem?.sourceUrl && (
                  <a
                    href={detailItem.sourceUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className={`inline-flex items-center gap-1.5 text-xs font-medium text-[#5B6075] hover:text-[#2B3AE8] ${focusRing}`}
                  >
                    <ExternalLink className="h-3.5 w-3.5" />
                    查看{isSteam ? "工坊主页" : `${noun}主页`}
                  </a>
                )}
              </div>
              <div className="flex items-center gap-2">
                <Button variant="ghost" onClick={() => setDetailTargetId(null)}>
                  关闭
                </Button>
                {stagedMap.has(detailTargetId) ? (
                  <Button
                    variant="danger"
                    onClick={() => {
                      removeAddon(detailTargetId);
                      setDetailTargetId(null);
                    }}
                  >
                    移除
                  </Button>
                ) : (
                  <Button
                    variant="primary"
                    icon={<Plus className="h-4 w-4" />}
                    loading={stagingIds.has(detailTargetId)}
                    disabled={
                      detailLoading ||
                      stagingIds.has(detailTargetId) ||
                      detailNoCompatibleVersion
                    }
                    onClick={() => {
                      if (detailItem) {
                        void stagePackages([detailItem]).then((n) => {
                          if (n > 0) setDetailTargetId(null);
                        });
                      }
                    }}
                  >
                    添加
                  </Button>
                )}
                {dirty && (
                  <Button
                    variant="primary"
                    loading={isUpdating}
                    onClick={() => void handleSave()}
                  >
                    保存更改 ({changedCount})
                  </Button>
                )}
              </div>
            </div>
          ) : undefined
        }
      >
        <AddonDetailBody
          item={detailItem}
          loading={detailLoading}
          noun={noun}
          idLabel={idLabel}
          hideTitle
          installedIds={new Set(stagedAddons.map((a) => a.package))}
          pinnedVersion={detailStaged?.version}
          versionOptions={detailVersionOptions}
          compatibilityWarning={detailCompatWarning}
          onSelectVersion={
            detailTargetId ? (v) => updateAddonVersion(detailTargetId, v) : undefined
          }
        />
      </Modal>

      <ArtifactLogModal
        open={!!logArtifact}
        onClose={() => setLogArtifactId(null)}
        artifact={logArtifact}
        title={
          logArtifact
            ? hydratedMap[logArtifact.spec.package]?.title || logArtifact.spec.package
            : undefined
        }
        token={token}
      />

      {(() => {
        const configAddon = configTargetPkg ? (stagedMap.get(configTargetPkg) ?? null) : null;
        const configPkgMeta = configAddon ? hydratedMap[configAddon.package] : undefined;
        const configArtifact = configAddon
          ? artifactMap.get(`${configAddon.package}@${configAddon.version}`)
          : undefined;
        return (
          <AddonConfigModal
            open={!!configTargetPkg && !!configAddon}
            onClose={() => setConfigTargetPkg(null)}
            instName={inst.metadata.name}
            running={running}
            noun={noun}
            addon={configAddon}
            pkgMeta={configPkgMeta}
            artifact={configArtifact}
            isAdmin={isAdmin}
            retrying={!!configArtifact && retryingCoord === configArtifact.metadata.name}
            onResyncArtifact={isAdmin ? retryFailedArtifact : undefined}
            onSaveSchemaConfig={handleSaveSchemaConfig}
            savingSchema={isUpdating}
          />
        );
      })()}
      <ConfirmDialog
        open={!!confirmRetryArt}
        onClose={() => {
          if (!retryingCoord) setConfirmRetryArt(null);
        }}
        title={`确认重新下载${noun}包`}
        body={
          confirmRetryArt
            ? `将重新同步“${confirmRetryArt.spec.package}”到对象存储镜像库，若本地缓存不可用则会从上游重新下载，是否继续？`
            : ""
        }
        confirmLabel="确认重新下载"
        loading={!!confirmRetryArt && retryingCoord === confirmRetryArt.metadata.name}
        onConfirm={async () => {
          if (!confirmRetryArt) return;
          const target = confirmRetryArt;
          await retryFailedArtifact(target);
          setConfirmRetryArt(null);
        }}
      />
      {isAdmin && (
        <DownloadProxyModal
          open={proxyModalOpen}
          onClose={() => setProxyModalOpen(false)}
          onSaved={() => setCatalogReloadSeq((n) => n + 1)}
        />
      )}
    </>
  );
}
