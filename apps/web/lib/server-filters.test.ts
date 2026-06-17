import { describe, expect, it } from "vitest";
import { filterServers, type ServerFilters } from "./server-filters";
import type { Server } from "./types";

const baseServer: Server = {
  id: "server-1",
  name: "Friends",
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
    mode: "tmodloader",
    status: "running",
    world: "Modded World",
    port: 7788,
    hostPort: 7788
  }
];

const defaultFilters: ServerFilters = {
  game: "all",
  query: "",
  status: "all",
  type: "all"
};

describe("server filters", () => {
  it("combines game, status, type, and search filters", () => {
    expect(filterServers(servers, { ...defaultFilters, game: "terraria" }).map((server) => server.id)).toEqual(["server-1", "server-2"]);
    expect(filterServers(servers, { ...defaultFilters, status: "running" }).map((server) => server.id)).toEqual(["server-2"]);
    expect(filterServers(servers, { ...defaultFilters, type: "modded" }).map((server) => server.id)).toEqual(["server-2"]);
    expect(filterServers(servers, { ...defaultFilters, query: "7777", type: "vanilla" }).map((server) => server.id)).toEqual(["server-1"]);
  });
});
