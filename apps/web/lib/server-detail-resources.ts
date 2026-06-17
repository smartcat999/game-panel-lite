import type { ProviderKey, Server, World } from "./types";

export function providerKeyForServer(server: Pick<Server, "mode">): ProviderKey {
  return server.mode === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla";
}

export function isWorldCompatibleWithServer(world: World, server: Pick<Server, "mode">): boolean {
  return !world.providerKey || world.providerKey === providerKeyForServer(server);
}

export function getWorldSourceServerId(world: World): string | undefined {
  return world.instanceId && world.instanceId !== "unassigned" ? world.instanceId : undefined;
}

export function isWorldActiveOnServer(world: World, serverId?: string): boolean {
  if (!serverId) return Boolean(world.activeInstanceId);
  return world.activeInstanceId === serverId;
}
