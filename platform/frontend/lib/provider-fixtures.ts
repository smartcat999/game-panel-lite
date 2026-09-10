export type ConfigurationField = {
  type: "string" | "integer" | "number" | "boolean" | "enum" | "secret" | "string-list";
  title: string;
  description?: string;
  default?: string | number | boolean | string[];
  minimum?: number;
  maximum?: number;
  enum?: Array<string | number>;
  applyBehavior: "hot-reload" | "restart-required" | "recreate-required" | "create-only";
};

export type ProviderManifestFixture = {
  providerReleaseId: string;
  gameKey: string;
  displayName: string;
  releaseVersion: string;
  gameVersions: string[];
  schemaVersion: number;
  capabilities: Array<"configuration" | "mods" | "console" | "logs" | "backup" | "player-observation" | "game-metrics">;
  configurationSchema: { type: "object"; properties: Record<string, ConfigurationField>; required: string[] };
  uiSchema: {
    sections: Array<{ id: string; title: string; order: number }>;
    fields: Record<string, { section: string; order: number; control: "text" | "textarea" | "password" | "tag-list" | "number" | "switch" | "select"; visibleWhen?: { field: string; equals: string | number | boolean } }>;
  };
  modCatalog?: { revision: number; entries: Array<{ modId: string; displayName: string; versions: Array<{ version: string }> }> };
};

export const providerManifests: ProviderManifestFixture[] = [
  {
    providerReleaseId: "prv_terraria_1449", gameKey: "terraria", displayName: "Terraria", releaseVersion: "1", gameVersions: ["1.4.4.9"], schemaVersion: 1,
    capabilities: ["configuration", "console", "logs", "backup", "player-observation"],
    configurationSchema: { type: "object", required: ["serverName", "maxPlayers", "worldSize", "difficulty"], properties: {
      serverName: { type: "string", title: "服务器名称", default: "Ember Realms", description: "显示在游戏服务器列表中", applyBehavior: "hot-reload" },
      maxPlayers: { type: "integer", title: "最大玩家数", default: 16, minimum: 2, maximum: 32, applyBehavior: "restart-required" },
      password: { type: "secret", title: "服务器密码", default: "", applyBehavior: "restart-required" },
      worldSize: { type: "enum", title: "世界大小", default: "medium", enum: ["small", "medium", "large"], applyBehavior: "create-only" },
      difficulty: { type: "enum", title: "难度", default: "expert", enum: ["classic", "expert", "master"], applyBehavior: "restart-required" },
      secure: { type: "boolean", title: "启用安全校验", default: true, applyBehavior: "restart-required" },
    } },
    uiSchema: { sections: [{ id: "server", title: "服务器", order: 1 }, { id: "session", title: "游戏规则", order: 2 }], fields: {
      serverName: { section: "server", order: 1, control: "text" }, maxPlayers: { section: "server", order: 2, control: "number" }, password: { section: "server", order: 3, control: "password" },
      worldSize: { section: "session", order: 1, control: "select" }, difficulty: { section: "session", order: 2, control: "select" }, secure: { section: "session", order: 3, control: "switch" },
    } },
  },
  {
    providerReleaseId: "prv_tmod_202506", gameKey: "tmodloader", displayName: "tModLoader", releaseVersion: "1", gameVersions: ["2025.06"], schemaVersion: 1,
    capabilities: ["configuration", "mods", "console", "logs", "backup", "player-observation"],
    configurationSchema: { type: "object", required: ["serverName", "maxPlayers"], properties: {
      serverName: { type: "string", title: "服务器名称", default: "Ember Modded", description: "显示在游戏服务器列表中", applyBehavior: "hot-reload" },
      maxPlayers: { type: "integer", title: "最大玩家数", default: 12, minimum: 2, maximum: 32, applyBehavior: "restart-required" },
      password: { type: "secret", title: "服务器密码", default: "", applyBehavior: "restart-required" },
      autoUpdate: { type: "boolean", title: "启动时更新模组", default: true, applyBehavior: "restart-required" },
      additionalArgs: { type: "string-list", title: "附加启动参数", default: [], applyBehavior: "recreate-required" },
    } },
    uiSchema: { sections: [{ id: "server", title: "服务器", order: 1 }, { id: "runtime", title: "运行", order: 2 }], fields: {
      serverName: { section: "server", order: 1, control: "text" }, maxPlayers: { section: "server", order: 2, control: "number" }, password: { section: "server", order: 3, control: "password" },
      autoUpdate: { section: "runtime", order: 1, control: "switch" }, additionalArgs: { section: "runtime", order: 2, control: "tag-list", visibleWhen: { field: "autoUpdate", equals: true } },
    } },
    modCatalog: { revision: 1, entries: [
      { modId: "CalamityMod", displayName: "Calamity Mod", versions: [{ version: "2.1.3" }, { version: "2.1.2" }] },
      { modId: "InfernumMode", displayName: "Infernum Mode", versions: [{ version: "2.0.1" }] },
    ] },
  },
];

export function defaultConfiguration(manifest: ProviderManifestFixture) {
  return Object.fromEntries(Object.entries(manifest.configurationSchema.properties).map(([key, field]) => [key, field.default ?? ""]));
}
