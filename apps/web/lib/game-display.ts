import type { MessageKey } from "./i18n";
import type { GameKey } from "./types";

const gameNameKeys: Partial<Record<string, MessageKey>> = {
  terraria: "gameNameTerraria",
  palworld: "gameNamePalworld",
  "dont-starve-together": "gameNameDST",
  minecraft: "gameNameMinecraft",
};

const gameDescKeys: Partial<Record<string, MessageKey>> = {
  terraria: "gameDescTerraria",
  palworld: "gameDescPalworld",
  "dont-starve-together": "gameDescDST",
  minecraft: "gameDescMinecraft",
};

export function gameDisplayName(key: GameKey | string | undefined, fallback: string, t: (key: MessageKey) => string): string {
  const msgKey = gameNameKeys[key ?? ""];
  return msgKey ? t(msgKey) : fallback;
}

export function gameDescription(key: GameKey | string | undefined, fallback: string, t: (key: MessageKey) => string): string {
  const msgKey = gameDescKeys[key ?? ""];
  return msgKey ? t(msgKey) : fallback;
}
