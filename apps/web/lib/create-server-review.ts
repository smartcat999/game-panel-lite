import type { GameKey } from "./types";

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
  const displayGameName = gameName ?? (gameKey === "palworld" ? "Palworld" : gameKey === "dont-starve-together" ? "Don't Starve Together" : gameKey === "terraria" || !gameKey ? "Terraria" : String(gameKey));
  const secret = password ? ` password: ${password}` : "";
  return `Join ${serverName} in ${displayGameName} at ${address}:${hostPortLabel}${secret}`;
}
