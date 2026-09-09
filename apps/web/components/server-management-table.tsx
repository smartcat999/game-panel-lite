"use client";

import Link from "next/link";
import { ArrowDown, ArrowUp, ChevronRight, Copy, Plug, Users } from "lucide-react";
import { ServerActions } from "@/components/server-actions";
import { ServerGameArt } from "@/components/server-game-art";
import { ServerStatusIndicator } from "@/components/server-badges";
import { SelectionBox } from "@/components/selection-box";
import { copyText } from "@/lib/clipboard";
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
import { usePermissions } from "@/lib/permissions";

export type ServerTableColumn = "players" | "resources" | "address" | "activity" | "version";
export type ServerTableSort = "name" | "status" | "updatedAt";

export function ServerManagementTable({
  servers,
  nodes = [],
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
  nodes?: Array<{ id: string; name: string; region?: string; isLocal?: boolean }>;
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
  const { isViewer } = usePermissions();
  const metricMap = new Map(metrics.map((metric) => [metric.id, metric]));
  const nodeMap = new Map(nodes.map((n) => [n.id, n]));
  const allSelected = servers.length > 0 && servers.every((server) => selectedIds.has(server.id));
  const partiallySelected = !allSelected && servers.some((server) => selectedIds.has(server.id));
  const togglePage = () => {
    const next = new Set(selectedIds);
    if (allSelected) servers.forEach((server) => next.delete(server.id));
    else servers.forEach((server) => next.add(server.id));
    onSelectionChange(next);
  };

  return (
    <div className="overflow-hidden rounded-xl border micro-border bg-white subtle-elevation">
      <div className="hidden max-h-[calc(100vh-19rem)] overflow-auto md:block">
        <table className="w-full min-w-[1040px] table-fixed border-collapse text-left text-xs">
          <thead className="sticky top-0 z-10 border-b border-slate-100 bg-slate-50/80 text-[11px] font-semibold text-slate-400 select-none">
            <tr>
              {!isViewer && (
                <th className="w-10 px-3 py-2.5">
                  <SelectionBox checked={allSelected} indeterminate={partiallySelected} label={t("selectCurrentPage")} onChange={togglePage} />
                </th>
              )}
              <SortableHeader active={sort === "name"} direction={direction} label="NAME" onClick={() => onSort("name")} className="w-64" />
              <SortableHeader active={sort === "status"} direction={direction} label="STATUS" onClick={() => onSort("status")} className="w-24" />
              <th className="w-36 px-2.5 py-2.5">ENGINE</th>
              <th className="w-28 px-2.5 py-2.5">ENDPOINT</th>
              {visibleColumns.has("players") ? <th className="w-24 px-2.5 py-2.5">PLAYERS</th> : null}
              {visibleColumns.has("resources") ? <th className="w-40 px-2.5 py-2.5">MEMORY</th> : null}
              {visibleColumns.has("address") ? <th className="w-48 px-2.5 py-2.5">ADDRESS</th> : null}
              {visibleColumns.has("activity") ? <SortableHeader active={sort === "updatedAt"} direction={direction} label="ACTIVITY" onClick={() => onSort("updatedAt")} className="w-28" /> : null}
              {visibleColumns.has("version") ? <th className="w-24 px-2.5 py-2.5">VERSION</th> : null}
              <th className="w-32 px-3 py-2.5 text-right">ACTIONS</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 text-xs">
            {servers.map((server) => {
              const metric = metricMap.get(server.id);
              const status = gameServerStatus(server);
              const address = formatAddress(publicHost, gameServerJoinPort(server));
              const provider = serverProviderDisplay(server);
              const maxPlayers = gameServerMaxPlayers(server);
              return (
                <tr
                  key={server.id}
                  className={cn(
                    "group transition-colors hover:bg-slate-50/80",
                    selectedIds.has(server.id) && "bg-emerald-50/40"
                  )}
                >
                  {!isViewer && (
                    <td className="px-3 py-2.5">
                      <SelectionBox
                        checked={selectedIds.has(server.id)}
                        label={t("selectServer", { name: server.name })}
                        onChange={() => {
                          const next = new Set(selectedIds);
                          if (next.has(server.id)) next.delete(server.id);
                          else next.add(server.id);
                          onSelectionChange(next);
                        }}
                      />
                    </td>
                  )}
                  <td className="px-2.5 py-2.5">
                    <Link className="flex min-w-0 items-center gap-2" href={`/servers/${server.id}`}>
                      <ServerGameArt
                        server={{ gameKey: server.gameKey, providerKey: server.providerKey, mode: gameServerMode(server) }}
                        className="size-7 rounded"
                        compact
                      />
                      <span className="min-w-0">
                        <span className="block max-w-56 truncate font-mono font-bold text-slate-900 group-hover:text-emerald-600 transition">
                          {server.name}
                        </span>
                        <div className="mt-0.5 flex items-center gap-1.5 font-mono text-[10px] text-slate-400">
                          <span>{server.id.slice(0, 8)}</span>
                          <span>·</span>
                          {(() => {
                            const nodeInfo = server.nodeId ? nodeMap.get(server.nodeId) : undefined;
                            const isWorker = server.nodeId && server.nodeId !== "node-local";
                            const displayName = nodeInfo?.name || server.nodeId || "local";
                            return (
                              <span className="inline-flex items-center gap-1 rounded bg-slate-100 px-1 py-0.2 text-[9px] text-slate-500 font-medium">
                                {isWorker && <span className="size-1 rounded-full bg-sky-500 animate-pulse" />}
                                {displayName}
                              </span>
                            );
                          })()}
                        </div>
                      </span>
                    </Link>
                  </td>
                  <td className="px-2.5 py-2.5">
                    <ServerStatusIndicator status={status} />
                  </td>
                  <td className="px-2.5 py-2.5">
                    <span
                      className={cn(
                        "font-mono text-[11px] px-1.5 py-0.5 rounded border",
                        gameServerMode(server) === "tmodloader"
                          ? "bg-purple-50 text-purple-700 border-purple-200"
                          : "bg-slate-100 text-slate-600 border-slate-200/60"
                      )}
                    >
                      {provider.label}
                    </span>
                  </td>
                  <td className="px-2.5 py-2.5 font-mono text-[11px] text-slate-500">
                    :{gameServerJoinPort(server)}
                  </td>
                  {visibleColumns.has("players") ? (
                    <td className="px-2.5 py-2.5 font-mono text-slate-800">
                      {typeof server.status.playersOnline === "number" ? (
                        <span>
                          <span className="font-bold text-slate-900">{server.status.playersOnline}</span>{" "}
                          <span className="text-[10px] text-slate-400">max {maxPlayers}</span>
                        </span>
                      ) : (
                        <DataMissing />
                      )}
                    </td>
                  ) : null}
                  {visibleColumns.has("resources") ? (
                    <td className="px-2.5 py-2.5">
                      <ResourceUsage metric={metric} />
                    </td>
                  ) : null}
                  {visibleColumns.has("address") ? (
                    <td className="overflow-hidden px-2.5 py-2.5">
                      <span className="flex min-w-0 items-center gap-1.5 font-mono text-slate-600">
                        <span className="truncate text-xs" title={address}>
                          {address}
                        </span>
                        <button
                          className="flex size-5 shrink-0 items-center justify-center rounded text-slate-400 opacity-0 transition hover:bg-slate-100 hover:text-slate-800 focus:opacity-100 group-hover:opacity-100"
                          aria-label={t("copyServerAddress")}
                          onClick={() => void copyText(address).then(onAddressCopied)}
                          type="button"
                        >
                          <Copy aria-hidden="true" className="size-3" />
                        </button>
                      </span>
                    </td>
                  ) : null}
                  {visibleColumns.has("activity") ? (
                    <td className="px-2.5 py-2.5 text-[11px] text-slate-400" title={formatTimestamp(server.updatedAt, locale)}>
                      {formatRelativeTime(server.updatedAt, locale)}
                    </td>
                  ) : null}
                  {visibleColumns.has("version") ? (
                    <td className="px-2.5 py-2.5 font-mono text-[11px] text-slate-500">{gameServerVersion(server)}</td>
                  ) : null}
                  <td className="px-3 py-2.5 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <ServerActions server={server} rowMode showInvite={false} showDelete />
                      <Link
                        href={`/servers/${server.id}`}
                        className="text-slate-300 hover:text-slate-600 transition"
                        title="Detail"
                      >
                        <ChevronRight className="size-3.5" />
                      </Link>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* Mobile Card / Table Fallback */}
      <div className="divide-y divide-slate-100 md:hidden">
        {servers.map((server) => {
          const metric = metricMap.get(server.id);
          const status = gameServerStatus(server);
          const address = formatAddress(publicHost, gameServerJoinPort(server));
          const maxPlayers = gameServerMaxPlayers(server);
          return (
            <article key={server.id} className={cn("p-3.5", selectedIds.has(server.id) && "bg-emerald-50/40")}>
              <div className="flex items-start gap-3">
                <SelectionBox
                  checked={selectedIds.has(server.id)}
                  label={t("selectServer", { name: server.name })}
                  onChange={() => {
                    const next = new Set(selectedIds);
                    if (next.has(server.id)) next.delete(server.id);
                    else next.add(server.id);
                    onSelectionChange(next);
                  }}
                />
                <ServerGameArt
                  server={{ gameKey: server.gameKey, providerKey: server.providerKey, mode: gameServerMode(server) }}
                  className="size-8 rounded"
                  compact
                />
                <div className="min-w-0 flex-1">
                  <Link className="block truncate font-mono font-bold text-slate-900" href={`/servers/${server.id}`}>
                    {server.name}
                  </Link>
                  <div className="mt-1 flex items-center gap-2 text-xs text-slate-500">
                    <ServerStatusIndicator status={status} />
                    <span>·</span>
                    <span className="truncate">{serverProviderDisplay(server).label}</span>
                  </div>
                </div>
              </div>
              <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 border-y border-slate-100 py-2.5 text-xs">
                <MobileMetric
                  icon={<Users className="size-3.5" />}
                  label={t("players")}
                  value={
                    typeof server.status.playersOnline === "number"
                      ? `${server.status.playersOnline} (max ${maxPlayers})`
                      : "—"
                  }
                />
                <MobileResourceUsage metric={metric} />
                <MobileMetric icon={<Plug className="size-3.5" />} label={t("serverAddress")} value={address} />
                <MobileMetric label={t("recentActivity")} value={formatRelativeTime(server.updatedAt, locale)} />
              </div>
              <div className="mt-3 flex items-center justify-between gap-3">
                <button
                  className="inline-flex items-center gap-1.5 text-xs text-slate-500 hover:text-slate-900"
                  onClick={() => void copyText(address).then(onAddressCopied)}
                  type="button"
                >
                  <Copy className="size-3.5" />
                  {t("copyAddress")}
                </button>
                <ServerActions server={server} rowMode showInvite={false} showDelete />
              </div>
            </article>
          );
        })}
      </div>
    </div>
  );
}

function SortableHeader({
  active,
  direction,
  label,
  onClick,
  className
}: {
  active: boolean;
  direction: "asc" | "desc";
  label: string;
  onClick: () => void;
  className?: string;
}) {
  return (
    <th className={cn("px-2.5 py-2.5", className)}>
      <button
        className={cn(
          "inline-flex items-center gap-1 hover:text-slate-700 focus:outline-none",
          active && "text-slate-800 font-bold"
        )}
        onClick={onClick}
        type="button"
      >
        <span>{label}</span>
        {active ? direction === "asc" ? <ArrowUp className="size-3" /> : <ArrowDown className="size-3" /> : null}
      </button>
    </th>
  );
}

function MobileMetric({ icon, label, value }: { icon?: React.ReactNode; label: string; value: string }) {
  return (
    <div className="min-w-0">
      <span className="flex items-center gap-1 text-slate-400">{icon}{label}</span>
      <span className="mt-0.5 block truncate font-mono text-slate-800">{value}</span>
    </div>
  );
}

function MobileResourceUsage({ metric }: { metric?: ObservabilityServerMetric }) {
  const { t } = useI18n();
  return (
    <div className="min-w-0">
      <span className="text-slate-400">{t("resources")}</span>
      <div className="mt-0.5"><ResourceUsage metric={metric} /></div>
    </div>
  );
}

function ResourceUsage({ metric }: { metric?: ObservabilityServerMetric }) {
  if (!metric?.statsAvailable) return <DataMissing />;
  const usage = formatMemory(metric.memoryMb);
  const cap = metric.memoryLimitMb > 0 ? formatMemory(metric.memoryLimitMb) : null;
  return (
    <div className="flex flex-col text-xs font-mono text-slate-700 leading-tight">
      <div>
        <span className="font-bold text-slate-900">{usage}</span>
        {cap && <span className="text-[10px] text-slate-400 ml-1">cap {cap}</span>}
      </div>
      <div className="text-[10px] text-slate-400">
        <span>CPU {metric.cpuPercent.toFixed(1)}%</span>
      </div>
    </div>
  );
}

function DataMissing() {
  return <span className="text-slate-300 font-mono">—</span>;
}

function formatMemory(value: number) {
  return value >= 1024 ? `${(value / 1024).toFixed(1)} GB` : `${Math.round(value)} MB`;
}

function formatAddress(host: string | undefined, port: number) {
  const normalized = host?.trim();
  if (!normalized) return `:${port}`;
  return normalized.includes(":") && !normalized.startsWith("[") ? `[${normalized}]:${port}` : `${normalized}:${port}`;
}

function formatTimestamp(value: string, locale: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? "—"
    : new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", { dateStyle: "medium", timeStyle: "short" }).format(date);
}

function formatRelativeTime(value: string, locale: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  const seconds = Math.round((date.getTime() - Date.now()) / 1000);
  const abs = Math.abs(seconds);
  const formatter = new Intl.RelativeTimeFormat(locale === "zh" ? "zh-CN" : "en-US", { numeric: "auto" });
  if (abs < 60) return formatter.format(seconds, "second");
  if (abs < 3600) return formatter.format(Math.round(seconds / 60), "minute");
  if (abs < 86400) return formatter.format(Math.round(seconds / 3600), "hour");
  return formatter.format(Math.round(seconds / 86400), "day");
}
