import { describe, expect, it } from "vitest";
import {
  gameKeyForServer,
  gameKeyForWorld,
  getWorldSourceServerId,
  isWorldCompatibleWithServer,
  isWorldActiveOnServer,
  providerKeyForServer
} from "./server-detail-resources";
import type { GameServerResource, World } from "./types";

const baseServer: GameServerResource = {
  id: "server-1",
  name: "Friends Server",
  gameKey: "terraria",
  providerKey: "terraria-vanilla",
  spec: {
    generation: 1,
    desiredState: "stopped",
    version: "1.4.5.6",
    config: { worldName: "Home", maxPlayers: 8, port: 7777 },
    network: { port: 7777, hostPort: 7777 }
  },
  status: {
    phase: "stopped",
    actualState: "stopped",
    observedGeneration: 1,
    appliedGeneration: 1
  },
  createdAt: "2026-06-20T00:00:00Z",
  updatedAt: "2026-06-20T00:00:00Z"
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
    expect(isWorldCompatibleWithServer(vanillaWorld, { ...baseServer, providerKey: "terraria-tmodloader" })).toBe(false);
  });

  it("uses explicit provider metadata before legacy Terraria mode fallback", () => {
    const palworldServer: GameServerResource = {
      ...baseServer,
      gameKey: "palworld",
      providerKey: "palworld",
      spec: { ...baseServer.spec, config: { saveName: "Pal Save" } }
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
    const minecraftServer: GameServerResource = {
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
