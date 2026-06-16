import type { ModFile } from "./types";

export function modDisplayName(mod: ModFile, locale: string) {
  if (mod.source === "workshop" && mod.workshopId) {
    return `${locale === "zh" ? "创意工坊" : "Workshop"} ${mod.workshopId}`;
  }
  return mod.fileName;
}

export function modSourceLabel(mod: ModFile, locale: string) {
  if (mod.source === "workshop") {
    return locale === "zh" ? "创意工坊" : "Workshop";
  }
  return ".tmod";
}
