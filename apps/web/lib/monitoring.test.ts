import { describe, expect, it } from "vitest";
import { createMonitoringModel } from "./monitoring";
import type { ActivityEvent, GameCatalogEntry, GameServerResource } from "./types";
import type { ObservabilityMetrics } from "./api";

const game = {
  key: "terraria",
  name: "Terraria",
  description: "",
  status: "available",
  providers: []
} satisfies GameCatalogEntry;

const server = {
  id: "server-1",
  name: "Friends",
  gameKey: "terraria",
  providerKey: "terraria-vanilla",
  spec: {
    generation: 2,
    desiredState: "running",
    version: "1.4.5.6",
    config: {
      serverName: "Friends",
      worldName: "Earth",
      maxPlayers: 8,
      port: 7777
    },
    resources: {
      cpuLimitCores: 2,
      memoryLimitMb: 2048
    },
    network: {
      port: 7777,
      hostPort: 17777
    }
  },
  status: {
    phase: "running",
    actualState: "running",
    playersOnline: 3,
    observedGeneration: 2,
    appliedGeneration: 2
  },
  createdAt: "2026-06-14T00:00:00.000Z",
  updatedAt: "2026-06-14T00:00:00.000Z"
} satisfies GameServerResource;

const metrics = {
  collectedAt: "2026-06-14T02:00:00.000Z",
  host: {
    runningWorkloads: 1,
    totalCpuPercent: 40,
    totalMemoryMb: 512,
    memoryLimitMb: 4096,
    storageUsedBytes: 0
  },
  servers: [
    {
      id: "server-1",
      name: "Friends",
      gameKey: "terraria",
      providerKey: "terraria-vanilla",
      status: "running",
      playersOnline: 4,
      maxPlayers: 8,
      hostPort: 17777,
      version: "1.4.5.6",
      cpuPercent: 35,
      memoryMb: 512,
      memoryLimitMb: 2048,
      statsAvailable: true
    }
  ],
  activity: {
    windowHours: 24,
    total: 1,
    lifecycle: 1,
    backups: 0,
    players: 0,
    failures: 0,
    byType: []
  }
} satisfies ObservabilityMetrics;

const activity = [
  {
    id: "event-1",
    instanceId: "server-1",
    type: "server.started",
    message: "Server started",
    created: "2026-06-14T02:00:00.000Z"
  }
] satisfies ActivityEvent[];

describe("monitoring model", () => {
  it("builds dashboard rows and events directly from game server resources", () => {
    const model = createMonitoringModel({
      activity,
      displayByEventId: new Map([["event-1", { message: "Friends started", typeLabel: "Started" }]]),
      games: [game],
      metrics,
      servers: [server]
    });

    expect(model.kpis.runningServers).toBe(1);
    expect(model.kpis.onlinePlayers).toBe(4);
    expect(model.serverRows[0]).toMatchObject({
      id: "server-1",
      gameLabel: "Terraria",
      playersOnline: 4,
      status: "running",
      version: "1.4.5.6"
    });
    expect(model.events[0]).toMatchObject({
      serverName: "Friends",
      severity: "success",
      title: "Friends started"
    });
  });
});
