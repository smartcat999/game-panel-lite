import { afterEach, describe, expect, it, vi } from "vitest";
import { listGlobalMods, uploadWorkspaceMod, WorkspaceModUploadError } from "./api";

const record = { id: "mod-1", organizationId: "space / one", instanceId: "unassigned", fileName: "file + one.tmod", providerKey: "custom-provider", sizeBytes: 5, enabled: true, createdAt: "2026-09-07T00:00:00Z", contentHash: "a".repeat(64) };
describe("workspace mod upload transport", () => {
  afterEach(() => vi.restoreAllMocks());
  it("sends raw bytes and explicit encoded ownership, retaining response ownership", async () => {
    const file = new File(["bytes"], record.fileName);
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify(record), { status: 201 }));
    const result = await uploadWorkspaceMod(record.organizationId, record.providerKey, file);
    const [input, init] = fetch.mock.calls[0]!;
    const url = new URL(String(input), "http://localhost");
    expect(url.pathname).toBe("/api/auth/me/mods/upload");
    expect(url.searchParams.get("organizationId")).toBe(record.organizationId);
    expect(url.searchParams.get("providerKey")).toBe(record.providerKey);
    expect(url.searchParams.get("fileName")).toBe(file.name);
    expect(init).toMatchObject({ method: "POST", credentials: "include", headers: { "Content-Type": "application/octet-stream" }, body: file });
    expect(result).toMatchObject({ organizationId: record.organizationId, contentHash: record.contentHash });
  });
  it("keeps reconciliation IDs and does not retry uncertain responses", async () => {
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ error: "retained", uploadId: "reconcile-id" }), { status: 503 }));
    try { await uploadWorkspaceMod("one", "plugin", new File(["bytes"], "file.tmod")); throw new Error("expected failure"); }
    catch (error) { expect(error).toBeInstanceOf(WorkspaceModUploadError); expect(error).toMatchObject({ uploadId: "reconcile-id", uncertain: true }); }
    expect(fetch).toHaveBeenCalledTimes(1);
  });
  it("distinguishes an ordinary rejection from an unavailable response", async () => {
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(new Response('{"error":"duplicate"}', { status: 409 })).mockRejectedValueOnce(new Error("connection closed"));
    const file = new File(["bytes"], "file.tmod");
    await expect(uploadWorkspaceMod("one", "plugin", file)).rejects.toMatchObject({ uncertain: false, status: 409 });
    await expect(uploadWorkspaceMod("one", "plugin", file)).rejects.toMatchObject({ uncertain: true, status: 0 });
    expect(fetch).toHaveBeenCalledTimes(2);
  });
  it("preserves workspace fields when refreshing the existing library endpoint", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify([record])));
    expect((await listGlobalMods())[0]).toMatchObject({ organizationId: record.organizationId, contentHash: record.contentHash });
  });
});
