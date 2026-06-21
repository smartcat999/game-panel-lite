import { describe, expect, it, vi } from "vitest";
import type { TerrariaConfig } from "@gamepanel-lite/shared";
import type { GameServerResource, World } from "./types";
import { createGameServerWithResources } from "./create-server-flow";

const config: TerrariaConfig = {
  serverName: "Friends",
  worldName: "PresetWorld",
  worldSize: "medium",
  worldEvil: "random",
  difficulty: "classic",
  maxPlayers: 8,
  port: 7777,
  password: "",
  motd: "",
  seed: "",
  specialSeeds: [],
  secretSeeds: [],
  secure: true,
  language: "en-US",
  autoCreateWorld: true
};

const server: GameServerResource = {
  id: "server-1",
  name: "Friends",
  gameKey: "terraria",
  providerKey: "terraria-vanilla",
  spec: {
    generation: 1,
    desiredState: "stopped",
    version: "1.4.5.6",
    config,
    network: { port: 7777, hostPort: 7777 }
  },
  status: {
    phase: "stopped",
    actualState: "stopped",
    observedGeneration: 1,
    appliedGeneration: 1
  },
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString()
};

const importedWorld: World = {
  id: "world-1",
  name: "UploadedWorld",
  size: "uploaded.wld",
  difficulty: "Imported",
  server: "server-1",
  modified: "Just now",
  bytes: "1 KB"
};

describe("createGameServerWithResources", () => {
  it("assigns the selected reusable world snapshot without overriding the requested world name", async () => {
    const deps = {
      createServer: vi.fn().mockResolvedValue(server),
      assignWorld: vi.fn().mockResolvedValue(importedWorld),
      assignMod: vi.fn()
    };

    const result = await createGameServerWithResources({
      config,
      mode: "vanilla",
      name: "Friends",
      worldId: "world-1",
      deps
    });

    expect(deps.assignWorld).toHaveBeenCalledWith("world-1", "server-1");
    expect(result.server.spec.config?.worldName).toBe("PresetWorld");
    expect(result.server.spec.sourceWorldId).toBe("world-1");
    expect(result.server.spec.sourceWorldName).toBe("UploadedWorld");
  });

  it("creates a server without world assignment when no worldId is given", async () => {
    const deps = {
      createServer: vi.fn().mockResolvedValue(server),
      assignWorld: vi.fn(),
      assignMod: vi.fn()
    };

    const result = await createGameServerWithResources({
      config,
      mode: "vanilla",
      name: "Friends",
      deps
    });

    expect(deps.assignWorld).not.toHaveBeenCalled();
    expect(result.server.spec.config?.worldName).toBe("PresetWorld");
  });

  it("passes the requested external port to server creation", async () => {
    const deps = {
      createServer: vi.fn().mockResolvedValue(server),
      assignWorld: vi.fn(),
      assignMod: vi.fn()
    };

    await createGameServerWithResources({
      config,
      hostPort: 17777,
      mode: "vanilla",
      name: "Friends",
      deps
    });

    expect(deps.createServer).toHaveBeenCalledWith(expect.objectContaining({ hostPort: 17777 }));
  });

  it("creates non-Terraria servers from provider payload without Terraria-shaped config", async () => {
    const deps = {
      createServer: vi.fn().mockResolvedValue({
        ...server,
        gameKey: "palworld",
        providerKey: "palworld",
        spec: {
          ...server.spec,
          config: { serverName: "Pal Friends", saveName: "Pal Save", maxPlayers: 10 }
        }
      }),
      assignWorld: vi.fn(),
      assignMod: vi.fn()
    };

    await createGameServerWithResources({
      config: { serverName: "Pal Friends", saveName: "Pal Save", maxPlayers: 10 },
      mode: "vanilla",
      name: "Pal Friends",
      providerKey: "palworld",
      deps
    });

    expect(deps.createServer).toHaveBeenCalledWith(expect.objectContaining({
      name: "Pal Friends",
      providerKey: "palworld",
      config: { serverName: "Pal Friends", saveName: "Pal Save", maxPlayers: 10 }
    }));
  });
});
