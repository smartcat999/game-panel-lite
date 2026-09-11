import { describe, expect, it } from "vitest";
import { dashboardNodeMetrics, dashboardResourceTotals } from "./dashboard-metrics";
import type { ComputeNode, GameServerResource } from "./types";

describe("dashboardResourceTotals", () => {
  it("counts configured limits without inventing defaults for unlimited servers", () => {
    const servers = [
      { spec: { resources: { cpuLimitCores: 2, memoryLimitMb: 4096 } } },
      { spec: { resources: { cpuLimitCores: 0, memoryLimitMb: 0 } } },
      { spec: {} }
    ] as GameServerResource[];

    expect(dashboardResourceTotals(servers)).toEqual({ cpuCores: 2, memoryMb: 4096 });
  });
});

describe("dashboardNodeMetrics", () => {
  const localNode = {
    id: "node-local",
    isLocal: true,
    status: "online",
    cpuCores: 2,
    cpuUsagePercent: 0,
    memoryTotalMb: 7800,
    memoryUsedMb: 2048
  } as ComputeNode;

  it("uses live Docker host metrics for the local node", () => {
    expect(dashboardNodeMetrics(localNode, {
      runningWorkloads: 2,
      cpuCores: 4,
      totalCpuPercent: 47.4,
      totalMemoryMb: 5018,
      memoryLimitMb: 8192,
      storageUsedBytes: 0
    })).toEqual({
      cpuCores: 4,
      cpuUsagePercent: 47.4,
      memoryTotalMb: 8192,
      memoryUsedMb: 5018,
      runningCount: 2
    });
  });

  it("uses the live workload count instead of the stored server total for the local node", () => {
    expect(dashboardNodeMetrics({ ...localNode, runningCount: 8 }, {
      runningWorkloads: 2,
      cpuCores: 2,
      totalCpuPercent: 45,
      totalMemoryMb: 5018,
      memoryLimitMb: 7800,
      storageUsedBytes: 0
    })).toMatchObject({ runningCount: 2 });
  });

  it("keeps heartbeat metrics for remote nodes", () => {
    const remoteNode = {
      ...localNode,
      id: "node-worker",
      isLocal: false,
      cpuUsagePercent: 23,
      memoryUsedMb: 3072
    };

    expect(dashboardNodeMetrics(remoteNode, {
      runningWorkloads: 0,
      cpuCores: 8,
      totalCpuPercent: 90,
      totalMemoryMb: 7000,
      memoryLimitMb: 8192,
      storageUsedBytes: 0
    })).toEqual({
      cpuCores: 2,
      cpuUsagePercent: 23,
      memoryTotalMb: 7800,
      memoryUsedMb: 3072,
      runningCount: 0
    });
  });

  it("does not present stale usage from an offline node", () => {
    expect(dashboardNodeMetrics({
      ...localNode,
      id: "node-offline",
      isLocal: false,
      status: "offline",
      cpuUsagePercent: 61,
      memoryUsedMb: 4096
    })).toEqual({
      cpuCores: 2,
      cpuUsagePercent: null,
      memoryTotalMb: 7800,
      memoryUsedMb: null,
      runningCount: null
    });
  });
});
