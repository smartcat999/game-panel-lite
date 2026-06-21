import { describe, expect, it } from "vitest";
import { createDefaultProviderConfigPayload, providerConfigValue, updateProviderConfigPayload } from "./provider-config";
import type { ProviderCatalog } from "./types";

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
});
