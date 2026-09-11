import type { HostStats } from "@/lib/api";
import type { ComputeNode, GameServerResource } from "@/lib/types";

export type DashboardResourceTotals = {
  cpuCores: number;
  memoryMb: number;
};

export type DashboardNodeMetrics = {
  cpuCores: number;
  cpuUsagePercent: number | null;
  memoryTotalMb: number;
  memoryUsedMb: number | null;
  runningCount: number | null;
};

export function dashboardResourceTotals(servers: GameServerResource[]): DashboardResourceTotals {
  return servers.reduce<DashboardResourceTotals>(
    (total, server) => ({
      cpuCores: total.cpuCores + Math.max(0, server.spec?.resources?.cpuLimitCores ?? 0),
      memoryMb: total.memoryMb + Math.max(0, server.spec?.resources?.memoryLimitMb ?? 0)
    }),
    { cpuCores: 0, memoryMb: 0 }
  );
}

export function dashboardNodeMetrics(node: ComputeNode, localHost?: HostStats): DashboardNodeMetrics {
  if (node.isLocal && localHost) {
    return {
      cpuCores: localHost.cpuCores || node.cpuCores,
      cpuUsagePercent: clampPercent(localHost.totalCpuPercent),
      memoryTotalMb: localHost.memoryLimitMb || node.memoryTotalMb,
      memoryUsedMb: Math.max(0, localHost.totalMemoryMb),
      runningCount: Math.max(0, localHost.runningWorkloads)
    };
  }

  const hasFreshHeartbeat = node.status === "online";
  return {
    cpuCores: node.cpuCores,
    cpuUsagePercent: hasFreshHeartbeat && typeof node.cpuUsagePercent === "number" ? clampPercent(node.cpuUsagePercent) : null,
    memoryTotalMb: node.memoryTotalMb,
    memoryUsedMb: hasFreshHeartbeat && typeof node.memoryUsedMb === "number" ? Math.max(0, node.memoryUsedMb) : null,
    runningCount: hasFreshHeartbeat ? Math.max(0, node.runningCount || 0) : null
  };
}

function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(100, Math.max(0, value));
}
