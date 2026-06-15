import { serverJoinPort } from "./server-join";
import type { Server } from "./types";

export type ServerGameFilter = "all" | "terraria";
export type ServerStatusFilter = "all" | "running" | "stopped";
export type ServerTypeFilter = "all" | "vanilla" | "modded";

export type ServerFilters = {
  game: ServerGameFilter;
  query: string;
  status: ServerStatusFilter;
  type: ServerTypeFilter;
};

export function serverGame(server: Server): ServerGameFilter {
  return server.mode === "vanilla" || server.mode === "tmodloader" ? "terraria" : "all";
}

export function filterServers(servers: Server[], filters: ServerFilters) {
  const term = filters.query.trim().toLowerCase();
  return servers.filter((server) => {
    const matchesSearch = !term || [server.name, server.world, String(serverJoinPort(server)), String(server.port), server.mode].some((value) => value.toLowerCase().includes(term));
    const matchesGame = filters.game === "all" || serverGame(server) === filters.game;
    const matchesStatus = filters.status === "all" || server.status === filters.status;
    const matchesType =
      filters.type === "all" ||
      (filters.type === "vanilla" && server.mode === "vanilla") ||
      (filters.type === "modded" && server.mode === "tmodloader");
    return matchesSearch && matchesGame && matchesStatus && matchesType;
  });
}
