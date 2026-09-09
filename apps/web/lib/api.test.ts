import { afterEach, describe, expect, it, vi } from "vitest";
import { applyGameUpdate, checkGameUpdate, createTenantInstance, downloadWorldFile, getGameServer, getGameUpdate, getPlatformInstanceView, getRegionDeployments, getRegionNodes, getWorldRegeneration, listBackups, listGames, listPlatformInstanceViews, listTenantInstanceViews, listWorlds, previewWorkshopItems, regenerateWorld, setModEnabled, updateGameUpdateAutoCheck } from "./api";

describe("api mappers", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("sends an idempotent plan-derived create command without infrastructure fields", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      instanceId: "instance-a", operationId: "operation-a", operationStatus: "pending", revisionId: "revision-a", specGeneration: 1, intentVersion: 1, regionId: "east"
    }), { status: 202, headers: { "Content-Type": "application/json" } }));

    await createTenantInstance({ organizationId: "tenant-a", name: "server", planId: "starter", planVersion: 2, idempotencyKey: "request-a", configuration: {} });

    const init = fetchMock.mock.calls[0]?.[1];
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/instances");
    expect(new Headers(init?.headers).get("Idempotency-Key")).toBe("request-a");
    expect(JSON.parse(String(init?.body))).toEqual(expect.not.objectContaining({ nodeId: expect.anything(), regionId: expect.anything(), cpu: expect.anything() }));
  });

  it("uses separate tenant and platform logical instance endpoints", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ id: "logical-1" }), { status: 200, headers: { "Content-Type": "application/json" } }));

    await listTenantInstanceViews("tenant-a", "cursor-a", 25);
    await listPlatformInstanceViews(undefined, undefined, 50);
    await getPlatformInstanceView("logical-1");

    const tenantURL = new URL(String(fetchMock.mock.calls[0]?.[0]), "http://local.test");
    expect(tenantURL.pathname).toBe("/api/instances");
    expect(tenantURL.searchParams.get("organizationId")).toBe("tenant-a");
    expect(tenantURL.searchParams.get("after")).toBe("cursor-a");
    expect(tenantURL.searchParams.get("limit")).toBe("25");
    const platformURL = new URL(String(fetchMock.mock.calls[1]?.[0]), "http://local.test");
    expect(platformURL.pathname).toBe("/api/platform/instances");
    expect(platformURL.searchParams.has("organizationId")).toBe(false);
    expect(String(fetchMock.mock.calls[2]?.[0])).toContain("/api/platform/instances/logical-1");
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

  it("reads node operations through the Region-scoped platform route", async () => {
    const page = { regionId: "region-east", observedAtMs: 10, nodes: [] };
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify(page), { status: 200, headers: { "Content-Type": "application/json" } })
    );

    await expect(getRegionNodes("region-east")).resolves.toEqual(page);
    expect(fetchSpy).toHaveBeenCalledWith(
      expect.stringContaining("/api/regions/region-east/nodes?limit=100"),
      expect.objectContaining({ cache: "no-store", credentials: "include" })
    );
  });

  it("reads regional deployments through the owning Region route", async () => {
    const page = { regionId: "region-east", observedAtMs: 10, deployments: [] };
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify(page), { status: 200, headers: { "Content-Type": "application/json" } })
    );

    await expect(getRegionDeployments("region-east")).resolves.toEqual(page);
    expect(fetchSpy).toHaveBeenCalledWith(
      expect.stringContaining("/api/regions/region-east/deployments?limit=100"),
      expect.objectContaining({ cache: "no-store", credentials: "include" })
    );
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

  it("uses server-scoped asynchronous game update endpoints", async () => {
    const job = {
      id: "update-1",
      instanceId: "server-1",
      providerKey: "palworld",
      operation: "apply",
      status: "queued",
      stage: "queued",
      progress: 0,
      startAfterUpdate: true,
      wasRunning: true,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString()
    };
    const fetchSpy = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify({ supported: true, status: "available", installedBuildId: "100", latestBuildId: "101" }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(job), { status: 202 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ enabled: false, intervalHours: 6 }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(job), { status: 202 }));

    const update = await getGameUpdate("server-1");
    await checkGameUpdate("server-1");
    await updateGameUpdateAutoCheck("server-1", false);
    const queued = await applyGameUpdate("server-1", true);

    expect(update.status).toBe("available");
    expect(queued.id).toBe("update-1");
    expect(fetchSpy).toHaveBeenNthCalledWith(1, expect.stringContaining("/api/servers/server-1/game-update"), expect.objectContaining({ cache: "no-store" }));
    expect(fetchSpy).toHaveBeenNthCalledWith(2, expect.stringContaining("/api/servers/server-1/game-update/check"), expect.objectContaining({ method: "POST" }));
    expect(fetchSpy).toHaveBeenNthCalledWith(3, expect.stringContaining("/api/servers/server-1/game-update/auto-check"), expect.objectContaining({
      method: "PUT",
      body: JSON.stringify({ enabled: false })
    }));
    expect(fetchSpy).toHaveBeenNthCalledWith(4, expect.stringContaining("/api/servers/server-1/game-update/apply"), expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ startAfterUpdate: true })
    }));
  });

  it("previews Steam Workshop items before import", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(new Response(JSON.stringify({
      previewId: "preview-1",
      collectionId: "",
      providerKey: "terraria-tmodloader",
      expiresAt: new Date().toISOString(),
      summary: { total: 1, new: 1, inLibrary: 0, inServer: 0, unavailable: 0 },
      items: [{ workshopId: "2824688072", title: "Calamity Mod", fileSize: 1048576, status: "new", selectable: true }]
    }), { status: 200 }));

    const preview = await previewWorkshopItems({ workshopIds: ["2824688072"], providerKey: "terraria-tmodloader" });

    expect(fetchSpy).toHaveBeenCalledWith(
      expect.stringContaining("/api/mods/workshop/items/preview"),
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ workshopIds: ["2824688072"], providerKey: "terraria-tmodloader" })
      })
    );
    expect(preview.items[0]).toMatchObject({ title: "Calamity Mod", size: "1.0 MB" });
  });

  it("uses server-scoped asynchronous world regeneration endpoints", async () => {
    const job = {
      id: "regeneration-1",
      instanceId: "server-1",
      providerKey: "dont-starve-together",
      status: "queued",
      stage: "queued",
      progress: 0,
      startAfter: true,
      wasRunning: true,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString()
    };
    const fetchSpy = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify({ supported: true, job }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(job), { status: 202 }));

    const state = await getWorldRegeneration("server-1");
    const queued = await regenerateWorld("server-1", true);

    expect(state.supported).toBe(true);
    expect(queued.id).toBe("regeneration-1");
    expect(fetchSpy).toHaveBeenNthCalledWith(1, expect.stringContaining("/api/servers/server-1/world-regeneration"), expect.objectContaining({ cache: "no-store" }));
    expect(fetchSpy).toHaveBeenNthCalledWith(2, expect.stringContaining("/api/servers/server-1/world-regeneration"), expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ startAfter: true })
    }));
  });
});
