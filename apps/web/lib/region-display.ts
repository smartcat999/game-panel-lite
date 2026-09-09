import type { RegionDirectoryEntry } from "./types";
import type { Locale } from "./i18n";

export function regionDisplayName(region: Pick<RegionDirectoryEntry, "id" | "name">, locale: Locale): string {
  if (region.id === "default") return locale === "zh" ? "默认区域" : "Default Region";
  return region.name;
}
