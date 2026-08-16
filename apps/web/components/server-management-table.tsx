"use client";

import Link from "next/link";
import { ArrowDown, ArrowUp, Copy, Plug, Users } from "lucide-react";
import { ServerActions } from "@/components/server-actions";
import { ServerGameArt } from "@/components/server-game-art";
import { ServerStatusIndicator } from "@/components/server-badges";
import { SelectionBox } from "@/components/selection-box";
import { copyText } from "@/lib/clipboard";
import { gameDisplayName } from "@/lib/game-display";
import {
  gameServerJoinPort,
  gameServerMaxPlayers,
  gameServerMode,
  gameServerStatus,
  gameServerVersion
} from "@/lib/game-server-resource";
import { useI18n } from "@/lib/i18n";
import { serverProviderDisplay } from "@/lib/server-display";
import type { ObservabilityServerMetric } from "@/lib/api";
import type { GameServerResource } from "@/lib/types";
import { cn } from "@/lib/utils";

export type ServerTableColumn = "players" | "resources" | "address" | "activity" | "version";
export type ServerTableSort = "name" | "status" | "updatedAt";

export function ServerManagementTable({
  servers,
  metrics = [],
  publicHost,
  selectedIds,
  visibleColumns,
  sort,
  direction,
  onSelectionChange,
  onSort,
  onAddressCopied
}: {
  servers: GameServerResource[];
  metrics?: ObservabilityServerMetric[];
  publicHost?: string;
  selectedIds: Set<string>;
  visibleColumns: Set<ServerTableColumn>;
  sort: ServerTableSort;
  direction: "asc" | "desc";
  onSelectionChange: (ids: Set<string>) => void;
  onSort: (sort: ServerTableSort) => void;
  onAddressCopied: () => void;
}) {
  const { t, locale } = useI18n();
  const metricMap = new Map(metrics.map((metric) => [metric.id, metric]));
  const allSelected = servers.length > 0 && servers.every((server) => selectedIds.has(server.id));
  const partiallySelected = !allSelected && servers.some((server) => selectedIds.has(server.id));
  const togglePage = () => {
    const next = new Set(selectedIds);
    if (allSelected) servers.forEach((server) => next.delete(server.id));
    else servers.forEach((server) => next.add(server.id));
    onSelectionChange(next);
  };

  return (
    <div className="overflow-hidden rounded-lg border border-panel-line bg-panel-card">
      <div className="hidden max-h-[calc(100vh-19rem)] overflow-auto md:block">
        <table className="w-full min-w-[1000px] border-collapse text-left text-sm">
          <thead className="sticky top-0 z-10 bg-slate-950 text-xs font-medium text-slate-500 shadow-[0_1px_0_rgba(51,65,85,0.65)]">
            <tr>
              <th className="w-11 px-3 py-2.5">
                <SelectionBox checked={allSelected} indeterminate={partiallySelected} label={t("selectCurrentPage")} onChange={togglePage} />
              </th>
              <SortableHeader active={sort === "name"} direction={direction} label={t("server")} onClick={() => onSort("name")} className="min-w-60" />
              <SortableHeader active={sort === "status"} direction={direction} label={t("status")} onClick={() => onSort("status")} />
              <th className="px-3 py-2.5">{t("gameAndMode")}</th>
              {visibleColumns.has("players") ? <th className="px-3 py-2.5">{t("players")}</th> : null}
              {visibleColumns.has("resources") ? <th className="px-3 py-2.5">{t("resources")}</th> : null}
              {visibleColumns.has("address") ? <th className="px-3 py-2.5">{t("serverAddress")}</th> : null}
              {visibleColumns.has("activity") ? <SortableHeader active={sort === "updatedAt"} direction={direction} label={t("recentActivity")} onClick={() => onSort("updatedAt")} /> : null}
              {visibleColumns.has("version") ? <th className="px-3 py-2.5">{t("version")}</th> : null}
              <th className="w-36 px-3 py-2.5 text-right">{t("actions")}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-panel-line">
            {servers.map((server) => {
              const metric = metricMap.get(server.id);
              const status = gameServerStatus(server);
              const address = formatAddress(publicHost, gameServerJoinPort(server));
              const provider = serverProviderDisplay(server);
              return (
                <tr key={server.id} className={cn("group transition-colors hover:bg-slate-800/35", selectedIds.has(server.id) && "bg-panel-green/[0.045]") }>
                  <td className="px-3 py-2.5">
                    <SelectionBox checked={selectedIds.has(server.id)} label={t("selectServer", { name: server.name })} onChange={() => {
                      const next = new Set(selectedIds);
                      if (next.has(server.id)) next.delete(server.id); else next.add(server.id);
                      onSelectionChange(next);
                    }} />
                  </td>
                  <td className="px-3 py-2.5">
                    <Link className="flex min-w-0 items-center gap-2.5" href={`/servers/${server.id}`}>
                      <ServerGameArt server={{ gameKey: server.gameKey, providerKey: server.providerKey, mode: gameServerMode(server) }} className="size-8 rounded" compact />
                      <span className="min-w-0">
                        <span className="block max-w-64 truncate font-medium text-slate-100 group-hover:text-panel-green">{server.name}</span>
                        <span className="mt-0.5 block font-mono text-[11px] text-slate-500">{server.id.slice(0, 8)}</span>
                      </span>
                    </Link>
                  </td>
                  <td className="px-3 py-2.5"><ServerStatusIndicator status={status} /></td>
                  <td className="px-3 py-2.5">
                    <span className="block text-slate-200">{gameDisplayName(server.gameKey, server.gameKey, t)}</span>
                    <span className="mt-0.5 block text-xs text-slate-500">{provider.label}</span>
                  </td>
                  {visibleColumns.has("players") ? <td className="px-3 py-2.5 font-mono text-slate-300">{typeof server.status.playersOnline === "number" ? `${server.status.playersOnline}/${gameServerMaxPlayers(server)}` : <DataMissing />}</td> : null}
                  {visibleColumns.has("resources") ? (
                    <td className="px-3 py-2.5 font-mono text-xs">
                      {metric?.statsAvailable ? <><span className="block text-slate-200">CPU {metric.cpuPercent.toFixed(1)}%</span><span className="mt-0.5 block text-slate-500">{formatMemory(metric.memoryMb)}</span></> : <DataMissing />}
                    </td>
                  ) : null}
                  {visibleColumns.has("address") ? (
                    <td className="px-3 py-2.5">
                      <span className="inline-flex items-center gap-1.5 font-mono text-slate-300">
                        {address}
                        <button className="flex size-7 items-center justify-center rounded text-slate-600 opacity-0 transition hover:bg-slate-800 hover:text-slate-200 focus:opacity-100 focus:outline-none focus-visible:ring-1 focus-visible:ring-panel-green group-hover:opacity-100" aria-label={t("copyServerAddress")} onClick={() => void copyText(address).then(onAddressCopied)} type="button"><Copy aria-hidden="true" className="size-3.5" /></button>
                      </span>
                    </td>
                  ) : null}
                  {visibleColumns.has("activity") ? <td className="px-3 py-2.5 text-xs text-slate-400" title={formatTimestamp(server.updatedAt, locale)}>{formatRelativeTime(server.updatedAt, locale)}</td> : null}
                  {visibleColumns.has("version") ? <td className="px-3 py-2.5 text-slate-400">{gameServerVersion(server)}</td> : null}
                  <td className="px-3 py-2.5"><ServerActions server={server} rowMode showInvite={false} showDelete /></td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <div className="divide-y divide-panel-line md:hidden">
        {servers.map((server) => {
          const metric = metricMap.get(server.id);
          const status = gameServerStatus(server);
          const address = formatAddress(publicHost, gameServerJoinPort(server));
          return (
            <article key={server.id} className={cn("p-3.5", selectedIds.has(server.id) && "bg-panel-green/[0.045]") }>
              <div className="flex items-start gap-3">
                <SelectionBox checked={selectedIds.has(server.id)} label={t("selectServer", { name: server.name })} onChange={() => {
                  const next = new Set(selectedIds);
                  if (next.has(server.id)) next.delete(server.id); else next.add(server.id);
                  onSelectionChange(next);
                }} />
                <ServerGameArt server={{ gameKey: server.gameKey, providerKey: server.providerKey, mode: gameServerMode(server) }} className="size-9 rounded" compact />
                <div className="min-w-0 flex-1">
                  <Link className="block truncate font-medium text-slate-100" href={`/servers/${server.id}`}>{server.name}</Link>
                  <div className="mt-1 flex items-center gap-2 text-xs text-slate-500"><ServerStatusIndicator status={status} /><span>·</span><span className="truncate">{serverProviderDisplay(server).label}</span></div>
                </div>
              </div>
              <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 border-y border-panel-line py-2.5 text-xs">
                <MobileMetric icon={<Users className="size-3.5" />} label={t("players")} value={typeof server.status.playersOnline === "number" ? `${server.status.playersOnline}/${gameServerMaxPlayers(server)}` : "—"} />
                <MobileMetric label={t("resources")} value={metric?.statsAvailable ? `${metric.cpuPercent.toFixed(1)}% · ${formatMemory(metric.memoryMb)}` : "—"} />
                <MobileMetric icon={<Plug className="size-3.5" />} label={t("serverAddress")} value={address} />
                <MobileMetric label={t("recentActivity")} value={formatRelativeTime(server.updatedAt, locale)} />
              </div>
              <div className="mt-3 flex items-center justify-between gap-3">
                <button className="inline-flex items-center gap-1.5 text-xs text-slate-400 hover:text-white" onClick={() => void copyText(address).then(onAddressCopied)} type="button"><Copy className="size-3.5" />{t("copyAddress")}</button>
                <ServerActions server={server} rowMode showInvite={false} showDelete />
              </div>
            </article>
          );
        })}
      </div>
    </div>
  );
}

function SortableHeader({ active, direction, label, onClick, className }: { active: boolean; direction: "asc" | "desc"; label: string; onClick: () => void; className?: string }) {
  return <th className={cn("px-3 py-2.5", className)}><button className={cn("inline-flex items-center gap-1.5 hover:text-slate-300 focus:outline-none focus-visible:text-panel-green", active && "text-slate-300")} onClick={onClick} type="button">{label}{active ? direction === "asc" ? <ArrowUp className="size-3" /> : <ArrowDown className="size-3" /> : null}</button></th>;
}

function MobileMetric({ icon, label, value }: { icon?: React.ReactNode; label: string; value: string }) {
  return <div className="min-w-0"><span className="flex items-center gap-1 text-slate-500">{icon}{label}</span><span className="mt-0.5 block truncate font-mono text-slate-200">{value}</span></div>;
}

function DataMissing() { return <span className="text-slate-600">—</span>; }
function formatMemory(value: number) { return value >= 1024 ? `${(value / 1024).toFixed(1)} GB` : `${Math.round(value)} MB`; }
function formatAddress(host: string | undefined, port: number) { const normalized = host?.trim(); if (!normalized) return `:${port}`; return normalized.includes(":") && !normalized.startsWith("[") ? `[${normalized}]:${port}` : `${normalized}:${port}`; }
function formatTimestamp(value: string, locale: string) { const date = new Date(value); return Number.isNaN(date.getTime()) ? "—" : new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", { dateStyle: "medium", timeStyle: "short" }).format(date); }
function formatRelativeTime(value: string, locale: string) { const date = new Date(value); if (Number.isNaN(date.getTime())) return "—"; const seconds = Math.round((date.getTime() - Date.now()) / 1000); const abs = Math.abs(seconds); const formatter = new Intl.RelativeTimeFormat(locale === "zh" ? "zh-CN" : "en-US", { numeric: "auto" }); if (abs < 60) return formatter.format(seconds, "second"); if (abs < 3600) return formatter.format(Math.round(seconds / 60), "minute"); if (abs < 86400) return formatter.format(Math.round(seconds / 3600), "hour"); return formatter.format(Math.round(seconds / 86400), "day"); }
