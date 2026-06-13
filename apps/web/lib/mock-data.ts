import type { Backup, Server, World } from "./types";

export const servers: Server[] = [
  {
    id: "journey-friends",
    name: "Journey Friends",
    mode: "tmodloader",
    status: "running",
    world: "Moon Garden",
    players: 3,
    maxPlayers: 8,
    port: 7777,
    version: "1.4.4.9",
    lastBackup: "12 min ago",
    password: "cat123",
    cpu: "8%",
    memory: "512 MB / 2 GB"
  },
  {
    id: "classic-world",
    name: "Classic World",
    mode: "vanilla",
    status: "running",
    world: "Classic",
    players: 2,
    maxPlayers: 8,
    port: 7778,
    version: "1.4.4.9",
    lastBackup: "1 h ago",
    password: "",
    cpu: "5%",
    memory: "420 MB / 2 GB"
  },
  {
    id: "building-server",
    name: "Building Server",
    mode: "vanilla",
    status: "stopped",
    world: "Builder's Heaven",
    players: 0,
    maxPlayers: 8,
    port: 7779,
    version: "1.4.4.9",
    lastBackup: "3 h ago",
    password: "",
    cpu: "0%",
    memory: "0 MB"
  },
  {
    id: "modded-adventure",
    name: "Modded Adventure",
    mode: "tmodloader",
    status: "stopped",
    world: "Adventure",
    players: 0,
    maxPlayers: 10,
    port: 7780,
    version: "1.4.4.9",
    lastBackup: "1 d ago",
    password: "mods",
    cpu: "0%",
    memory: "0 MB"
  }
];

export const worlds: World[] = [
  { id: "moon", name: "Moon Garden", size: "Medium World", difficulty: "Expert", server: "Journey Friends", modified: "18 min ago", bytes: "84.7 MB" },
  { id: "classic", name: "Classic", size: "Medium World", difficulty: "Classic", server: "Classic World", modified: "1 h ago", bytes: "67.2 MB" },
  { id: "builder", name: "Builder's Heaven", size: "Large World", difficulty: "Classic", modified: "3 h ago", bytes: "128.4 MB" },
  { id: "adventure", name: "Adventure", size: "Medium World", difficulty: "Expert", server: "Modded Adventure", modified: "1 d ago", bytes: "91.3 MB" },
  { id: "new-world", name: "New World", size: "Medium World", difficulty: "Classic", modified: "2 d ago", bytes: "67.1 MB" }
];

export const backups: Backup[] = [
  { id: "b1", name: "Journey Friends - 2024-06-01 12:00", server: "Journey Friends", world: "Moon Garden", type: "Auto", size: "84.7 MB", created: "12 min ago" },
  { id: "b2", name: "Journey Friends - 2024-06-01 00:00", server: "Journey Friends", world: "Moon Garden", type: "Auto", size: "84.3 MB", created: "12 h ago" },
  { id: "b3", name: "Classic World - 2024-05-31 12:00", server: "Classic World", world: "Classic", type: "Auto", size: "67.2 MB", created: "1 d ago" },
  { id: "b4", name: "Modded Adventure - 2024-05-30 18:00", server: "Modded Adventure", world: "Adventure", type: "Manual", size: "91.3 MB", created: "2 d ago" }
];

export const activity = [
  "Backup completed for Journey Friends",
  "Player smartcat joined Journey Friends",
  "Server Classic World started",
  "Backup completed for Classic World"
];

export const logs = [
  "[12:01:22] [Info] Server started",
  "[12:01:23] [Info] tModLoader 1.4.4.9",
  "[12:01:25] [Info] Listening on port 7777",
  "[12:02:11] [Info] Player smartcat joined (3/8)",
  "[12:04:45] [Info] Player Luna joined (4/8)",
  "[12:05:01] [Info] Saving world data...",
  "[12:05:01] [Info] World saved",
  "[12:06:17] [Info] Player hero123 joined (5/8)",
  "[12:07:45] [Warn] High memory usage: 85%",
  "[12:08:22] [Info] Auto-save completed"
];
