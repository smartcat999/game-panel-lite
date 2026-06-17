import { defaultCreateServerConfig } from "./create-server-defaults";
import type { ProviderCatalog, ProviderConfigField, ProviderKey } from "./types";
import type { TerrariaConfig } from "@gamepanel-lite/shared";

export type ProviderConfigPayload = Record<string, unknown>;

export function createDefaultProviderConfigPayload(
  provider?: ProviderCatalog,
  overrides: ProviderConfigPayload = {}
): ProviderConfigPayload {
  const payload: ProviderConfigPayload = {};
  for (const field of provider?.configSchema ?? []) {
    payload[field.name] = defaultProviderFieldValue(field);
  }
  return { ...payload, ...overrides };
}

export function updateProviderConfigPayload(
  payload: ProviderConfigPayload,
  field: ProviderConfigField,
  value: string | boolean
): ProviderConfigPayload {
  return {
    ...payload,
    [field.name]: coerceProviderFieldValue(field, value)
  };
}

export function providerPayloadToTerrariaConfig(
  providerKey: ProviderKey,
  payload: ProviderConfigPayload | undefined,
  fallback: TerrariaConfig = defaultCreateServerConfig
): TerrariaConfig {
  if (!payload || providerKey === "terraria-vanilla" || providerKey === "terraria-tmodloader") {
    return fallback;
  }

  const serverName = stringValue(payload.serverName) || fallback.serverName || "Game Server";
  const worldName = stringValue(payload.worldName) || stringValue(payload.saveName) || fallback.worldName || serverName;
  const maxPlayers = numberValue(payload.maxPlayers, fallback.maxPlayers);
  const password = stringValue(payload.password) ?? stringValue(payload.serverPassword) ?? fallback.password ?? "";
  const motd = stringValue(payload.motd) ?? stringValue(payload.adminPassword) ?? fallback.motd ?? "";

  return {
    ...fallback,
    serverName,
    worldName,
    maxPlayers,
    password,
    motd
  };
}

function defaultProviderFieldValue(field: ProviderConfigField): unknown {
  if (field.default !== undefined) return field.default;
  if (field.type === "number") return 0;
  if (field.type === "boolean") return false;
  return "";
}

function coerceProviderFieldValue(field: ProviderConfigField, value: string | boolean): unknown {
  if (field.type === "boolean") return Boolean(value);
  if (field.type === "number") {
    const nextValue = Number(value);
    return Number.isFinite(nextValue) ? nextValue : 0;
  }
  return String(value);
}

function stringValue(value: unknown): string | undefined {
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  return undefined;
}

function numberValue(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const nextValue = Number(value);
    if (Number.isFinite(nextValue)) return nextValue;
  }
  return fallback;
}
