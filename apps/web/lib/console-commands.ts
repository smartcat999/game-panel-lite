import type { MessageKey } from "./i18n";
import type { Server } from "./types";

type ConsoleServer = Pick<Server, "gameKey" | "providerKey">;

export function supportsTerrariaConsoleShortcuts(server: ConsoleServer) {
  return server.gameKey === "terraria" || server.providerKey === "terraria-vanilla" || server.providerKey === "terraria-tmodloader";
}

export function consoleReadyMessageKey(server: ConsoleServer): MessageKey {
  if (supportsTerrariaConsoleShortcuts(server)) return "consoleReady";
  if (server.gameKey === "minecraft" || server.providerKey === "minecraft") return "minecraftConsoleReady";
  return "genericConsoleReady";
}
