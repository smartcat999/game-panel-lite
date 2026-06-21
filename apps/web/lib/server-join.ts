import { gameServerJoinPort, gameServerPassword } from "./game-server-resource";
import type { GameServerResource } from "./types";

export function serverJoinPort(server: GameServerResource): number {
  return gameServerJoinPort(server);
}

export function serverJoinAddress(_server: GameServerResource): string {
  return "127.0.0.1";
}

export function serverJoinPassword(server: GameServerResource): string {
  return gameServerPassword(server);
}

export function serverInviteText(server: GameServerResource): string {
  const passwordValue = gameServerPassword(server);
  const password = passwordValue ? ` password: ${passwordValue}` : "";
  return `Join ${server.name} at 127.0.0.1:${serverJoinPort(server)}${password}`;
}
