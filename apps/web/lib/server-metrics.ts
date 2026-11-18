import type { Backup, GameServerResource } from "./types";

export function attachLatestBackupTimesToGameServers(servers: GameServerResource[], backups: Backup[]) {
  const latestByServer = latestBackupByServer(backups);
  return servers.map((server) => ({
    ...server,
    latestBackup: latestByServer.get(server.id)
  }));
}

function latestBackupByServer(backups: Backup[]) {
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
  return latestByServer;
}
