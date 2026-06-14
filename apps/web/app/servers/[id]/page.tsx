"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Copy } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { AppShell } from "@/components/app-shell";
import { ServerActions } from "@/components/server-actions";
import { ServerModeBadge, ServerStatusBadge } from "@/components/server-badges";
import { Button, Card, Input } from "@/components/ui";
import { getServer, listBackups, listMods, listWorlds, serverLogsUrl } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { Backup, ModFile, World } from "@/lib/types";

export default function ServerDetailPage() {
  const { t } = useI18n();
  const params = useParams<{ id: string }>();
  const id = params.id;
  const query = useQuery({ queryKey: ["server", id], queryFn: () => getServer(id), retry: false });
  const server = query.data;
  const worldsQuery = useQuery({ queryKey: ["worlds", id], queryFn: listWorlds, enabled: Boolean(server), retry: false });
  const backupsQuery = useQuery({ queryKey: ["backups", id], queryFn: listBackups, enabled: Boolean(server), retry: false });
  const modsQuery = useQuery({
    queryKey: ["mods", id],
    queryFn: () => listMods(id),
    enabled: Boolean(server && server.mode === "tmodloader"),
    retry: false
  });
  const [copied, setCopied] = useState("");
  const [logs, setLogs] = useState<string[]>([]);
  const [logStatus, setLogStatus] = useState<"connecting" | "connected" | "error">("connecting");
  const logViewportRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!id) return;
    setLogs([]);
    setLogStatus("connecting");
    const source = new EventSource(serverLogsUrl(id));
    source.onopen = () => setLogStatus("connected");
    source.addEventListener("log", (event) => {
      setLogs((current) => [...current, event.data].slice(-200));
    });
    source.addEventListener("error", (event) => {
      setLogStatus("error");
      const data = "data" in event && typeof event.data === "string" ? event.data : "";
      if (data) {
        setLogs((current) => [...current, data].slice(-200));
      }
    });
    source.onerror = () => setLogStatus("error");
    return () => source.close();
  }, [id]);

  useEffect(() => {
    const viewport = logViewportRef.current;
    if (viewport) {
      viewport.scrollTop = viewport.scrollHeight;
    }
  }, [logs]);

  const logStatusLabel = useMemo(() => {
    if (logStatus === "connected") return t("logsConnected");
    if (logStatus === "error") return t("logsDisconnected");
    return t("logsConnecting");
  }, [logStatus, t]);
  const serverWorlds = useMemo(
    () => (server ? (worldsQuery.data ?? []).filter((world) => world.server === server.id || world.name === server.world).slice(0, 3) : []),
    [server, worldsQuery.data]
  );
  const serverBackups = useMemo(
    () => (server ? (backupsQuery.data ?? []).filter((backup) => backup.instanceId === server.id).slice(0, 3) : []),
    [backupsQuery.data, server]
  );
  const serverMods = useMemo(() => (modsQuery.data ?? []).slice(0, 3), [modsQuery.data]);

  if (!server) {
    return (
      <AppShell>
        <Link href="/servers" className="text-sm text-slate-400 hover:text-panel-green">{t("backToServers")}</Link>
        <Card className="mt-4 p-6 text-sm text-slate-400">{t("serverNotFound")}</Card>
      </AppShell>
    );
  }

  const invite = `Join ${server.name} at 127.0.0.1:${server.port}${server.password ? ` password: ${server.password}` : ""}`;
  const tabs = [
    t("tabOverview"),
    t("tabConsole"),
    t("tabLogs"),
    t("tabConfig"),
    t("tabWorlds"),
    t("tabBackups"),
    ...(server.mode === "tmodloader" ? [t("tabMods")] : [])
  ];
  const copy = async (label: string, value: string) => {
    await navigator.clipboard.writeText(value);
    setCopied(label);
    window.setTimeout(() => setCopied(""), 1500);
  };

  return (
    <AppShell>
      <Link href="/servers" className="text-sm text-slate-400 hover:text-panel-green">{t("backToServers")}</Link>
      {query.isError && <p className="mt-3 text-sm text-panel-gold">{t("apiDetailUnavailable")}</p>}
      <div className="mt-3 flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold">{server.name}</h1>
            <ServerModeBadge mode={server.mode} />
            <ServerStatusBadge status={server.status} />
          </div>
          <p className="mt-2 text-sm text-slate-400">
            {t("serverDetailSummary", { players: server.players, maxPlayers: server.maxPlayers, port: server.port, version: server.version })}
          </p>
        </div>
        <ServerActions server={server} />
      </div>
      <div className="mt-6 grid gap-4 xl:grid-cols-[1fr_320px]">
        <Card className="p-4">
          <div className="mb-4 flex gap-5 border-b border-panel-line pb-3 text-sm text-slate-400">
            {tabs.map((tab) => (
              <span key={tab} className={tab === t("tabConsole") ? "text-panel-green" : ""}>{tab}</span>
            ))}
          </div>
          <div className="mb-3 flex items-center justify-between rounded-md border border-panel-line bg-slate-950/50 px-3 py-2 text-xs">
            <span className="text-slate-400">{t("liveLogs")}</span>
            <span className={logStatus === "connected" ? "text-panel-green" : logStatus === "error" ? "text-panel-gold" : "text-slate-400"}>{logStatusLabel}</span>
          </div>
          <div ref={logViewportRef} className="h-[420px] overflow-auto rounded-md bg-slate-950 p-4 font-mono text-xs leading-6 text-slate-300">
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
          <div className="mt-3 flex gap-2">
            <Input placeholder={t("consoleCommandUnsupported")} disabled />
            <Button disabled title={t("consoleCommandTitle")}>{t("send")}</Button>
          </div>
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
            <Info label={t("difficulty")} value={t("tagExpert")} />
            <Info label={t("worldSize")} value={t("tagMediumWorld")} />
            <Info label={t("maxPlayers")} value={String(server.maxPlayers)} />
          </Card>
        </div>
      </div>
      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <ResourceCard
          title={t("recentWorlds")}
          href="/worlds"
          cta={t("manageWorlds")}
          isError={worldsQuery.isError}
          error={t("apiWorldsUnavailable")}
          empty={t("noWorldsYet")}
          items={serverWorlds}
          renderItem={(world) => (
            <ResourceRow key={world.id} title={world.name} meta={`${world.bytes} · ${world.modified}`} />
          )}
        />
        <ResourceCard
          title={t("recentBackups")}
          href="/backups"
          cta={t("manageBackups")}
          isError={backupsQuery.isError}
          error={t("apiBackupsUnavailable")}
          empty={t("noBackupsYet")}
          items={serverBackups}
          renderItem={(backup) => (
            <ResourceRow key={backup.id} title={backup.name} meta={`${backup.world} · ${backup.size} · ${backup.created}`} />
          )}
        />
        {server.mode === "tmodloader" && (
          <ResourceCard
            title={t("recentMods")}
            href="/mods"
            cta={t("manageMods")}
            isError={modsQuery.isError}
            error={t("modsApiUnavailable")}
            empty={t("noModsUploaded")}
            items={serverMods}
            renderItem={(mod) => (
              <ResourceRow key={mod.id} title={mod.fileName} meta={`${mod.size} · ${mod.enabled ? t("enabled") : t("disabled")}`} />
            )}
          />
        )}
      </div>
    </AppShell>
  );
}

function ResourceCard<T extends World | Backup | ModFile>({
  title,
  href,
  cta,
  isError,
  error,
  empty,
  items,
  renderItem
}: {
  title: string;
  href: string;
  cta: string;
  isError: boolean;
  error: string;
  empty: string;
  items: T[];
  renderItem: (item: T) => ReactNode;
}) {
  return (
    <Card className="p-4">
      <div className="flex items-center justify-between gap-3">
        <h2 className="font-semibold">{title}</h2>
        <Link href={href} className="text-sm text-panel-green hover:text-panel-green/80">{cta}</Link>
      </div>
      {isError ? (
        <p className="mt-3 text-sm text-panel-gold">{error}</p>
      ) : items.length === 0 ? (
        <p className="mt-3 text-sm text-slate-400">{empty}</p>
      ) : (
        <div className="mt-3 space-y-2">{items.map(renderItem)}</div>
      )}
    </Card>
  );
}

function ResourceRow({ title, meta }: { title: string; meta: string }) {
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/50 px-3 py-2">
      <p className="truncate text-sm font-medium">{title}</p>
      <p className="mt-1 truncate text-xs text-slate-500">{meta}</p>
    </div>
  );
}

function CopyRow({
  label,
  value,
  copied,
  copiedLabel,
  copyLabel,
  onCopy
}: {
  label: string;
  value: string;
  copied: string;
  copiedLabel: string;
  copyLabel: string;
  onCopy: (label: string, value: string) => Promise<void>;
}) {
  return (
    <div className="mt-3">
      <p className="text-xs text-slate-400">{label}</p>
      <div className="mt-1 flex items-center justify-between rounded-md bg-slate-950 px-3 py-2 text-sm">
        <span>{value}</span>
        <button className="text-panel-green" type="button" onClick={() => void onCopy(label, value)}>{copied === label ? copiedLabel : copyLabel}</button>
      </div>
    </div>
  );
}

function Info({ label, value }: { label: string; value: string }) {
  return <div className="mt-3 flex justify-between text-sm"><span className="text-slate-400">{label}</span><span>{value}</span></div>;
}
