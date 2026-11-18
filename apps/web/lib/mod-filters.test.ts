import { describe, expect, it } from "vitest";
import { filterModResources, modGameFilterKeys, modResourceGame } from "./mod-filters";
import type { GameCatalogEntry, ModFile, ModPack } from "./types";

const games: GameCatalogEntry[] = [
  {
    key: "terraria",
    name: "Terraria",
    description: "",
    status: "available",
    serverCount: 0,
    providers: [
      {
        key: "terraria-tmodloader",
        name: "tModLoader",
        description: "",
        recommended: true,
        versions: [],
        capabilities: {
          consoleCommands: true,
          playerList: true,
          kickPlayer: false,
          banPlayer: false,
          whitelist: false,
          saveSnapshots: true,
          backups: true,
          mods: true,
          versions: true
        },
        configSchema: []
      }
    ]
  },
  {
    key: "palworld",
    name: "Palworld",
    description: "",
    status: "available",
    serverCount: 0,
    providers: [
      {
        key: "palworld",
        name: "Palworld",
        description: "",
        recommended: true,
        versions: [],
        capabilities: {
          consoleCommands: false,
          playerList: false,
          kickPlayer: false,
          banPlayer: false,
          whitelist: false,
          saveSnapshots: true,
          backups: true,
          mods: false,
          versions: true
        },
        configSchema: []
      }
    ]
  }
];

const mods: ModFile[] = [
  {
    id: "mod-1",
    instanceId: "unassigned",
    gameKey: "terraria",
    providerKey: "terraria-tmodloader",
    fileName: "BossChecklist.tmod",
    size: "1 MB",
    enabled: true,
    created: "just now"
  },
  {
    id: "mod-2",
    instanceId: "unassigned",
    gameKey: "future-game",
    providerKey: "future-provider",
    fileName: "FutureMod.zip",
    size: "1 MB",
    enabled: true,
    created: "just now"
  }
];

describe("mod filters", () => {
  it("uses resource metadata before provider fallback", () => {
    expect(modResourceGame(mods[0]!)).toBe("terraria");
    expect(modResourceGame({ providerKey: "dont-starve-together" })).toBe("dont-starve-together");
  });

  it("filters mod resources by game", () => {
    expect(filterModResources(mods, "terraria").map((item) => item.id)).toEqual(["mod-1"]);
    expect(filterModResources(mods, "future-game").map((item) => item.id)).toEqual(["mod-2"]);
    expect(filterModResources(mods, "all").map((item) => item.id)).toEqual(["mod-1", "mod-2"]);
  });

  it("infers a mod pack game from contained mods when pack metadata is missing", () => {
    const packs: ModPack[] = [
      {
        id: "pack-1",
        name: "Terraria Pack",
        description: "",
        modIds: ["mod-1"],
        mods: [mods[0]!],
        created: "just now"
      },
      {
        id: "pack-2",
        name: "Mixed Pack",
        description: "",
        modIds: ["mod-1", "mod-2"],
        mods,
        created: "just now"
      }
    ];

    expect(modResourceGame(packs[0]!)).toBe("terraria");
    expect(modResourceGame(packs[1]!)).toBeUndefined();
    expect(filterModResources(packs, "terraria").map((pack) => pack.id)).toEqual(["pack-1"]);
  });

  it("includes all available games plus games from existing resources", () => {
    expect(modGameFilterKeys(games, mods)).toEqual(["terraria", "palworld", "future-game"]);
  });
});
