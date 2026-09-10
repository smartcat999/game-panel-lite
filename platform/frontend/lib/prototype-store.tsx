"use client";

import { createContext, useContext, useMemo, useState } from "react";

export type InstanceState = "running" | "stopped" | "starting" | "failed";

export type PrototypeInstance = {
  id: string;
  name: string;
  state: InstanceState;
  provider: "Terraria" | "tModLoader";
  version: string;
  region: string;
  endpoint?: string;
  transports: Array<"TCP" | "UDP">;
  cpuMilli: number;
  memoryMiB: number;
  diskGiB: number;
  players?: { current: number; maximum: number };
  supportsMods: boolean;
  stale?: boolean;
};

export type PrototypeBackup = {
  id: string;
  instanceId: string;
  label: string;
  size: string;
  createdAt: string;
  state: "ready" | "restoring";
};

type NewInstance = Omit<PrototypeInstance, "id" | "state" | "endpoint" | "transports"> & {
  serverName: string;
  maxPlayers: number;
  password: string;
  modIds: string[];
};

type PrototypeContextValue = {
  balanceMinor: number;
  instances: PrototypeInstance[];
  backups: PrototypeBackup[];
  invited: boolean;
  acceptInvitation: () => void;
  grantCredit: (amountMinor: number) => void;
  exhaustWallet: () => void;
  createInstance: (input: NewInstance) => PrototypeInstance;
  setInstanceState: (id: string, state: InstanceState) => void;
  createBackup: (instanceId: string) => void;
  restoreBackup: (id: string) => void;
};

const initialInstances: PrototypeInstance[] = [
  { id: "lin_terraria01", name: "terraria-hardcore-01", state: "running", provider: "Terraria", version: "1.4.4.9", region: "华东 1", endpoint: "192.168.2.4:31777", transports: ["TCP"], cpuMilli: 2000, memoryMiB: 4096, diskGiB: 20, players: { current: 12, maximum: 16 }, supportsMods: false },
  { id: "lin_modded03", name: "calamity-infernum-03", state: "running", provider: "tModLoader", version: "2025.06", region: "华东 1", endpoint: "192.168.2.4:31779", transports: ["TCP"], cpuMilli: 3000, memoryMiB: 6144, diskGiB: 35, players: { current: 6, maximum: 12 }, supportsMods: true },
  { id: "lin_casual02", name: "terraria-casual-02", state: "stopped", provider: "Terraria", version: "1.4.4.9", region: "华北 1", endpoint: "192.168.2.4:31778", transports: ["TCP"], cpuMilli: 1000, memoryMiB: 2048, diskGiB: 15, supportsMods: false },
  { id: "lin_builder04", name: "builder-creative-04", state: "running", provider: "Terraria", version: "1.4.4.9", region: "华东 1", endpoint: "192.168.2.4:31780", transports: ["TCP"], cpuMilli: 1000, memoryMiB: 2048, diskGiB: 12, supportsMods: false, stale: true },
];

const PrototypeContext = createContext<PrototypeContextValue | null>(null);

export function PrototypeProvider({ children }: { children: React.ReactNode }) {
  const [balanceMinor, setBalanceMinor] = useState(12840);
  const [instances, setInstances] = useState(initialInstances);
  const [invited, setInvited] = useState(false);
  const [backups, setBackups] = useState<PrototypeBackup[]>([
    { id: "bkp_01", instanceId: "lin_terraria01", label: "更新前备份", size: "684 MB", createdAt: "今天 09:42", state: "ready" },
    { id: "bkp_02", instanceId: "lin_modded03", label: "自动备份", size: "1.2 GB", createdAt: "昨天 03:00", state: "ready" },
  ]);

  const value = useMemo<PrototypeContextValue>(() => ({
    balanceMinor,
    instances,
    backups,
    invited,
    acceptInvitation: () => setInvited(true),
    grantCredit: (amountMinor) => setBalanceMinor((current) => current + amountMinor),
    exhaustWallet: () => {
      setBalanceMinor(0);
      setInstances((current) => current.map((instance) => instance.state === "running" ? { ...instance, state: "stopped" } : instance));
    },
    createInstance: (input) => {
      const instance: PrototypeInstance = {
        id: `lin_${Date.now()}`,
        name: input.name,
        state: "starting",
        provider: input.provider,
        version: input.version,
        region: input.region,
        transports: ["TCP"],
        cpuMilli: input.cpuMilli,
        memoryMiB: input.memoryMiB,
        diskGiB: input.diskGiB,
        players: { current: 0, maximum: input.maxPlayers },
        supportsMods: input.supportsMods,
      };
      setInstances((current) => [instance, ...current]);
      return instance;
    },
    setInstanceState: (id, state) => setInstances((current) => current.map((instance, index) => instance.id === id ? {
      ...instance,
      state,
      endpoint: state === "running" && !instance.endpoint ? `192.168.2.4:${31800 + index}` : instance.endpoint,
    } : instance)),
    createBackup: (instanceId) => setBackups((current) => [{ id: `bkp_${Date.now()}`, instanceId, label: "手动备份", size: "处理中", createdAt: "刚刚", state: "ready" }, ...current]),
    restoreBackup: (id) => {
      setBackups((current) => current.map((backup) => backup.id === id ? { ...backup, state: "restoring" } : backup));
      window.setTimeout(() => setBackups((current) => current.map((backup) => backup.id === id ? { ...backup, state: "ready" } : backup)), 900);
    },
  }), [backups, balanceMinor, instances, invited]);

  return <PrototypeContext.Provider value={value}>{children}</PrototypeContext.Provider>;
}

export function usePrototype() {
  const value = useContext(PrototypeContext);
  if (!value) throw new Error("usePrototype must be used within PrototypeProvider");
  return value;
}
