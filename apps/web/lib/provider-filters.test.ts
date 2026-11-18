import { describe, expect, it } from "vitest";
import { providerFilterOptions } from "./provider-filters";
import type { GameCatalogEntry } from "./types";

const games: GameCatalogEntry[] = [
  {
    key: "terraria",
    name: "Terraria",
    description: "",
    status: "available",
    providers: [
      { key: "terraria-vanilla", name: "Terraria", description: "", recommended: true, versions: [], capabilities: emptyCapabilities(), configSchema: [] },
      { key: "terraria-tmodloader", name: "tModLoader", description: "", recommended: false, versions: [], capabilities: emptyCapabilities(), configSchema: [] }
    ]
  },
  {
    key: "palworld",
    name: "Palworld",
    description: "",
    status: "available",
    providers: [
      { key: "palworld", name: "Palworld", description: "", recommended: true, versions: [], capabilities: emptyCapabilities(), configSchema: [] }
    ]
  }
];

describe("provider filter options", () => {
  it("builds provider filters from catalog and existing resources", () => {
    expect(providerFilterOptions(games, "All", ["minecraft"]).map((option) => option.key)).toEqual([
      "all",
      "minecraft",
      "palworld",
      "terraria-vanilla",
      "terraria-tmodloader"
    ]);
  });

  it("scopes provider filters to the selected game", () => {
    expect(providerFilterOptions(games, "All", ["minecraft", "terraria-tmodloader"], "terraria").map((option) => option.key)).toEqual([
      "all",
      "terraria-tmodloader",
      "terraria-vanilla"
    ]);
    expect(providerFilterOptions(games, "All", ["minecraft", "terraria-tmodloader"], "minecraft").map((option) => option.key)).toEqual([
      "all",
      "minecraft"
    ]);
  });
});

function emptyCapabilities() {
  return {
    consoleCommands: false,
    playerList: false,
    kickPlayer: false,
    banPlayer: false,
    whitelist: false,
    saveSnapshots: false,
    backups: false,
    mods: false,
    versions: false
  };
}
