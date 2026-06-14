import { describe, expect, it } from "vitest";
import { attachLatestBackupTimes } from "./server-metrics";
import type { Backup, Server } from "./types";

const server = {
  id: "server-1",
  name: "Friends",
  mode: "vanilla",
  status: "stopped",
  world: "Earth",
  players: 0,
  maxPlayers: 8,
  port: 7777,
  version: "1.4.4.9",
  hostPort: 7777,
  lastBackup: "Not yet",
  password: "",
  cpu: "0%",
  memory: "0 MB",
  config: {
    serverName: "Friends",
    worldName: "Earth",
    worldSize: "medium",
    worldEvil: "random",
    difficulty: "classic",
    maxPlayers: 8,
    port: 7777,
    password: "",
    motd: "",
    seed: "",
    secure: true,
    language: "en-US",
    autoCreateWorld: true
  }
} satisfies Server;

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
  it("attaches the newest backup time to matching servers", () => {
    const result = attachLatestBackupTimes([server], [
      backup("older", "1 h ago", "2026-06-14T01:00:00.000Z"),
      backup("newer", "Just now", "2026-06-14T02:00:00.000Z")
    ]);

    expect(result[0]?.lastBackup).toBe("Just now");
  });
});
