import type { ProviderConfigField } from "./types";
import type { ProviderConfigPayload } from "./provider-config";

export function createSevenDaysWorldSeed(): string {
  const bytes = new Uint32Array(2);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (value) => value.toString(16).padStart(8, "0")).join("");
}

export function sevenDaysCreationOverrides(): ProviderConfigPayload {
  const seed = createSevenDaysWorldSeed();
  return { worldName: `World-${seed}`, worldGenSeed: seed };
}

export function isSevenDaysFieldVisible(field: ProviderConfigField, payload: ProviderConfigPayload): boolean {
  if (["worldGenSeed", "worldGenSize"].includes(field.name)) return payload.gameWorld === "RWG";
  if (["dynamicMeshLandClaimOnly", "dynamicMeshMaxItemCache", "dynamicMeshLandClaimBuffer"].includes(field.name) && payload.dynamicMeshEnabled === false) return false;
  if (field.name === "dynamicMeshLandClaimBuffer" && payload.dynamicMeshLandClaimOnly === false) return false;
  return true;
}

export type SevenDaysConfigSection = "basic" | "server" | "performance" | "landClaims";

export function sevenDaysConfigSection(field: ProviderConfigField): SevenDaysConfigSection {
  if (["serverName", "serverPassword", "serverVisibility", "maxPlayers", "eacEnabled", "region", "language"].includes(field.name)) return "server";
  if (field.group === "sevenDays.advanced") return "performance";
  if (field.group === "sevenDays.performance") return "performance";
  if (field.group === "sevenDays.landClaims" || field.name === "playerKillingMode") return "landClaims";
  return "basic";
}
