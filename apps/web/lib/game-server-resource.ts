import type { TerrariaConfig } from "@gamepanel-lite/shared";
import { gameKeyFromProvider } from "./game-filters";
import type { GameServerResource, ProviderKey, ServerMode, ServerStatus } from "./types";

export function gameServerStatus(server: GameServerResource): ServerStatus {
  const phase = server.status?.phase;
  if (!phase) return "stopped";
  if (phase === "running") return "running";
  if (phase === "stopped") return "stopped";
  if (phase === "failed") return "errored";
  if (phase === "deleting" || phase === "deleted") return "deleting";
  if (phase === "pending" || phase === "reconciling") {
    if (server.spec?.desiredState === "stopped") return "stopping";
    if (server.spec?.desiredState === "deleted") return "deleting";
    return "starting";
  }
  return "stopped";
}

export function gameServerMode(server: Pick<GameServerResource, "providerKey">): ServerMode {
  return server.providerKey === "terraria-tmodloader" ? "tmodloader" : "vanilla";
}

export function gameServerWorldName(server: GameServerResource): string {
  const config = server.spec?.config;
  return (
    stringConfigValue(config, "worldName", "") ||
    stringConfigValue(config, "saveName", "") ||
    stringConfigValue(config, "clusterName", "") ||
    server.name
  );
}

export function gameServerPassword(server: GameServerResource): string {
  const config = server.spec?.config;
  return stringConfigValue(config, "password", "") || stringConfigValue(config, "serverPassword", "");
}

export function gameServerMaxPlayers(server: GameServerResource): number {
  return numberConfigValue(server.spec?.config, "maxPlayers", 0);
}

export function gameServerJoinPort(server: GameServerResource): number {
  return server.spec?.network?.hostPort || server.spec?.network?.port || numberConfigValue(server.spec?.config, "port", 0);
}

export function gameServerVersion(server: GameServerResource): string {
  return server.spec?.version || defaultVersionForProvider(server.providerKey);
}

export function gameServerConfigPendingRestart(server: GameServerResource): boolean {
  return gameServerStatus(server) === "running" && (server.spec?.generation ?? 0) > (server.status?.appliedGeneration ?? 0);
}

export function terrariaConfigFromGameServer(server: GameServerResource): TerrariaConfig {
  const specConfig = server.spec?.config;
  return {
    serverName: stringConfigValue(specConfig, "serverName", server.name),
    worldName: gameServerWorldName(server),
    worldSize: stringConfigValue(specConfig, "worldSize", "medium") as TerrariaConfig["worldSize"],
    worldEvil: stringConfigValue(specConfig, "worldEvil", "random") as TerrariaConfig["worldEvil"],
    difficulty: stringConfigValue(specConfig, "difficulty", "classic") as TerrariaConfig["difficulty"],
    maxPlayers: numberConfigValue(specConfig, "maxPlayers", 8),
    port: numberConfigValue(specConfig, "port", server.spec?.network?.port ?? 7777),
    password: gameServerPassword(server),
    motd: stringConfigValue(specConfig, "motd", stringConfigValue(specConfig, "adminPassword", stringConfigValue(specConfig, "clusterToken", ""))),
    seed: stringConfigValue(specConfig, "seed", ""),
    specialSeeds: arrayConfigValue(specConfig, "specialSeeds", []),
    secretSeeds: arrayConfigValue(specConfig, "secretSeeds", []),
    secure: booleanConfigValue(specConfig, "secure", booleanConfigValue(specConfig, "eulaAccepted", true)),
    language: stringConfigValue(specConfig, "language", "en-US"),
    autoCreateWorld: booleanConfigValue(specConfig, "autoCreateWorld", true)
  };
}

export function gameServerSearchText(server: GameServerResource, providerLabel: string): string[] {
  return [
    server.name,
    gameServerWorldName(server),
    String(gameServerJoinPort(server)),
    String(server.spec?.network?.port ?? ""),
    gameServerMode(server),
    providerLabel
  ];
}

export function gameKeyForGameServer(server: Pick<GameServerResource, "gameKey" | "providerKey">): string {
  return server.gameKey ?? gameKeyFromProvider(server.providerKey) ?? "all";
}

function defaultVersionForProvider(providerKey: ProviderKey): string {
  if (providerKey === "terraria-vanilla") return "1.4.5.6";
  if (providerKey === "terraria-tmodloader") return "v2026.04.3.0";
  return "latest";
}

function stringConfigValue(config: Record<string, unknown> | undefined, key: string, fallback: string): string {
  const value = config?.[key];
  return typeof value === "string" ? value : fallback;
}

function numberConfigValue(config: Record<string, unknown> | undefined, key: string, fallback: number): number {
  const value = config?.[key];
  return typeof value === "number" ? value : fallback;
}

function booleanConfigValue(config: Record<string, unknown> | undefined, key: string, fallback: boolean): boolean {
  const value = config?.[key];
  return typeof value === "boolean" ? value : fallback;
}

function arrayConfigValue(config: Record<string, unknown> | undefined, key: string, fallback: string[]): string[] {
  const value = config?.[key];
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : fallback;
}
