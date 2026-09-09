import { afterEach, describe, expect, it, vi } from "vitest";
import { getGameServer, listGameServers, ModInstallationError, requestModInstallation } from "./api";

describe("workspace installation transport", () => {
  afterEach(() => vi.restoreAllMocks());
  it("sends the reviewed generation and validates a requested receipt", async () => {
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ serverId: "server / 1", generation: 8, modIds: ["mod"], state: "requested" }), { status: 202 }));
    expect(await requestModInstallation("server / 1", "mod", 7)).toMatchObject({ generation: 8, state: "requested" });
    expect(fetch.mock.calls[0]?.[0]).toContain("/servers/server%20%2F%201/mods/installation-requests");
    expect(JSON.parse(String(fetch.mock.calls[0]?.[1]?.body))).toEqual({ modId: "mod", generation: 7 });
    expect(fetch).toHaveBeenCalledTimes(1);
  });
  it.each([
    { serverId: "foreign", generation: 8, modIds: ["mod"], state: "requested" },
    { serverId: "server", generation: 8, modIds: ["other"], state: "requested" },
    { serverId: "server", generation: 19, modIds: ["mod"], state: "requested" },
    { serverId: "server", generation: 8, modIds: ["mod"], state: "installed" }
  ])("treats unverifiable successful receipts as uncertain", async receipt => {
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify(receipt), { status: 202 }));
    await expect(requestModInstallation("server", "mod", 7)).rejects.toMatchObject({ uncertain: true, status: 0 });
    expect(fetch).toHaveBeenCalledTimes(1);
  });
  it("does not retry rejected or uncertain requests", async () => {
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(new Response("{}", { status: 409 })).mockResolvedValueOnce(new Response("{}", { status: 500 })).mockRejectedValueOnce(new Error("connection lost"));
    await expect(requestModInstallation("server", "mod", 7)).rejects.toMatchObject({ uncertain: false, status: 409 });
    await expect(requestModInstallation("server", "mod", 7)).rejects.toMatchObject({ uncertain: true, status: 500 });
    await expect(requestModInstallation("server", "mod", 7)).rejects.toBeInstanceOf(ModInstallationError);
    expect(fetch).toHaveBeenCalledTimes(3);
  });
  it("preserves ownership in list and detail conversion", async () => {
    const record = { id: "server", organizationId: "space", spec: { generation: 7 }, status: { phase: "stopped" } };
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(new Response(JSON.stringify([record]))).mockResolvedValueOnce(new Response(JSON.stringify(record)));
    expect((await listGameServers())[0]?.organizationId).toBe("space");
    expect((await getGameServer("server")).organizationId).toBe("space");
  });
  it("sends the selected workspace when listing instances", async () => {
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("[]"));
    await listGameServers("space / one");
    expect(fetch.mock.calls[0]?.[0]).toContain("organizationId=space+%2F+one");
  });
});
