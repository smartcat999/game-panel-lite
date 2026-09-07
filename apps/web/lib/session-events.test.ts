import { afterEach, describe, expect, it, vi } from "vitest";
import { getAuthBootstrap, listGames } from "./api";
import { sessionExpiredEvent } from "./session-events";

describe("session loss notification", () => {
  afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });
  it("signals an unauthorized business response without changing the API error", async () => {
    const target = new EventTarget();
    vi.stubGlobal("window", target);
    const expired = vi.fn();
    target.addEventListener(sessionExpiredEvent, expired);
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response('{"error":"session expired"}', { status: 401 }));
    await expect(listGames()).rejects.toThrow();
    expect(expired).toHaveBeenCalledTimes(1);
  });
  it("does not create a bootstrap refresh loop or treat forbidden as logout", async () => {
    const target = new EventTarget();
    vi.stubGlobal("window", target);
    const expired = vi.fn();
    target.addEventListener(sessionExpiredEvent, expired);
    const fetch = vi.spyOn(globalThis, "fetch");
    fetch.mockResolvedValueOnce(new Response('{"error":"not authenticated"}', { status: 401 }));
    const signal = new AbortController().signal;
    await expect(getAuthBootstrap(signal)).rejects.toThrow("not authenticated");
    expect(fetch).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ signal, credentials: "include" }));
    fetch.mockResolvedValueOnce(new Response('{"error":"forbidden"}', { status: 403 }));
    await expect(listGames()).rejects.toThrow();
    expect(expired).not.toHaveBeenCalled();
  });
});
