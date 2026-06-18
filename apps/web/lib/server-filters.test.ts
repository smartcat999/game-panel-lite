import { describe, expect, it } from "vitest";
import { filterServers, type ServerFilters } from "./server-filters";
import type { Server } from "./types";

const baseServer: Server = {
  id: "server-1",
  name: "Friends",
  gameKey: "terraria",
  providerKey: "terraria-vanilla",
  mode: "vanilla",
  status: "stopped",
  world: "Classic World",
  players: 0,
  maxPlayers: 8,
  port: 7777,
  hostPort: 7777,
  version: "1.4.4.9",
  cpuLimitCores: 0,
  memoryLimitMb: 0,
  lastBackup: "Not yet",
  password: "",
  cpu: "0%",
  memory: "0 MB",
  config: {
    serverName: "Friends",
    worldName: "Classic World",
    worldSize: "medium",
    worldEvil: "random",
    difficulty: "classic",
    maxPlayers: 8,
    port: 7777,
    password: "",
    motd: "",
    seed: "",
    secure: true,
    language: "en-US",
    autoCreateWorld: true
  }
};

const servers: Server[] = [
  baseServer,
  {
    ...baseServer,
    id: "server-2",
    name: "Mods",
    providerKey: "terraria-tmodloader",
    mode: "tmodloader",
    status: "running",
    world: "Modded World",
    port: 7788,
    hostPort: 7788
  },
  {
    ...baseServer,
    id: "server-3",
    name: "Pal Friends",
    gameKey: "palworld",
    providerKey: "palworld",
    mode: "vanilla",
    status: "running",
    world: "Pal Save",
    port: 8211,
    hostPort: 18211
  }
];

const defaultFilters: ServerFilters = {
  game: "all",
  provider: "all",
  query: "",
  status: "all"
};

describe("server filters", () => {
  it("combines game, status, provider, and search filters", () => {
    expect(filterServers(servers, { ...defaultFilters, game: "terraria" }).map((server) => server.id)).toEqual(["server-1", "server-2"]);
    expect(filterServers(servers, { ...defaultFilters, game: "palworld" }).map((server) => server.id)).toEqual(["server-3"]);
    expect(filterServers(servers, { ...defaultFilters, status: "running" }).map((server) => server.id)).toEqual(["server-2", "server-3"]);
    expect(filterServers(servers, { ...defaultFilters, provider: "terraria-tmodloader" }).map((server) => server.id)).toEqual(["server-2"]);
    expect(filterServers(servers, { ...defaultFilters, query: "7777", provider: "terraria-vanilla" }).map((server) => server.id)).toEqual(["server-1"]);
    expect(filterServers(servers, { ...defaultFilters, provider: "palworld" }).map((server) => server.id)).toEqual(["server-3"]);
    expect(filterServers(servers, { ...defaultFilters, query: "Palworld" }).map((server) => server.id)).toEqual(["server-3"]);
  });
});
