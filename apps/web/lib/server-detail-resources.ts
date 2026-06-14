import type { Server } from "./types";

export function getDetailTargetServers(servers: Server[], currentServerId: string): Server[] {
  return servers.filter((server) => server.id !== currentServerId);
}

export function nextWorldCopyName(worldName: string, suffix: string): string {
  return `${worldName} ${suffix}`.trim();
}
