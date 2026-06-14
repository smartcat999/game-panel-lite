import type { TerrariaConfig } from "@gamepanel-lite/shared";
import { assignMod, assignWorld, createServer, migrateWorld } from "./api";
import type { Server, World } from "./types";

type CreateMode = "vanilla" | "tmodloader";

type CreateServerWithWorldDeps = {
  createServer: typeof createServer;
  migrateWorld: typeof migrateWorld;
  assignWorld: typeof assignWorld;
  assignMod: typeof assignMod;
};

export type CreateServerWithWorldInput = {
  config: TerrariaConfig;
  deps?: CreateServerWithWorldDeps;
  mode: CreateMode;
  worldId?: string;
  modIds?: string[];
  version?: string;
};

export type CreatedServerWithWorld = {
  assignedWorld?: World;
  server: Server;
};

const defaultDeps: CreateServerWithWorldDeps = {
  assignWorld,
  createServer,
  migrateWorld,
  assignMod
};

export async function createTerrariaServerWithWorld({
  config,
  deps = defaultDeps,
  mode,
  worldId,
  modIds = [],
  version
}: CreateServerWithWorldInput): Promise<CreatedServerWithWorld> {
  let server = await deps.createServer({
    name: config.serverName || "Terraria Server",
    providerKey: mode === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla",
    config,
    version
  });

  let assignedWorld: World | undefined;
  if (worldId) {
    const copiedWorld = await deps.migrateWorld(worldId, server.id);
    assignedWorld = await deps.assignWorld(copiedWorld.id, server.id);
    server = {
      ...server,
      config: {
        ...server.config,
        worldName: assignedWorld.name
      },
      world: assignedWorld.name
    };
  }

  if (mode === "tmodloader" && modIds.length > 0) {
    await Promise.all(modIds.map((modId) => deps.assignMod(modId, server.id)));
  }

  return { assignedWorld, server };
}
