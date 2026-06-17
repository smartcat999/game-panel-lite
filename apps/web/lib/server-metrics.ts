import type { Backup, Server } from "./types";

export function attachLatestBackupTimes(servers: Server[], backups: Backup[]) {
  const latestByServer = new Map<string, Backup>();
  for (const backup of backups) {
    const serverId = backup.instanceId ?? backup.server;
    if (!serverId) {
      continue;
    }
    const current = latestByServer.get(serverId);
    if (!current || new Date(backup.createdAt).getTime() > new Date(current.createdAt).getTime()) {
      latestByServer.set(serverId, backup);
    }
  }
  return servers.map((server) => ({
    ...server,
    lastBackup: latestByServer.get(server.id)?.created ?? server.lastBackup
  }));
}
