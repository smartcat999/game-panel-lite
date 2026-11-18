import { gameKeyForGameServer, gameServerSearchText, gameServerStatus } from "./game-server-resource";
import { serverProviderDisplay } from "./server-display";
import type { GameServerResource } from "./types";

export type ServerGameFilter = "all" | string;
export type ServerStatusFilter = "all" | "running" | "stopped";
export type ServerProviderFilter = "all" | string;

export type ServerFilters = {
  game: ServerGameFilter;
  provider: ServerProviderFilter;
  query: string;
  status: ServerStatusFilter;
};

export function gameServerGame(server: GameServerResource): ServerGameFilter {
  return gameKeyForGameServer(server);
}

export function filterGameServers(servers: GameServerResource[], filters: ServerFilters) {
  const term = filters.query.trim().toLowerCase();
  return servers.filter((server) => {
    const provider = serverProviderDisplay(server);
    const matchesSearch = !term || gameServerSearchText(server, provider.label).some((value) => value.toLowerCase().includes(term));
    const matchesGame = filters.game === "all" || gameServerGame(server) === filters.game;
    const matchesStatus = filters.status === "all" || gameServerStatus(server) === filters.status;
    const matchesProvider = filters.provider === "all" || server.providerKey === filters.provider;
    return matchesSearch && matchesGame && matchesStatus && matchesProvider;
  });
}
