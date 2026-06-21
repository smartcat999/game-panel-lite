import { afterEach, describe, expect, it, vi } from "vitest";
import { downloadWorldFile, getGameServer, listBackups, listGames, listWorlds, setModEnabled } from "./api";

describe("api mappers", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("keeps raw backup bytes for aggregate dashboard metrics", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify([
          {
            id: "backup-1",
            instanceId: "server-1",
            fileName: "server-1.zip",
            worldName: "Earth",
            sizeBytes: 1536,
            type: "Manual",
            createdAt: new Date().toISOString()
          }
        ]),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );

    const backups = await listBackups();

    expect(backups[0]?.sizeBytes).toBe(1536);
  });

  it("preserves world file ownership separately from active server usage", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify([
          {
            id: "world-1",
            instanceId: "source-server",
            activeInstanceId: "active-server",
            name: "SharedName",
            fileName: "SharedName.wld",
            sizeBytes: 2048,
            createdAt: new Date().toISOString()
          }
        ]),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );

    const worlds = await listWorlds();

    expect(worlds[0]).toMatchObject({
      instanceId: "source-server",
      activeInstanceId: "active-server",
      server: "active-server"
    });
  });

  it("keeps server runtime error details for errored servers", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: "server-1",
          name: "Broken tModLoader",
          gameKey: "terraria",
          providerKey: "terraria-tmodloader",
          spec: {
            generation: 1,
            desiredState: "running",
            version: "v2026.04.3.0",
            config: { worldName: "Modded", maxPlayers: 8, port: 7777 },
            network: { port: 7777 }
          },
          status: {
            phase: "failed",
            actualState: "stopped",
            observedGeneration: 1,
            appliedGeneration: 0,
            lastError: "container exited (exit code 1)"
          },
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString()
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );

    const server = await getGameServer("server-1");

    expect(server.status.lastError).toBe("container exited (exit code 1)");
  });

  it("maps backend online player count onto server cards", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: "server-1",
          name: "Friends Server",
          gameKey: "terraria",
          providerKey: "terraria-vanilla",
          spec: {
            generation: 1,
            desiredState: "running",
            version: "1.4.5.6",
            config: { worldName: "Friends World", maxPlayers: 8, port: 7777 },
            network: { port: 7777 }
          },
          status: {
            phase: "running",
            actualState: "running",
            playersOnline: 2,
            observedGeneration: 1,
            appliedGeneration: 1
          },
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString()
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );

    const server = await getGameServer("server-1");

    expect(server.status.playersOnline).toBe(2);
    expect(server.gameKey).toBe("terraria");
    expect(server.providerKey).toBe("terraria-vanilla");
  });

  it("preserves the controller resource spec and status on mapped servers", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: "server-1",
          name: "Friends Server",
          gameKey: "terraria",
          providerKey: "terraria-vanilla",
          spec: {
            generation: 3,
            desiredState: "running",
            version: "1.4.5.6",
            config: { worldName: "Friends World", maxPlayers: 8, port: 7777 },
            resources: { cpuLimitCores: 2, memoryLimitMb: 2048 },
            network: { port: 7777, hostPort: 30001 }
          },
          status: {
            phase: "running",
            actualState: "running",
            playersOnline: 2,
            observedGeneration: 2,
            appliedGeneration: 2,
            conditions: [
              {
                type: "RuntimeReady",
                status: "True",
                observedGeneration: 2,
                lastTransitionAt: new Date().toISOString()
              }
            ]
          },
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString()
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );

    const server = await getGameServer("server-1");

    expect(server.status.phase).toBe("running");
    expect(server.status.playersOnline).toBe(2);
    expect(server.spec.generation).toBe(3);
    expect(server.status.phase).toBe("running");
    expect(server.spec.generation).toBeGreaterThan(server.status.appliedGeneration);
  });

  it("loads game catalog entries with provider capabilities", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify([
          {
            key: "terraria",
            name: "Terraria",
            description: "Sandbox adventure",
            status: "available",
            providers: [
              {
                key: "terraria-vanilla",
                name: "Terraria Vanilla",
                description: "Official server",
                recommended: true,
                versions: ["1.4.5.6"],
                capabilities: {
                  consoleCommands: true,
                  playerList: true,
                  kickPlayer: true,
                  banPlayer: true,
                  whitelist: false,
                  saveSnapshots: true,
                  backups: true,
                  mods: false,
                  versions: true
                },
                configSchema: [{ name: "serverName", label: "服务器名称", type: "text", required: true }]
              }
            ]
          },
          {
            key: "palworld",
            name: "Palworld",
            description: "Survival crafting",
            status: "planned",
            providers: []
          }
        ]),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );

    const games = await listGames();

    expect(games.find((game) => game.key === "terraria")?.providers[0]?.capabilities.consoleCommands).toBe(true);
    expect(games.find((game) => game.key === "palworld")?.status).toBe("planned");
  });

  it("surfaces backend download errors before the browser navigates away", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "world file is missing" }), {
        status: 404,
        headers: { "Content-Type": "application/json" }
      })
    );

    await expect(downloadWorldFile("world-1")).rejects.toThrow("world file is missing");
  });

  it("updates mod enabled state through the server-scoped endpoint", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: "mod-1",
          instanceId: "server-1",
          fileName: "example.tmod",
          sizeBytes: 128,
          enabled: false,
          createdAt: new Date().toISOString()
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );

    const mod = await setModEnabled("server-1", "mod-1", false);

    expect(fetchSpy).toHaveBeenCalledWith(
      expect.stringContaining("/api/servers/server-1/mods/mod-1"),
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ enabled: false })
      })
    );
    expect(mod.enabled).toBe(false);
  });
});
