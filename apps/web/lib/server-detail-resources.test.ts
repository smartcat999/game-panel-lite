import { describe, expect, it } from "vitest";
import {
  gameKeyForServer,
  gameKeyForWorld,
  getWorldSourceServerId,
  isWorldCompatibleWithServer,
  isWorldActiveOnServer,
  providerKeyForServer
} from "./server-detail-resources";
import type { Server, World } from "./types";

const baseServer: Server = {
  id: "server-1",
  name: "Friends Server",
  mode: "vanilla",
  status: "stopped",
  world: "Home",
  players: 0,
  maxPlayers: 8,
  port: 7777,
  version: "1.4.4.9",
  hostPort: 7777,
  cpuLimitCores: 0,
  memoryLimitMb: 0,
  lastBackup: "Not yet",
  password: "",
  cpu: "0%",
  memory: "0 MB",
  config: {
    serverName: "Friends Server",
    worldName: "Home",
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
};

describe("server detail resource helpers", () => {
  it("matches reusable worlds by server provider type", () => {
    const vanillaWorld: World = {
      id: "world-1",
      instanceId: "source-server",
      providerKey: "terraria-vanilla",
      name: "Home",
      size: "Home.wld",
      difficulty: "Imported",
      modified: "Just now",
      bytes: "1 KB"
    };

    expect(providerKeyForServer(baseServer)).toBe("terraria-vanilla");
    expect(isWorldCompatibleWithServer(vanillaWorld, baseServer)).toBe(true);
    expect(isWorldCompatibleWithServer(vanillaWorld, { ...baseServer, mode: "tmodloader" })).toBe(false);
  });

  it("uses explicit provider metadata before legacy Terraria mode fallback", () => {
    const palworldServer: Server = {
      ...baseServer,
      gameKey: "palworld",
      providerKey: "palworld",
      mode: "vanilla",
      world: "Pal Save"
    };
    const palworldSave: World = {
      id: "save-1",
      instanceId: "source-server",
      gameKey: "palworld",
      providerKey: "palworld",
      name: "Pal Save",
      size: "Pal Save",
      difficulty: "Saved",
      modified: "Just now",
      bytes: "1 KB"
    };
    const terrariaWorld: World = {
      ...palworldSave,
      id: "world-2",
      gameKey: "terraria",
      providerKey: "terraria-vanilla"
    };

    expect(providerKeyForServer(palworldServer)).toBe("palworld");
    expect(gameKeyForServer(palworldServer)).toBe("palworld");
    expect(gameKeyForWorld(palworldSave)).toBe("palworld");
    expect(isWorldCompatibleWithServer(palworldSave, palworldServer)).toBe(true);
    expect(isWorldCompatibleWithServer(terrariaWorld, palworldServer)).toBe(false);
  });

  it("can fall back to game metadata for older world records without provider keys", () => {
    const minecraftServer: Server = {
      ...baseServer,
      gameKey: "minecraft",
      providerKey: "minecraft"
    };
    const minecraftSave: World = {
      id: "save-1",
      instanceId: "source-server",
      gameKey: "minecraft",
      name: "Block Save",
      size: "Block Save",
      difficulty: "Saved",
      modified: "Just now",
      bytes: "1 KB"
    };

    expect(isWorldCompatibleWithServer(minecraftSave, minecraftServer)).toBe(true);
  });

  it("keeps world ownership separate from active world state", () => {
    const world: World = {
      id: "world-1",
      instanceId: "server-1",
      activeInstanceId: "",
      name: "Home",
      size: "Home.wld",
      difficulty: "Imported",
      server: "server-1",
      modified: "Just now",
      bytes: "1 KB"
    };

    expect(getWorldSourceServerId(world)).toBe("server-1");
    expect(isWorldActiveOnServer(world, "server-1")).toBe(false);
    expect(isWorldActiveOnServer({ ...world, activeInstanceId: "server-1" }, "server-1")).toBe(true);
  });
});
