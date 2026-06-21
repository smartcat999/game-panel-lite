import type { MessageKey } from "./i18n";
import type { ProviderKey, ServerMode } from "./types";

export type ServerDisplayProvider = {
  label: string;
  tone: "green" | "purple" | "sky" | "amber" | "slate";
};

const providerLabels: Record<string, ServerDisplayProvider> = {
  "terraria-vanilla": { label: "Terraria", tone: "green" },
  "terraria-tmodloader": { label: "tModLoader", tone: "purple" },
  palworld: { label: "Palworld", tone: "sky" },
  "dont-starve-together": { label: "Don't Starve Together", tone: "amber" },
  minecraft: { label: "Minecraft Java", tone: "green" }
};

export function serverProviderDisplay(server: { mode?: ServerMode; providerKey?: ProviderKey }): ServerDisplayProvider {
  const providerKey = server.providerKey || legacyProviderKey(server.mode);
  return providerLabels[providerKey] ?? { label: formatProviderKey(providerKey), tone: "slate" };
}

export function serverResourceLabelKey(server: { gameKey?: string; providerKey?: ProviderKey }): MessageKey {
  const providerKey = server.providerKey ?? "";
  if (providerKey === "palworld") return "save";
  if (providerKey === "dont-starve-together") return "clusterSave";
  return "world";
}

function legacyProviderKey(mode?: ServerMode): ProviderKey {
  return mode === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla";
}

function formatProviderKey(key: string) {
  return key
    .split("-")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}
