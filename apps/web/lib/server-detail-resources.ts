import { gameKeyFromProvider } from "./game-filters";
import type { GameKey, GameServerResource, ProviderKey, World } from "./types";

export function providerKeyForServer(server: Pick<GameServerResource, "providerKey">): ProviderKey {
  return server.providerKey;
}

export function gameKeyForServer(server: Pick<GameServerResource, "gameKey" | "providerKey">): GameKey | undefined {
  return server.gameKey ?? gameKeyFromProvider(server.providerKey);
}

export function gameKeyForWorld(world: Pick<World, "gameKey" | "providerKey">): GameKey | undefined {
  return world.gameKey ?? gameKeyFromProvider(world.providerKey);
}

export function isWorldCompatibleWithServer(world: World, server: Pick<GameServerResource, "gameKey" | "providerKey">): boolean {
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


export function serverTopologyPorts(server: Pick<GameServerResource, "providerKey" | "spec">): { internal: string; external: string } {
  const internalPort = server.spec.network?.port ?? 0;
  const hostPort = server.spec.network?.hostPort ?? 0;
  if (server.providerKey === "project-zomboid") {
    const firstHostPort = hostPort || internalPort || 16261;
    return { internal: "16261, 16262 / udp", external: `${firstHostPort}, ${firstHostPort + 1} / udp` };
  }
  return {
    internal: `${internalPort || 7777} / ${server.spec.network?.protocol || "tcp"}`,
    external: String(hostPort || internalPort || 7777)
  };
}
