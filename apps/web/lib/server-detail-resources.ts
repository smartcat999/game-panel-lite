import { gameKeyFromProvider } from "./game-filters";
import type { GameKey, ProviderKey, Server, World } from "./types";

export function providerKeyForServer(server: Pick<Server, "mode" | "providerKey">): ProviderKey {
  if (server.providerKey) return server.providerKey;
  return server.mode === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla";
}

export function gameKeyForServer(server: Pick<Server, "gameKey" | "providerKey">): GameKey | undefined {
  return server.gameKey ?? gameKeyFromProvider(server.providerKey);
}

export function gameKeyForWorld(world: Pick<World, "gameKey" | "providerKey">): GameKey | undefined {
  return world.gameKey ?? gameKeyFromProvider(world.providerKey);
}

export function isWorldCompatibleWithServer(world: World, server: Pick<Server, "mode" | "gameKey" | "providerKey">): boolean {
  const serverProvider = providerKeyForServer(server);
  if (world.providerKey) return world.providerKey === serverProvider;

  const worldGame = gameKeyForWorld(world);
  if (!worldGame) return true;

  return worldGame === gameKeyForServer(server);
}

export function getWorldSourceServerId(world: World): string | undefined {
  return world.instanceId && world.instanceId !== "unassigned" ? world.instanceId : undefined;
}

export function isWorldActiveOnServer(world: World, serverId?: string): boolean {
  if (!serverId) return Boolean(world.activeInstanceId);
  return world.activeInstanceId === serverId;
}
