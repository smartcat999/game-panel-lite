"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Ban, Braces, Check, CheckCircle2, Clock, Copy, Cpu, ExternalLink, Eye, EyeOff, FileText, KeyRound, Megaphone, MemoryStick, Moon, MoreHorizontal, Package, Pencil, Plug, Power, RotateCcw, Save, Send, Share2, Sun, Sunrise, Terminal, Trash2, Upload, UserX, Users, Waves, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from "react";
import type { TerrariaConfig } from "@gamepanel-lite/shared";
import { secretSeedKeyFor, terrariaInternalPort, terrariaSecretSeeds, terrariaSeedModeCodes } from "@gamepanel-lite/shared";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { GameUpdateCard } from "@/components/game-update-card";
import { WorldRegenerationAction } from "@/components/world-regeneration-card";
import { PlayersPanel } from "@/components/players-panel";
import { ProviderConfigEditor } from "@/components/provider-config-editor";
import { ResourceLimitSlider, formatCpuResourceLimit, formatMemoryResourceLimit } from "@/components/resource-limit-slider";
import { ServerLobbyBanner } from "@/components/server-lobby-banner";
import { ServerTimeMachine } from "@/components/server-time-machine";
import { ServerGameRules } from "@/components/server-game-rules";
import { WorldMigrationHub } from "@/components/world-migration-hub";
import { WorldRadarGrid } from "@/components/world-radar-grid";
import { Button, Card, Input, ToastNotice } from "@/components/ui";
import { ActivityLatestOperation } from "@/features/monitoring/components";
import { getServerMonitoringEvents } from "@/features/monitoring/api";
import type { MonitoringEvent } from "@/features/monitoring/types";
import {
  assignMod,
  createBackup,
  createWorldSnapshot,
  deleteBackup,
  disableServerShare,
  deleteMod,
  deleteWorld,
  enableServerShare,
  getDockerStatus,
  getModConfig,
  getGameServer,
  getRuntimeStats,
  getServerJoinInfo,
  getSettings,
  listComputeNodes,
  listGames,
  getServerGatewayStatus,
  getServerLogSnapshot,
  getServerShare,
  getServerStats,
  listGlobalMods,
  listModPacks,
  listModConfigs,
  listMods,
  listWorlds,
  previewTerrariaConfig,
  restoreBackup,
  sendServerCommand,
  gameServerAction,
  setModEnabled,
  saveModConfig,
  serverLogsUrl,
  serverWatchUrl,
  updateGameServerConfig,
  uploadModConfig,
  deleteModConfig,
  uploadMod,
  type ServerConfigUpdatePayload,
  type ServerWatchSnapshot,
} from "@/lib/api";
import { copyText } from "@/lib/clipboard";
import { consoleReadyMessageKey, supportsTerrariaConsoleShortcuts } from "@/lib/console-commands";
import { isWorldOrBackupEventType, showWorldAndBackupFeatures } from "@/lib/feature-flags";
import { gameServerConfigPendingRestart, gameServerJoinPort, gameServerMode, gameServerStatus, gameServerVersion, terrariaConfigFromGameServer } from "@/lib/game-server-resource";
import { localizeRelativeTime, useI18n, type MessageKey } from "@/lib/i18n";
import { dstModScope, isServerAssignableMod, modDisplayName, modRuntimeState, type ModRuntimeState } from "@/lib/mod-display";
import { createDefaultProviderConfigPayload, isCuratedGameRuleField, isWorldGenerationProviderConfigField, providerConfigFieldChanged, restoreProviderConfigDefaults, updateProviderConfigPath, updateProviderConfigPayload, type ProviderConfigPayload } from "@/lib/provider-config";
import { describeResourceAction, formatServerDetailError, isServerLifecyclePending, shouldRenderServerDetailTabs } from "@/lib/server-detail-actions";
import { serverInviteText, serverJoinAddress, serverJoinPassword } from "@/lib/server-join";
import { cn } from "@/lib/utils";
import { usePermissions } from "@/lib/permissions";
import type { Backup, GameServerResource, ModConfigFile, ModFile, ModPack, ProviderCapabilities, ProviderCatalog, ProviderConfigField, ResourceLimits, ServerStatus, World } from "@/lib/types";

type TabId = "overview" | "console" | "logs" | "players" | "version" | "config" | "worlds" | "backups" | "mods";
type ModInstallSource = "library" | "packs";
type ModPanelSection = "installed" | "configs";

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
  versions: true,
  worldRegeneration: false
};

function formatCpuLimitLabel(value: number, t: (key: "unlimited" | "cpuCoresValue", values?: Record<string, string | number>) => string) {
  return value > 0 ? t("cpuCoresValue", { cores: value }) : t("unlimited");
}

function formatMemoryLimitLabel(value: number, t: (key: "unlimited" | "memoryGbValue", values?: Record<string, string | number>) => string) {
  return value > 0 ? t("memoryGbValue", { gb: value / 1024 }) : t("unlimited");
}

export default function ServerDetailPage() {
  const { locale, t } = useI18n();
  const { canControlServer, canManageShares, isViewer } = usePermissions();
  const params = useParams<{ id: string }>();
  const id = params.id;
  const client = useQueryClient();
  const modUploadInputRef = useRef<HTMLInputElement>(null);
  const logViewportRef = useRef<HTMLDivElement>(null);
  const logServerIdRef = useRef("");
  const logReplayIndexRef = useRef(0);
  const [activeTab, setActiveTab] = useState<TabId>("overview");
  const [gameUpdateActive, setGameUpdateActive] = useState(false);
  const [gameUpdateActivity, setGameUpdateActivity] = useState<"checking" | "updating" | null>(null);
  const [worldRegenerationActive, setWorldRegenerationActive] = useState(false);
  const [worldRegenerationDialogOpen, setWorldRegenerationDialogOpen] = useState(false);
  const handleGameUpdateActiveChange = useCallback((active: boolean, updateStatus?: string) => {
    setGameUpdateActive(active);
    setGameUpdateActivity(active ? updateStatus === "checking" ? "checking" : "updating" : null);
  }, []);

  const query = useQuery({ queryKey: ["game-server", id], queryFn: () => getGameServer(id), retry: false });
  const serverResource = query.data;
  const resourceStatus = serverResource ? gameServerStatus(serverResource) : undefined;
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, enabled: Boolean(serverResource), staleTime: 5 * 60 * 1000, retry: false });
  const providerCatalog = useMemo(
    () => gamesQuery.data?.flatMap((game) => game.providers).find((provider) => provider.key === serverResource?.providerKey),
    [gamesQuery.data, serverResource?.providerKey]
  );
  const renderTabs = shouldRenderServerDetailTabs({
    providerResolved: Boolean(providerCatalog),
    providerQueryPending: gamesQuery.isPending
  });
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
    enabled: resourceStatus === "running",
    refetchInterval: resourceStatus === "running" ? 5000 : false,
    retry: false
  });
  const worldsQuery = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, enabled: Boolean(!isViewer && serverResource && visibleCapabilities.saveSnapshots), retry: false });
  const modsQuery = useQuery({
    queryKey: ["mods", id],
    queryFn: () => listMods(id),
    enabled: Boolean(!isViewer && serverResource && capabilities.mods),
    retry: false
  });
  const globalModsQuery = useQuery({
    queryKey: ["global-mods"],
    queryFn: listGlobalMods,
    enabled: Boolean(!isViewer && serverResource && capabilities.mods),
    retry: false
  });
  const modPacksQuery = useQuery({
    queryKey: ["mod-packs"],
    queryFn: listModPacks,
    enabled: Boolean(!isViewer && serverResource && capabilities.mods),
    retry: false
  });
  const dockerStatusQuery = useQuery({ queryKey: ["docker-status"], queryFn: getDockerStatus, enabled: Boolean(serverResource && capabilities.mods), retry: false, staleTime: 5 * 60 * 1000 });
  const shareQuery = useQuery({ queryKey: ["server-share", id], queryFn: () => getServerShare(id), enabled: Boolean(canManageShares && serverResource), retry: false });
  const runtimeStatsQuery = useQuery({ queryKey: ["runtime-stats"], queryFn: getRuntimeStats, enabled: Boolean(serverResource), retry: false, staleTime: 30_000 });
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: getSettings, staleTime: 5 * 60 * 1000, retry: false });
  const joinInfoQuery = useQuery({
    queryKey: ["server-join-info", id],
    queryFn: () => getServerJoinInfo(id),
    enabled: Boolean(serverResource),
    retry: false
  });
  const [copied, setCopied] = useState("");
  const [logs, setLogs] = useState<string[]>([]);
  const [command, setCommand] = useState("");
  const [shareIncludePassword, setShareIncludePassword] = useState(false);
  const [shareDialogOpen, setShareDialogOpen] = useState(false);
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
  const serverEventsQuery = useQuery({
    queryKey: ["server-monitoring-events", id],
    queryFn: () => getServerMonitoringEvents(id, 50),
    enabled: Boolean(serverResource),
    retry: false,
    staleTime: 5000
  });
  const [logStatus, setLogStatus] = useState<"idle" | "connecting" | "connected" | "error" | "paused">("idle");
  const [logStreamPaused, setLogStreamPaused] = useState(false);
  const [configSaved, setConfigSaved] = useState(false);
  const [modsPendingRestart, setModsPendingRestart] = useState(false);
  const noticeTimerRef = useRef<number | null>(null);
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
    if (noticeTimerRef.current) window.clearTimeout(noticeTimerRef.current);
    setErrorMessage("");
    setSuccessMessage(message);
    noticeTimerRef.current = window.setTimeout(() => setSuccessMessage(""), 3000);
  };

  const showError = (message: string) => {
    if (noticeTimerRef.current) window.clearTimeout(noticeTimerRef.current);
    setSuccessMessage("");
    setErrorMessage(message);
    noticeTimerRef.current = window.setTimeout(() => setErrorMessage(""), 6000);
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
      if (noticeTimerRef.current) window.clearTimeout(noticeTimerRef.current);
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
  const serverAction = useMutation({
    mutationFn: (action: "start" | "stop" | "restart") => gameServerAction(id, action),
    onSuccess: async (updatedServer, action) => {
      const message = action === "start"
        ? t("serverStartQueued")
        : action === "stop"
        ? t("serverStopQueued")
        : t("serverRestartQueued");
      showSuccess(message);
      setServerResourceCache(updatedServer);
      await client.invalidateQueries({ queryKey: ["game-server", id] });
      await client.invalidateQueries({ queryKey: ["game-servers"] });
    },
    onError: (error) => showError(formatActionError(error, t("actionWorking")))
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
      setShareDialogOpen(false);
      showSuccess(t("sharePageEnabled"));
      await client.invalidateQueries({ queryKey: ["server-share", id] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("sharePageUnavailable"))
  });
  const shareDisable = useMutation({
    mutationFn: () => disableServerShare(id),
    onSuccess: async () => {
      setShareDialogOpen(false);
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
  const modUpload = useMutation({
    mutationFn: async (files: File[]) => {
      for (const file of files) {
        await uploadMod(id, file);
      }
      return files.length;
    },
    onSuccess: async (count) => {
      showSuccess(t("modsUploadedSummary", { count }));
      markModsChanged();
      if (modUploadInputRef.current) modUploadInputRef.current.value = "";
      await client.invalidateQueries({ queryKey: ["mods", id] });
      await client.invalidateQueries({ queryKey: ["game-server", id] });
    },
    onError: (error) => showError(formatActionError(error, t("unableUploadMod")))
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
  const serverMods = useMemo(() => modsQuery.data ?? [], [modsQuery.data]);
  const globalMods = useMemo(() => globalModsQuery.data ?? [], [globalModsQuery.data]);
  const providerGlobalMods = useMemo(
    () => globalMods.filter((mod) => !serverResource || mod.providerKey === serverResource.providerKey),
    [globalMods, serverResource]
  );
  const modPacks = useMemo(() => modPacksQuery.data ?? [], [modPacksQuery.data]);
  const providerModPacks = useMemo(
    () => modPacks.filter((pack) => !serverResource || pack.providerKey === serverResource.providerKey),
    [modPacks, serverResource]
  );
  const modUploadAccept = serverResource?.providerKey === "palworld" ? ".pak" : serverResource?.providerKey === "terraria-tmodloader" ? ".tmod" : "";
  const supportsDirectModUpload = Boolean(modUploadAccept);
  const workshopUnsupported = isArmArchitecture(dockerStatusQuery.data?.architecture);
  const isZh = locale.startsWith("zh");
  const tabs: { id: TabId; label: string }[] = useMemo(() => {
    const overviewTab = { id: "overview" as const, label: isZh ? "🎮 服务器大厅" : t("tabOverview") };
    if (isViewer) return [overviewTab];
    return [
    overviewTab,
    ...(visibleCapabilities.backups ? [{ id: "backups" as const, label: isZh ? "💾 备份与回档" : t("tabBackups") }] : []),
    { id: "config", label: isZh ? "⚙️ 游戏配置" : t("tabConfig") },
    ...(capabilities.mods ? [{ id: "mods" as const, label: isZh ? "📦 模组管理" : t("tabMods") }] : []),
    ...(visibleCapabilities.saveSnapshots ? [{ id: "worlds" as const, label: isZh ? "🌍 世界地图" : t("tabWorlds") }] : []),
    ...(capabilities.consoleCommands ? [{ id: "console" as const, label: isZh ? "📟 控制台与日志" : t("tabConsole") }] : []),
    ...(!capabilities.consoleCommands ? [{ id: "logs" as const, label: isZh ? "📟 运行日志" : t("tabLogs") }] : []),
    ...(capabilities.playerList ? [{ id: "players" as const, label: isZh ? "👥 在线玩家" : t("tabPlayers") }] : []),
    ...(serverResource?.providerKey === "palworld" ? [{ id: "version" as const, label: t("tabVersion") }] : [])
    ];
  }, [capabilities.consoleCommands, capabilities.mods, capabilities.playerList, isViewer, isZh, serverResource?.providerKey, visibleCapabilities.backups, visibleCapabilities.saveSnapshots, t]);
  useEffect(() => {
    if (serverResource && !tabs.some((tab) => tab.id === activeTab)) {
      setActiveTab("overview");
    }
  }, [activeTab, serverResource, tabs]);
  useEffect(() => {
    if (serverResource?.providerKey === "palworld" && new URLSearchParams(window.location.search).get("tab") === "version") {
      setActiveTab("version");
    }
  }, [serverResource?.providerKey]);
  useEffect(() => {
    if (shareQuery.data) {
      setShareIncludePassword(shareQuery.data.includePassword);
    }
  }, [shareQuery.data]);
  const visibleServerEvents = useMemo(
    () => (serverEventsQuery.data?.events ?? []).filter((event) => showWorldAndBackupFeatures || !isWorldOrBackupEventType(event.type)),
    [serverEventsQuery.data?.events]
  );
  if (!serverResource) {
    return (
      <>
        <Link href="/servers" className="text-sm text-slate-400 hover:text-panel-green">{t("backToServers")}</Link>
        <Card className="mt-4 p-6 text-sm text-slate-400">{query.isLoading ? t("loading") : t("serverNotFound")}</Card>
      </>
    );
  }

  const status = gameServerStatus(serverResource);
  const playersOnline = serverResource.status.playersOnline ?? 0;
  const joinPort = joinInfoQuery.data?.port ?? gameServerJoinPort(serverResource);
  const invite = joinInfoQuery.data?.inviteText ?? serverInviteText(serverResource);
  const joinAddress = joinInfoQuery.data?.address ?? serverJoinAddress(serverResource);
  const joinPassword = joinInfoQuery.data?.password ?? serverJoinPassword(serverResource);
  const share = shareQuery.data;
  const savedShareIncludePassword = share?.includePassword ?? false;
  const sharePath = share?.sharePath ?? "";
  const shareUrl = sharePath ? `${typeof window === "undefined" ? "" : window.location.origin}${sharePath}` : "";
  const logStatusLabel = logStatus === "connected" ? t("logsConnected") : logStatus === "error" ? t("logsDisconnected") : logStatus === "paused" ? t("logsPaused") : logStatus === "idle" ? t("logsIdle") : t("logsConnecting");
  const runtimeErrorMessage = status === "errored" && serverResource.status.lastError ? formatActionError(new Error(serverResource.status.lastError), serverResource.status.lastError) : "";
  const modLifecycleMessage = activeTab === "mods" && capabilities.mods && isServerLifecyclePending(status)
    ? t("modChangesLifecycleBusy")
    : "";
  const noticeMessage = errorMessage || successMessage || modLifecycleMessage;
  const noticeTone = errorMessage ? "error" : successMessage ? "success" : "warning";
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
  const openShareDialog = () => {
    setShareIncludePassword(savedShareIncludePassword);
    setShareDialogOpen(true);
  };
  const closeShareDialog = () => {
    setShareIncludePassword(savedShareIncludePassword);
    setShareDialogOpen(false);
  };
  const submitCommand = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    runCommand(command);
  };

  return (
    <>
      {supportsDirectModUpload ? (
        <input
          ref={modUploadInputRef}
          className="hidden"
          type="file"
          accept={modUploadAccept}
          multiple
          onChange={(event) => {
            const files = Array.from(event.target.files ?? []);
            if (files.length > 0) modUpload.mutate(files);
          }}
        />
      ) : null}
      <Link href="/servers" className="text-sm text-slate-400 hover:text-panel-green">{t("backToServers")}</Link>
      {query.isError && <p className="mt-3 text-sm text-panel-gold">{t("apiDetailUnavailable")}</p>}
      {noticeMessage && (
        <div className="pointer-events-none fixed inset-x-4 bottom-4 z-[60] flex justify-end md:inset-x-auto md:bottom-auto md:right-6 md:top-24">
          <ToastNotice
            closeLabel={t("cancel")}
            message={noticeMessage}
            tone={noticeTone}
            onClose={errorMessage || successMessage ? () => {
              if (noticeTimerRef.current) window.clearTimeout(noticeTimerRef.current);
              setErrorMessage("");
              setSuccessMessage("");
            } : undefined}
          />
        </div>
      )}
      {/* Top Game Room Lobby Banner */}
      <div className="mt-3">
        <ServerLobbyBanner
          server={serverResource}
          publicHost={settingsQuery.data?.publicHost}
          canControl={canControlServer}
          disabled={gameUpdateActive || worldRegenerationActive}
          onAction={(action) => serverAction.mutate(action)}
          onOpenShare={canManageShares ? openShareDialog : undefined}
        />
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_280px]">
        <div className="min-w-0">
          {renderTabs ? (
            <div className="mb-4 flex gap-2 overflow-x-auto rounded-lg border border-panel-line bg-panel-card px-3 py-3" role="tablist" aria-label={serverResource.name}>
              {tabs.map((tab) => (
                <button
                  key={tab.id}
                  id={`server-detail-tab-${tab.id}`}
                  type="button"
                  role="tab"
                  aria-controls="server-detail-tabpanel"
                  aria-selected={activeTab === tab.id}
                  tabIndex={activeTab === tab.id ? 0 : -1}
                  className={cn(
                    "relative shrink-0 rounded-md border border-transparent px-3 py-2 text-sm font-medium text-slate-400 transition hover:bg-slate-800/80 hover:text-white focus:outline-none focus:ring-2 focus:ring-inset focus:ring-panel-green/50",
                    activeTab === tab.id && "border-panel-green/40 bg-panel-green/15 text-white shadow-[inset_0_0_0_1px_rgba(123,217,120,0.18)]"
                  )}
                  onClick={() => setActiveTab(tab.id)}
                  onKeyDown={(event) => {
                    const currentIndex = tabs.findIndex((item) => item.id === tab.id);
                    const nextIndex = event.key === "Home"
                      ? 0
                      : event.key === "End"
                        ? tabs.length - 1
                        : event.key === "ArrowRight"
                          ? (currentIndex + 1) % tabs.length
                          : event.key === "ArrowLeft"
                            ? (currentIndex - 1 + tabs.length) % tabs.length
                            : -1;
                    if (nextIndex < 0) return;
                    event.preventDefault();
                    const nextTab = tabs[nextIndex];
                    if (!nextTab) return;
                    setActiveTab(nextTab.id);
                    window.requestAnimationFrame(() => document.getElementById(`server-detail-tab-${nextTab.id}`)?.focus());
                  }}
                >
                  <span>{tab.label}</span>
                </button>
              ))}
            </div>
          ) : (
            <div className="mb-4 flex h-[66px] items-center gap-3 rounded-lg border border-panel-line bg-panel-card px-4" aria-busy="true" aria-label={t("loading")}>
              <span className="h-9 w-32 animate-pulse rounded-md bg-slate-800" aria-hidden="true" />
              <span className="h-9 w-28 animate-pulse rounded-md bg-slate-800" aria-hidden="true" />
              <span className="h-9 w-28 animate-pulse rounded-md bg-slate-800" aria-hidden="true" />
            </div>
          )}

          {gameUpdateActive && activeTab !== "version" ? (
            <button
              className="mb-4 flex w-full items-center justify-between gap-3 rounded-lg border border-panel-green/30 bg-panel-green/10 px-4 py-3 text-left text-sm text-panel-green transition hover:bg-panel-green/15 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
              type="button"
              onClick={() => setActiveTab("version")}
            >
              <span className="flex min-w-0 items-center gap-2 font-medium">
                <RotateCcw aria-hidden="true" className="size-4 shrink-0 animate-spin motion-reduce:animate-none" />
                <span className="truncate">{gameUpdateActivity === "checking" ? t("gameUpdateStatusChecking") : t("gameUpdateStatusUpdating")}</span>
              </span>
              <span className="shrink-0 text-xs">{t("gameUpdateView")}</span>
            </button>
          ) : null}

          {worldRegenerationActive ? (
            <button
              className="mb-4 flex w-full items-center justify-between gap-3 rounded-lg border border-panel-green/30 bg-panel-green/10 px-4 py-3 text-left text-sm text-panel-green transition hover:bg-panel-green/15 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
              type="button"
              onClick={() => setActiveTab("logs")}
            >
              <span className="flex min-w-0 items-center gap-2 font-medium">
                <RotateCcw aria-hidden="true" className="size-4 shrink-0 animate-spin motion-reduce:animate-none" />
                <span className="truncate">{t("worldRegenerationRunning")}</span>
              </span>
              <span className="shrink-0 text-xs">{t("tabLogs")}</span>
            </button>
          ) : null}

          <div
            id="server-detail-tabpanel"
            role="tabpanel"
            aria-labelledby={`server-detail-tab-${activeTab}`}
            tabIndex={0}
          >
          {activeTab === "overview" && (
            <OverviewTab
              resource={serverResource}
              events={visibleServerEvents}
              eventsLoading={serverEventsQuery.isLoading}
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
          {activeTab === "players" && capabilities.playerList && (
            <PlayersPanel serverId={serverResource.id} />
          )}
          {!isViewer && serverResource.providerKey === "palworld" ? (
            <div className={activeTab === "version" ? "" : "hidden"}>
              <GameUpdateCard
                playersOnline={playersOnline}
                runtimeVersion={gameServerVersion(serverResource)}
                serverId={serverResource.id}
                serverStatus={status}
                onActiveChange={handleGameUpdateActiveChange}
              />
            </div>
          ) : null}
          {activeTab === "config" && (
            <div className="space-y-6">
              <ConfigTab
                provider={providerCatalog}
                resource={serverResource}
                saveError={configSave.error instanceof Error ? configSave.error.message : ""}
                savePending={configSave.isPending}
                saveSuccess={configSaved}
                restartPending={configRestart.isPending}
                onRegenerateWorld={capabilities.worldRegeneration ? () => setWorldRegenerationDialogOpen(true) : undefined}
                onRestart={() => setPendingConfigRestart(true)}
                onSave={(nextConfig, hostPort) => configSave.mutate({ config: nextConfig, hostPort })}
              />
              <ResourceLimitsCard
                cpuPercent={statsQuery.data?.cpuPercent}
                memoryMb={statsQuery.data?.memoryMb}
                resource={serverResource}
                restartPending={configRestart.isPending}
                onEdit={() => setResourceDialogOpen(true)}
                onRestart={() => setPendingConfigRestart(true)}
              />
            </div>
          )}
          {activeTab === "worlds" && visibleCapabilities.saveSnapshots ? (
            <div className="space-y-6">
              <WorldMigrationHub
                targetServerId={serverResource.id}
                onImportSuccess={() => worldsQuery.refetch()}
              />
              <WorldRadarGrid
                worlds={serverWorlds}
                currentServer={serverResource}
              />
            </div>
          ) : null}
          {activeTab === "backups" && visibleCapabilities.backups && (
            <div className="space-y-6">
              <ServerTimeMachine server={serverResource} />
            </div>
          )}
          {activeTab === "mods" && capabilities.mods && (
            <ModsTab
              serverId={serverResource.id}
              supportsModConfigs={serverResource.providerKey === "terraria-tmodloader"}
              availableMods={providerGlobalMods}
              assigning={modAssign.isPending}
              deleting={modDelete.isPending}
              isError={modsQuery.isError}
              isLoading={modsQuery.isLoading}
              items={serverMods}
              libraryError={globalModsQuery.isError || modPacksQuery.isError}
              modPacks={providerModPacks}
              pendingRestart={modsPendingRestart}
              packInstalling={modPackAssign.isPending}
              serverStatus={status}
              uploadAccept={modUploadAccept}
              uploading={modUpload.isPending}
              toggling={modEnabled.isPending}
              workshopUnsupported={workshopUnsupported}
              onAssignMods={(mods) => modAssign.mutate(mods.map((mod) => mod.id))}
              onDelete={setPendingModDelete}
              onInstallPack={setPendingModPackInstall}
              onUpload={supportsDirectModUpload ? () => modUploadInputRef.current?.click() : undefined}
              onToggle={(mod) => setPendingModToggle({ mod, enabled: !mod.enabled })}
            />
          )}
          </div>
        </div>

        <aside className="hidden space-y-4 xl:sticky xl:top-24 xl:block xl:self-start" data-testid="server-detail-side-panel">
          <JoinServerPanel
            copied={copied}
            invite={invite}
            joinAddress={joinAddress}
            joinPassword={joinPassword}
            joinPort={joinPort}
            onCopy={copy}
          />
          <ShareServerPanel
            enabled={Boolean(share?.enabled)}
            onOpen={openShareDialog}
          />
          <RuntimeMonitorCard
            cpuPercent={statsQuery.data?.cpuPercent}
            memoryMb={statsQuery.data?.memoryMb}
            resource={serverResource}
          />
        </aside>
      </div>

      {canManageShares ? <ShareServerDialog
        copied={copied}
        open={shareDialogOpen}
        shareDisabling={shareDisable.isPending}
        shareEnabled={Boolean(share?.enabled)}
        shareIncludePassword={shareIncludePassword}
        savedIncludePassword={savedShareIncludePassword}
        shareLoading={shareQuery.isLoading}
        sharePath={sharePath}
        shareSaving={shareEnable.isPending}
        shareUrl={shareUrl}
        onCancel={closeShareDialog}
        onCopy={copy}
        onDisableShare={() => shareDisable.mutate()}
        onEnableShare={() => shareEnable.mutate()}
        onShareIncludePasswordChange={setShareIncludePassword}
      /> : null}

      {!isViewer && capabilities.worldRegeneration ? (
        <WorldRegenerationAction
          open={worldRegenerationDialogOpen}
          playersOnline={playersOnline}
          serverId={serverResource.id}
          serverStatus={status}
          onActiveChange={setWorldRegenerationActive}
          onOpenChange={setWorldRegenerationDialogOpen}
        />
      ) : null}

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
        hostCpuCores={runtimeStatsQuery.data?.cpuCores}
        hostMemoryMb={runtimeStatsQuery.data?.memoryLimitMb}
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
  events,
  eventsLoading,
  resource,
  runtimeError
}: {
  events: MonitoringEvent[];
  eventsLoading: boolean;
  resource: GameServerResource;
  runtimeError: string;
}) {
  const { canEditSettings, canManageNodes } = usePermissions();
  const nodesQuery = useQuery({ queryKey: ["compute-nodes"], queryFn: listComputeNodes, retry: false, staleTime: 60000 });
  const nodeInfo = nodesQuery.data?.find((n) => n.id === resource.nodeId);
  const isWorker = Boolean(resource.nodeId && resource.nodeId !== "node-local");
  const nodeDisplayName = nodeInfo
    ? `${nodeInfo.name}${nodeInfo.region ? ` (${nodeInfo.region})` : ""}`
    : (resource.nodeId || "主控本机 (Local Daemon)");

  const hostPort = resource.spec.network?.hostPort ?? 0;
  const internalPort = resource.spec.network?.port ?? 0;
  return (
    <div className="space-y-6">
      {/* Compute Node Topology Card */}
      <div className="rounded-xl border border-slate-800 bg-slate-950/40 p-4 sm:p-5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="flex size-7 items-center justify-center rounded-lg border border-slate-800 bg-slate-900 text-panel-green">
              <Plug className="size-3.5" />
            </span>
            <div>
              <h2 className="text-xs font-bold uppercase tracking-wider text-slate-200">部署计算节点与拓扑</h2>
              <p className="text-[11px] text-slate-500">实例容器所在的物理/云端主机及连接路由模式</p>
            </div>
          </div>
          {canManageNodes && canEditSettings ? (
            <Link href="/settings" className="text-xs text-panel-green hover:underline flex items-center gap-1">
              管理集群节点 <ExternalLink className="size-3" />
            </Link>
          ) : null}
        </div>

        <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div className="rounded-lg border border-slate-800/80 bg-slate-900/60 p-3">
            <span className="text-[11px] text-slate-500 block">部署计算节点</span>
            <div className="mt-1 flex items-center gap-1.5">
              <span className="size-2 rounded-full bg-emerald-400 animate-pulse" />
              <span className="text-sm font-semibold text-slate-200">
                {isWorker ? (
                  <span className="flex items-center gap-1.5">
                    <span className="text-sky-300 font-bold">{nodeDisplayName}</span>
                    <span className="text-[11px] text-slate-500 font-mono">({resource.nodeId?.slice(0, 13)})</span>
                  </span>
                ) : (
                  "主控本机 (Local Daemon)"
                )}
              </span>
            </div>
          </div>

          <div className="rounded-lg border border-slate-800/80 bg-slate-900/60 p-3">
            <span className="text-[11px] text-slate-500 block">流量连接路由</span>
            <div className="mt-1 flex items-center gap-1.5">
              <span className="text-sm font-semibold text-slate-200">
                {resource.nodeId && resource.nodeId !== "node-local" ? "Gateway 网关流代理中转" : "主控直连出口"}
              </span>
            </div>
          </div>

          <div className="rounded-lg border border-slate-800/80 bg-slate-900/60 p-3">
            <span className="text-[11px] text-slate-500 block">容器内部端口</span>
            <span className="mt-1 text-sm font-mono font-semibold text-slate-200 block">
              {internalPort || 7777} / {resource.spec.network?.protocol || "tcp"}
            </span>
          </div>

          <div className="rounded-lg border border-slate-800/80 bg-slate-900/60 p-3">
            <span className="text-[11px] text-slate-500 block">对外服务端口</span>
            <span className="mt-1 text-sm font-mono font-semibold text-panel-green block">
              {hostPort || internalPort || 7777}
            </span>
          </div>
        </div>
      </div>

      <ActivityLatestOperation events={events} loading={eventsLoading} runtimeError={runtimeError} />
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
      <div className="mt-4 grid gap-2 md:grid-cols-2">
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

function RuntimeMonitorCard({
  cpuPercent,
  memoryMb,
  resource
}: {
  cpuPercent?: number;
  memoryMb?: number;
  resource: GameServerResource;
}) {
  const { t } = useI18n();
  const running = gameServerStatus(resource) === "running";
  const cpuLimitCores = resource.spec.resources?.cpuLimitCores ?? 0;
  const memoryLimitMb = resource.spec.resources?.memoryLimitMb ?? 0;
  const memoryPercent = running && memoryMb !== undefined && memoryLimitMb > 0
    ? memoryMb / memoryLimitMb * 100
    : undefined;

  return (
    <Card className="p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="font-semibold">{t("runtimeOverview")}</h2>
          <p className="mt-1 text-xs text-slate-400">{t("runtimeOverviewHint")}</p>
        </div>
        <span className="flex size-8 shrink-0 items-center justify-center rounded-md border border-panel-green/30 bg-panel-green/10 text-panel-green">
          <Activity aria-hidden="true" className="size-4" />
        </span>
      </div>
      <div className="mt-4 space-y-2.5">
        <RuntimeMonitorMetric
          icon={<Cpu aria-hidden="true" className="size-4" />}
          label={t("cpu")}
          value={running && cpuPercent !== undefined ? `${cpuPercent.toFixed(1)}%` : t("notRunning")}
          hint={formatCpuLimitLabel(cpuLimitCores, t)}
          percent={running && cpuPercent !== undefined ? cpuPercent : undefined}
        />
        <RuntimeMonitorMetric
          icon={<MemoryStick aria-hidden="true" className="size-4" />}
          label={t("memory")}
          value={running && memoryMb !== undefined ? `${memoryMb} MB` : t("notRunning")}
          hint={formatMemoryLimitLabel(memoryLimitMb, t)}
          percent={memoryPercent}
          tone="neutral"
        />
      </div>
    </Card>
  );
}

function RuntimeMonitorMetric({
  hint,
  icon,
  label,
  percent,
  tone = "green",
  value
}: {
  hint: string;
  icon: ReactNode;
  label: string;
  percent?: number;
  tone?: "green" | "neutral";
  value: string;
}) {
  const accentClass = tone === "neutral" ? "text-slate-300" : "text-panel-green";
  const barClass = tone === "neutral" ? "bg-slate-400" : "bg-panel-green";
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/35 p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className={accentClass}>{icon}</span>
          <span className="text-xs text-slate-400">{label}</span>
        </div>
        <div className="min-w-0 text-right">
          <p className="truncate font-mono text-sm font-semibold text-slate-100">{value}</p>
          <p className="mt-0.5 truncate text-[11px] text-slate-400">{hint}</p>
        </div>
      </div>
      {percent !== undefined ? (
        <div className="mt-2.5 h-1.5 overflow-hidden rounded-full bg-slate-800">
          <div className={cn("h-full rounded-full", barClass)} style={{ width: `${Math.max(3, Math.min(percent, 100))}%` }} />
        </div>
      ) : null}
    </div>
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

  const gatewayQuery = useQuery({
    queryKey: ["server-gateway-status", server.id],
    queryFn: () => getServerGatewayStatus(server.id),
    refetchInterval: 5000,
    retry: false
  });
  const diag = gatewayQuery.data;

  const isRemoteNode = Boolean(
    (server.nodeId && server.nodeId !== "node-local") ||
    diag?.isRemote
  );
  const nodeName = diag?.nodeName || (server.nodeId && server.nodeId !== "node-local" ? "WH-node01" : "Local Host Daemon");
  const region = diag?.region || (server.nodeId && server.nodeId !== "node-local" ? "Wu Han" : "");
  const isListening = diag ? diag.gatewayListening : true;
  const isOnline = diag ? diag.nodeOnline : true;

  return (
    <div>
      {isRemoteNode && (
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-sky-500/30 bg-sky-950/40 px-3.5 py-2.5 text-xs text-slate-300 shadow-sm">
          <div className="flex flex-wrap items-center gap-3">
            <span className="flex items-center gap-1.5 font-medium text-sky-400">
              <span className="relative flex size-2">
                <span className="absolute inline-flex size-full animate-ping rounded-full bg-sky-400 opacity-75" />
                <span className="relative inline-flex size-2 rounded-full bg-sky-500" />
              </span>
              🛰️ 分布式集群代理与网关诊断
            </span>
            <span className="text-slate-600">|</span>
            <span className="flex items-center gap-1">
              部署节点: <strong className="text-slate-100">{nodeName} {region ? `(${region})` : ""}</strong>
              {isOnline ? (
                <span className="ml-1 inline-flex items-center gap-1 rounded bg-emerald-500/10 px-1.5 py-0.5 text-[10px] text-emerald-400 font-mono">
                  🟢 在线 {diag?.latencyMs ? `${diag.latencyMs}ms` : ""}
                </span>
              ) : (
                <span className="ml-1 inline-flex items-center gap-1 rounded bg-rose-500/10 px-1.5 py-0.5 text-[10px] text-rose-400">
                  🔴 离线
                </span>
              )}
            </span>
            <span className="text-slate-600">|</span>
            <span className="flex items-center gap-1">
              公网网关转发: <strong className="font-mono text-slate-100">{diag?.listenPort || server.spec.network?.hostPort || 7781}</strong>
              <span className={cn(
                "ml-1 inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-mono",
                isListening ? "bg-emerald-500/10 text-emerald-400" : "bg-amber-500/10 text-amber-400"
              )}>
                {isListening ? "🟢 监听中 (已接管)" : "🟡 准备中"}
              </span>
            </span>
          </div>
          <span className="text-[11px] text-slate-400">
            日志缓存: <strong className="font-mono text-slate-200">{diag?.cachedLogLines ?? logs.length}</strong> 行
          </span>
        </div>
      )}

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
  onRegenerateWorld,
  onRestart,
  onSave,
  provider,
  resource,
  restartPending,
  saveError,
  savePending,
  saveSuccess
}: {
  onRegenerateWorld?: () => void;
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
  const [worldGenerationOnlyPending, setWorldGenerationOnlyPending] = useState(false);
  const submittedRestartRequiredRef = useRef(false);
  const submittedWorldGenerationRef = useRef(false);
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
  const providerFields = provider?.configSchema ?? [];
  const providerEditorFields = providerFields.filter((field) => !isCuratedGameRuleField(resource.providerKey, field));
  const worldGenerationDirty = !isTerrariaProvider && providerFields.some((field) =>
    isWorldGenerationProviderConfigField(resource.providerKey, field)
      && providerConfigFieldChanged(currentProviderPayload, providerDraft, field)
  );
  const providerRuntimeDirty = !isTerrariaProvider && providerFields.some((field) =>
    !isWorldGenerationProviderConfigField(resource.providerKey, field)
      && providerConfigFieldChanged(currentProviderPayload, providerDraft, field)
  );
  const configDirty = isTerrariaProvider
    ? JSON.stringify(normalizedDraft) !== JSON.stringify({ ...resourceTerrariaConfig, port: terrariaInternalPort })
    : JSON.stringify(providerDraft) !== JSON.stringify(currentProviderPayload);
  const hostPortDirty = hostPortDraft !== resourceHostPort;
  const dirty = configDirty || hostPortDirty;
  const lifecycleLocked = isServerLifecyclePending(resourceStatus);
  const running = resourceStatus === "running";
  const disabled = lifecycleLocked || savePending;
  const restartRequired = running && !dirty && (restartRecommended || (!worldGenerationOnlyPending && gameServerConfigPendingRestart(resource)));
  const showConfigActions = dirty || savePending || saveSuccess || restartRequired || lifecycleLocked;
  const update = <K extends keyof TerrariaConfig>(key: K, value: TerrariaConfig[K]) => setDraft((current) => ({ ...current, [key]: value }));
  const updateGameRule = (key: string, value: unknown) => {
    if (isTerrariaProvider) {
      setDraft((current) => ({ ...current, [key]: value } as TerrariaConfig));
      return;
    }
    setProviderDraft((current) => updateProviderConfigPath(current, key, value));
  };
  useEffect(() => {
    if (dirty || !running || !gameServerConfigPendingRestart(resource)) {
      setRestartRecommended(false);
    }
  }, [dirty, resource, running]);
  useEffect(() => {
    if (saveSuccess && running) {
      setRestartRecommended(submittedRestartRequiredRef.current);
      setWorldGenerationOnlyPending(submittedWorldGenerationRef.current && !submittedRestartRequiredRef.current);
    }
  }, [running, saveSuccess]);
  useEffect(() => {
    if (!running) setWorldGenerationOnlyPending(false);
  }, [running]);
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
  const worldSizeLabel = draft.worldSize === "small" ? t("tagSmallWorld") : draft.worldSize === "medium" ? t("tagMediumWorld") : t("tagLargeWorld");
  const seedLabel = secretSeed
    ? terrariaSecretSeeds.find((s) => s.key === secretSeed)?.label ?? draft.seed ?? ""
    : (draft.seed || t("tagRandom"));
  const seedModeCount = terrariaSeedModeCodes(draft).length;
  return (
    <form className="space-y-4" onSubmit={(event) => {
      event.preventDefault();
      if (!disabled && dirty) {
        submittedRestartRequiredRef.current = isTerrariaProvider || hostPortDirty || providerRuntimeDirty;
        submittedWorldGenerationRef.current = worldGenerationDirty;
        onSave(isTerrariaProvider ? normalizedDraft : providerDraft, hostPortDraft);
      }
    }}>
      <ServerGameRules
        disabled={disabled}
        draft={isTerrariaProvider ? (draft as unknown as Record<string, unknown>) : providerDraft}
        onChange={updateGameRule}
        providerFields={providerFields}
        server={resource}
      />

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
              </div>
            </div>
            <div className="mt-4 grid overflow-hidden rounded-md border border-panel-line bg-slate-950/40 sm:grid-cols-2 sm:divide-x sm:divide-panel-line max-sm:divide-y max-sm:divide-panel-line">
              <ConfigSwitch label={t("secureMode")} checked={draft.secure} onChange={(checked) => update("secure", checked)} disabled={disabled} />
              <ConfigSwitch label={t("autoCreateWorld")} checked={draft.autoCreateWorld} onChange={(checked) => update("autoCreateWorld", checked)} disabled={disabled} />
            </div>
          </>
        ) : (
          <>
            <ProviderConfigEditor
              disabled={disabled}
              fields={providerEditorFields}
              hasUnsavedWorldGenerationChanges={worldGenerationDirty}
              payload={providerDraft}
              providerKey={provider?.key ?? ""}
              surface="server"
              fieldLabel={(field) => providerFieldLabel(field, t)}
              fieldHelp={(field) => providerFieldHelp(field, t)}
              onChange={(field, value) => setProviderDraft((current) => updateProviderConfigPayload(current, field, value))}
              onRegenerateWorld={onRegenerateWorld}
              onRestoreDefaults={(fields) => setProviderDraft((current) => restoreProviderConfigDefaults(current, fields))}
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
  return createDefaultProviderConfigPayload(provider, resource.spec.config ?? {});
}

function providerFieldLabel(field: ProviderConfigField, t: (key: MessageKey, params?: Record<string, string | number>) => string) {
  const key = providerFieldLabelKeys[field.name];
  return key ? t(key) : field.label;
}

function providerFieldHelp(field: ProviderConfigField, t: (key: MessageKey, params?: Record<string, string | number>) => string) {
  if (field.name === "adminPassword") return t("adminPasswordHelp");
  if (field.name === "clusterToken" || field.name === "identity.clusterToken") return t("clusterTokenHelp");
  if (field.name === "eulaAccepted") return t("minecraftEulaHelp");
  return field.help ?? "";
}

function ResourceLimitsDialog({
  open,
  resource,
  hostCpuCores,
  hostMemoryMb,
  savePending,
  onCancel,
  onSave
}: {
  open: boolean;
  resource: GameServerResource;
  hostCpuCores?: number;
  hostMemoryMb?: number;
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
  const cpuInvalid = draft.cpuLimitCores < 0 || (draft.cpuLimitCores > 0 && (draft.cpuLimitCores < 0.25 || draft.cpuLimitCores > 64));
  const memoryInvalid = draft.memoryLimitMb < 0 || (draft.memoryLimitMb > 0 && (draft.memoryLimitMb < 256 || draft.memoryLimitMb > 262144));
  const invalid = cpuInvalid || memoryInvalid;
  const cpuSliderMax = Math.max(1, hostCpuCores ?? 8, Math.ceil(draft.cpuLimitCores));
  const memorySliderMax = Math.max(1024, Math.floor((hostMemoryMb ?? 16384) / 1024) * 1024, Math.ceil(draft.memoryLimitMb / 1024) * 1024);
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
          if (!savePending && !lifecycleLocked && dirty && !invalid) onSave(draft);
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
        <div className="mt-5 grid gap-4">
          <div className={cn("rounded-lg border bg-slate-950/35 p-4", cpuInvalid ? "border-red-400/60" : "border-panel-line")}>
            <ResourceLimitSlider
              disabled={savePending || lifecycleLocked}
              formatValue={(value) => formatCpuResourceLimit(value, t("unlimited"), t("cpuUnit"))}
              label={t("cpuLimit")}
              max={cpuSliderMax}
              step={0.25}
              value={draft.cpuLimitCores}
              onChange={(value) => setDraft((current) => ({ ...current, cpuLimitCores: value }))}
            />
            {cpuInvalid ? <p className="mt-2 text-xs text-red-300">{t("cpuLimitRange")}</p> : null}
          </div>
          <div className={cn("rounded-lg border bg-slate-950/35 p-4", memoryInvalid ? "border-red-400/60" : "border-panel-line")}>
            <ResourceLimitSlider
              disabled={savePending || lifecycleLocked}
              formatValue={(value) => formatMemoryResourceLimit(value, t("unlimited"))}
              label={t("memoryLimit")}
              max={memorySliderMax}
              step={1024}
              value={draft.memoryLimitMb}
              onChange={(value) => setDraft((current) => ({ ...current, memoryLimitMb: value }))}
            />
            {memoryInvalid ? <p className="mt-2 text-xs text-red-300">{t("memoryLimitRange")}</p> : null}
          </div>
        </div>
        {lifecycleLocked && <p className="mt-3 rounded-md border border-panel-gold/25 bg-panel-gold/10 px-3 py-2 text-xs text-panel-gold">{t("configLifecycleLocked")}</p>}
        <div className="mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button type="button" variant="secondary" onClick={onCancel} disabled={savePending}>{t("cancel")}</Button>
          <Button disabled={savePending || lifecycleLocked || !dirty || invalid}>{savePending ? t("savingConfig") : t("saveResourceLimits")}</Button>
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

function ConfigSwitch({ checked, disabled, label, onChange }: { checked: boolean; disabled?: boolean; label: string; onChange: (checked: boolean) => void }) {
  return (
    <label className={cn(
      "group flex min-h-12 cursor-pointer items-center justify-between gap-4 px-3.5 py-2.5 text-sm text-slate-300 outline-none transition hover:bg-slate-900/60 hover:text-slate-100 focus-within:bg-slate-900/60 focus-within:ring-2 focus-within:ring-inset focus-within:ring-panel-green/40",
      disabled && "cursor-not-allowed opacity-60"
    )}>
      <span className="min-w-0 font-medium leading-5">{label}</span>
      <input
        className="sr-only"
        checked={checked}
        disabled={disabled}
        role="switch"
        type="checkbox"
        onChange={(event) => onChange(event.target.checked)}
      />
      <span
        aria-hidden="true"
        className={cn(
          "relative h-5 w-9 shrink-0 rounded-full border transition-colors duration-200",
          checked
            ? "border-panel-green bg-panel-green"
            : "border-slate-600 bg-slate-700 group-hover:border-slate-500"
        )}
      >
        <span
          className={cn(
            "absolute left-0.5 top-0.5 size-3.5 rounded-full bg-white transition-transform duration-200",
            checked && "translate-x-4"
          )}
        />
      </span>
    </label>
  );
}

function ModsTab({
  serverId,
  supportsModConfigs,
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
  uploadAccept,
  uploading,
  workshopUnsupported,
  onAssignMods,
  onDelete,
  onInstallPack,
  onUpload,
  onToggle
}: {
  serverId: string;
  supportsModConfigs: boolean;
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
  uploadAccept: string;
  uploading: boolean;
  workshopUnsupported: boolean;
  onAssignMods: (mods: ModFile[]) => void;
  onDelete: (mod: ModFile) => void;
  onInstallPack: (pack: ModPack) => void;
  onUpload?: () => void;
  onToggle: (mod: ModFile) => void;
}) {
  const { locale, t } = useI18n();
  const [installerOpen, setInstallerOpen] = useState(false);
  const [installSource, setInstallSource] = useState<ModInstallSource>("library");
  const [selectedModIds, setSelectedModIds] = useState<string[]>([]);
  const [activeSection, setActiveSection] = useState<ModPanelSection>("installed");
  const modConfigsSummary = useQuery({
    queryKey: ["server-mod-configs", serverId],
    queryFn: () => listModConfigs(serverId),
    enabled: supportsModConfigs
  });
  const modAction = describeResourceAction({ kind: "modifyMods", serverStatus });
  const blocked = modAction.disabled;
  const workshopBlockReason = workshopUnsupported ? t("workshopArmUnsupported") : "";
  const selectedMods = useMemo(
    () => availableMods.filter((mod) => isServerAssignableMod(mod) && selectedModIds.includes(mod.id) && !isModInstalledOnServer(mod, items)),
    [availableMods, items, selectedModIds]
  );
  const serverAssignableMods = useMemo(
    () => availableMods.filter(isServerAssignableMod),
    [availableMods]
  );
  useEffect(() => {
    setSelectedModIds((current) => current.filter((modId) => {
      const mod = availableMods.find((item) => item.id === modId);
      return mod ? isServerAssignableMod(mod) && !isModInstalledOnServer(mod, items) : false;
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

  const sectionTabs = supportsModConfigs ? (
    <div aria-label={t("detailModActions")} className="inline-flex rounded-lg border border-panel-line bg-slate-950/45 p-1" role="tablist">
      <button
        aria-controls="installed-mods-panel"
        aria-selected={activeSection === "installed"}
        className={cn(
          "rounded-md px-3 py-2 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-panel-green/50",
          activeSection === "installed" ? "bg-panel-green/10 text-panel-green" : "text-slate-400 hover:bg-slate-900 hover:text-slate-100"
        )}
        id="installed-mods-tab"
        onClick={() => setActiveSection("installed")}
        role="tab"
        type="button"
      >
        {t("installedModsTab", { count: items.length })}
      </button>
      <button
        aria-controls="mod-configs-panel"
        aria-selected={activeSection === "configs"}
        className={cn(
          "rounded-md px-3 py-2 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-panel-green/50",
          activeSection === "configs" ? "bg-panel-green/10 text-panel-green" : "text-slate-400 hover:bg-slate-900 hover:text-slate-100"
        )}
        id="mod-configs-tab"
        onClick={() => setActiveSection("configs")}
        role="tab"
        type="button"
      >
        {t("modConfigsTab", { count: modConfigsSummary.data?.length ?? 0 })}
      </button>
    </div>
  ) : null;

  return (
    <div>
      <div className="mb-4">
        <h2 className="text-xl font-semibold text-white">{t("detailModActions")}</h2>
        <p className="mt-1 text-sm text-slate-400">{t("serverModsCombinedHint")}</p>
      </div>
      <div className="space-y-4">
        {libraryError ? <p className="text-sm text-panel-gold">{t("modsApiUnavailable")}</p> : null}
        {pendingRestart ? (
          <div className="rounded-md border border-panel-gold/30 bg-panel-gold/10 px-3 py-2 text-sm text-panel-gold">
            {t("modChangesPendingRestart")}
          </div>
        ) : null}

        {activeSection === "installed" ? (
          <div aria-labelledby={supportsModConfigs ? "installed-mods-tab" : undefined} id="installed-mods-panel" role={supportsModConfigs ? "tabpanel" : undefined}>
            <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
              {sectionTabs}
              <Button onClick={() => setInstallerOpen(true)} disabled={blocked}>
                <Package aria-hidden="true" />
                {t("installMods")}
              </Button>
            </div>
            <div className="rounded-lg border border-panel-line bg-slate-950/35">
              {isError ? <p className="px-4 py-4 text-sm text-panel-gold">{t("modsApiUnavailable")}</p> : null}
              {!isError && isLoading ? <p className="px-4 py-4 text-sm text-slate-400">{t("loading")}</p> : null}
              {!isError && !isLoading && items.length === 0 ? (
                <div className="flex min-h-40 flex-col items-center justify-center px-5 py-8 text-center">
                  <Package aria-hidden="true" className="mb-3 size-6 text-slate-500" />
                  <p className="font-medium text-slate-100">{t("noInstalledMods")}</p>
                  <p className="mt-1 max-w-md text-sm text-slate-400">{t("noInstalledModsHint")}</p>
                  <div className="mt-4 flex flex-wrap justify-center gap-2">
                    <Link href="/mods" className="inline-flex h-10 items-center justify-center rounded-md border border-panel-line bg-slate-900/70 px-3 text-sm font-medium text-slate-100 transition hover:bg-slate-800 focus:outline-none focus:ring-2 focus:ring-panel-green/50">
                      {t("openFullManager")}
                    </Link>
                    {onUpload ? (
                      <Button variant="secondary" onClick={onUpload} disabled={uploading || blocked} title={uploadAccept ? `${t("uploadMod")} ${uploadAccept}` : undefined}>
                        <Upload aria-hidden="true" />
                        {uploading ? t("actionWorking") : t("uploadLocalMod")}
                      </Button>
                    ) : null}
                  </div>
                </div>
              ) : null}
              {items.length > 0 ? (
                <>
                  {onUpload ? (
                    <div className="flex justify-end border-b border-panel-line px-4 py-2.5">
                      <button className="text-sm font-medium text-slate-400 transition hover:text-panel-green focus:outline-none focus:ring-2 focus:ring-panel-green/50" disabled={uploading || blocked} onClick={onUpload} type="button">
                        {uploading ? t("actionWorking") : t("uploadLocalMod")}
                      </button>
                    </div>
                  ) : null}
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
                </>
              ) : null}
            </div>
          </div>
        ) : supportsModConfigs ? (
          <ModConfigsPanel serverId={serverId} serverStatus={serverStatus} tabs={sectionTabs} />
        ) : null}
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
                  serverAssignableMods.length > 0 ? (
                    <>
                    <div className="max-h-[44vh] divide-y divide-panel-line overflow-y-auto">
                      {serverAssignableMods.map((mod) => {
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
                      const containsUnsupportedDSTMod = pack.mods.some((mod) => !isServerAssignableMod(mod));
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
                              disabled={packInstalling || blocked || pack.modIds.length === 0 || blockedByArchitecture || containsUnsupportedDSTMod || allInstalled}
                              title={containsUnsupportedDSTMod ? t("dstUnsupportedPackBlocked") : blockedByArchitecture ? workshopBlockReason : modAction.reasonKey ? t(modAction.reasonKey) : undefined}
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
    </div>
  );
}

function ModConfigsPanel({ serverId, serverStatus, tabs }: { serverId: string; serverStatus: ServerStatus; tabs: ReactNode }) {
  const { locale, t } = useI18n();
  const client = useQueryClient();
  const uploadRef = useRef<HTMLInputElement>(null);
  const [editing, setEditing] = useState<ModConfigFile | null>(null);
  const [content, setContent] = useState("");
  const [pendingDelete, setPendingDelete] = useState<ModConfigFile | null>(null);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [restartRequired, setRestartRequired] = useState(false);
  const action = describeResourceAction({ kind: "modifyMods", serverStatus });
  const queryKey = ["server-mod-configs", serverId];
  const configs = useQuery({
    queryKey,
    queryFn: () => listModConfigs(serverId)
  });
  const openEditor = useMutation({
    mutationFn: (name: string) => getModConfig(serverId, name),
    onSuccess: (item) => {
      setEditing(item);
      setContent(item.content ?? "");
      setError("");
    },
    onError: (reason) => setError(reason instanceof Error ? reason.message : t("modConfigLoadFailed"))
  });
  const save = useMutation({
    mutationFn: () => saveModConfig(serverId, editing?.name ?? "", content),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey });
      setEditing(null);
      setRestartRequired(true);
      setNotice(t("modConfigSaved"));
      setError("");
    },
    onError: (reason) => setError(reason instanceof Error ? reason.message : t("modConfigLoadFailed"))
  });
  const upload = useMutation({
    mutationFn: (file: File) => uploadModConfig(serverId, file),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey });
      setRestartRequired(true);
      setNotice(t("modConfigUploaded"));
      setError("");
    },
    onError: (reason) => setError(reason instanceof Error ? reason.message : t("modConfigLoadFailed"))
  });
  const remove = useMutation({
    mutationFn: (name: string) => deleteModConfig(serverId, name),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey });
      setPendingDelete(null);
      setRestartRequired(true);
      setNotice(t("modConfigDeleted"));
      setError("");
    },
    onError: (reason) => setError(reason instanceof Error ? reason.message : t("modConfigLoadFailed"))
  });
  useEffect(() => {
    if (!editing) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !save.isPending) setEditing(null);
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [editing, save.isPending]);
  useEffect(() => {
    if (!notice) return;
    const timer = window.setTimeout(() => setNotice(""), 3500);
    return () => window.clearTimeout(timer);
  }, [notice]);

  return (
    <section aria-labelledby="mod-configs-tab" id="mod-configs-panel" role="tabpanel">
      <input
        ref={uploadRef}
        className="hidden"
        type="file"
        accept=".json,application/json"
        onChange={(event) => {
          const file = event.target.files?.[0];
          if (file) upload.mutate(file);
          event.target.value = "";
        }}
      />
      {(error || notice) && !editing ? (
        <div className="pointer-events-none fixed inset-x-4 bottom-4 z-[60] flex justify-end md:inset-x-auto md:bottom-auto md:right-6 md:top-24">
          <ToastNotice
            closeLabel={t("cancel")}
            message={error || notice}
            tone={error ? "error" : "success"}
            onClose={() => {
              setError("");
              setNotice("");
            }}
          />
        </div>
      ) : null}
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        {tabs}
        <Button onClick={() => uploadRef.current?.click()} disabled={upload.isPending || action.disabled} title={action.reasonKey ? t(action.reasonKey) : undefined}>
          <Upload aria-hidden="true" />
          {upload.isPending ? t("actionWorking") : t("uploadModConfig")}
        </Button>
      </div>
      <div className="rounded-lg border border-panel-line bg-slate-950/35">
        {restartRequired ? <p className="border-b border-panel-gold/20 bg-panel-gold/10 px-4 py-2.5 text-sm text-panel-gold">{t("modConfigRestartRequired")}</p> : null}
        {configs.isLoading ? <p className="px-4 py-4 text-sm text-slate-400">{t("loading")}</p> : null}
        {configs.isError ? <p className="px-4 py-4 text-sm text-panel-gold">{t("modConfigLoadFailed")}</p> : null}
        {!configs.isLoading && !configs.isError ? (
          <div className="flex flex-wrap items-center justify-between gap-2 border-b border-panel-line px-4 py-3 text-xs text-slate-500">
            <span>{t("modConfigFileCount", { count: configs.data?.length ?? 0 })}</span>
            {(configs.data?.length ?? 0) > 0 ? <span>{t("modConfigEffectHint")}</span> : null}
          </div>
        ) : null}
        {!configs.isLoading && !configs.isError && (configs.data?.length ?? 0) === 0 ? (
          <div className="flex min-h-40 flex-col items-center justify-center px-5 py-8 text-center">
            <Braces aria-hidden="true" className="mb-3 size-6 text-slate-500" />
            <p className="font-medium text-slate-100">{t("noModConfigsTitle")}</p>
            <p className="mt-1 max-w-lg text-sm text-slate-400">{t("noModConfigs")}</p>
          </div>
        ) : null}
        <div className="divide-y divide-panel-line">
          {(configs.data ?? []).map((item) => (
            <div className="flex min-h-16 items-center justify-between gap-4 px-4 py-3 transition hover:bg-slate-900/35" key={item.name}>
              <div className="min-w-0">
                <p className="truncate font-medium text-slate-100">{item.name}</p>
                <p className="mt-1 text-xs text-slate-500">{formatModConfigSize(item.sizeBytes)} · {new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(new Date(item.updatedAt))}</p>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Button variant="secondary" className="h-9 px-3 text-sm" onClick={() => openEditor.mutate(item.name)} disabled={openEditor.isPending || action.disabled}>
                  <Pencil aria-hidden="true" />
                  {t("edit")}
                </Button>
                <details className="group relative">
                  <summary aria-haspopup="menu" aria-label={t("moreActions")} className="flex size-9 cursor-pointer list-none items-center justify-center rounded-md border border-panel-line bg-slate-900/70 text-slate-300 transition hover:bg-slate-800 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/50 [&::-webkit-details-marker]:hidden" role="button">
                    <MoreHorizontal aria-hidden="true" className="size-4" />
                  </summary>
                  <div className="absolute right-0 top-11 z-20 min-w-36 rounded-md border border-panel-line bg-panel-card p-1">
                    <button
                      className="flex w-full items-center gap-2 rounded px-3 py-2 text-left text-sm text-red-300 transition hover:bg-red-400/10 focus:outline-none focus:ring-2 focus:ring-red-400/40"
                      disabled={remove.isPending || action.disabled}
                      onClick={(event) => {
                        event.currentTarget.closest("details")?.removeAttribute("open");
                        setPendingDelete(item);
                      }}
                      type="button"
                    >
                      <Trash2 aria-hidden="true" className="size-4" />
                      {t("deleteModConfig")}
                    </button>
                  </div>
                </details>
              </div>
            </div>
          ))}
        </div>
      </div>
      {editing ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/75 px-4 backdrop-blur-sm" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && !save.isPending) setEditing(null); }}>
          <div className="w-full max-w-3xl overflow-hidden rounded-lg border border-panel-line bg-panel-card shadow-[0_12px_40px_rgba(0,0,0,0.35)]" role="dialog" aria-modal="true" aria-labelledby="mod-config-editor-title">
            <div className="flex items-start justify-between gap-4 border-b border-panel-line px-5 py-4">
              <div className="min-w-0">
                <h3 className="truncate font-semibold text-white" id="mod-config-editor-title">{t("editModConfigTitle", { name: editing.name })}</h3>
                <p className="mt-1 text-sm text-slate-400">{t("modConfigValidationHint")}</p>
              </div>
              <button className="flex size-9 items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/50" onClick={() => setEditing(null)} aria-label={t("cancel")} type="button"><X aria-hidden="true" className="size-4" /></button>
            </div>
            <div className="p-5">
              <label className="text-sm font-medium text-slate-300" htmlFor="mod-config-content">{t("modConfigContent")}</label>
              <textarea autoFocus id="mod-config-content" className="mt-2 min-h-[360px] w-full resize-y rounded-md border border-panel-line bg-slate-950/70 p-3 font-mono text-sm leading-6 text-slate-100 outline-none transition focus:border-panel-green focus:ring-2 focus:ring-panel-green/20" spellCheck={false} value={content} onChange={(event) => setContent(event.target.value)} />
              {error ? <p className="mt-2 text-sm text-red-300">{error}</p> : null}
              <div className="mt-4 flex justify-end gap-2">
                <Button variant="secondary" onClick={() => setEditing(null)} disabled={save.isPending}>{t("cancel")}</Button>
                <Button onClick={() => save.mutate()} disabled={save.isPending}><Save aria-hidden="true" />{save.isPending ? t("actionWorking") : t("saveModConfig")}</Button>
              </div>
            </div>
          </div>
        </div>
      ) : null}
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        eyebrow={t("destructiveAction")}
        title={t("deleteModConfigConfirm", { name: pendingDelete?.name ?? "" })}
        description={t("modConfigRestartRequired")}
        cancelLabel={t("cancel")}
        confirmLabel={remove.isPending ? t("actionWorking") : t("delete")}
        busy={remove.isPending}
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => pendingDelete && remove.mutate(pendingDelete.name)}
      />
    </section>
  );
}

function formatModConfigSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  return `${Math.max(1, Math.round(bytes / 1024))} KB`;
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
  const status = modRuntimeState(mod);
  const scope = dstModScope(mod);
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
            {status ? (
              <span
                className={cn("shrink-0 rounded px-2 py-0.5 text-xs font-medium", modRuntimeStatusClassName(status))}
                title={status === "runtimeFileMissing" ? t("runtimeFileMissingHint") : undefined}
                aria-label={status === "runtimeFileMissing" ? `${t(status)}：${t("runtimeFileMissingHint")}` : t(status)}
              >
                {t(status)}
              </span>
            ) : null}
            {scope !== "unknown" ? (
              <span className={cn(
                "shrink-0 rounded border px-2 py-0.5 text-xs font-medium",
                scope === "client"
                  ? "border-sky-400/25 bg-sky-400/10 text-sky-300"
                  : scope === "server"
                    ? "border-panel-green/25 bg-panel-green/10 text-panel-green"
                    : "border-panel-gold/25 bg-panel-gold/10 text-panel-gold"
              )}>
                {t(scope === "client"
                  ? "dstClientOnlyMod"
                  : scope === "server"
                    ? "dstServerOnlyMod"
                    : scope === "required"
                      ? "dstServerRequiredMod"
                      : "dstUnknownModScope")}
              </span>
            ) : null}
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

function modRuntimeStatusClassName(status: ModRuntimeState): string {
  if (status === "enabled") return "bg-panel-green/15 text-panel-green";
  if (status === "notApplied") return "bg-panel-gold/15 text-panel-gold";
  if (status === "runtimeFileMissing") return "bg-panel-gold/15 text-panel-gold";
  return "bg-slate-800 text-slate-300";
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



function JoinServerPanel({
  copied,
  invite,
  joinAddress,
  joinPassword,
  joinPort,
  onCopy
}: {
  copied: string;
  invite: string;
  joinAddress: string;
  joinPassword: string;
  joinPort: number;
  onCopy: (label: string, value: string) => void | Promise<void>;
}) {
  const { t } = useI18n();
  const endpoint = `${joinAddress}:${joinPort}`;
  return (
    <Card className="p-4">
      <h2 className="font-semibold">{t("joinServer")}</h2>
      <CopyRow className="mt-3" label={t("serverAddress")} value={endpoint} copied={copied} copiedLabel={t("copied")} copyLabel={t("copy")} onCopy={onCopy} />
      <CopyRow className="mt-2" label={t("password")} value={joinPassword || t("none")} secret={Boolean(joinPassword)} copied={copied} copiedLabel={t("copied")} copyLabel={t("copy")} onCopy={onCopy} />
      <div className="mt-3">
        <Button className="w-full px-2 text-xs" variant="secondary" onClick={() => void onCopy("Invite", invite)}>
          <Copy aria-hidden="true" />
          {copied === "Invite" ? t("copied") : t("actionCopyInvite")}
        </Button>
      </div>
    </Card>
  );
}

function ShareServerPanel({ enabled, onOpen }: { enabled: boolean; onOpen: () => void }) {
  const { t } = useI18n();
  return (
    <Card className="p-4">
      <div className="flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/45 text-panel-green">
          <Share2 aria-hidden="true" className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="font-semibold">{t("shareServer")}</h2>
            <span className={cn(
              "inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-[11px] font-medium",
              enabled
                ? "border-panel-green/30 bg-panel-green/10 text-panel-green"
                : "border-panel-line bg-slate-950/45 text-slate-400"
            )}>
              <span className="size-1.5 rounded-full bg-current" />
              {enabled ? t("sharePageEnabled") : t("sharePageDisabled")}
            </span>
          </div>
          <p className="mt-1 text-xs leading-5 text-slate-400">{t("shareServerDescription")}</p>
        </div>
      </div>
      <Button className="mt-3 w-full text-xs" variant="secondary" onClick={onOpen}>
        <Share2 aria-hidden="true" className="size-3.5" />
        {t("manageShareServer")}
      </Button>
    </Card>
  );
}

function ShareServerDialog({
  copied,
  open,
  shareDisabling,
  shareEnabled,
  shareIncludePassword,
  shareLoading,
  sharePath,
  shareSaving,
  shareUrl,
  savedIncludePassword,
  onCancel,
  onCopy,
  onDisableShare,
  onEnableShare,
  onShareIncludePasswordChange
}: {
  copied: string;
  open: boolean;
  shareDisabling: boolean;
  shareEnabled: boolean;
  shareIncludePassword: boolean;
  shareLoading: boolean;
  sharePath: string;
  shareSaving: boolean;
  shareUrl: string;
  savedIncludePassword: boolean;
  onCancel: () => void;
  onCopy: (label: string, value: string) => void | Promise<void>;
  onDisableShare: () => void;
  onEnableShare: () => void;
  onShareIncludePasswordChange: (value: boolean) => void;
}) {
  const { t } = useI18n();
  const busy = shareSaving || shareDisabling;
  const shareDraftDirty = shareIncludePassword !== savedIncludePassword;
  return (
    <ConfirmDialog
      open={open}
      eyebrow={shareEnabled ? t("sharePageEnabled") : t("sharePageDisabled")}
      eyebrowTone={shareEnabled ? "green" : "neutral"}
      title={t("shareServer")}
      description={t("shareServerDescription")}
      detail={(
        <div className="space-y-3">
          <label className="flex items-center gap-2 rounded-md border border-panel-line bg-slate-900/45 px-3 py-2.5 text-sm text-slate-300">
            <input
              className="size-4 accent-panel-green"
              type="checkbox"
              checked={shareIncludePassword}
              onChange={(event) => onShareIncludePasswordChange(event.target.checked)}
            />
            {t("includePasswordInShare")}
          </label>
          {shareEnabled && shareUrl ? (
            <div className="grid gap-2 sm:grid-cols-2">
              <Button variant="secondary" disabled={busy || shareDraftDirty} onClick={() => void onCopy("ShareLink", shareUrl)}>
                <Copy aria-hidden="true" />
                {copied === "ShareLink" ? t("shareLinkCopied") : t("copyShareLink")}
              </Button>
              <Link
                aria-disabled={busy || shareDraftDirty}
                className={cn(
                  "inline-flex h-10 items-center justify-center gap-2 rounded-md border border-panel-line bg-slate-900/70 px-3 text-sm font-medium text-slate-200 transition hover:border-panel-green/40 hover:text-panel-green",
                  (busy || shareDraftDirty) && "pointer-events-none opacity-50"
                )}
                href={sharePath}
                tabIndex={busy || shareDraftDirty ? -1 : undefined}
                target="_blank"
              >
                <ExternalLink aria-hidden="true" className="size-4" />
                {t("openSharePage")}
              </Link>
              <Button className="sm:col-span-2" variant="danger" onClick={onDisableShare} disabled={busy}>
                {shareDisabling ? t("saving") : t("disableSharePage")}
              </Button>
            </div>
          ) : null}
          {shareDraftDirty ? <p className="text-xs leading-5 text-panel-gold">{t("shareSaveBeforeOpen")}</p> : null}
        </div>
      )}
      cancelLabel={t("cancel")}
      confirmLabel={shareSaving ? t("saving") : shareEnabled ? t("saveButton") : t("enableSharePage")}
      confirmVariant="primary"
      busy={busy}
      confirmDisabled={shareLoading}
      onCancel={onCancel}
      onConfirm={onEnableShare}
    />
  );
}

function CopyRow({
  className,
  copied,
  copiedLabel,
  copyLabel,
  label,
  onCopy,
  secret = false,
  value
}: {
  className?: string;
  copied: string;
  copiedLabel: string;
  copyLabel: string;
  label: string;
  onCopy: (label: string, value: string) => void;
  secret?: boolean;
  value: string;
}) {
  const { t } = useI18n();
  const [revealed, setRevealed] = useState(false);
  return (
    <div className={cn("flex items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/50 px-3 py-2", className)}>
      <div className="min-w-0">
        <p className="text-xs text-slate-400">{label}</p>
        <p className="truncate text-sm">{secret && !revealed ? "••••••••" : value}</p>
      </div>
      <div className="flex shrink-0 gap-1">
        {secret ? (
          <Button aria-label={revealed ? t("hideSensitiveValue", { label }) : t("showSensitiveValue", { label })} className="size-8 p-0" variant="ghost" onClick={() => setRevealed((val) => !val)}>
            {revealed ? <EyeOff aria-hidden="true" className="size-4" /> : <Eye aria-hidden="true" className="size-4" />}
          </Button>
        ) : null}
        <Button className="h-8 px-2 text-xs" variant="secondary" onClick={() => onCopy(label, value)}>
          {copied === label ? copiedLabel : copyLabel}
        </Button>
      </div>
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
