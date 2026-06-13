import { describe, expect, it } from "vitest";

import {
  gameServerInstanceSchema,
  serverPortSchema,
  terrariaConfigSchema
} from "./index";

describe("shared schemas", () => {
  it("accepts a valid Terraria server configuration", () => {
    const result = terrariaConfigSchema.parse({
      serverName: "Friends Server",
      worldName: "Journey Base",
      worldSize: "medium",
      difficulty: "expert",
      maxPlayers: 8,
      port: 7777,
      password: "friends-only",
      motd: "Bring potions",
      seed: "not-the-bees",
      secure: true,
      language: "en-US",
      autoCreate: true
    });

    expect(result.maxPlayers).toBe(8);
    expect(result.worldSize).toBe("medium");
  });

  it("rejects invalid ports before they reach runtime adapters", () => {
    expect(() => serverPortSchema.parse(80)).toThrow();
    expect(() => serverPortSchema.parse(7777)).not.toThrow();
    expect(() => serverPortSchema.parse(70000)).toThrow();
  });

  it("keeps V1 server instances scoped to Terraria provider keys", () => {
    const parsed = gameServerInstanceSchema.parse({
      id: "srv_terraria_01",
      name: "Friends World",
      gameKey: "terraria",
      providerKey: "terraria-vanilla",
      status: "stopped",
      port: 7777,
      maxPlayers: 8,
      createdAt: new Date(),
      updatedAt: new Date()
    });

    expect(parsed.providerKey).toBe("terraria-vanilla");
  });
});
