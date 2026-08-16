"use client";

import Link from "next/link";
import { Plug, Users } from "lucide-react";
import { ServerActions } from "@/components/server-actions";
import { ServerGameArt } from "@/components/server-game-art";
import { ServerProviderLabel, ServerStatusIndicator } from "@/components/server-badges";
import {
  gameServerJoinPort,
  gameServerMaxPlayers,
  gameServerMode,
  gameServerStatus,
  gameServerVersion
} from "@/lib/game-server-resource";
import { useI18n } from "@/lib/i18n";
import type { ObservabilityServerMetric } from "@/lib/api";
import type { GameServerResource } from "@/lib/types";

export function ServerResourceTable({
  servers,
  metrics = [],
  showActions = true,
  showVersion = true,
  limit,
  flat = false,
  publicHost
}: {
  servers: GameServerResource[];
  metrics?: ObservabilityServerMetric[];
  showActions?: boolean;
  showVersion?: boolean;
  limit?: number;
  flat?: boolean;
  publicHost?: string;
}) {
  const { t } = useI18n();
  const rows = typeof limit === "number" ? servers.slice(0, limit) : servers;
  const metricMap = new Map(metrics.map((metric) => [metric.id, metric]));

  return (
    <div className={flat ? "overflow-hidden border-y border-panel-line" : "overflow-hidden rounded-lg border border-panel-line bg-panel-card"}>
      <div className="hidden overflow-x-auto md:block">
        <table className="w-full min-w-[880px] border-collapse text-left text-sm">
          <thead className="bg-slate-950/45 text-xs font-medium text-slate-500">
            <tr>
              <th className="px-4 py-3">{t("server")}</th>
              <th className="px-3 py-3">{t("serverType")}</th>
              <th className="px-3 py-3">{t("status")}</th>
              <th className="px-3 py-3">{t("players")}</th>
              <th className="px-3 py-3">CPU</th>
              <th className="px-3 py-3">{t("memory")}</th>
              <th className="px-3 py-3">{t("serverAddress")}</th>
              {showVersion ? <th className="px-3 py-3">{t("version")}</th> : null}
              {showActions ? <th className="px-4 py-3 text-right">{t("actions")}</th> : null}
            </tr>
          </thead>
          <tbody className="divide-y divide-panel-line">
            {rows.map((server) => {
              const metric = metricMap.get(server.id);
              const status = gameServerStatus(server);
              const players = server.status.playersOnline;
              const maxPlayers = gameServerMaxPlayers(server);
              const displayServer = { gameKey: server.gameKey, providerKey: server.providerKey, mode: gameServerMode(server) };
              return (
                <tr key={server.id} className="group bg-panel-card transition-colors hover:bg-slate-800/35">
                  <td className="px-4 py-3">
                    <Link className="flex min-w-0 items-center gap-3" href={`/servers/${server.id}`}>
                      <ServerGameArt server={displayServer} className="size-9 rounded" compact />
                      <span className="min-w-0">
                        <span className="block max-w-64 truncate font-medium text-slate-100 group-hover:text-panel-green">{server.name}</span>
                        <span className="mt-0.5 block text-xs text-slate-500">{server.id.slice(0, 8)}</span>
                      </span>
                    </Link>
                  </td>
                  <td className="max-w-52 px-3 py-3"><ServerProviderLabel server={displayServer} /></td>
                  <td className="px-3 py-3"><ServerStatusIndicator status={status} /></td>
                  <td className="px-3 py-3 font-mono text-slate-300">
                    {typeof players === "number" ? `${players}/${maxPlayers}` : <DataMissing />}
                  </td>
                  <td className="px-3 py-3 font-mono text-slate-300">{metric?.statsAvailable ? `${metric.cpuPercent.toFixed(1)}%` : <DataMissing />}</td>
                  <td className="px-3 py-3 font-mono text-slate-300">{metric?.statsAvailable ? formatMemory(metric.memoryMb) : <DataMissing />}</td>
                  <td className="px-3 py-3 font-mono text-slate-300">{formatAddress(publicHost, gameServerJoinPort(server))}</td>
                  {showVersion ? <td className="px-3 py-3 text-slate-300">{gameServerVersion(server)}</td> : null}
                  {showActions ? (
                    <td className="px-4 py-3">
                      <ServerActions server={server} rowMode showInvite={false} showDelete />
                    </td>
                  ) : null}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <div className="divide-y divide-panel-line md:hidden">
        {rows.map((server) => {
          const metric = metricMap.get(server.id);
          const status = gameServerStatus(server);
          const players = server.status.playersOnline;
          const maxPlayers = gameServerMaxPlayers(server);
          const displayServer = { gameKey: server.gameKey, providerKey: server.providerKey, mode: gameServerMode(server) };
          return (
            <div key={server.id} className="px-4 py-4">
              <div className="flex items-start gap-3">
                <ServerGameArt server={displayServer} className="size-10 rounded" compact />
                <div className="min-w-0 flex-1">
                  <Link className="block truncate font-medium text-slate-100" href={`/servers/${server.id}`}>{server.name}</Link>
                  <div className="mt-1.5 flex flex-wrap items-center gap-2">
                    <ServerProviderLabel server={displayServer} />
                    <span aria-hidden="true" className="h-3 w-px bg-panel-line" />
                    <ServerStatusIndicator status={status} />
                  </div>
                </div>
              </div>
              <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 border-y border-panel-line py-3 text-xs">
                <Metric label={t("players")} icon={<Users aria-hidden="true" className="size-3.5" />} value={typeof players === "number" ? `${players}/${maxPlayers}` : "—"} />
                <Metric label="CPU" value={metric?.statsAvailable ? `${metric.cpuPercent.toFixed(1)}%` : "—"} />
                <Metric label={t("memory")} value={metric?.statsAvailable ? formatMemory(metric.memoryMb) : "—"} />
                <Metric label={t("serverAddress")} icon={<Plug aria-hidden="true" className="size-3.5" />} value={formatAddress(publicHost, gameServerJoinPort(server))} />
              </div>
              {showActions ? <div className="mt-3"><ServerActions server={server} rowMode showInvite={false} showDelete /></div> : null}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function DataMissing() {
  return <span className="text-slate-600">—</span>;
}

function Metric({ icon, label, value }: { icon?: React.ReactNode; label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="flex items-center gap-1.5 text-slate-500">{icon}{label}</span>
      <span className="font-mono text-slate-200">{value}</span>
    </div>
  );
}

function formatMemory(value: number) {
  return value >= 1024 ? `${(value / 1024).toFixed(1)} GB` : `${Math.round(value)} MB`;
}

function formatAddress(host: string | undefined, port: number) {
  const normalizedHost = host?.trim();
  if (!normalizedHost) return `:${port}`;
  return normalizedHost.includes(":") && !normalizedHost.startsWith("[") ? `[${normalizedHost}]:${port}` : `${normalizedHost}:${port}`;
}
