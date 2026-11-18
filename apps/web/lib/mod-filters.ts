import { gameKeyFromProvider } from "./game-filters";
import type { GameCatalogEntry, GameKey, ModFile, ModPack, RecommendedMod } from "./types";

export type ModResource = Pick<ModFile | RecommendedMod | ModPack, "gameKey" | "providerKey"> & {
  mods?: ModResource[];
};

export function modResourceGame(resource: ModResource): GameKey | undefined {
  const directGame = resource.gameKey ?? gameKeyFromProvider(resource.providerKey);
  if (directGame) return directGame;

  const childGames = Array.from(new Set((resource.mods ?? []).map((mod) => modResourceGame(mod)).filter((key): key is GameKey => Boolean(key))));
  return childGames.length === 1 ? childGames[0] : undefined;
}

export function filterModResources<T extends ModResource>(items: T[], gameFilter: string): T[] {
  if (gameFilter === "all") return items;
  return items.filter((item) => modResourceGame(item) === gameFilter);
}

export function modGameFilterKeys(games: GameCatalogEntry[], resources: ModResource[] = []): string[] {
  const keys = new Set<string>();
  for (const game of games) {
    if (game.status !== "available") continue;
    keys.add(game.key);
  }
  for (const resource of resources) {
    const key = modResourceGame(resource);
    if (key) keys.add(key);
  }
  return Array.from(keys);
}
