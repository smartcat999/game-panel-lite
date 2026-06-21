import { describe, expect, it } from "vitest";

import {
  getTerrariaPreset,
  renderTerrariaServerConfig,
  terrariaConfigSchema,
  terrariaPresetSchema,
  terrariaPresets
} from "./index";

describe("Terraria presets", () => {
  it("defines valid preset configurations", () => {
    expect(terrariaPresets).toHaveLength(5);

    for (const preset of terrariaPresets) {
      expect(() => terrariaPresetSchema.parse(preset)).not.toThrow();
      expect(() => terrariaConfigSchema.parse(preset.config)).not.toThrow();
    }
  });

  it("resolves presets by key", () => {
    expect(getTerrariaPreset("expert-adventure").label).toBe(
      "Expert Adventure"
    );
  });

  it("uses English as the fixed server language for presets", () => {
    expect(getTerrariaPreset("friends-casual").config.language).toBe("en-US");
  });
});

describe("Terraria config renderer", () => {
  it("renders serverconfig.txt from a validated config", () => {
    const config = terrariaConfigSchema.parse({
      serverName: "Moon Base",
      worldName: "Lunar Outpost",
      worldSize: "large",
      worldEvil: "corruption",
      difficulty: "master",
      maxPlayers: 12,
      port: 7778,
      password: "stars",
      motd: "Mind the wyverns",
      seed: "05162020",
      secure: true,
      language: "en-US",
      autoCreate: true
    });

    expect(renderTerrariaServerConfig(config)).toMatchInlineSnapshot(`
      "world=worlds/Lunar Outpost.wld
      autocreate=3
      difficulty=2
      worldevil=1
      maxplayers=12
      port=7778
      password=stars
      motd=Mind the wyverns
      seed=05162020
      secure=1
      language=en-US"
    `);
  });

  it("renders Terraria 1.4.5 seed mode combinations", () => {
    const config = terrariaConfigSchema.parse({
      ...getTerrariaPreset("friends-casual").config,
      seed: "daily-run",
      specialSeeds: ["for the worthy", "skyblock"],
      secretSeeds: ["beam me up"]
    });

    expect(renderTerrariaServerConfig(config)).toContain(
      "seed=1.1.1.daily-run.for the worthy|skyblock|beam me up|"
    );
  });

  it("rejects invalid ports", () => {
    expect(() =>
      terrariaConfigSchema.parse({
        ...getTerrariaPreset("friends-casual").config,
        port: 70000
      })
    ).toThrow(/Port must be between/);
  });

  it("rejects invalid max players", () => {
    expect(() =>
      terrariaConfigSchema.parse({
        ...getTerrariaPreset("friends-casual").config,
        maxPlayers: 0
      })
    ).toThrow(/Max players must be between/);
  });

  it("rejects invalid world names", () => {
    expect(() =>
      terrariaConfigSchema.parse({
        ...getTerrariaPreset("friends-casual").config,
        worldName: "../outside"
      })
    ).toThrow(/World name/);
  });
});
