import type { MessageKey } from "./i18n";
import type { GameKey, ProviderKey } from "./types";

type ConsoleServer = { gameKey?: GameKey; providerKey?: ProviderKey };

export function supportsTerrariaConsoleShortcuts(server: ConsoleServer) {
  return server.providerKey === "terraria-vanilla" || server.providerKey === "terraria-tmodloader";
}

export function consoleReadyMessageKey(server: ConsoleServer): MessageKey {
  if (supportsTerrariaConsoleShortcuts(server)) return "consoleReady";
  if (server.gameKey === "minecraft" || server.providerKey === "minecraft") return "minecraftConsoleReady";
  return "genericConsoleReady";
}
