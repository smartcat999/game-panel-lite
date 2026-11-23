"use client";

import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowUpCircle,
  Layers,
  Loader2,
  Package as PackageIcon,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  Settings,
  Terminal,
  Trash2,
} from "lucide-react";
import { useConsole } from "../../../console/ConsoleProvider";
import { ArtifactLogModal } from "../../../components/console/admin/ArtifactLogModal";
import { DownloadProxyModal } from "../../../components/console/admin/DownloadProxyModal";
import {
  PackageGroup,
  RegistryVersionModal,
  artifactStageShortLabel,
  formatAddonVersion,
  formatByteValue,
  humanizeSyncError,
  isFloatingVersionAlias,
} from "../../../components/console/admin/RegistryVersionModal";
import { gameCover } from "../../../console/lib/presets";
import {
  Badge,
  Button,
  Card,
  ConfirmDialog,
  EmptyState,
  Field,
  IconButton,
  Input,
  type MenuItem,
  Modal,
  Mono,
  MoreMenu,
  PageHeader,
  Segmented,
  Select,
  Skeleton,
} from "../../../components/console/ui";
import { formatBytes } from "../../../console/lib/format";
import { errorMessage } from "../../../console/lib/session";
import type {
  AppItem,
  KubeList,
  RegistryArtifact,
  RegistryPackage,
  RegistryRepository,
  RegistryUpstream,
} from "../../../console/lib/types";
import { subscribeResourceWatch } from "../../../console/lib/ws";

type CategoryFilter = "all" | "game" | "mod";
type StatusFilter = "all" | "syncing" | "failed" | "ready";
type PrePullKind = "game" | "mod";

const PAGE_SIZE = 15;
const AUTO_PREWARM_ANNOTATION = "core.gamepanel.io/auto-prewarm-defaults";
const LEGACY_SERVER_SUFFIX_RE = /\s*(\u670d\u52a1\u7aef|\u521b\u610f\u5de5\u574a|\u63d2\u4ef6\u5e93|\u6a21\u7ec4\u5e93)$/i;

function parseAddonInput(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) return "";
  const idMatch = trimmed.match(/[?&]id=(\d{5,12})/);
  if (idMatch) return idMatch[1];
  const modrinthMatch = trimmed.match(/modrinth\.com\/(?:mod|plugin|datapack)\/([^/?#]+)/i);
  if (modrinthMatch) return modrinthMatch[1];
  const tsMatch = trimmed.match(/thunderstore\.io\/(?:c\/[^/]+\/p|package)\/([^/?#]+)\/([^/?#]+)/i);
  if (tsMatch) return `${tsMatch[1]}-${tsMatch[2]}`;
  const umodPluginMatch = trimmed.match(/umod\.org\/plugins\/([^\s,;/?#"<>]+)/i);
  if (umodPluginMatch) return umodPluginMatch[1].replace(/\.(json|cs)$/i, "");
  const umodGameMatch = trimmed.match(/umod\.org\/games\/([^\s,;/?#"<>]+)/i);
  if (umodGameMatch) return `oxide-${umodGameMatch[1].replace(/\.json$/i, "").toLowerCase()}`;
  const ficsitMatch = trimmed.match(/ficsit\.app\/mod\/([^\s,;/?#"<>]+)/i);
  if (ficsitMatch) return ficsitMatch[1];
  const ghMatch = trimmed.match(/github\.com\/([A-Za-z0-9._-]+)\/([A-Za-z0-9._-]+)/i);
  if (ghMatch) {
    const owner = ghMatch[1];
    const repo = ghMatch[2].replace(/\.git$/i, "");
    if (owner.toLowerCase() === "roflmuffin" && repo.toLowerCase() === "counterstrikesharp") {
      return "counterstrikesharp";
    }
    return `${owner}--${repo}`;
  }
  return trimmed;
}

function appHasAddonRepo(app: AppItem | undefined, repo: string): boolean {
  if (!app?.addonBinding || !repo) return false;
  if (app.addonBinding.repository === repo) return true;
  return (app.addonBinding.additional || []).some((sub) => sub.repository === repo);
}

function cleanRepoDisplayTitle(repo?: RegistryRepository, fallback?: string): string {
  const raw = (repo?.spec.displayName || fallback || "").trim();
  return raw.replace(LEGACY_SERVER_SUFFIX_RE, "").trim();
}

function isStaleArtifactVersion(existing: RegistryArtifact, incoming: RegistryArtifact): boolean {
  if (existing.metadata.uid && incoming.metadata.uid && existing.metadata.uid !== incoming.metadata.uid) {
    return false;
  }
  const curRV = Number(existing.metadata.resourceVersion || 0);
  const nextRV = Number(incoming.metadata.resourceVersion || 0);
  if (curRV > 0 && nextRV > 0 && nextRV !== curRV) {
    return nextRV < curRV;
  }
  const curSV = Number(existing.metadata.statusVersion || 0);
  const nextSV = Number(incoming.metadata.statusVersion || 0);
  return curSV > 0 && nextSV > 0 && nextSV < curSV;
}

export default function AdminRegistryPage() {
  const { apps, tenants, token, apiFetch, toast } = useConsole();
  const [repos, setRepos] = useState<RegistryRepository[]>([]);
  const [artifacts, setArtifacts] = useState<RegistryArtifact[]>([]);
  const [packages, setPackages] = useState<RegistryPackage[]>([]);
  const [upstreams, setUpstreams] = useState<RegistryUpstream[]>([]);
  const [loading, setLoading] = useState(true);

  const [category, setCategory] = useState<CategoryFilter>("all");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [appFilter, setAppFilter] = useState<string>("all");
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [initialRepoFilter, setInitialRepoFilter] = useState("");
  const [selectedGroupKey, setSelectedGroupKey] = useState<string | null>(null);

  const [proxyModalOpen, setProxyModalOpen] = useState(false);
  const [prePullOpen, setPrePullOpen] = useState(false);
  const [prePullAppName, setPrePullAppName] = useState("");
  const [prePullKind, setPrePullKind] = useState<PrePullKind>("game");
  const [prePullVersion, setPrePullVersion] = useState("");
  const [prePullCustomVersion, setPrePullCustomVersion] = useState("");
  const [prePullAvailableVersions, setPrePullAvailableVersions] = useState<string[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [prePullAddonInput, setPrePullAddonInput] = useState("");
  const [prePullBusy, setPrePullBusy] = useState(false);

  const [busyCoord, setBusyCoord] = useState<string | null>(null);
  const [hydratingCoord, setHydratingCoord] = useState<string | null>(null);
  const [cleaningTarget, setCleaningTarget] = useState<{
    artifactNames: string[];
    label: string;
  } | null>(null);
  const [cleanBusy, setCleanBusy] = useState(false);
  const [logArtifactName, setLogArtifactName] = useState<string | null>(null);

  const loadData = useCallback(
    async (silent = false) => {
      if (!silent) setLoading(true);
      try {
        const repoPromise = apiFetch("/apis/registry.gamepanel.io/v1/repositories?limit=200");
        const artPromise = apiFetch("/apis/registry.gamepanel.io/v1/artifacts?limit=500");
        const pkgPromise = apiFetch(
          "/apis/registry.gamepanel.io/v1/packages?fieldSelector=cached=true&limit=500",
        );
        const upPromise = apiFetch("/apis/registry.gamepanel.io/v1/upstreams?limit=200");

        void pkgPromise
          .then(async (res) => {
            if (!res.ok) return;
            const json = (await res.json()) as KubeList<RegistryPackage>;
            setPackages(json.items || []);
          })
          .catch(() => {});

        void upPromise
          .then(async (res) => {
            if (!res.ok) return;
            const json = (await res.json()) as KubeList<RegistryUpstream>;
            setUpstreams(json.items || []);
          })
          .catch(() => {});

        const [repoRes, artRes] = await Promise.all([repoPromise, artPromise]);
        if (!repoRes.ok) throw new Error(await errorMessage(repoRes, "加载镜像仓库失败"));
        if (!artRes.ok) throw new Error(await errorMessage(artRes, "加载镜像缓存文件失败"));

        const [repoJson, artJson] = await Promise.all([
          repoRes.json() as Promise<KubeList<RegistryRepository>>,
          artRes.json() as Promise<KubeList<RegistryArtifact>>,
        ]);

        setRepos(repoJson.items || []);
        setArtifacts(artJson.items || []);
      } catch (e) {
        if (!silent) {
          toast("error", e instanceof Error ? e.message : "加载镜像站数据失败");
        }
      } finally {
        if (!silent) setLoading(false);
      }
    },
    [apiFetch, toast],
  );

  const reloadPackages = useCallback(
    async (refresh = false) => {
      try {
        const sel = refresh ? "cached=true,refresh=true" : "cached=true";
        const res = await apiFetch(
          `/apis/registry.gamepanel.io/v1/packages?fieldSelector=${encodeURIComponent(sel)}&limit=500`,
        );
        if (!res.ok) return;
        const json = (await res.json()) as KubeList<RegistryPackage>;
        setPackages(json.items || []);
      } catch {
        // silent metadata refresh
      }
    },
    [apiFetch],
  );

  useEffect(() => {
    void loadData(false);
    const t = setTimeout(() => {
      void reloadPackages(true);
    }, 300);
    return () => clearTimeout(t);
  }, [loadData, reloadPackages]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const syncFromUrl = () => {
      const sp = new URLSearchParams(window.location.search);
      const itemParam = sp.get("item")?.trim() || "";
      setSelectedGroupKey(itemParam || null);
    };
    const sp = new URLSearchParams(window.location.search);
    const repoParam = sp.get("repo")?.trim() || "";
    const appParam = sp.get("app")?.trim() || "";
    if (appParam) {
      setAppFilter(appParam);
    } else if (repoParam) {
      setInitialRepoFilter(repoParam);
    }
    syncFromUrl();
    window.addEventListener("popstate", syncFromUrl);
    return () => {
      window.removeEventListener("popstate", syncFromUrl);
    };
  }, []);

  const selectGroup = useCallback((key: string | null) => {
    setSelectedGroupKey(key);
    if (typeof window === "undefined") return;
    const url = new URL(window.location.href);
    if (key) {
      url.searchParams.set("item", key);
    } else {
      url.searchParams.delete("item");
    }
    window.history.pushState(null, "", url.pathname + url.search);
  }, []);

  const repoByName = useMemo(() => {
    const m = new Map<string, RegistryRepository>();
    for (const r of repos) m.set(r.metadata.name, r);
    return m;
  }, [repos]);

  const appByRepo = useMemo(() => {
    const m = new Map<string, AppItem>();
    for (const a of apps) {
      if (a.serverBinding?.repository) m.set(a.serverBinding.repository, a);
      if (a.addonBinding?.repository) m.set(a.addonBinding.repository, a);
      for (const sub of a.addonBinding?.additional || []) {
        if (sub.repository) m.set(sub.repository, a);
      }
    }
    return m;
  }, [apps]);

  useEffect(() => {
    if (!initialRepoFilter || appByRepo.size === 0) return;
    const matched = appByRepo.get(initialRepoFilter);
    if (matched) {
      setAppFilter(matched.metadata.name);
      setInitialRepoFilter("");
    }
  }, [initialRepoFilter, appByRepo]);

  const pkgByRepoAndId = useMemo(() => {
    const m = new Map<string, RegistryPackage>();
    for (const p of packages) {
      m.set(`${p.repository}/${p.package}`, p);
    }
    return m;
  }, [packages]);

  const pkgByRepoAndIdRef = useRef(pkgByRepoAndId);
  pkgByRepoAndIdRef.current = pkgByRepoAndId;

  // 100% WebSocket Watch subscription for artifacts (zero HTTP polling)
  useEffect(() => {
    if (!token) return;
    let pkgRefreshTimer: ReturnType<typeof setTimeout> | null = null;

    const schedulePkgRefresh = () => {
      if (pkgRefreshTimer !== null) return;
      pkgRefreshTimer = setTimeout(() => {
        pkgRefreshTimer = null;
        void reloadPackages();
      }, 350);
    };

    const stopWatch = subscribeResourceWatch<RegistryArtifact>({
      path: "/apis/registry.gamepanel.io/v1/artifacts",
      token,
      onReconnect: () => {
        void loadData(true);
      },
      onEvent: (ev) => {
        const obj = ev?.object;
        const name = obj?.metadata?.name;
        if (!name) return;

        if (ev.type === "DELETED") {
          setArtifacts((prev) => prev.filter((item) => item.metadata.name !== name));
          return;
        }

        if (ev.type === "ADDED" || ev.type === "MODIFIED") {
          if (
            obj.status?.phase === "Ready" &&
            !pkgByRepoAndIdRef.current.has(`${obj.spec.repository}/${obj.spec.package}`)
          ) {
            schedulePkgRefresh();
          }
          setArtifacts((prev) => {
            const idx = prev.findIndex((item) => item.metadata.name === name);
            if (idx === -1) return [...prev, obj];
            if (isStaleArtifactVersion(prev[idx], obj)) return prev;
            const next = [...prev];
            next[idx] = obj;
            return next;
          });
        }
      },
    });

    return () => {
      stopWatch();
      if (pkgRefreshTimer !== null) {
        clearTimeout(pkgRefreshTimer);
      }
    };
  }, [token, loadData, reloadPackages]);

  const packageGroups = useMemo<PackageGroup[]>(() => {
    const active = artifacts.filter((a) => !a.metadata.deletionTimestamp);
    const byKey = new Map<string, RegistryArtifact[]>();

    for (const art of active) {
      const key = `${art.spec.repository}.${art.spec.package}`;
      const list = byKey.get(key) || [];
      list.push(art);
      byKey.set(key, list);
    }

    const groups: PackageGroup[] = [];
    for (const [key, rawList] of byKey.entries()) {
      const sorted = [...rawList].sort((a, b) => {
        const aBusy =
          a.status?.phase === "Pending" || a.status?.phase === "Syncing"
            ? 0
            : a.status?.phase === "Failed"
              ? 1
              : 2;
        const bBusy =
          b.status?.phase === "Pending" || b.status?.phase === "Syncing"
            ? 0
            : b.status?.phase === "Failed"
              ? 1
              : 2;
        if (aBusy !== bBusy) return aBusy - bBusy;
        const ta = new Date(a.metadata.creationTimestamp || 0).getTime();
        const tb = new Date(b.metadata.creationTimestamp || 0).getTime();
        return tb - ta;
      });

      const primary = sorted[0];
      const repository = primary.spec.repository;
      const packageId = primary.spec.package;
      const repo = repoByName.get(repository);
      const ownerApp = appByRepo.get(repository);
      const isGamePkg =
        repo?.spec.format === "server_package" ||
        (ownerApp?.serverBinding?.repository === repository &&
          !appHasAddonRepo(ownerApp, repository));
      const isLocalUpload = sorted.every(
        (art) =>
          repo?.spec.mode === "Hosted" ||
          Boolean(art.spec.descriptor && !art.spec.sourceURL),
      );
      const pkgMeta = pkgByRepoAndId.get(`${repository}/${packageId}`);
      const appName = ownerApp?.metadata.name || repository;
      const appLabel =
        ownerApp?.displayName ||
        cleanRepoDisplayTitle(repo, repository) ||
        repository;

      const activeArtifact = sorted.find(
        (x) => x.status?.phase === "Pending" || x.status?.phase === "Syncing",
      );
      const failedArtifact = sorted.find((x) => x.status?.phase === "Failed");
      const readyArtifacts = sorted.filter((x) => x.status?.phase === "Ready");

      let overallPhase: PackageGroup["overallPhase"] = "Ready";
      if (activeArtifact) {
        overallPhase = "Syncing";
      } else if (failedArtifact) {
        overallPhase = "Failed";
      } else if (isLocalUpload) {
        overallPhase = "Local";
      }

      const seenDigests = new Set<string>();
      let groupSize = 0;
      for (const art of sorted) {
        const sz =
          art.status?.descriptor?.sizeBytes ?? art.spec.descriptor?.sizeBytes ?? 0;
        if (sz <= 0) continue;
        const digestKey =
          art.status?.descriptor?.digest ||
          art.spec.descriptor?.digest ||
          art.metadata.name;
        if (seenDigests.has(digestKey)) continue;
        seenDigests.add(digestKey);
        groupSize += sz;
      }
      if (groupSize === 0 && pkgMeta?.sizeBytes && pkgMeta.sizeBytes > 0) {
        groupSize = pkgMeta.sizeBytes;
      }

      const latestVersion =
        readyArtifacts[0]?.spec.version || primary.spec.version || "latest";
      const title = isGamePkg ? appLabel : pkgMeta?.title?.trim() || packageId;

      let subtitle = "";
      if (activeArtifact) {
        subtitle = `正在同步 ${activeArtifact.spec.version} · ${artifactStageShortLabel(activeArtifact)}`;
      } else if (failedArtifact) {
        subtitle = humanizeSyncError(failedArtifact.status?.message);
      } else if (isGamePkg) {
        subtitle = isLocalUpload
          ? "本地上传安装包"
          : ownerApp?.tagline?.trim() || "官方发行版";
      } else {
        subtitle = `编号 ${packageId}`;
      }

      const iconUrl = isGamePkg
        ? gameCover(ownerApp?.metadata.name || appName, ownerApp?.coverImage)
        : pkgMeta?.iconUrl;

      const mergedAvailableVersions = Array.from(
        new Set([...(pkgMeta?.availableVersions || []), ...(ownerApp?.versions || [])]),
      );
      const rawUpstreamLatest = (
        pkgMeta?.latestVersion ||
        pkgMeta?.availableVersions?.[0] ||
        ""
      ).trim();
      const upstreamNewerVersion =
        !isLocalUpload &&
        rawUpstreamLatest &&
        !isFloatingVersionAlias(rawUpstreamLatest) &&
        !sorted.some(
          (a) => a.spec.version === rawUpstreamLatest && a.status?.phase !== "Failed",
        )
          ? rawUpstreamLatest
          : undefined;

      const searchHaystack = [
        title,
        subtitle,
        appLabel,
        packageId,
        ...sorted.map((a) => a.spec.version),
        ...sorted.map((a) => a.metadata.name),
      ]
        .join(" ")
        .toLowerCase();

      groups.push({
        key,
        repository,
        packageId,
        repo,
        isGamePkg,
        isLocalUpload,
        appName,
        ownerAppName: ownerApp?.metadata.name,
        appLabel,
        title,
        subtitle,
        iconUrl,
        availableVersions: mergedAvailableVersions,
        recommendedVersion: ownerApp?.recommendedVersion,
        artifacts: sorted,
        artifactNames: sorted.map((x) => x.metadata.name),
        activeArtifact,
        failedArtifact,
        readyCount: readyArtifacts.length,
        totalSizeBytes: groupSize,
        latestVersion,
        upstreamNewerVersion,
        overallPhase,
        searchHaystack,
      });
    }

    return groups.sort((a, b) => {
      const rank = (g: PackageGroup) =>
        g.overallPhase === "Syncing" ? 0 : g.overallPhase === "Failed" ? 1 : 2;
      const ra = rank(a);
      const rb = rank(b);
      if (ra !== rb) return ra - rb;
      if (a.appLabel !== b.appLabel) return a.appLabel.localeCompare(b.appLabel, "zh-Hans-CN");
      return a.title.localeCompare(b.title, "zh-Hans-CN");
    });
  }, [artifacts, repoByName, appByRepo, pkgByRepoAndId]);

  const selectedGroup = useMemo(
    () =>
      selectedGroupKey
        ? packageGroups.find((g) => g.key === selectedGroupKey) || null
        : null,
    [packageGroups, selectedGroupKey],
  );

  useEffect(() => {
    if (!loading && selectedGroupKey && !selectedGroup) {
      selectGroup(null);
    }
  }, [loading, selectedGroupKey, selectedGroup, selectGroup]);

  const gameFilterOptions = useMemo(() => {
    const map = new Map<string, string>();
    for (const g of packageGroups) {
      map.set(g.appName, g.appLabel);
    }
    return Array.from(map.entries())
      .map(([value, label]) => ({ value, label }))
      .sort((a, b) => a.label.localeCompare(b.label, "zh-Hans-CN"));
  }, [packageGroups]);

  const filteredGroups = useMemo(() => {
    const q = query.trim().toLowerCase();
    return packageGroups.filter((g) => {
      if (category === "game" && !g.isGamePkg) return false;
      if (category === "mod" && g.isGamePkg) return false;
      if (statusFilter === "syncing" && g.overallPhase !== "Syncing") return false;
      if (statusFilter === "failed" && g.overallPhase !== "Failed") return false;
      if (
        statusFilter === "ready" &&
        g.overallPhase !== "Ready" &&
        g.overallPhase !== "Local"
      ) {
        return false;
      }
      if (appFilter !== "all" && g.appName !== appFilter) return false;
      if (q && !g.searchHaystack.includes(q)) return false;
      return true;
    });
  }, [packageGroups, category, statusFilter, appFilter, query]);

  useEffect(() => {
    setPage(1);
  }, [category, statusFilter, appFilter, query]);

  const totalPages = Math.max(1, Math.ceil(filteredGroups.length / PAGE_SIZE));
  useEffect(() => {
    setPage((p) => Math.min(p, totalPages));
  }, [totalPages]);

  const pagedGroups = useMemo(() => {
    const safePage = Math.min(page, totalPages);
    const start = (safePage - 1) * PAGE_SIZE;
    return filteredGroups.slice(start, start + PAGE_SIZE);
  }, [filteredGroups, page, totalPages]);

  const totalSizeBytes = useMemo(() => {
    const seen = new Set<string>();
    let total = 0;
    for (const art of artifacts) {
      if (art.metadata.deletionTimestamp) continue;
      const sz = art.status?.descriptor?.sizeBytes ?? art.spec.descriptor?.sizeBytes ?? 0;
      if (sz <= 0) continue;
      const key =
        art.status?.descriptor?.digest || art.spec.descriptor?.digest || art.metadata.name;
      if (seen.has(key)) continue;
      seen.add(key);
      total += sz;
    }
    return total;
  }, [artifacts]);

  const hasProxy = useMemo(
    () => upstreams.some((u) => Boolean(u.proxyURL?.trim())),
    [upstreams],
  );

  const autoPrewarmEnabled = useMemo(() => {
    const adminTenant = tenants.find((t) => t.metadata.name === "admin");
    return (
      (adminTenant?.metadata.annotations?.[AUTO_PREWARM_ANNOTATION] || "").toLowerCase() ===
      "true"
    );
  }, [tenants]);

  const prePullApps = useMemo(
    () =>
      apps.filter((a) => {
        const sRepo = a.serverBinding?.repository
          ? repoByName.get(a.serverBinding.repository)
          : undefined;
        const canGameProxy = Boolean(
          a.serverBinding?.repository &&
            a.serverBinding?.package &&
            sRepo?.spec.mode !== "Hosted",
        );
        const canModProxy = Boolean(a.addonBinding?.repository);
        return canGameProxy || canModProxy;
      }),
    [apps, repoByName],
  );

  const selectedPrePullApp = useMemo(
    () => prePullApps.find((a) => a.metadata.name === prePullAppName) || prePullApps[0],
    [prePullApps, prePullAppName],
  );

  const selectedAppSupportsGame = useMemo(() => {
    if (!selectedPrePullApp?.serverBinding?.repository || !selectedPrePullApp.serverBinding?.package) {
      return false;
    }
    const r = repoByName.get(selectedPrePullApp.serverBinding.repository);
    return r?.spec.mode !== "Hosted";
  }, [selectedPrePullApp, repoByName]);

  const selectedAppSupportsMod = Boolean(selectedPrePullApp?.addonBinding?.repository);

  const effectivePrePullKind: PrePullKind =
    prePullKind === "game" && !selectedAppSupportsGame && selectedAppSupportsMod
      ? "mod"
      : prePullKind === "mod" && !selectedAppSupportsMod && selectedAppSupportsGame
        ? "game"
        : prePullKind;

  const prePullKindItems = useMemo<Array<{ value: PrePullKind; label: string }>>(
    () => [
      ...(selectedAppSupportsGame ? [{ value: "game" as const, label: "游戏版本" }] : []),
      ...(selectedAppSupportsMod ? [{ value: "mod" as const, label: "模组" }] : []),
    ],
    [selectedAppSupportsGame, selectedAppSupportsMod],
  );

  useEffect(() => {
    if (!prePullOpen || !selectedPrePullApp) return;
    if (prePullKind === "game" && !selectedAppSupportsGame && selectedAppSupportsMod) {
      setPrePullKind("mod");
    } else if (prePullKind === "mod" && !selectedAppSupportsMod && selectedAppSupportsGame) {
      setPrePullKind("game");
    }
  }, [prePullOpen, selectedPrePullApp, prePullKind, selectedAppSupportsGame, selectedAppSupportsMod]);

  useEffect(() => {
    if (!prePullOpen || !selectedAppSupportsGame || !selectedPrePullApp?.serverBinding) {
      setLoadingVersions(false);
      setPrePullAvailableVersions([]);
      return;
    }
    const sRepo = selectedPrePullApp.serverBinding.repository;
    const sPkg = selectedPrePullApp.serverBinding.package;
    if (!sRepo || !sPkg) return;
    let cancelled = false;
    const fallbackVersions = selectedPrePullApp.versions || [];
    setLoadingVersions(true);
    setPrePullAvailableVersions(fallbackVersions);
    setPrePullVersion(selectedPrePullApp.recommendedVersion || fallbackVersions[0] || "");
    const coord = `${sRepo}.${sPkg}`;
    void (async () => {
      try {
        const res = await apiFetch(
          `/apis/registry.gamepanel.io/v1/packages/${encodeURIComponent(coord)}`,
        );
        if (!res.ok) return;
        const pkg = (await res.json()) as RegistryPackage;
        if (cancelled) return;
        const upstream = (pkg.availableVersions || []).filter(Boolean);
        const appVers = selectedPrePullApp.versions || [];
        const combined = Array.from(new Set([...appVers, ...upstream]));
        setPrePullAvailableVersions(combined);
        setPrePullVersion((prev) =>
          prev && (combined.includes(prev) || prev === "__custom__")
            ? prev
            : selectedPrePullApp.recommendedVersion || combined[0] || "",
        );
      } finally {
        if (!cancelled) setLoadingVersions(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [prePullOpen, selectedAppSupportsGame, selectedPrePullApp, apiFetch]);

  function openPrePullModal(preferredAppName?: string) {
    const preferred =
      (preferredAppName
        ? prePullApps.find((a) => a.metadata.name === preferredAppName)
        : undefined) ||
      (appFilter !== "all"
        ? prePullApps.find((a) => a.metadata.name === appFilter)
        : undefined) ||
      prePullApps[0];
    setPrePullAppName(preferred?.metadata.name || "");
    const supportsGame = Boolean(
      preferred?.serverBinding?.repository &&
        repoByName.get(preferred.serverBinding.repository)?.spec.mode !== "Hosted",
    );
    setPrePullKind(supportsGame ? "game" : "mod");
    setPrePullVersion(preferred?.recommendedVersion || preferred?.versions?.[0] || "");
    setPrePullCustomVersion("");
    setPrePullAddonInput("");
    setPrePullOpen(true);
  }

  async function handlePrePull(e: FormEvent) {
    e.preventDefault();
    if (!selectedPrePullApp) return;

    let spec: { repository: string; package: string; version: string };
    if (effectivePrePullKind === "game") {
      const sb = selectedPrePullApp.serverBinding;
      const ver = (prePullVersion === "__custom__" ? prePullCustomVersion : prePullVersion).trim();
      if (!sb?.repository || !sb?.package || !ver) {
        toast("error", "请选择或填写游戏版本号");
        return;
      }
      spec = { repository: sb.repository, package: sb.package, version: ver };
    } else {
      const ab = selectedPrePullApp.addonBinding;
      const pkg = parseAddonInput(prePullAddonInput);
      if (!ab?.repository || !pkg) {
        toast("error", "请输入模组编号 / 链接");
        return;
      }
      spec = { repository: ab.repository, package: pkg, version: "latest" };
    }

    setPrePullBusy(true);
    try {
      const res = await apiFetch(`/apis/registry.gamepanel.io/v1/artifacts`, {
        method: "POST",
        body: JSON.stringify({
          metadata: {
            annotations: {
              "registry.gamepanel.io/retry-requested-at": new Date().toISOString(),
            },
          },
          spec,
        }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "添加资源失败"));
      toast("success", "已加入镜像站拉取队列，正在后台同步");
      setPrePullOpen(false);
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "添加资源失败");
    } finally {
      setPrePullBusy(false);
    }
  }

  async function handlePullGameVersion(group: PackageGroup, version: string) {
    const cleanVer = version.trim();
    if (!cleanVer) return;
    const coord = `${group.repository}.${group.packageId}.${cleanVer}`;
    setBusyCoord(coord);
    try {
      const res = await apiFetch(`/apis/registry.gamepanel.io/v1/artifacts`, {
        method: "POST",
        body: JSON.stringify({
          metadata: {
            annotations: {
              "registry.gamepanel.io/retry-requested-at": new Date().toISOString(),
            },
          },
          spec: {
            repository: group.repository,
            package: group.packageId,
            version: cleanVer,
          },
        }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "发起拉取失败"));
      toast("success", `已将 ${group.title} ${cleanVer} 加入拉取队列`);
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "拉取失败");
    } finally {
      setBusyCoord(null);
    }
  }

  async function handleRefreshArtifact(art: RegistryArtifact) {
    const coord = art.metadata.name;
    const repo = repoByName.get(art.spec.repository);
    const ownerApp = appByRepo.get(art.spec.repository);
    const isGamePkg =
      repo?.spec.format === "server_package" ||
      (ownerApp?.serverBinding?.repository === art.spec.repository &&
        !appHasAddonRepo(ownerApp, art.spec.repository));
    const isFailed = art.status?.phase === "Failed";

    setBusyCoord(coord);
    try {
      const retryAt = new Date().toISOString();
      const res = isFailed
        ? await apiFetch(`/apis/registry.gamepanel.io/v1/artifacts/${encodeURIComponent(coord)}`, {
            method: "PATCH",
            body: JSON.stringify({
              metadata: {
                annotations: {
                  "registry.gamepanel.io/retry-requested-at": retryAt,
                },
              },
            }),
          })
        : await apiFetch(`/apis/registry.gamepanel.io/v1/artifacts`, {
            method: "POST",
            body: JSON.stringify({
              metadata: {
                annotations: {
                  "registry.gamepanel.io/retry-requested-at": retryAt,
                },
              },
              spec: {
                repository: art.spec.repository,
                package: art.spec.package,
                version: isGamePkg ? art.spec.version : "latest",
              },
            }),
          });
      if (!res.ok) throw new Error(await errorMessage(res, "同步上游失败"));
      toast("success", isFailed ? "已重新加入拉取队列" : "已触发上游版本校验与更新");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "操作失败");
    } finally {
      setBusyCoord(null);
    }
  }

  async function handleHydrateArtifact(coord: string) {
    setHydratingCoord(coord);
    try {
      const res = await apiFetch(
        `/apis/registry.gamepanel.io/v1/artifacts/${encodeURIComponent(coord)}`,
        {
          method: "PATCH",
          body: JSON.stringify({
            metadata: {
              annotations: {
                "registry.gamepanel.io/hydrate-requested-at": new Date().toISOString(),
              },
            },
          }),
        },
      );
      if (!res.ok) return;
      const hydrated = (await res.json()) as RegistryArtifact;
      setArtifacts((prev) =>
        prev.map((item) => (item.metadata.name === hydrated.metadata.name ? hydrated : item)),
      );
    } catch {
      // keep existing metadata on silent hydration error
    } finally {
      setHydratingCoord((cur) => (cur === coord ? null : cur));
    }
  }

  async function handleUpdateToLatest(group: PackageGroup) {
    if (group.isGamePkg && group.upstreamNewerVersion) {
      await handlePullGameVersion(group, group.upstreamNewerVersion);
      return;
    }
    const coord = `${group.repository}.${group.packageId}.latest`;
    const primaryCoord = group.artifacts[0]?.metadata.name || coord;
    setBusyCoord(primaryCoord);
    try {
      const res = await apiFetch(`/apis/registry.gamepanel.io/v1/artifacts`, {
        method: "POST",
        body: JSON.stringify({
          metadata: {
            annotations: {
              "registry.gamepanel.io/retry-requested-at": new Date().toISOString(),
            },
          },
          spec: {
            repository: group.repository,
            package: group.packageId,
            version: "latest",
          },
        }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "检查更新失败"));
      const returned = (await res.json()) as RegistryArtifact;
      setArtifacts((prev) => {
        const idx = prev.findIndex((a) => a.metadata.name === returned.metadata.name);
        if (idx >= 0) {
          const next = [...prev];
          next[idx] = returned;
          return next;
        }
        return [returned, ...prev];
      });
      const verLabel = formatAddonVersion(
        returned.spec.version || group.upstreamNewerVersion || "latest",
      );
      if (returned.status?.phase === "Ready") {
        toast("info", `「${group.title}」当前已是最新版本（${verLabel}）`);
      } else {
        toast("success", `发现新版本 ${verLabel}，已加入同步队列`);
      }
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "操作失败");
    } finally {
      setBusyCoord(null);
    }
  }

  async function confirmCleanTarget() {
    if (!cleaningTarget) return;
    setCleanBusy(true);
    try {
      await Promise.all(
        cleaningTarget.artifactNames.map(async (coord) => {
          const res = await apiFetch(
            `/apis/registry.gamepanel.io/v1/artifacts/${encodeURIComponent(coord)}`,
            { method: "DELETE" },
          );
          if (!res.ok && res.status !== 404) {
            throw new Error(await errorMessage(res, "清理缓存文件失败"));
          }
        }),
      );
      toast("success", `已清理「${cleaningTarget.label}」的镜像缓存`);
      setCleaningTarget(null);
      await loadData(true);
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "清理失败");
    } finally {
      setCleanBusy(false);
    }
  }

  const headerSubtitle = useMemo(() => {
    if (loading && artifacts.length === 0) {
      return "正在加载镜像资源...";
    }
    const sizeText = totalSizeBytes > 0 ? formatBytes(totalSizeBytes) : "0 B";
    const parts = [
      `共 ${packageGroups.length} 项（${sizeText}）`,
      hasProxy ? "代理已开启" : "直连上游",
      autoPrewarmEnabled ? "自动预热已开启" : "自动预热已关闭",
    ];
    return parts.join(" · ");
  }, [loading, artifacts.length, packageGroups.length, totalSizeBytes, hasProxy, autoPrewarmEnabled]);

  const activeLogArtifact = useMemo(
    () =>
      logArtifactName
        ? artifacts.find((a) => a.metadata.name === logArtifactName) || null
        : null,
    [artifacts, logArtifactName],
  );

  const activeLogTitle = useMemo(() => {
    if (!activeLogArtifact) return undefined;
    const key = `${activeLogArtifact.spec.repository}.${activeLogArtifact.spec.package}`;
    const grp = packageGroups.find((g) => g.key === key);
    return grp?.title || activeLogArtifact.spec.package;
  }, [activeLogArtifact, packageGroups]);

  return (
    <div>
      <PageHeader
        title="镜像站"
        description={headerSubtitle}
        actions={
          <div className="flex flex-wrap items-center gap-2.5">
            <Button
              type="button"
              variant="secondary"
              icon={<Settings className="h-4 w-4" />}
              onClick={() => setProxyModalOpen(true)}
            >
              镜像设置
            </Button>
            <Button
              type="button"
              variant="primary"
              icon={<Plus className="h-4 w-4" />}
              onClick={() => openPrePullModal()}
              disabled={prePullApps.length === 0}
            >
              添加资源
            </Button>
          </div>
        }
      />

      <search aria-label="镜像站缓存筛选" className="mb-5 flex flex-wrap items-center justify-between gap-3">
          <Segmented
            label="缓存资源类型"
            value={category}
            onChange={(v) => setCategory(v as CategoryFilter)}
            items={[
              { value: "all", label: "全部" },
              { value: "game", label: "游戏" },
              { value: "mod", label: "模组" },
            ]}
          />
          <div className="flex flex-wrap items-center gap-2.5">
            <div className="w-36 shrink-0">
              <Select
                aria-label="状态筛选"
                value={statusFilter}
                onChange={(e) => setStatusFilter(e.target.value as StatusFilter)}
              >
                <option value="all">全部状态</option>
                <option value="syncing">同步中</option>
                <option value="failed">异常</option>
                <option value="ready">已就绪</option>
              </Select>
            </div>
            <div className="w-44 shrink-0">
              <Select
                aria-label="所属游戏筛选"
                value={appFilter}
                onChange={(e) => setAppFilter(e.target.value)}
              >
                <option value="all">全部游戏</option>
                {gameFilterOptions.map((g) => (
                  <option key={g.value} value={g.value}>
                    {g.label}
                  </option>
                ))}
              </Select>
            </div>
            <div className="relative w-72">
              <Search className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-[#8A90A6]" />
              <Input
                type="search"
                name="q"
                aria-label="搜索缓存文件"
                placeholder="搜索名称、版本号或模组编号..."
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                autoCorrect="off"
                autoCapitalize="none"
                spellCheck={false}
                className="pl-10"
              />
            </div>
          </div>
        </search>

      {loading ? (
        <Skeleton className="h-80 rounded-[20px]" />
      ) : (
        <Card className="overflow-hidden">
          {filteredGroups.length === 0 ? (
            <EmptyState
              title="暂无镜像资源"
              body={
                packageGroups.length === 0
                  ? "创建服务器时将自动同步所需文件，也可点击右上角「添加资源」提前同步"
                  : "当前筛选条件下没有匹配的缓存记录"
              }
              action={
                packageGroups.length === 0 ? (
                  <Button
                    type="button"
                    variant="primary"
                    icon={<Plus className="h-4 w-4" />}
                    onClick={() => openPrePullModal()}
                    disabled={prePullApps.length === 0}
                  >
                    添加资源
                  </Button>
                ) : (
                  <Button
                    type="button"
                    variant="secondary"
                    onClick={() => {
                      setCategory("all");
                      setStatusFilter("all");
                      setAppFilter("all");
                      setQuery("");
                    }}
                  >
                    重置筛选条件
                  </Button>
                )
              }
            />
          ) : (
            <>
              <div
                tabIndex={0}
                role="region"
                aria-label="镜像缓存列表"
                className="overflow-x-auto [scrollbar-gutter:stable] focus:outline-none"
              >
                <table className="w-full min-w-[840px] table-fixed text-left text-sm">
                  <colgroup>
                    <col className="w-[34%]" />
                    <col className="w-[18%]" />
                    <col className="w-[23%]" />
                    <col className="w-[11%]" />
                    <col className="w-[14%]" />
                  </colgroup>
                  <thead>
                    <tr className="border-b border-[#EEF0F7] text-[12px] font-semibold text-[#5B6075]">
                      <th className="px-6 py-3.5">名称</th>
                      <th className="px-4 py-3.5">所属游戏</th>
                      <th className="px-4 py-3.5">版本与大小</th>
                      <th className="px-4 py-3.5">状态</th>
                      <th className="px-6 py-3.5 text-right">操作</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-[#EEF0F7]">
                    {pagedGroups.map((group) => {
                      const activeArt = group.activeArtifact;
                      const failedArt = group.failedArtifact;
                      const logTarget = activeArt || failedArt || group.artifacts[0];
                      const singleArt = group.artifacts.length === 1 ? group.artifacts[0] : null;
                      const activeProgress = activeArt?.status?.progress ?? 0;
                      const activeProcessed = activeArt?.status?.processedBytes ?? 0;
                      const activeTotal = activeArt?.status?.totalBytes ?? 0;
                      const activeSpeed = activeArt?.status?.speedBytesPerSec ?? 0;
                      const isIndeterminate =
                        Boolean(activeArt) &&
                        (activeArt?.status?.stage === "Queued" ||
                          activeArt?.status?.stage === "Resolving" ||
                          activeArt?.status?.stage === "Archiving" ||
                          (activeProgress <= 0 && activeTotal <= 0));
                      const activeTelemetry =
                        activeSpeed > 0
                          ? `${formatByteValue(activeSpeed)}/s`
                          : activeTotal > 0
                            ? `${formatByteValue(activeProcessed)} / ${formatByteValue(activeTotal)}`
                            : `${activeProgress}%`;
                      const versionSummaryText = group.isGamePkg
                        ? singleArt &&
                          isFloatingVersionAlias(singleArt.spec.version) &&
                          singleArt.status?.build
                          ? `${singleArt.spec.version} (Build ${singleArt.status.build.slice(0, 12)})`
                          : group.latestVersion
                        : formatAddonVersion(group.latestVersion);
                      const sizeSummaryText = formatByteValue(group.totalSizeBytes);
                      const idleVersionAndSize =
                        group.artifacts.length > 1
                          ? `${versionSummaryText} 等 ${group.artifacts.length} 个版本 · ${sizeSummaryText}`
                          : `${versionSummaryText} · ${sizeSummaryText}`;
                      const newerLabel = group.upstreamNewerVersion
                        ? group.isGamePkg
                          ? group.upstreamNewerVersion
                          : formatAddonVersion(group.upstreamNewerVersion)
                        : "";
                      const fullVersionTitle = newerLabel
                        ? `${idleVersionAndSize} · 可更新至 ${newerLabel}`
                        : idleVersionAndSize;
                      const isRowBusy = Boolean(
                        (failedArt && busyCoord === failedArt.metadata.name) ||
                          (singleArt && busyCoord === singleArt.metadata.name) ||
                          busyCoord === group.artifacts[0]?.metadata.name,
                      );
                      const floatingArt =
                        !group.isLocalUpload &&
                        !activeArt &&
                        !failedArt &&
                        !newerLabel &&
                        group.isGamePkg
                          ? group.artifacts.find(
                              (a) =>
                                isFloatingVersionAlias(a.spec.version) &&
                                a.status?.phase === "Ready",
                            )
                          : undefined;
                      const overflowItems: MenuItem[] = [];
                      if (floatingArt) {
                        overflowItems.push({
                          label: "检查更新",
                          icon: <RefreshCw className="h-4 w-4" />,
                          disabled: busyCoord === floatingArt.metadata.name,
                          onSelect: () => void handleRefreshArtifact(floatingArt),
                        });
                      } else if (
                        !group.isGamePkg &&
                        !group.isLocalUpload &&
                        !activeArt &&
                        !failedArt
                      ) {
                        overflowItems.push({
                          label: "检查更新",
                          icon: <RefreshCw className="h-4 w-4" />,
                          disabled: isRowBusy,
                          onSelect: () => void handleUpdateToLatest(group),
                        });
                      }
                      if (logTarget && !group.isLocalUpload) {
                        overflowItems.push({
                          label: "查看日志",
                          icon: <Terminal className="h-4 w-4" />,
                          onSelect: () => setLogArtifactName(logTarget.metadata.name),
                        });
                      }
                      if (!group.isLocalUpload) {
                        overflowItems.push({
                          label: "清理缓存",
                          icon: <Trash2 className="h-4 w-4" />,
                          danger: true,
                          onSelect: () =>
                            setCleaningTarget({
                              artifactNames: group.artifactNames,
                              label: group.title,
                            }),
                        });
                      }

                      return (
                        <tr key={group.key} className="transition-colors hover:bg-[#F7F8FC]/60">
                          <td className="px-6 py-4">
                            <div className="flex items-center gap-3">
                              {group.iconUrl ? (
                                // eslint-disable-next-line @next/next/no-img-element
                                <img
                                  src={group.iconUrl}
                                  alt=""
                                  className="h-9 w-9 shrink-0 rounded-lg object-cover ring-1 ring-[#E3E6F2]"
                                />
                              ) : (
                                <span className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[#F4F6FC] text-[#5B6075] ring-1 ring-[#E3E6F2]">
                                  <PackageIcon className="h-4 w-4" />
                                </span>
                              )}
                              <div className="min-w-0">
                                <button
                                  type="button"
                                  onClick={() => selectGroup(group.key)}
                                  className="block max-w-full truncate text-left font-semibold text-[#14161F] hover:text-[#2B3AE8]"
                                  title={group.title}
                                >
                                  {group.title}
                                </button>
                                <div
                                  className={`mt-0.5 truncate text-xs ${group.overallPhase === "Failed" ? "text-[#B42318]" : "text-[#8A90A6]"}`}
                                  title={group.subtitle}
                                >
                                  {group.subtitle}
                                </div>
                              </div>
                            </div>
                          </td>
                          <td className="px-4 py-4">
                            <span className="block truncate font-medium text-[#14161F]" title={group.appLabel}>
                              {group.appLabel}
                            </span>
                          </td>
                          <td className="px-4 py-4 tabular-nums text-[#5B6075]">
                            {activeArt ? (
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
                                    value={Math.min(100, Math.max(0, activeProgress))}
                                    className="gp-progress w-full"
                                    aria-label="同步进度"
                                  />
                                )}
                                <span className="truncate text-[11px] text-[#8A90A6]">
                                  {activeArt.spec.version} · {activeTelemetry}
                                </span>
                              </div>
                            ) : (
                              <span className="block truncate" title={fullVersionTitle}>
                                {idleVersionAndSize}
                                {newerLabel && (
                                  <span className="font-medium text-[#2B3AE8]">
                                    {" "}
                                    · 可更新至 {newerLabel}
                                  </span>
                                )}
                              </span>
                            )}
                          </td>
                          <td className="px-4 py-4">
                            {group.overallPhase === "Syncing" && activeArt ? (
                              <Badge tone="accent">
                                <span className="inline-flex items-center gap-1">
                                  <Loader2 className="h-3 w-3 animate-spin" />
                                  {artifactStageShortLabel(activeArt)}
                                </span>
                              </Badge>
                            ) : group.overallPhase === "Failed" ? (
                              <Badge tone="danger">异常</Badge>
                            ) : group.overallPhase === "Local" ? (
                              <Badge tone="neutral">本地上传</Badge>
                            ) : (
                              <Badge tone="success">已就绪</Badge>
                            )}
                          </td>
                          <td className="px-6 py-4 text-right">
                            <div className="inline-flex items-center justify-end gap-1 whitespace-nowrap">
                              {!group.isLocalUpload && !activeArt && failedArt && (
                                <IconButton
                                  label={`重试拉取 ${failedArt.spec.version}`}
                                  disabled={isRowBusy}
                                  className="h-8 w-8 bg-transparent"
                                  onClick={() => void handleRefreshArtifact(failedArt)}
                                >
                                  <RotateCcw className="h-4 w-4" aria-hidden="true" />
                                </IconButton>
                              )}
                              {!group.isLocalUpload && !activeArt && !failedArt && newerLabel && (
                                <IconButton
                                  label={`更新至新版本 ${newerLabel}`}
                                  disabled={isRowBusy}
                                  className="h-8 w-8 bg-transparent text-[#2B3AE8] hover:bg-[#EFF1FF] hover:text-[#2B3AE8]"
                                  onClick={() => void handleUpdateToLatest(group)}
                                >
                                  <ArrowUpCircle className="h-4 w-4" aria-hidden="true" />
                                </IconButton>
                              )}
                              <IconButton
                                label="版本管理"
                                className="h-8 w-8 bg-transparent"
                                onClick={() => selectGroup(group.key)}
                              >
                                <Layers className="h-4 w-4" aria-hidden="true" />
                              </IconButton>
                              <MoreMenu
                                label={`更多操作 (${group.title})`}
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

              {filteredGroups.length > PAGE_SIZE && (
                <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[#EEF0F7] px-6 py-3.5 text-xs text-[#5B6075]">
                  <span>
                    第 {Math.min(page, totalPages)} / {totalPages} 页 · 共 {filteredGroups.length} 项
                  </span>
                  <div className="flex items-center gap-2">
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
                </div>
              )}
            </>
          )}
        </Card>
      )}

      <RegistryVersionModal
        open={Boolean(selectedGroup) && !logArtifactName && !cleaningTarget}
        group={selectedGroup}
        busyCoord={busyCoord}
        hydratingCoord={hydratingCoord}
        onClose={() => selectGroup(null)}
        onPullGameVersion={handlePullGameVersion}
        onPullLatestAddon={handleUpdateToLatest}
        onRefreshArtifact={(art) => void handleRefreshArtifact(art)}
        onHydrateArtifact={handleHydrateArtifact}
        onOpenLogs={(coord) => setLogArtifactName(coord)}
        onRequestClean={(artifactNames, label) =>
          setCleaningTarget({ artifactNames, label })
        }
      />

      <DownloadProxyModal
        open={proxyModalOpen}
        onClose={() => setProxyModalOpen(false)}
        onSaved={() => void loadData(true)}
      />

      <ArtifactLogModal
        open={Boolean(logArtifactName)}
        onClose={() => setLogArtifactName(null)}
        artifact={activeLogArtifact}
        title={activeLogTitle}
        token={token}
        backLabel={selectedGroup ? "返回版本列表" : "关闭"}
      />

      <Modal
        open={prePullOpen}
        onClose={() => setPrePullOpen(false)}
        title="添加资源"
        description="从上游同步指定游戏版本或创意工坊模组至镜像站"
        footer={
          <>
            <Button type="button" variant="ghost" onClick={() => setPrePullOpen(false)}>
              取消
            </Button>
            <Button type="submit" form="mirror-prepull-form" loading={prePullBusy}>
              开始同步
            </Button>
          </>
        }
      >
        <form id="mirror-prepull-form" onSubmit={(e) => void handlePrePull(e)} className="space-y-4">
          <Field label="所属游戏" htmlFor="prepull-app">
            <Select
              id="prepull-app"
              name="appName"
              value={selectedPrePullApp?.metadata.name || ""}
              onChange={(e) => setPrePullAppName(e.target.value)}
            >
              {prePullApps.map((a) => (
                <option key={a.metadata.name} value={a.metadata.name}>
                  {a.displayName}
                </option>
              ))}
            </Select>
          </Field>

          {prePullKindItems.length > 0 && (
            <div className="space-y-1.5">
              <div className="text-[13px] font-medium text-[#3D4258]">资源类型</div>
              <Segmented
                label="资源类型"
                value={effectivePrePullKind}
                onChange={(v) => setPrePullKind(v as PrePullKind)}
                items={prePullKindItems}
              />
            </div>
          )}

          {(() => {
            const isGamePrePull = effectivePrePullKind === "game" && selectedAppSupportsGame;
            const useCustomGameVer = prePullAvailableVersions.length > 0 && prePullVersion === "__custom__";
            return (
              <div className="grid grid-cols-1">
                <div
                  role="tabpanel"
                  aria-label="游戏版本"
                  inert={!isGamePrePull}
                  aria-hidden={!isGamePrePull}
                  className={
                    isGamePrePull
                      ? "col-start-1 row-start-1"
                      : "invisible pointer-events-none col-start-1 row-start-1 select-none"
                  }
                >
                  <Field
                    label="游戏版本号"
                    htmlFor={useCustomGameVer ? "prepull-custom-ver" : "prepull-game-ver"}
                    hint={loadingVersions ? "正在从上游获取可用版本..." : undefined}
                  >
                    {prePullAvailableVersions.length > 0 ? (
                      useCustomGameVer ? (
                        <div className="flex items-center gap-2">
                          <Input
                            id="prepull-custom-ver"
                            name="customVersion"
                            value={prePullCustomVersion}
                            onChange={(e) => setPrePullCustomVersion(e.target.value)}
                            disabled={!isGamePrePull}
                            placeholder="输入指定版本号，如 1.21.4"
                            autoCorrect="off"
                            autoCapitalize="none"
                            spellCheck={false}
                            className="flex-1"
                          />
                          <Button
                            type="button"
                            variant="secondary"
                            disabled={!isGamePrePull}
                            onClick={() => setPrePullVersion(prePullAvailableVersions[0] || "")}
                          >
                            返回列表
                          </Button>
                        </div>
                      ) : (
                        <Select
                          id="prepull-game-ver"
                          name="version"
                          value={prePullVersion}
                          onChange={(e) => setPrePullVersion(e.target.value)}
                          disabled={!isGamePrePull}
                        >
                          {prePullAvailableVersions.map((v) => (
                            <option key={v} value={v}>
                              {v}
                            </option>
                          ))}
                          <option value="__custom__">手动输入其他版本号...</option>
                        </Select>
                      )
                    ) : (
                      <Input
                        id="prepull-game-ver"
                        name="version"
                        value={prePullVersion}
                        onChange={(e) => setPrePullVersion(e.target.value)}
                        disabled={!isGamePrePull}
                        placeholder="例如 latest 或 1.21.4"
                        autoCorrect="off"
                        autoCapitalize="none"
                        spellCheck={false}
                      />
                    )}
                  </Field>
                </div>
                <div
                  role="tabpanel"
                  aria-label="模组"
                  inert={isGamePrePull}
                  aria-hidden={isGamePrePull}
                  className={
                    !isGamePrePull
                      ? "col-start-1 row-start-1"
                      : "invisible pointer-events-none col-start-1 row-start-1 select-none"
                  }
                >
                  <Field
                    label="模组 / 插件编号或链接"
                    htmlFor="prepull-addon-id"
                    hint="支持 Steam 创意工坊、Modrinth、ficsit.app 或 GitHub 插件仓库链接"
                  >
                    <Input
                      id="prepull-addon-id"
                      name="addonId"
                      value={prePullAddonInput}
                      onChange={(e) => setPrePullAddonInput(e.target.value)}
                      disabled={isGamePrePull}
                      placeholder="例如 1207269058 或 https://steamcommunity.com/sharedfiles/filedetails/?id=..."
                      autoCorrect="off"
                      autoCapitalize="none"
                      spellCheck={false}
                    />
                  </Field>
                </div>
              </div>
            );
          })()}
        </form>
      </Modal>

      <ConfirmDialog
        open={Boolean(cleaningTarget)}
        onClose={() => setCleaningTarget(null)}
        title="确认清理镜像缓存"
        body={
          cleaningTarget ? (
            <>
              确定从镜像站清理 <Mono>{cleaningTarget.label}</Mono> 吗？清理后若有新服务器需要该文件，平台将重新按需从上游拉取。
            </>
          ) : undefined
        }
        confirmLabel="确认清理"
        danger
        loading={cleanBusy}
        onConfirm={() => void confirmCleanTarget()}
      />
    </div>
  );
}
