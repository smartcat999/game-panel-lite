"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, ArrowRight, Ban, CheckCircle2, Clock, Copy, Cpu, Download, FileArchive, FileText, KeyRound, Megaphone, MemoryStick, Moon, Package, Plug, Power, RotateCcw, Save, Send, Sun, Sunrise, Terminal, Trash2, Upload, UserX, Users, Waves, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from "react";
import type { TerrariaConfig } from "@gamepanel-lite/shared";
import { secretSeedKeyFor, terrariaInternalPort, terrariaSecretSeeds } from "@gamepanel-lite/shared";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { ServerActions } from "@/components/server-actions";
import { ServerModeBadge, ServerStatusBadge } from "@/components/server-badges";
import { Button, Card, Input } from "@/components/ui";
import {
  assignMod,
  createBackup,
  createWorldSnapshot,
  deleteBackup,
  deleteMod,
  deleteWorld,
  downloadBackupFile,
  downloadWorldFile,
  getServer,
  getServerLogSnapshot,
  getServerStats,
  importWorkshopMods,
  listBackups,
  listGlobalMods,
  listModPacks,
  listMods,
  listWorlds,
  previewTerrariaConfig,
  restoreBackup,
  sendServerCommand,
  serverAction,
  setModEnabled,
  serverLogsUrl,
  updateServerConfig,
  uploadMod,
} from "@/lib/api";
import { saveBlob } from "@/lib/download";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import { describeResourceAction, formatServerDetailError, isServerLifecyclePending } from "@/lib/server-detail-actions";
import { isWorldActiveOnServer } from "@/lib/server-detail-resources";
import { serverInviteText, serverJoinPort } from "@/lib/server-join";
import { cn } from "@/lib/utils";
import type { Backup, ModFile, ModPack, Server, World } from "@/lib/types";

type TabId = "overview" | "console" | "logs" | "config" | "worlds" | "backups" | "mods";

export default function ServerDetailPage() {
  const { t } = useI18n();
  const params = useParams<{ id: string }>();
  const id = params.id;
  const client = useQueryClient();
  const modInputRef = useRef<HTMLInputElement>(null);
  const logViewportRef = useRef<HTMLDivElement>(null);
  const logServerIdRef = useRef("");

  const query = useQuery({ queryKey: ["server", id], queryFn: () => getServer(id), retry: false, refetchInterval: 5000 });
  const server = query.data;
  const statsQuery = useQuery({ queryKey: ["server-stats", id], queryFn: () => getServerStats(id), enabled: server?.status === "running", refetchInterval: 3000, retry: false });
  const worldsQuery = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, enabled: Boolean(server), retry: false });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, enabled: Boolean(server), retry: false });
  const modsQuery = useQuery({
    queryKey: ["mods", id],
    queryFn: () => listMods(id),
    enabled: Boolean(server && server.mode === "tmodloader"),
    retry: false
  });
  const globalModsQuery = useQuery({
    queryKey: ["global-mods"],
    queryFn: listGlobalMods,
    enabled: Boolean(server && server.mode === "tmodloader"),
    retry: false
  });
  const modPacksQuery = useQuery({
    queryKey: ["mod-packs"],
    queryFn: listModPacks,
    enabled: Boolean(server && server.mode === "tmodloader"),
    retry: false
  });
  const [activeTab, setActiveTab] = useState<TabId>("overview");
  const [copied, setCopied] = useState("");
  const [logs, setLogs] = useState<string[]>([]);
  const [command, setCommand] = useState("");
  const [workshopIdsText, setWorkshopIdsText] = useState("");
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [consoleError, setConsoleError] = useState("");
  const [pendingWorldDelete, setPendingWorldDelete] = useState<World | null>(null);
  const [pendingWorldSnapshot, setPendingWorldSnapshot] = useState(false);
  const [pendingBackupCreate, setPendingBackupCreate] = useState(false);
  const [pendingRestore, setPendingRestore] = useState<Backup | null>(null);
  const [pendingBackupDelete, setPendingBackupDelete] = useState<Backup | null>(null);
  const [pendingModUpload, setPendingModUpload] = useState<File | null>(null);
  const [pendingModDelete, setPendingModDelete] = useState<ModFile | null>(null);
  const [pendingModToggle, setPendingModToggle] = useState<{ mod: ModFile; enabled: boolean } | null>(null);
  const [pendingModAssign, setPendingModAssign] = useState<ModFile | null>(null);
  const [pendingModPackInstall, setPendingModPackInstall] = useState<ModPack | null>(null);
  const [pendingWorkshopImport, setPendingWorkshopImport] = useState(false);
  const [pendingConfigRestart, setPendingConfigRestart] = useState(false);
  const [downloadingResourceId, setDownloadingResourceId] = useState("");
  const [logStatus, setLogStatus] = useState<"idle" | "connecting" | "connected" | "error" | "paused">("idle");
  const [logStreamPaused, setLogStreamPaused] = useState(false);
  const [configSaved, setConfigSaved] = useState(false);
  const successTimerRef = useRef<number | null>(null);
  const formatActionError = (error: unknown, fallback: string) => formatServerDetailError(error, {
    dockerUnavailable: t("detailDockerUnavailable"),
    containerUnavailable: t("detailContainerUnavailable")
  }) || fallback;
  const formatSnapshotError = (error: unknown) => {
    const message = error instanceof Error ? error.message : String(error || "");
    if (message.toLowerCase().includes("current world file has not been created yet")) {
      return t("worldSnapshotRequiresGeneratedWorld");
    }
    return formatActionError(error, t("unableSaveWorldSnapshot"));
  };

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
    mutationFn: ({ config, hostPort }: { config: TerrariaConfig; hostPort: number }) => updateServerConfig(id, config, hostPort),
    onSuccess: async (updatedServer) => {
      showSuccess(t("configSaved"));
      setConfigSaved(true);
      client.setQueryData(["server", id], updatedServer);
      await client.invalidateQueries({ queryKey: ["servers"] });
      window.setTimeout(() => setConfigSaved(false), 1800);
    },
    onError: (error) => {
      setConfigSaved(false);
      showError(formatActionError(error, t("unableUpdateConfig")));
    }
  });
  const configRestart = useMutation({
    mutationFn: () => serverAction(id, "restart"),
    onSuccess: async (updatedServer) => {
      showSuccess(t("serverRestartQueued"));
      setPendingConfigRestart(false);
      if (updatedServer) {
        client.setQueryData(["server", id], updatedServer);
      }
      await client.invalidateQueries({ queryKey: ["server", id] });
      await client.invalidateQueries({ queryKey: ["servers"] });
    },
    onError: (error) => showError(formatActionError(error, t("unableAction", { action: t("actionRestart") })))
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
      await client.invalidateQueries({ queryKey: ["server", id] });
      await client.invalidateQueries({ queryKey: ["servers"] });
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
  const modUpload = useMutation({
    mutationFn: (file: File) => uploadMod(id, file),
    onSuccess: async () => {
      showSuccess(t("modUploaded"));
      setPendingModUpload(null);
      if (modInputRef.current) modInputRef.current.value = "";
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => showError(formatActionError(error, t("unableUploadMod")))
  });
  const modEnabled = useMutation({
    mutationFn: ({ modId, enabled }: { modId: string; enabled: boolean }) => setModEnabled(id, modId, enabled),
    onSuccess: async (updatedMod) => {
      showSuccess(updatedMod.enabled ? t("modEnabled") : t("modDisabled"));
      setPendingModToggle(null);
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => showError(formatActionError(error, t("unableUpdateMod")))
  });
  const modDelete = useMutation({
    mutationFn: (modId: string) => deleteMod(id, modId),
    onSuccess: async () => {
      showSuccess(t("modDeleted"));
      setPendingModDelete(null);
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => showError(formatActionError(error, t("unableDeleteMod")))
  });
  const modAssign = useMutation({
    mutationFn: (modId: string) => assignMod(modId, id),
    onSuccess: async () => {
      showSuccess(t("modAssigned"));
      setPendingModAssign(null);
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
      setPendingModPackInstall(null);
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => showError(formatActionError(error, t("unableAssignMod")))
  });
  const workshopImport = useMutation({
    mutationFn: () => importWorkshopMods(id, parseWorkshopIds(workshopIdsText)),
    onSuccess: async () => {
      showSuccess(t("workshopModsImported"));
      setPendingWorkshopImport(false);
      setWorkshopIdsText("");
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => showError(formatActionError(error, t("unableImportWorkshopMods")))
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
    }
    if (server?.status !== "running") {
      let alive = true;
      setLogStatus("connecting");
      getServerLogSnapshot(id)
        .then((lines) => {
          if (!alive) return;
          setLogs(lines.slice(-300));
          setConsoleError("");
          setLogStatus("idle");
        })
        .catch((error) => {
          if (!alive) return;
          setLogStatus("error");
          setConsoleError(formatActionError(error, t("logsUnavailable")));
        });
      return () => {
        alive = false;
      };
    }
    setLogStatus("connecting");
    const source = new EventSource(serverLogsUrl(id));
    source.onopen = () => {
      setConsoleError("");
      setLogStatus("connected");
    };
    source.addEventListener("log", (event) => {
      setLogs((current) => [...current, event.data].slice(-300));
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
    return () => source.close();
  }, [activeTab, id, server?.status, logStreamPaused, t]);

  useEffect(() => {
    const viewport = logViewportRef.current;
    if (viewport) viewport.scrollTop = viewport.scrollHeight;
  }, [logs, activeTab]);

  const serverWorlds = useMemo(
    () => (
      server
        ? (worldsQuery.data ?? []).filter((world) => world.instanceId === server.id)
        : []
    ),
    [server, worldsQuery.data]
  );
  const serverBackups = useMemo(
    () => (server ? (backupsQuery.data ?? []).filter((backup) => backup.instanceId === server.id) : []),
    [backupsQuery.data, server]
  );
  const serverMods = useMemo(() => modsQuery.data ?? [], [modsQuery.data]);
  const globalMods = useMemo(() => globalModsQuery.data ?? [], [globalModsQuery.data]);
  const modPacks = useMemo(() => modPacksQuery.data ?? [], [modPacksQuery.data]);
  const workshopIds = useMemo(() => parseWorkshopIds(workshopIdsText), [workshopIdsText]);
  if (!server) {
    return (
      <>
        <Link href="/servers" className="text-sm text-slate-400 hover:text-panel-green">{t("backToServers")}</Link>
        <Card className="mt-4 p-6 text-sm text-slate-400">{query.isLoading ? t("loading") : t("serverNotFound")}</Card>
      </>
    );
  }

  const tabs: { id: TabId; label: string }[] = [
    { id: "overview", label: t("tabOverview") },
    { id: "console", label: t("tabConsole") },
    { id: "logs", label: t("tabLogs") },
    { id: "config", label: t("tabConfig") },
    { id: "worlds", label: t("tabWorlds") },
    { id: "backups", label: t("tabBackups") },
    ...(server.mode === "tmodloader" ? [{ id: "mods" as const, label: t("tabMods") }] : [])
  ];
  const invite = serverInviteText(server);
  const logStatusLabel = logStatus === "connected" ? t("logsConnected") : logStatus === "error" ? t("logsDisconnected") : logStatus === "paused" ? t("logsPaused") : logStatus === "idle" ? t("logsIdle") : t("logsConnecting");
  const copy = async (label: string, value: string) => {
    try {
      await navigator.clipboard.writeText(value);
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
      {server.status === "errored" && server.lastError && (
        <div className="mt-3 rounded-md border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-100">
          <p className="font-medium">{t("serverRuntimeError")}</p>
          <p className="mt-1 break-words text-red-100/85">{formatActionError(new Error(server.lastError), server.lastError)}</p>
        </div>
      )}
      {errorMessage && <p className="mt-3 rounded-md border border-panel-gold/30 bg-panel-gold/10 px-3 py-2 text-sm text-panel-gold">{errorMessage}</p>}
      {successMessage && <p className="mt-3 rounded-md border border-panel-green/30 bg-panel-green/10 px-3 py-2 text-sm text-panel-green">{successMessage}</p>}
      <div className="mt-3 flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold">{server.name}</h1>
            <ServerModeBadge mode={server.mode} />
            <ServerStatusBadge status={server.status} />
          </div>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-slate-400">
            <HeaderMetric
              icon={<Users aria-hidden="true" className="size-3.5" />}
              label={t("players")}
              value={`${server.players} / ${server.maxPlayers}`}
            />
            <HeaderMetric
              icon={<Plug aria-hidden="true" className="size-3.5" />}
              label={t("port")}
              value={String(serverJoinPort(server))}
            />
            <HeaderMetric
              accent
              icon={<Cpu aria-hidden="true" className="size-3.5" />}
              label={t("serverCpu")}
              value={server.status === "running" ? `${(statsQuery.data?.cpuPercent ?? 0).toFixed(1)}%` : "—"}
              active={server.status === "running"}
            />
            <HeaderMetric
              accent
              icon={<MemoryStick aria-hidden="true" className="size-3.5" />}
              label={t("serverMemory")}
              value={server.status === "running" && statsQuery.data ? `${statsQuery.data.memoryMb} MB` : "—"}
              active={server.status === "running"}
            />
          </div>
        </div>
        <ServerActions server={server} showInvite={false} />
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
        <Card className="min-w-0 p-4">
          <div className="mb-4 flex gap-2 overflow-x-auto border-b border-panel-line px-1 pb-4 pt-1" role="tablist" aria-label={server.name}>
            {tabs.map((tab) => (
              <button
                key={tab.id}
                type="button"
                role="tab"
                aria-selected={activeTab === tab.id}
                className={cn(
                  "shrink-0 rounded-md border border-transparent px-3 py-2 text-sm font-medium text-slate-400 transition hover:bg-slate-800/80 hover:text-white focus:outline-none focus:ring-2 focus:ring-inset focus:ring-panel-green/50",
                  activeTab === tab.id && "border-panel-green/40 bg-panel-green/15 text-white shadow-[inset_0_0_0_1px_rgba(123,217,120,0.18)]"
                )}
                onClick={() => setActiveTab(tab.id)}
              >
                {tab.label}
              </button>
            ))}
          </div>

          {activeTab === "overview" && (
            <OverviewTab
              server={server}
              worldCount={serverWorlds.length}
              backupCount={serverBackups.length}
              modCount={serverMods.length}
              onSelectTab={setActiveTab}
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
              server={server}
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
              saveError={configSave.error instanceof Error ? configSave.error.message : ""}
              savePending={configSave.isPending}
              saveSuccess={configSaved}
              restartPending={configRestart.isPending}
              server={server}
              onRestart={() => setPendingConfigRestart(true)}
              onSave={(nextConfig, hostPort) => configSave.mutate({ config: nextConfig, hostPort })}
            />
          )}
          {activeTab === "worlds" && (
            <WorldsTab
              isError={worldsQuery.isError}
              isLoading={worldsQuery.isLoading}
              items={serverWorlds}
              deleting={worldDelete.isPending}
              currentServerId={server.id}
              downloadingId={downloadingResourceId}
              snapshotting={worldSnapshotCreate.isPending}
              onDelete={setPendingWorldDelete}
              onDownload={(world) => void downloadWorld(world)}
              onCreateSnapshot={() => setPendingWorldSnapshot(true)}
            />
          )}
          {activeTab === "backups" && (
            <BackupsTab
              creating={backupCreate.isPending}
              isError={backupsQuery.isError}
              isLoading={backupsQuery.isLoading}
              items={serverBackups}
              deleting={backupDelete.isPending}
              downloadingId={downloadingResourceId}
              restoring={backupRestore.isPending}
              serverStatus={server.status}
              onDelete={setPendingBackupDelete}
              onDownload={(backup) => void downloadBackup(backup)}
              onCreate={() => setPendingBackupCreate(true)}
              onRestore={setPendingRestore}
            />
          )}
          {activeTab === "mods" && server.mode === "tmodloader" && (
            <ModsTab
              availableMods={globalMods}
              assigning={modAssign.isPending}
              deleting={modDelete.isPending}
              importingWorkshop={workshopImport.isPending}
              isError={modsQuery.isError}
              isLoading={modsQuery.isLoading}
              items={serverMods}
              libraryError={globalModsQuery.isError || modPacksQuery.isError}
              modPacks={modPacks}
              packInstalling={modPackAssign.isPending}
              serverStatus={server.status}
              toggling={modEnabled.isPending}
              uploading={modUpload.isPending}
              workshopIdsCount={workshopIds.length}
              workshopIdsText={workshopIdsText}
              onAssignMod={setPendingModAssign}
              onDelete={setPendingModDelete}
              onImportWorkshop={() => setPendingWorkshopImport(true)}
              onInstallPack={setPendingModPackInstall}
              onToggle={(mod) => setPendingModToggle({ mod, enabled: !mod.enabled })}
              onUploadClick={() => modInputRef.current?.click()}
              onWorkshopIdsChange={setWorkshopIdsText}
            />
          )}
        </Card>

        <div className="flex flex-col gap-4">
          <Card className="p-4">
            <h2 className="font-semibold">{t("joinServer")}</h2>
            <CopyRow label={t("ipAddress")} value="127.0.0.1" copied={copied} copiedLabel={t("copied")} copyLabel={t("copy")} onCopy={copy} />
            <CopyRow label={t("port")} value={String(serverJoinPort(server))} copied={copied} copiedLabel={t("copied")} copyLabel={t("copy")} onCopy={copy} />
            <CopyRow label={t("password")} value={server.password || t("none")} copied={copied} copiedLabel={t("copied")} copyLabel={t("copy")} onCopy={copy} />
            <Button className="mt-4 w-full" variant="secondary" onClick={() => void copy("Invite", invite)}>
              <Copy aria-hidden="true" />
              {copied === "Invite" ? t("copied") : t("copyInviteText")}
            </Button>
          </Card>
          <Card className="p-4">
            <h2 className="font-semibold">{t("worldTemplate")}</h2>
            {server.sourceWorldId ? (
              <Link
                href={`/worlds/${server.sourceWorldId}`}
                className="mt-4 flex items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/35 px-3 py-3 transition hover:border-panel-green/50 hover:bg-slate-900/60 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
              >
                <p className="truncate text-sm font-medium text-slate-100">{server.sourceWorldName || t("worldTemplate")}</p>
                <ArrowRight aria-hidden="true" className="size-4 shrink-0 text-slate-500" />
              </Link>
            ) : (
              <div className="mt-4 rounded-md border border-panel-line bg-slate-950/35 px-3 py-3">
                <p className="truncate text-sm font-medium text-slate-500">{t("noWorldTemplate")}</p>
              </div>
            )}
          </Card>
        </div>
      </div>

      <input
        ref={modInputRef}
        className="hidden"
        type="file"
        accept=".tmod,.txt,.json"
        onChange={(event) => {
          const file = event.target.files?.[0];
          if (file) setPendingModUpload(file);
        }}
      />

      <ConfirmDialog
        open={pendingConfigRestart}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmServerActionTitle", { action: t("actionRestart") })}
        description={t("confirmRestartForConfigDescription", { name: server.name })}
        detail={<DetailLine label={t("server")} value={server.name} />}
        cancelLabel={t("cancel")}
        confirmLabel={configRestart.isPending ? t("actionWorking") : t("confirmServerActionButton", { action: t("actionRestart") })}
        confirmVariant="gold"
        busy={configRestart.isPending}
        onCancel={() => setPendingConfigRestart(false)}
        onConfirm={() => configRestart.mutate()}
      />
      <ConfirmDialog
        open={Boolean(pendingWorldDelete)}
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
        open={pendingWorldSnapshot}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmWorldSnapshotTitle", { name: server.name })}
        description={t("confirmWorldSnapshotDescription", { name: server.name })}
        detail={<DetailLine label={t("server")} value={server.name} />}
        cancelLabel={t("cancel")}
        confirmLabel={worldSnapshotCreate.isPending ? t("actionWorking") : t("saveWorldSnapshot")}
        confirmVariant="gold"
        busy={worldSnapshotCreate.isPending}
        onCancel={() => setPendingWorldSnapshot(false)}
        onConfirm={() => worldSnapshotCreate.mutate()}
      />
      <ConfirmDialog
        open={pendingBackupCreate}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmBackupCreateTitle", { name: server.name })}
        description={t("confirmBackupCreateDescription", { name: server.name })}
        detail={<DetailLine label={t("server")} value={server.name} />}
        cancelLabel={t("cancel")}
        confirmLabel={backupCreate.isPending ? t("actionWorking") : t("createBackupNow")}
        confirmVariant="gold"
        busy={backupCreate.isPending}
        onCancel={() => setPendingBackupCreate(false)}
        onConfirm={() => backupCreate.mutate()}
      />
      <ConfirmDialog
        open={Boolean(pendingRestore)}
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
        open={Boolean(pendingBackupDelete)}
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
        open={Boolean(pendingModUpload)}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmModUploadTitle", { name: pendingModUpload?.name ?? "" })}
        description={t("confirmModUploadDescription", { name: pendingModUpload?.name ?? "", server: server.name })}
        detail={pendingModUpload ? <DetailLine label={t("modsTitle")} value={pendingModUpload.name} /> : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={modUpload.isPending ? t("actionWorking") : t("uploadMod")}
        confirmVariant="gold"
        busy={modUpload.isPending}
        onCancel={() => {
          setPendingModUpload(null);
          if (modInputRef.current) modInputRef.current.value = "";
        }}
        onConfirm={() => pendingModUpload && modUpload.mutate(pendingModUpload)}
      />
      <ConfirmDialog
        open={Boolean(pendingModToggle)}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmModToggleTitle", { action: pendingModToggle?.enabled ? t("enable") : t("disable"), name: pendingModToggle?.mod.fileName ?? "" })}
        description={t("confirmModToggleDescription", { action: pendingModToggle?.enabled ? t("enable") : t("disable"), name: pendingModToggle?.mod.fileName ?? "" })}
        detail={pendingModToggle ? <DetailLine label={t("modsTitle")} value={pendingModToggle.mod.fileName} /> : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={modEnabled.isPending ? t("actionWorking") : pendingModToggle?.enabled ? t("enable") : t("disable")}
        confirmVariant="gold"
        busy={modEnabled.isPending}
        onCancel={() => setPendingModToggle(null)}
        onConfirm={() => pendingModToggle && modEnabled.mutate({ modId: pendingModToggle.mod.id, enabled: pendingModToggle.enabled })}
      />
      <ConfirmDialog
        open={Boolean(pendingModAssign)}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmModInstallTitle", { name: pendingModAssign?.fileName ?? "" })}
        description={t("confirmModInstallDescription", { name: pendingModAssign?.fileName ?? "", server: server.name })}
        detail={pendingModAssign ? <DetailLine label={t("modsTitle")} value={pendingModAssign.fileName} /> : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={modAssign.isPending ? t("actionWorking") : t("installToServer")}
        confirmVariant="gold"
        busy={modAssign.isPending}
        onCancel={() => setPendingModAssign(null)}
        onConfirm={() => pendingModAssign && modAssign.mutate(pendingModAssign.id)}
      />
      <ConfirmDialog
        open={Boolean(pendingModPackInstall)}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmModPackInstallTitle", { name: pendingModPackInstall?.name ?? "" })}
        description={t("confirmModPackInstallDescription", { name: pendingModPackInstall?.name ?? "", server: server.name })}
        detail={pendingModPackInstall ? <DetailLine label={t("modPacks")} value={pendingModPackInstall.name} /> : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={modPackAssign.isPending ? t("actionWorking") : t("installModPack")}
        confirmVariant="gold"
        busy={modPackAssign.isPending}
        onCancel={() => setPendingModPackInstall(null)}
        onConfirm={() => pendingModPackInstall && modPackAssign.mutate(pendingModPackInstall)}
      />
      <ConfirmDialog
        open={pendingWorkshopImport}
        eyebrow={t("confirmActionEyebrow")}
        title={t("confirmWorkshopImportTitle", { count: workshopIds.length })}
        description={t("confirmWorkshopImportDescription", { count: workshopIds.length, server: server.name })}
        detail={<DetailLine label={t("workshopIds")} value={String(workshopIds.length)} />}
        cancelLabel={t("cancel")}
        confirmLabel={workshopImport.isPending ? t("actionWorking") : t("importWorkshopMods")}
        confirmVariant="gold"
        busy={workshopImport.isPending}
        onCancel={() => setPendingWorkshopImport(false)}
        onConfirm={() => workshopImport.mutate()}
      />
      <ConfirmDialog
        open={Boolean(pendingModDelete)}
        eyebrow={t("destructiveAction")}
        title={t("deleteModConfirm", { name: pendingModDelete?.fileName ?? "" })}
        description={t("confirmDeleteModDescription", { name: pendingModDelete?.fileName ?? "" })}
        detail={pendingModDelete ? <DetailLine label={t("modsTitle")} value={pendingModDelete.fileName} /> : undefined}
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
  server,
  worldCount,
  backupCount,
  modCount,
  onSelectTab
}: {
  server: Server;
  worldCount: number;
  backupCount: number;
  modCount: number;
  onSelectTab: (tab: TabId) => void;
}) {
  const { t } = useI18n();
  const detailItems = [
    { label: t("world"), value: server.world },
    { label: t("difficulty"), value: difficultyLabel(server.config.difficulty, t) },
    { label: t("worldSize"), value: worldSizeLabel(server.config.worldSize, t) },
    { label: t("maxPlayers"), value: String(server.maxPlayers) },
    { label: t("version"), value: server.version },
    ...(server.hostPort > 0 && server.hostPort !== server.port ? [{ label: t("hostPort"), value: String(server.hostPort) }] : [])
  ];
  return (
    <div className="space-y-4">
      <div className="grid gap-3 md:grid-cols-3">
        <SummaryButton icon={<FileText aria-hidden="true" />} label={t("tabWorlds")} value={String(worldCount)} onClick={() => onSelectTab("worlds")} />
        <SummaryButton icon={<Archive aria-hidden="true" />} label={t("tabBackups")} value={String(backupCount)} onClick={() => onSelectTab("backups")} />
        {server.mode === "tmodloader" && <SummaryButton icon={<Package aria-hidden="true" />} label={t("tabMods")} value={String(modCount)} onClick={() => onSelectTab("mods")} />}
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

function ConsoleTab({
  command,
  commandPending,
  consoleError,
  logs,
  logStatus,
  logStatusLabel,
  logStreamPaused,
  server,
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
  logs: string[];
  logStatus: "idle" | "connecting" | "connected" | "error" | "paused";
  logStatusLabel: string;
  logStreamPaused: boolean;
  server: Server;
  viewportRef: React.RefObject<HTMLDivElement | null>;
  onChangeCommand: (value: string) => void;
  onClear: () => void;
  onQuickCommand: (value: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onTogglePause: () => void;
}) {
  const { t } = useI18n();
  const consoleEnabled = server.status === "running";
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
        <ConsoleCommandPanel commandPending={commandPending} disabled={!consoleEnabled} onRun={onQuickCommand} />
        <form className="flex items-center gap-2 border-t border-panel-line bg-slate-950/70 px-3 py-3" onSubmit={onSubmit}>
          <span className={consoleEnabled ? "font-mono text-sm text-panel-green" : "font-mono text-sm text-slate-600"}>$</span>
          <input
            className="h-9 min-w-0 flex-1 bg-transparent font-mono text-sm text-slate-100 outline-none placeholder:text-slate-600 disabled:cursor-not-allowed disabled:text-slate-600"
            placeholder={consoleEnabled ? t("consoleReady") : t("consoleRequiresRunning")}
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
  commandPending,
  disabled,
  onRun
}: {
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
    { key: "kick", label: t("consoleActionKick"), command: "kick", icon: <UserX aria-hidden="true" className="size-3.5" />, placeholder: t("consoleActionPlayerPlaceholder"), danger: true },
    { key: "ban", label: t("consoleActionBan"), command: "ban", icon: <Ban aria-hidden="true" className="size-3.5" />, placeholder: t("consoleActionPlayerPlaceholder"), danger: true },
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
          <CommandGroup title={t("consoleWorldGroup")}>
            <QuickCommandButton disabled={blocked} icon={<Sunrise aria-hidden="true" className="size-3.5" />} label={t("consoleActionDawn")} onClick={() => runSimple(t("consoleActionDawn"), "dawn")} />
            <QuickCommandButton disabled={blocked} icon={<Sun aria-hidden="true" className="size-3.5" />} label={t("consoleActionNoon")} onClick={() => runSimple(t("consoleActionNoon"), "noon")} />
            <QuickCommandButton disabled={blocked} icon={<Moon aria-hidden="true" className="size-3.5" />} label={t("consoleActionDusk")} onClick={() => runSimple(t("consoleActionDusk"), "dusk")} />
            <QuickCommandButton disabled={blocked} icon={<Moon aria-hidden="true" className="size-3.5" />} label={t("consoleActionMidnight")} onClick={() => runSimple(t("consoleActionMidnight"), "midnight")} />
            <QuickCommandButton disabled={blocked} icon={<Waves aria-hidden="true" className="size-3.5" />} label={t("consoleActionSettle")} onClick={() => runSimple(t("consoleActionSettle"), "settle")} />
          </CommandGroup>
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
  restartPending,
  saveError,
  savePending,
  saveSuccess,
  server
}: {
  onRestart: () => void;
  onSave: (config: TerrariaConfig, hostPort: number) => void;
  restartPending: boolean;
  saveError: string;
  savePending: boolean;
  saveSuccess: boolean;
  server: Server;
}) {
  const { t } = useI18n();
  const [draft, setDraft] = useState<TerrariaConfig>(server.config);
  const [hostPortDraft, setHostPortDraft] = useState(serverJoinPort(server));
  const [previewOpen, setPreviewOpen] = useState(false);
  const [restartRecommended, setRestartRecommended] = useState(false);
  useEffect(() => setDraft(server.config), [server.config, server.id]);
  useEffect(() => setHostPortDraft(serverJoinPort(server)), [server.hostPort, server.id, server.port]);
  const normalizedDraft = useMemo(() => ({ ...draft, port: terrariaInternalPort }), [draft]);
  const preview = useQuery({
    queryKey: ["server-config-preview", server.id, normalizedDraft],
    queryFn: () => previewTerrariaConfig(normalizedDraft),
    enabled: previewOpen,
    retry: false
  });
  const configDirty = JSON.stringify(normalizedDraft) !== JSON.stringify({ ...server.config, port: terrariaInternalPort });
  const hostPortDirty = hostPortDraft !== serverJoinPort(server);
  const dirty = configDirty || hostPortDirty;
  const lifecycleLocked = isServerLifecyclePending(server.status);
  const running = server.status === "running";
  const disabled = lifecycleLocked || savePending;
  const restartRequired = running && !dirty && (server.configPendingRestart || restartRecommended);
  const showConfigActions = dirty || savePending || saveSuccess || restartRequired || lifecycleLocked;
  const update = <K extends keyof TerrariaConfig>(key: K, value: TerrariaConfig[K]) => setDraft((current) => ({ ...current, [key]: value }));
  useEffect(() => {
    if (dirty || !running || !server.configPendingRestart) {
      setRestartRecommended(false);
    }
  }, [dirty, running, server.configPendingRestart]);
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
  return (
    <form className="space-y-4" onSubmit={(event) => {
      event.preventDefault();
      if (!disabled && dirty) onSave(normalizedDraft, hostPortDraft);
    }}>
      <div className="rounded-lg border border-panel-line bg-slate-950/40 p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="font-semibold">{t("serverConfig")}</h2>
            {lifecycleLocked && <span className="mt-1 inline-block rounded bg-panel-gold/15 px-2 py-1 text-xs text-panel-gold">{t("configLifecycleLocked")}</span>}
          </div>
          <Button type="button" variant="secondary" className="h-8 px-2 text-xs" onClick={() => setPreviewOpen(true)}>
            <FileText aria-hidden="true" className="size-3.5" />
            {t("showPreview")}
          </Button>
        </div>
        <div className="mt-4 grid gap-4 lg:grid-cols-2">
          <div className="space-y-3">
            <Field label={t("serverName")}>
              <Input value={draft.serverName ?? ""} onChange={(event) => update("serverName", event.target.value)} disabled={disabled} />
            </Field>
            <Field label={t("password")}>
              <Input value={draft.password ?? ""} onChange={(event) => update("password", event.target.value)} disabled={disabled} />
            </Field>
            <Field label={t("motd")}>
              <Input value={draft.motd ?? ""} onChange={(event) => update("motd", event.target.value)} disabled={disabled} />
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
      </div>

      <div className="rounded-lg border border-panel-line bg-slate-950/40 p-4">
        <h2 className="font-semibold">{t("worldCreationSettings")}</h2>
        <p className="mt-1 text-xs text-slate-500">{t("worldCreationReadonlyHint")}</p>
        <div className="mt-4 grid gap-4 lg:grid-cols-2">
          <ReadOnlyField label={t("worldName")} value={draft.worldName} />
          <ReadOnlyField label={t("gameVersion")} value={server.version || "1.4.5.6"} />
          <ReadOnlyField label={t("worldSize")} value={worldSizeLabel} />
          <ReadOnlyField label={t("worldEvil")} value={worldEvilLabel} />
          <ReadOnlyField label={t("difficulty")} value={difficultyLabel} />
          <ReadOnlyField label={t("internalPort")} value={String(terrariaInternalPort)} />
          <ReadOnlyField label={t("customSeed")} value={seedLabel} />
        </div>
      </div>

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
                setDraft(server.config);
                setHostPortDraft(serverJoinPort(server));
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
      {previewOpen && (
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

function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid gap-1.5">
      <span className="text-xs font-medium text-slate-500">{label}</span>
      <div className="flex h-10 items-center rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-400">{value}</div>
    </div>
  );
}

function Field({ children, label }: { children: ReactNode; label: string }) {
  return (
    <label className="grid gap-1.5">
      <span className="text-xs font-medium text-slate-500">{label}</span>
      {children}
    </label>
  );
}

function Checkbox({ checked, disabled, label, onChange }: { checked: boolean; disabled?: boolean; label: string; onChange: (checked: boolean) => void }) {
  return (
    <label className="flex items-center justify-between gap-3 text-sm text-slate-300">
      <span>{label}</span>
      <input
        className="size-4 accent-panel-green disabled:cursor-not-allowed"
        checked={checked}
        disabled={disabled}
        type="checkbox"
        onChange={(event) => onChange(event.target.checked)}
      />
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
  serverStatus: Server["status"];
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
      <div className="grid gap-3 xl:grid-cols-2">
        {items.map((backup) => (
          <div key={backup.id} className="rounded-lg border border-panel-line bg-slate-950/35 p-4">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="flex min-w-0 items-center gap-2">
                  <Link href={`/backups/${backup.id}`} className="truncate font-medium text-white transition hover:text-panel-green">{backup.name}</Link>
                  <span className={cn("shrink-0 rounded px-2 py-0.5 text-xs font-medium", backup.type === "Auto" ? "bg-slate-800 text-slate-300" : "bg-panel-gold/15 text-panel-gold")}>
                    {backup.type === "Auto" ? t("typeAuto") : t("typeManual")}
                  </span>
                </div>
                <p className="mt-1 truncate text-sm text-slate-500">{backup.world}</p>
              </div>
              <span className="shrink-0 text-xs text-slate-500">{localizeRelativeTime(backup.created, locale)}</span>
            </div>
            <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-panel-line pt-3">
              <span className="text-sm font-medium text-slate-300">{backup.size}</span>
              <div className="flex flex-wrap gap-2">
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
          </div>
        ))}
      </div>
    </ResourcePanel>
  );
}

function ModsTab({
  availableMods,
  assigning,
  deleting,
  importingWorkshop,
  isError,
  isLoading,
  items,
  libraryError,
  modPacks,
  packInstalling,
  serverStatus,
  toggling,
  uploading,
  workshopIdsCount,
  workshopIdsText,
  onAssignMod,
  onDelete,
  onImportWorkshop,
  onInstallPack,
  onToggle,
  onUploadClick,
  onWorkshopIdsChange
}: {
  availableMods: ModFile[];
  assigning: boolean;
  deleting: boolean;
  importingWorkshop: boolean;
  isError: boolean;
  isLoading: boolean;
  items: ModFile[];
  libraryError: boolean;
  modPacks: ModPack[];
  packInstalling: boolean;
  serverStatus: Server["status"];
  toggling: boolean;
  uploading: boolean;
  workshopIdsCount: number;
  workshopIdsText: string;
  onAssignMod: (mod: ModFile) => void;
  onDelete: (mod: ModFile) => void;
  onImportWorkshop: () => void;
  onInstallPack: (pack: ModPack) => void;
  onToggle: (mod: ModFile) => void;
  onUploadClick: () => void;
  onWorkshopIdsChange: (value: string) => void;
}) {
  const { locale, t } = useI18n();
  const modAction = describeResourceAction({ kind: "modifyMods", serverStatus });
  const blocked = modAction.disabled;
  return (
    <ResourcePanel
      title={t("detailModActions")}
      href="/mods"
      action={
        <Button variant="secondary" onClick={onUploadClick} disabled={uploading || blocked} title={modAction.reasonKey ? t(modAction.reasonKey) : undefined}>
          <Upload aria-hidden="true" />
          {uploading ? t("uploading") : t("uploadMod")}
        </Button>
      }
    >
      <div className="space-y-4">
        {modAction.reasonKey ? <p className="text-sm text-panel-gold">{t(modAction.reasonKey)}</p> : null}
        {libraryError ? <p className="text-sm text-panel-gold">{t("modsApiUnavailable")}</p> : null}

        <div className="rounded-lg border border-panel-line bg-slate-950/35 p-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 className="font-semibold text-white">{t("serverMods")}</h3>
              <p className="mt-1 text-sm text-slate-500">{t("serverModsHint")}</p>
            </div>
          </div>
          {isError ? <p className="mt-4 text-sm text-panel-gold">{t("modsApiUnavailable")}</p> : null}
          {!isError && isLoading ? <p className="mt-4 text-sm text-slate-400">{t("loading")}</p> : null}
          {!isError && !isLoading && items.length === 0 ? <p className="mt-4 text-sm text-slate-400">{t("noModsUploaded")}</p> : null}
          <div className="mt-4 grid gap-2">
            {items.map((mod) => (
              <ResourceRow
                key={mod.id}
                title={<Link href={`/mods/${mod.id}`} className="transition hover:text-panel-green">{mod.fileName}</Link>}
                meta={`${mod.size} · ${mod.enabled ? t("enabled") : t("disabled")} · ${localizeRelativeTime(mod.created, locale)}`}
                actions={
                  <>
                    <Button variant="secondary" onClick={() => onToggle(mod)} disabled={toggling || blocked} title={modAction.reasonKey ? t(modAction.reasonKey) : undefined}>
                      <Power aria-hidden="true" />
                      {mod.enabled ? t("disable") : t("enable")}
                    </Button>
                    <Button variant="danger" aria-label={t("delete")} onClick={() => onDelete(mod)} disabled={deleting || blocked} title={modAction.reasonKey ? t(modAction.reasonKey) : undefined}>
                      <Trash2 aria-hidden="true" />
                    </Button>
                  </>
                }
              />
            ))}
          </div>
        </div>

        <details className="rounded-lg border border-panel-line bg-slate-950/35 p-4">
          <summary className="cursor-pointer select-none text-sm font-semibold text-slate-100 outline-none transition hover:text-panel-green focus-visible:ring-2 focus-visible:ring-panel-green/50">
            {t("installOptions")}
            <span className="ml-2 font-normal text-slate-500">{t("installOptionsHint")}</span>
          </summary>
          <div className="mt-4 space-y-4">
            <div>
              <h3 className="font-semibold text-white">{t("installFromLibrary")}</h3>
              <p className="mt-1 text-sm text-slate-500">{t("installFromLibraryHint")}</p>
              <div className="mt-4 grid gap-2 xl:grid-cols-2">
                {availableMods.map((mod) => (
                  <ResourceRow
                    key={mod.id}
                    title={<Link href={`/mods/${mod.id}`} className="transition hover:text-panel-green">{mod.fileName}</Link>}
                    meta={`${mod.size} · ${localizeRelativeTime(mod.created, locale)}`}
                    actions={
                      <Button variant="secondary" onClick={() => onAssignMod(mod)} disabled={assigning || blocked} title={modAction.reasonKey ? t(modAction.reasonKey) : undefined}>
                        <Package aria-hidden="true" />
                        {t("installToServer")}
                      </Button>
                    }
                  />
                ))}
                {availableMods.length === 0 && <p className="text-sm text-slate-500 xl:col-span-2">{t("noGlobalMods")}</p>}
              </div>
            </div>

            <div className="grid gap-4 xl:grid-cols-2">
              <div className="rounded-md border border-panel-line bg-slate-950/45 p-4">
                <h3 className="font-semibold text-white">{t("modPacks")}</h3>
                <p className="mt-1 text-sm text-slate-500">{t("installModPacksHint")}</p>
                <div className="mt-4 grid gap-2">
                  {modPacks.map((pack) => (
                    <ResourceRow
                      key={pack.id}
                      title={<Link href={`/mods/packs/${pack.id}`} className="transition hover:text-panel-green">{pack.name}</Link>}
                      meta={`${pack.mods.length} · ${pack.description || pack.mods.map((mod) => mod.fileName).join(", ")}`}
                      actions={
                        <Button variant="secondary" onClick={() => onInstallPack(pack)} disabled={packInstalling || blocked || pack.modIds.length === 0} title={modAction.reasonKey ? t(modAction.reasonKey) : undefined}>
                          <Package aria-hidden="true" />
                          {t("installModPack")}
                        </Button>
                      }
                    />
                  ))}
                  {modPacks.length === 0 && <p className="text-sm text-slate-500">{t("noModPacks")}</p>}
                </div>
              </div>

              <div className="rounded-md border border-panel-line bg-slate-950/45 p-4">
                <h3 className="font-semibold text-white">{t("importWorkshopMods")}</h3>
                <p className="mt-1 text-sm text-slate-500">{t("workshopImportHint")}</p>
                <textarea
                  className="mt-4 min-h-24 w-full resize-none rounded-md border border-panel-line bg-slate-950 px-3 py-2 text-sm text-slate-100 outline-none placeholder:text-slate-600 focus:border-panel-green"
                  placeholder={t("workshopIdsPlaceholder")}
                  value={workshopIdsText}
                  onChange={(event) => onWorkshopIdsChange(event.target.value)}
                  disabled={importingWorkshop || blocked}
                />
                <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
                  <span className="text-xs text-slate-500">{t("workshopIdsSelected", { count: workshopIdsCount })}</span>
                  <Button variant="secondary" onClick={onImportWorkshop} disabled={importingWorkshop || blocked || workshopIdsCount === 0} title={modAction.reasonKey ? t(modAction.reasonKey) : undefined}>
                    <Download aria-hidden="true" />
                    {importingWorkshop ? t("actionWorking") : t("importWorkshopMods")}
                  </Button>
                </div>
              </div>
            </div>
          </div>
        </details>
      </div>
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

function ResourceRow({ title, meta, actions }: { title: ReactNode; meta: string; actions?: ReactNode }) {
  return (
    <div className="flex flex-col gap-3 rounded-md border border-panel-line bg-slate-950/50 px-3 py-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <div className="truncate text-sm font-medium">{title}</div>
        <p className="mt-1 text-xs text-slate-500">{meta}</p>
      </div>
      {actions && <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>}
    </div>
  );
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

function HeaderMetric({ accent = false, active = true, icon, label, value }: { accent?: boolean; active?: boolean; icon: ReactNode; label: string; value: string }) {
  const highlighted = accent && active;
  return (
    <span className={cn(
      "inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs",
      highlighted ? "border-panel-green/25 bg-panel-green/10 text-slate-200" : "border-panel-line bg-slate-950/50 text-slate-400"
    )}>
      <span className={highlighted ? "text-panel-green" : "text-slate-500"}>{icon}</span>
      <span className="text-slate-500">{label}</span>
      <span className={cn("font-mono font-medium", active ? "text-slate-100" : "text-slate-500")}>{value}</span>
    </span>
  );
}

function difficultyLabel(value: Server["config"]["difficulty"], t: ReturnType<typeof useI18n>["t"]) {
  const labels = {
    journey: t("tagJourney"),
    classic: t("tagClassic"),
    expert: t("tagExpert"),
    master: t("tagMaster")
  };
  return labels[value] ?? value;
}

function worldSizeLabel(value: Server["config"]["worldSize"], t: ReturnType<typeof useI18n>["t"]) {
  const labels = {
    small: t("tagSmallWorld"),
    medium: t("tagMediumWorld"),
    large: t("tagLargeWorld")
  };
  return labels[value] ?? value;
}

function parseWorkshopIds(value: string) {
  const seen = new Set<string>();
  return value
    .split(/[\s,，]+/)
    .map((item) => item.trim())
    .filter((item) => {
      if (!item || seen.has(item)) return false;
      seen.add(item);
      return true;
    });
}
