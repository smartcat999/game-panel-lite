import { describe, expect, it } from "vitest";
import { consoleReadyMessageKey, supportsTerrariaConsoleShortcuts } from "./console-commands";
import type { Server } from "./types";

const server = (gameKey: string, providerKey: string): Pick<Server, "gameKey" | "providerKey"> => ({ gameKey, providerKey });

describe("console command helpers", () => {
  it("only exposes Terraria shortcut commands for Terraria providers", () => {
    expect(supportsTerrariaConsoleShortcuts(server("terraria", "terraria-vanilla"))).toBe(true);
    expect(supportsTerrariaConsoleShortcuts(server("terraria", "terraria-tmodloader"))).toBe(true);
    expect(supportsTerrariaConsoleShortcuts(server("minecraft", "minecraft"))).toBe(false);
    expect(supportsTerrariaConsoleShortcuts(server("dont-starve-together", "dont-starve-together"))).toBe(false);
  });

  it("uses command examples that match the server provider", () => {
    expect(consoleReadyMessageKey(server("terraria", "terraria-vanilla"))).toBe("consoleReady");
    expect(consoleReadyMessageKey(server("minecraft", "minecraft"))).toBe("minecraftConsoleReady");
    expect(consoleReadyMessageKey(server("palworld", "palworld"))).toBe("genericConsoleReady");
  });
});
