import { describe, expect, it } from "vitest";
import { consoleReadyMessageKey, supportsTerrariaConsoleShortcuts } from "./console-commands";
import type { GameServerResource } from "./types";

const server = (gameKey: string, providerKey: string) => ({ gameKey, providerKey });

describe("console command helpers", () => {
  it("only exposes Terraria shortcut commands for Terraria providers", () => {
    expect(supportsTerrariaConsoleShortcuts(server("terraria", "terraria-vanilla"))).toBe(true);
    expect(supportsTerrariaConsoleShortcuts(server("terraria", "terraria-tmodloader"))).toBe(true);
    expect(supportsTerrariaConsoleShortcuts(server("terraria", "custom-provider"))).toBe(false);
    expect(supportsTerrariaConsoleShortcuts(server("minecraft", "minecraft"))).toBe(false);
    expect(supportsTerrariaConsoleShortcuts(server("dont-starve-together", "dont-starve-together"))).toBe(false);
  });

  it("uses command examples that match the server provider", () => {
    expect(consoleReadyMessageKey(server("terraria", "terraria-vanilla"))).toBe("consoleReady");
    expect(consoleReadyMessageKey(server("minecraft", "minecraft"))).toBe("minecraftConsoleReady");
    expect(consoleReadyMessageKey(server("palworld", "palworld"))).toBe("genericConsoleReady");
  });

  it("accepts GameServer resources", () => {
    const resource = {
      id: "server-1",
      name: "Minecraft",
      gameKey: "minecraft",
      providerKey: "minecraft",
      spec: { generation: 1, desiredState: "running" },
      status: { phase: "running", actualState: "running", observedGeneration: 1, appliedGeneration: 1 },
      createdAt: "2026-06-21T00:00:00Z",
      updatedAt: "2026-06-21T00:00:00Z"
    } satisfies GameServerResource;

    expect(supportsTerrariaConsoleShortcuts(resource)).toBe(false);
    expect(consoleReadyMessageKey(resource)).toBe("minecraftConsoleReady");
  });
});
