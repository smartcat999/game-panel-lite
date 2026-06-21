"use client";

import Link from "next/link";
import { Plug, Users } from "lucide-react";
import { ServerProviderBadge, ServerStatusBadge } from "./server-badges";
import { ServerActions } from "./server-actions";
import { ServerGameArt } from "./server-game-art";
import { Card } from "@/components/ui";
import { gameServerJoinPort, gameServerMaxPlayers, gameServerMode, gameServerStatus, gameServerVersion } from "@/lib/game-server-resource";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { GameServerResource } from "@/lib/types";

export function ServerCard({ server, compact = false }: { server: GameServerResource; compact?: boolean }) {
  const { t } = useI18n();
  const status = gameServerStatus(server);
  const players = server.status.playersOnline ?? 0;
  const maxPlayers = gameServerMaxPlayers(server);
  const version = gameServerVersion(server);
  const joinPort = gameServerJoinPort(server);
  const displayServer = { gameKey: server.gameKey, providerKey: server.providerKey, mode: gameServerMode(server) };
  return (
    <Card className="group overflow-hidden p-0 transition hover:border-panel-green/35">
      <div className="flex flex-col gap-4 p-4 sm:flex-row sm:items-start">
        <Link
          aria-label={server.name}
          className="w-fit shrink-0 rounded-md focus:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/50 focus-visible:ring-offset-2 focus-visible:ring-offset-panel-card"
          href={`/servers/${server.id}`}
        >
          <ServerGameArt server={displayServer} className="size-20" />
        </Link>
        <div className="min-w-0 flex-1 space-y-4">
          <div className="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
            <div className="min-w-0">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <Link href={`/servers/${server.id}`} className="min-w-0 truncate text-base font-semibold text-white transition hover:text-panel-green">
                  {server.name}
                </Link>
                <ServerProviderBadge server={displayServer} />
                <ServerStatusBadge status={status} />
              </div>
            </div>
            <PlayerPill label={t("players")} players={players} maxPlayers={maxPlayers} running={status === "running"} />
          </div>
          <div className="grid gap-2 sm:grid-cols-2">
            <InfoTile label={t("version")} value={version} />
            <InfoTile icon={<Plug aria-hidden="true" />} label={t("port")} value={String(joinPort)} />
          </div>
        </div>
      </div>
      {!compact && (
        <div className="border-t border-panel-line bg-slate-950/25 px-4 py-3">
          <ServerActions server={server} compact className="sm:justify-end" />
        </div>
      )}
    </Card>
  );
}

function PlayerPill({ label, maxPlayers, players, running }: { label: string; maxPlayers: number; players: number; running: boolean }) {
  return (
    <div
      className={cn(
        "flex w-full items-center justify-between gap-3 rounded-md border bg-slate-950/35 px-3 py-2 lg:w-auto lg:min-w-32",
        running ? "border-panel-green/30 text-panel-green" : "border-panel-line text-slate-300"
      )}
    >
      <div className="flex items-center gap-2 text-xs font-medium text-slate-400">
        <Users aria-hidden="true" className={cn("size-4", running ? "text-panel-green" : "text-slate-500")} />
        {label}
      </div>
      <p className="whitespace-nowrap text-sm font-semibold text-white">{players} / {maxPlayers}</p>
    </div>
  );
}

function InfoTile({ icon, label, value }: { icon?: React.ReactNode; label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md border border-panel-line bg-slate-950/30 px-3 py-2">
      <div className="flex items-center gap-1.5 text-xs text-slate-500">
        {icon ? <span className="text-slate-500 [&>svg]:size-3.5">{icon}</span> : null}
        <span>{label}</span>
      </div>
      <p className="mt-1 truncate text-sm font-medium text-slate-100">{value}</p>
    </div>
  );
}
