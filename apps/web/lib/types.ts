import type { TerrariaConfig } from "@gamepanel-lite/shared";

export type ServerStatus = "creating" | "starting" | "running" | "stopping" | "stopped" | "restarting" | "deleting" | "errored";
export type ServerMode = "vanilla" | "tmodloader";
export type GameKey = "terraria" | "palworld" | string;
export type ProviderKey = "terraria-vanilla" | "terraria-tmodloader" | string;

export type ProviderCapabilities = {
  consoleCommands: boolean;
  playerList: boolean;
  kickPlayer: boolean;
  banPlayer: boolean;
  saveSnapshots: boolean;
  backups: boolean;
  mods: boolean;
  versions: boolean;
};

export type ProviderConfigField = {
  name: string;
  label: string;
  type: "text" | "password" | "number" | "select" | "boolean" | string;
  required: boolean;
  default?: unknown;
  help?: string;
  options?: Array<{ value: string; label: string }>;
};

export type ProviderCatalog = {
  key: ProviderKey;
  name: string;
  description: string;
  recommended: boolean;
  versions: string[];
  capabilities: ProviderCapabilities;
  configSchema: ProviderConfigField[];
};

export type GameCatalogEntry = {
  key: GameKey;
  name: string;
  description: string;
  status: "available" | "planned" | string;
  providers: ProviderCatalog[];
};

export type ResourceLimits = {
  cpuLimitCores: number;
  memoryLimitMb: number;
};

export type ServerJoinInfo = {
  address: string;
  port: number;
  password?: string;
  inviteText: string;
  instructions?: string[];
};

export type Server = {
  id: string;
  name: string;
  gameKey?: GameKey;
  providerKey?: ProviderKey;
  mode: ServerMode;
  status: ServerStatus;
  world: string;
  players: number;
  maxPlayers: number;
  port: number;
  hostPort: number;
  cpuLimitCores: number;
  memoryLimitMb: number;
  version: string;
  lastError?: string;
  sourceWorldId?: string;
  sourceWorldName?: string;
  lastBackup: string;
  password: string;
  cpu: string;
  memory: string;
  config: TerrariaConfig;
  configPayload?: Record<string, unknown>;
  joinInfo?: ServerJoinInfo;
  configPendingRestart?: boolean;
};

export type World = {
  id: string;
  instanceId?: string;
  activeInstanceId?: string;
  providerKey?: ProviderKey;
  name: string;
  size: string;
  difficulty: string;
  server?: string;
  modified: string;
  bytes: string;
  source?: string;
  config?: TerrariaConfig;
};

export type Backup = {
  id: string;
  name: string;
  instanceId?: string;
  server: string;
  world: string;
  type: "Auto" | "Manual";
  size: string;
  sizeBytes: number;
  created: string;
  createdAt: string;
};

export type ModFile = {
  id: string;
  instanceId: string;
  fileName: string;
  source?: "upload" | "workshop" | string;
  workshopId?: string;
  modName?: string;
  title?: string;
  modVersion?: string;
  tmodVersion?: string;
  creatorSteamId?: string;
  previewUrl?: string;
  description?: string;
  tags?: string[];
  subscriptions?: number;
  favorited?: number;
  views?: number;
  updatedAtSteam?: number;
  size: string;
  sizeBytes?: number;
  enabled: boolean;
  runtimeEnabled?: boolean;
  runtimePresent?: boolean;
  dependencies?: string[];
  created: string;
};

export type RecommendedMod = {
  rank: number;
  workshopId: string;
  modName?: string;
  title: string;
  creatorSteamId?: string;
  previewUrl?: string;
  fileSize: number;
  size: string;
  subscriptions?: number;
  favorited?: number;
  views?: number;
  timeCreated?: number;
  timeUpdated?: number;
  tags?: string[];
  description?: string;
  dependencies?: string[];
  inLibrary: boolean;
  modId?: string;
};

export type ModPack = {
  id: string;
  name: string;
  description: string;
  modIds: string[];
  mods: ModFile[];
  created: string;
};

export type ActivityEvent = {
  id: string;
  instanceId?: string;
  type: string;
  message: string;
  created: string;
};
