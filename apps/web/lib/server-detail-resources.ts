import type { Server } from "./types";

export function getDetailTargetServers(servers: Server[], currentServerId: string): Server[] {
  return servers.filter((server) => server.id !== currentServerId);
}

export function getMigrationTargetServers(servers: Server[], sourceServerId?: string): Server[] {
  if (!sourceServerId) return servers;
  return servers.filter((server) => server.id !== sourceServerId);
}

export function resolveMigrationTargetId(servers: Server[], selectedTargetId: string, sourceServerId?: string): string {
  const targets = getMigrationTargetServers(servers, sourceServerId);
  if (targets.some((server) => server.id === selectedTargetId)) {
    return selectedTargetId;
  }
  return targets[0]?.id ?? "";
}

export function nextWorldCopyName(worldName: string, suffix: string): string {
  return `${worldName} ${suffix}`.trim();
}
