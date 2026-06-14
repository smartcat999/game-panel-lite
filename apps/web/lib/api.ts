import type { TerrariaConfig } from "@gamepanel-lite/shared";
import type { Server } from "./types";

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
