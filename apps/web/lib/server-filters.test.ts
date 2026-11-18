import { describe, expect, it } from "vitest";
import { filterGameServers, type ServerFilters } from "./server-filters";
import type { GameServerResource } from "./types";

const gameServers: GameServerResource[] = [
  {
    id: "resource-1",
    name: "Friends",
    gameKey: "terraria",
    providerKey: "terraria-vanilla",
    spec: {
      generation: 1,
      desiredState: "stopped",
      version: "1.4.5.6",
      config: { worldName: "Classic World", maxPlayers: 8, port: 7777 },
      network: { port: 7777, hostPort: 7777 }
    },
    status: {
      phase: "stopped",
      actualState: "stopped",
      observedGeneration: 1,
      appliedGeneration: 1
    },
    createdAt: "2026-06-20T00:00:00Z",
    updatedAt: "2026-06-20T00:00:00Z"
  },
  {
    id: "resource-2",
    name: "Pal Friends",
    gameKey: "palworld",
    providerKey: "palworld",
    spec: {
      generation: 2,
      desiredState: "running",
      version: "latest",
      config: { saveName: "Pal Save", maxPlayers: 16, port: 8211 },
      network: { port: 8211, hostPort: 18211 }
    },
    status: {
      phase: "reconciling",
      actualState: "stopped",
      observedGeneration: 1,
      appliedGeneration: 1
    },
    createdAt: "2026-06-20T00:00:00Z",
    updatedAt: "2026-06-20T00:00:00Z"
  }
];

const defaultFilters: ServerFilters = {
  game: "all",
  provider: "all",
  query: "",
  status: "all"
};

describe("server filters", () => {
  it("filters GameServer resources without converting the API result to legacy Server first", () => {
    expect(filterGameServers(gameServers, { ...defaultFilters, game: "palworld" }).map((server) => server.id)).toEqual(["resource-2"]);
    expect(filterGameServers(gameServers, { ...defaultFilters, status: "running" }).map((server) => server.id)).toEqual([]);
    expect(filterGameServers(gameServers, { ...defaultFilters, query: "Pal Save" }).map((server) => server.id)).toEqual(["resource-2"]);
    expect(filterGameServers(gameServers, { ...defaultFilters, query: "18211" }).map((server) => server.id)).toEqual(["resource-2"]);
    expect(filterGameServers(gameServers, { ...defaultFilters, provider: "terraria-vanilla" }).map((server) => server.id)).toEqual(["resource-1"]);
    expect(filterGameServers(gameServers, { ...defaultFilters, query: "Palworld" }).map((server) => server.id)).toEqual(["resource-2"]);
  });
});
