import type { TerrariaConfig } from "@gamepanel-lite/shared";
import type { Backup, ModFile, Server, World } from "./types";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:4000";

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
  status: "running" | "stopped" | "errored" | "creating" | "restarting";
  worldName: string;
  port: number;
  maxPlayers: number;
  password?: string;
};

type ApiWorld = {
  id: string;
  instanceId: string;
  name: string;
  fileName: string;
  sizeBytes: number;
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

function formatBytes(bytes: number) {
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

export async function listServers(): Promise<Server[]> {
  const response = await fetch(`${API_BASE}/api/servers`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load servers");
  }
  const payload = (await response.json()) as ApiServer[];
  return payload.map((server) => ({
    id: server.id,
    name: server.name,
    mode: server.providerKey === "terraria-tmodloader" ? "tmodloader" : "vanilla",
    status: server.status === "running" ? "running" : server.status === "errored" ? "errored" : "stopped",
    world: server.worldName,
    players: 0,
    maxPlayers: server.maxPlayers,
    port: server.port,
    version: "1.4.4.9",
    lastBackup: "Not yet",
    password: server.password ?? "",
    cpu: "0%",
    memory: "0 MB"
  }));
}

export async function getServer(id: string): Promise<Server> {
  const response = await fetch(`${API_BASE}/api/servers/${id}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load server");
  }
  const server = (await response.json()) as ApiServer;
  return {
    id: server.id,
    name: server.name,
    mode: server.providerKey === "terraria-tmodloader" ? "tmodloader" : "vanilla",
    status: server.status === "running" ? "running" : server.status === "errored" ? "errored" : "stopped",
    world: server.worldName,
    players: 0,
    maxPlayers: server.maxPlayers,
    port: server.port,
    version: "1.4.4.9",
    lastBackup: "Not yet",
    password: server.password ?? "",
    cpu: "0%",
    memory: "0 MB"
  };
}

export async function getDockerStatus(): Promise<{ available: boolean; message: string; host: string }> {
  const response = await fetch(`${API_BASE}/api/runtime/docker`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load Docker status");
  }
  return (await response.json()) as { available: boolean; message: string; host: string };
}

export async function createServer(input: {
  name: string;
  providerKey: "terraria-vanilla" | "terraria-tmodloader";
  config: TerrariaConfig;
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
  return {
    id: server.id,
    name: server.name,
    mode: server.providerKey === "terraria-tmodloader" ? "tmodloader" : "vanilla",
    status: server.status === "running" ? "running" : server.status === "errored" ? "errored" : "stopped",
    world: server.worldName,
    players: 0,
    maxPlayers: server.maxPlayers,
    port: server.port,
    version: "1.4.4.9",
    lastBackup: "Not yet",
    password: server.password ?? "",
    cpu: "0%",
    memory: "0 MB"
  };
}

export async function serverAction(id: string, action: "start" | "stop" | "restart" | "delete") {
  const response = await fetch(`${API_BASE}/api/servers/${id}${action === "delete" ? "" : `/${action}`}`, {
    method: action === "delete" ? "DELETE" : "POST"
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? `Unable to ${action} server`);
  }
}

export async function listWorlds(): Promise<World[]> {
  const response = await fetch(`${API_BASE}/api/worlds`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("Unable to load worlds");
  }
  const payload = (await response.json()) as ApiWorld[];
  return payload.map((world) => ({
    id: world.id,
    name: world.name,
    size: world.fileName,
    difficulty: "Imported",
    server: world.activeInstanceId || (world.instanceId !== "unassigned" ? world.instanceId : undefined),
    modified: formatRelative(world.updatedAt ?? world.createdAt),
    bytes: formatBytes(world.sizeBytes)
  }));
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
  return {
    id: world.id,
    name: world.name,
    size: world.fileName,
    difficulty: "Imported",
    modified: formatRelative(world.updatedAt ?? world.createdAt),
    bytes: formatBytes(world.sizeBytes)
  };
}

export async function duplicateWorld(id: string, name: string): Promise<World> {
  const fileName = `${name.trim().replaceAll(/[^a-zA-Z0-9._-]/g, "-") || "world-copy"}.wld`;
  const response = await fetch(`${API_BASE}/api/worlds/${id}/duplicate`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, fileName })
  });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to duplicate world");
  }
  const world = (await response.json()) as ApiWorld;
  return {
    id: world.id,
    name: world.name,
    size: world.fileName,
    difficulty: "Imported",
    modified: formatRelative(world.updatedAt ?? world.createdAt),
    bytes: formatBytes(world.sizeBytes)
  };
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
    created: formatRelative(backup.createdAt)
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
    created: formatRelative(backup.createdAt)
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

export async function listMods(serverId: string): Promise<ModFile[]> {
  const response = await fetch(`${API_BASE}/api/servers/${serverId}/mods`, { cache: "no-store" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to load mods");
  }
  const payload = (await response.json()) as ApiModFile[];
  return payload.map((file) => ({
    id: file.id,
    instanceId: file.instanceId,
    fileName: file.fileName,
    size: formatBytes(file.sizeBytes),
    enabled: file.enabled,
    created: formatRelative(file.createdAt)
  }));
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
  return {
    id: item.id,
    instanceId: item.instanceId,
    fileName: item.fileName,
    size: formatBytes(item.sizeBytes),
    enabled: item.enabled,
    created: formatRelative(item.createdAt)
  };
}

export async function deleteMod(serverId: string, modId: string) {
  const response = await fetch(`${API_BASE}/api/servers/${serverId}/mods/${modId}`, { method: "DELETE" });
  if (!response.ok) {
    const payload = (await response.json().catch(() => ({}))) as { error?: string };
    throw new Error(payload.error ?? "Unable to delete mod");
  }
}
