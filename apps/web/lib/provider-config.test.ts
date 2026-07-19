import { describe, expect, it } from "vitest";
import { createDefaultProviderConfigPayload, isAdvancedProviderConfigField, isProviderFieldModified, isWorldGenerationProviderConfigField, providerConfigFieldChanged, providerConfigValue, restoreProviderConfigDefaults, updateProviderConfigPayload } from "./provider-config";
import type { ProviderCatalog, ProviderConfigField } from "./types";

const provider: ProviderCatalog = {
  key: "palworld",
  name: "Palworld",
  description: "Palworld dedicated server",
  recommended: true,
  versions: ["latest"],
  capabilities: {
    backups: true,
    banPlayer: false,
    consoleCommands: false,
    kickPlayer: false,
    mods: false,
    playerList: false,
    saveSnapshots: true,
    whitelist: false,
    versions: true
  },
  configSchema: [
    { name: "serverName", label: "服务器名称", type: "text", required: true, default: "Palworld Server" },
    { name: "saveName", label: "存档名称", type: "text", required: true, default: "Palworld Save" },
    { name: "maxPlayers", label: "最大玩家数", type: "number", required: true, default: 8 },
    { name: "serverPassword", label: "服务器密码", type: "password", required: false },
    { name: "community", label: "社区服务器", type: "boolean", required: false }
  ]
};

function field(overrides: Partial<ProviderConfigField>): ProviderConfigField {
  return {
    name: "setting",
    label: "Setting",
    type: "select",
    required: false,
    ...overrides
  };
}

describe("provider config helpers", () => {
  it("creates defaults from provider schema", () => {
    expect(createDefaultProviderConfigPayload(provider)).toEqual({
      community: false,
      maxPlayers: 8,
      saveName: "Palworld Save",
      serverName: "Palworld Server",
      serverPassword: ""
    });
  });

  it("separates DST world generation fields from restart-applied settings", () => {
    const worldgen = field({ name: "world.overrides.world_size", group: "dst.world.worldgen.global", default: "default" });
    const worldSettings = field({ name: "world.overrides.day", group: "dst.world.worldsettings.global", default: "default" });
    expect(isWorldGenerationProviderConfigField("dont-starve-together", worldgen)).toBe(true);
    expect(isWorldGenerationProviderConfigField("dont-starve-together", worldSettings)).toBe(false);
    expect(isWorldGenerationProviderConfigField("dont-starve-together", field({ name: "world.preset" }))).toBe(true);
  });

  it("compares provider field values using their declared type", () => {
    const numeric = field({ name: "gameplay.rate", type: "number", default: 1 });
    expect(providerConfigFieldChanged({ gameplay: { rate: 1 } }, { gameplay: { rate: "1" } }, numeric)).toBe(false);
    expect(providerConfigFieldChanged({ gameplay: { rate: 1 } }, { gameplay: { rate: 2 } }, numeric)).toBe(true);
  });

  it("restores multiple provider fields in one payload update", () => {
    const enabled = field({ name: "features.enabled", type: "boolean", default: false });
    const rate = field({ name: "gameplay.rate", type: "number", default: 1 });
    expect(restoreProviderConfigDefaults(
      { features: { enabled: true }, gameplay: { rate: 3 } },
      [enabled, rate]
    )).toEqual({ features: { enabled: false }, gameplay: { rate: 1 } });
  });

  it("coerces updated schema values", () => {
    const payload = createDefaultProviderConfigPayload(provider);
    const maxPlayersField = provider.configSchema.find((field) => field.name === "maxPlayers");
    const communityField = provider.configSchema.find((field) => field.name === "community");

    expect(maxPlayersField).toBeDefined();
    expect(communityField).toBeDefined();
    expect(updateProviderConfigPayload(payload, maxPlayersField!, "12")).toMatchObject({ maxPlayers: 12 });
    expect(updateProviderConfigPayload(payload, communityField!, true)).toMatchObject({ community: true });
  });

  it("supports provider-owned nested schema paths", () => {
    const nestedProvider: ProviderCatalog = {
      ...provider,
      key: "dont-starve-together",
      configSchema: [
        { name: "identity.serverName", label: "Server", type: "text", required: true, default: "DST Friends" },
        { name: "gameplay.maxPlayers", label: "Players", type: "number", required: true, default: 6 },
        { name: "caves.enabled", label: "Caves", type: "boolean", required: false, default: false }
      ]
    };
    const payload = createDefaultProviderConfigPayload(nestedProvider);

    expect(payload).toEqual({
      caves: { enabled: false },
      gameplay: { maxPlayers: 6 },
      identity: { serverName: "DST Friends" }
    });
    const playersField = nestedProvider.configSchema[1];
    expect(playersField).toBeDefined();
    const updated = updateProviderConfigPayload(payload, playersField!, "12");
    expect(providerConfigValue(updated, "gameplay.maxPlayers")).toBe(12);
  });

  it("keeps nested schema defaults when stored override groups are empty", () => {
    const nestedProvider: ProviderCatalog = {
      ...provider,
      key: "dont-starve-together",
      configSchema: [
        { name: "world.overrides.grass", label: "Grass", type: "select", required: false, default: "default" },
        { name: "world.overrides.sapling", label: "Saplings", type: "select", required: false, default: "default" }
      ]
    };

    const payload = createDefaultProviderConfigPayload(nestedProvider, {
      world: {
        preset: "forest_default",
        overrides: {}
      }
    });

    expect(payload).toEqual({
      world: {
        preset: "forest_default",
        overrides: {
          grass: "default",
          sapling: "default"
        }
      }
    });
  });

  it("separates advanced DST and Palworld settings from their basic fields", () => {
    const dstRule: ProviderConfigField = { name: "world.overrides.grass", label: "Grass", type: "select", required: false, default: "default", group: "dst.world.worldsettings.resources" };
    const palRate: ProviderConfigField = { name: "expRate", label: "EXP", type: "number", required: true, default: 1, group: "世界倍率" };
    const palName: ProviderConfigField = { name: "serverName", label: "Name", type: "text", required: true, default: "Palworld Server", group: "基础设置" };

    expect(isAdvancedProviderConfigField("dont-starve-together", dstRule)).toBe(true);
    expect(isAdvancedProviderConfigField("palworld", palRate)).toBe(true);
    expect(isAdvancedProviderConfigField("palworld", palName)).toBe(false);
  });

  it("detects numeric and nested values that differ from schema defaults", () => {
    const expRate: ProviderConfigField = { name: "expRate", label: "EXP", type: "number", required: true, default: 1 };
    const grass: ProviderConfigField = { name: "world.overrides.grass", label: "Grass", type: "select", required: false, default: "default" };

    expect(isProviderFieldModified({ expRate: 1, world: { overrides: { grass: "default" } } }, expRate)).toBe(false);
    expect(isProviderFieldModified({ expRate: 2 }, expRate)).toBe(true);
    expect(isProviderFieldModified({ world: { overrides: { grass: "often" } } }, grass)).toBe(true);
  });
});
