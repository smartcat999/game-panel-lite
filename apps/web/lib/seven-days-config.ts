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
  return !["worldGenSeed", "worldGenSize"].includes(field.name) || payload.gameWorld === "RWG";
}
