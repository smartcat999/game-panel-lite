import { serverJoinPort } from "./server-join";
import { gameKeyFromProvider } from "./game-filters";
import { serverProviderDisplay } from "./server-display";
import type { Server } from "./types";

export type ServerGameFilter = "all" | string;
export type ServerStatusFilter = "all" | "running" | "stopped";
export type ServerProviderFilter = "all" | string;

export type ServerFilters = {
  game: ServerGameFilter;
  provider: ServerProviderFilter;
  query: string;
  status: ServerStatusFilter;
};

export function serverGame(server: Server): ServerGameFilter {
  return server.gameKey ?? gameKeyFromProvider(server.providerKey) ?? "all";
}

export function filterServers(servers: Server[], filters: ServerFilters) {
  const term = filters.query.trim().toLowerCase();
  return servers.filter((server) => {
    const provider = serverProviderDisplay(server);
    const matchesSearch = !term || [server.name, server.world, String(serverJoinPort(server)), String(server.port), server.mode, provider.label].some((value) => value.toLowerCase().includes(term));
    const matchesGame = filters.game === "all" || serverGame(server) === filters.game;
    const matchesStatus = filters.status === "all" || server.status === filters.status;
    const matchesProvider = filters.provider === "all" || server.providerKey === filters.provider;
    return matchesSearch && matchesGame && matchesStatus && matchesProvider;
  });
}
