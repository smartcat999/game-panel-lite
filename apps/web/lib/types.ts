import type { TerrariaConfig } from "@gamepanel-lite/shared";

export type ServerStatus = "running" | "stopped" | "errored";
export type ServerMode = "vanilla" | "tmodloader";

export type Server = {
  id: string;
  name: string;
  mode: ServerMode;
  status: ServerStatus;
  world: string;
  players: number;
  maxPlayers: number;
  port: number;
  version: string;
  lastBackup: string;
  password: string;
  cpu: string;
  memory: string;
  config: TerrariaConfig;
};

export type World = {
  id: string;
  name: string;
  size: string;
  difficulty: string;
  server?: string;
  modified: string;
  bytes: string;
};

export type Backup = {
  id: string;
  name: string;
  instanceId?: string;
  server: string;
  world: string;
  type: "Auto" | "Manual";
  size: string;
  created: string;
};

export type ModFile = {
  id: string;
  instanceId: string;
  fileName: string;
  size: string;
  enabled: boolean;
  created: string;
};

export type ActivityEvent = {
  id: string;
  instanceId?: string;
  type: string;
  message: string;
  created: string;
};
