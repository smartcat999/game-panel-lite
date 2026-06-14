import { afterEach, describe, expect, it, vi } from "vitest";
import { listBackups } from "./api";

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
});
