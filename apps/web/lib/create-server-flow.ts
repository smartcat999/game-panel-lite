import { assignWorld, createGameServer } from "./api";
import type { GameServerResource, ProviderKey, ResourceLimits, World } from "./types";

type CreateMode = "vanilla" | "tmodloader";

type CreateGameServerDeps = {
  createServer: typeof createGameServer;
  assignWorld: typeof assignWorld;
};

export type CreateGameServerInput = {
  config: Record<string, unknown>;
  deps?: CreateGameServerDeps;
  hostPort?: number;
  mode: CreateMode;
  name: string;
  providerKey?: ProviderKey;
  resources?: ResourceLimits;
  worldId?: string;
  modIds?: string[];
  version?: string;
};

export type CreatedGameServer = {
  assignedWorld?: World;
  server: GameServerResource;
};

const defaultDeps: CreateGameServerDeps = {
  assignWorld,
  createServer: createGameServer
};

export async function createGameServerWithResources({
  config,
  deps = defaultDeps,
  hostPort,
  mode,
  name,
  providerKey,
  resources,
  worldId,
  modIds = [],
  version
}: CreateGameServerInput): Promise<CreatedGameServer> {
  const nextProviderKey = providerKey ?? (mode === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla");
  let server = await deps.createServer({
    name: name || "Game Server",
    providerKey: nextProviderKey,
    config,
    hostPort,
    modIds: modIds.length > 0 ? modIds : undefined,
    resources,
    version
  });

  let assignedWorld: World | undefined;
  if (worldId) {
    assignedWorld = await deps.assignWorld(worldId, server.id);
    server = {
      ...server,
      spec: {
        ...server.spec,
        sourceWorldId: assignedWorld.id,
        sourceWorldName: assignedWorld.name
      }
    };
  }

  return { assignedWorld, server };
}
