import type { MessageKey } from "./i18n";
import type { RuntimeImageStatus } from "./types";

const phases = {
  resolving: "runtimeInstallResolving",
  downloading: "runtimeInstallDownloading",
  extracting: "runtimeInstallExtracting",
  saving: "runtimeInstallSaving",
  finalizing: "runtimeInstallFinalizing",
  complete: "gameLibraryInstalled"
} satisfies Record<string, MessageKey>;

export function runtimeInstallProgress(status?: RuntimeImageStatus, now = Date.now()) {
  const phase = status?.phase ?? "resolving";
  const current = Math.max(0, status?.currentBytes ?? 0);
  const total = Math.max(0, status?.totalBytes ?? 0);
  const measurable = (phase === "downloading" || phase === "saving") && total > 0;
  const fresh = Boolean(status?.updatedAt && now - Date.parse(status.updatedAt) < 5000);
  return {
    phase,
    label: phases[phase] ?? phases.resolving,
    current,
    total,
    percent: measurable ? Math.min(100, Math.floor(current / total * 100)) : undefined,
    speed: phase === "downloading" && fresh ? Math.max(0, status?.bytesPerSecond ?? 0) : 0,
    completedLayers: Math.max(0, status?.completedLayers ?? 0)
  };
}

export function formatTransferBytes(bytes: number) {
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  const value = Math.max(0, bytes);
  const index = value > 0 ? Math.min(units.length - 1, Math.floor(Math.log(value) / Math.log(1024))) : 0;
  return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}
