import { describe, expect, it } from "vitest";
import { applyPZPreset, isPZFieldVisible, isPZRuntimeReadOnlyField, isPZSpawnPoint, pzINIStringError, pzMetadata, pzPages, pzSandboxDefaults, pzStartDayMax, updatePZField, validatePZConfig } from "./project-zomboid-config";
import { createDefaultProviderConfigPayload, providerConfigValue } from "./provider-config";
import type { ProviderCatalog, ProviderConfigField } from "./types";

const field = (name: string): ProviderConfigField => ({ name, type: "text", label: name, required: false });

// Deliberately synthetic full fixture. This does not represent any official preset.
const fixture = { id: "test-fixture", values: Object.fromEntries(pzMetadata.sandbox.map((option) => [option.name, option.defaultValue])) };

describe("Project Zomboid native configuration", () => {
  it("replaces the whole sandbox once and preserves server options and passwords", () => {
    const server = { PublicName: "Friends", Password: "join-secret", MaxPlayers: 16 };
    const payload = { server, adminPassword: "bootstrap-secret", sandbox: { obsolete: true, ZombieLore: { Speed: 99 } } };
    const result = applyPZPreset(payload, fixture);
    expect(result.server).toBe(server);
    expect(result.adminPassword).toBe("bootstrap-secret");
    expect(result.preset).toBe("test-fixture");
    expect(providerConfigValue(result, "sandbox.obsolete")).toBeUndefined();
    for (const option of pzMetadata.sandbox) expect(providerConfigValue(result, `sandbox.${option.name}`)).toEqual(fixture.values[option.name]);
    expect(payload.sandbox.ZombieLore.Speed).toBe(99);
  });

  it("does not replay native onChange callbacks while applying presets", () => {
    const raw = { ...fixture, values: { ...fixture.values, ZombieRespawn: 4, "ZombieConfig.RespawnHours": 123, "ZombieConfig.RespawnMultiplier": 0.75 } };
    const result = applyPZPreset({}, raw);
    expect(providerConfigValue(result, "sandbox.ZombieRespawn")).toBe(4);
    expect(providerConfigValue(result, "sandbox.ZombieConfig.RespawnHours")).toBe(123);
    expect(providerConfigValue(result, "sandbox.ZombieConfig.RespawnMultiplier")).toBe(0.75);
  });

  it("keeps native enum indices, numbers and booleans typed and dot names nested", () => {
    let result = updatePZField({}, field("sandbox.ZombieLore.Speed"), "2");
    result = updatePZField(result, field("sandbox.ZombieConfig.PopulationMultiplier"), "0.25");
    result = updatePZField(result, field("server.PVP"), false);
    expect(providerConfigValue(result, "sandbox.ZombieLore.Speed")).toBe(2);
    expect(providerConfigValue(result, "sandbox.ZombieConfig.PopulationMultiplier")).toBe(0.25);
    expect(providerConfigValue(result, "server.PVP")).toBe(false);
    expect((result.sandbox as Record<string, unknown>)["ZombieLore.Speed"]).toBeUndefined();
  });

  it("runs lookup and boolean dependencies for single edits without changing preset metadata", () => {
    const original = structuredClone(fixture);
    const payload = applyPZPreset({}, fixture);
    const changed = updatePZField(payload, field("sandbox.ZombieRespawn"), "1");
    expect(providerConfigValue(changed, "sandbox.ZombieConfig.RespawnHours")).toBe(16);
    expect(providerConfigValue(changed, "sandbox.ZombieConfig.RespawnUnseenHours")).toBe(6);
    expect(providerConfigValue(changed, "sandbox.ZombieConfig.RespawnMultiplier")).toBe(0.5);
    const disabled = updatePZField(changed, field("sandbox.ZombieMigrate"), false);
    expect(providerConfigValue(disabled, "sandbox.ZombieConfig.RedistributeHours")).toBe(0);
    expect(fixture).toEqual(original);
    expect(providerConfigValue(payload, "sandbox.ZombieConfig.RespawnHours")).toBe(fixture.values["ZombieConfig.RespawnHours"]);
  });

  it("clamps dates using the native date constraint and ignores hidden StartYear", () => {
    const january = { sandbox: { StartYear: 1, StartMonth: 1, StartDay: 31 } };
    const february = updatePZField(january, field("sandbox.StartMonth"), "2");
    expect(providerConfigValue(february, "sandbox.StartDay")).toBe(28);
    expect(pzStartDayMax(february)).toBe(28);
    const leap = updatePZField({ sandbox: { StartYear: 4, StartMonth: 1, StartDay: 31 } }, field("sandbox.StartMonth"), "2");
    expect(providerConfigValue(leap, "sandbox.StartDay")).toBe(28);
    const nextYear = updatePZField(leap, field("sandbox.StartYear"), "5");
    expect(providerConfigValue(nextYear, "sandbox.StartDay")).toBe(28);
    const april = updatePZField(january, field("sandbox.StartMonth"), "4");
    expect(providerConfigValue(april, "sandbox.StartDay")).toBe(30);
  });

  it("uses native category order and hides unsupported/internal fields", () => {
    expect(pzPages("server").map((page) => page.id)).toEqual(pzMetadata.pages.server.filter((page) => page.visible && page.fields.some((name) => isPZFieldVisible(field(`server.${name}`)))).map((page) => page.id));
    expect(pzPages("sandbox").map((page) => page.id)).toEqual(pzMetadata.pages.sandbox.filter((page) => page.visible).map((page) => page.id));
    for (const path of ["server.Mods", "server.WorkshopItems", "server.ResetID", "sandbox.WaterShut", "sandbox.ElecShut"]) expect(isPZFieldVisible(field(path))).toBe(false);
    expect(isPZFieldVisible(field("server.LootRespawn"))).toBe(pzMetadata.server.find((option) => option.name === "LootRespawn")?.visible === true);
    expect(isPZFieldVisible(field("server.PublicName"))).toBe(true);
  });

  it("seeds hidden sandbox defaults without labeling native defaults as a preset", () => {
    const provider = { key: "project-zomboid", configSchema: [] } as unknown as ProviderCatalog;
    const payload = createDefaultProviderConfigPayload(provider, { server: { PublicName: "Native defaults" } });
    expect(payload.sandbox).toEqual(pzSandboxDefaults());
    expect(payload.preset).toBeUndefined();
    expect(payload.server).toEqual({ PublicName: "Native defaults", UPnP: false, RCONPassword: "" });
  });
});


describe("Project Zomboid create validation", () => {
  const fields = ["server.PingLimit", "sandbox.WaterShutModifier", "sandbox.CharacterFreePoints", "server.PVP", "adminPassword", "server.ResetID"].map((name) => ({ ...field(name), required: true }));
  it("accepts native zero/negative values and false boolean with an empty initial password", () => {
    expect(validatePZConfig({ server: { PingLimit: 0, PVP: false }, sandbox: { WaterShutModifier: -1, CharacterFreePoints: -100 }, adminPassword: "" }, fields, "zh")).toEqual({});
  });
  it("rejects values below native minima and above maxima without validating hidden fields", () => {
    const errors = validatePZConfig({ server: { PingLimit: -1, PVP: false }, sandbox: { WaterShutModifier: -2, CharacterFreePoints: 101 } }, fields, "en");
    expect(Object.keys(errors).sort()).toEqual(["sandbox.CharacterFreePoints", "sandbox.WaterShutModifier", "server.PingLimit"]);
    expect(errors["server.ResetID"]).toBeUndefined();
    expect(errors.adminPassword).toBeUndefined();
  });
});


describe("verified official Project Zomboid presets", () => {
  it.each(pzMetadata.presets)("loads all native values for $id exactly once", (preset) => {
    expect(Object.keys(preset.values)).toHaveLength(pzMetadata.sandbox.length);
    const server = { PublicName: "Existing room", Password: "keep-password" };
    const applied = applyPZPreset({ server, adminPassword: "keep-admin", sandbox: { stale: true } }, preset);
    for (const option of pzMetadata.sandbox) {
      expect(providerConfigValue(applied, `sandbox.${option.name}`)).toEqual(preset.values[option.name]);
      expect(typeof providerConfigValue(applied, `sandbox.${option.name}`)).toBe(typeof option.defaultValue);
    }
    expect(applied.server).toBe(server);
    expect(applied.adminPassword).toBe("keep-admin");
    expect(providerConfigValue(applied, "sandbox.stale")).toBeUndefined();
  });
});


describe("verified PZ native UI interactions", () => {
  it.each([2.5, 1.6, 1.2, 0.65, 0.15, 0])("uses the verified Zombies lookup for population %s", (population) => {
    const index = [2.5, 1.6, 1.2, 0.65, 0.15, 0].indexOf(population) + 1;
    const result = updatePZField({ sandbox: { ZombieConfig: { PopulationMultiplier: 99 } } }, field("sandbox.Zombies"), String(index));
    expect(providerConfigValue(result, "sandbox.ZombieConfig.PopulationMultiplier")).toBe(population);
  });

  it("retains native default region order instead of sorting the official region inventory", () => {
    const spawnPage = pzPages("server").find((page) => page.id === "SpawnRegions");
    expect(spawnPage?.defaultRegions?.map((region) => region.name)).toEqual([
      "West Point, KY", "Rosewood, KY", "Echo Creek, KY", "Ekron, KY", "Valley Station, KY", "March Ridge, KY",
      "Irvington, KY", "Muldraugh, KY", "Fallas Lake, KY", "Riverside, KY", "Brandenburg, KY"
    ]);
    expect(pzPages("server").find((page) => page.id === "Map")?.officialMaps).toEqual(["Muldraugh, KY"]);
    expect(spawnPage?.defaultRegions?.every((region) => spawnPage.officialRegions?.some((official) => official.id === region.name))).toBe(true);
  });

  it("accepts non-negative integer coordinates without an invented map bound", () => {
    for (const value of ["0,0,0", "10000000,20000000,300"]) {
      expect(isPZSpawnPoint(value)).toBe(true);
      expect(providerConfigValue(updatePZField({}, field("server.SpawnPoint"), value), "server.SpawnPoint")).toBe(value);
    }
    for (const value of ["-1,0,0", "1.5,0,0", "1,2", "1,,3", "1e3,0,0", "1,2,3,4"]) {
      const payload = { server: { SpawnPoint: "0,0,0" } };
      expect(isPZSpawnPoint(value)).toBe(false);
      expect(updatePZField(payload, field("server.SpawnPoint"), value)).toBe(payload);
      expect(validatePZConfig({ server: { SpawnPoint: value } }, [field("server.SpawnPoint")], "en")["server.SpawnPoint"]).toBeTruthy();
    }
  });
});

describe("PZ INI string safety", () => {
  it.each(["\r", "\n", "\0", "\x01", "="])("rejects the forbidden character %j in every INI string/text option", (character) => {
    for (const option of pzMetadata.server.filter((option) => ["string", "text"].includes(option.type))) {
      const path = `server.${option.name}`;
      const value = `before${character}after`;
      expect(pzINIStringError(path, value, "en")).toBeTruthy();
      const payload = { server: { [option.name]: "safe" } };
      expect(updatePZField(payload, field(path), value)).toBe(payload);
      if (option.visible) expect(validatePZConfig({ server: { [option.name]: value } }, [field(path)], "zh")[path]).toBeTruthy();
    }
  });

  it("preserves literal backslash-n and <LINE> without decoding them", () => {
    const value = String.raw`Welcome\nSurvivor<LINE>Good luck`;
    const path = "server.ServerWelcomeMessage";
    expect(pzINIStringError(path, value, "zh")).toBeUndefined();
    expect(providerConfigValue(updatePZField({}, field(path), value), path)).toBe(value);
    expect(validatePZConfig({ server: { ServerWelcomeMessage: value } }, [field(path)], "en")).toEqual({});
  });
});


describe("PZ native runtime restrictions", () => {
  it("locks RCON and UPnP without hiding or changing official metadata", () => {
    for (const name of ["RCONPassword", "RCONPort", "UPnP"]) {
      expect(isPZRuntimeReadOnlyField(`server.${name}`)).toBe(true);
      expect(isPZFieldVisible(field(`server.${name}`))).toBe(true);
    }
    expect(isPZRuntimeReadOnlyField("server.PublicName")).toBe(false);
    expect(pzMetadata.server.find((option) => option.name === "UPnP")?.defaultValue).toBe(true);
    expect(pzMetadata.server.find((option) => option.name === "RCONPassword")?.defaultValue).toBe("");
    expect(pzMetadata.server.find((option) => option.name === "RCONPort")?.defaultValue).toBe(27015);
  });

  it("uses runtime-safe defaults for fresh drafts without rewriting loaded configuration", () => {
    const provider = { key: "project-zomboid", configSchema: [
      { ...field("server.UPnP"), type: "boolean", default: true },
      { ...field("server.RCONPassword"), type: "password", default: "" },
      { ...field("server.RCONPort"), type: "number", default: 27015 }
    ] } as unknown as ProviderCatalog;
    const fresh = createDefaultProviderConfigPayload(provider);
    expect(providerConfigValue(fresh, "server.UPnP")).toBe(false);
    expect(providerConfigValue(fresh, "server.RCONPassword")).toBe("");
    expect(providerConfigValue(fresh, "server.RCONPort")).toBe(27015);
    const loaded = createDefaultProviderConfigPayload(provider, { server: { PublicName: "Existing", UPnP: false, RCONPassword: "" } });
    expect(providerConfigValue(loaded, "server.PublicName")).toBe("Existing");
    expect(providerConfigValue(loaded, "server.UPnP")).toBe(false);
  });
});


it("rejects trailing ASCII controls in INI passwords without restricting sandbox Lua strings", () => {
  const password = "join\x01";
  expect(pzINIStringError("server.Password", password, "en")).toBeTruthy();
  expect(validatePZConfig({ server: { Password: password } }, [field("server.Password")], "en")["server.Password"]).toBeTruthy();
  for (let code = 0; code < 0x20; code++) {
    expect(pzINIStringError("server.Password", `join${String.fromCharCode(code)}`, "en")).toBeTruthy();
  }
  const sandboxString = pzMetadata.sandbox.find((option) => ["string", "text"].includes(option.type));
  expect(sandboxString).toBeDefined();
  expect(pzINIStringError(`sandbox.${sandboxString!.name}`, password, "en")).toBeUndefined();
});
