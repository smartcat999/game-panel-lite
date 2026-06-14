"use client";

import Link from "next/link";
import { Copy } from "lucide-react";
import { ServerActions } from "@/components/server-actions";
import { ServerModeBadge, ServerStatusBadge } from "@/components/server-badges";
import { Button, Card, Input } from "@/components/ui";
import { useI18n } from "@/lib/i18n";
import type { Backup, Server, World } from "@/lib/types";

export function ServerDetailView({ server, worlds, backups, logs }: { server: Server; worlds: World[]; backups: Backup[]; logs: string[] }) {
  const { t } = useI18n();
  const tabs = [
    t("tabOverview"),
    t("tabConsole"),
    t("tabLogs"),
    t("tabConfig"),
    t("tabWorlds"),
    t("tabBackups"),
    ...(server.mode === "tmodloader" ? [t("tabMods")] : [])
  ];

  return (
    <>
      <Link href="/servers" className="text-sm text-slate-400 hover:text-panel-green">{t("backToServers")}</Link>
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
          <div className="h-[420px] rounded-md bg-slate-950 p-4 font-mono text-xs leading-6 text-slate-300">
            {logs.map((line) => <p key={line}><span className={line.includes("[Warn]") ? "text-panel-gold" : "text-panel-green"}>{line.slice(0, 18)}</span>{line.slice(18)}</p>)}
          </div>
          <div className="mt-3 flex gap-2">
            <Input placeholder={t("enterCommand")} />
            <Button>{t("send")}</Button>
          </div>
        </Card>
        <div className="flex flex-col gap-4">
          <Card className="p-4">
            <h2 className="font-semibold">{t("joinServer")}</h2>
            <CopyRow label={t("ipAddress")} value="192.168.1.20" copyLabel={t("copy")} />
            <CopyRow label={t("port")} value={String(server.port)} copyLabel={t("copy")} />
            <CopyRow label={t("password")} value={server.password || t("none")} copyLabel={t("copy")} />
            <Button className="mt-4 w-full"><Copy aria-hidden="true" />{t("copyInviteText")}</Button>
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
        <Card className="p-4"><h2 className="font-semibold">{t("tabWorlds")}</h2><p className="mt-2 text-sm text-slate-400">{worlds[0]?.name}</p></Card>
        <Card className="p-4"><h2 className="font-semibold">{t("tabBackups")}</h2><p className="mt-2 text-sm text-slate-400">{backups[0]?.name}</p></Card>
      </div>
    </>
  );
}

function CopyRow({ label, value, copyLabel }: { label: string; value: string; copyLabel: string }) {
  return (
    <div className="mt-3">
      <p className="text-xs text-slate-400">{label}</p>
      <div className="mt-1 flex items-center justify-between rounded-md bg-slate-950 px-3 py-2 text-sm">
        <span>{value}</span>
        <button className="text-panel-green">{copyLabel}</button>
      </div>
    </div>
  );
}

function Info({ label, value }: { label: string; value: string }) {
  return <div className="mt-3 flex justify-between text-sm"><span className="text-slate-400">{label}</span><span>{value}</span></div>;
}
