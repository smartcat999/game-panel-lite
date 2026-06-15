import type { TerrariaConfig } from "@gamepanel-lite/shared";
import type { ActivityEvent, Backup, ModFile, ModPack, Server, World } from "./types";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:4000";
const DOCKER_CHECK_TIMEOUT_MS = 5000;

async function fetchWithTimeout(
  input: RequestInfo | URL,
  init: RequestInit = {},
  timeoutMs = DOCKER_CHECK_TIMEOUT_MS,
  timeoutMessage = "Request timed out"
) {
  const controller = new AbortController();
  let timeout: ReturnType<typeof setTimeout> | undefined;
  const timeoutPromise = new Promise<never>((_, reject) => {
    timeout = setTimeout(() => {
      controller.abort();
      reject(new Error(timeoutMessage));
    }, timeoutMs);
  });
  try {
    return await Promise.race([fetch(input, { ...init, signal: controller.signal }), timeoutPromise]);
  } finally {
    if (timeout) {
      clearTimeout(timeout);
    }
  }
}

export async function getApiHealth(): Promise<{ status: string }> {
  const response = await fetchWithTimeout(`${API_BASE}/healthz`, { cache: "no-store" }, 3000, "API health check timed out");
  if (!response.ok) {
    throw new Error("Unable to load API health");
  }
  return (await response.json()) as { status: string };
}

export async function previewTerrariaConfig(config: TerrariaConfig): Promise<string> {
  const response = await fetch(`${API_BASE}/api/terraria/config/preview`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ config })
  });
  const payload = (await response.json()) as { serverconfig?: string; error?: string };
  if (!response.ok) {
    throw new Error(payload.error ?? "Unable to render Terraria config");
  }
  return payload.serverconfig ?? "";
}

type ApiServer = {
  id: string;
  name: string;
  providerKey: "terraria-vanilla" | "terraria-tmodloader";
  status: Server["status"];
  worldName: string;
  playersOnline?: number;
  port: number;
  maxPlayers: number;
  password?: string;
  version?: string;
  lastError?: string;
  hostPort?: number;
  sourceWorldId?: string;
  sourceWorldName?: string;
  config?: TerrariaConfig;
  configRevision?: number;
  appliedConfigRevision?: number;
};

type ApiWorld = {
  id: string;
  instanceId: string;
  providerKey?: "terraria-vanilla" | "terraria-tmodloader";
  name: string;
  fileName: string;
  sizeBytes: number;
  source?: string;
  config?: TerrariaConfig;
  activeInstanceId?: string;
  updatedAt?: string;
  createdAt: string;
};

type ApiBackup = {
  id: string;
  instanceId: string;
  fileName: string;
  worldName: string;
  sizeBytes: number;
  type: "Auto" | "Manual";
  createdAt: string;
};

type ApiModFile = {
  id: string;
  instanceId: string;
  fileName: string;
  sizeBytes: number;
  enabled: boolean;
  createdAt: string;
};

type ApiModPack = {
  id: string;
  name: string;
  description: string;
  modIds: string[];
  mods: ApiModFile[];
  createdAt: string;
};

type ApiActivityEvent = {
  id: string;
  instanceId?: string;
  type: string;
  message: string;
  createdAt: string;
};

export function formatBytes(bytes: number) {
  if (bytes < 1024 * 1024) {
    return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  }
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function formatRelative(value?: string) {
  if (!value) {
    return "Unknown";
  }
  const timestamp = new Date(value).getTime();
  if (Number.isNaN(timestamp)) {
    return "Unknown";
  }
  const diff = Math.max(0, Date.now() - timestamp);
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return "Just now";
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} h ago`;
  return `${Math.floor(hours / 24)} d ago`;
}

function configFromServer(server: ApiServer): TerrariaConfig {
  return {
    serverName: server.config?.serverName ?? server.name,
    worldName: server.config?.worldName ?? server.worldName,
    worldSize: server.config?.worldSize ?? "medium",
    worldEvil: server.config?.worldEvil ?? "random",
    difficulty: server.config?.difficulty ?? "classic",
    maxPlayers: server.config?.maxPlayers ?? server.maxPlayers,
    port: server.config?.port ?? server.port,
    password: server.config?.password ?? server.password ?? "",
    motd: server.config?.motd ?? "",
    seed: server.config?.seed ?? "",
    secure: server.config?.secure ?? true,
    language: server.config?.language ?? "en-US",
    autoCreateWorld: server.config?.autoCreateWorld ?? true
  };
}

function toServer(server: ApiServer): Server {
  const config = configFromServer(server);
  const configRevision = server.configRevision ?? 0;
  const appliedConfigRevision = server.appliedConfigRevision ?? 0;
  return {
    id: server.id,
    name: server.name,
    mode: server.providerKey === "terraria-tmodloader" ? "tmodloader" : "vanilla",
    status: server.status,
    world: server.worldName,
    players: server.playersOnline ?? 0,
    maxPlayers: server.maxPlayers,
    port: server.port,
    version: server.version ?? "1.4.5.6",
    hostPort: server.hostPort ?? 0,
    lastError: server.lastError,
    sourceWorldId: server.sourceWorldId,
    sourceWorldName: server.sourceWorldName,
    lastBackup: "Not yet",
    password: server.password ?? "",
    cpu: "0%",
    memory: "0 MB",
    config,
    configPendingRestart: server.status === "running" && configRevision > appliedConfigRevision
  };
}

function toWorld(world: ApiWorld): World {
  return {
    id: world.id,
    instanceId: world.instanceId,
    activeInstanceId: world.activeInstanceId,
    providerKey: world.providerKey,
    name: world.name,
    size: world.fileName,
    difficulty: "Imported",
    server: world.activeInstanceId || (world.instanceId !== "unassigned" ? world.instanceId : undefined),
    modified: formatRelative(world.updatedAt ?? world.createdAt),
    bytes: formatBytes(world.sizeBytes),
    source: world.source,
    config: world.config
  };
}

export async function listServers(): Promise<Server[]> {
  const response = await fetch(`${API_BASE}/api/servers`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load servers");
  }
  const payload = (await response.json()) as ApiServer[];
  return payload.map(toServer);
}

export async function getServer(id: string): Promise<Server> {
  const response = await fetch(`${API_BASE}/api/servers/${id}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load server");
  }
  const server = (await response.json()) as ApiServer;
  return toServer(server);
}

export async function updateServerConfig(id: string, config: TerrariaConfig, hostPort?: number): Promise<Server> {
  const response = await fetch(`${API_BASE}/api/servers/${id}/config`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ config, hostPort })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to update server config");
  }
  const server = (await response.json()) as ApiServer;
  return toServer(server);
}

export async function getDockerStatus(): Promise<DockerStatus> {
  const response = await fetchWithTimeout(`${API_BASE}/api/runtime/docker`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load Docker status");
  }
  return (await response.json()) as DockerStatus;
}

export type DockerHostCandidate = {
  host: string;
  label: string;
  source: string;
  exists: boolean;
  active: boolean;
};

export type DockerStatus = {
  available: boolean;
  message: string;
  host: string;
  lastCheckedAt?: string;
};

export type AppSettings = {
  host: string;
  port: string;
  dataDir: string;
  dbPath: string;
  dockerHost: string;
};

export type ServerStats = {
  cpuPercent: number;
  memoryMb: number;
  memoryLimitMb: number;
};

export async function getServerStats(id: string): Promise<ServerStats> {
  const response = await fetch(`${API_BASE}/api/servers/${id}/stats`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load server stats");
  }
  return (await response.json()) as ServerStats;
}

export function serverLogsUrl(id: string) {
  return `${API_BASE}/api/servers/${id}/logs`;
}

export type HostStats = {
  runningContainers: number;
  totalCpuPercent: number;
  totalMemoryMb: number;
  memoryLimitMb: number;
};

export async function getRuntimeStats(): Promise<HostStats> {
  const response = await fetch(`${API_BASE}/api/runtime/stats`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load runtime stats");
  }
  return (await response.json()) as HostStats;
}

export async function getServerLogSnapshot(id: string): Promise<string[]> {
  const response = await fetch(`${API_BASE}/api/servers/${id}/logs/snapshot`, { cache: "no-store" });
  const payload = (await response.json().catch(() => ({}))) as { lines?: string[]; error?: string };
  if (!response.ok) {
    throw new Error(payload.error ?? "Unable to load server logs");
  }
  return payload.lines ?? [];
}

export async function getSettings(): Promise<AppSettings> {
  const response = await fetch(`${API_BASE}/api/settings`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load settings");
  }
  return (await response.json()) as AppSettings;
}

export async function updateSettings(settings: Partial<Pick<AppSettings, "dockerHost">>): Promise<AppSettings> {
  const response = await fetch(`${API_BASE}/api/settings`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(settings)
  });
  const payload = (await response.json().catch(() => ({}))) as AppSettings & { error?: string };
  if (!response.ok) {
    throw new Error(payload.error ?? "Unable to update settings");
  }
  return payload;
}

export async function getDockerHosts(): Promise<{ currentHost: string; candidates: DockerHostCandidate[] }> {
  const response = await fetchWithTimeout(`${API_BASE}/api/runtime/docker/hosts`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load Docker host candidates");
  }
  return (await response.json()) as { currentHost: string; candidates: DockerHostCandidate[] };
}

export async function applyDockerHost(host: string): Promise<{ available: boolean; message: string; host: string }> {
  const response = await fetchWithTimeout(`${API_BASE}/api/runtime/docker/host`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ host })
  });
  const payload = (await response.json().catch(() => ({}))) as { available?: boolean; message?: string; host?: string; error?: string };
  if (!response.ok) {
    throw new Error(payload.error ?? "Unable to apply Docker host");
  }
  return {
    available: Boolean(payload.available),
    message: payload.message ?? "",
    host: payload.host ?? host
  };
}

export async function getTerrariaVersions(): Promise<Record<string, string[]>> {
  const response = await fetch(`${API_BASE}/api/terraria/versions`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load versions");
  }
  return (await response.json()) as Record<string, string[]>;
}

export async function createServer(input: {
  name: string;
  providerKey: "terraria-vanilla" | "terraria-tmodloader";
  config: TerrariaConfig;
  hostPort?: number;
  version?: string;
}): Promise<Server> {
  const response = await fetch(`${API_BASE}/api/servers`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input)
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to create server");
  }
  const server = (await response.json()) as ApiServer;
  return toServer(server);
}

export async function serverAction(id: string, action: "start" | "stop" | "restart" | "delete"): Promise<Server | null> {
  const response = await fetch(`${API_BASE}/api/servers/${id}${action === "delete" ? "" : `/${action}`}`, {
    method: action === "delete" ? "DELETE" : "POST"
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? `Unable to ${action} server`);
  }
  if (action === "delete") {
    return null;
  }
  const server = (await response.json()) as ApiServer;
  return toServer(server);
}

export async function sendServerCommand(id: string, command: string) {
  const response = await fetch(`${API_BASE}/api/servers/${id}/command`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ command })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to send command");
  }
}

export async function listWorlds(): Promise<World[]> {
  const response = await fetch(`${API_BASE}/api/worlds`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load worlds");
  }
  const payload = (await response.json()) as ApiWorld[];
  return payload.map(toWorld);
}

export async function importWorld(file: File, instanceId = "unassigned"): Promise<World> {
  const body = new FormData();
  body.set("file", file);
  body.set("instanceId", instanceId);
  const response = await fetch(`${API_BASE}/api/worlds/import`, { method: "POST", body });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to import world");
  }
  const world = (await response.json()) as ApiWorld;
  return toWorld(world);
}

export async function createWorldSnapshot(serverId: string, name?: string): Promise<World> {
  const response = await fetch(`${API_BASE}/api/servers/${serverId}/world-snapshots`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to save world snapshot");
  }
  const world = (await response.json()) as ApiWorld;
  return toWorld(world);
}

export async function assignWorld(id: string, instanceId: string): Promise<World> {
  const response = await fetch(`${API_BASE}/api/worlds/${id}/assign`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ instanceId })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to assign world");
  }
  const world = (await response.json()) as ApiWorld;
  return toWorld(world);
}

export async function deleteWorld(id: string) {
  const response = await fetch(`${API_BASE}/api/worlds/${id}`, { method: "DELETE" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to delete world");
  }
}

export function worldDownloadUrl(id: string) {
  return `${API_BASE}/api/worlds/${id}/download`;
}

async function downloadBlob(url: string, fallbackMessage: string): Promise<Blob> {
  const response = await fetch(url);
  if (!response.ok) {
    const payload = (await response.clone().json().catch(() => ({}))) as { error?: string };
    if (payload.error) {
      throw new Error(payload.error);
    }
    const text = await response.text().catch(() => "");
    throw new Error(text.trim() || fallbackMessage);
  }
  return response.blob();
}

export async function downloadWorldFile(id: string): Promise<Blob> {
  return downloadBlob(worldDownloadUrl(id), "Unable to download world");
}

export async function listBackups(): Promise<Backup[]> {
  const response = await fetch(`${API_BASE}/api/backups`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load backups");
  }
  const payload = (await response.json()) as ApiBackup[];
  return payload.map((backup) => ({
    id: backup.id,
    instanceId: backup.instanceId,
    name: backup.fileName,
    server: backup.instanceId,
    world: backup.worldName,
    type: backup.type,
    size: formatBytes(backup.sizeBytes),
    sizeBytes: backup.sizeBytes,
    created: formatRelative(backup.createdAt),
    createdAt: backup.createdAt
  }));
}

export async function createBackup(serverId: string): Promise<Backup> {
  const response = await fetch(`${API_BASE}/api/servers/${serverId}/backups`, { method: "POST" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to create backup");
  }
  const backup = (await response.json()) as ApiBackup;
  return {
    id: backup.id,
    instanceId: backup.instanceId,
    name: backup.fileName,
    server: backup.instanceId,
    world: backup.worldName,
    type: backup.type,
    size: formatBytes(backup.sizeBytes),
    sizeBytes: backup.sizeBytes,
    created: formatRelative(backup.createdAt),
    createdAt: backup.createdAt
  };
}

export async function restoreBackup(id: string) {
  const response = await fetch(`${API_BASE}/api/backups/${id}/restore`, { method: "POST" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to restore backup");
  }
}

export async function deleteBackup(id: string) {
  const response = await fetch(`${API_BASE}/api/backups/${id}`, { method: "DELETE" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to delete backup");
  }
}

export function backupDownloadUrl(id: string) {
  return `${API_BASE}/api/backups/${id}/download`;
}

export async function downloadBackupFile(id: string): Promise<Blob> {
  return downloadBlob(backupDownloadUrl(id), "Unable to download backup");
}

export async function listMods(serverId: string): Promise<ModFile[]> {
  const response = await fetch(`${API_BASE}/api/servers/${serverId}/mods`, { cache: "no-store" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to load mods");
  }
  const payload = (await response.json()) as ApiModFile[];
  return payload.map(toModFile);
}

function toModFile(file: ApiModFile): ModFile {
  return {
    id: file.id,
    instanceId: file.instanceId,
    fileName: file.fileName,
    size: formatBytes(file.sizeBytes),
    sizeBytes: file.sizeBytes,
    enabled: file.enabled,
    created: formatRelative(file.createdAt)
  };
}

function toModPack(pack: ApiModPack): ModPack {
  return {
    id: pack.id,
    name: pack.name,
    description: pack.description,
    modIds: pack.modIds,
    mods: pack.mods.map(toModFile),
    created: formatRelative(pack.createdAt)
  };
}

export async function uploadMod(serverId: string, file: File): Promise<ModFile> {
  const body = new FormData();
  body.set("file", file);
  const response = await fetch(`${API_BASE}/api/servers/${serverId}/mods/upload`, { method: "POST", body });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to upload mod");
  }
  const item = (await response.json()) as ApiModFile;
  return toModFile(item);
}

export async function setModEnabled(serverId: string, modId: string, enabled: boolean): Promise<ModFile> {
  const response = await fetch(`${API_BASE}/api/servers/${serverId}/mods/${modId}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to update mod");
  }
  const item = (await response.json()) as ApiModFile;
  return toModFile(item);
}

export async function deleteMod(serverId: string, modId: string) {
  const response = await fetch(`${API_BASE}/api/servers/${serverId}/mods/${modId}`, { method: "DELETE" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to delete mod");
  }
}

export async function listGlobalMods(): Promise<ModFile[]> {
  const response = await fetch(`${API_BASE}/api/mods`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load mod library");
  }
  const payload = (await response.json()) as ApiModFile[];
  return payload.map(toModFile);
}

export async function uploadGlobalMod(file: File): Promise<ModFile> {
  const body = new FormData();
  body.set("file", file);
  const response = await fetch(`${API_BASE}/api/mods/upload`, { method: "POST", body });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to upload mod");
  }
  const item = (await response.json()) as ApiModFile;
  return toModFile(item);
}

export async function assignMod(modId: string, instanceId: string): Promise<ModFile> {
  const response = await fetch(`${API_BASE}/api/mods/${modId}/assign`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ instanceId })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to assign mod");
  }
  const item = (await response.json()) as ApiModFile;
  return toModFile(item);
}

export async function deleteGlobalMod(modId: string) {
  const response = await fetch(`${API_BASE}/api/mods/${modId}`, { method: "DELETE" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to delete mod");
  }
}

export async function listModPacks(): Promise<ModPack[]> {
  const response = await fetch(`${API_BASE}/api/mod-packs`, { cache: "no-store" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to load mod packs");
  }
  const payload = (await response.json()) as ApiModPack[];
  return payload.map(toModPack);
}

export async function createModPack(input: { name: string; description?: string; modIds: string[] }): Promise<ModPack> {
  const response = await fetch(`${API_BASE}/api/mod-packs`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input)
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to create mod pack");
  }
  const pack = (await response.json()) as ApiModPack;
  return toModPack(pack);
}

export async function deleteModPack(id: string) {
  const response = await fetch(`${API_BASE}/api/mod-packs/${id}`, { method: "DELETE" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to delete mod pack");
  }
}

export async function listActivity(): Promise<ActivityEvent[]> {
  const response = await fetch(`${API_BASE}/api/activity`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load activity");
  }
  const payload = (await response.json()) as ApiActivityEvent[];
  return payload.map((event) => ({
    id: event.id,
    instanceId: event.instanceId,
    type: event.type,
    message: event.message,
    created: formatRelative(event.createdAt)
  }));
}
