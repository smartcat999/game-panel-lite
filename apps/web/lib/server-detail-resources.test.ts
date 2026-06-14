import { describe, expect, it } from "vitest";
import { getDetailTargetServers, nextWorldCopyName } from "./server-detail-resources";
import type { Server } from "./types";

const baseServer: Server = {
  id: "server-1",
  name: "Friends Server",
  mode: "vanilla",
  status: "stopped",
  world: "Home",
  players: 0,
  maxPlayers: 8,
  port: 7777,
  version: "1.4.4.9",
  lastBackup: "Not yet",
  password: "",
  cpu: "0%",
  memory: "0 MB",
  config: {
    serverName: "Friends Server",
    worldName: "Home",
    worldSize: "medium",
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

describe("server detail resource helpers", () => {
  it("uses other servers as migration targets without offering the current server", () => {
    const targets = getDetailTargetServers(
      [
        baseServer,
        { ...baseServer, id: "server-2", name: "Builder Server" },
        { ...baseServer, id: "server-3", name: "Expert Server" }
      ],
      "server-1"
    );

    expect(targets.map((server) => server.id)).toEqual(["server-2", "server-3"]);
  });

  it("creates a localized default world copy name", () => {
    expect(nextWorldCopyName("Home", "副本")).toBe("Home 副本");
  });
});
