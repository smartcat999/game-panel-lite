import type { ModFile } from "./types";

export function modDisplayName(mod: ModFile, locale: string) {
  if (mod.title?.trim()) {
    return mod.title.trim();
  }
  if (mod.source === "workshop" && mod.workshopId) {
    return `${locale === "zh" ? "创意工坊" : "Workshop"} ${mod.workshopId}`;
  }
  return mod.fileName;
}

export function modSourceLabel(mod: ModFile, locale: string) {
  if (mod.source === "workshop") {
    return locale === "zh" ? "创意工坊" : "Workshop";
  }
  if (mod.providerKey === "palworld") {
    return locale === "zh" ? "文件模组 .pak" : "File mod .pak";
  }
  return ".tmod";
}

export type DSTModScope = "client" | "server" | "required" | "unknown";

export function dstModScopeFromTags(providerKey: string | undefined, values: string[] | undefined): DSTModScope {
  if (providerKey !== "dont-starve-together") return "unknown";
  const tags = new Set((values ?? []).map((tag) => tag.toLowerCase()));
  if (tags.has("client_only_mod")) return "client";
  if (tags.has("server_only_mod")) return "server";
  if (tags.has("all_clients_require_mod")) return "required";
  return "unknown";
}

export function dstModScope(mod: ModFile): DSTModScope {
  return dstModScopeFromTags(mod.providerKey, mod.tags);
}

export type ModRuntimeState = "configured" | "disabled" | "enabled" | "notApplied" | "notSynced" | "pendingRestart";

export function modRuntimeState(mod: ModFile): ModRuntimeState | null {
  if (!mod.enabled) return "disabled";
  if (mod.runtimePresent === false) return "notSynced";
  if (mod.runtimeEnabled === false) return "notApplied";
  if (mod.runtimeEnabled === true) return "enabled";

  if (mod.providerKey === "dont-starve-together") {
    return dstModScope(mod) === "client" ? null : "configured";
  }

  return "pendingRestart";
}
