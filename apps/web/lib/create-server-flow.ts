import type { TerrariaConfig } from "@gamepanel-lite/shared";
import { assignMod, assignWorld, createServer } from "./api";
import type { ProviderKey, ResourceLimits, Server, World } from "./types";

type CreateMode = "vanilla" | "tmodloader";

type CreateServerWithWorldDeps = {
  createServer: typeof createServer;
  assignWorld: typeof assignWorld;
  assignMod: typeof assignMod;
};

export type CreateServerWithWorldInput = {
  config: TerrariaConfig;
  configPayload?: Record<string, unknown>;
  deps?: CreateServerWithWorldDeps;
  hostPort?: number;
  mode: CreateMode;
  providerKey?: ProviderKey;
  resources?: ResourceLimits;
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
  assignMod
};

export async function createTerrariaServerWithWorld({
  config,
  configPayload,
  deps = defaultDeps,
  hostPort,
  mode,
  providerKey,
  resources,
  worldId,
  modIds = [],
  version
}: CreateServerWithWorldInput): Promise<CreatedServerWithWorld> {
  const nextProviderKey = providerKey ?? (mode === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla");
  let server = await deps.createServer({
    name: config.serverName || "Terraria Server",
    providerKey: nextProviderKey,
    config: configPayload ?? config,
    hostPort,
    resources,
    version
  });

  let assignedWorld: World | undefined;
  if (worldId) {
    assignedWorld = await deps.assignWorld(worldId, server.id);
    server = {
      ...server,
      sourceWorldId: assignedWorld.id,
      sourceWorldName: assignedWorld.name
    };
  }

  if (mode === "tmodloader" && modIds.length > 0) {
    await Promise.all(modIds.map((modId) => deps.assignMod(modId, server.id)));
  }

  return { assignedWorld, server };
}
