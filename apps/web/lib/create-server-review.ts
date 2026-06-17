import type { GameKey } from "./types";

export type ReviewInvitePreviewInput = {
  address?: string;
  gameKey?: GameKey;
  hostPortLabel: string;
  password?: string;
  serverName: string;
};

export function createReviewInvitePreview({
  address = "127.0.0.1",
  gameKey,
  hostPortLabel,
  password,
  serverName
}: ReviewInvitePreviewInput): string {
  const gameName = gameKey === "palworld" ? "Palworld" : gameKey === "terraria" || !gameKey ? "Terraria" : String(gameKey);
  const secret = password ? ` password: ${password}` : "";
  return `Join ${serverName} in ${gameName} at ${address}:${hostPortLabel}${secret}`;
}
