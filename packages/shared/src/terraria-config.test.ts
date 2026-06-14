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
      difficulty=3
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
