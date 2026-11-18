import { describe, expect, it } from "vitest";
import { attachLatestBackupTimesToGameServers } from "./server-metrics";
import type { Backup, GameServerResource } from "./types";

const gameServer = {
  id: "server-1",
  name: "Friends",
  gameKey: "terraria",
  providerKey: "terraria-vanilla",
  spec: {
    generation: 1,
    desiredState: "stopped",
    version: "1.4.5.6",
    config: {
      serverName: "Friends",
      worldName: "Earth",
      maxPlayers: 8,
      port: 7777
    },
    resources: {
      cpuLimitCores: 0,
      memoryLimitMb: 0
    },
    network: {
      port: 7777,
      hostPort: 7777
    }
  },
  status: {
    phase: "stopped",
    actualState: "stopped",
    playersOnline: 0,
    observedGeneration: 1,
    appliedGeneration: 1
  },
  createdAt: "2026-06-14T00:00:00.000Z",
  updatedAt: "2026-06-14T00:00:00.000Z"
} satisfies GameServerResource;

function backup(id: string, created: string, createdAt: string): Backup {
  return {
    id,
    instanceId: "server-1",
    name: `${id}.zip`,
    server: "server-1",
    world: "Earth",
    type: "Manual",
    size: "1 KB",
    sizeBytes: 1024,
    created,
    createdAt
  };
}

describe("server metrics", () => {
  it("attaches the newest backup object to matching game server resources", () => {
    const result = attachLatestBackupTimesToGameServers([gameServer], [
      backup("older", "1 h ago", "2026-06-14T01:00:00.000Z"),
      backup("newer", "Just now", "2026-06-14T02:00:00.000Z")
    ]);

    expect(result[0]?.latestBackup?.id).toBe("newer");
  });
});
