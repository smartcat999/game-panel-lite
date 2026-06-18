import { gameKeyFromProvider } from "./game-filters";
import type { GameCatalogEntry, ProviderKey } from "./types";

export type ProviderFilterOption = {
  key: string;
  label: string;
};

const fallbackProviderLabels: Record<string, string> = {
  "terraria-vanilla": "Terraria",
  "terraria-tmodloader": "tModLoader",
  palworld: "Palworld",
  "dont-starve-together": "Don't Starve Together",
  minecraft: "Minecraft Java"
};

export function providerFilterOptions(
  games: GameCatalogEntry[],
  allLabel: string,
  providerKeys: Array<ProviderKey | undefined>,
  gameFilter: string = "all"
): ProviderFilterOption[] {
  const labels = new Map<string, string>();

  for (const game of games) {
    if (game.status !== "available") continue;
    if (gameFilter !== "all" && game.key !== gameFilter) continue;
    for (const provider of game.providers) {
      labels.set(provider.key, provider.name);
    }
  }

  for (const providerKey of providerKeys) {
    if (!providerKey) continue;
    const providerGame = gameKeyFromProvider(providerKey);
    if (gameFilter !== "all" && providerGame !== gameFilter) continue;
    labels.set(providerKey, labels.get(providerKey) ?? fallbackProviderLabels[providerKey] ?? formatProviderKey(providerKey));
  }

  return [
    { key: "all", label: allLabel },
    ...Array.from(labels.entries())
      .sort((a, b) => a[1].localeCompare(b[1]))
      .map(([key, label]) => ({ key, label }))
  ];
}

function formatProviderKey(key: string) {
  return key
    .split("-")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}
