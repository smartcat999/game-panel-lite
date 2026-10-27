import { describe, expect, it } from "vitest";
import { createSevenDaysWorldSeed, isSevenDaysFieldVisible, sevenDaysCreationOverrides, sevenDaysConfigSection } from "./seven-days-config";
import { updateProviderConfigPayload } from "./provider-config";
import type { ProviderConfigField } from "./types";

const seedField: ProviderConfigField = { name: "worldGenSeed", label: "Seed", required: false, type: "text" };
const sizeField: ProviderConfigField = { name: "worldGenSize", label: "Size", required: true, type: "number" };

describe("Seven Days world configuration", () => {
  it("creates distinct safe save IDs and seeds without localized display names", () => {
    const first = sevenDaysCreationOverrides();
    const second = sevenDaysCreationOverrides();
    expect(first.worldGenSeed).toMatch(/^[a-f0-9]{16}$/);
    expect(first.worldName).toBe(`World-${first.worldGenSeed}`);
    expect(first.worldName).not.toBe(second.worldName);
    expect(first.worldGenSeed).not.toBe(second.worldGenSeed);
    expect(createSevenDaysWorldSeed()).toMatch(/^[a-f0-9]{16}$/);
  });

  it("only shows generation controls for RWG without mutating saved values", () => {
    const config = { gameWorld: "RWG", worldGenSeed: "my-seed", worldGenSize: 8192 };
    expect(isSevenDaysFieldVisible(seedField, config)).toBe(true);
    expect(isSevenDaysFieldVisible(sizeField, config)).toBe(true);
    for (const gameWorld of ["Navezgane", "Pregen06k01", "Pregen06k02", "Pregen08k01", "Pregen08k02"]) {
      const fixed = { ...config, gameWorld };
      expect(isSevenDaysFieldVisible(seedField, fixed)).toBe(false);
      expect(isSevenDaysFieldVisible(sizeField, fixed)).toBe(false);
      expect(fixed.worldGenSeed).toBe("my-seed");
    }
    const renamed = updateProviderConfigPayload(config, { name: "serverName", type: "text", label: "Name", required: true }, "New name");
    expect(renamed.worldGenSeed).toBe("my-seed");
  });

  it("keeps discrete map size choices numeric in submitted configuration", () => {
    const next = updateProviderConfigPayload({}, sizeField, "10240");
    expect(next.worldGenSize).toBe(10240);
  });
});

describe("Seven Days settings dependencies", () => {
  const field = (name: string): ProviderConfigField => ({ name, label: name, type: "number", required: true });

  it("hides inactive dynamic mesh controls without discarding their values", () => {
    const payload = { dynamicMeshEnabled: false, dynamicMeshLandClaimOnly: true, dynamicMeshLandClaimBuffer: 5, dynamicMeshMaxItemCache: 2 };
    for (const name of ["dynamicMeshLandClaimOnly", "dynamicMeshLandClaimBuffer", "dynamicMeshMaxItemCache"]) {
      expect(isSevenDaysFieldVisible(field(name), payload)).toBe(false);
    }
    expect(isSevenDaysFieldVisible(field("maxSpawnedZombies"), payload)).toBe(true);
    const enabled = { ...payload, dynamicMeshEnabled: true };
    expect(isSevenDaysFieldVisible(field("dynamicMeshLandClaimBuffer"), enabled)).toBe(true);
    expect(enabled.dynamicMeshLandClaimBuffer).toBe(5);
    expect(isSevenDaysFieldVisible(field("dynamicMeshLandClaimBuffer"), { ...enabled, dynamicMeshLandClaimOnly: false })).toBe(false);
  });
});


describe("Seven Days settings sections", () => {
  it("keeps listing fields basic and groups PvP with multiplayer claim settings", () => {
    const base = { name: "region", label: "Region", type: "select", required: true };
    expect(sevenDaysConfigSection(base)).toBe("basic");
    expect(sevenDaysConfigSection({ ...base, name: "playerKillingMode" })).toBe("landClaims");
    expect(sevenDaysConfigSection({ ...base, name: "maxSpawnedZombies", group: "sevenDays.performance" })).toBe("performance");
    expect(sevenDaysConfigSection({ ...base, name: "landClaimSize", group: "sevenDays.landClaims" })).toBe("landClaims");
  });
});
