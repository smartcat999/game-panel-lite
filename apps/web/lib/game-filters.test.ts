import { describe, expect, it } from "vitest";
import { gameFilterOptions, gameFilterOptionsForKeys, gameKeyFromProvider } from "./game-filters";
import type { MessageKey } from "./i18n";
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

const zhMessages: Partial<Record<MessageKey, string>> = {
  gameNameDST: "饥荒联机版",
  gameNameMinecraft: "我的世界 (Java版)",
  gameNamePalworld: "幻兽帕鲁",
  gameNameTerraria: "泰拉瑞亚"
};
const t = (key: MessageKey) => zhMessages[key] ?? key;

describe("game filter options", () => {
  it("uses available catalog games and preserves unknown resource games", () => {
    expect(gameFilterOptions(games, "All", ["minecraft", "terraria"]).map((item) => item.key)).toEqual(["all", "minecraft", "palworld", "terraria"]);
  });

  it("maps known providers to their game key", () => {
    expect(gameKeyFromProvider("terraria-tmodloader")).toBe("terraria");
    expect(gameKeyFromProvider("dont-starve-together")).toBe("dont-starve-together");
  });

  it("can build options only for explicit resource games", () => {
    expect(gameFilterOptionsForKeys(games, "All", ["terraria", "minecraft", "terraria"]).map((item) => item.key)).toEqual(["all", "minecraft", "terraria"]);
  });

  it("localizes known game labels when a translator is provided", () => {
    expect(gameFilterOptions(games, "全部", ["minecraft"], t).map((item) => item.label)).toEqual(["全部", "幻兽帕鲁", "我的世界 (Java版)", "泰拉瑞亚"]);
  });
});
