"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Copy } from "lucide-react";
import { useState } from "react";
import { AppShell } from "@/components/app-shell";
import { ServerActions } from "@/components/server-actions";
import { ServerModeBadge, ServerStatusBadge } from "@/components/server-badges";
import { Button, Card, Input } from "@/components/ui";
import { getServer } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export default function ServerDetailPage() {
  const { t } = useI18n();
  const params = useParams<{ id: string }>();
  const id = params.id;
  const query = useQuery({ queryKey: ["server", id], queryFn: () => getServer(id), retry: false });
  const server = query.data;
  const [copied, setCopied] = useState("");

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
          <div className="h-[420px] rounded-md bg-slate-950 p-4 font-mono text-xs leading-6 text-slate-300">
            <p className="text-slate-500">{t("logsNeedLiveServer")}</p>
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
        <Card className="p-4"><h2 className="font-semibold">{t("tabWorlds")}</h2><p className="mt-2 text-sm text-slate-400">{server.world}</p></Card>
        <Card className="p-4"><h2 className="font-semibold">{t("tabBackups")}</h2><p className="mt-2 text-sm text-slate-400">{t("manageBackupsInBackupsPage")}</p></Card>
      </div>
    </AppShell>
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
