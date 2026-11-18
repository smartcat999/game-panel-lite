export interface ObjectMeta {
  name: string;
  tenant?: string;
  uid?: string;
  generation?: number;
  resourceVersion?: string;
  statusVersion?: string;
  creationTimestamp?: string;
  deletionTimestamp?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
}

export interface KubeList<T> {
  items?: T[];
}

export interface TenantQuota {
  cpu?: string;
  memory?: string;
  storage?: string;
  instances?: string;
  backups?: string;
  backupStorage?: string;
}

export interface TenantAlertRuleSpec {
  id: string;
  title: string;
  description: string;
  metric: string;
  threshold: number;
  forSeconds?: number;
  severity: "warning" | "critical";
  scope: string;
  enabled: boolean;
}

export interface TenantAlertChannelSpec {
  id: string;
  name: string;
  type: "feishu" | "dingtalk" | "wecom" | "slack" | "shoutrrr" | "webhook";
  url: string;
  secret?: string;
  consoleURL?: string;
  severity?: "all" | "critical";
  enabled: boolean;
  state?: "healthy" | "retrying" | "failed";
  retryCount?: number;
  lastDeliveryAt?: string;
  lastError?: string;
  nextRetryAt?: string;
}

export interface TenantAlertSilenceSpec {
  id: string;
  scope?: string;
  targetKind: "instances" | "nodes" | "*";
  targetName: string;
  ruleID?: string;
  reason?: string;
  createdBy?: string;
  startsAt?: string;
  expiresAt: string;
}

export type TenantBillingMode = "Manual" | "Subscription";

export interface TenantSubscriptionStatus {
  planName?: string;
  planDisplayName?: string;
  state?: "Trialing" | "Active" | "Grace" | "Suspended" | "None" | string;
  expiresAt?: string;
  graceUntil?: string;
  baseQuota?: TenantQuota;
  addonQuota?: TenantQuota;
  effectiveQuota?: TenantQuota;
  activeOrderIDs?: string[];
}

export interface Tenant {
  metadata: ObjectMeta;
  spec: {
    displayName?: string;
    type?: "Personal" | "Organization";
    billingMode?: TenantBillingMode;
    owner?: string;
    quota?: TenantQuota;
    unlimitedQuota?: string[];
    alertRules?: TenantAlertRuleSpec[];
    alertChannels?: TenantAlertChannelSpec[];
    platformAlertChannels?: TenantAlertChannelSpec[];
    alertSilences?: TenantAlertSilenceSpec[];
    platformAlertSilences?: TenantAlertSilenceSpec[];
  };
  status?: {
    observedGeneration?: number;
    phase?: string;
    usage?: TenantQuota;
    reserved?: TenantQuota;
    subscription?: TenantSubscriptionStatus;
  };
}

export interface ResourcePlanPrice {
  currency: string;
  amountCents: number;
  durationDays: number;
}

export interface ResourcePlanItem {
  metadata: ObjectMeta;
  displayName: string;
  description?: string;
  planKind: "Base" | "Addon" | string;
  quota: TenantQuota;
  sortOrder?: number;
  published: boolean;
  prices?: ResourcePlanPrice[];
}

export interface QuotaOrderItem {
  metadata: ObjectMeta;
  id: string;
  tenantUID: string;
  tenantName?: string;
  orderKind: "BasePlan" | "AddonPack" | string;
  planName: string;
  source: "Trial" | "Redeem" | "AdminGrant" | "Checkout" | string;
  sourceRef?: string;
  reason?: string;
  quotaDelta: TenantQuota;
  durationDays: number;
  amountCents: number;
  currency?: string;
  state: "Pending" | "Paid" | "Cancelled" | string;
  paidAt?: string;
  expiresAt?: string;
  appliedAt?: string;
  policyVersion?: number;
  createdBy?: string;
  createdAt?: string;
}

export interface RedeemCodeItem {
  metadata: ObjectMeta;
  id: string;
  codeHint: string;
  batchName?: string;
  orderKind: string;
  planName: string;
  durationDays: number;
  maxRedemptions: number;
  redeemedCount: number;
  redeemedByTenantUID?: string;
  redeemedOrderID?: string;
  redeemedAt?: string;
  disabled: boolean;
  expiresAt?: string;
  createdBy?: string;
  createdAt?: string;
}

export interface RedeemCodeBatchResponse {
  batchName?: string;
  orderKind: string;
  planName: string;
  durationDays: number;
  count: number;
  codes: Array<{ id: string; code: string; codeHint: string }>;
}

export interface QuotaPolicySnapshot {
  quota?: TenantQuota;
  unlimitedQuota?: string[];
  expiresAt?: string;
}

export interface QuotaPolicyChangeItem {
  metadata: ObjectMeta;
  changeKind: "Bootstrap" | "Subscription" | "AdminAdjust" | "Expiry" | string;
  fromVersion: number;
  toVersion: number;
  tenantResourceVersion: number;
  orderIDs?: string[];
  old: QuotaPolicySnapshot;
  target: QuotaPolicySnapshot;
}

export interface ReclaimTenantResponse {
  tenant: string;
  reclaimedCount: number;
  reclaimedInstances?: string[];
}

export interface RuntimeClass {
  metadata: ObjectMeta;
  handler: string;
  isDefault?: boolean;
  parameters?: Record<string, string>;
  scheduling?: {
    nodeSelector?: Record<string, string>;
  };
}

export interface NodeAddress {
  type: string;
  address: string;
}

export interface NodeItem {
  metadata: ObjectMeta;
  spec?: {
    unschedulable?: boolean;
    externalIP?: string;
    allowedApps?: string[];
  };
  status?: {
    observedGeneration?: number;
    phase?: string;
    addresses?: NodeAddress[];
    capacity?: Record<string, string>;
    allocatable?: Record<string, string>;
    reserved?: Record<string, string>;
    usage?: {
      cpuMillicores?: number;
      memoryBytes?: number;
      storageBytes?: number;
      networkRxBytesPerSec?: number;
      networkTxBytesPerSec?: number;
    };
    cachedArtifacts?: string[];
    cachedBlobs?: string[];
    warmImages?: string[];
    lastHeartbeatTime?: string;
    runtimes?: Array<{ name: string; handler?: string; ready?: boolean; version?: string }>;
    conditions?: Array<{
      type: string;
      status: string;
      reason?: string;
      message?: string;
      lastTransitionTime?: string;
    }>;
  };
}

export interface NodeMetricSample {
  timestamp: string;
  cpuMillicores: number;
  memoryBytes: number;
  storageBytes: number;
  networkRxBytesPerSec?: number;
  networkTxBytesPerSec?: number;
}

export interface InstancePort {
  name?: string;
  protocol?: string;
  targetPort: number;
  publishedPort?: number;
}

export interface InstanceFile {
  path: string;
  content: string;
}

export interface InstanceSchedule {
  id: string;
  name: string;
  action: "command" | "restart" | "backup";
  command?: string;
  intervalMinutes?: number;
  dailyAt?: string;
  retainCount?: number;
  enabled: boolean;
  manualTriggerAt?: string;
}

export interface InstanceScheduleStatus {
  id: string;
  runID?: string;
  claimedAt?: string;
  lastRunAt?: string;
  lastResult?: "Running" | "Unknown" | "Succeeded" | "Failed" | string;
  lastOutput?: string;
  stage?: string;
  progress?: number;
  startedAt?: string;
  finishedAt?: string;
}

export interface ArtifactRef {
  repository: string;
  package: string;
  version: string;
  targetPath?: string;
}

export interface InstanceAddon {
  repository: string;
  package: string;
  version: string;
  title?: string;
  installMode?: "flat_dir" | "tmod_latest" | "dst_local_mods" | "workshop_tree" | string;
  targetDir?: string;
  disabled?: boolean;
  config?: Record<string, string>;
}

export interface InstanceBackupAddon {
  repository?: string;
  package: string;
  version?: string;
  title?: string;
  installMode?: string;
  targetDir?: string;
  archivePath?: string;
  disabled?: boolean;
  configCount?: number;
  scopes?: string[];
}

export interface InstanceBackupSnapshot {
  capturedAt?: string;
  appName?: string;
  serverVersion?: string;
  configCount?: number;
  addonCount?: number;
  addons?: InstanceBackupAddon[];
}

export interface InstanceBackupInspectionSummary {
  schemaVersion?: number;
  phase?: string;
  archiveFormat?: string;
  fileCount?: number;
  uncompressedBytes?: number;
  recommendedSource?: string;
  extractedAddons?: InstanceBackupAddon[];
}

export interface InstanceBackupRecord {
  uid?: string;
  operationID?: string;
  allocationID?: string;
  imported?: boolean;
  phase?:
    | "PendingQuota"
    | "Ready"
    | "Uploading"
    | "Unknown"
    | "Inspecting"
    | "InspectFailed"
    | "Completed"
    | "Restoring"
    | "Deleting"
    | string;
  reservedBytes?: number;
  path: string;
  sizeBytes?: number;
  sha256?: string;
  completedAt?: string;
  taskID?: string;
  stage?: string;
  progress?: number;
  processedBytes?: number;
  totalBytes?: number;
  speedBytesPerSec?: number;
  startedAt?: string;
  message?: string;
  lastRestoreTaskID?: string;
  lastRestoreResult?: "Running" | "Succeeded" | "Failed" | string;
  lastRestoreMessage?: string;
  lastRestoreAt?: string;
  restoreSourcePath?: string;
  restoreTargetPath?: string;
  restoreReplace?: boolean;
  snapshot?: InstanceBackupSnapshot;
  inspection?: InstanceBackupInspectionSummary;
}

export interface InstanceItem {
  metadata: ObjectMeta;
  spec: {
    runtimeClassName?: string;
    nodeName?: string;
    state?: "Running" | "Stopped";
    image?: string;
    command?: string[];
    args?: string[];
    workingDir?: string;
    serverArtifact?: ArtifactRef;
    addons?: InstanceAddon[];
    /** apiserver models env as a plain map (InstanceSpec.Env map[string]string). */
    env?: Record<string, string>;
    files?: InstanceFile[];
    ports?: InstancePort[];
    resources?: {
      requests?: Record<string, string>;
      limits?: Record<string, string>;
    };
    backupPolicy?: {
      enabled?: boolean;
      intervalHours?: number;
      retentionCount?: number;
      subtree?: string;
      anchorFiles?: string[];
      replaceOnRestore?: boolean;
      backupRequiresStop?: boolean;
      preBackupCommand?: string;
      postBackupCommand?: string;
      consoleDelivery?: string;
    };
    schedules?: InstanceSchedule[];
    metrics?: AppMetricSpec[];
  };
  status?: {
    observedGeneration?: number;
    phase?: string;
    runtimeID?: string;
    startedAt?: string;
    message?: string;
    endpoint?: {
      hostIP?: string;
      ports?: InstancePort[];
    };
    usage?: {
      cpuMillicores?: number;
      memoryBytes?: number;
      storageBytes?: number;
      networkRxBytesPerSec?: number;
      networkTxBytesPerSec?: number;
      customMetrics?: InstanceCustomMetric[];
    };
    backups?: InstanceBackupRecord[];
    scheduleStatuses?: InstanceScheduleStatus[];
    resourceControl?: {
      suspended?: boolean;
    };
    conditions?: Array<{
      type: string;
      status: string;
      reason?: string;
      message?: string;
      lastTransitionTime?: string;
    }>;
  };
}

export interface UserItem {
  metadata: ObjectMeta;
  spec: {
    email?: string;
    displayName?: string;
    state?: string;
  };
}

export interface UserMessageItem {
  metadata: ObjectMeta;
  category: "system" | "alert" | "member" | "backup" | "quota" | string;
  title: string;
  content: string;
  workspace?: string;
  actionUrl?: string;
  actionLabel?: string;
  read: boolean;
  timestamp: string;
}

export interface PolicyRule {
  apiGroups?: string[];
  resources?: string[];
  verbs?: string[];
  resourceNames?: string[];
}

export interface GlobalRoleItem {
  metadata: ObjectMeta;
  description?: string;
  rules?: PolicyRule[];
}

export interface GlobalRoleBindingItem {
  metadata: ObjectMeta;
  roleRef: { name: string };
  subjects?: Array<{ kind: string; name: string }>;
}

export interface FileEntry {
  name: string;
  path?: string;
  isDir?: boolean;
  size?: number;
  modTime?: string;
}

export interface AuditEventItem {
  metadata: ObjectMeta;
  actor: string;
  verb: string;
  apiGroup?: string;
  resource: string;
  subresource?: string;
  resourceName?: string;
  action?: string;
  targetName?: string;
  attributes?: Record<string, string>;
  statusCode: number;
  clientIP?: string;
  requestID?: string;
  timestamp: string;
}

export interface AlertItem {
  metadata: ObjectMeta;
  title: string;
  summary: string;
  severity: "critical" | "warning";
  state: "firing" | "resolved";
  scope: "platform" | "tenant";
  resource: string;
  targetName: string;
  value?: string;
  condition?: string;
  startsAt: string;
  silenced?: boolean;
  silencedUntil?: string;
  silenceID?: string;
}

export interface PlatformLogEntry {
  metadata: ObjectMeta;
  timestamp: string;
  level: string;
  service: string;
  message: string;
  fields?: Record<string, string>;
}

export interface AppConfigOption {
  value: string;
  label: string;
  labelEn?: string;
}

export interface AppConfigField {
  name: string;
  label: string;
  labelEn?: string;
  type: "text" | "password" | "number" | "select" | "boolean";
  group?: string;
  required?: boolean;
  default?: string;
  min?: number;
  max?: number;
  step?: number;
  options?: AppConfigOption[];
  optionsSource?: "addon_maps" | string;
  help?: string;
  helpEn?: string;
  envKey?: string;
  fileTarget?: string;
}

export interface AppConfigPreset {
  id: string;
  name: string;
  description?: string;
  values: Record<string, string>;
}

export interface AppFileTemplate {
  path: string;
  content: string;
}

export interface AppConsoleParam {
  name: string;
  label: string;
  type?: "text" | "number" | "select";
  defaultValue?: string;
  options?: AppConfigOption[];
  optionsSource?: "addon_maps" | string;
}

export interface AppConsoleAction {
  id: string;
  title: string;
  group?: string;
  command: string;
  starred?: boolean;
  danger?: boolean;
  params?: AppConsoleParam[];
}

export interface AppConsoleTarget {
  id: string;
  label: string;
  format?: string;
  enabledField?: string;
}

export interface AppStoragePolicy {
  backupSubtree?: string;
  replaceOnRestore?: boolean;
  backupRequiresStop?: boolean;
  preBackupCommand?: string;
  postBackupCommand?: string;
  regeneratePaths?: string[];
}

export interface AppMetricItemAction {
  id: string;
  title: string;
  command: string;
  danger?: boolean;
}

export interface AppMetricSpec {
  id: string;
  label: string;
  unit?: string;
  mode: "regex_value" | "regex_count" | "log_delta" | string;
  source?: "stdin_log" | "exec" | "file" | string;
  filePath?: string;
  probeCommand?: string;
  intervalSec?: number;
  pattern?: string;
  incPattern?: string;
  decPattern?: string;
  resetPattern?: string;
  maxConfigKey?: string;
  maxValue?: string;
  itemPattern?: string;
  bindParam?: string;
  quickActionIds?: string[];
  itemActions?: AppMetricItemAction[];
}

export interface InstanceCustomMetric {
  id: string;
  label: string;
  unit?: string;
  value: number;
  display: string;
  max?: string;
  items?: string[];
  bindParam?: string;
  quickActionIds?: string[];
  itemActions?: AppMetricItemAction[];
}

export interface AppServerBinding {
  repository: string;
  package: string;
  targetPath?: string;
}

export interface AppAddonBinding {
  repository: string;
  installMode: "flat_dir" | "tmod_latest" | "dst_local_mods" | "workshop_tree" | string;
  targetDir: string;
}

export interface AppItem {
  metadata: ObjectMeta;
  displayName: string;
  tagline?: string;
  summary?: string;
  category?: string;
  coverImage?: string;
  image: string;
  imageTemplate?: string;
  versions?: string[];
  recommendedVersion?: string;
  serverBinding?: AppServerBinding;
  addonBinding?: AppAddonBinding;
  runtimeClassName?: string;
  preStartCommand?: string;
  startupCommand?: string;
  preStopCommand?: string;
  command?: string[];
  workingDir?: string;
  ports?: InstancePort[];
  env?: Record<string, string>;
  recommendedPlan?: string;
  resources?: Record<string, string>;
  configSchema?: AppConfigField[];
  configPresets?: AppConfigPreset[];
  fileTemplates?: AppFileTemplate[];
  consoleDelivery?: string;
  consoleTargets?: AppConsoleTarget[];
  consoleActions?: AppConsoleAction[];
  storagePolicy?: AppStoragePolicy;
  metrics?: AppMetricSpec[];
  capabilities?: string[];
  published: boolean;
  builtIn?: boolean;
}

export type PrewarmJobPhase = "Pending" | "Running" | "Succeeded" | "Failed" | string;

export interface PrewarmNodeStatus {
  nodeName: string;
  phase: PrewarmJobPhase;
  message?: string;
  completedAt?: string;
}

export interface PrewarmJobItem {
  metadata: ObjectMeta;
  spec: {
    nodeNames?: string[];
    nodeSelector?: Record<string, string>;
    artifacts?: ArtifactRef[];
    images?: string[];
  };
  status?: {
    observedGeneration?: number;
    phase?: PrewarmJobPhase;
    targetNodes?: number;
    readyNodes?: number;
    failedNodes?: number;
    nodes?: PrewarmNodeStatus[];
    conditions?: Array<{
      type: string;
      status: string;
      reason?: string;
      message?: string;
      lastTransitionTime?: string;
    }>;
  };
}

// -------------------------------------------- registry.gamepanel.io/v1 types

export interface RegistryUpstream {
  metadata: ObjectMeta;
  provider: "papermc" | "purpur" | "vanilla" | "modrinth" | "steam" | "http" | string;
  endpoint?: string;
  proxyURL?: string;
  steamAppId?: number;
  modrinthLoader?: string;
  modrinthProjectType?: string;
  apiKey?: string;
  rateLimitRps?: number;
}

export interface RegistryRepositoryRetentionPolicy {
  maxVersionsPerPackage?: number;
  maxAgeDays?: number;
}

export interface RegistryRepository {
  metadata: ObjectMeta;
  spec: {
    displayName?: string;
    description?: string;
    mode: "Hosted" | "Proxy" | "Virtual" | string;
    format: "server_package" | "workshop_mod" | "modrinth_addon" | "generic" | string;
    upstreamName?: string;
    memberRepos?: string[];
    publicRead?: boolean;
    retentionPolicy?: RegistryRepositoryRetentionPolicy;
  };
  status?: {
    observedGeneration?: number;
    phase?: string;
    packageCount?: number;
    artifactCount?: number;
    storageBytes?: number;
    lastSyncedAt?: string;
  };
}

export interface RegistryPackageDependency {
  package: string;
  title?: string;
}

export interface AddonConfigOption {
  label: string;
  value: string;
  valueType?: "string" | "number" | "boolean";
  description?: string;
}

export interface AddonConfigField {
  name: string;
  label: string;
  description?: string;
  type: "boolean" | "number" | "select" | "text";
  default?: string;
  defaultType?: "string" | "number" | "boolean";
  min?: number;
  max?: number;
  step?: number;
  options?: AddonConfigOption[];
}

export interface RegistryPackage {
  metadata: ObjectMeta;
  repository: string;
  package: string;
  title: string;
  summary?: string;
  description?: string;
  author?: string;
  iconUrl?: string;
  galleryUrls?: string[];
  sourceUrl?: string;
  downloads?: number;
  sizeBytes?: number;
  latestVersion?: string;
  availableVersions?: string[];
  updatedAt?: string;
  tags?: string[];
  modIds?: string[];
  mapFolders?: string[];
  dependencies?: RegistryPackageDependency[];
  configSchema?: AddonConfigField[];
}

export interface RegistryBlobDescriptor {
  digest: string;
  sizeBytes: number;
  mediaType?: string;
  archiveFormat?: "raw" | "tar.gz" | "zip" | "jar" | "tmod" | string;
  fileName?: string;
}

export interface RegistryArtifact {
  metadata: ObjectMeta;
  spec: {
    repository: string;
    package: string;
    version: string;
    sourceURL?: string;
    archiveFormat?: string;
    targetPath?: string;
    gameVersions?: string[];
    loaders?: string[];
    descriptor?: RegistryBlobDescriptor;
  };
  status?: {
    observedGeneration?: number;
    phase?: "Pending" | "Syncing" | "Ready" | "Failed" | "Deleting" | string;
    stage?:
      | "Queued"
      | "Resolving"
      | "Downloading"
      | "Archiving"
      | "Uploading"
      | "Completed"
      | "Failed"
      | string;
    progress?: number;
    processedBytes?: number;
    totalBytes?: number;
    speedBytesPerSec?: number;
    startedAt?: string;
    build?: string;
    descriptor?: RegistryBlobDescriptor;
    modIds?: string[];
    mapFolders?: string[];
    dependencies?: RegistryPackageDependency[];
    configSchema?: AddonConfigField[];
    publishedAt?: string;
    changelog?: string;
    upstreamURL?: string;
    syncedAt?: string;
    message?: string;
  };
}

export interface RegistryBlob {
  metadata: ObjectMeta;
  digest: string;
  sizeBytes: number;
  mediaType?: string;
  objectKey: string;
  refCount: number;
}

export interface RegistryBlobUploadPart {
  partNumber: number;
  url?: string;
  etag?: string;
}

export interface RegistryBlobUpload {
  metadata: ObjectMeta;
  digest?: string;
  sizeBytes: number;
  mediaType?: string;
  fileName?: string;
  objectKey?: string;
  uploadID?: string;
  partSize?: number;
  parts?: RegistryBlobUploadPart[];
  presignedURL?: string;
  expiresAt?: string;
  uploadState?: string;
}

export interface RegistryReplicationPolicy {
  metadata: ObjectMeta;
  repository: string;
  enabled: boolean;
  mode: "TrackReleases" | "TopPopular" | "ExplicitPackages" | string;
  intervalMinutes?: number;
  topN?: number;
  packages?: string[];
  gameVersion?: string;
  lastRunAt?: string;
  lastMessage?: string;
}

export interface InstanceMeteringItem {
  name: string;
  game?: string;
  runtimeClassName?: string;
  nodeName?: string;
  phase: string;
  cpu: string;
  memory: string;
  storage: string;
  runningHours: number;
  cpuCoreHours: number;
  memoryGiBHours: number;
  storageGiBHours: number;
  backupCount: number;
  backupBytes: number;
  startedAt?: string;
}

export interface MeteringCloudEventRecord {
  id: string;
  type: string;
  subject: string;
  timestamp: string;
  phase?: string;
  cpu?: string;
  memory?: string;
  storage?: string;
}

export interface TenantMeteringItem {
  metadata: ObjectMeta;
  displayName: string;
  type?: "Personal" | "Organization";
  owner?: string;
  window: string;
  phase?: string;
  activeInstances: number;
  totalInstances: number;
  allocatedCPU: string;
  allocatedMemory: string;
  allocatedStorage: string;
  quotaCPU: string;
  quotaMemory: string;
  quotaStorage: string;
  cpuCoreHours: number;
  memoryGiBHours: number;
  storageGiBHours: number;
  backupCount: number;
  backupStorageBytes: number;
  cloudEventsCount: number;
  instances?: InstanceMeteringItem[];
  recentCloudEvents?: MeteringCloudEventRecord[];
}


