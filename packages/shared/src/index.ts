import { z } from "zod";

export const gameKeySchema = z.literal("terraria");

export const terrariaProviderKeySchema = z.enum([
  "terraria-vanilla",
  "terraria-tmodloader"
]);

export const serverStatusSchema = z.enum([
  "creating",
  "running",
  "stopped",
  "restarting",
  "errored"
]);

export const worldSizeSchema = z.enum(["small", "medium", "large"]);

export const worldEvilSchema = z.enum(["random", "corruption", "crimson"]);

export const terrariaDifficultySchema = z.enum([
  "journey",
  "classic",
  "expert",
  "master"
]);

export const serverPortSchema = z
  .number()
  .int()
  .min(1024, "Port must be between 1024 and 65535")
  .max(65535, "Port must be between 1024 and 65535");

export const terrariaInternalPort = 7777;
export const terrariaDefaultLanguage = "en-US";

export const terrariaConfigSchema = z.object({
  serverName: z.string().min(1).max(80).optional(),
  worldName: z.string().min(1).max(80),
  worldSize: worldSizeSchema,
  worldEvil: worldEvilSchema.default("random"),
  difficulty: terrariaDifficultySchema,
  maxPlayers: z
    .number()
    .int()
    .min(1, "Max players must be between 1 and 255")
    .max(255, "Max players must be between 1 and 255"),
  port: serverPortSchema,
  password: z.string().max(128).optional(),
  motd: z.string().max(256).optional(),
  seed: z.string().max(128).optional(),
  secure: z.boolean().default(true),
  language: z.string().min(2).max(12).default(terrariaDefaultLanguage),
  autoCreateWorld: z.boolean().default(true)
}).superRefine((config, context) => {
  if (config.worldName.includes("..") || /[/\\]/.test(config.worldName)) {
    context.addIssue({
      code: z.ZodIssueCode.custom,
      message: "World name cannot contain path traversal characters",
      path: ["worldName"]
    });
  }
});

export const terrariaPresetSchema = z.object({
  key: z.enum([
    "friends-casual",
    "expert-adventure",
    "master-challenge",
    "building-world",
    "modded-starter"
  ]),
  label: z.string().min(1),
  description: z.string().min(1),
  config: terrariaConfigSchema
});

export const terrariaPresets = [
  {
    key: "friends-casual",
    label: "Friends Casual",
    description: "Relaxed co-op defaults for a small friend group.",
    config: {
      serverName: "Friends Server",
      worldName: "Friends World",
      worldSize: "medium",
      worldEvil: "random",
      difficulty: "classic",
      maxPlayers: 8,
      port: terrariaInternalPort,
      password: "",
      motd: "Welcome to GamePanel Lite",
      seed: "",
      secure: true,
      language: terrariaDefaultLanguage,
      autoCreateWorld: true
    }
  },
  {
    key: "expert-adventure",
    label: "Expert Adventure",
    description: "A tougher cooperative world for experienced players.",
    config: {
      serverName: "Expert Adventure",
      worldName: "Expert Adventure",
      worldSize: "large",
      worldEvil: "random",
      difficulty: "expert",
      maxPlayers: 8,
      port: terrariaInternalPort,
      password: "",
      motd: "Bring potions",
      seed: "",
      secure: true,
      language: terrariaDefaultLanguage,
      autoCreateWorld: true
    }
  },
  {
    key: "master-challenge",
    label: "Master Challenge",
    description: "High-intensity defaults for players who want pressure.",
    config: {
      serverName: "Master Challenge",
      worldName: "Master Challenge",
      worldSize: "large",
      worldEvil: "random",
      difficulty: "master",
      maxPlayers: 6,
      port: terrariaInternalPort,
      password: "",
      motd: "Good luck",
      seed: "",
      secure: true,
      language: terrariaDefaultLanguage,
      autoCreateWorld: true
    }
  },
  {
    key: "building-world",
    label: "Building World",
    description: "Roomy, calm defaults for builders and decorators.",
    config: {
      serverName: "Building World",
      worldName: "Builder Base",
      worldSize: "large",
      worldEvil: "random",
      difficulty: "classic",
      maxPlayers: 12,
      port: terrariaInternalPort,
      password: "",
      motd: "Build something sharp",
      seed: "",
      secure: true,
      language: terrariaDefaultLanguage,
      autoCreateWorld: true
    }
  },
  {
    key: "modded-starter",
    label: "Modded Starter",
    description: "A conservative starting point for tModLoader servers.",
    config: {
      serverName: "Modded Starter",
      worldName: "Modded Starter",
      worldSize: "medium",
      worldEvil: "random",
      difficulty: "classic",
      maxPlayers: 8,
      port: terrariaInternalPort,
      password: "",
      motd: "Mods enabled",
      seed: "",
      secure: true,
      language: terrariaDefaultLanguage,
      autoCreateWorld: true
    }
  }
] as const;

export function getTerrariaPreset(
  key: z.infer<typeof terrariaPresetSchema>["key"]
) {
  const preset = terrariaPresets.find((item) => item.key === key);

  if (!preset) {
    throw new Error(`Unknown Terraria preset: ${key}`);
  }

  return preset;
}

export const terrariaSecretSeeds = [
  { key: "05162020", label: "Drunk World", description: "融合两个世界布局，NPC 和物品混搭" },
  { key: "for the worthy", label: "For the Worthy", description: "大幅增加难度，敌人更强、掉落更丰厚" },
  { key: "not the bees", label: "Not the Bees", description: "世界主要由蜂巢块和幼虫组成" },
  { key: "celebrationmk10", label: "CelebrationMK10", description: "派对世界，稀有物品和烟花" },
  { key: "dontdigup", label: "Don't Dig Up", description: "世界反转，出生点在地下深处" },
  { key: "getfixedboi", label: "Get Fixed Boi (天顶)", description: "所有彩蛋种子效果的终极组合" }
] as const;

export function secretSeedKeyFor(seed: string | undefined): string {
  return terrariaSecretSeeds.find((item) => item.key === (seed ?? "").toLowerCase())?.key ?? "";
}

const worldEvilConfigValues = {
  random: 0,
  corruption: 1,
  crimson: 2
} satisfies Record<z.infer<typeof worldEvilSchema>, number>;

const worldSizeConfigValues = {
  small: 1,
  medium: 2,
  large: 3
} satisfies Record<z.infer<typeof worldSizeSchema>, number>;

const difficultyConfigValues = {
  journey: 0,
  classic: 1,
  expert: 2,
  master: 3
} satisfies Record<z.infer<typeof terrariaDifficultySchema>, number>;

export function renderTerrariaServerConfig(config: TerrariaConfig): string {
  return [
    `world=worlds/${config.worldName}.wld`,
    `autocreate=${worldSizeConfigValues[config.worldSize]}`,
    `difficulty=${difficultyConfigValues[config.difficulty]}`,
    `worldevil=${worldEvilConfigValues[config.worldEvil]}`,
    `maxplayers=${config.maxPlayers}`,
    `port=${config.port}`,
    `password=${config.password ?? ""}`,
    `motd=${config.motd ?? ""}`,
    `seed=${config.seed ?? ""}`,
    `secure=${config.secure ? 1 : 0}`,
    `language=${terrariaDefaultLanguage}`
  ].join("\n");
}

export const resourceLimitSchema = z.object({
  cpuLimitCores: z.number().min(0).max(64).refine((value) => value === 0 || value >= 0.25, "CPU limit must be 0 or at least 0.25 cores").default(0),
  memoryLimitMb: z.number().int().min(0).max(262144).default(0)
});

export const gameServerInstanceSchema = z.object({
  id: z.string().min(1),
  name: z.string().min(1).max(80),
  gameKey: gameKeySchema,
  providerKey: terrariaProviderKeySchema,
  status: serverStatusSchema,
  port: serverPortSchema,
  maxPlayers: z.number().int().min(1).max(255),
  cpuLimitCores: z.number().nonnegative().optional(),
  memoryLimitMb: z.number().int().nonnegative().optional(),
  lastError: z.string().optional(),
  sourceWorldId: z.string().optional(),
  sourceWorldName: z.string().optional(),
  createdAt: z.date(),
  updatedAt: z.date()
});

export const backupSchema = z.object({
  id: z.string().min(1),
  instanceId: z.string().min(1),
  fileName: z.string().min(1),
  sizeBytes: z.number().int().nonnegative(),
  createdAt: z.date()
});

export const worldSchema = z.object({
  id: z.string().min(1),
  providerKey: terrariaProviderKeySchema.optional(),
  name: z.string().min(1).max(80),
  fileName: z.string().endsWith(".wld"),
  sizeBytes: z.number().int().nonnegative(),
  activeInstanceId: z.string().min(1).nullable(),
  createdAt: z.date(),
  updatedAt: z.date()
});

export const modFileSchema = z.object({
  id: z.string().min(1),
  fileName: z.string().min(1),
  source: z.enum(["upload", "workshop"]).optional(),
  workshopId: z.string().regex(/^\d+$/).optional(),
  sizeBytes: z.number().int().nonnegative(),
  createdAt: z.date()
});

export const activityEventSchema = z.object({
  id: z.string().min(1),
  instanceId: z.string().min(1).nullable(),
  type: z.string().min(1).max(64),
  message: z.string().min(1).max(256),
  createdAt: z.date()
});

export type GameKey = z.infer<typeof gameKeySchema>;
export type TerrariaProviderKey = z.infer<typeof terrariaProviderKeySchema>;
export type ServerStatus = z.infer<typeof serverStatusSchema>;
export type TerrariaConfig = z.infer<typeof terrariaConfigSchema>;
export type TerrariaPreset = z.infer<typeof terrariaPresetSchema>;
export type ResourceLimits = z.infer<typeof resourceLimitSchema>;
export type GameServerInstance = z.infer<typeof gameServerInstanceSchema>;
export type Backup = z.infer<typeof backupSchema>;
export type World = z.infer<typeof worldSchema>;
export type ModFile = z.infer<typeof modFileSchema>;
export type ActivityEvent = z.infer<typeof activityEventSchema>;
