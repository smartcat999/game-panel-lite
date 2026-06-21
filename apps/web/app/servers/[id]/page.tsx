"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, ArrowRight, Ban, Check, CheckCircle2, Clock, Copy, Cpu, Download, ExternalLink, FileArchive, FileText, KeyRound, Megaphone, MemoryStick, Moon, Package, Plug, Power, RotateCcw, Save, Send, Share2, Sun, Sunrise, Terminal, Trash2, UserX, Users, Waves, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from "react";
import type { TerrariaConfig } from "@gamepanel-lite/shared";
import { secretSeedKeyFor, terrariaInternalPort, terrariaSecretSeeds, terrariaSeedModeCodes } from "@gamepanel-lite/shared";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PlayersPanel } from "@/components/players-panel";
import { ServerActions } from "@/components/server-actions";
import { ServerModeBadge, ServerStatusBadge } from "@/components/server-badges";
import { Button, Card, Input } from "@/components/ui";
import { ActivityTimeline, MonitoringChartCard } from "@/features/monitoring/components";
import { getServerMonitoringEvents, getServerMonitoringMetrics } from "@/features/monitoring/api";
import type { MetricSeries, MonitoringEvent, MonitoringRange } from "@/features/monitoring/types";
import {
  assignMod,
  createBackup,
  createWorldSnapshot,
  deleteBackup,
  disableServerShare,
  deleteMod,
  deleteWorld,
  downloadBackupFile,
  downloadWorldFile,
  enableServerShare,
  getDockerStatus,
  getGameServer,
  listGames,
  getServerLogSnapshot,
  getServerShare,
  getServerStats,
  listBackups,
  listGlobalMods,
  listModPacks,
  listMods,
  listWorlds,
  previewTerrariaConfig,
  restoreBackup,
  sendServerCommand,
  gameServerAction,
  setModEnabled,
  serverLogsUrl,
  serverWatchUrl,
  updateGameServerConfig,
  type ServerConfigUpdatePayload,
  type ServerWatchSnapshot,
} from "@/lib/api";
import { copyText } from "@/lib/clipboard";
import { consoleReadyMessageKey, supportsTerrariaConsoleShortcuts } from "@/lib/console-commands";
import { saveBlob } from "@/lib/download";
import { isWorldOrBackupEventType, showWorldAndBackupFeatures } from "@/lib/feature-flags";
import { gameServerConfigPendingRestart, gameServerJoinPort, gameServerMaxPlayers, gameServerMode, gameServerStatus, gameServerVersion, terrariaConfigFromGameServer } from "@/lib/game-server-resource";
import { localizeRelativeTime, useI18n, type MessageKey } from "@/lib/i18n";
import { modDisplayName } from "@/lib/mod-display";
import { updateProviderConfigPayload, type ProviderConfigPayload } from "@/lib/provider-config";
import { describeResourceAction, formatServerDetailError, isServerLifecyclePending } from "@/lib/server-detail-actions";
import { isWorldActiveOnServer } from "@/lib/server-detail-resources";
import { serverInviteText, serverJoinAddress, serverJoinPassword } from "@/lib/server-join";
import { cn } from "@/lib/utils";
import type { Backup, GameServerResource, ModFile, ModPack, ProviderCapabilities, ProviderCatalog, ProviderConfigField, ResourceLimits, ServerStatus, World } from "@/lib/types";

type TabId = "overview" | "monitoring" | "activity" | "console" | "logs" | "config" | "worlds" | "backups" | "mods";
type MonitoringRangeValue = "15m" | "1h" | "6h" | "24h";
type ModInstallSource = "library" | "packs";

const cpuLimitOptions = [0, 0.5, 1, 2, 4] as const;
const memoryLimitOptions = [0, 1024, 2048, 4096, 8192] as const;
const terrariaProviderKeys = new Set(["terraria-vanilla", "terraria-tmodloader"]);
const providerFieldLabelKeys: Record<string, MessageKey> = {
  cavesEnabled: "cavesEnabled",
  clusterDescription: "clusterDescription",
  serverName: "serverName",
  saveName: "saveName",
  clusterName: "clusterName",
  worldName: "worldName",
  maxPlayers: "maxPlayersInput",
  serverPassword: "serverPassword",
  adminPassword: "adminPassword",
  clusterToken: "clusterToken",
  consoleEnabled: "consoleEnabled",
  gameMode: "gameMode",
  offlineServer: "offlineServer",
  onlineMode: "onlineMode",
  pauseWhenEmpty: "pauseWhenEmpty",
  pvp: "pvp",
  worldPreset: "worldPreset",
  eulaAccepted: "minecraftEulaAccepted"
};

const defaultCapabilities: ProviderCapabilities = {
  consoleCommands: true,
  playerList: true,
  kickPlayer: true,
  banPlayer: true,
  whitelist: false,
  saveSnapshots: true,
  backups: true,
  mods: false,
  versions: true
};

function formatCpuLimitLabel(value: number, t: (key: "unlimited" | "cpuCoresValue", values?: Record<string, string | number>) => string) {
  return value > 0 ? t("cpuCoresValue", { cores: value }) : t("unlimited");
}

function formatMemoryLimitLabel(value: number, t: (key: "unlimited" | "memoryGbValue", values?: Record<string, string | number>) => string) {
  return value > 0 ? t("memoryGbValue", { gb: value / 1024 }) : t("unlimited");
}

export default function ServerDetailPage() {
  const { locale, t } = useI18n();
  const params = useParams<{ id: string }>();
  const id = params.id;
  const client = useQueryClient();
  const logViewportRef = useRef<HTMLDivElement>(null);
  const logServerIdRef = useRef("");
  const logReplayIndexRef = useRef(0);
  const [activeTab, setActiveTab] = useState<TabId>("overview");

  const query = useQuery({ queryKey: ["game-server", id], queryFn: () => getGameServer(id), retry: false });
  const serverResource = query.data;
  const resourceStatus = serverResource ? gameServerStatus(serverResource) : undefined;
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, enabled: Boolean(serverResource), staleTime: 5 * 60 * 1000, retry: false });
  const providerCatalog = useMemo(
    () => gamesQuery.data?.flatMap((game) => game.providers).find((provider) => provider.key === serverResource?.providerKey),
    [gamesQuery.data, serverResource?.providerKey]
  );
  const capabilities = providerCatalog?.capabilities ?? {
    ...defaultCapabilities,
    mods: serverResource ? gameServerMode(serverResource) === "tmodloader" : false
  };
  const visibleCapabilities = useMemo(
    () => ({
      ...capabilities,
      saveSnapshots: showWorldAndBackupFeatures && capabilities.saveSnapshots,
      backups: showWorldAndBackupFeatures && capabilities.backups
    }),
    [capabilities]
  );
  const statsQuery = useQuery({
    queryKey: ["server-stats", id],
    queryFn: () => getServerStats(id),
    enabled: false,
    retry: false
  });
  const worldsQuery = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, enabled: Boolean(serverResource && visibleCapabilities.saveSnapshots), retry: false });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, enabled: Boolean(serverResource && visibleCapabilities.backups), retry: false });
  const modsQuery = useQuery({
    queryKey: ["mods", id],
    queryFn: () => listMods(id),
    enabled: Boolean(serverResource && capabilities.mods),
    retry: false
  });
  const globalModsQuery = useQuery({
    queryKey: ["global-mods"],
    queryFn: listGlobalMods,
    enabled: Boolean(serverResource && capabilities.mods),
    retry: false
  });
  const modPacksQuery = useQuery({
    queryKey: ["mod-packs"],
    queryFn: listModPacks,
    enabled: Boolean(serverResource && capabilities.mods),
    retry: false
  });
  const dockerStatusQuery = useQuery({ queryKey: ["docker-status"], queryFn: getDockerStatus, enabled: Boolean(serverResource && capabilities.mods), retry: false, staleTime: 5 * 60 * 1000 });
  const shareQuery = useQuery({ queryKey: ["server-share", id], queryFn: () => getServerShare(id), enabled: Boolean(serverResource), retry: false });
  const [activitySeenAt, setActivitySeenAt] = useState(() => Date.now());
  const [monitoringRange, setMonitoringRange] = useState<MonitoringRangeValue>("1h");
  const [copied, setCopied] = useState("");
  const [logs, setLogs] = useState<string[]>([]);
  const [command, setCommand] = useState("");
  const [shareIncludePassword, setShareIncludePassword] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [consoleError, setConsoleError] = useState("");
  const [pendingWorldDelete, setPendingWorldDelete] = useState<World | null>(null);
  const [pendingWorldSnapshot, setPendingWorldSnapshot] = useState(false);
  const [pendingBackupCreate, setPendingBackupCreate] = useState(false);
  const [pendingRestore, setPendingRestore] = useState<Backup | null>(null);
  const [pendingBackupDelete, setPendingBackupDelete] = useState<Backup | null>(null);
  const [pendingModDelete, setPendingModDelete] = useState<ModFile | null>(null);
  const [pendingModToggle, setPendingModToggle] = useState<{ mod: ModFile; enabled: boolean } | null>(null);
  const [pendingModPackInstall, setPendingModPackInstall] = useState<ModPack | null>(null);
  const [pendingConfigRestart, setPendingConfigRestart] = useState(false);
  const [resourceDialogOpen, setResourceDialogOpen] = useState(false);
  const [downloadingResourceId, setDownloadingResourceId] = useState("");
  const monitoringStep = monitoringRange === "15m" ? "30s" : monitoringRange === "1h" ? "1m" : monitoringRange === "6h" ? "5m" : "15m";
  const serverMetricsQuery = useQuery({
    queryKey: ["server-monitoring-metrics", id, monitoringRange],
    queryFn: () => getServerMonitoringMetrics(id, monitoringRange, monitoringStep),
    enabled: Boolean(serverResource && activeTab === "monitoring"),
    retry: false,
    refetchInterval: 5000
  });
  const serverEventsQuery = useQuery({
    queryKey: ["server-monitoring-events", id],
    queryFn: () => getServerMonitoringEvents(id, 50),
    enabled: false,
    retry: false,
    staleTime: 5000
  });
  const [logStatus, setLogStatus] = useState<"idle" | "connecting" | "connected" | "error" | "paused">("idle");
  const [logStreamPaused, setLogStreamPaused] = useState(false);
  const [configSaved, setConfigSaved] = useState(false);
  const [modsPendingRestart, setModsPendingRestart] = useState(false);
  const successTimerRef = useRef<number | null>(null);
  const formatActionError = (error: unknown, fallback: string) => formatServerDetailError(error, {
    dockerUnavailable: t("detailDockerUnavailable"),
    containerUnavailable: t("detailContainerUnavailable"),
    portAlreadyAllocated: (port) => t("detailPortAlreadyAllocated", { port })
  }) || fallback;
  const formatSnapshotError = (error: unknown) => {
    const message = error instanceof Error ? error.message : String(error || "");
    if (message.toLowerCase().includes("current world file has not been created yet")) {
      return t("worldSnapshotRequiresGeneratedWorld");
    }
    return formatActionError(error, t("unableSaveWorldSnapshot"));
  };

  useEffect(() => {
    if (!serverResource?.id) return;
    const source = new EventSource(serverWatchUrl(id), { withCredentials: true });
    source.addEventListener("snapshot", (event) => {
      try {
        const snapshot = JSON.parse((event as MessageEvent).data) as ServerWatchSnapshot;
        client.setQueryData(["game-server", id], snapshot.server);
        client.setQueryData(["server-stats", id], snapshot.stats);
        client.setQueryData(["server-monitoring-events", id], {
          collectedAt: snapshot.collectedAt,
          events: snapshot.events
        });
      } catch (error) {
        console.warn("failed to parse server watch snapshot", error);
      }
    });
    return () => source.close();
  }, [client, id, serverResource?.id]);

  const showSuccess = (message: string) => {
    setErrorMessage("");
    setSuccessMessage(message);
    if (successTimerRef.current) window.clearTimeout(successTimerRef.current);
    successTimerRef.current = window.setTimeout(() => setSuccessMessage(""), 2200);
  };

  const showError = (message: string) => {
    setSuccessMessage("");
    setErrorMessage(message);
  };

  const setServerResourceCache = (updatedServer: GameServerResource | null | undefined) => {
    if (!updatedServer) {
      return;
    }
    client.setQueryData(["game-server", id], updatedServer);
  };

  const markModsChanged = () => {
    if (resourceStatus === "running") {
      setModsPendingRestart(true);
    }
  };

  useEffect(() => {
    return () => {
      if (successTimerRef.current) window.clearTimeout(successTimerRef.current);
    };
  }, []);

  const commandMutation = useMutation({
    mutationFn: (value: string) => sendServerCommand(id, value),
    onSuccess: (_, value) => {
      setLogs((current) => [...current, `> ${value}`].slice(-300));
      setCommand("");
      setConsoleError("");
    },
    onError: (error) => {
      setSuccessMessage("");
      setConsoleError(formatActionError(error, t("commandSendFailed")));
    }
  });
  const runCommand = (value: string) => {
    const next = value.trim();
    if (!next || commandMutation.isPending) return;
    commandMutation.mutate(next);
  };
  const configSave = useMutation({
    mutationFn: ({ config, hostPort }: { config: ServerConfigUpdatePayload; hostPort: number }) => updateGameServerConfig(id, config, hostPort),
    onSuccess: async (updatedServer) => {
      showSuccess(t("configSaved"));
      setConfigSaved(true);
      setServerResourceCache(updatedServer);
      await client.invalidateQueries({ queryKey: ["game-server", id] });
      await client.invalidateQueries({ queryKey: ["game-servers"] });
      window.setTimeout(() => setConfigSaved(false), 1800);
    },
    onError: (error) => {
      setConfigSaved(false);
      showError(formatActionError(error, t("unableUpdateConfig")));
    }
  });
  const resourceSave = useMutation({
    mutationFn: ({ resources }: { resources: ResourceLimits }) => {
      if (!serverResource) throw new Error("server not loaded");
      return updateGameServerConfig(id, terrariaConfigFromGameServer(serverResource), gameServerJoinPort(serverResource), resources);
    },
    onSuccess: async (updatedServer) => {
      showSuccess(t("resourceLimitsSaved"));
      setResourceDialogOpen(false);
      setServerResourceCache(updatedServer);
      await client.invalidateQueries({ queryKey: ["game-server", id] });
      await client.invalidateQueries({ queryKey: ["game-servers"] });
    },
    onError: (error) => showError(formatActionError(error, t("unableUpdateConfig")))
  });
  const configRestart = useMutation({
    mutationFn: () => gameServerAction(id, "restart"),
    onSuccess: async (updatedServer) => {
      showSuccess(t("serverRestartQueued"));
      setPendingConfigRestart(false);
      setServerResourceCache(updatedServer);
      await client.invalidateQueries({ queryKey: ["game-server", id] });
      await client.invalidateQueries({ queryKey: ["game-servers"] });
    },
    onError: (error) => showError(formatActionError(error, t("unableAction", { action: t("actionRestart") })))
  });
  const shareEnable = useMutation({
    mutationFn: () => enableServerShare(id, shareIncludePassword),
    onSuccess: async () => {
      showSuccess(t("sharePageEnabled"));
      await client.invalidateQueries({ queryKey: ["server-share", id] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("sharePageUnavailable"))
  });
  const shareDisable = useMutation({
    mutationFn: () => disableServerShare(id),
    onSuccess: async () => {
      showSuccess(t("sharePageDisabled"));
      await client.invalidateQueries({ queryKey: ["server-share", id] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("sharePageUnavailable"))
  });
  const worldDelete = useMutation({
    mutationFn: deleteWorld,
    onSuccess: async () => {
      showSuccess(t("worldDeleted"));
      setPendingWorldDelete(null);
      await client.invalidateQueries({ queryKey: ["worlds"] });
    },
    onError: (error) => {
      const message = error instanceof Error ? error.message : "";
      showError(message.includes("active world") ? t("unableDeleteActiveWorld") : formatActionError(error, t("unableDeleteWorld")));
    }
  });
  const backupCreate = useMutation({
    mutationFn: () => createBackup(id),
    onSuccess: async () => {
      showSuccess(t("backupCreated"));
      setPendingBackupCreate(false);
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (error) => showError(formatActionError(error, t("unableCreateBackup")))
  });
  const backupRestore = useMutation({
    mutationFn: restoreBackup,
    onSuccess: async () => {
      showSuccess(t("backupRestored"));
      setPendingRestore(null);
      await client.invalidateQueries({ queryKey: ["backups"] });
      await client.invalidateQueries({ queryKey: ["game-server", id] });
      await client.invalidateQueries({ queryKey: ["game-servers"] });
    },
    onError: (error) => showError(formatActionError(error, t("unableRestoreBackup")))
  });
  const worldSnapshotCreate = useMutation({
    mutationFn: () => createWorldSnapshot(id),
    onSuccess: async () => {
      showSuccess(t("worldSnapshotSaved"));
      setPendingWorldSnapshot(false);
      await client.invalidateQueries({ queryKey: ["worlds"] });
    },
    onError: (error) => showError(formatSnapshotError(error))
  });
  const backupDelete = useMutation({
    mutationFn: deleteBackup,
    onSuccess: async () => {
      showSuccess(t("backupDeleted"));
      setPendingBackupDelete(null);
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (error) => showError(formatActionError(error, t("unableDeleteBackup")))
  });
  const modEnabled = useMutation({
    mutationFn: ({ modId, enabled }: { modId: string; enabled: boolean }) => setModEnabled(id, modId, enabled),
    onSuccess: async (updatedMod) => {
      showSuccess(updatedMod.enabled ? t("modEnabled") : t("modDisabled"));
      markModsChanged();
      setPendingModToggle(null);
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => showError(formatActionError(error, t("unableUpdateMod")))
  });
  const modDelete = useMutation({
    mutationFn: (modId: string) => deleteMod(id, modId),
    onSuccess: async () => {
      showSuccess(t("modDeleted"));
      markModsChanged();
      setPendingModDelete(null);
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => showError(formatActionError(error, t("unableDeleteMod")))
  });
  const modAssign = useMutation({
    mutationFn: async (modIds: string[]) => {
      for (const modId of modIds) {
        await assignMod(modId, id);
      }
    },
    onSuccess: async () => {
      showSuccess(t("modAssigned"));
      markModsChanged();
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => showError(formatActionError(error, t("unableAssignMod")))
  });
  const modPackAssign = useMutation({
    mutationFn: async (pack: ModPack) => {
      for (const modId of pack.modIds) {
        await assignMod(modId, id);
      }
    },
    onSuccess: async () => {
      showSuccess(t("modPackInstalled"));
      markModsChanged();
      setPendingModPackInstall(null);
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => showError(formatActionError(error, t("unableAssignMod")))
  });
  useEffect(() => {
    if (!id || (activeTab !== "console" && activeTab !== "logs")) return;
    if (logStreamPaused) {
      setLogStatus("paused");
      return;
    }
    if (logServerIdRef.current !== id) {
      logServerIdRef.current = id;
      setLogs([]);
      logReplayIndexRef.current = 0;
    }
    let alive = true;
    let source: EventSource | null = null;
    setLogStatus("connecting");

    getServerLogSnapshot(id)
      .then((lines) => {
        if (!alive) return;
        const snapshotLines = lines.slice(-300);
        setLogs(snapshotLines);
        logReplayIndexRef.current = 0;
        setConsoleError("");
        if (resourceStatus !== "running") {
          setLogStatus("idle");
          return;
        }
        source = new EventSource(serverLogsUrl(id), { withCredentials: true });
        source.onopen = () => {
          setConsoleError("");
          setLogStatus("connected");
        };
        source.addEventListener("log", (event) => {
          setLogs((current) => {
            const replayIndex = logReplayIndexRef.current;
            if (replayIndex < snapshotLines.length && event.data === snapshotLines[replayIndex]) {
              logReplayIndexRef.current = replayIndex + 1;
              return current;
            }
            logReplayIndexRef.current = snapshotLines.length;
            return [...current, event.data].slice(-300);
          });
        });
        source.addEventListener("error", (event) => {
          setLogStatus("error");
          const data = "data" in event && typeof event.data === "string" ? event.data : "";
          if (data) {
            setLogs((current) => [...current, data].slice(-300));
            setConsoleError(data);
          }
        });
        source.onerror = () => setLogStatus("error");
      })
      .catch((error) => {
        if (!alive) return;
        setLogStatus("error");
        setConsoleError(formatActionError(error, t("logsUnavailable")));
      });

    return () => {
      alive = false;
      source?.close();
    };
  }, [activeTab, id, resourceStatus, logStreamPaused, t]);

  useEffect(() => {
    const viewport = logViewportRef.current;
    if (viewport) viewport.scrollTop = viewport.scrollHeight;
  }, [logs, activeTab]);

  useEffect(() => {
    if (resourceStatus !== "running") {
      setModsPendingRestart(false);
    }
  }, [resourceStatus]);

  const serverWorlds = useMemo(
    () => (
      serverResource && visibleCapabilities.saveSnapshots
        ? (worldsQuery.data ?? []).filter((world) => world.instanceId === serverResource.id)
        : []
    ),
    [serverResource, visibleCapabilities.saveSnapshots, worldsQuery.data]
  );
  const serverBackups = useMemo(
    () => (serverResource && visibleCapabilities.backups ? (backupsQuery.data ?? []).filter((backup) => backup.instanceId === serverResource.id).sort(sortBackupsNewestFirst) : []),
    [backupsQuery.data, serverResource, visibleCapabilities.backups]
  );
  const serverMods = useMemo(() => modsQuery.data ?? [], [modsQuery.data]);
  const globalMods = useMemo(() => globalModsQuery.data ?? [], [globalModsQuery.data]);
  const modPacks = useMemo(() => modPacksQuery.data ?? [], [modPacksQuery.data]);
  const workshopUnsupported = isArmArchitecture(dockerStatusQuery.data?.architecture);
  const tabs: { id: TabId; label: string }[] = useMemo(() => [
    { id: "overview", label: t("tabOverview") },
    { id: "monitoring", label: t("monitoringTabTitle") },
    { id: "activity", label: t("activityTitle") },
    ...(capabilities.consoleCommands ? [{ id: "console" as const, label: t("tabConsole") }] : []),
    { id: "logs", label: t("tabLogs") },
    { id: "config", label: t("tabConfig") },
    ...(visibleCapabilities.saveSnapshots ? [{ id: "worlds" as const, label: t("tabWorlds") }] : []),
    ...(visibleCapabilities.backups ? [{ id: "backups" as const, label: t("tabBackups") }] : []),
    ...(capabilities.mods ? [{ id: "mods" as const, label: t("tabMods") }] : [])
  ], [capabilities.consoleCommands, capabilities.mods, visibleCapabilities.backups, visibleCapabilities.saveSnapshots, t]);
  useEffect(() => {
    if (serverResource && !tabs.some((tab) => tab.id === activeTab)) {
      setActiveTab("overview");
    }
  }, [activeTab, serverResource, tabs]);
  useEffect(() => {
    if (shareQuery.data) {
      setShareIncludePassword(shareQuery.data.includePassword);
    }
  }, [shareQuery.data]);
  const visibleServerEvents = useMemo(
    () => (serverEventsQuery.data?.events ?? []).filter((event) => showWorldAndBackupFeatures || !isWorldOrBackupEventType(event.type)),
    [serverEventsQuery.data?.events]
  );
  const latestActivityAt = latestMonitoringEventTime(visibleServerEvents);
  useEffect(() => {
    if (activeTab === "activity") {
      setActivitySeenAt((current) => Math.max(current, latestActivityAt, Date.now()));
    }
  }, [activeTab, latestActivityAt]);
  if (!serverResource) {
    return (
      <>
        <Link href="/servers" className="text-sm text-slate-400 hover:text-panel-green">{t("backToServers")}</Link>
        <Card className="mt-4 p-6 text-sm text-slate-400">{query.isLoading ? t("loading") : t("serverNotFound")}</Card>
      </>
    );
  }

  const mode = gameServerMode(serverResource);
  const status = gameServerStatus(serverResource);
  const playersOnline = serverResource.status.playersOnline ?? 0;
  const maxPlayers = gameServerMaxPlayers(serverResource);
  const joinPort = gameServerJoinPort(serverResource);
  const invite = serverInviteText(serverResource);
  const joinAddress = serverJoinAddress(serverResource);
  const joinPassword = serverJoinPassword(serverResource);
  const share = shareQuery.data;
  const sharePath = share?.sharePath ?? "";
  const shareUrl = sharePath ? `${typeof window === "undefined" ? "" : window.location.origin}${sharePath}` : "";
  const logStatusLabel = logStatus === "connected" ? t("logsConnected") : logStatus === "error" ? t("logsDisconnected") : logStatus === "paused" ? t("logsPaused") : logStatus === "idle" ? t("logsIdle") : t("logsConnecting");
  const runtimeErrorMessage = status === "errored" && serverResource.status.lastError ? formatActionError(new Error(serverResource.status.lastError), serverResource.status.lastError) : "";
  const hasUnreadActivity = latestActivityAt > activitySeenAt;
  const showActivityIndicator = activeTab !== "activity" && (Boolean(runtimeErrorMessage) || hasUnreadActivity);
  const copy = async (label: string, value: string) => {
    try {
      await copyText(value);
      setCopied(label);
      setErrorMessage("");
      window.setTimeout(() => setCopied(""), 1500);
    } catch (error) {
      setCopied("");
      showError(error instanceof Error ? error.message : t("copyInviteFailed"));
    }
  };
  const submitCommand = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    runCommand(command);
  };
  const downloadWorld = async (world: World) => {
    setDownloadingResourceId(world.id);
    try {
      const blob = await downloadWorldFile(world.id);
      saveBlob(blob, `${world.name}.wld`);
      showSuccess(t("downloadStarted"));
    } catch (error) {
      showError(formatActionError(error, t("unableDownloadWorld")));
    } finally {
      setDownloadingResourceId("");
    }
  };
  const downloadBackup = async (backup: Backup) => {
    setDownloadingResourceId(backup.id);
    try {
      const blob = await downloadBackupFile(backup.id);
      saveBlob(blob, backup.name);
      showSuccess(t("downloadStarted"));
    } catch (error) {
      showError(formatActionError(error, t("unableDownloadBackup")));
    } finally {
      setDownloadingResourceId("");
    }
  };

  return (
    <>
      <Link href="/servers" className="text-sm text-slate-400 hover:text-panel-green">{t("backToServers")}</Link>
      {query.isError && <p className="mt-3 text-sm text-panel-gold">{t("apiDetailUnavailable")}</p>}
      {errorMessage && <p className="mt-3 rounded-md border border-panel-gold/30 bg-panel-gold/10 px-3 py-2 text-sm text-panel-gold">{errorMessage}</p>}
      {successMessage && <p className="mt-3 rounded-md border border-panel-green/30 bg-panel-green/10 px-3 py-2 text-sm text-panel-green">{successMessage}</p>}
      <div className="mt-3 flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold">{serverResource.name}</h1>
            <ServerModeBadge mode={mode} />
            <ServerStatusBadge status={status} />
            <PlayerCountBadge
              label={t("players")}
              value={`${playersOnline} / ${maxPlayers}`}
            />
          </div>
        </div>
        <div className="hidden md:block">
          <ServerActions server={serverResource} showInvite={false} />
        </div>
      </div>
      <MobileServerControls
        copied={copied}
        invite={invite}
        joinAddress={joinAddress}
        joinPort={joinPort}
        server={serverResource}
        onCopy={copy}
      />

      <div className="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
        <Card className="min-w-0 p-4">
          <div className="mb-4 flex gap-2 overflow-x-auto border-b border-panel-line px-1 pb-4 pt-1" role="tablist" aria-label={serverResource.name}>
            {tabs.map((tab) => (
              <button
                key={tab.id}
                type="button"
                role="tab"
                aria-selected={activeTab === tab.id}
                className={cn(
                  "relative shrink-0 rounded-md border border-transparent px-3 py-2 text-sm font-medium text-slate-400 transition hover:bg-slate-800/80 hover:text-white focus:outline-none focus:ring-2 focus:ring-inset focus:ring-panel-green/50",
                  activeTab === tab.id && "border-panel-green/40 bg-panel-green/15 text-white shadow-[inset_0_0_0_1px_rgba(123,217,120,0.18)]"
                )}
                onClick={() => setActiveTab(tab.id)}
              >
                <span>{tab.label}</span>
                {tab.id === "activity" ? (
                  <span
                    className={cn(
                      "pointer-events-none absolute right-1.5 top-1.5 size-1.5 rounded-full transition-opacity",
                      showActivityIndicator ? runtimeErrorMessage ? "bg-red-400 opacity-100" : "bg-panel-green opacity-100" : "opacity-0"
                    )}
                    aria-label={showActivityIndicator ? t("serverActivityUnread") : undefined}
                  />
                ) : null}
              </button>
            ))}
          </div>

          {activeTab === "overview" && (
            <OverviewTab
              capabilities={visibleCapabilities}
              resource={serverResource}
              worldCount={serverWorlds.length}
              backupCount={serverBackups.length}
              modCount={serverMods.length}
              onSelectTab={setActiveTab}
            />
          )}
          {activeTab === "monitoring" && (
            <MonitoringTab
              metrics={serverMetricsQuery.data?.series}
              metricsRange={serverMetricsQuery.data?.range}
              range={monitoringRange}
              server={serverResource}
              onRangeChange={setMonitoringRange}
            />
          )}
          {activeTab === "activity" && (
            <ServerActivityTab
              events={visibleServerEvents}
              loading={serverEventsQuery.isLoading}
              runtimeError={runtimeErrorMessage}
            />
          )}
          {activeTab === "console" && (
            <ConsoleTab
              command={command}
              commandPending={commandMutation.isPending}
              consoleError={consoleError}
              logs={logs}
              logStatus={logStatus}
              logStatusLabel={logStatusLabel}
              logStreamPaused={logStreamPaused}
              capabilities={capabilities}
              server={serverResource}
              serverStatus={status}
              viewportRef={logViewportRef}
              onChangeCommand={(value) => {
                setCommand(value);
                setConsoleError("");
              }}
              onClear={() => setLogs([])}
              onQuickCommand={runCommand}
              onSubmit={submitCommand}
              onTogglePause={() => setLogStreamPaused((current) => !current)}
            />
          )}
          {activeTab === "logs" && (
            <LogsTab
              logStatus={logStatus}
              logStatusLabel={logStatusLabel}
              logStreamPaused={logStreamPaused}
              logs={logs}
              viewportRef={logViewportRef}
              onClear={() => setLogs([])}
              onTogglePause={() => setLogStreamPaused((current) => !current)}
            />
          )}
          {activeTab === "config" && (
            <ConfigTab
              provider={providerCatalog}
              resource={serverResource}
              saveError={configSave.error instanceof Error ? configSave.error.message : ""}
              savePending={configSave.isPending}
              saveSuccess={configSaved}
              restartPending={configRestart.isPending}
              onRestart={() => setPendingConfigRestart(true)}
              onSave={(nextConfig, hostPort) => configSave.mutate({ config: nextConfig, hostPort })}
            />
          )}
          {activeTab === "worlds" && visibleCapabilities.saveSnapshots && (
            <WorldsTab
              isError={worldsQuery.isError}
              isLoading={worldsQuery.isLoading}
              items={serverWorlds}
              deleting={worldDelete.isPending}
              currentServerId={serverResource.id}
              downloadingId={downloadingResourceId}
              snapshotting={worldSnapshotCreate.isPending}
              onDelete={setPendingWorldDelete}
              onDownload={(world) => void downloadWorld(world)}
              onCreateSnapshot={() => setPendingWorldSnapshot(true)}
            />
          )}
          {activeTab === "backups" && visibleCapabilities.backups && (
            <BackupsTab
              creating={backupCreate.isPending}
              isError={backupsQuery.isError}
              isLoading={backupsQuery.isLoading}
              items={serverBackups}
              deleting={backupDelete.isPending}
              downloadingId={downloadingResourceId}
              restoring={backupRestore.isPending}
              serverStatus={status}
              onDelete={setPendingBackupDelete}
              onDownload={(backup) => void downloadBackup(backup)}
              onCreate={() => setPendingBackupCreate(true)}
              onRestore={setPendingRestore}
            />
          )}
          {activeTab === "mods" && capabilities.mods && (
            <ModsTab
              availableMods={globalMods}
              assigning={modAssign.isPending}
              deleting={modDelete.isPending}
              isError={modsQuery.isError}
              isLoading={modsQuery.isLoading}
              items={serverMods}
              libraryError={globalModsQuery.isError || modPacksQuery.isError}
              modPacks={modPacks}
              pendingRestart={modsPendingRestart}
              packInstalling={modPackAssign.isPending}
              serverStatus={status}
              toggling={modEnabled.isPending}
              workshopUnsupported={workshopUnsupported}
              onAssignMods={(mods) => modAssign.mutate(mods.map((mod) => mod.id))}
              onDelete={setPendingModDelete}
              onInstallPack={setPendingModPackInstall}
              onToggle={(mod) => setPendingModToggle({ mod, enabled: !mod.enabled })}
            />
          )}
        </Card>

        <div className="flex flex-col gap-4">
          <Card className="p-4">
            <h2 className="font-semibold">{t("joinServer")}</h2>
            <CopyRow label={t("ipAddress")} value={joinAddress} copied={copied} copiedLabel={t("copied")} copyLabel={t("copy")} onCopy={copy} />
            <CopyRow label={t("port")} value={String(joinPort)} copied={copied} copiedLabel={t("copied")} copyLabel={t("copy")} onCopy={copy} />
            <CopyRow label={t("password")} value={joinPassword || t("none")} copied={copied} copiedLabel={t("copied")} copyLabel={t("copy")} onCopy={copy} />
            <Button className="mt-4 w-full" variant="secondary" onClick={() => void copy("Invite", invite)}>
              <Copy aria-hidden="true" />
              {copied === "Invite" ? t("copied") : t("copyInviteText")}
            </Button>
          </Card>
          <Card className="p-4">
            <div className="flex items-start gap-3">
              <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/45 text-panel-green">
                <Share2 aria-hidden="true" className="size-4" />
              </span>
              <div className="min-w-0">
                <h2 className="font-semibold">{t("shareServer")}</h2>
                <p className="mt-1 text-xs leading-5 text-slate-500">{t("shareServerDescription")}</p>
              </div>
            </div>
            <label className="mt-4 flex items-center gap-2 rounded-md border border-panel-line bg-slate-950/35 px-3 py-2 text-sm text-slate-300">
              <input
                className="size-4 accent-panel-green"
                type="checkbox"
                checked={shareIncludePassword}
                onChange={(event) => setShareIncludePassword(event.target.checked)}
              />
              {t("includePasswordInShare")}
            </label>
            <div className="mt-3 grid gap-2">
              <Button className="w-full" variant={share?.enabled ? "secondary" : "primary"} onClick={() => shareEnable.mutate()} disabled={shareEnable.isPending || shareQuery.isLoading}>
                {shareEnable.isPending ? t("saving") : share?.enabled ? t("saveButton") : t("enableSharePage")}
              </Button>
              {share?.enabled && shareUrl && (
                <>
                  <Button className="w-full" variant="secondary" onClick={() => void copy("ShareLink", shareUrl)}>
                    <Copy aria-hidden="true" />
                    {copied === "ShareLink" ? t("shareLinkCopied") : t("copyShareLink")}
                  </Button>
                  <Link className="inline-flex h-10 items-center justify-center gap-2 rounded-md border border-panel-line bg-slate-950/45 px-3 text-sm font-medium text-slate-200 transition hover:border-panel-green/40 hover:text-panel-green" href={sharePath} target="_blank">
                    <ExternalLink aria-hidden="true" className="size-4" />
                    {t("openSharePage")}
                  </Link>
                  <Button className="w-full" variant="danger" onClick={() => shareDisable.mutate()} disabled={shareDisable.isPending}>
                    {shareDisable.isPending ? t("saving") : t("disableSharePage")}
                  </Button>
                </>
              )}
            </div>
          </Card>
          {capabilities.playerList && <PlayersPanel serverId={serverResource.id} />}
          <ResourceLimitsCard
            cpuPercent={statsQuery.data?.cpuPercent}
            memoryMb={statsQuery.data?.memoryMb}
            resource={serverResource}
            restartPending={configRestart.isPending}
            onEdit={() => setResourceDialogOpen(true)}
            onRestart={() => setPendingConfigRestart(true)}
          />
          {visibleCapabilities.saveSnapshots && (
            <Card className="p-4">
              <h2 className="font-semibold">{t("worldTemplate")}</h2>
              {serverResource.spec.sourceWorldId ? (
                <Link
                  href={`/worlds/${serverResource.spec.sourceWorldId}`}
                  className="mt-4 flex items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/35 px-3 py-3 transition hover:border-panel-green/50 hover:bg-slate-900/60 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
                >
                  <p className="truncate text-sm font-medium text-slate-100">{serverResource.spec.sourceWorldName || t("worldTemplate")}</p>
                  <ArrowRight aria-hidden="true" className="size-4 shrink-0 text-slate-500" />
                </Link>
              ) : (
                <div className="mt-4 rounded-md border border-panel-line bg-slate-950/35 px-3 py-3">
                  <p className="truncate text-sm font-medium text-slate-500">{t("noWorldTemplate")}</p>
                </div>
              )}
            </Card>
          )}
        </div>
      </div>


      <ConfirmDialog
        open={pendingConfigRestart}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmServerActionTitle", { action: t("actionRestart") })}
        description={t("confirmRestartForConfigDescription", { name: serverResource.name })}
        detail={<DetailLine label={t("server")} value={serverResource.name} />}
        cancelLabel={t("cancel")}
        confirmLabel={configRestart.isPending ? t("actionWorking") : t("confirmServerActionButton", { action: t("actionRestart") })}
        confirmVariant="gold"
        busy={configRestart.isPending}
        onCancel={() => setPendingConfigRestart(false)}
        onConfirm={() => configRestart.mutate()}
      />
      <ResourceLimitsDialog
        open={resourceDialogOpen}
        resource={serverResource}
        savePending={resourceSave.isPending}
        onCancel={() => setResourceDialogOpen(false)}
        onSave={(resources) => resourceSave.mutate({ resources })}
      />
      <ConfirmDialog
        open={showWorldAndBackupFeatures && Boolean(pendingWorldDelete)}
        eyebrow={t("destructiveAction")}
        title={t("deleteWorldConfirm", { name: pendingWorldDelete?.name ?? "" })}
        description={t("confirmDeleteWorldDescription", { name: pendingWorldDelete?.name ?? "" })}
        detail={pendingWorldDelete ? <DetailLine label={t("world")} value={pendingWorldDelete.name} /> : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={worldDelete.isPending ? t("actionWorking") : t("delete")}
        busy={worldDelete.isPending}
        onCancel={() => setPendingWorldDelete(null)}
        onConfirm={() => pendingWorldDelete && worldDelete.mutate(pendingWorldDelete.id)}
      />
      <ConfirmDialog
        open={showWorldAndBackupFeatures && pendingWorldSnapshot}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmWorldSnapshotTitle", { name: serverResource.name })}
        description={t("confirmWorldSnapshotDescription", { name: serverResource.name })}
        detail={<DetailLine label={t("server")} value={serverResource.name} />}
        cancelLabel={t("cancel")}
        confirmLabel={worldSnapshotCreate.isPending ? t("actionWorking") : t("saveWorldSnapshot")}
        confirmVariant="gold"
        busy={worldSnapshotCreate.isPending}
        onCancel={() => setPendingWorldSnapshot(false)}
        onConfirm={() => worldSnapshotCreate.mutate()}
      />
      <ConfirmDialog
        open={showWorldAndBackupFeatures && pendingBackupCreate}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmBackupCreateTitle", { name: serverResource.name })}
        description={t("confirmBackupCreateDescription", { name: serverResource.name })}
        detail={<DetailLine label={t("server")} value={serverResource.name} />}
        cancelLabel={t("cancel")}
        confirmLabel={backupCreate.isPending ? t("actionWorking") : t("createBackupNow")}
        confirmVariant="gold"
        busy={backupCreate.isPending}
        onCancel={() => setPendingBackupCreate(false)}
        onConfirm={() => backupCreate.mutate()}
      />
      <ConfirmDialog
        open={showWorldAndBackupFeatures && Boolean(pendingRestore)}
        eyebrow={t("destructiveAction")}
        title={t("restoreBackupConfirm", { name: pendingRestore?.name ?? "" })}
        description={t("confirmRestoreBackupDescription", { name: pendingRestore?.name ?? "" })}
        detail={pendingRestore ? <DetailLine label={t("backupName")} value={pendingRestore.name} /> : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={backupRestore.isPending ? t("actionWorking") : t("restore")}
        confirmVariant="gold"
        busy={backupRestore.isPending}
        onCancel={() => setPendingRestore(null)}
        onConfirm={() => pendingRestore && backupRestore.mutate(pendingRestore.id)}
      />
      <ConfirmDialog
        open={showWorldAndBackupFeatures && Boolean(pendingBackupDelete)}
        eyebrow={t("destructiveAction")}
        title={t("deleteBackupConfirm", { name: pendingBackupDelete?.name ?? "" })}
        description={t("confirmDeleteBackupDescription", { name: pendingBackupDelete?.name ?? "" })}
        detail={pendingBackupDelete ? <DetailLine label={t("backupName")} value={pendingBackupDelete.name} /> : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={backupDelete.isPending ? t("actionWorking") : t("delete")}
        busy={backupDelete.isPending}
        onCancel={() => setPendingBackupDelete(null)}
        onConfirm={() => pendingBackupDelete && backupDelete.mutate(pendingBackupDelete.id)}
      />
      <ConfirmDialog
        open={Boolean(pendingModToggle)}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmModToggleTitle", { action: pendingModToggle?.enabled ? t("enable") : t("disable"), name: pendingModToggle ? modDisplayName(pendingModToggle.mod, locale) : "" })}
        description={t("confirmModToggleDescription", { action: pendingModToggle?.enabled ? t("enable") : t("disable"), name: pendingModToggle ? modDisplayName(pendingModToggle.mod, locale) : "" })}
        detail={pendingModToggle ? <DetailLine label={t("modsTitle")} value={modDisplayName(pendingModToggle.mod, locale)} /> : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={modEnabled.isPending ? t("actionWorking") : pendingModToggle?.enabled ? t("enable") : t("disable")}
        confirmVariant="gold"
        busy={modEnabled.isPending}
        onCancel={() => setPendingModToggle(null)}
        onConfirm={() => pendingModToggle && modEnabled.mutate({ modId: pendingModToggle.mod.id, enabled: pendingModToggle.enabled })}
      />
      <ConfirmDialog
        open={Boolean(pendingModPackInstall)}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmModPackInstallTitle", { name: pendingModPackInstall?.name ?? "" })}
        description={t("confirmModPackInstallDescription", { name: pendingModPackInstall?.name ?? "", server: serverResource.name })}
        detail={pendingModPackInstall ? (
          <InstallDependencyDetail
            dependencies={dependencyNamesForMods(pendingModPackInstall.mods)}
            label={t("modPacks")}
            name={pendingModPackInstall.name}
            summary={t("modPackIncludes", { count: pendingModPackInstall.mods.length })}
          />
        ) : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={modPackAssign.isPending ? t("actionWorking") : t("installModPack")}
        confirmVariant="gold"
        busy={modPackAssign.isPending}
        onCancel={() => setPendingModPackInstall(null)}
        onConfirm={() => pendingModPackInstall && modPackAssign.mutate(pendingModPackInstall)}
      />
      <ConfirmDialog
        open={Boolean(pendingModDelete)}
        eyebrow={t("destructiveAction")}
        title={t("deleteModConfirm", { name: pendingModDelete ? modDisplayName(pendingModDelete, locale) : "" })}
        description={t("confirmDeleteModDescription", { name: pendingModDelete ? modDisplayName(pendingModDelete, locale) : "" })}
        detail={pendingModDelete ? <DetailLine label={t("modsTitle")} value={modDisplayName(pendingModDelete, locale)} /> : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={modDelete.isPending ? t("actionWorking") : t("delete")}
        busy={modDelete.isPending}
        onCancel={() => setPendingModDelete(null)}
        onConfirm={() => pendingModDelete && modDelete.mutate(pendingModDelete.id)}
      />
    </>
  );
}

function OverviewTab({
  capabilities,
  resource,
  worldCount,
  backupCount,
  modCount,
  onSelectTab
}: {
  capabilities: ProviderCapabilities;
  resource: GameServerResource;
  worldCount: number;
  backupCount: number;
  modCount: number;
  onSelectTab: (tab: TabId) => void;
}) {
  const { t } = useI18n();
  const resourceConfig = resource.spec.config ?? {};
  const hostPort = resource.spec.network?.hostPort ?? 0;
  const internalPort = resource.spec.network?.port ?? 0;
  const detailItems = [
    { label: t("difficulty"), value: difficultyLabel(stringProviderValue(resourceConfig, "difficulty", "classic"), t) },
    { label: t("maxPlayers"), value: String(gameServerMaxPlayers(resource)) },
    { label: t("version"), value: gameServerVersion(resource) },
    ...(hostPort > 0 && hostPort !== internalPort ? [{ label: t("hostPort"), value: String(hostPort) }] : [])
  ];
  return (
    <div className="space-y-4">
      <div className="grid gap-3 md:grid-cols-3">
        {capabilities.saveSnapshots && <SummaryButton icon={<FileText aria-hidden="true" />} label={t("tabWorlds")} value={String(worldCount)} onClick={() => onSelectTab("worlds")} />}
        {capabilities.backups && <SummaryButton icon={<Archive aria-hidden="true" />} label={t("tabBackups")} value={String(backupCount)} onClick={() => onSelectTab("backups")} />}
        {capabilities.mods && <SummaryButton icon={<Package aria-hidden="true" />} label={t("tabMods")} value={String(modCount)} onClick={() => onSelectTab("mods")} />}
      </div>
      <div className="rounded-lg border border-panel-line bg-slate-950/35 p-4">
        <h2 className="font-semibold">{t("serverInfo")}</h2>
        <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {detailItems.map((item) => <Info key={item.label} label={item.label} value={item.value} />)}
        </div>
      </div>
    </div>
  );
}

function stringProviderValue(payload: Record<string, unknown> | undefined, key: string, fallback = "") {
  const value = payload?.[key];
  return typeof value === "string" ? value : fallback;
}

function latestMonitoringEventTime(events: MonitoringEvent[]) {
  return events.reduce((latest, event) => {
    const next = Date.parse(event.timestamp);
    return Number.isFinite(next) ? Math.max(latest, next) : latest;
  }, 0);
}

function MonitoringTab({
  metrics,
  metricsRange,
  range,
  server,
  onRangeChange
}: {
  metrics?: Record<string, MetricSeries>;
  metricsRange?: MonitoringRange;
  range: MonitoringRangeValue;
  server: GameServerResource;
  onRangeChange: (range: MonitoringRangeValue) => void;
}) {
  const { t } = useI18n();
  const ranges: MonitoringRangeValue[] = ["15m", "1h", "6h", "24h"];
  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 rounded-lg border border-panel-line bg-slate-950/35 p-4 md:flex-row md:items-center md:justify-between">
        <div>
          <h2 className="font-semibold text-slate-100">{t("monitoringTabTitle")}</h2>
          <p className="mt-1 text-sm text-slate-500">{t("monitoringTabDescription", { name: server.name })}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {ranges.map((item) => (
            <button
              key={item}
              type="button"
              className={cn(
                "h-8 rounded-md border border-panel-line px-3 text-xs font-medium text-slate-400 transition hover:bg-slate-900 hover:text-slate-100",
                range === item && "border-panel-green/40 bg-panel-green/15 text-panel-green"
              )}
              onClick={() => onRangeChange(item)}
            >
              {item}
            </button>
          ))}
        </div>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <MonitoringChartCard color="#59d46f" icon={<Cpu aria-hidden="true" className="size-4" />} range={metricsRange} series={metrics?.cpu} />
        <MonitoringChartCard color="#a873ff" icon={<MemoryStick aria-hidden="true" className="size-4" />} range={metricsRange} series={metrics?.memory} />
        <MonitoringChartCard color="#59d46f" icon={<Users aria-hidden="true" className="size-4" />} range={metricsRange} series={metrics?.players} />
        <MonitoringChartCard color="#e6b84a" icon={<Clock aria-hidden="true" className="size-4" />} range={metricsRange} series={metrics?.uptime} />
      </div>
    </div>
  );
}

function ServerActivityTab({ events, loading, runtimeError }: { events: MonitoringEvent[]; loading: boolean; runtimeError: string }) {
  const { t } = useI18n();
  return (
    <div className="space-y-3">
      <div className="rounded-lg border border-panel-line bg-slate-950/35 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-100">{t("recentAlertsTitle")}</h2>
        <p className="mt-1 text-xs text-slate-500">{t("serverRecentEventsDescription")}</p>
      </div>
      {runtimeError ? (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-100">
          <p className="font-medium">{t("serverRuntimeError")}</p>
          <p className="mt-1 break-words text-red-100/85">{runtimeError}</p>
        </div>
      ) : null}
      {loading ? (
        <div className="rounded-lg border border-panel-line bg-slate-950/35 px-4 py-8 text-center text-sm text-slate-500">{t("loading")}</div>
      ) : (
        <ActivityTimeline events={events} />
      )}
    </div>
  );
}

function ResourceLimitsCard({
  cpuPercent,
  memoryMb,
  resource,
  restartPending,
  onEdit,
  onRestart
}: {
  cpuPercent?: number;
  memoryMb?: number;
  resource: GameServerResource;
  restartPending: boolean;
  onEdit: () => void;
  onRestart: () => void;
}) {
  const { t } = useI18n();
  const status = gameServerStatus(resource);
  const running = status === "running";
  const lifecycleLocked = isServerLifecyclePending(status);
  const cpuLimitCores = resource.spec.resources?.cpuLimitCores ?? 0;
  const memoryLimitMb = resource.spec.resources?.memoryLimitMb ?? 0;
  const configPendingRestart = gameServerConfigPendingRestart(resource);
  return (
    <Card className="p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="font-semibold">{t("runtimeResources")}</h2>
          <p className="mt-1 text-xs text-slate-500">{t("runtimeResourcesHint")}</p>
        </div>
        <Button className="h-8 px-2 text-xs" variant="secondary" onClick={onEdit} disabled={lifecycleLocked}>
          {t("adjustResources")}
        </Button>
      </div>
      <div className="mt-4 grid gap-2">
        <ResourceMetric
          icon={<Cpu aria-hidden="true" className="size-4" />}
          label={t("cpuLimit")}
          value={formatCpuLimitLabel(cpuLimitCores, t)}
          subValue={running && cpuPercent !== undefined ? `${cpuPercent.toFixed(1)}%` : t("notRunning")}
        />
        <ResourceMetric
          icon={<MemoryStick aria-hidden="true" className="size-4" />}
          label={t("memoryLimit")}
          value={formatMemoryLimitLabel(memoryLimitMb, t)}
          subValue={running && memoryMb !== undefined ? `${memoryMb} MB` : t("notRunning")}
        />
      </div>
      {configPendingRestart && (
        <div className="mt-3 rounded-md border border-panel-gold/25 bg-panel-gold/10 p-3">
          <p className="text-xs font-medium text-panel-gold">{t("resourceLimitsPendingRestart")}</p>
          <Button className="mt-2 h-8 px-2 text-xs" variant="gold" onClick={onRestart} disabled={restartPending || lifecycleLocked}>
            <RotateCcw aria-hidden="true" className="size-3.5" />
            {restartPending ? t("actionRestarting") : t("restartServerNow")}
          </Button>
        </div>
      )}
    </Card>
  );
}

function ResourceMetric({ icon, label, subValue, value }: { icon: ReactNode; label: string; subValue: string; value: string }) {
  return (
    <div className="flex items-center gap-3 rounded-md border border-panel-line bg-slate-950/35 p-3">
      <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-panel-green/25 bg-panel-green/10 text-panel-green">{icon}</span>
      <div className="min-w-0 flex-1">
        <p className="text-xs text-slate-500">{label}</p>
        <p className="mt-0.5 truncate text-sm font-semibold text-slate-100">{value}</p>
      </div>
      <span className="shrink-0 rounded-md border border-panel-line bg-slate-950/50 px-2 py-1 text-xs text-slate-400">{subValue}</span>
    </div>
  );
}

function ConsoleTab({
  command,
  commandPending,
  consoleError,
  capabilities,
  logs,
  logStatus,
  logStatusLabel,
  logStreamPaused,
  server,
  serverStatus,
  viewportRef,
  onChangeCommand,
  onClear,
  onQuickCommand,
  onSubmit,
  onTogglePause
}: {
  command: string;
  commandPending: boolean;
  consoleError: string;
  capabilities: ProviderCapabilities;
  logs: string[];
  logStatus: "idle" | "connecting" | "connected" | "error" | "paused";
  logStatusLabel: string;
  logStreamPaused: boolean;
  server: GameServerResource;
  serverStatus: ServerStatus;
  viewportRef: React.RefObject<HTMLDivElement | null>;
  onChangeCommand: (value: string) => void;
  onClear: () => void;
  onQuickCommand: (value: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onTogglePause: () => void;
}) {
  const { t } = useI18n();
  const consoleEnabled = serverStatus === "running";
  const showTerrariaShortcuts = supportsTerrariaConsoleShortcuts(server);
  const readyMessage = t(consoleReadyMessageKey(server));
  return (
    <div>
      <div className="overflow-hidden rounded-lg border border-panel-line bg-[#070b14]">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-panel-line bg-slate-950/70 px-4 py-2.5">
          <div className="flex min-w-0 items-center gap-3">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-900 text-panel-green">
              <Terminal aria-hidden="true" className="size-4" />
            </span>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-slate-100">{t("consoleCommandTitle")}</p>
              <p className="mt-0.5 truncate text-xs text-slate-500">{t("consoleOutputHint")}</p>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <span className={cn(
              "inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs",
              consoleEnabled ? "border-panel-green/25 bg-panel-green/10 text-panel-green" : "border-panel-line bg-slate-900/70 text-slate-500"
            )}>
              <span className={cn("size-1.5 rounded-full", consoleEnabled ? "bg-panel-green" : "bg-slate-600")} />
              {consoleEnabled ? logStatusLabel : t("statusStopped")}
            </span>
            <Button variant="secondary" className="px-2 py-1 text-xs" onClick={onTogglePause} disabled={!consoleEnabled || (logStatus !== "connected" && logStatus !== "paused")}>
              {logStreamPaused ? t("resumeLogs") : t("pauseLogs")}
            </Button>
            <Button variant="secondary" className="px-2 py-1 text-xs" onClick={onClear} disabled={logs.length === 0}>
              {t("clearLogs")}
            </Button>
          </div>
        </div>
        <LogViewport
          className="h-[420px] rounded-none border-0 bg-[#060a12]"
          emptyMessage={consoleEnabled ? t("consoleNoOutput") : t("consoleRequiresRunning")}
          logs={logs}
          logStatus={logStatus}
          viewportRef={viewportRef}
        />
        {showTerrariaShortcuts && <ConsoleCommandPanel capabilities={capabilities} commandPending={commandPending} disabled={!consoleEnabled} onRun={onQuickCommand} />}
        <form className="flex items-center gap-2 border-t border-panel-line bg-slate-950/70 px-3 py-3" onSubmit={onSubmit}>
          <span className={consoleEnabled ? "font-mono text-sm text-panel-green" : "font-mono text-sm text-slate-600"}>$</span>
          <input
            className="h-9 min-w-0 flex-1 bg-transparent font-mono text-sm text-slate-100 outline-none placeholder:text-slate-600 disabled:cursor-not-allowed disabled:text-slate-600"
            placeholder={consoleEnabled ? readyMessage : t("consoleRequiresRunning")}
            value={command}
            onChange={(event) => onChangeCommand(event.target.value)}
            disabled={!consoleEnabled || commandPending}
          />
          <Button className="h-9 px-3" variant="secondary" disabled={!consoleEnabled || command.trim() === "" || commandPending}>
            {commandPending ? t("sending") : t("send")}
          </Button>
        </form>
      </div>
      {consoleError && <p className="mt-3 rounded-md border border-panel-gold/30 bg-panel-gold/10 px-3 py-2 text-sm text-panel-gold">{consoleError}</p>}
    </div>
  );
}

type ParameterCommand = {
  key: string;
  label: string;
  command: string;
  icon: ReactNode;
  placeholder: string;
  danger?: boolean;
};

function ConsoleCommandPanel({
  capabilities,
  commandPending,
  disabled,
  onRun
}: {
  capabilities: ProviderCapabilities;
  commandPending: boolean;
  disabled: boolean;
  onRun: (value: string) => void;
}) {
  const { t } = useI18n();
  const [activeCommand, setActiveCommand] = useState<ParameterCommand | null>(null);
  const [parameter, setParameter] = useState("");
  const [pendingConfirm, setPendingConfirm] = useState<{ label: string; command: string } | null>(null);
  const blocked = disabled || commandPending;
  const parameterCommands: ParameterCommand[] = [
    { key: "say", label: t("consoleActionSay"), command: "say", icon: <Megaphone aria-hidden="true" className="size-3.5" />, placeholder: t("consoleActionSayPlaceholder") },
    ...(capabilities.kickPlayer ? [{ key: "kick", label: t("consoleActionKick"), command: "kick", icon: <UserX aria-hidden="true" className="size-3.5" />, placeholder: t("consoleActionPlayerPlaceholder"), danger: true }] : []),
    ...(capabilities.banPlayer ? [{ key: "ban", label: t("consoleActionBan"), command: "ban", icon: <Ban aria-hidden="true" className="size-3.5" />, placeholder: t("consoleActionPlayerPlaceholder"), danger: true }] : []),
    { key: "password", label: t("consoleActionPassword"), command: "password", icon: <KeyRound aria-hidden="true" className="size-3.5" />, placeholder: t("consoleActionPasswordPlaceholder") },
    { key: "motd", label: t("consoleActionMotd"), command: "motd", icon: <Megaphone aria-hidden="true" className="size-3.5" />, placeholder: t("consoleActionMotdPlaceholder") }
  ];
  const selectParameterCommand = (item: ParameterCommand) => {
    setPendingConfirm(null);
    setActiveCommand(item);
    setParameter("");
  };
  const runSimple = (label: string, command: string, danger = false) => {
    setActiveCommand(null);
    setParameter("");
    if (danger) {
      setPendingConfirm({ label, command });
      return;
    }
    setPendingConfirm(null);
    onRun(command);
  };
  const submitParameter = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!activeCommand) return;
    const value = parameter.trim();
    if (!value) return;
    const command = `${activeCommand.command} ${value}`;
    if (activeCommand.danger) {
      setPendingConfirm({ label: activeCommand.label, command });
      return;
    }
    onRun(command);
    setActiveCommand(null);
    setParameter("");
  };
  const confirmPending = () => {
    if (!pendingConfirm) return;
    onRun(pendingConfirm.command);
    setPendingConfirm(null);
    setActiveCommand(null);
    setParameter("");
  };
  return (
    <div className="border-t border-panel-line bg-slate-950/50 px-3 py-3">
      <div className="flex flex-wrap items-center gap-2">
        <QuickCommandButton disabled={blocked} icon={<Save aria-hidden="true" className="size-3.5" />} label={t("consoleActionSave")} onClick={() => runSimple(t("consoleActionSave"), "save")} />
        <QuickCommandButton disabled={blocked} icon={<Users aria-hidden="true" className="size-3.5" />} label={t("playerListCommand")} onClick={() => runSimple(t("playerListCommand"), "playing")} />
        <QuickCommandButton disabled={blocked} icon={<Clock aria-hidden="true" className="size-3.5" />} label={t("consoleActionTime")} onClick={() => runSimple(t("consoleActionTime"), "time")} />
        <QuickCommandButton disabled={blocked} icon={<FileText aria-hidden="true" className="size-3.5" />} label={t("consoleActionSeed")} onClick={() => runSimple(t("consoleActionSeed"), "seed")} />
        {parameterCommands.slice(0, 1).map((item) => (
          <QuickCommandButton key={item.key} disabled={blocked} icon={item.icon} label={item.label} onClick={() => selectParameterCommand(item)} />
        ))}
      </div>
      <details className="mt-2 group">
        <summary className="inline-flex cursor-pointer select-none items-center gap-2 rounded-md px-2 py-1 text-xs font-medium text-slate-400 transition hover:bg-slate-900 hover:text-slate-200 focus:outline-none focus:ring-2 focus:ring-panel-green/50">
          {t("consoleMoreActions")}
        </summary>
        <div className="mt-2 grid gap-2 lg:grid-cols-3">
          <CommandGroup title={t("consoleQueryGroup")}>
            <QuickCommandButton disabled={blocked} icon={<FileText aria-hidden="true" className="size-3.5" />} label={t("consoleActionVersion")} onClick={() => runSimple(t("consoleActionVersion"), "version")} />
            <QuickCommandButton disabled={blocked} icon={<Plug aria-hidden="true" className="size-3.5" />} label={t("consoleActionPort")} onClick={() => runSimple(t("consoleActionPort"), "port")} />
            <QuickCommandButton disabled={blocked} icon={<Users aria-hidden="true" className="size-3.5" />} label={t("consoleActionMaxPlayers")} onClick={() => runSimple(t("consoleActionMaxPlayers"), "maxplayers")} />
            <QuickCommandButton disabled={blocked} icon={<KeyRound aria-hidden="true" className="size-3.5" />} label={t("consoleActionShowPassword")} onClick={() => runSimple(t("consoleActionShowPassword"), "password")} />
            <QuickCommandButton disabled={blocked} icon={<Megaphone aria-hidden="true" className="size-3.5" />} label={t("consoleActionShowMotd")} onClick={() => runSimple(t("consoleActionShowMotd"), "motd")} />
          </CommandGroup>
          {showWorldAndBackupFeatures && (
            <CommandGroup title={t("consoleWorldGroup")}>
              <QuickCommandButton disabled={blocked} icon={<Sunrise aria-hidden="true" className="size-3.5" />} label={t("consoleActionDawn")} onClick={() => runSimple(t("consoleActionDawn"), "dawn")} />
              <QuickCommandButton disabled={blocked} icon={<Sun aria-hidden="true" className="size-3.5" />} label={t("consoleActionNoon")} onClick={() => runSimple(t("consoleActionNoon"), "noon")} />
              <QuickCommandButton disabled={blocked} icon={<Moon aria-hidden="true" className="size-3.5" />} label={t("consoleActionDusk")} onClick={() => runSimple(t("consoleActionDusk"), "dusk")} />
              <QuickCommandButton disabled={blocked} icon={<Moon aria-hidden="true" className="size-3.5" />} label={t("consoleActionMidnight")} onClick={() => runSimple(t("consoleActionMidnight"), "midnight")} />
              <QuickCommandButton disabled={blocked} icon={<Waves aria-hidden="true" className="size-3.5" />} label={t("consoleActionSettle")} onClick={() => runSimple(t("consoleActionSettle"), "settle")} />
            </CommandGroup>
          )}
          <CommandGroup title={t("consoleManageGroup")}>
            {parameterCommands.slice(1).map((item) => (
              <QuickCommandButton key={item.key} disabled={blocked} icon={item.icon} label={item.label} onClick={() => selectParameterCommand(item)} />
            ))}
            <QuickCommandButton disabled={blocked} danger icon={<Power aria-hidden="true" className="size-3.5" />} label={t("consoleActionExit")} onClick={() => runSimple(t("consoleActionExit"), "exit", true)} />
            <QuickCommandButton disabled={blocked} danger icon={<Power aria-hidden="true" className="size-3.5" />} label={t("consoleActionExitNoSave")} onClick={() => runSimple(t("consoleActionExitNoSave"), "exit-nosave", true)} />
          </CommandGroup>
        </div>
      </details>
      {activeCommand && (
        <form className="mt-3 flex flex-col gap-2 rounded-md border border-panel-line bg-slate-950/70 p-2 sm:flex-row sm:items-center" onSubmit={submitParameter}>
          <span className="inline-flex items-center gap-2 text-sm text-slate-300">{activeCommand.icon}{activeCommand.label}</span>
          <input
            className="h-9 min-w-0 flex-1 rounded-md border border-panel-line bg-slate-950 px-3 text-sm text-slate-100 outline-none placeholder:text-slate-500 focus:border-panel-green"
            placeholder={activeCommand.placeholder}
            value={parameter}
            onChange={(event) => setParameter(event.target.value)}
            disabled={blocked}
          />
          <Button type="submit" className="h-9 px-3" variant={activeCommand.danger ? "danger" : "secondary"} disabled={blocked || parameter.trim() === ""}>
            <Send aria-hidden="true" className="size-3.5" />
            {activeCommand.danger ? t("consoleReviewCommand") : t("send")}
          </Button>
          <Button type="button" className="h-9 px-3" variant="ghost" onClick={() => setActiveCommand(null)} disabled={blocked}>{t("cancel")}</Button>
        </form>
      )}
      {pendingConfirm && (
        <div className="mt-3 flex flex-col gap-2 rounded-md border border-red-500/20 bg-red-500/10 p-2 sm:flex-row sm:items-center sm:justify-between">
          <span className="text-sm text-red-100">{t("consoleConfirmCommand", { command: pendingConfirm.command })}</span>
          <div className="flex gap-2">
            <Button type="button" className="h-8 px-2 text-xs" variant="secondary" onClick={() => setPendingConfirm(null)} disabled={blocked}>{t("cancel")}</Button>
            <Button type="button" className="h-8 px-2 text-xs" variant="danger" onClick={confirmPending} disabled={blocked}>{pendingConfirm.label}</Button>
          </div>
        </div>
      )}
    </div>
  );
}

function CommandGroup({ children, title }: { children: ReactNode; title: string }) {
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/40 p-2">
      <p className="mb-2 text-xs font-medium text-slate-500">{title}</p>
      <div className="flex flex-wrap gap-2">{children}</div>
    </div>
  );
}

function QuickCommandButton({ danger, disabled, icon, label, onClick }: { danger?: boolean; disabled: boolean; icon: ReactNode; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      className={cn(
        "inline-flex h-8 items-center gap-1.5 rounded-md border px-2 text-xs font-medium transition focus:outline-none focus:ring-2 focus:ring-panel-green/50 disabled:cursor-not-allowed disabled:opacity-45",
        danger ? "border-red-500/20 bg-red-500/10 text-red-100 hover:bg-red-500/15" : "border-panel-line bg-slate-900/70 text-slate-200 hover:border-slate-600 hover:bg-slate-800"
      )}
      disabled={disabled}
      onClick={onClick}
    >
      {icon}
      <span>{label}</span>
    </button>
  );
}

function LogsTab({
  logStatus,
  logStatusLabel,
  logStreamPaused,
  logs,
  viewportRef,
  onClear,
  onTogglePause
}: {
  logStatus: "idle" | "connecting" | "connected" | "error" | "paused";
  logStatusLabel: string;
  logStreamPaused: boolean;
  logs: string[];
  viewportRef: React.RefObject<HTMLDivElement | null>;
  onClear: () => void;
  onTogglePause: () => void;
}) {
  const { t } = useI18n();
  return (
    <div className="overflow-hidden rounded-lg border border-panel-line bg-[#070b14]">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-panel-line bg-slate-950/70 px-4 py-2.5">
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-900 text-panel-green">
            <Terminal aria-hidden="true" className="size-4" />
          </span>
          <div className="min-w-0">
            <p className="truncate text-sm font-medium text-slate-100">{t("liveLogs")}</p>
            <p className="mt-0.5 truncate text-xs text-slate-500">{t("logsOutputHint")}</p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <span className={cn(
            "inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs",
            logStatus === "connected" ? "border-panel-green/25 bg-panel-green/10 text-panel-green" : logStatus === "error" ? "border-panel-gold/25 bg-panel-gold/10 text-panel-gold" : "border-panel-line bg-slate-900/70 text-slate-500"
          )}>
            <span className={cn("size-1.5 rounded-full", logStatus === "connected" ? "bg-panel-green" : logStatus === "error" ? "bg-panel-gold" : "bg-slate-600")} />
            {logStatusLabel}
          </span>
          <Button variant="secondary" className="px-2 py-1 text-xs" onClick={onTogglePause} disabled={logStatus !== "connected" && logStatus !== "paused"}>{logStreamPaused ? t("resumeLogs") : t("pauseLogs")}</Button>
          <Button variant="secondary" className="px-2 py-1 text-xs" onClick={onClear} disabled={logs.length === 0}>{t("clearLogs")}</Button>
        </div>
      </div>
      <LogViewport className="h-[420px] rounded-none border-0 bg-[#060a12]" logs={logs} logStatus={logStatus} viewportRef={viewportRef} />
      <div className="border-t border-panel-line bg-slate-950/60 px-3 py-2 text-xs text-slate-500">
        {logStatus === "idle" ? t("logsRequiresRunning") : t("logsLiveHint")}
      </div>
    </div>
  );
}

function ConfigTab({
  onRestart,
  onSave,
  provider,
  resource,
  restartPending,
  saveError,
  savePending,
  saveSuccess
}: {
  onRestart: () => void;
  onSave: (config: ServerConfigUpdatePayload, hostPort: number) => void;
  provider?: ProviderCatalog;
  resource: GameServerResource;
  restartPending: boolean;
  saveError: string;
  savePending: boolean;
  saveSuccess: boolean;
}) {
  const { t } = useI18n();
  const resourceTerrariaConfig = useMemo(() => terrariaConfigFromGameServer(resource), [resource]);
  const resourceProviderPayload = useMemo(() => initialProviderDraftFromResource(resource, provider), [provider, resource]);
  const resourceHostPort = gameServerJoinPort(resource);
  const resourceStatus = gameServerStatus(resource);
  const [draft, setDraft] = useState<TerrariaConfig>(resourceTerrariaConfig);
  const [providerDraft, setProviderDraft] = useState<ProviderConfigPayload>(() => resourceProviderPayload);
  const [hostPortDraft, setHostPortDraft] = useState(resourceHostPort);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [restartRecommended, setRestartRecommended] = useState(false);
  useEffect(() => setDraft(resourceTerrariaConfig), [resource.id, resourceTerrariaConfig]);
  useEffect(() => setProviderDraft(resourceProviderPayload), [resource.id, resourceProviderPayload]);
  useEffect(() => setHostPortDraft(resourceHostPort), [resource.id, resourceHostPort]);
  const isTerrariaProvider = terrariaProviderKeys.has(resource.providerKey);
  const normalizedDraft = useMemo(() => ({ ...draft, port: terrariaInternalPort }), [draft]);
  const preview = useQuery({
    queryKey: ["server-config-preview", resource.id, normalizedDraft],
    queryFn: () => previewTerrariaConfig(normalizedDraft),
    enabled: previewOpen && isTerrariaProvider,
    retry: false
  });
  const currentProviderPayload = resourceProviderPayload;
  const configDirty = isTerrariaProvider
    ? JSON.stringify(normalizedDraft) !== JSON.stringify({ ...resourceTerrariaConfig, port: terrariaInternalPort })
    : JSON.stringify(providerDraft) !== JSON.stringify(currentProviderPayload);
  const hostPortDirty = hostPortDraft !== resourceHostPort;
  const dirty = configDirty || hostPortDirty;
  const lifecycleLocked = isServerLifecyclePending(resourceStatus);
  const running = resourceStatus === "running";
  const disabled = lifecycleLocked || savePending;
  const restartRequired = running && !dirty && (gameServerConfigPendingRestart(resource) || restartRecommended);
  const showConfigActions = dirty || savePending || saveSuccess || restartRequired || lifecycleLocked;
  const update = <K extends keyof TerrariaConfig>(key: K, value: TerrariaConfig[K]) => setDraft((current) => ({ ...current, [key]: value }));
  useEffect(() => {
    if (dirty || !running || !gameServerConfigPendingRestart(resource)) {
      setRestartRecommended(false);
    }
  }, [dirty, resource, running]);
  useEffect(() => {
    if (saveSuccess && running) {
      setRestartRecommended(true);
    }
  }, [running, saveSuccess]);
  useEffect(() => {
    if (!previewOpen) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setPreviewOpen(false);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [previewOpen]);
  const secretSeed = secretSeedKeyFor(draft.seed);
  const worldEvilLabel = draft.worldEvil === "corruption" ? t("tagCorruption") : draft.worldEvil === "crimson" ? t("tagCrimson") : t("tagRandom");
  const difficultyLabel = draft.difficulty === "journey" ? t("tagJourney") : draft.difficulty === "classic" ? t("tagClassic") : draft.difficulty === "expert" ? t("tagExpert") : t("tagMaster");
  const worldSizeLabel = draft.worldSize === "small" ? t("tagSmallWorld") : draft.worldSize === "medium" ? t("tagMediumWorld") : t("tagLargeWorld");
  const seedLabel = secretSeed
    ? terrariaSecretSeeds.find((s) => s.key === secretSeed)?.label ?? draft.seed ?? ""
    : (draft.seed || t("tagRandom"));
  const seedModeCount = terrariaSeedModeCodes(draft).length;
  return (
    <form className="space-y-4" onSubmit={(event) => {
      event.preventDefault();
      if (!disabled && dirty) onSave(isTerrariaProvider ? normalizedDraft : providerDraft, hostPortDraft);
    }}>
      <div className="rounded-lg border border-panel-line bg-slate-950/40 p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="font-semibold">{t("serverConfig")}</h2>
            {lifecycleLocked && <span className="mt-1 inline-block rounded bg-panel-gold/15 px-2 py-1 text-xs text-panel-gold">{t("configLifecycleLocked")}</span>}
          </div>
          {isTerrariaProvider ? (
            <Button type="button" variant="secondary" className="h-8 px-2 text-xs" onClick={() => setPreviewOpen(true)}>
              <FileText aria-hidden="true" className="size-3.5" />
              {t("showPreview")}
            </Button>
          ) : null}
        </div>
        {isTerrariaProvider ? (
          <>
            <div className="mt-4 grid gap-4 lg:grid-cols-2">
              <div className="space-y-3">
                <Field label={t("serverName")}>
                  <Input value={draft.serverName ?? ""} onChange={(event) => update("serverName", event.target.value)} disabled={disabled} />
                </Field>
                <Field label={t("password")}>
                  <Input value={draft.password ?? ""} onChange={(event) => update("password", event.target.value)} disabled={disabled} />
                </Field>
                <Field label={t("motd")}>
                  <Input
                    type="text"
                    value={draft.motd ?? ""}
                    onChange={(event) => update("motd", event.target.value)}
                    disabled={disabled}
                  />
                </Field>
              </div>
              <div className="space-y-3">
                <Field label={t("externalPort")}>
                  <Input type="number" min={1024} max={65535} value={hostPortDraft} onChange={(event) => setHostPortDraft(Number(event.target.value))} disabled={disabled} />
                </Field>
                <Field label={t("maxPlayers")}>
                  <Input type="number" min={1} max={255} value={draft.maxPlayers} onChange={(event) => update("maxPlayers", Number(event.target.value))} disabled={disabled} />
                </Field>
              </div>
            </div>
            <div className="mt-3 grid gap-2 rounded-md border border-panel-line bg-slate-950/50 p-3">
              <Checkbox label={t("secureMode")} checked={draft.secure} onChange={(checked) => update("secure", checked)} disabled={disabled} />
              <Checkbox label={t("autoCreateWorld")} checked={draft.autoCreateWorld} onChange={(checked) => update("autoCreateWorld", checked)} disabled={disabled} />
            </div>
          </>
        ) : (
          <>
            <ProviderConfigFields
              disabled={disabled}
              fields={provider?.configSchema ?? []}
              payload={providerDraft}
              onChange={(field, value) => setProviderDraft((current) => updateProviderConfigPayload(current, field, value))}
            />
            <div className="mt-4 grid gap-4 lg:grid-cols-2">
              <Field label={t("externalPort")}>
                <Input type="number" min={1024} max={65535} value={hostPortDraft} onChange={(event) => setHostPortDraft(Number(event.target.value))} disabled={disabled} />
              </Field>
            </div>
          </>
        )}
      </div>

      {isTerrariaProvider ? <div className="rounded-lg border border-panel-line bg-slate-950/40 p-4">
        <h2 className="font-semibold">{t("worldCreationSettings")}</h2>
        <p className="mt-1 text-xs text-slate-500">{t("worldCreationReadonlyHint")}</p>
        <div className="mt-4 grid gap-4 lg:grid-cols-2">
          <ReadOnlyField label={t("worldName")} value={draft.worldName} />
          <ReadOnlyField label={t("gameVersion")} value={gameServerVersion(resource)} />
          <ReadOnlyField label={t("worldSize")} value={worldSizeLabel} />
          <ReadOnlyField label={t("worldEvil")} value={worldEvilLabel} />
          <ReadOnlyField label={t("difficulty")} value={difficultyLabel} />
          <ReadOnlyField label={t("internalPort")} value={String(terrariaInternalPort)} />
          <ReadOnlyField label={t("customSeed")} value={seedLabel} help={t("worldSeedHint")} />
          {seedModeCount > 0 ? (
            <ReadOnlyField label={t("seedModes")} value={t("seedModesSummary", { special: draft.specialSeeds?.length ?? 0, secret: draft.secretSeeds?.length ?? 0 })} />
          ) : null}
        </div>
      </div> : null}

      {showConfigActions && (
        <div className="sticky bottom-4 z-10 flex flex-col gap-3 rounded-lg border border-panel-line bg-panel-card/95 p-3 shadow-[0_10px_30px_rgba(0,0,0,0.25)] sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <p className={cn("text-sm font-medium", dirty || restartRequired ? "text-slate-100" : "text-slate-400")}>
              {lifecycleLocked
                ? t("configLifecycleLocked")
                : dirty || savePending
                  ? t("unsavedConfigChanges")
                  : restartRequired
                    ? t("configSavedRestartRequired")
                    : t("configSaved")}
            </p>
            {(dirty || restartRequired) && <p className="mt-0.5 text-xs text-slate-500">{restartRequired ? t("configRestartPrompt") : t("configActionHint")}</p>}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {restartRequired && (
              <Button type="button" variant="gold" disabled={restartPending} onClick={onRestart}>
                <RotateCcw aria-hidden="true" />
                {restartPending ? t("actionRestarting") : t("restartServerNow")}
              </Button>
            )}
            <Button
              type="button"
              variant="secondary"
              disabled={savePending || !dirty}
              onClick={() => {
                setDraft(resourceTerrariaConfig);
                setProviderDraft(resourceProviderPayload);
                setHostPortDraft(resourceHostPort);
              }}
            >
              {t("resetChanges")}
            </Button>
            <Button disabled={disabled || !dirty}>
              {savePending ? t("savingConfig") : t("saveConfig")}
            </Button>
          </div>
        </div>
      )}
      {saveError && <p className="rounded-md border border-panel-gold/30 bg-panel-gold/10 px-3 py-2 text-sm text-panel-gold">{saveError}</p>}
      {previewOpen && isTerrariaProvider && (
        <div
          className="fixed inset-0 z-50 flex justify-end bg-slate-950/50 backdrop-blur-sm"
          role="presentation"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setPreviewOpen(false);
          }}
        >
          <aside
            aria-label={t("previewServerConfig")}
            className="flex h-full w-full max-w-2xl flex-col border-l border-panel-line bg-panel-card shadow-[0_0_40px_rgba(0,0,0,0.35)]"
          >
            <div className="flex items-start justify-between gap-4 border-b border-panel-line px-5 py-4">
              <div className="min-w-0">
                <p className="text-sm font-semibold text-white">{t("previewServerConfig")}</p>
                <p className="mt-1 text-xs text-slate-500">{t("configPreviewHint")}</p>
              </div>
              <button
                aria-label={t("hidePreview")}
                className="flex size-8 shrink-0 items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/50"
                onClick={() => setPreviewOpen(false)}
                title={t("hidePreview")}
                type="button"
              >
                <X aria-hidden="true" className="size-4" />
              </button>
            </div>
            <div className="border-b border-panel-line bg-slate-950/50 px-5 py-2">
              <span className="rounded bg-slate-900 px-2 py-1 font-mono text-xs text-slate-500">serverconfig.txt</span>
            </div>
            <div className="min-h-0 flex-1 overflow-auto bg-[#060a12] p-5">
              {preview.isLoading ? (
                <p className="text-sm text-slate-400">{t("rendering")}</p>
              ) : preview.isError ? (
                <p className="text-sm text-panel-gold">{t("configPreviewUnavailable")}</p>
              ) : (
                <pre className="whitespace-pre-wrap font-mono text-xs leading-6 text-slate-300">{preview.data}</pre>
              )}
            </div>
          </aside>
        </div>
      )}
    </form>
  );
}

function ReadOnlyField({ help, label, value }: { help?: string; label: string; value: string }) {
  return (
    <div className="grid gap-1.5">
      <span className="flex items-center gap-2 text-xs font-medium text-slate-500">
        <span>{label}</span>
        {help ? <FieldHelp text={help} /> : null}
      </span>
      <div className="flex h-10 items-center rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-400">{value}</div>
    </div>
  );
}

function FieldHelp({ text }: { text: string }) {
  return (
    <span className="group/help relative inline-flex shrink-0">
      <button
        aria-label={text}
        className="flex size-4 cursor-help select-none items-center justify-center rounded-full border border-slate-600 bg-slate-950/70 text-[10px] font-bold leading-none text-slate-300 transition hover:border-panel-green/70 hover:text-panel-green focus:border-panel-green focus:text-panel-green focus:outline-none focus:ring-2 focus:ring-panel-green/30"
        type="button"
      >
        ?
      </button>
      <span className="pointer-events-none absolute left-1/2 top-6 z-20 hidden w-64 -translate-x-1/2 rounded-md border border-panel-line bg-slate-950 px-3 py-2 text-xs font-normal leading-5 text-slate-300 shadow-[0_10px_30px_rgba(0,0,0,0.35)] group-hover/help:block group-focus-within/help:block">
        {text}
      </span>
    </span>
  );
}

function initialProviderDraftFromResource(resource: GameServerResource, provider?: ProviderCatalog): ProviderConfigPayload {
  const payload: ProviderConfigPayload = {};
  const configPayload = resource.spec.config ?? {};
  for (const field of provider?.configSchema ?? []) {
    payload[field.name] = configPayload[field.name] ?? defaultProviderFieldValue(field);
  }
  return {
    ...payload,
    ...configPayload
  };
}

function defaultProviderFieldValue(field: ProviderConfigField): unknown {
  if (field.default !== undefined) return field.default;
  if (field.type === "number") return 0;
  if (field.type === "boolean") return false;
  return "";
}

function providerFieldLabel(field: ProviderConfigField, t: (key: MessageKey, params?: Record<string, string | number>) => string) {
  const key = providerFieldLabelKeys[field.name];
  return key ? t(key) : field.label;
}

function providerFieldHelp(field: ProviderConfigField, t: (key: MessageKey, params?: Record<string, string | number>) => string) {
  if (field.name === "adminPassword") return t("adminPasswordHelp");
  if (field.name === "clusterToken") return t("clusterTokenHelp");
  if (field.name === "eulaAccepted") return t("minecraftEulaHelp");
  return field.help ?? "";
}

function ProviderConfigFields({
  disabled,
  fields,
  onChange,
  payload
}: {
  disabled: boolean;
  fields: ProviderConfigField[];
  onChange: (field: ProviderConfigField, value: string | boolean) => void;
  payload: ProviderConfigPayload;
}) {
  const { t } = useI18n();
  if (fields.length === 0) {
    return <p className="mt-4 rounded-md border border-panel-line bg-slate-950/50 px-3 py-2 text-sm text-slate-500">{t("none")}</p>;
  }
  return (
    <div className="mt-4 grid gap-4 lg:grid-cols-2">
      {fields.map((field) => {
        const label = providerFieldLabel(field, t);
        const help = providerFieldHelp(field, t);
        const value = payload[field.name];
        if (field.type === "boolean") {
          return (
            <div key={field.name} className="rounded-md border border-panel-line bg-slate-950/50 px-3 py-2">
              <Checkbox label={label} checked={Boolean(value)} onChange={(checked) => onChange(field, checked)} disabled={disabled} />
              {help ? <p className="mt-1 text-xs text-slate-500">{help}</p> : null}
            </div>
          );
        }
        return (
          <Field key={field.name} label={label} required={field.required}>
            {field.type === "select" ? (
              <Select disabled={disabled} value={String(value ?? "")} onChange={(nextValue) => onChange(field, nextValue)}>
                {(field.options ?? []).map((option) => (
                  <option key={option.value} value={option.value}>{option.label}</option>
                ))}
              </Select>
            ) : (
              <Input
                disabled={disabled}
                type={field.type === "password" ? "password" : field.type === "number" ? "number" : "text"}
                value={field.type === "number" ? Number(value ?? 0) : String(value ?? "")}
                onChange={(event) => onChange(field, event.target.value)}
              />
            )}
            {help ? <span className="text-xs text-slate-500">{help}</span> : null}
          </Field>
        );
      })}
    </div>
  );
}

function ResourceLimitsDialog({
  open,
  resource,
  savePending,
  onCancel,
  onSave
}: {
  open: boolean;
  resource: GameServerResource;
  savePending: boolean;
  onCancel: () => void;
  onSave: (resources: ResourceLimits) => void;
}) {
  const { t } = useI18n();
  const resourceLimits = useMemo<ResourceLimits>(
    () => ({
      cpuLimitCores: resource.spec.resources?.cpuLimitCores ?? 0,
      memoryLimitMb: resource.spec.resources?.memoryLimitMb ?? 0
    }),
    [resource.spec.resources?.cpuLimitCores, resource.spec.resources?.memoryLimitMb]
  );
  const status = gameServerStatus(resource);
  const [draft, setDraft] = useState<ResourceLimits>(resourceLimits);
  const lifecycleLocked = isServerLifecyclePending(status);
  const dirty = draft.cpuLimitCores !== resourceLimits.cpuLimitCores || draft.memoryLimitMb !== resourceLimits.memoryLimitMb;
  useEffect(() => {
    if (open) {
      setDraft(resourceLimits);
    }
  }, [open, resource.id, resourceLimits]);
  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !savePending) onCancel();
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onCancel, open, savePending]);
  if (!open) return null;
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/70 px-4 backdrop-blur-sm"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !savePending) onCancel();
      }}
    >
      <form
        aria-labelledby="resource-dialog-title"
        aria-modal="true"
        className="w-full max-w-lg rounded-lg border border-panel-line bg-panel-card p-5 shadow-[0_12px_40px_rgba(0,0,0,0.35)]"
        role="dialog"
        onSubmit={(event) => {
          event.preventDefault();
          if (!savePending && !lifecycleLocked && dirty) onSave(draft);
        }}
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-panel-green">{t("runtimeResources")}</p>
            <h2 className="mt-2 text-lg font-semibold text-white" id="resource-dialog-title">{t("adjustResources")}</h2>
            <p className="mt-2 text-sm leading-6 text-slate-400">{status === "running" ? t("resourceLimitsApplyAfterRestart") : t("resourceLimitsApplyOnStart")}</p>
          </div>
          <button
            aria-label={t("cancel")}
            className="flex size-8 items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/50"
            disabled={savePending}
            onClick={onCancel}
            type="button"
          >
            <X aria-hidden="true" className="size-4" />
          </button>
        </div>
        <div className="mt-5 grid gap-3 sm:grid-cols-2">
          <Field label={t("cpuLimit")}>
            <Select value={String(draft.cpuLimitCores)} onChange={(value) => setDraft((current) => ({ ...current, cpuLimitCores: Number(value) }))} disabled={savePending || lifecycleLocked}>
              {cpuLimitOptions.map((value) => (
                <option key={value} value={value}>{formatCpuLimitLabel(value, t)}</option>
              ))}
            </Select>
          </Field>
          <Field label={t("memoryLimit")}>
            <Select value={String(draft.memoryLimitMb)} onChange={(value) => setDraft((current) => ({ ...current, memoryLimitMb: Number(value) }))} disabled={savePending || lifecycleLocked}>
              {memoryLimitOptions.map((value) => (
                <option key={value} value={value}>{formatMemoryLimitLabel(value, t)}</option>
              ))}
            </Select>
          </Field>
        </div>
        {lifecycleLocked && <p className="mt-3 rounded-md border border-panel-gold/25 bg-panel-gold/10 px-3 py-2 text-xs text-panel-gold">{t("configLifecycleLocked")}</p>}
        <div className="mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button type="button" variant="secondary" onClick={onCancel} disabled={savePending}>{t("cancel")}</Button>
          <Button disabled={savePending || lifecycleLocked || !dirty}>{savePending ? t("savingConfig") : t("saveResourceLimits")}</Button>
        </div>
      </form>
    </div>
  );
}

function Field({ children, label, required }: { children: ReactNode; label: string; required?: boolean }) {
  const { t } = useI18n();
  return (
    <label className="grid gap-1.5">
      <span className="flex items-center gap-2 text-xs font-medium text-slate-500">
        <span>{label}</span>
        {required ? <span className="rounded bg-panel-gold/15 px-1.5 py-0.5 text-[10px] font-semibold text-panel-gold">{t("requiredField")}</span> : null}
      </span>
      {children}
    </label>
  );
}

function Select({ children, disabled, onChange, value }: { children: ReactNode; disabled?: boolean; onChange: (value: string) => void; value: string }) {
  return (
    <select
      className="h-10 rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-100 outline-none focus:border-panel-green disabled:cursor-not-allowed disabled:opacity-60"
      disabled={disabled}
      value={value}
      onChange={(event) => onChange(event.target.value)}
    >
      {children}
    </select>
  );
}

function Checkbox({ checked, disabled, label, onChange }: { checked: boolean; disabled?: boolean; label: string; onChange: (checked: boolean) => void }) {
  return (
    <label className={cn(
      "grid cursor-pointer grid-cols-[1fr_auto] items-center gap-3 rounded-md border px-3 py-2.5 text-sm font-medium transition",
      checked ? "border-panel-green/45 bg-panel-green/10 text-slate-100" : "border-panel-line bg-slate-950/50 text-slate-300 hover:bg-slate-900/70",
      disabled && "cursor-not-allowed opacity-60"
    )}>
      <span className="min-w-0 leading-5">{label}</span>
      <input
        className="sr-only"
        checked={checked}
        disabled={disabled}
        type="checkbox"
        onChange={(event) => onChange(event.target.checked)}
      />
      <span className={cn(
        "flex size-5 shrink-0 items-center justify-center rounded border",
        checked ? "border-panel-green bg-panel-green text-slate-950" : "border-panel-line bg-slate-950 text-transparent"
      )}>
        <Check aria-hidden="true" className="size-3.5" />
      </span>
    </label>
  );
}

function WorldsTab({
  currentServerId,
  deleting,
  downloadingId,
  isError,
  isLoading,
  items,
  onDelete,
  onDownload,
  onCreateSnapshot,
  snapshotting
}: {
  currentServerId: string;
  deleting: boolean;
  downloadingId: string;
  isError: boolean;
  isLoading: boolean;
  items: World[];
  onDelete: (world: World) => void;
  onDownload: (world: World) => void;
  onCreateSnapshot: () => void;
  snapshotting: boolean;
}) {
  const { locale, t } = useI18n();
  return (
    <ResourcePanel
      title={t("detailWorldActions")}
      href="/worlds"
      action={
        <Button variant="secondary" onClick={onCreateSnapshot} disabled={snapshotting}>
          <FileArchive aria-hidden="true" />
          {snapshotting ? t("savingSnapshot") : t("saveWorldSnapshot")}
        </Button>
      }
    >
      {isError ? <p className="text-sm text-panel-gold">{t("apiWorldsUnavailable")}</p> : null}
      {!isError && isLoading ? <p className="text-sm text-slate-400">{t("loading")}</p> : null}
      {!isError && !isLoading && items.length === 0 ? <p className="text-sm text-slate-400">{t("noServerWorldSnapshots")}</p> : null}
      <div className="grid gap-2">
        {items.map((world) => (
          <ResourceRow
            key={world.id}
            title={<Link href={`/worlds/${world.id}`} className="transition hover:text-panel-green">{world.name}</Link>}
            meta={`${world.bytes} · ${localizeRelativeTime(world.modified, locale)}`}
            actions={
              <>
                {isWorldActiveOnServer(world, currentServerId) && (
                  <span className="inline-flex items-center gap-2 rounded-md border border-panel-green/30 bg-panel-green/10 px-3 py-2 text-sm font-medium text-panel-green">
                    <CheckCircle2 aria-hidden="true" className="size-4" />
                    {t("currentWorld")}
                  </span>
                )}
                <ActionButton
                  disabled={downloadingId === world.id}
                  label={downloadingId === world.id ? t("downloading") : t("download")}
                  icon={<Download aria-hidden="true" />}
                  onClick={() => onDownload(world)}
                />
                <Button variant="danger" aria-label={t("delete")} onClick={() => onDelete(world)} disabled={deleting}>
                  <Trash2 aria-hidden="true" />
                </Button>
              </>
            }
          />
        ))}
      </div>
    </ResourcePanel>
  );
}

function BackupsTab({
  creating,
  deleting,
  downloadingId,
  isError,
  isLoading,
  items,
  onDelete,
  onDownload,
  restoring,
  serverStatus,
  onCreate,
  onRestore
}: {
  creating: boolean;
  deleting: boolean;
  downloadingId: string;
  isError: boolean;
  isLoading: boolean;
  items: Backup[];
  onDelete: (backup: Backup) => void;
  onDownload: (backup: Backup) => void;
  restoring: boolean;
  serverStatus: ServerStatus;
  onCreate: () => void;
  onRestore: (backup: Backup) => void;
}) {
  const { locale, t } = useI18n();
  const restoreAction = describeResourceAction({ kind: "restoreBackup", serverStatus });
  return (
    <ResourcePanel
      title={t("detailBackupActions")}
      href="/backups"
      action={
        <Button variant="gold" onClick={onCreate} disabled={creating}>
          <Archive aria-hidden="true" />
          {creating ? t("backingUp") : t("createBackupNow")}
        </Button>
      }
    >
      {isError ? <p className="text-sm text-panel-gold">{t("apiBackupsUnavailable")}</p> : null}
      {!isError && restoreAction.reasonKey ? <p className="mb-3 text-sm text-slate-500">{t(restoreAction.reasonKey)}</p> : null}
      {!isError && isLoading ? <p className="text-sm text-slate-400">{t("loading")}</p> : null}
      {!isError && !isLoading && items.length === 0 ? <p className="text-sm text-slate-400">{t("noBackupsYet")}</p> : null}
      {items.length > 0 ? (
        <div className="overflow-hidden rounded-lg border border-panel-line bg-slate-950/35">
          <div className="divide-y divide-panel-line">
            {items.map((backup) => (
              <div key={backup.id} className="grid gap-3 px-4 py-3 transition hover:bg-slate-900/40 lg:grid-cols-[8rem_minmax(0,1fr)_auto] lg:items-center">
                <div className="flex items-center gap-2 text-sm text-slate-400 lg:block">
                  <Clock aria-hidden="true" className="size-4 text-slate-500 lg:mb-2" />
                  <p className="font-medium text-slate-200">{localizeRelativeTime(backup.created, locale)}</p>
                  <p className="hidden text-xs text-slate-500 lg:block">{formatBackupDate(backup.createdAt, locale)}</p>
                </div>
                <div className="min-w-0">
                  <div className="flex min-w-0 flex-wrap items-center gap-2">
                    <Link href={`/backups/${backup.id}`} className="truncate font-medium text-white transition hover:text-panel-green">{backup.name}</Link>
                    <span className={cn("shrink-0 rounded px-2 py-0.5 text-xs font-medium", backup.type === "Auto" ? "bg-slate-800 text-slate-300" : "bg-panel-gold/15 text-panel-gold")}>
                      {backup.type === "Auto" ? t("typeAuto") : t("typeManual")}
                    </span>
                  </div>
                  <p className="mt-1 truncate text-sm text-slate-500">{backup.world}</p>
                  <p className="mt-1 text-sm font-medium text-slate-300">{backup.size}</p>
                </div>
                <div className="flex shrink-0 flex-wrap gap-2 lg:justify-end">
                  <Button
                    variant="secondary"
                    aria-label={t("restore")}
                    onClick={() => onRestore(backup)}
                    disabled={restoreAction.disabled || restoring}
                    title={restoreAction.reasonKey ? t(restoreAction.reasonKey) : undefined}
                  >
                    <RotateCcw aria-hidden="true" />
                  </Button>
                  <ActionButton
                    disabled={downloadingId === backup.id}
                    label={downloadingId === backup.id ? t("downloading") : t("download")}
                    icon={<Download aria-hidden="true" />}
                    onClick={() => onDownload(backup)}
                  />
                  <Button variant="danger" aria-label={t("delete")} onClick={() => onDelete(backup)} disabled={deleting}>
                    <Trash2 aria-hidden="true" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : null}
    </ResourcePanel>
  );
}

function ModsTab({
  availableMods,
  assigning,
  deleting,
  isError,
  isLoading,
  items,
  libraryError,
  modPacks,
  pendingRestart,
  packInstalling,
  serverStatus,
  toggling,
  workshopUnsupported,
  onAssignMods,
  onDelete,
  onInstallPack,
  onToggle
}: {
  availableMods: ModFile[];
  assigning: boolean;
  deleting: boolean;
  isError: boolean;
  isLoading: boolean;
  items: ModFile[];
  libraryError: boolean;
  modPacks: ModPack[];
  pendingRestart: boolean;
  packInstalling: boolean;
  serverStatus: ServerStatus;
  toggling: boolean;
  workshopUnsupported: boolean;
  onAssignMods: (mods: ModFile[]) => void;
  onDelete: (mod: ModFile) => void;
  onInstallPack: (pack: ModPack) => void;
  onToggle: (mod: ModFile) => void;
}) {
  const { locale, t } = useI18n();
  const [installerOpen, setInstallerOpen] = useState(false);
  const [installSource, setInstallSource] = useState<ModInstallSource>("library");
  const [selectedModIds, setSelectedModIds] = useState<string[]>([]);
  const modAction = describeResourceAction({ kind: "modifyMods", serverStatus });
  const blocked = modAction.disabled;
  const workshopBlockReason = workshopUnsupported ? t("workshopArmUnsupported") : "";
  const selectedMods = useMemo(
    () => availableMods.filter((mod) => selectedModIds.includes(mod.id) && !isModInstalledOnServer(mod, items)),
    [availableMods, items, selectedModIds]
  );
  useEffect(() => {
    setSelectedModIds((current) => current.filter((modId) => {
      const mod = availableMods.find((item) => item.id === modId);
      return mod ? !isModInstalledOnServer(mod, items) : false;
    }));
  }, [availableMods, items]);
  useEffect(() => {
    if (!installerOpen) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setInstallerOpen(false);
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [installerOpen]);

  return (
    <ResourcePanel
      title={t("detailModActions")}
      href="/mods"
      action={
        <Button variant="secondary" onClick={() => setInstallerOpen(true)}>
          <Package aria-hidden="true" />
          {t("installMods")}
        </Button>
      }
    >
      <div className="space-y-4">
        {modAction.reasonKey ? <p className="text-sm text-panel-gold">{t(modAction.reasonKey)}</p> : null}
        {libraryError ? <p className="text-sm text-panel-gold">{t("modsApiUnavailable")}</p> : null}
        {pendingRestart ? (
          <div className="rounded-md border border-panel-gold/30 bg-panel-gold/10 px-3 py-2 text-sm text-panel-gold">
            {t("modChangesPendingRestart")}
          </div>
        ) : null}

        <div className="overflow-hidden rounded-lg border border-panel-line bg-slate-950/35">
          {isError ? <p className="px-4 pb-4 text-sm text-panel-gold">{t("modsApiUnavailable")}</p> : null}
          {!isError && isLoading ? <p className="px-4 py-4 text-sm text-slate-400">{t("loading")}</p> : null}
          {!isError && !isLoading && items.length === 0 ? <p className="px-4 py-4 text-sm text-slate-400">{t("noModsUploaded")}</p> : null}
          <div className="divide-y divide-panel-line">
            {items.map((mod) => (
              <ServerModRow
                key={mod.id}
                disabled={blocked}
                deleting={deleting}
                mod={mod}
                toggling={toggling}
                onDelete={onDelete}
                onToggle={onToggle}
              />
            ))}
          </div>
        </div>
      </div>
      {installerOpen ? (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/70 px-4 backdrop-blur-sm"
          role="presentation"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setInstallerOpen(false);
          }}
        >
          <div
            aria-labelledby="mod-installer-title"
            aria-modal="true"
            className="max-h-[82vh] w-full max-w-5xl overflow-hidden rounded-lg border border-panel-line bg-panel-card shadow-[0_12px_40px_rgba(0,0,0,0.35)]"
            role="dialog"
          >
            <div className="flex items-start justify-between gap-4 border-b border-panel-line px-5 py-4">
              <div>
                <h3 className="font-semibold text-white" id="mod-installer-title">{t("installMods")}</h3>
                <p className="mt-1 text-sm text-slate-500">{t("installOptionsHint")}</p>
              </div>
              <button
                aria-label={t("cancel")}
                className="flex size-8 items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/50"
                onClick={() => setInstallerOpen(false)}
                type="button"
              >
                <X aria-hidden="true" className="size-4" />
              </button>
            </div>
            <div className="max-h-[calc(82vh-5rem)] overflow-y-auto p-5">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex flex-wrap gap-2">
                  <InstallerSourceTab
                    active={installSource === "library"}
                    count={availableMods.length}
                    label={t("modLibrary")}
                    onClick={() => setInstallSource("library")}
                  />
                  <InstallerSourceTab
                    active={installSource === "packs"}
                    count={modPacks.length}
                    label={t("modPacks")}
                    onClick={() => setInstallSource("packs")}
                  />
                </div>
                <Link href="/mods" className="inline-flex items-center justify-center rounded-md border border-panel-line bg-slate-900/70 px-3 py-2 text-sm font-medium text-slate-100 transition hover:bg-slate-800">
                  {t("openFullManager")}
                </Link>
              </div>

              <div className="mt-4 rounded-lg border border-panel-line bg-slate-950/35">
                <div className="flex flex-col gap-3 border-b border-panel-line px-4 py-3 lg:flex-row lg:items-center lg:justify-between">
                  <div>
                    <h4 className="font-semibold text-white">{installSource === "library" ? t("installFromLibrary") : t("modPacks")}</h4>
                    <p className="mt-1 text-sm text-slate-500">{installSource === "library" ? t("installFromLibraryHint") : t("installModPacksHint")}</p>
                  </div>
                  {installSource === "library" ? (
                    <div className="flex shrink-0 flex-wrap items-center gap-2">
                      <span className="rounded-md border border-panel-line bg-slate-950/60 px-2.5 py-1.5 text-xs font-medium text-slate-400">
                        {t("selectedModsCount", { count: selectedMods.length })}
                      </span>
                      {selectedModIds.length > 0 ? (
                        <Button variant="secondary" className="h-8 px-2 text-xs" onClick={() => setSelectedModIds([])} disabled={assigning}>
                          {t("clearSelection")}
                        </Button>
                      ) : null}
                    </div>
                  ) : null}
                </div>

                {installSource === "library" ? (
                  availableMods.length > 0 ? (
                    <>
                    <div className="max-h-[44vh] divide-y divide-panel-line overflow-y-auto">
                      {availableMods.map((mod) => {
                        const blockedByArchitecture = workshopUnsupported && isWorkshopBackedMod(mod);
                        const installed = isModInstalledOnServer(mod, items);
                        const selected = selectedModIds.includes(mod.id);
                        const disabled = assigning || blocked || blockedByArchitecture || installed;
                        return (
                          <ModInstallOptionRow
                            blockedReason={blockedByArchitecture ? workshopBlockReason : modAction.reasonKey ? t(modAction.reasonKey) : undefined}
                            disabled={disabled}
                            installed={installed}
                            key={mod.id}
                            meta={modInstallMeta(mod, locale, t)}
                            selected={selected}
                            title={modDisplayName(mod, locale)}
                            onToggle={() => {
                              if (disabled) return;
                              setSelectedModIds((current) => current.includes(mod.id) ? current.filter((id) => id !== mod.id) : [...current, mod.id]);
                            }}
                          />
                        );
                      })}
                    </div>
                    <div className="flex flex-col gap-3 border-t border-panel-line bg-slate-950/45 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                      <p className="text-sm text-slate-500">
                        {selectedMods.length > 0 ? t("installSelectedModsHint", { count: selectedMods.length }) : t("selectModsToInstallHint")}
                      </p>
                      <Button
                        onClick={() => {
                          if (selectedMods.length === 0) return;
                          onAssignMods(selectedMods);
                          setSelectedModIds([]);
                          setInstallerOpen(false);
                        }}
                        disabled={assigning || blocked || selectedMods.length === 0}
                        title={modAction.reasonKey ? t(modAction.reasonKey) : undefined}
                      >
                        <Package aria-hidden="true" />
                        {assigning ? t("actionWorking") : t("installSelectedMods", { count: selectedMods.length })}
                      </Button>
                    </div>
                    </>
                  ) : (
                    <InstallerEmptyState message={t("noGlobalMods")} />
                  )
                ) : modPacks.length > 0 ? (
                  <div className="divide-y divide-panel-line">
                    {modPacks.map((pack) => {
                      const blockedByArchitecture = workshopUnsupported && modPackHasWorkshopMods(pack);
                      const installedCount = pack.mods.filter((mod) => isModInstalledOnServer(mod, items)).length;
                      const allInstalled = pack.mods.length > 0 && installedCount === pack.mods.length;
                      return (
                        <ResourceRow
                          className="rounded-none border-0 bg-transparent px-4"
                          key={pack.id}
                          title={<Link href={`/mods/packs/${pack.id}`} className="transition hover:text-panel-green">{pack.name}</Link>}
                          meta={modPackInstallMeta(pack, locale, t, installedCount)}
                          actions={
                            <Button
                              variant="secondary"
                              onClick={() => {
                                setInstallerOpen(false);
                                onInstallPack(pack);
                              }}
                              disabled={packInstalling || blocked || pack.modIds.length === 0 || blockedByArchitecture || allInstalled}
                              title={blockedByArchitecture ? workshopBlockReason : modAction.reasonKey ? t(modAction.reasonKey) : undefined}
                            >
                              {allInstalled ? <CheckCircle2 aria-hidden="true" /> : <Package aria-hidden="true" />}
                              {allInstalled ? t("alreadyInstalled") : t("installModPack")}
                            </Button>
                          }
                        />
                      );
                    })}
                  </div>
                ) : (
                  <InstallerEmptyState message={t("noModPacks")} />
                )}
              </div>
            </div>
          </div>
        </div>
      ) : null}
    </ResourcePanel>
  );
}

function ResourcePanel({
  title,
  href,
  action,
  children,
  target
}: {
  title: string;
  href: string;
  action?: ReactNode;
  children: ReactNode;
  target?: ReactNode;
}) {
  const { t } = useI18n();
  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <h2 className="font-semibold">{title}</h2>
        <div className="flex flex-wrap items-center gap-2">
          {target}
          {action ?? null}
          <Link href={href} className="inline-flex items-center justify-center rounded-md border border-panel-line bg-slate-900/70 px-3 py-2 text-sm font-medium text-slate-100 transition hover:bg-slate-800">
            {t("openFullManager")}
          </Link>
        </div>
      </div>
      {children}
    </div>
  );
}

function ServerModRow({
  deleting,
  disabled,
  mod,
  toggling,
  onDelete,
  onToggle
}: {
  deleting: boolean;
  disabled: boolean;
  mod: ModFile;
  toggling: boolean;
  onDelete: (mod: ModFile) => void;
  onToggle: (mod: ModFile) => void;
}) {
  const { locale, t } = useI18n();
  const status = modRuntimeStatus(mod);
  return (
    <div className="flex flex-col gap-3 px-4 py-3 transition hover:bg-slate-900/40 lg:flex-row lg:items-center lg:justify-between">
      <div className="flex min-w-0 items-start gap-3">
        <span className={cn(
          "mt-0.5 flex size-10 shrink-0 items-center justify-center rounded-md border",
          mod.enabled ? "border-panel-green/30 bg-panel-green/10 text-panel-green" : "border-panel-line bg-slate-950/60 text-slate-500"
        )}>
          <Package aria-hidden="true" className="size-4" />
        </span>
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <Link href={`/mods/${mod.id}`} className="truncate text-sm font-semibold text-white transition hover:text-panel-green">
              {modDisplayName(mod, locale)}
            </Link>
            <span className={cn("shrink-0 rounded px-2 py-0.5 text-xs font-medium", status.className)}>
              {t(status.labelKey)}
            </span>
          </div>
          <p className="mt-1 truncate text-xs text-slate-500">
            {mod.size} · {localizeRelativeTime(mod.created, locale)}
          </p>
          {mod.dependencies && mod.dependencies.length > 0 ? (
            <p className="mt-1 truncate text-xs text-slate-500">
              {t("dependencies")}: {mod.dependencies.join(", ")}
            </p>
          ) : null}
        </div>
      </div>
      <div className="flex shrink-0 flex-wrap gap-2 lg:justify-end">
        <Button variant="secondary" onClick={() => onToggle(mod)} disabled={toggling || disabled}>
          <Power aria-hidden="true" />
          {mod.enabled ? t("disable") : t("enable")}
        </Button>
        <Button variant="danger" aria-label={t("delete")} onClick={() => onDelete(mod)} disabled={deleting || disabled}>
          <Trash2 aria-hidden="true" />
        </Button>
      </div>
    </div>
  );
}

function InstallDependencyDetail({
  dependencies,
  label,
  name,
  summary
}: {
  dependencies: string[];
  label: string;
  name: string;
  summary?: string;
}) {
  const { t } = useI18n();
  return (
    <div className="space-y-1">
      <DetailLine label={label} value={name} />
      {summary ? <p className="text-slate-400">{summary}</p> : null}
      {dependencies.length > 0 ? (
        <p className="text-panel-gold">{t("autoInstallDependencies", { names: dependencies.join(", ") })}</p>
      ) : null}
    </div>
  );
}

function modInstallMeta(mod: ModFile, locale: ReturnType<typeof useI18n>["locale"], t: ReturnType<typeof useI18n>["t"]) {
  const base = `${mod.size} · ${localizeRelativeTime(mod.created, locale)}`;
  if (!mod.dependencies || mod.dependencies.length === 0) return base;
  return `${base} · ${t("dependencies")}: ${mod.dependencies.join(", ")}`;
}

function modPackInstallMeta(pack: ModPack, locale: ReturnType<typeof useI18n>["locale"], t: ReturnType<typeof useI18n>["t"], installedCount = 0) {
  const description = pack.description || pack.mods.map((mod) => modDisplayName(mod, locale)).join(", ");
  const dependencies = dependencyNamesForMods(pack.mods);
  const dependencyText = dependencies.length > 0 ? ` · ${t("dependencies")}: ${dependencies.join(", ")}` : "";
  const installedText = installedCount > 0 ? ` · ${t("installedModsCount", { count: installedCount, total: pack.mods.length })}` : "";
  return `${pack.mods.length} · ${description}${dependencyText}${installedText}`;
}

function dependencyNamesForMods(mods: ModFile[]) {
  return Array.from(new Set(mods.flatMap((mod) => mod.dependencies ?? [])));
}

function isWorkshopBackedMod(mod: ModFile) {
  return mod.source === "workshop" || Boolean(mod.workshopId);
}

function modPackHasWorkshopMods(pack: ModPack) {
  return pack.mods.some(isWorkshopBackedMod);
}

function modInstallKeys(mod: ModFile) {
  return [
    mod.workshopId ? `workshop:${mod.workshopId}` : "",
    mod.fileName ? `file:${mod.fileName.toLowerCase()}` : "",
    mod.modName ? `name:${mod.modName.toLowerCase()}` : "",
    mod.title ? `title:${mod.title.toLowerCase()}` : ""
  ].filter(Boolean);
}

function isModInstalledOnServer(mod: ModFile, installedMods: ModFile[]) {
  const candidateKeys = new Set(modInstallKeys(mod));
  if (candidateKeys.size === 0) return installedMods.some((item) => item.id === mod.id);
  return installedMods.some((item) => item.id === mod.id || modInstallKeys(item).some((key) => candidateKeys.has(key)));
}

function isArmArchitecture(architecture: string | undefined) {
  const value = (architecture ?? "").toLowerCase();
  return value.startsWith("arm") || value.includes("aarch64");
}

function modRuntimeStatus(mod: ModFile): { labelKey: "enabled" | "disabled" | "notSynced" | "notApplied" | "pendingRestart"; className: string } {
  if (!mod.enabled) {
    return { labelKey: "disabled", className: "bg-slate-800 text-slate-400" };
  }
  if (mod.runtimePresent === false) {
    return { labelKey: "notSynced", className: "bg-sky-500/10 text-sky-300" };
  }
  if (mod.runtimeEnabled === false) {
    return { labelKey: "notApplied", className: "bg-panel-gold/15 text-panel-gold" };
  }
  if (mod.runtimeEnabled === undefined) {
    return { labelKey: "pendingRestart", className: "bg-slate-800 text-slate-300" };
  }
  return { labelKey: "enabled", className: "bg-panel-green/15 text-panel-green" };
}

function LogViewport({
  className,
  emptyMessage,
  logs,
  logStatus,
  viewportRef
}: {
  className?: string;
  emptyMessage?: string;
  logs: string[];
  logStatus: "idle" | "connecting" | "connected" | "error" | "paused";
  viewportRef: React.RefObject<HTMLDivElement | null>;
}) {
  const { t } = useI18n();
  return (
    <div ref={viewportRef} className={cn("h-[420px] overflow-auto rounded-md bg-slate-950 p-4 font-mono text-xs leading-6 text-slate-300", className)}>
      {logs.length === 0 ? (
        <p className="text-slate-500">{emptyMessage ?? (logStatus === "error" ? t("logsUnavailable") : logStatus === "idle" ? t("logsNoHistory") : logStatus === "paused" ? t("logsPaused") : t("logsWaiting"))}</p>
      ) : logs.map((line, index) => (
        <p key={`${index}-${line}`} className={line.startsWith(">") ? "text-slate-100" : undefined}>
          {line.startsWith(">") ? (
            <>
              <span className="mr-2 text-panel-green">$</span>
              {line.slice(2)}
            </>
          ) : (
            <>
              <span className={line.includes("[Warn]") || line.toLowerCase().includes("error") ? "text-panel-gold" : "text-panel-green"}>
                {line.slice(0, 18)}
              </span>
              {line.slice(18)}
            </>
          )}
        </p>
      ))}
    </div>
  );
}

function InstallerSourceTab({ active, count, label, onClick }: { active: boolean; count: number; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      className={cn(
        "inline-flex items-center gap-2 rounded-md border px-3 py-2 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-panel-green/50",
        active ? "border-panel-green/50 bg-panel-green/15 text-panel-green" : "border-panel-line bg-slate-950/45 text-slate-300 hover:bg-slate-900"
      )}
      onClick={onClick}
    >
      {label}
      <span className={cn("rounded px-1.5 py-0.5 text-xs", active ? "bg-panel-green/15 text-panel-green" : "bg-slate-800 text-slate-400")}>{count}</span>
    </button>
  );
}

function InstallerEmptyState({ message }: { message: string }) {
  return (
    <div className="flex min-h-36 flex-col items-center justify-center px-4 py-8 text-center">
      <span className="flex size-10 items-center justify-center rounded-md border border-panel-line bg-slate-950/60 text-slate-400">
        <Package aria-hidden="true" className="size-5" />
      </span>
      <p className="mt-3 max-w-md text-sm text-slate-500">{message}</p>
    </div>
  );
}

function ModInstallOptionRow({
  blockedReason,
  disabled,
  installed,
  meta,
  selected,
  title,
  onToggle
}: {
  blockedReason?: string;
  disabled: boolean;
  installed: boolean;
  meta: string;
  selected: boolean;
  title: string;
  onToggle: () => void;
}) {
  const { t } = useI18n();
  return (
    <button
      className={cn(
        "flex w-full items-start gap-3 px-4 py-3 text-left transition focus:outline-none focus:ring-2 focus:ring-inset focus:ring-panel-green/50",
        selected ? "bg-panel-green/10" : installed ? "bg-slate-900/45" : "bg-transparent hover:bg-slate-900/40",
        disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer"
      )}
      disabled={disabled}
      onClick={onToggle}
      title={blockedReason}
      type="button"
    >
      <span className={cn(
        "mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-md border",
        installed || selected ? "border-panel-green/40 bg-panel-green/15 text-panel-green" : "border-panel-line bg-slate-950/70 text-slate-500"
      )}>
        {installed || selected ? <Check aria-hidden="true" className="size-4" /> : <Package aria-hidden="true" className="size-4" />}
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate text-sm font-semibold text-white">{title}</span>
          {installed ? (
            <span className="rounded bg-panel-green/15 px-2 py-0.5 text-xs font-medium text-panel-green">{t("alreadyInstalled")}</span>
          ) : selected ? (
            <span className="rounded bg-panel-green/15 px-2 py-0.5 text-xs font-medium text-panel-green">{t("selected")}</span>
          ) : null}
        </span>
        <span className="mt-1 block text-xs leading-5 text-slate-500">{meta}</span>
      </span>
    </button>
  );
}

function ResourceRow({ actions, className, meta, title }: { title: ReactNode; meta: string; actions?: ReactNode; className?: string }) {
  return (
    <div className={cn("flex flex-col gap-3 rounded-md border border-panel-line bg-slate-950/50 px-3 py-3 sm:flex-row sm:items-center sm:justify-between", className)}>
      <div className="min-w-0">
        <div className="truncate text-sm font-medium">{title}</div>
        <p className="mt-1 text-xs text-slate-500">{meta}</p>
      </div>
      {actions && <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>}
    </div>
  );
}

function sortBackupsNewestFirst(a: Backup, b: Backup) {
  return new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime();
}

function formatBackupDate(value: string, locale: string) {
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  }).format(new Date(value));
}

function ActionButton({
  disabled,
  icon,
  label,
  onClick
}: {
  disabled?: boolean;
  icon: ReactNode;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      className="inline-flex items-center justify-center gap-2 rounded-md border border-panel-line bg-slate-900/70 px-3 py-2 text-sm font-medium text-slate-100 transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-50"
      disabled={disabled}
      type="button"
      onClick={onClick}
    >
      {icon}
      {label}
    </button>
  );
}

function SummaryButton({ icon, label, onClick, value }: { icon: ReactNode; label: string; onClick: () => void; value: string }) {
  return (
    <button
      type="button"
      className="rounded-md border border-panel-line bg-slate-950/50 p-4 text-left transition hover:border-panel-green/50 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
      onClick={onClick}
    >
      <div className="flex items-center justify-between gap-3">
        <span className="text-slate-400">{label}</span>
        <span className="text-panel-green">{icon}</span>
      </div>
      <p className="mt-3 text-2xl font-semibold">{value}</p>
    </button>
  );
}

function MobileServerControls({
  copied,
  invite,
  joinAddress,
  joinPort,
  onCopy,
  server
}: {
  copied: string;
  invite: string;
  joinAddress: string;
  joinPort: number;
  onCopy: (label: string, value: string) => void;
  server: GameServerResource;
}) {
  const { t } = useI18n();
  const joinValue = `${joinAddress}:${joinPort}`;
  return (
    <Card className="mt-4 p-3 md:hidden">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs text-slate-500">{t("joinServer")}</p>
          <p className="mt-1 truncate text-sm font-medium text-slate-100">{joinValue}</p>
        </div>
        <Button className="h-10 shrink-0 px-3" variant="secondary" onClick={() => onCopy("Invite", invite)}>
          <Copy aria-hidden="true" />
          {copied === "Invite" ? t("copied") : t("actionCopyInvite")}
        </Button>
      </div>
      <ServerActions className="mt-3" compact server={server} showDelete={false} showInvite={false} />
    </Card>
  );
}

function CopyRow({
  copied,
  copiedLabel,
  copyLabel,
  label,
  onCopy,
  value
}: {
  copied: string;
  copiedLabel: string;
  copyLabel: string;
  label: string;
  onCopy: (label: string, value: string) => void;
  value: string;
}) {
  return (
    <div className="mt-4 flex items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/50 px-3 py-2">
      <div className="min-w-0">
        <p className="text-xs text-slate-500">{label}</p>
        <p className="truncate text-sm">{value}</p>
      </div>
      <Button variant="secondary" onClick={() => onCopy(label, value)}>
        {copied === label ? copiedLabel : copyLabel}
      </Button>
    </div>
  );
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/50 px-3 py-2">
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 break-words text-sm text-slate-200">{value}</p>
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

function PlayerCountBadge({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex h-6 items-center gap-1.5 rounded-md border border-panel-line bg-slate-950/50 px-2 text-xs font-medium text-slate-300">
      <Users aria-hidden="true" className="size-3.5 text-slate-500" />
      <span className="text-slate-500">{label}</span>
      <span className="font-semibold text-slate-100">{value}</span>
    </span>
  );
}

function difficultyLabel(value: string, t: ReturnType<typeof useI18n>["t"]) {
  const labels: Record<string, string> = {
    journey: t("tagJourney"),
    classic: t("tagClassic"),
    expert: t("tagExpert"),
    master: t("tagMaster")
  };
  return labels[value] ?? value;
}
