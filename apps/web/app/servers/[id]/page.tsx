"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, Copy, Download, FileText, Package, RotateCcw, Terminal, Trash2, Upload } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from "react";
import { AppShell } from "@/components/app-shell";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { ServerActions } from "@/components/server-actions";
import { ServerModeBadge, ServerStatusBadge } from "@/components/server-badges";
import { Button, Card, Input } from "@/components/ui";
import {
  backupDownloadUrl,
  createBackup,
  deleteMod,
  getServer,
  importWorld,
  listBackups,
  listMods,
  listWorlds,
  previewTerrariaConfig,
  restoreBackup,
  sendServerCommand,
  serverLogsUrl,
  uploadMod,
  worldDownloadUrl
} from "@/lib/api";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
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

  const query = useQuery({ queryKey: ["server", id], queryFn: () => getServer(id), retry: false });
  const server = query.data;
  const worldsQuery = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, enabled: Boolean(server), retry: false });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, enabled: Boolean(server), retry: false });
  const modsQuery = useQuery({
    queryKey: ["mods", id],
    queryFn: () => listMods(id),
    enabled: Boolean(server && server.mode === "tmodloader"),
    retry: false
  });
  const previewQuery = useQuery({
    queryKey: ["server-config-preview", id, server?.config],
    queryFn: () => {
      if (!server) throw new Error(t("serverNotFound"));
      return previewTerrariaConfig(server.config);
    },
    enabled: Boolean(server),
    retry: false
  });

  const [activeTab, setActiveTab] = useState<TabId>("overview");
  const [copied, setCopied] = useState("");
  const [logs, setLogs] = useState<string[]>([]);
  const [command, setCommand] = useState("");
  const [errorMessage, setErrorMessage] = useState("");
  const [consoleError, setConsoleError] = useState("");
  const [pendingRestore, setPendingRestore] = useState<Backup | null>(null);
  const [pendingModDelete, setPendingModDelete] = useState<ModFile | null>(null);
  const [logStatus, setLogStatus] = useState<"connecting" | "connected" | "error">("connecting");

  const commandMutation = useMutation({
    mutationFn: (value: string) => sendServerCommand(id, value),
    onSuccess: (_, value) => {
      setLogs((current) => [...current, `> ${value}`].slice(-300));
      setCommand("");
      setConsoleError("");
    },
    onError: (error) => setConsoleError(error instanceof Error ? error.message : t("commandSendFailed"))
  });
  const worldUpload = useMutation({
    mutationFn: (file: File) => importWorld(file, id),
    onSuccess: async () => {
      setErrorMessage("");
      if (worldInputRef.current) worldInputRef.current.value = "";
      await client.invalidateQueries({ queryKey: ["worlds"] });
    },
    onError: (error) => setErrorMessage(error instanceof Error ? error.message : t("unableImportWorld"))
  });
  const backupCreate = useMutation({
    mutationFn: () => createBackup(id),
    onSuccess: async () => {
      setErrorMessage("");
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (error) => setErrorMessage(error instanceof Error ? error.message : t("unableCreateBackup"))
  });
  const backupRestore = useMutation({
    mutationFn: restoreBackup,
    onSuccess: async () => {
      setErrorMessage("");
      setPendingRestore(null);
      await client.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (error) => setErrorMessage(error instanceof Error ? error.message : t("unableRestoreBackup"))
  });
  const modUpload = useMutation({
    mutationFn: (file: File) => uploadMod(id, file),
    onSuccess: async () => {
      setErrorMessage("");
      if (modInputRef.current) modInputRef.current.value = "";
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => setErrorMessage(error instanceof Error ? error.message : t("unableUploadMod"))
  });
  const modDelete = useMutation({
    mutationFn: (modId: string) => deleteMod(id, modId),
    onSuccess: async () => {
      setErrorMessage("");
      setPendingModDelete(null);
      await client.invalidateQueries({ queryKey: ["mods", id] });
    },
    onError: (error) => setErrorMessage(error instanceof Error ? error.message : t("unableDeleteMod"))
  });

  useEffect(() => {
    if (!id) return;
    setLogs([]);
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
  }, [id]);

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

  if (!server) {
    return (
      <AppShell>
        <Link href="/servers" className="text-sm text-slate-400 hover:text-panel-green">{t("backToServers")}</Link>
        <Card className="mt-4 p-6 text-sm text-slate-400">{query.isLoading ? t("loading") : t("serverNotFound")}</Card>
      </AppShell>
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
  const invite = `Join ${server.name} at 127.0.0.1:${server.port}${server.password ? ` password: ${server.password}` : ""}`;
  const logStatusLabel = logStatus === "connected" ? t("logsConnected") : logStatus === "error" ? t("logsDisconnected") : t("logsConnecting");
  const copy = async (label: string, value: string) => {
    await navigator.clipboard.writeText(value);
    setCopied(label);
    window.setTimeout(() => setCopied(""), 1500);
  };
  const submitCommand = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const value = command.trim();
    if (!value || commandMutation.isPending) return;
    commandMutation.mutate(value);
  };

  return (
    <AppShell>
      <Link href="/servers" className="text-sm text-slate-400 hover:text-panel-green">{t("backToServers")}</Link>
      {query.isError && <p className="mt-3 text-sm text-panel-gold">{t("apiDetailUnavailable")}</p>}
      {errorMessage && <p className="mt-3 rounded-md border border-panel-gold/30 bg-panel-gold/10 px-3 py-2 text-sm text-panel-gold">{errorMessage}</p>}
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
            />
          )}
          {activeTab === "console" && (
            <ConsoleTab
              command={command}
              commandPending={commandMutation.isPending}
              consoleError={consoleError}
              logStatus={logStatus}
              logStatusLabel={logStatusLabel}
              logs={logs}
              server={server}
              viewportRef={logViewportRef}
              onChangeCommand={(value) => {
                setCommand(value);
                setConsoleError("");
              }}
              onSubmit={submitCommand}
            />
          )}
          {activeTab === "logs" && (
            <LogsTab
              logStatus={logStatus}
              logStatusLabel={logStatusLabel}
              logs={logs}
              viewportRef={logViewportRef}
              onClear={() => setLogs([])}
            />
          )}
          {activeTab === "config" && (
            <ConfigTab
              preview={previewQuery.data}
              previewError={previewQuery.isError}
              previewLoading={previewQuery.isLoading}
              server={server}
            />
          )}
          {activeTab === "worlds" && (
            <WorldsTab
              isError={worldsQuery.isError}
              isLoading={worldsQuery.isLoading}
              items={serverWorlds}
              uploading={worldUpload.isPending}
              onUploadClick={() => worldInputRef.current?.click()}
            />
          )}
          {activeTab === "backups" && (
            <BackupsTab
              creating={backupCreate.isPending}
              isError={backupsQuery.isError}
              isLoading={backupsQuery.isLoading}
              items={serverBackups}
              restoring={backupRestore.isPending}
              onCreate={() => backupCreate.mutate()}
              onRestore={setPendingRestore}
            />
          )}
          {activeTab === "mods" && server.mode === "tmodloader" && (
            <ModsTab
              deleting={modDelete.isPending}
              isError={modsQuery.isError}
              isLoading={modsQuery.isLoading}
              items={serverMods}
              uploading={modUpload.isPending}
              onDelete={setPendingModDelete}
              onUploadClick={() => modInputRef.current?.click()}
            />
          )}
        </Card>

        <div className="flex flex-col gap-4">
          <Card className="p-4">
            <h2 className="font-semibold">{t("joinServer")}</h2>
            <CopyRow label={t("ipAddress")} value="127.0.0.1" copied={copied} copiedLabel={t("copied")} copyLabel={t("copy")} onCopy={copy} />
            <CopyRow label={t("port")} value={String(server.port)} copied={copied} copiedLabel={t("copied")} copyLabel={t("copy")} onCopy={copy} />
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
          </Card>
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
    </AppShell>
  );
}

function OverviewTab({
  server,
  worldCount,
  backupCount,
  modCount
}: {
  server: Server;
  worldCount: number;
  backupCount: number;
  modCount: number;
}) {
  const { t } = useI18n();
  return (
    <div className="grid gap-3 md:grid-cols-3">
      <SummaryLink href="/worlds" icon={<FileText aria-hidden="true" />} label={t("tabWorlds")} value={String(worldCount)} />
      <SummaryLink href="/backups" icon={<Archive aria-hidden="true" />} label={t("tabBackups")} value={String(backupCount)} />
      {server.mode === "tmodloader" && <SummaryLink href="/mods" icon={<Package aria-hidden="true" />} label={t("tabMods")} value={String(modCount)} />}
    </div>
  );
}

function ConsoleTab({
  command,
  commandPending,
  consoleError,
  logStatus,
  logStatusLabel,
  logs,
  server,
  viewportRef,
  onChangeCommand,
  onSubmit
}: {
  command: string;
  commandPending: boolean;
  consoleError: string;
  logStatus: "connecting" | "connected" | "error";
  logStatusLabel: string;
  logs: string[];
  server: Server;
  viewportRef: React.RefObject<HTMLDivElement | null>;
  onChangeCommand: (value: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const { t } = useI18n();
  return (
    <div>
      <LogHeader logStatus={logStatus} logStatusLabel={logStatusLabel} title={t("liveLogs")} />
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
  logs,
  viewportRef,
  onClear
}: {
  logStatus: "connecting" | "connected" | "error";
  logStatusLabel: string;
  logs: string[];
  viewportRef: React.RefObject<HTMLDivElement | null>;
  onClear: () => void;
}) {
  const { t } = useI18n();
  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <LogHeader logStatus={logStatus} logStatusLabel={logStatusLabel} title={t("liveLogs")} />
        <Button variant="secondary" onClick={onClear} disabled={logs.length === 0}>{t("clearLogs")}</Button>
      </div>
      <LogViewport className="h-[520px]" logs={logs} logStatus={logStatus} viewportRef={viewportRef} />
    </div>
  );
}

function ConfigTab({
  preview,
  previewError,
  previewLoading,
  server
}: {
  preview?: string;
  previewError: boolean;
  previewLoading: boolean;
  server: Server;
}) {
  const { t } = useI18n();
  const config = server.config;
  const rows: [string, string][] = [
    [t("serverName"), config.serverName || server.name],
    [t("worldName"), config.worldName || server.world],
    [t("worldSize"), worldSizeLabel(config.worldSize, t)],
    [t("difficulty"), difficultyLabel(config.difficulty, t)],
    [t("maxPlayers"), String(config.maxPlayers)],
    [t("port"), String(config.port)],
    [t("password"), config.password || t("none")],
    [t("motd"), config.motd || t("none")],
    [t("secureMode"), config.secure ? t("enabled") : t("disabled")],
    [t("languageSetting"), config.language],
    [t("autoCreateWorld"), config.autoCreateWorld ? t("enabled") : t("disabled")]
  ];
  return (
    <div className="grid gap-4 xl:grid-cols-[360px_1fr]">
      <div className="grid gap-2">
        {rows.map(([label, value]) => <Info key={label} label={label} value={value} />)}
      </div>
      <div className="min-w-0 rounded-md border border-panel-line bg-slate-950 p-4">
        <div className="mb-3 flex items-center gap-2 text-sm font-medium text-white">
          <FileText aria-hidden="true" className="size-4 text-panel-green" />
          {t("previewServerConfig")}
        </div>
        {previewLoading ? (
          <p className="text-sm text-slate-400">{t("rendering")}</p>
        ) : previewError ? (
          <p className="text-sm text-panel-gold">{t("configPreviewUnavailable")}</p>
        ) : (
          <pre className="overflow-auto whitespace-pre-wrap font-mono text-xs leading-6 text-slate-300">{preview}</pre>
        )}
      </div>
    </div>
  );
}

function WorldsTab({
  isError,
  isLoading,
  items,
  uploading,
  onUploadClick
}: {
  isError: boolean;
  isLoading: boolean;
  items: World[];
  uploading: boolean;
  onUploadClick: () => void;
}) {
  const { locale, t } = useI18n();
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
            actions={<ActionLink href={worldDownloadUrl(world.id)} label={t("download")} icon={<Download aria-hidden="true" />} />}
          />
        ))}
      </div>
    </ResourcePanel>
  );
}

function BackupsTab({
  creating,
  isError,
  isLoading,
  items,
  restoring,
  onCreate,
  onRestore
}: {
  creating: boolean;
  isError: boolean;
  isLoading: boolean;
  items: Backup[];
  restoring: boolean;
  onCreate: () => void;
  onRestore: (backup: Backup) => void;
}) {
  const { locale, t } = useI18n();
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
                <Button variant="secondary" aria-label={t("restore")} onClick={() => onRestore(backup)} disabled={restoring}>
                  <RotateCcw aria-hidden="true" />
                </Button>
                <ActionLink href={backupDownloadUrl(backup.id)} label={t("download")} icon={<Download aria-hidden="true" />} />
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
  uploading,
  onDelete,
  onUploadClick
}: {
  deleting: boolean;
  isError: boolean;
  isLoading: boolean;
  items: ModFile[];
  uploading: boolean;
  onDelete: (mod: ModFile) => void;
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
              <Button variant="danger" aria-label={t("delete")} onClick={() => onDelete(mod)} disabled={deleting}>
                <Trash2 aria-hidden="true" />
              </Button>
            }
          />
        ))}
      </div>
    </ResourcePanel>
  );
}

function ResourcePanel({ title, href, action, children }: { title: string; href: string; action: ReactNode; children: ReactNode }) {
  const { t } = useI18n();
  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <h2 className="font-semibold">{title}</h2>
        <div className="flex flex-wrap gap-2">
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

function LogHeader({ logStatus, logStatusLabel, title }: { logStatus: "connecting" | "connected" | "error"; logStatusLabel: string; title: string }) {
  return (
    <div className="mb-3 flex min-w-0 flex-1 items-center justify-between rounded-md border border-panel-line bg-slate-950/50 px-3 py-2 text-xs">
      <span className="text-slate-400">{title}</span>
      <span className={logStatus === "connected" ? "text-panel-green" : logStatus === "error" ? "text-panel-gold" : "text-slate-400"}>{logStatusLabel}</span>
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
  logStatus: "connecting" | "connected" | "error";
  viewportRef: React.RefObject<HTMLDivElement | null>;
}) {
  const { t } = useI18n();
  return (
    <div ref={viewportRef} className={cn("h-[420px] overflow-auto rounded-md bg-slate-950 p-4 font-mono text-xs leading-6 text-slate-300", className)}>
      {logs.length === 0 ? (
        <p className="text-slate-500">{logStatus === "error" ? t("logsUnavailable") : t("logsWaiting")}</p>
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

function ActionLink({ href, icon, label }: { href: string; icon: ReactNode; label: string }) {
  return (
    <a className="inline-flex items-center justify-center gap-2 rounded-md border border-panel-line bg-slate-900/70 px-3 py-2 text-sm font-medium text-slate-100 transition hover:bg-slate-800" href={href}>
      {icon}
      {label}
    </a>
  );
}

function SummaryLink({ href, icon, label, value }: { href: string; icon: ReactNode; label: string; value: string }) {
  return (
    <Link href={href} className="rounded-md border border-panel-line bg-slate-950/50 p-4 transition hover:border-panel-green/50">
      <div className="flex items-center justify-between gap-3">
        <span className="text-slate-400">{label}</span>
        <span className="text-panel-green">{icon}</span>
      </div>
      <p className="mt-3 text-2xl font-semibold">{value}</p>
    </Link>
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
