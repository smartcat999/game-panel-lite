import type { GameKey } from "./types";
import type { MessageKey } from "./i18n";

export type ReviewInvitePreviewInput = {
  address?: string;
  gameKey?: GameKey;
  gameName?: string;
  hostPortLabel: string;
  password?: string;
  serverName: string;
};

export function createReviewInvitePreview({
  address = "127.0.0.1",
  gameKey,
  gameName,
  hostPortLabel,
  password,
  serverName
}: ReviewInvitePreviewInput): string {
  const displayGameName = gameName ?? reviewGameName(gameKey);
  const secret = password ? ` password: ${password}` : "";
  return `Join ${serverName} in ${displayGameName} at ${address}:${hostPortLabel}${secret}`;
}

function reviewGameName(gameKey?: GameKey) {
  switch (gameKey) {
    case "palworld":
      return "Palworld";
    case "dont-starve-together":
      return "Don't Starve Together";
    case "minecraft":
      return "Minecraft";
    case "terraria":
    case undefined:
      return "Terraria";
    default:
      return String(gameKey);
  }
}

export function reviewJoinInstructionKey(gameKey?: GameKey): MessageKey {
  switch (gameKey) {
    case "palworld":
      return "reviewPalworldJoinInstruction";
    case "dont-starve-together":
      return "reviewDstJoinInstruction";
    case "minecraft":
      return "reviewMinecraftJoinInstruction";
    default:
      return "reviewTerrariaJoinInstruction";
  }
}

export function reviewResourceSummaryKey(gameKey?: GameKey): MessageKey {
  switch (gameKey) {
    case "palworld":
      return "reviewSavePlayers";
    case "dont-starve-together":
      return "reviewClusterPlayers";
    case "minecraft":
      return "reviewMinecraftWorldPlayers";
    default:
      return "reviewWorldPlayers";
  }
}
