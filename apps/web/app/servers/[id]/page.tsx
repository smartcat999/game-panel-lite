"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, CheckCircle2, Copy, Cpu, Download, FileText, MemoryStick, MoveRight, Package, Plus, Power, RotateCcw, Terminal, Trash2, Upload } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from "react";
import type { TerrariaConfig } from "@gamepanel-lite/shared";
import { secretSeedKeyFor, terrariaSecretSeeds } from "@gamepanel-lite/shared";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { ServerActions } from "@/components/server-actions";
import { ServerModeBadge, ServerStatusBadge } from "@/components/server-badges";
import { Button, Card, Input } from "@/components/ui";
import {
  assignWorld,
  createBackup,
  deleteBackup,
  deleteMod,
  deleteWorld,
  downloadBackupFile,
  downloadWorldFile,
  duplicateWorld,
  getServer,
  getServerStats,
  importWorld,
  listBackups,
  listServers,
  listMods,
  listWorlds,
  migrateBackup,
  migrateWorld,
  previewTerrariaConfig,
  restoreBackup,
  sendServerCommand,
  setModEnabled,
  serverLogsUrl,
  updateServerConfig,
  uploadMod,
} from "@/lib/api";
import { saveBlob } from "@/lib/download";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import { getDetailTargetServers, nextWorldCopyName } from "@/lib/server-detail-resources";
import { serverInviteText, serverJoinPort } from "@/lib/server-join";
import { cn } from "@/lib/utils";
import type { Backup, ModFile, Server, World } from "@/lib/types";

type TabId = "overview" | "console" | "logs" | "config" | "worlds" | "backups" | "mods";

export default function ServerDetailPage() {
  const { t } = useI18n();
  const params = useParams<{ id: string }>();
  const id = params.id;
  const client = useQueryClient();
  const worldInputRef = useRef<HTMLInputElement>(null);
  const modInputRef = useRef<HTMLInputElement>(null);
  const logViewportRef = useRef<HTMLDivElement>(null);
  const logServerIdRef = useRef("");

  const query = useQuery({ queryKey: ["server", id], queryFn: () => getServer(id), retry: false, refetchInterval: 5000 });
  const server = query.data;
  const statsQuery = useQuery({ queryKey: ["server-stats", id], queryFn: () => getServerStats(id), enabled: server?.status === "running", refetchInterval: 3000, retry: false });
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: listServers, enabled: Boolean(server), retry: false });
  const worldsQuery = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, enabled: Boolean(server), retry: false });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, enabled: Boolean(server), retry: false });
  const modsQuery = useQuery({
    queryKey: ["mods", id],
    queryFn: () => listMods(id),
    enabled: Boolean(server && server.mode === "tmodloader"),
    retry: false
  });
  const [activeTab, setActiveTab] = useState<TabId>("overview");
  const [copied, setCopied] = useState("");
  const [logs, setLogs] = useState<string[]>([]);
  const [command, setCommand] = useState("");
  const [errorMessage, setErrorMessage] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [consoleError, setConsoleError] = useState("");
  const [pendingWorldDelete, setPendingWorldDelete] = useState<World | null>(null);
  const [pendingRestore, setPendingRestore] = useState<Backup | null>(null);
  const [pendingBackupDelete, setPendingBackupDelete] = useState<Backup | null>(null);
  const [pendingModDelete, setPendingModDelete] = useState<ModFile | null>(null);
  const [downloadingResourceId, setDownloadingResourceId] = useState("");
  const [targetServerId, setTargetServerId] = useState("");
  const [logStatus, setLogStatus] = useState<"idle" | "connecting" | "connected" | "error" | "paused">("idle");
  const [logStreamPaused, setLogStreamPaused] = useState(false);
  const [configSaved, setConfigSaved] = useState(false);
  const successTimerRef = useRef<number | null>(null);

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
      showSuccess(t("commandSent"));
    },
    onError: (error) => {
      setSuccessMessage("");
      setConsoleError(error instanceof Error ? error.message : t("commandSendFailed"));
    }
  });
  const worldUpload = useMutation({
    mutationFn: (file: File) => importWorld(file, id),
    onSuccess: async () => {
      showSuccess(t("worldImported"));
      if (worldInputRef.current) worldInputRef.current.value = "";
      await client.invalidateQueries({ queryKey: ["worlds"] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("unableImportWorld"))
  });
  const configSave = useMutation({
    mutationFn: (nextConfig: TerrariaConfig) => updateServerConfig(id, nextConfig),
    onSuccess: async (updatedServer) => {
      showSuccess(t("configSaved"));
      setConfigSaved(true);
      client.setQueryData(["server", id], updatedServer);
      await client.invalidateQueries({ queryKey: ["servers"] });
      window.setTimeout(() => setConfigSaved(false), 1800);
    },
    onError: (error) => {
      setConfigSaved(false);
      showError(error instanceof Error ? error.message : t("unableUpdateConfig"));
    }
  });
  const worldAssign = useMutation({
    mutationFn: (worldId: string) => assignWorld(worldId, id),
    onSuccess: async () => {
      showSuccess(t("worldAssigned"));
      await client.invalidateQueries({ queryKey: ["worlds"] });
      await client.invalidateQueries({ queryKey: ["server", id] });
      await client.invalidateQueries({ queryKey: ["servers"] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("unableAssignWorld"))
  });
  const worldDuplicate = useMutation({
    mutationFn: (world: World) => duplicateWorld(world.id, nextWorldCopyName(world.name, t("duplicateSuffix"))),
    onSuccess: async () => {
      showSuccess(t("worldDuplicated"));
      await client.invalidateQueries({ queryKey: ["worlds"] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("unableDuplicateWorld"))
  });
  const worldMigrate = useMutation({
    mutationFn: ({ id: worldId, instanceId }: { id: string; instanceId: string }) => migrateWorld(worldId, instanceId),
    onSuccess: async () => {
      showSuccess(t("worldMigrated"));
      await client.invalidateQueries({ queryKey: ["worlds"] });
      await client.invalidateQueries({ queryKey: ["servers"] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("unableMigrateWorld"))
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
      showError(message.includes("active world") ? t("unableDeleteActiveWorld") : message || t("unableDeleteWorld"));
    }
  });
  const backupCreate = useMutation({
    mutationFn: () => createBackup(id),
    onSuccess: async () => {
      showSuccess(t("backupCreated"));
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("unableCreateBackup"))
  });
  const backupRestore = useMutation({
    mutationFn: restoreBackup,
    onSuccess: async () => {
      showSuccess(t("backupRestored"));
      setPendingRestore(null);
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("unableRestoreBackup"))
  });
  const backupMigrate = useMutation({
    mutationFn: ({ id: backupId, instanceId }: { id: string; instanceId: string }) => migrateBackup(backupId, instanceId),
    onSuccess: async () => {
      showSuccess(t("backupMigrated"));
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("unableMigrateBackup"))
  });
  const backupDelete = useMutation({
    mutationFn: deleteBackup,
    onSuccess: async () => {
      showSuccess(t("backupDeleted"));
      setPendingBackupDelete(null);
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("unableDeleteBackup"))
  });
  const modUpload = useMutation({
    mutationFn: (file: File) => uploadMod(id, file),
    onSuccess: async () => {
      showSuccess(t("modUploaded"));
      if (modInputRef.current) modInputRef.current.value = "";
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("unableUploadMod"))
  });
  const modEnabled = useMutation({
    mutationFn: ({ modId, enabled }: { modId: string; enabled: boolean }) => setModEnabled(id, modId, enabled),
    onSuccess: async (updatedMod) => {
      showSuccess(updatedMod.enabled ? t("modEnabled") : t("modDisabled"));
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("unableUpdateMod"))
  });
  const modDelete = useMutation({
    mutationFn: (modId: string) => deleteMod(id, modId),
    onSuccess: async () => {
      showSuccess(t("modDeleted"));
      setPendingModDelete(null);
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => showError(error instanceof Error ? error.message : t("unableDeleteMod"))
  });

  useEffect(() => {
    if (!id || (activeTab !== "console" && activeTab !== "logs")) return;
    if (server?.status !== "running") {
      setLogStatus("idle");
      setLogs([]);
      return;
    }
    if (logStreamPaused) {
      setLogStatus("paused");
      return;
    }
    if (logServerIdRef.current !== id) {
      logServerIdRef.current = id;
      setLogs([]);
    }
    setLogStatus("connecting");
    const source = new EventSource(serverLogsUrl(id));
    source.onopen = () => setLogStatus("connected");
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
  }, [activeTab, id, server?.status, logStreamPaused]);

  useEffect(() => {
    const viewport = logViewportRef.current;
    if (viewport) viewport.scrollTop = viewport.scrollHeight;
  }, [logs, activeTab]);

  const serverWorlds = useMemo(
    () => (server ? (worldsQuery.data ?? []).filter((world) => world.server === server.id || world.name === server.world) : []),
    [server, worldsQuery.data]
  );
  const serverBackups = useMemo(
    () => (server ? (backupsQuery.data ?? []).filter((backup) => backup.instanceId === server.id) : []),
    [backupsQuery.data, server]
  );
  const serverMods = useMemo(() => modsQuery.data ?? [], [modsQuery.data]);
  const targetServers = useMemo(() => (server ? getDetailTargetServers(serversQuery.data ?? [], server.id) : []), [server, serversQuery.data]);
  const activeTargetServerId = targetServerId || targetServers[0]?.id || "";

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
    const value = command.trim();
    if (!value || commandMutation.isPending) return;
    commandMutation.mutate(value);
  };
  const downloadWorld = async (world: World) => {
    setDownloadingResourceId(world.id);
    try {
      const blob = await downloadWorldFile(world.id);
      saveBlob(blob, `${world.name}.wld`);
      showSuccess(t("downloadStarted"));
    } catch (error) {
      showError(error instanceof Error ? error.message : t("unableDownloadWorld"));
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
      showError(error instanceof Error ? error.message : t("unableDownloadBackup"));
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
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold">{server.name}</h1>
            <ServerModeBadge mode={server.mode} />
            <ServerStatusBadge status={server.status} />
          </div>
          <p className="mt-2 text-sm text-slate-400">
            {t("world")}: {server.world}
          </p>
        </div>
        <ServerActions server={server} />
      </div>

      <div className="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
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
              logStatus={logStatus}
              logStatusLabel={logStatusLabel}
              logStreamPaused={logStreamPaused}
              logs={logs}
              server={server}
              viewportRef={logViewportRef}
              onChangeCommand={(value) => {
                setCommand(value);
                setConsoleError("");
              }}
              onTogglePause={() => setLogStreamPaused((current) => !current)}
              onSubmit={submitCommand}
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
              server={server}
              onSave={(nextConfig) => configSave.mutate(nextConfig)}
            />
          )}
          {activeTab === "worlds" && (
            <WorldsTab
              isError={worldsQuery.isError}
              isLoading={worldsQuery.isLoading}
              items={serverWorlds}
              deleting={worldDelete.isPending}
              assigning={worldAssign.isPending}
              duplicating={worldDuplicate.isPending}
              currentWorldName={server.world}
              serverStatus={server.status}
              downloadingId={downloadingResourceId}
              migrating={worldMigrate.isPending}
              targetServerId={activeTargetServerId}
              targetServers={targetServers}
              uploading={worldUpload.isPending}
              onAssign={(world) => worldAssign.mutate(world.id)}
              onDelete={setPendingWorldDelete}
              onDownload={(world) => void downloadWorld(world)}
              onDuplicate={(world) => worldDuplicate.mutate(world)}
              onMigrate={(world) => activeTargetServerId && worldMigrate.mutate({ id: world.id, instanceId: activeTargetServerId })}
              onTargetServerChange={setTargetServerId}
              onUploadClick={() => worldInputRef.current?.click()}
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
              migrating={backupMigrate.isPending}
              restoring={backupRestore.isPending}
              serverStatus={server.status}
              targetServerId={activeTargetServerId}
              targetServers={targetServers}
              onDelete={setPendingBackupDelete}
              onDownload={(backup) => void downloadBackup(backup)}
              onCreate={() => backupCreate.mutate()}
              onMigrate={(backup) => activeTargetServerId && backupMigrate.mutate({ id: backup.id, instanceId: activeTargetServerId })}
              onRestore={setPendingRestore}
              onTargetServerChange={setTargetServerId}
            />
          )}
          {activeTab === "mods" && server.mode === "tmodloader" && (
            <ModsTab
              deleting={modDelete.isPending}
              isError={modsQuery.isError}
              isLoading={modsQuery.isLoading}
              items={serverMods}
              toggling={modEnabled.isPending}
              uploading={modUpload.isPending}
              onDelete={setPendingModDelete}
              onToggle={(mod) => modEnabled.mutate({ modId: mod.id, enabled: !mod.enabled })}
              onUploadClick={() => modInputRef.current?.click()}
            />
          )}
        </Card>

        <div className="flex flex-col gap-4">
          <Card className="p-4">
            <h2 className="font-semibold">{t("joinServer")}</h2>
            <CopyRow label={t("ipAddress")} value="127.0.0.1" copied={copied} copiedLabel={t("copied")} copyLabel={t("copy")} onCopy={copy} />
            <CopyRow label={t("port")} value={String(serverJoinPort(server))} copied={copied} copiedLabel={t("copied")} copyLabel={t("copy")} onCopy={copy} />
            <CopyRow label={t("password")} value={server.password || t("none")} copied={copied} copiedLabel={t("copied")} copyLabel={t("copy")} onCopy={copy} />
            <Button className="mt-4 w-full" onClick={() => void copy("Invite", invite)}>
              <Copy aria-hidden="true" />
              {copied === "Invite" ? t("copied") : t("copyInviteText")}
            </Button>
          </Card>
          <Card className="p-4">
            <h2 className="font-semibold">{t("serverInfo")}</h2>
            <Info label={t("world")} value={server.world} />
            <Info label={t("difficulty")} value={difficultyLabel(server.config.difficulty, t)} />
            <Info label={t("worldSize")} value={worldSizeLabel(server.config.worldSize, t)} />
            <Info label={t("maxPlayers")} value={String(server.maxPlayers)} />
            <Info label={t("version")} value={server.version} />
            {server.hostPort > 0 && server.hostPort !== server.port && (
              <Info label={t("hostPort")} value={String(server.hostPort)} />
            )}
          </Card>
          {server.status === "running" && (
            <Card className="p-4">
              <h2 className="font-semibold">{t("resourceUsage")}</h2>
              <div className="mt-3 flex flex-col gap-3">
                <ResourceBar icon={<Cpu aria-hidden="true" className="size-4 text-panel-green" />} label={t("cpu")} value={statsQuery.data?.cpuPercent ?? 0} suffix="%" />
                <ResourceBar icon={<MemoryStick aria-hidden="true" className="size-4 text-panel-purple" />} label={t("memory")} value={memoryPercent(statsQuery.data)} suffix="" displayText={memoryDisplay(statsQuery.data)} />
              </div>
            </Card>
          )}
        </div>
      </div>

      <input
        ref={worldInputRef}
        className="hidden"
        type="file"
        accept=".wld"
        onChange={(event) => {
          const file = event.target.files?.[0];
          if (file) worldUpload.mutate(file);
        }}
      />
      <input
        ref={modInputRef}
        className="hidden"
        type="file"
        accept=".tmod,.txt,.json"
        onChange={(event) => {
          const file = event.target.files?.[0];
          if (file) modUpload.mutate(file);
        }}
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
  return (
    <div className="grid gap-3 md:grid-cols-3">
      <SummaryButton icon={<FileText aria-hidden="true" />} label={t("tabWorlds")} value={String(worldCount)} onClick={() => onSelectTab("worlds")} />
      <SummaryButton icon={<Archive aria-hidden="true" />} label={t("tabBackups")} value={String(backupCount)} onClick={() => onSelectTab("backups")} />
      {server.mode === "tmodloader" && <SummaryButton icon={<Package aria-hidden="true" />} label={t("tabMods")} value={String(modCount)} onClick={() => onSelectTab("mods")} />}
    </div>
  );
}

function ConsoleTab({
  command,
  commandPending,
  consoleError,
  logStatus,
  logStatusLabel,
  logStreamPaused,
  logs,
  server,
  viewportRef,
  onChangeCommand,
  onTogglePause,
  onSubmit
}: {
  command: string;
  commandPending: boolean;
  consoleError: string;
  logStatus: "idle" | "connecting" | "connected" | "error" | "paused";
  logStatusLabel: string;
  logStreamPaused: boolean;
  logs: string[];
  server: Server;
  viewportRef: React.RefObject<HTMLDivElement | null>;
  onChangeCommand: (value: string) => void;
  onTogglePause: () => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const { t } = useI18n();
  return (
    <div>
      <LogHeader
        action={<Button variant="secondary" className="px-2 py-1 text-xs" onClick={onTogglePause} disabled={logStatus !== "connected" && logStatus !== "paused"}>{logStreamPaused ? t("resumeLogs") : t("pauseLogs")}</Button>}
        logStatus={logStatus}
        logStatusLabel={logStatusLabel}
        title={t("liveLogs")}
      />
      <LogViewport logs={logs} logStatus={logStatus} viewportRef={viewportRef} />
      {consoleError && <p className="mt-3 rounded-md border border-panel-gold/30 bg-panel-gold/10 px-3 py-2 text-sm text-panel-gold">{consoleError}</p>}
      <form className="mt-3 flex gap-2" onSubmit={onSubmit}>
        <Input
          placeholder={server.status === "running" ? t("enterCommand") : t("consoleRequiresRunning")}
          value={command}
          onChange={(event) => onChangeCommand(event.target.value)}
          disabled={server.status !== "running" || commandPending}
        />
        <Button disabled={server.status !== "running" || command.trim() === "" || commandPending}>
          <Terminal aria-hidden="true" />
          {commandPending ? t("sending") : t("send")}
        </Button>
      </form>
    </div>
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
    <div>
      <LogHeader
        action={(
          <>
            <Button variant="secondary" className="px-2 py-1 text-xs" onClick={onTogglePause} disabled={logStatus !== "connected" && logStatus !== "paused"}>{logStreamPaused ? t("resumeLogs") : t("pauseLogs")}</Button>
            <Button variant="secondary" className="px-2 py-1 text-xs" onClick={onClear} disabled={logs.length === 0}>{t("clearLogs")}</Button>
          </>
        )}
        logStatus={logStatus}
        logStatusLabel={logStatusLabel}
        title={t("liveLogs")}
      />
      <LogViewport className="h-[520px]" logs={logs} logStatus={logStatus} viewportRef={viewportRef} />
    </div>
  );
}

function ConfigTab({
  onSave,
  saveError,
  savePending,
  saveSuccess,
  server
}: {
  onSave: (config: TerrariaConfig) => void;
  saveError: string;
  savePending: boolean;
  saveSuccess: boolean;
  server: Server;
}) {
  const { t } = useI18n();
  const [draft, setDraft] = useState<TerrariaConfig>(server.config);
  useEffect(() => setDraft(server.config), [server.config, server.id]);
  const preview = useQuery({
    queryKey: ["server-config-preview", server.id, draft],
    queryFn: () => previewTerrariaConfig(draft),
    retry: false
  });
  const dirty = JSON.stringify(draft) !== JSON.stringify(server.config);
  const disabled = server.status === "running" || savePending;
  const update = <K extends keyof TerrariaConfig>(key: K, value: TerrariaConfig[K]) => setDraft((current) => ({ ...current, [key]: value }));
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
      if (!disabled && dirty) onSave(draft);
    }}>
      <div className="rounded-lg border border-panel-line bg-slate-950/40 p-4">
        <h2 className="font-semibold">{t("serverConfig")}</h2>
        {server.status === "running" && <span className="mt-1 inline-block rounded bg-panel-gold/15 px-2 py-1 text-xs text-panel-gold">{t("configRequiresStopped")}</span>}
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
            <Field label={t("port")}>
              <Input type="number" min={1024} max={65535} value={draft.port} onChange={(event) => update("port", Number(event.target.value))} disabled={disabled} />
            </Field>
            <Field label={t("maxPlayers")}>
              <Input type="number" min={1} max={255} value={draft.maxPlayers} onChange={(event) => update("maxPlayers", Number(event.target.value))} disabled={disabled} />
            </Field>
            <Field label={t("languageSetting")}>
              <Input value={draft.language ?? ""} onChange={(event) => update("language", event.target.value)} disabled={disabled} />
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
          <ReadOnlyField label={t("customSeed")} value={seedLabel} />
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button disabled={disabled || !dirty}>
          {savePending ? t("savingConfig") : t("saveConfig")}
        </Button>
        <Button type="button" variant="secondary" disabled={savePending || !dirty} onClick={() => setDraft(server.config)}>
          {t("resetChanges")}
        </Button>
        {saveSuccess && <span className="text-sm text-panel-green">{t("configSaved")}</span>}
      </div>
      {saveError && <p className="rounded-md border border-panel-gold/30 bg-panel-gold/10 px-3 py-2 text-sm text-panel-gold">{saveError}</p>}

      <div className="rounded-md border border-panel-line bg-slate-950 p-4">
        <div className="mb-3 flex items-center gap-2 text-sm font-medium text-white">
          <FileText aria-hidden="true" className="size-4 text-panel-green" />
          {t("previewServerConfig")}
        </div>
        {preview.isLoading ? (
          <p className="text-sm text-slate-400">{t("rendering")}</p>
        ) : preview.isError ? (
          <p className="text-sm text-panel-gold">{t("configPreviewUnavailable")}</p>
        ) : (
          <pre className="max-h-[560px] overflow-auto whitespace-pre-wrap font-mono text-xs leading-6 text-slate-300">{preview.data}</pre>
        )}
      </div>
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
  assigning,
  currentWorldName,
  deleting,
  duplicating,
  downloadingId,
  isError,
  isLoading,
  items,
  onAssign,
  onDelete,
  onDownload,
  onDuplicate,
  onMigrate,
  onTargetServerChange,
  migrating,
  uploading,
  serverStatus,
  targetServerId,
  targetServers,
  onUploadClick
}: {
  assigning: boolean;
  currentWorldName: string;
  deleting: boolean;
  duplicating: boolean;
  downloadingId: string;
  isError: boolean;
  isLoading: boolean;
  items: World[];
  onAssign: (world: World) => void;
  onDelete: (world: World) => void;
  onDownload: (world: World) => void;
  onDuplicate: (world: World) => void;
  onMigrate: (world: World) => void;
  onTargetServerChange: (value: string) => void;
  migrating: boolean;
  uploading: boolean;
  serverStatus: Server["status"];
  targetServerId: string;
  targetServers: Server[];
  onUploadClick: () => void;
}) {
  const { locale, t } = useI18n();
  const canAssignWorld = serverStatus !== "running";
  return (
    <ResourcePanel
      title={t("detailWorldActions")}
      href="/worlds"
      action={
        <Button variant="secondary" onClick={onUploadClick} disabled={uploading}>
          <Upload aria-hidden="true" />
          {uploading ? t("importing") : t("importWorldForServer")}
        </Button>
      }
      target={
        <TargetServerSelect
          disabled={targetServers.length === 0}
          label={t("migrationTarget")}
          noTargetLabel={t("noOtherServers")}
          onChange={onTargetServerChange}
          servers={targetServers}
          value={targetServerId}
        />
      }
    >
      {isError ? <p className="text-sm text-panel-gold">{t("apiWorldsUnavailable")}</p> : null}
      {!isError && isLoading ? <p className="text-sm text-slate-400">{t("loading")}</p> : null}
      {!isError && !isLoading && items.length === 0 ? <p className="text-sm text-slate-400">{t("noWorldsYet")}</p> : null}
      <div className="grid gap-2">
        {items.map((world) => (
          <ResourceRow
            key={world.id}
            title={world.name}
            meta={`${world.bytes} · ${localizeRelativeTime(world.modified, locale)}`}
            actions={
              <>
                {world.name === currentWorldName ? (
                  <span className="inline-flex items-center gap-2 rounded-md border border-panel-green/30 bg-panel-green/10 px-3 py-2 text-sm font-medium text-panel-green">
                    <CheckCircle2 aria-hidden="true" className="size-4" />
                    {t("currentWorld")}
                  </span>
                ) : (
                  <Button
                    variant="secondary"
                    onClick={() => onAssign(world)}
                    disabled={!canAssignWorld || assigning}
                    title={!canAssignWorld ? t("assignWorldRequiresStopped") : undefined}
                  >
                    <CheckCircle2 aria-hidden="true" />
                    {assigning ? t("actionWorking") : t("setCurrentWorld")}
                  </Button>
                )}
                <Button variant="secondary" onClick={() => onDuplicate(world)} disabled={duplicating}>
                  <Plus aria-hidden="true" />
                  {duplicating ? t("actionWorking") : t("duplicate")}
                </Button>
                <Button variant="secondary" onClick={() => onMigrate(world)} disabled={targetServers.length === 0 || migrating}>
                  <MoveRight aria-hidden="true" />
                  {migrating ? t("actionWorking") : t("migrate")}
                </Button>
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
  onMigrate,
  restoring,
  migrating,
  serverStatus,
  targetServerId,
  targetServers,
  onCreate,
  onRestore,
  onTargetServerChange
}: {
  creating: boolean;
  deleting: boolean;
  downloadingId: string;
  isError: boolean;
  isLoading: boolean;
  items: Backup[];
  onDelete: (backup: Backup) => void;
  onDownload: (backup: Backup) => void;
  onMigrate: (backup: Backup) => void;
  restoring: boolean;
  migrating: boolean;
  serverStatus: Server["status"];
  targetServerId: string;
  targetServers: Server[];
  onCreate: () => void;
  onRestore: (backup: Backup) => void;
  onTargetServerChange: (value: string) => void;
}) {
  const { locale, t } = useI18n();
  const canRestore = serverStatus !== "running";
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
      target={
        <TargetServerSelect
          disabled={targetServers.length === 0}
          label={t("migrationTarget")}
          noTargetLabel={t("noOtherServers")}
          onChange={onTargetServerChange}
          servers={targetServers}
          value={targetServerId}
        />
      }
    >
      {isError ? <p className="text-sm text-panel-gold">{t("apiBackupsUnavailable")}</p> : null}
      {!isError && isLoading ? <p className="text-sm text-slate-400">{t("loading")}</p> : null}
      {!isError && !isLoading && items.length === 0 ? <p className="text-sm text-slate-400">{t("noBackupsYet")}</p> : null}
      <div className="grid gap-2">
        {items.map((backup) => (
          <ResourceRow
            key={backup.id}
            title={backup.name}
            meta={`${backup.world} · ${backup.size} · ${localizeRelativeTime(backup.created, locale)}`}
            actions={
              <>
                <Button
                  variant="secondary"
                  aria-label={t("restore")}
                  onClick={() => onRestore(backup)}
                  disabled={!canRestore || restoring}
                  title={!canRestore ? t("restoreRequiresStopped") : undefined}
                >
                  <RotateCcw aria-hidden="true" />
                </Button>
                <ActionButton
                  disabled={downloadingId === backup.id}
                  label={downloadingId === backup.id ? t("downloading") : t("download")}
                  icon={<Download aria-hidden="true" />}
                  onClick={() => onDownload(backup)}
                />
                <Button variant="secondary" aria-label={t("migrate")} onClick={() => onMigrate(backup)} disabled={targetServers.length === 0 || migrating}>
                  <MoveRight aria-hidden="true" />
                </Button>
                <Button variant="danger" aria-label={t("delete")} onClick={() => onDelete(backup)} disabled={deleting}>
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

function ModsTab({
  deleting,
  isError,
  isLoading,
  items,
  toggling,
  uploading,
  onDelete,
  onToggle,
  onUploadClick
}: {
  deleting: boolean;
  isError: boolean;
  isLoading: boolean;
  items: ModFile[];
  toggling: boolean;
  uploading: boolean;
  onDelete: (mod: ModFile) => void;
  onToggle: (mod: ModFile) => void;
  onUploadClick: () => void;
}) {
  const { locale, t } = useI18n();
  return (
    <ResourcePanel
      title={t("detailModActions")}
      href="/mods"
      action={
        <Button variant="secondary" onClick={onUploadClick} disabled={uploading}>
          <Upload aria-hidden="true" />
          {uploading ? t("uploading") : t("uploadMod")}
        </Button>
      }
    >
      {isError ? <p className="text-sm text-panel-gold">{t("modsApiUnavailable")}</p> : null}
      {!isError && isLoading ? <p className="text-sm text-slate-400">{t("loading")}</p> : null}
      {!isError && !isLoading && items.length === 0 ? <p className="text-sm text-slate-400">{t("noModsUploaded")}</p> : null}
      <div className="grid gap-2">
        {items.map((mod) => (
          <ResourceRow
            key={mod.id}
            title={mod.fileName}
            meta={`${mod.size} · ${mod.enabled ? t("enabled") : t("disabled")} · ${localizeRelativeTime(mod.created, locale)}`}
            actions={
              <>
                <Button variant="secondary" onClick={() => onToggle(mod)} disabled={toggling}>
                  <Power aria-hidden="true" />
                  {mod.enabled ? t("disable") : t("enable")}
                </Button>
                <Button variant="danger" aria-label={t("delete")} onClick={() => onDelete(mod)} disabled={deleting}>
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

function ResourcePanel({
  title,
  href,
  action,
  children,
  target
}: {
  title: string;
  href: string;
  action: ReactNode;
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
          {action}
          <Link href={href} className="inline-flex items-center justify-center rounded-md border border-panel-line bg-slate-900/70 px-3 py-2 text-sm font-medium text-slate-100 transition hover:bg-slate-800">
            {t("openFullManager")}
          </Link>
        </div>
      </div>
      {children}
    </div>
  );
}

function TargetServerSelect({
  disabled,
  label,
  noTargetLabel,
  onChange,
  servers,
  value
}: {
  disabled: boolean;
  label: string;
  noTargetLabel: string;
  onChange: (value: string) => void;
  servers: Server[];
  value: string;
}) {
  return (
    <label className="flex items-center gap-2 text-xs text-slate-500">
      <span className="hidden sm:inline">{label}</span>
      <select
        className="h-9 min-w-36 rounded-md border border-panel-line bg-slate-950/60 px-2 text-sm text-slate-100 outline-none focus:border-panel-green disabled:cursor-not-allowed disabled:opacity-50"
        disabled={disabled}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        {servers.length === 0 ? <option value="">{noTargetLabel}</option> : servers.map((server) => <option key={server.id} value={server.id}>{server.name}</option>)}
      </select>
    </label>
  );
}

function LogHeader({ action, logStatus, logStatusLabel, title }: { action?: ReactNode; logStatus: "idle" | "connecting" | "connected" | "error" | "paused"; logStatusLabel: string; title: string }) {
  return (
    <div className="mb-3 flex min-w-0 flex-1 items-center justify-between gap-2 rounded-md border border-panel-line bg-slate-950/50 px-3 py-2 text-xs">
      <span className="text-slate-400">{title}</span>
      <div className="flex shrink-0 items-center gap-2">
        <span className={logStatus === "connected" ? "text-panel-green" : logStatus === "error" ? "text-panel-gold" : "text-slate-400"}>{logStatusLabel}</span>
        {action}
      </div>
    </div>
  );
}

function LogViewport({
  className,
  logs,
  logStatus,
  viewportRef
}: {
  className?: string;
  logs: string[];
  logStatus: "idle" | "connecting" | "connected" | "error" | "paused";
  viewportRef: React.RefObject<HTMLDivElement | null>;
}) {
  const { t } = useI18n();
  return (
    <div ref={viewportRef} className={cn("h-[420px] overflow-auto rounded-md bg-slate-950 p-4 font-mono text-xs leading-6 text-slate-300", className)}>
      {logs.length === 0 ? (
        <p className="text-slate-500">{logStatus === "error" ? t("logsUnavailable") : logStatus === "idle" ? t("logsRequiresRunning") : logStatus === "paused" ? t("logsPaused") : t("logsWaiting")}</p>
      ) : logs.map((line, index) => (
        <p key={`${index}-${line}`}>
          <span className={line.includes("[Warn]") || line.toLowerCase().includes("error") ? "text-panel-gold" : "text-panel-green"}>
            {line.slice(0, 18)}
          </span>
          {line.slice(18)}
        </p>
      ))}
    </div>
  );
}

function ResourceRow({ title, meta, actions }: { title: string; meta: string; actions?: ReactNode }) {
  return (
    <div className="flex flex-col gap-3 rounded-md border border-panel-line bg-slate-950/50 px-3 py-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">{title}</p>
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
    <div className="mt-3 rounded-md border border-panel-line bg-slate-950/50 px-3 py-2">
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

function memoryPercent(stats?: { memoryMb: number; memoryLimitMb: number }): number {
  if (!stats || stats.memoryLimitMb <= 0) return 0;
  return Math.min(100, (stats.memoryMb / stats.memoryLimitMb) * 100);
}

function memoryDisplay(stats?: { memoryMb: number; memoryLimitMb: number }): string {
  if (!stats) return "—";
  return `${stats.memoryMb} MB / ${stats.memoryLimitMb} MB`;
}

function ResourceBar({ icon, label, value, suffix, displayText }: { icon: ReactNode; label: string; value: number; suffix: string; displayText?: string }) {
  return (
    <div>
      <div className="mb-1 flex items-center justify-between text-xs">
        <span className="flex items-center gap-2 text-slate-400">{icon}{label}</span>
        <span className="font-mono text-slate-300">{displayText ?? `${value.toFixed(1)}${suffix}`}</span>
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-slate-800">
        <div className="h-full rounded-full bg-panel-green transition-all" style={{ width: `${Math.min(100, value)}%` }} />
      </div>
    </div>
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
