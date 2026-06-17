import { afterEach, describe, expect, it, vi } from "vitest";
import { downloadWorldFile, getServer, listBackups, listGames, listWorlds, setModEnabled } from "./api";

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
          providerKey: "terraria-tmodloader",
          status: "errored",
          worldName: "Modded",
          port: 7777,
          maxPlayers: 8,
          lastError: "container exited (exit code 1)",
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString()
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );

    const server = await getServer("server-1");

    expect(server.lastError).toBe("container exited (exit code 1)");
  });

  it("maps backend online player count onto server cards", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: "server-1",
          name: "Friends Server",
          gameKey: "terraria",
          providerKey: "terraria-vanilla",
          status: "running",
          worldName: "Friends World",
          playersOnline: 2,
          port: 7777,
          maxPlayers: 8,
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString()
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );

    const server = await getServer("server-1");

    expect(server.players).toBe(2);
    expect(server.gameKey).toBe("terraria");
    expect(server.providerKey).toBe("terraria-vanilla");
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
