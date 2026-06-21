export type ServerStatus = "creating" | "starting" | "running" | "stopping" | "stopped" | "restarting" | "deleting" | "errored";
export type ServerMode = "vanilla" | "tmodloader";
export type GameKey = "terraria" | "palworld" | string;
export type ProviderKey = "terraria-vanilla" | "terraria-tmodloader" | string;

export type ProviderCapabilities = {
  consoleCommands: boolean;
  playerList: boolean;
  kickPlayer: boolean;
  banPlayer: boolean;
  whitelist: boolean;
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

export type RuntimeImageStatus = {
  image: string;
  status: "ready" | "missing" | "preparing" | "failed" | "unsupported" | string;
  message?: string;
  progress?: number;
  updatedAt?: string;
};

export type ProviderCatalog = {
  key: ProviderKey;
  name: string;
  description: string;
  recommended: boolean;
  versions: string[];
  recommendedVersion?: string;
  capabilities: ProviderCapabilities;
  configSchema: ProviderConfigField[];
  saveDisplayName?: string;
  runtimeImage?: RuntimeImageStatus;
};

export type GameCatalogEntry = {
  key: GameKey;
  name: string;
  description: string;
  status: "available" | "planned" | string;
  coverImage?: string;
  serverCount?: number;
  providers: ProviderCatalog[];
};

export type ServerPlayerListResponse = {
  supported: boolean;
  players: Array<{ name?: string }>;
};

export type ServerWhitelistResponse = {
  supported: boolean;
  running: boolean;
};

export type SaveSnapshotListResponse = {
  saveDisplayName: string;
  saves: Backup[];
};

export type ResourceLimits = {
  cpuLimitCores: number;
  memoryLimitMb: number;
};

export type ServerDesiredState = "running" | "stopped" | "deleted" | string;
export type ServerPhase = "pending" | "reconciling" | "running" | "stopped" | "failed" | "deleting" | "deleted" | string;
export type ServerActualState = "running" | "stopped" | "missing" | "unknown" | string;

export type ServerResourceSpec = {
  generation: number;
  desiredState: ServerDesiredState;
  version?: string;
  config?: Record<string, unknown>;
  sourceWorldId?: string;
  sourceWorldName?: string;
  resources?: Partial<ResourceLimits>;
  network?: {
    port?: number;
    hostPort?: number;
    protocol?: string;
  };
  runtime?: {
    dataDir?: string;
    image?: string;
    env?: string[];
    cmd?: string[];
  };
};

export type ServerCondition = {
  type: string;
  status: "True" | "False" | "Unknown" | string;
  reason?: string;
  message?: string;
  observedGeneration?: number;
  lastTransitionAt: string;
};

export type ServerRuntimeStatus = {
  phase: ServerPhase;
  actualState: ServerActualState;
  runtimeId?: string;
  playersOnline?: number;
  observedGeneration: number;
  appliedGeneration: number;
  conditions?: ServerCondition[];
  lastError?: string;
  lastReconcileAt?: string;
  lastTransitionAt?: string;
};

export type GameServerResource = {
  id: string;
  name: string;
  gameKey: GameKey;
  providerKey: ProviderKey;
  spec: ServerResourceSpec;
  status: ServerRuntimeStatus;
  createdAt: string;
  updatedAt: string;
};

export type ServerJoinInfo = {
  address: string;
  port: number;
  password?: string;
  inviteText: string;
  instructions?: string[];
};

export type ServerShare = {
  enabled: boolean;
  token?: string;
  sharePath?: string;
  includePassword: boolean;
  createdAt?: string;
  updatedAt?: string;
};

export type ConfigPreset = {
  id: string;
  name: string;
  gameKey: GameKey;
  providerKey: ProviderKey;
  version?: string;
  config: Record<string, unknown>;
  configPayload?: Record<string, unknown>;
  cpuLimitCores: number;
  memoryLimitMb: number;
  modPackId?: string;
  createdAt: string;
  updatedAt: string;
};

export type PublicServerShare = {
  name: string;
  gameKey: GameKey;
  providerKey: ProviderKey;
  status: ServerStatus;
  players: number;
  maxPlayers: number;
  joinInfo: ServerJoinInfo;
};

export type World = {
  id: string;
  instanceId?: string;
  activeInstanceId?: string;
  gameKey?: GameKey;
  providerKey?: ProviderKey;
  name: string;
  size: string;
  difficulty: string;
  server?: string;
  modified: string;
  bytes: string;
  source?: string;
  config?: Record<string, unknown>;
};

export type Backup = {
  id: string;
  name: string;
  instanceId?: string;
  gameKey?: GameKey;
  providerKey?: ProviderKey;
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
  gameKey?: GameKey;
  providerKey?: ProviderKey;
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
  gameKey?: GameKey;
  providerKey?: ProviderKey;
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
  gameKey?: GameKey;
  providerKey?: ProviderKey;
  modIds: string[];
  mods: ModFile[];
  created: string;
};

export type ActivityEvent = {
  id: string;
  instanceId?: string;
  type: string;
  message: string;
  payload?: Record<string, unknown>;
  created: string;
};
