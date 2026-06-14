import { describe, expect, it, vi } from "vitest";
import type { TerrariaConfig } from "@gamepanel-lite/shared";
import type { Server, World } from "./types";
import { createTerrariaServerWithWorld } from "./create-server-flow";

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
  secure: true,
  language: "en-US",
  autoCreateWorld: true
};

const server: Server = {
  id: "server-1",
  name: "Friends",
  mode: "vanilla",
  status: "stopped",
  world: "PresetWorld",
  players: 0,
  maxPlayers: 8,
  port: 7777,
  version: "1.4.4.9",
  hostPort: 7777,
  lastBackup: "Never",
  password: "",
  cpu: "0%",
  memory: "0 MB",
  config
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

const migratedWorld: World = {
  ...importedWorld,
  id: "migrated-world-1",
  server: "server-1"
};

describe("createTerrariaServerWithWorld", () => {
  it("copies the selected world into the new server before assigning it", async () => {
    const deps = {
      createServer: vi.fn().mockResolvedValue(server),
      migrateWorld: vi.fn().mockResolvedValue(migratedWorld),
      assignWorld: vi.fn().mockResolvedValue(importedWorld),
      assignMod: vi.fn()
    };

    const result = await createTerrariaServerWithWorld({
      config,
      mode: "vanilla",
      worldId: "world-1",
      deps
    });

    expect(deps.migrateWorld).toHaveBeenCalledWith("world-1", "server-1");
    expect(deps.assignWorld).toHaveBeenCalledWith("migrated-world-1", "server-1");
    expect(result.server.world).toBe("UploadedWorld");
    expect(result.server.config.worldName).toBe("UploadedWorld");
  });

  it("creates a server without world assignment when no worldId is given", async () => {
    const deps = {
      createServer: vi.fn().mockResolvedValue(server),
      migrateWorld: vi.fn(),
      assignWorld: vi.fn(),
      assignMod: vi.fn()
    };

    const result = await createTerrariaServerWithWorld({
      config,
      mode: "vanilla",
      deps
    });

    expect(deps.assignWorld).not.toHaveBeenCalled();
    expect(deps.migrateWorld).not.toHaveBeenCalled();
    expect(result.server.world).toBe("PresetWorld");
  });
});
