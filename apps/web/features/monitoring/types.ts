export type MonitoringDataSource = {
  kind: "prometheus";
  connected: boolean;
  endpointConfigured: boolean;
  lastQueryAt: string | null;
  error?: string;
};

export type MonitoringRange = {
  range: string;
  step: string;
  start: string;
  end: string;
};

export type MetricPoint = {
  timestamp: string;
  value: number;
};

export type MetricSeries = {
  key: "cpu" | "memory" | "players" | "events" | "uptime" | "requests" | "errors" | "latencyP95" | "sse" | string;
  title: string;
  unit: "%" | "MB" | "players" | "events" | "s" | "req/s" | "errors/s" | "ms" | "connections" | string;
  chartType: "line" | "area" | "bar";
  currentValue: number | null;
  avg: number | null;
  max: number | null;
  threshold?: number;
  points: MetricPoint[];
  emptyReason?: "prometheus_unconfigured" | "prometheus_unavailable" | "no_samples" | "server_stopped" | string;
};

export type MonitoringOverviewResponse = {
  collectedAt: string;
  dataSource: MonitoringDataSource;
  health: {
    overall: "healthy" | "warning" | "critical" | "unknown";
    prometheusConnected: boolean;
    dockerRuntime: "healthy" | "degraded" | "unknown" | string;
    lastSync: string | null;
    failedTargets: number;
  };
  kpis: {
    totalServers: number;
    runningServers: number;
    issues: number;
    onlinePlayers: number;
    playerCapacity: number;
    resourceUsagePercent: number;
    storageBytes: number;
  };
};

export type MonitoringMetricsResponse = {
  collectedAt: string;
  range: MonitoringRange;
  dataSource: MonitoringDataSource;
  series: Record<string, MetricSeries>;
};

export type ServerLoadRow = {
  serverId: string;
  serverName: string;
  gameKey: string;
  providerKey: string;
  version?: string;
  status: string;
  cpuPercent: number;
  memoryMb: number;
  memoryLimitMb: number;
  playersOnline: number;
  maxPlayers: number;
  lastActive: string;
  severity: "normal" | "warning" | "critical";
};

export type ServerLoadResponse = {
  collectedAt: string;
  dataSource: MonitoringDataSource;
  rows: ServerLoadRow[];
};

export type MonitoringEvent = {
  id: string;
  severity: "success" | "warning" | "error" | "info";
  type: string;
  title: string;
  message: string;
  serverId?: string;
  serverName?: string;
  operator: string;
  timestamp: string;
  metadata?: Record<string, string>;
};

export type MonitoringEventsResponse = {
  collectedAt: string;
  events: MonitoringEvent[];
};

export type PlatformService = {
  name: string;
  status: "healthy" | "down" | "degraded" | string;
  instance?: string;
  lastError?: string;
};

export type RouteMetric = {
  route: string;
  method: string;
  requestRate: number;
  errorRate: number;
  p95Ms: number;
};

export type PlatformResponse = {
  collectedAt: string;
  range: MonitoringRange;
  dataSource: MonitoringDataSource;
  services: PlatformService[];
  series: Record<string, MetricSeries>;
  topRoutes: RouteMetric[];
};
