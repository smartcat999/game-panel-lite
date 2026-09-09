import type { ModFile } from "./types";

export function dstConfiguredWorkshopIds(config: Record<string, unknown> | undefined): string[] {
  const mods = config?.mods;
  if (!mods || typeof mods !== "object" || Array.isArray(mods)) return [];
  const workshopIds = (mods as Record<string, unknown>).workshopIds;
  if (!Array.isArray(workshopIds)) return [];

  return [...new Set(workshopIds.map(String).map((id) => id.trim()).filter(Boolean))];
}

export function mergeConfiguredWorkshopMods(
  installedMods: ModFile[],
  libraryMods: ModFile[],
  workshopIds: string[]
): ModFile[] {
  const merged = [...installedMods];
  const installedWorkshopIds = new Set(installedMods.map((mod) => mod.workshopId).filter(Boolean));
  const libraryByWorkshopId = new Map(
    libraryMods
      .filter((mod) => mod.workshopId)
      .map((mod) => [mod.workshopId as string, mod])
  );

  for (const workshopId of workshopIds) {
    if (installedWorkshopIds.has(workshopId)) continue;
    const libraryMod = libraryByWorkshopId.get(workshopId);
    if (libraryMod) merged.push({ ...libraryMod, enabled: true });
  }
  return merged;
}

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

export function isServerAssignableMod(mod: ModFile): boolean {
  if (mod.providerKey !== "dont-starve-together") return true;
  const scope = dstModScope(mod);
  return scope === "server" || scope === "required";
}

export type ModRuntimeState = "configured" | "disabled" | "enabled" | "notApplied" | "runtimeFileMissing" | "pendingRestart";

export function modRuntimeState(mod: ModFile): ModRuntimeState | null {
  if (!mod.enabled) return "disabled";
  if (mod.runtimePresent === false) return "runtimeFileMissing";
  if (mod.runtimeEnabled === false) return "notApplied";
  if (mod.runtimeEnabled === true) return "enabled";

  if (mod.providerKey === "dont-starve-together") {
    return dstModScope(mod) === "client" ? null : "configured";
  }

  return "pendingRestart";
}
