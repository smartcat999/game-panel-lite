import type { TerrariaConfig } from "@gamepanel-lite/shared";

export type ServerStatus = "creating" | "starting" | "running" | "stopping" | "stopped" | "restarting" | "deleting" | "errored";
export type ServerMode = "vanilla" | "tmodloader";
export type ProviderKey = "terraria-vanilla" | "terraria-tmodloader";

export type Server = {
  id: string;
  name: string;
  mode: ServerMode;
  status: ServerStatus;
  world: string;
  players: number;
  maxPlayers: number;
  port: number;
  hostPort: number;
  version: string;
  lastError?: string;
  sourceWorldId?: string;
  sourceWorldName?: string;
  lastBackup: string;
  password: string;
  cpu: string;
  memory: string;
  config: TerrariaConfig;
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
  size: string;
  sizeBytes?: number;
  enabled: boolean;
  created: string;
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
