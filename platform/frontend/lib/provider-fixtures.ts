export type ConfigurationField = {
  type: "string" | "integer" | "number" | "boolean" | "enum" | "secret" | "string-list";
  title: string;
  description?: string;
  default?: string | number | boolean | string[];
  minimum?: number;
  maximum?: number;
  options?: Array<{ label: string; value: string }>;
};

export type ProviderManifestFixture = {
  id: string;
  game: "Terraria" | "tModLoader";
  version: string;
  capabilities: Array<"configuration" | "mods" | "console" | "logs" | "backup" | "player-observation">;
  configuration: Array<{ id: string; title: string; fields: Record<string, ConfigurationField> }>;
};

export const providerManifests: ProviderManifestFixture[] = [
  {
    id: "prv_terraria_1449",
    game: "Terraria",
    version: "1.4.4.9",
    capabilities: ["configuration", "console", "logs", "backup", "player-observation"],
    configuration: [
      { id: "server", title: "服务器", fields: {
        serverName: { type: "string", title: "服务器名称", default: "Ember Realms", description: "显示在游戏服务器列表中" },
        maxPlayers: { type: "integer", title: "最大玩家数", default: 16, minimum: 2, maximum: 32 },
        password: { type: "secret", title: "服务器密码", default: "" },
      } },
      { id: "world", title: "世界", fields: {
        worldSize: { type: "enum", title: "世界大小", default: "medium", options: [{ label: "小", value: "small" }, { label: "中", value: "medium" }, { label: "大", value: "large" }] },
        difficulty: { type: "enum", title: "难度", default: "expert", options: [{ label: "经典", value: "classic" }, { label: "专家", value: "expert" }, { label: "大师", value: "master" }] },
        secure: { type: "boolean", title: "启用安全校验", default: true },
      } },
    ],
  },
  {
    id: "prv_tmod_202506",
    game: "tModLoader",
    version: "2025.06",
    capabilities: ["configuration", "mods", "console", "logs", "backup", "player-observation"],
    configuration: [
      { id: "server", title: "服务器", fields: {
        serverName: { type: "string", title: "服务器名称", default: "Ember Modded", description: "显示在游戏服务器列表中" },
        maxPlayers: { type: "integer", title: "最大玩家数", default: 12, minimum: 2, maximum: 32 },
        password: { type: "secret", title: "服务器密码", default: "" },
      } },
      { id: "runtime", title: "运行", fields: {
        autoUpdate: { type: "boolean", title: "启动时更新模组", default: true },
        additionalArgs: { type: "string-list", title: "附加启动参数", default: [] },
      } },
    ],
  },
];

export function defaultConfiguration(manifest: ProviderManifestFixture) {
  return Object.fromEntries(manifest.configuration.flatMap((section) => Object.entries(section.fields).map(([key, field]) => [key, field.default ?? ""])));
}
