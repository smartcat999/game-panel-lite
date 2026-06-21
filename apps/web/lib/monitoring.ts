import {
  gameServerMaxPlayers,
  gameServerMode,
  gameServerStatus,
  gameServerVersion
} from "./game-server-resource";
import type { ActivityEvent, GameCatalogEntry, GameServerResource } from "./types";
import type { ObservabilityMetrics, ObservabilityServerMetric } from "./api";

export type MonitoringSeverity = "success" | "warning" | "error" | "info";

export type MonitoringEventType = "all" | "lifecycle" | "backup" | "player" | "failure" | "mods" | "world" | "other";

export type MonitoringHealth = {
  dockerRuntime: "healthy" | "degraded" | "down";
  failedTargets: number;
  lastSyncLabel: string;
  overall: "healthy" | "warning" | "critical";
  prometheusConnected: boolean;
};

export type MonitoringServerRow = {
  actionHref: string;
  cpuPercent: number;
  gameLabel: string;
  id: string;
  lastActive: string;
  memoryMb: number;
  memoryPercent: number;
  name: string;
  playersOnline: number;
  providerLabel: string;
  severity: MonitoringSeverity;
  status: string;
  version: string;
  maxPlayers: number;
};

export type MonitoringEvent = {
  id: string;
  kind: MonitoringEventType;
  operator: "system" | "local";
  rawType: string;
  searchText: string;
  serverName: string;
  severity: MonitoringSeverity;
  timestamp: string;
  title: string;
  typeLabel: string;
};

export type MonitoringModel = {
  events: MonitoringEvent[];
  health: MonitoringHealth;
  kpis: {
    issues: number;
    onlinePlayers: number;
    playerCapacity: number;
    resourceUsagePercent: number;
    runningServers: number;
    totalServers: number;
  };
  serverRows: MonitoringServerRow[];
  trends: {
    cpuPercent: number;
    eventCount: number;
    memoryLimitMb: number;
    memoryMb: number;
    playerCount: number;
  };
};

export type MonitoringTimeSeriesPoint = {
  timestamp: string;
  value: number;
};

export const severityOptions = ["all", "error", "warning", "success", "info"] as const;

export type MonitoringSeverityFilter = typeof severityOptions[number];

export const eventTypeOptions: MonitoringEventType[] = ["all", "lifecycle", "backup", "player", "failure", "mods", "world", "other"];

export function summarizeTimeSeries(data: MonitoringTimeSeriesPoint[]) {
  if (data.length === 0) {
    return { average: 0, peak: 0, samples: 0 };
  }
  const values = data.map((point) => point.value);
  return {
    average: values.reduce((sum, value) => sum + value, 0) / values.length,
    peak: Math.max(...values),
    samples: values.length
  };
}

export function createMonitoringModel({
  activity,
  displayByEventId,
  games,
  metrics,
  servers
}: {
  activity: ActivityEvent[];
  displayByEventId: Map<string, { message: string; typeLabel: string }>;
  games: GameCatalogEntry[];
  metrics?: ObservabilityMetrics;
  servers: GameServerResource[];
}): MonitoringModel {
  const metricServers = metrics?.servers ?? [];
  const serverById = new Map(servers.map((server) => [server.id, server]));
  const metricById = new Map(metricServers.map((server) => [server.id, server]));
  const gameByKey = new Map(games.map((game) => [game.key, game.name]));
  const rows = buildGameServerRows(servers, metricById, gameByKey);
  const events = activity.map((event) => {
    const display = displayByEventId.get(event.id);
    const server = event.instanceId ? serverById.get(event.instanceId) : undefined;
    const severity = eventSeverity(event.type);
    const kind = eventKind(event.type);
    const serverName = server?.name ?? event.instanceId ?? "System";
    const typeLabel = display?.typeLabel ?? event.type;
    const title = display?.message ?? event.message;
    return {
      id: event.id,
      kind,
      operator: "system" as const,
      rawType: event.type,
      searchText: [title, typeLabel, serverName, severity, kind].join(" ").toLowerCase(),
      serverName,
      severity,
      timestamp: event.created,
      title,
      typeLabel
    };
  });
  const failedTargets = rows.filter((row) => row.severity === "error").length + (metrics?.activity.failures ?? 0);
  const runningServers = rows.filter((row) => row.status === "running").length;
  const totalServers = rows.length;
  const onlinePlayers = rows.reduce((sum, row) => sum + row.playersOnline, 0);
  const playerCapacity = rows.reduce((sum, row) => sum + row.maxPlayers, 0);
  const cpuPercent = metrics?.host.totalCpuPercent ?? average(rows.map((row) => row.cpuPercent));
  const memoryMb = metrics?.host.totalMemoryMb ?? rows.reduce((sum, row) => sum + row.memoryMb, 0);
  const memoryLimitMb = Math.max(1024, metrics?.host.memoryLimitMb ?? rows.reduce((sum, row) => sum + row.memoryMb / Math.max(row.memoryPercent / 100, 0.01), 0));
  const resourceUsagePercent = Math.max(
    0,
    Math.min(100, Math.round((Math.min(cpuPercent, 100) + Math.min(memoryMb / memoryLimitMb * 100, 100)) / 2))
  );
  return {
    events,
    health: {
      dockerRuntime: metrics ? (metrics.host.runningWorkloads >= runningServers ? "healthy" : "degraded") : "down",
      failedTargets,
      lastSyncLabel: "Just now",
      overall: failedTargets > 0 ? "warning" : "healthy",
      prometheusConnected: Boolean(metrics)
    },
    kpis: {
      issues: failedTargets,
      onlinePlayers,
      playerCapacity,
      resourceUsagePercent,
      runningServers,
      totalServers
    },
    serverRows: rows,
    trends: {
      cpuPercent,
      eventCount: metrics?.activity.total ?? events.length,
      memoryLimitMb,
      memoryMb,
      playerCount: onlinePlayers
    }
  };
}

function buildGameServerRows(servers: GameServerResource[], metricById: Map<string, ObservabilityServerMetric>, gameByKey: Map<string, string>): MonitoringServerRow[] {
  return servers.map((server) => {
    const metric = metricById.get(server.id);
    const status = gameServerStatus(server);
    const cpuPercent = metric?.cpuPercent ?? 0;
    const memoryMb = metric?.memoryMb ?? 0;
    const memoryLimit = metric?.memoryLimitMb ?? server.spec.resources?.memoryLimitMb ?? 0;
    const memoryPercent = memoryLimit > 0 ? Math.min(100, memoryMb / memoryLimit * 100) : 0;
    const severity: MonitoringSeverity = status === "errored" || server.status.lastError ? "error" : cpuPercent > 80 || memoryPercent > 85 ? "warning" : status === "running" ? "success" : "info";
    return {
      actionHref: `/servers/${server.id}`,
      cpuPercent,
      gameLabel: gameByKey.get(server.gameKey ?? "") ?? server.gameKey ?? "Game",
      id: server.id,
      lastActive: "Just now",
      memoryMb,
      memoryPercent,
      name: server.name,
      playersOnline: metric?.playersOnline ?? server.status.playersOnline ?? 0,
      providerLabel: server.providerKey ?? gameServerMode(server),
      severity,
      status,
      version: metric?.version || gameServerVersion(server),
      maxPlayers: metric?.maxPlayers ?? gameServerMaxPlayers(server)
    };
  }).sort((a, b) => severityRank(b.severity) - severityRank(a.severity) || b.cpuPercent - a.cpuPercent);
}

function eventSeverity(type: string): MonitoringSeverity {
  if (type.includes("failed") || type.includes("failure") || type.includes("error")) return "error";
  if (type.includes("queued") || type.includes("deleted") || type.includes("stopped")) return "warning";
  if (type.includes("started") || type.includes("created") || type.includes("restored") || type.includes("updated")) return "success";
  return "info";
}

function eventKind(type: string): MonitoringEventType {
  if (type.includes("failed") || type.includes("failure") || type.includes("error")) return "failure";
  if (type.startsWith("backup.")) return "backup";
  if (type.startsWith("mod.")) return "mods";
  if (type.startsWith("world.")) return "world";
  if (type.includes("player")) return "player";
  if (type.startsWith("server.")) return "lifecycle";
  return "other";
}

function average(values: number[]) {
  if (values.length === 0) return 0;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function severityRank(severity: MonitoringSeverity) {
  return { error: 4, warning: 3, success: 2, info: 1 }[severity];
}
