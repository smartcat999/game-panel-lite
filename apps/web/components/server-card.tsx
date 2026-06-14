"use client";

import Link from "next/link";
import { ServerModeBadge, ServerStatusBadge } from "./server-badges";
import { ServerActions } from "./server-actions";
import { ServerGameArt } from "./server-game-art";
import { Card } from "@/components/ui";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import type { Server } from "@/lib/types";

export function ServerCard({ server, compact = false }: { server: Server; compact?: boolean }) {
  const { locale, t } = useI18n();
  return (
    <Card className="p-4">
      <div className="flex gap-4">
        <ServerGameArt mode={server.mode} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <Link href={`/servers/${server.id}`} className="font-semibold text-white hover:text-panel-green">
              {server.name}
            </Link>
            <ServerModeBadge mode={server.mode} />
            <ServerStatusBadge status={server.status} />
          </div>
          <p className="mt-1 text-xs text-slate-400">{t("world")}: {server.world}</p>
          <div className="mt-4 grid grid-cols-2 gap-3 text-xs text-slate-400 md:grid-cols-4">
            <Metric label={t("players")} value={`${server.players} / ${server.maxPlayers}`} />
            <Metric label={t("port")} value={String(server.port)} />
            <Metric label={t("version")} value={server.version} />
            <Metric label={t("lastBackup")} value={localizeRelativeTime(server.lastBackup, locale)} />
          </div>
          {!compact && (
            <div className="mt-4">
              <ServerActions server={server} />
            </div>
          )}
        </div>
      </div>
    </Card>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p>{label}</p>
      <p className="mt-1 font-medium text-white">{value}</p>
    </div>
  );
}
