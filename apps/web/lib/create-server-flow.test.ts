import { describe, expect, it, vi } from "vitest";
import type { TerrariaConfig } from "@gamepanel-lite/shared";
import type { Server, World } from "./types";
import { createTerrariaServerWithAssets } from "./create-server-flow";

const config: TerrariaConfig = {
  serverName: "Friends",
  worldName: "PresetWorld",
  worldSize: "medium",
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

describe("createTerrariaServerWithAssets", () => {
  it("assigns an uploaded world to the newly created server", async () => {
    const deps = {
      createServer: vi.fn().mockResolvedValue(server),
      importWorld: vi.fn().mockResolvedValue(importedWorld),
      assignWorld: vi.fn().mockResolvedValue(importedWorld),
      uploadMod: vi.fn()
    };
    const worldFile = new File(["world"], "uploaded.wld");

    const result = await createTerrariaServerWithAssets({
      config,
      mode: "vanilla",
      worldFile,
      modFiles: [],
      deps
    });

    expect(deps.importWorld).toHaveBeenCalledWith(worldFile, "server-1");
    expect(deps.assignWorld).toHaveBeenCalledWith("world-1", "server-1");
    expect(result.server.world).toBe("UploadedWorld");
    expect(result.server.config.worldName).toBe("UploadedWorld");
  });
});
