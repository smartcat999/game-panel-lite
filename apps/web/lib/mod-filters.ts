import { gameKeyFromProvider } from "./game-filters";
import type { GameCatalogEntry, GameKey, ModFile, ModPack, RecommendedMod } from "./types";

export type ModResource = Pick<ModFile | RecommendedMod | ModPack, "gameKey" | "providerKey">;

export function modResourceGame(resource: ModResource): GameKey | undefined {
  return resource.gameKey ?? gameKeyFromProvider(resource.providerKey);
}

export function filterModResources<T extends ModResource>(items: T[], gameFilter: string): T[] {
  if (gameFilter === "all") return items;
  return items.filter((item) => modResourceGame(item) === gameFilter);
}

export function modGameFilterKeys(games: GameCatalogEntry[], resources: ModResource[] = []): string[] {
  const keys = new Set<string>();
  for (const game of games) {
    if (game.status !== "available") continue;
    if (game.providers.some((provider) => provider.capabilities.mods)) {
      keys.add(game.key);
    }
  }
  for (const resource of resources) {
    const key = modResourceGame(resource);
    if (key) keys.add(key);
  }
  return Array.from(keys);
}
