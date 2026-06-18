import { describe, expect, it } from "vitest";
import { gameFilterOptions, gameKeyFromProvider } from "./game-filters";
import type { GameCatalogEntry } from "./types";

const games: GameCatalogEntry[] = [
  {
    key: "terraria",
    name: "Terraria",
    description: "",
    status: "available",
    serverCount: 0,
    providers: []
  },
  {
    key: "palworld",
    name: "Palworld",
    description: "",
    status: "available",
    serverCount: 0,
    providers: []
  },
  {
    key: "planned-game",
    name: "Planned Game",
    description: "",
    status: "planned",
    serverCount: 0,
    providers: []
  }
];

describe("game filter options", () => {
  it("uses available catalog games and preserves unknown resource games", () => {
    expect(gameFilterOptions(games, "All", ["minecraft", "terraria"]).map((item) => item.key)).toEqual(["all", "minecraft", "palworld", "terraria"]);
  });

  it("maps known providers to their game key", () => {
    expect(gameKeyFromProvider("terraria-tmodloader")).toBe("terraria");
    expect(gameKeyFromProvider("dont-starve-together")).toBe("dont-starve-together");
  });
});
