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

export function dstModScope(mod: ModFile): "client" | "required" | "unknown" {
  if (mod.providerKey !== "dont-starve-together") return "unknown";
  const tags = new Set((mod.tags ?? []).map((tag) => tag.toLowerCase()));
  if (tags.has("client_only_mod")) return "client";
  if (tags.has("all_clients_require_mod") || tags.has("server_only_mod")) return "required";
  return "unknown";
}
