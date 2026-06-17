import type { TerrariaConfig } from "@gamepanel-lite/shared";
import type { ActivityEvent, Backup, GameCatalogEntry, ModFile, ModPack, ProviderKey, RecommendedMod, ResourceLimits, SaveSnapshotListResponse, Server, ServerJoinInfo, ServerPlayerListResponse, World } from "./types";

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
    return await Promise.race([apiFetch(input, { ...init, signal: controller.signal }), timeoutPromise]);
  } finally {
    if (timeout) {
      clearTimeout(timeout);
    }
  }
}

async function apiFetch(input: RequestInfo | URL, init: RequestInit = {}) {
  return fetch(input, { ...init, credentials: "include" });
}

async function readPayload<T>(response: Response, fallback: string): Promise<T> {
  const payload = (await response.json().catch(() => ({}))) as T & { error?: string };
  if (!response.ok) {
    throw new Error(payload.error ?? fallback);
  }
  return payload;
}

export type AuthAccount = {
  id: string;
  username: string;
};

export type AuthBootstrap = {
  initialized: boolean;
  account?: AuthAccount;
};

export async function getApiHealth(): Promise<{ status: string }> {
  const response = await fetchWithTimeout(`${API_BASE}/healthz`, { cache: "no-store" }, 3000, "API health check timed out");
  if (!response.ok) {
    throw new Error("Unable to load API health");
  }
  return (await response.json()) as { status: string };
}

export async function getAuthBootstrap(): Promise<AuthBootstrap> {
  const response = await apiFetch(`${API_BASE}/api/auth/bootstrap`, { cache: "no-store" });
  return readPayload<AuthBootstrap>(response, "Unable to load auth state");
}

export async function setupAdmin(username: string, password: string): Promise<AuthAccount> {
  const response = await apiFetch(`${API_BASE}/api/auth/setup`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password })
  });
  return readPayload<AuthAccount>(response, "Unable to create admin account");
}

export async function loginAdmin(username: string, password: string): Promise<AuthAccount> {
  const response = await apiFetch(`${API_BASE}/api/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password })
  });
  return readPayload<AuthAccount>(response, "Unable to sign in");
}

export async function logoutAdmin(): Promise<void> {
  const response = await apiFetch(`${API_BASE}/api/auth/logout`, { method: "POST" });
  await readPayload<{ status: string }>(response, "Unable to sign out");
}

export async function changeAdminPassword(currentPassword: string, newPassword: string): Promise<AuthAccount> {
  const response = await apiFetch(`${API_BASE}/api/auth/password`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ currentPassword, newPassword })
  });
  return readPayload<AuthAccount>(response, "Unable to change password");
}

export async function listGames(): Promise<GameCatalogEntry[]> {
  const response = await apiFetch(`${API_BASE}/api/games`, { cache: "no-store" });
  return readPayload<GameCatalogEntry[]>(response, "Unable to load game catalog");
}

export async function getGame(gameKey: string): Promise<GameCatalogEntry> {
  const response = await apiFetch(`${API_BASE}/api/games/${gameKey}`, { cache: "no-store" });
  return readPayload<GameCatalogEntry>(response, "Unable to load game");
}

export async function getGameVersions(gameKey: string): Promise<Record<ProviderKey, string[]>> {
  const response = await apiFetch(`${API_BASE}/api/games/${gameKey}/versions`, { cache: "no-store" });
  return readPayload<Record<ProviderKey, string[]>>(response, "Unable to load game versions");
}

export async function previewTerrariaConfig(config: TerrariaConfig): Promise<string> {
  const response = await apiFetch(`${API_BASE}/api/terraria/config/preview`, {
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
  gameKey?: string;
  providerKey: ProviderKey;
  status: Server["status"];
  worldName: string;
  playersOnline?: number;
  port: number;
  maxPlayers: number;
  password?: string;
  version?: string;
  lastError?: string;
  hostPort?: number;
  cpuLimitCores?: number;
  memoryLimitMb?: number;
  sourceWorldId?: string;
  sourceWorldName?: string;
  config?: TerrariaConfig;
  configPayload?: Record<string, unknown>;
  joinInfo?: Server["joinInfo"];
  configRevision?: number;
  appliedConfigRevision?: number;
};

type ApiWorld = {
  id: string;
  instanceId: string;
  providerKey?: ProviderKey;
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

function toBackup(backup: ApiBackup): Backup {
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

type ApiModFile = {
  id: string;
  instanceId: string;
  fileName: string;
  source?: string;
  workshopId?: string;
  modName?: string;
  title?: string;
  modVersion?: string;
  tmodVersion?: string;
  creatorSteamId?: string;
  previewUrl?: string;
  description?: string;
  tags?: string[];
  subscriptions?: number;
  favorited?: number;
  views?: number;
  updatedAtSteam?: number;
  sizeBytes: number;
  enabled: boolean;
  runtimeEnabled?: boolean;
  runtimePresent?: boolean;
  dependencies?: string[];
  createdAt: string;
};

type ApiRecommendedMod = {
  rank: number;
  workshopId: string;
  modName?: string;
  title: string;
  creatorSteamId?: string;
  previewUrl?: string;
  description?: string;
  fileSize: number;
  subscriptions?: number;
  favorited?: number;
  views?: number;
  timeCreated?: number;
  timeUpdated?: number;
  tags?: string[];
  dependencies?: string[];
  inLibrary: boolean;
  modId?: string;
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
    gameKey: server.gameKey,
    providerKey: server.providerKey,
    mode: server.providerKey === "terraria-tmodloader" ? "tmodloader" : "vanilla",
    status: server.status,
    world: server.worldName,
    players: server.playersOnline ?? 0,
    maxPlayers: server.maxPlayers,
    port: server.port,
    version: server.version ?? "1.4.5.6",
    hostPort: server.hostPort ?? 0,
    cpuLimitCores: server.cpuLimitCores ?? 0,
    memoryLimitMb: server.memoryLimitMb ?? 0,
    lastError: server.lastError,
    sourceWorldId: server.sourceWorldId,
    sourceWorldName: server.sourceWorldName,
    lastBackup: "Not yet",
    password: server.password ?? "",
    cpu: "0%",
    memory: "0 MB",
    config,
    configPayload: server.configPayload,
    joinInfo: server.joinInfo,
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
  const response = await apiFetch(`${API_BASE}/api/servers`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load servers");
  }
  const payload = (await response.json()) as ApiServer[];
  return payload.map(toServer);
}

export async function getServer(id: string): Promise<Server> {
  const response = await apiFetch(`${API_BASE}/api/servers/${id}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load server");
  }
  const server = (await response.json()) as ApiServer;
  return toServer(server);
}

export async function updateServerConfig(id: string, config: TerrariaConfig, hostPort?: number, resources?: ResourceLimits): Promise<Server> {
  const response = await apiFetch(`${API_BASE}/api/servers/${id}/config`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ config, hostPort, resources })
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

export type DockerStatus = {
  available: boolean;
  message: string;
  host: string;
  architecture?: string;
  lastCheckedAt?: string;
};

export type AppSettings = {
  host: string;
  port: string;
  dataDir: string;
  dbPath: string;
  dockerHost: string;
  publicHost: string;
};

export type ServerStats = {
  cpuPercent: number;
  memoryMb: number;
  memoryLimitMb: number;
};

export async function getServerStats(id: string): Promise<ServerStats> {
  const response = await apiFetch(`${API_BASE}/api/servers/${id}/stats`, { cache: "no-store" });
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
  const response = await apiFetch(`${API_BASE}/api/runtime/stats`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load runtime stats");
  }
  return (await response.json()) as HostStats;
}

export async function getServerLogSnapshot(id: string): Promise<string[]> {
  const response = await apiFetch(`${API_BASE}/api/servers/${id}/logs/snapshot`, { cache: "no-store" });
  const payload = (await response.json().catch(() => ({}))) as { lines?: string[]; error?: string };
  if (!response.ok) {
    throw new Error(payload.error ?? "Unable to load server logs");
  }
  return payload.lines ?? [];
}

export async function getSettings(): Promise<AppSettings> {
  const response = await apiFetch(`${API_BASE}/api/settings`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load settings");
  }
  return (await response.json()) as AppSettings;
}

export async function getTerrariaVersions(): Promise<Record<string, string[]>> {
  const response = await apiFetch(`${API_BASE}/api/terraria/versions`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load versions");
  }
  return (await response.json()) as Record<string, string[]>;
}

export async function createServer(input: {
  name: string;
  providerKey: ProviderKey;
  config: TerrariaConfig | Record<string, unknown>;
  hostPort?: number;
  version?: string;
  resources?: ResourceLimits;
}): Promise<Server> {
  const response = await apiFetch(`${API_BASE}/api/servers`, {
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
  const response = await apiFetch(`${API_BASE}/api/servers/${id}${action === "delete" ? "" : `/${action}`}`, {
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
  const response = await apiFetch(`${API_BASE}/api/servers/${id}/command`, {
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
  const response = await apiFetch(`${API_BASE}/api/worlds`, { cache: "no-store" });
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
  const response = await apiFetch(`${API_BASE}/api/worlds/import`, { method: "POST", body });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to import world");
  }
  const world = (await response.json()) as ApiWorld;
  return toWorld(world);
}

export async function createWorldSnapshot(serverId: string, name?: string): Promise<World> {
  const response = await apiFetch(`${API_BASE}/api/servers/${serverId}/world-snapshots`, {
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
  const response = await apiFetch(`${API_BASE}/api/worlds/${id}/assign`, {
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
  const response = await apiFetch(`${API_BASE}/api/worlds/${id}`, { method: "DELETE" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to delete world");
  }
}

export function worldDownloadUrl(id: string) {
  return `${API_BASE}/api/worlds/${id}/download`;
}

async function downloadBlob(url: string, fallbackMessage: string): Promise<Blob> {
  const response = await apiFetch(url);
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
  const response = await apiFetch(`${API_BASE}/api/backups`, { cache: "no-store" });
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
  const response = await apiFetch(`${API_BASE}/api/servers/${serverId}/backups`, { method: "POST" });
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
  const response = await apiFetch(`${API_BASE}/api/backups/${id}/restore`, { method: "POST" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to restore backup");
  }
}

export async function deleteBackup(id: string) {
  const response = await apiFetch(`${API_BASE}/api/backups/${id}`, { method: "DELETE" });
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
  const response = await apiFetch(`${API_BASE}/api/servers/${serverId}/mods`, { cache: "no-store" });
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
    source: file.source,
    workshopId: file.workshopId,
    modName: file.modName,
    title: file.title,
    modVersion: file.modVersion,
    tmodVersion: file.tmodVersion,
    creatorSteamId: file.creatorSteamId,
    previewUrl: file.previewUrl,
    description: file.description,
    tags: file.tags,
    subscriptions: file.subscriptions,
    favorited: file.favorited,
    views: file.views,
    updatedAtSteam: file.updatedAtSteam,
    size: formatBytes(file.sizeBytes),
    sizeBytes: file.sizeBytes,
    enabled: file.enabled,
    runtimeEnabled: file.runtimeEnabled,
    runtimePresent: file.runtimePresent,
    dependencies: file.dependencies,
    created: formatRelative(file.createdAt)
  };
}

function toRecommendedMod(mod: ApiRecommendedMod): RecommendedMod {
  return {
    rank: mod.rank,
    workshopId: mod.workshopId,
    modName: mod.modName,
    title: mod.title,
    creatorSteamId: mod.creatorSteamId,
    previewUrl: mod.previewUrl,
    description: mod.description,
    fileSize: mod.fileSize,
    size: formatBytes(mod.fileSize),
    subscriptions: mod.subscriptions,
    favorited: mod.favorited,
    views: mod.views,
    timeCreated: mod.timeCreated,
    timeUpdated: mod.timeUpdated,
    tags: mod.tags,
    dependencies: mod.dependencies,
    inLibrary: mod.inLibrary,
    modId: mod.modId
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
  const response = await apiFetch(`${API_BASE}/api/servers/${serverId}/mods/upload`, { method: "POST", body });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to upload mod");
  }
  const item = (await response.json()) as ApiModFile;
  return toModFile(item);
}

export async function importWorkshopMods(serverId: string, workshopIds: string[]): Promise<ModFile[]> {
  const response = await apiFetch(`${API_BASE}/api/servers/${serverId}/mods/workshop`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ workshopIds })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to import workshop mods");
  }
  const payload = (await response.json()) as ApiModFile[];
  return payload.map(toModFile);
}

export async function setModEnabled(serverId: string, modId: string, enabled: boolean): Promise<ModFile> {
  const response = await apiFetch(`${API_BASE}/api/servers/${serverId}/mods/${modId}`, {
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
  const response = await apiFetch(`${API_BASE}/api/servers/${serverId}/mods/${modId}`, { method: "DELETE" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to delete mod");
  }
}

export async function listGlobalMods(): Promise<ModFile[]> {
  const response = await apiFetch(`${API_BASE}/api/mods`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load mod library");
  }
  const payload = (await response.json()) as ApiModFile[];
  return payload.map(toModFile);
}

export async function uploadGlobalMod(file: File): Promise<ModFile> {
  const body = new FormData();
  body.set("file", file);
  const response = await apiFetch(`${API_BASE}/api/mods/upload`, { method: "POST", body });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to upload mod");
  }
  const item = (await response.json()) as ApiModFile;
  return toModFile(item);
}

export async function listRecommendedMods(): Promise<RecommendedMod[]> {
  const response = await apiFetch(`${API_BASE}/api/mods/recommended`, { cache: "no-store" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to load recommended mods");
  }
  const payload = (await response.json()) as ApiRecommendedMod[];
  return payload.map(toRecommendedMod);
}

export async function importGlobalWorkshopMods(workshopIds: string[]): Promise<ModFile[]> {
  const response = await apiFetch(`${API_BASE}/api/mods/workshop`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ workshopIds })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to import workshop mods");
  }
  const payload = (await response.json()) as ApiModFile[];
  return payload.map(toModFile);
}

export async function assignMod(modId: string, instanceId: string): Promise<ModFile> {
  const response = await apiFetch(`${API_BASE}/api/mods/${modId}/assign`, {
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
  const response = await apiFetch(`${API_BASE}/api/mods/${modId}`, { method: "DELETE" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to delete mod");
  }
}

export async function listModPacks(): Promise<ModPack[]> {
  const response = await apiFetch(`${API_BASE}/api/mod-packs`, { cache: "no-store" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to load mod packs");
  }
  const payload = (await response.json()) as ApiModPack[];
  return payload.map(toModPack);
}

export async function createModPack(input: { name: string; description?: string; modIds: string[] }): Promise<ModPack> {
  const response = await apiFetch(`${API_BASE}/api/mod-packs`, {
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

export async function updateModPack(id: string, input: { name: string; description?: string; modIds: string[] }): Promise<ModPack> {
  const response = await apiFetch(`${API_BASE}/api/mod-packs/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input)
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to update mod pack");
  }
  const pack = (await response.json()) as ApiModPack;
  return toModPack(pack);
}

export async function deleteModPack(id: string) {
  const response = await apiFetch(`${API_BASE}/api/mod-packs/${id}`, { method: "DELETE" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to delete mod pack");
  }
}

export async function listActivity(): Promise<ActivityEvent[]> {
  const response = await apiFetch(`${API_BASE}/api/activity`, { cache: "no-store" });
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

export async function updatePublicHost(publicHost: string): Promise<{ publicHost: string }> {
  const response = await apiFetch(`${API_BASE}/api/settings/public-host`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ publicHost })
  });
  return readPayload<{ publicHost: string }>(response, "Unable to update public host");
}

export async function getServerJoinInfo(id: string): Promise<ServerJoinInfo> {
  const response = await apiFetch(`${API_BASE}/api/servers/${id}/join-info`, { cache: "no-store" });
  return readPayload<ServerJoinInfo>(response, "Unable to load join info");
}

export async function listServerSaves(id: string): Promise<SaveSnapshotListResponse> {
  const response = await apiFetch(`${API_BASE}/api/servers/${id}/saves`, { cache: "no-store" });
  const payload = await readPayload<{ saveDisplayName: string; saves: ApiBackup[] }>(response, "Unable to load saves");
  return {
    saveDisplayName: payload.saveDisplayName,
    saves: payload.saves.map((backup) => toBackup(backup))
  };
}

export async function createServerSaveSnapshot(id: string): Promise<Backup> {
  const response = await apiFetch(`${API_BASE}/api/servers/${id}/saves/snapshot`, { method: "POST" });
  const payload = await readPayload<{ save: ApiBackup }>(response, "Unable to create save snapshot");
  return toBackup(payload.save);
}

export async function downloadServerSave(serverId: string, saveId: string): Promise<Blob> {
  const response = await apiFetch(`${API_BASE}/api/servers/${serverId}/saves/${saveId}/download`);
  if (!response.ok) {
    throw new Error("Unable to download save snapshot");
  }
  return response.blob();
}

export async function restoreServerSave(serverId: string, saveId: string): Promise<void> {
  const response = await apiFetch(`${API_BASE}/api/servers/${serverId}/saves/${saveId}/restore`, { method: "POST" });
  await readPayload<{ status: string }>(response, "Unable to restore save snapshot");
}

export async function listServerPlayers(id: string): Promise<ServerPlayerListResponse> {
  const response = await apiFetch(`${API_BASE}/api/servers/${id}/players`, { cache: "no-store" });
  return readPayload<ServerPlayerListResponse>(response, "Unable to load players");
}

export async function kickServerPlayer(id: string, player: string): Promise<void> {
  const response = await apiFetch(`${API_BASE}/api/servers/${id}/players/${encodeURIComponent(player)}/kick`, { method: "POST" });
  await readPayload<{ status: string }>(response, "Unable to kick player");
}

export async function banServerPlayer(id: string, player: string): Promise<void> {
  const response = await apiFetch(`${API_BASE}/api/servers/${id}/players/${encodeURIComponent(player)}/ban`, { method: "POST" });
  await readPayload<{ status: string }>(response, "Unable to ban player");
}
