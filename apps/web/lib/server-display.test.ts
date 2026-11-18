import { describe, expect, it } from "vitest";
import { serverProviderDisplay, serverResourceLabelKey } from "./server-display";

const baseServer = {
  gameKey: "terraria",
  mode: "vanilla",
  providerKey: "terraria-vanilla"
} as const;

describe("server display helpers", () => {
  it("uses provider metadata for server badges", () => {
    expect(serverProviderDisplay(baseServer)).toEqual({ label: "Terraria", tone: "green" });
    expect(serverProviderDisplay({ ...baseServer, providerKey: "terraria-tmodloader", mode: "tmodloader" })).toEqual({ label: "tModLoader", tone: "purple" });
    expect(serverProviderDisplay({ ...baseServer, providerKey: "palworld" })).toEqual({ label: "Palworld", tone: "sky" });
    expect(serverProviderDisplay({ ...baseServer, providerKey: "minecraft" })).toEqual({ label: "Minecraft Java", tone: "green" });
  });

  it("uses game-specific resource nouns", () => {
    expect(serverResourceLabelKey(baseServer)).toBe("world");
    expect(serverResourceLabelKey({ ...baseServer, gameKey: "palworld", providerKey: "palworld" })).toBe("save");
    expect(serverResourceLabelKey({ ...baseServer, gameKey: "dont-starve-together", providerKey: "dont-starve-together" })).toBe("clusterSave");
  });
});
