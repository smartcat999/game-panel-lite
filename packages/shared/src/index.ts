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
  seed: z.string().max(512).optional(),
  specialSeeds: z.array(z.string().min(1).max(64)).default([]),
  secretSeeds: z.array(z.string().min(1).max(64)).default([]),
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
      specialSeeds: [],
      secretSeeds: [],
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
      specialSeeds: [],
      secretSeeds: [],
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
      specialSeeds: [],
      secretSeeds: [],
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
      specialSeeds: [],
      secretSeeds: [],
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
      specialSeeds: [],
      secretSeeds: [],
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

export const terrariaSpecialWorldSeeds = [
  { key: "05162020", label: "Drunk World", description: "融合两个世界布局，NPC 和物品混搭" },
  { key: "for the worthy", label: "For the Worthy", description: "大幅增加难度，敌人更强、掉落更丰厚" },
  { key: "not the bees", label: "Not the Bees", description: "世界主要由蜂巢块和幼虫组成" },
  { key: "celebrationmk10", label: "CelebrationMK10", description: "派对世界，稀有物品和烟花" },
  { key: "the constant", label: "The Constant", description: "饥荒联动世界，加入饥饿与独特光照机制" },
  { key: "no traps", label: "No Traps", description: "陷阱密度显著提升，探索更危险" },
  { key: "dontdigup", label: "Don't Dig Up", description: "世界反转，出生点在地下深处" },
  { key: "getfixedboi", label: "Get Fixed Boi (天顶)", description: "多个特殊世界规则的终极组合" },
  { key: "skyblock", label: "Skyblock", description: "1.4.5 新增，空岛式世界生成" }
] as const;

export const terrariaSecretWorldSeeds145 = [
  { key: "invisible plane", label: "Invisible Plane", description: "隐藏部分世界结构，探索更依赖记忆与试探" },
  { key: "monochrome", label: "Monochrome", description: "世界呈现单色视觉风格" },
  { key: "negative infinity", label: "Negative Infinity", description: "改变地下与洞穴生成节奏" },
  { key: "x-ray vision", label: "X-ray Vision", description: "强调可视化矿物与地下结构" },
  { key: "jagged rocks", label: "Jagged Rocks", description: "地形更加尖锐破碎" },
  { key: "mole people", label: "Mole People", description: "地下活动与结构更突出" },
  { key: "planetoids", label: "Planetoids", description: "生成更偏碎片化的星体式地形" },
  { key: "such great heights", label: "Such Great Heights", description: "强化高空探索与垂直空间" },
  { key: "waterpark", label: "Waterpark", description: "水体与水上探索更显著" },
  { key: "arachnophobia", label: "Arachnophobia", description: "蜘蛛主题内容更集中" },
  { key: "does that sparkle", label: "Does That Sparkle", description: "宝石与闪光资源更突出" },
  { key: "fish mox", label: "Fish Mox", description: "钓鱼与水生内容更突出" },
  { key: "purify this", label: "Purify This", description: "净化与邪恶生态相关变化" },
  { key: "sandy britches", label: "Sandy Britches", description: "沙漠生态与沙地生成更突出" },
  { key: "toadstool", label: "Toadstool", description: "蘑菇生态内容更突出" },
  { key: "winter is coming", label: "Winter Is Coming", description: "雪地与寒冷生态更突出" },
  { key: "abandoned manors", label: "Abandoned Manors", description: "废弃建筑与探索点更多" },
  { key: "beam me up", label: "Beam Me Up", description: "传送与空间主题变化" },
  { key: "more traps please", label: "More Traps Please", description: "额外陷阱与危险机关" },
  { key: "pumpkin season", label: "Pumpkin Season", description: "南瓜与季节主题内容更突出" },
  { key: "rainbow road", label: "Rainbow Road", description: "彩虹视觉与高空路线主题" },
  { key: "save the rainforest", label: "Save the Rainforest", description: "丛林生态更突出" },
  { key: "the care bears movie", label: "The Care Bears Movie", description: "色彩与友好主题变化" },
  { key: "truck stop", label: "Truck Stop", description: "旅途与补给主题变化" },
  { key: "we don't even test for that", label: "We Don't Even Test For That", description: "实验性组合规则" },
  { key: "bring a towel", label: "Bring a Towel", description: "水与旅行主题变化" },
  { key: "hocus pocus", label: "Hocus Pocus", description: "魔法主题内容更突出" },
  { key: "jingle all the way", label: "Jingle All The Way", description: "节日与雪地氛围变化" },
  { key: "how did i get here", label: "How Did I Get Here", description: "探索路径更出人意料" },
  { key: "royale with cheese", label: "Royale With Cheese", description: "特殊掉落与趣味主题变化" },
  { key: "double daring dangers", label: "Double Daring Dangers", description: "危险要素进一步叠加" },
  { key: "i am error", label: "I Am Error", description: "异常与错位主题变化" },
  { key: "night of the living dead", label: "Night Of The Living Dead", description: "亡灵与夜晚威胁更突出" },
  { key: "too easy", label: "Too Easy", description: "降低部分挑战感的趣味规则" },
  { key: "what a horrible night to have a curse", label: "What A Horrible Night To Have A Curse", description: "夜晚与诅咒主题变化" }
] as const;

export const terrariaSecretSeeds = terrariaSpecialWorldSeeds;
export const terrariaLegacySpecialWorldSeeds = terrariaSpecialWorldSeeds.filter((seed) => seed.key !== "skyblock");

function normalizeSeedCode(seed: string | undefined): string {
  return (seed ?? "").trim().toLowerCase().replace(/[^a-z0-9]/g, "");
}

export function secretSeedKeyFor(seed: string | undefined): string {
  const normalized = normalizeSeedCode(seed);
  return terrariaSpecialWorldSeeds.find((item) => normalizeSeedCode(item.key) === normalized)?.key ?? "";
}

export function isTerrariaVersionAtLeast(version: string | undefined, target: string): boolean {
  const left = (version ?? "").split(".").map((part) => Number.parseInt(part, 10));
  const right = target.split(".").map((part) => Number.parseInt(part, 10));
  const length = Math.max(left.length, right.length);
  for (let index = 0; index < length; index += 1) {
    const leftValue = left[index] ?? 0;
    const rightValue = right[index] ?? 0;
    const leftPart = Number.isFinite(leftValue) ? leftValue : 0;
    const rightPart = Number.isFinite(rightValue) ? rightValue : 0;
    if (leftPart > rightPart) return true;
    if (leftPart < rightPart) return false;
  }
  return true;
}

function uniqueKnownSeeds(values: string[] | undefined, known: readonly { key: string }[]) {
  const knownByCode = new Map(known.map((item) => [normalizeSeedCode(item.key), item.key]));
  const next: string[] = [];
  for (const value of values ?? []) {
    const key = knownByCode.get(normalizeSeedCode(value));
    if (key && !next.includes(key)) next.push(key);
  }
  return next;
}

export function terrariaSeedModeCodes(config: TerrariaConfig): string[] {
  return [
    ...uniqueKnownSeeds(config.specialSeeds, terrariaSpecialWorldSeeds),
    ...uniqueKnownSeeds(config.secretSeeds, terrariaSecretWorldSeeds145)
  ];
}

export function renderTerrariaSeedValue(config: TerrariaConfig): string {
  const seed = (config.seed ?? "").trim();
  const modes = terrariaSeedModeCodes(config);
  if (modes.length === 0) return seed;
  return `1.1.1.${seed || "0"}.${modes.join("|")}|`;
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
    `seed=${renderTerrariaSeedValue(config)}`,
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
  payload: z.record(z.string(), z.unknown()).optional(),
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
