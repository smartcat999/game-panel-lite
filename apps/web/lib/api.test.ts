import { afterEach, describe, expect, it, vi } from "vitest";
import { downloadWorldFile, listBackups, setModEnabled } from "./api";

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
