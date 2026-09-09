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
  worldRegeneration?: boolean;
};

export type ProviderConfigField = {
  name: string;
  label: string;
  labelEn?: string;
  type: "text" | "password" | "number" | "select" | "boolean" | string;
  required: boolean;
  default?: unknown;
  help?: string;
  options?: Array<{ value: string; label: string }>;
  min?: number;
  max?: number;
  step?: number;
  group?: string;
};

export type RuntimeImageStatus = {
  image: string;
  status: "ready" | "missing" | "update_available" | "preparing" | "failed" | "unsupported" | string;
  message?: string;
  progress?: number;
  installedVersion?: string;
  targetVersion?: string;
  updatedAt?: string;
};

export type ProviderCatalog = {
  uploadExtensions?: string[];
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
  gameVersion?: ProviderVersionStatus;
};

export type ProviderVersionStatus = {
  supported: boolean;
  status: "unknown" | "checking" | "ready" | "failed" | "unsupported" | string;
  latestBuildId?: string;
  checkedAt?: string;
  autoCheckEnabled: boolean;
  autoCheckIntervalHours: number;
  job?: GameUpdateJob;
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

export type GameUpdateJobStatus = "queued" | "running" | "succeeded" | "failed";

export type GameUpdateJobStage =
  | "queued"
  | "preflight"
  | "backing_up"
  | "stopping"
  | "refreshing_metadata"
  | "validating"
  | "downloading"
  | "installing"
  | "starting"
  | "health_check"
  | "completed";

export type GameUpdateJob = {
  id: string;
  instanceId: string;
  providerKey: ProviderKey;
  operation: "check" | "apply";
  status: GameUpdateJobStatus;
  stage: GameUpdateJobStage;
  progress: number;
  installedBuildId?: string;
  latestBuildId?: string;
  error?: string;
  startAfterUpdate: boolean;
  wasRunning: boolean;
  createdAt: string;
  updatedAt: string;
  checkedAt?: string;
  completedAt?: string;
};

export type GameUpdateState = {
  supported: boolean;
  status: "unknown" | "checking" | "up_to_date" | "available" | "updating" | "failed";
  autoCheckEnabled: boolean;
  autoCheckIntervalHours: number;
  installedBuildId?: string;
  latestBuildId?: string;
  checkedAt?: string;
  job?: GameUpdateJob;
};

export type WorldRegenerationJobStatus = "queued" | "running" | "succeeded" | "failed";

export type WorldRegenerationJobStage =
  | "queued"
  | "stopping"
  | "backing_up"
  | "resetting"
  | "starting"
  | "health_check"
  | "rolling_back"
  | "completed";

export type WorldRegenerationJob = {
  id: string;
  instanceId: string;
  providerKey: ProviderKey;
  status: WorldRegenerationJobStatus;
  stage: WorldRegenerationJobStage;
  progress: number;
  backupId?: string;
  startAfter: boolean;
  wasRunning: boolean;
  error?: string;
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
};

export type WorldRegenerationState = {
  supported: boolean;
  job?: WorldRegenerationJob;
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
  modIds?: string[];
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
  organizationId?: string;
  id: string;
  name: string;
  nodeId?: string;
  region?: string;
  subscription?: {
    status?: string;
    expiresAtMs?: number;
    planId?: string;
  };
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
  organizationId?: string;
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
  modIds: string[];
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
  type: "Auto" | "Manual" | "Pre-update" | "Pre-regeneration";
  size: string;
  sizeBytes: number;
  created: string;
  createdAt: string;
};

export type ModFile = {
  id: string;
  organizationId?: string;
  contentHash?: string;
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

export type ModConfigFile = {
  name: string;
  sizeBytes: number;
  updatedAt: string;
  content?: string;
};

export type RecommendedMod = {
  rank: number;
  source?: string;
  externalId?: string;
  workshopId?: string;
  fileName?: string;
  sourceUrl?: string;
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

export type WorkshopPreviewItem = {
  workshopId: string;
  title: string;
  creatorSteamId?: string;
  previewUrl?: string;
  description?: string;
  fileSize: number;
  size: string;
  subscriptions?: number;
  favorited?: number;
  views?: number;
  timeCreated?: number;
  timeUpdated?: number;
  tags?: string[];
  status: "new" | "in_library" | "in_server" | "unavailable";
  selectable: boolean;
};

export type WorkshopPreview = {
  previewId: string;
  collectionId: string;
  collectionName?: string;
  providerKey: ProviderKey;
  expiresAt: string;
  items: WorkshopPreviewItem[];
  summary: {
    total: number;
    new: number;
    inLibrary: number;
    inServer: number;
    unavailable: number;
  };
};

export type ModPack = {
  organizationId?: string;
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

export type UserRole = "admin" | "member" | "viewer";
export type PlatformRole = "platform_admin" | "user";

export type Permission =
  | "server.view"
  | "server.create"
  | "server.control"
  | "server.configure"
  | "server.delete"
  | "backup.manage"
  | "world.manage"
  | "mod.manage"
  | "player.manage"
  | "share.manage"
  | "node.manage"
  | "team.manage"
  | "settings.manage"
  | "system.manage";

export type UserAccount = {
	id: string;
	username: string;
	role: UserRole;
	platformRole: PlatformRole;
  permissions?: Permission[];
  preferences?: AccountPreferences;
  createdAt?: string;
};

export type ThemeMode = "light" | "dark" | "system";

export type AccountPreferences = {
  locale: "zh" | "en";
  theme: ThemeMode;
};

export type AuthBootstrap = {
  initialized: boolean;
  allowRegistration: boolean;
  account?: UserAccount;
};

export type ComputeNode = {
  id: string;
  name: string;
  host: string;
  port: number;
  token?: string;
  publicIp?: string;
  publicDomain?: string;
  region?: string;
  status: "online" | "offline" | "degraded";
  isLocal: boolean;
  cpuCores: number;
  cpuUsagePercent?: number;
  memoryTotalMb: number;
  memoryUsedMb: number;
  diskTotalGb?: number;
  diskUsedGb?: number;
  dockerVersion?: string;
  agentVersion?: string;
  osInfo?: string;
  pingLatencyMs?: number;
  runningCount: number;
  unschedulable?: boolean;
  lastHeartbeat?: string;
  createdAt?: string;
  updatedAt?: string;
};

export type OrganizationMember = {
  id: string;
  organizationId: string;
  userId: string;
  username?: string;
  role: "owner" | "admin" | "member" | "viewer";
  createdAt: string;
};

export type OrganizationInvitation = {
  id: string;
  organizationId: string;
  inviterUserId: string;
  token: string;
  role: "admin" | "member" | "viewer";
  maxUses: number;
  usedCount: number;
  expiresAt: string;
  revoked: boolean;
  createdAt: string;
};

export type InvitationSummary = {
  token: string;
  organizationId: string;
  organizationName: string;
  inviterName: string;
  role: "admin" | "member" | "viewer";
  expiresAt: string;
  isExpired: boolean;
};

export type NodeJoinCommand = {
  nodeId: string;
  token: string;
  masterUrl: string;
  dockerCommand: string;
  shellCommand: string;
};

export type DrainMigrationResult = {
  serverId: string;
  serverName: string;
  targetNodeId?: string;
  success: boolean;
  error?: string;
};

export type DrainNodeResponse = {
  nodeId: string;
  totalServers: number;
  migratedCount: number;
  failedCount: number;
  details: DrainMigrationResult[];
};

export type CreditTransaction = {
  id: string;
  organizationId: string;
  amount: number;
  balanceAfter: number;
  type: string;
  description: string;
  createdBy?: string;
  createdAt: string;
};

export type UserCreditsResponse = {
  organizationId: string;
  organizationName: string;
  credits: number;
  transactions: CreditTransaction[];
};

export type OAuthProviderStatus = {
  github: boolean;
  google: boolean;
};

export type RegionDirectoryEntry = {
  id: string;
  name: string;
  acceptingCreates: boolean;
  version: number;
};

export type RegionStatusSnapshot = {
  schemaVersion: number;
  eventId: string;
  regionId: string;
  sequence: number;
  observedAtMs: number;
  nodes: { total: number; online: number; schedulable: number };
  capacity: { cpuTotal: number; cpuReserved: number; memoryTotalMb: number; memoryReservedMb: number };
  deployments: { total: number; pending: number; reserved: number; rejected: number };
  tasks: { awaitingAuthority: number };
};

export type CommercePlanVersion = {
  planId: string;
  version: number;
  providerKey: string;
  regionId: string;
  cpu: number;
  memoryMb: number;
  storageBytes: number;
  currency: string;
  unitAmountMinor: number;
  periodSeconds: number;
};

export type CommerceOrder = {
  id: string;
  organizationId: string;
  serverId: string;
  revisionId: string;
  placementEpoch: number;
  status: "pending" | "paid" | "cancelled" | "expired";
  quote: {
    plan: CommercePlanVersion;
    periods: number;
    amountMinor: number;
    durationMs?: number;
  };
  createdAtMs: number;
  expiresAtMs: number;
};

export type CommerceSubscription = {
  id: string;
  organizationId: string;
  serverId: string;
  orderId: string;
  paymentId: string;
  status: "pending_activation" | "active" | "expired" | "cancelled";
  quote: {
    plan: CommercePlanVersion;
    periods: number;
    amountMinor: number;
    durationMs?: number;
  };
  createdAtMs: number;
};

export type ServerOperation = {
  id: string;
  organizationId: string;
  serverId: string;
  kind: string;
  status: "pending" | "succeeded" | "failed";
  createdAt: string;
};
