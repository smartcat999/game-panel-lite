import type { GameCatalogEntry, GameKey, ProviderKey } from "./types";

export type GameFilterOption = {
  key: string;
  label: string;
};

const providerGameMap: Record<string, GameKey> = {
  "terraria-vanilla": "terraria",
  "terraria-tmodloader": "terraria",
  palworld: "palworld",
  "dont-starve-together": "dont-starve-together",
  minecraft: "minecraft"
};

const fallbackGameLabels: Record<string, string> = {
  terraria: "Terraria",
  palworld: "Palworld",
  "dont-starve-together": "Don't Starve Together",
  minecraft: "Minecraft Java"
};

export function gameKeyFromProvider(providerKey?: ProviderKey): GameKey | undefined {
  if (!providerKey) return undefined;
  return providerGameMap[providerKey];
}

export function gameFilterOptions(games: GameCatalogEntry[], allLabel: string, extraKeys: Array<string | undefined> = []): GameFilterOption[] {
  const labels = new Map<string, string>();
  for (const game of games) {
    if (game.status === "available") {
      labels.set(game.key, game.name);
    }
  }
  for (const key of extraKeys) {
    if (!key) continue;
    labels.set(key, labels.get(key) ?? fallbackGameLabels[key] ?? formatGameKey(key));
  }
  return [
    { key: "all", label: allLabel },
    ...Array.from(labels.entries())
      .sort((a, b) => a[1].localeCompare(b[1]))
      .map(([key, label]) => ({ key, label }))
  ];
}

export function gameFilterOptionsForKeys(games: GameCatalogEntry[], allLabel: string, keys: Array<string | undefined>): GameFilterOption[] {
  const labels = new Map(games.map((game) => [game.key, game.name]));
  const options = uniqueGameKeys(keys).map((key) => ({
    key,
    label: labels.get(key) ?? fallbackGameLabels[key] ?? formatGameKey(key)
  }));
  return [
    { key: "all", label: allLabel },
    ...options.sort((a, b) => a.label.localeCompare(b.label))
  ];
}

function uniqueGameKeys(keys: Array<string | undefined>) {
  return Array.from(new Set(keys.filter((key): key is string => Boolean(key))));
}

function formatGameKey(key: string) {
  return key
    .split("-")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}
