import type { MessageKey } from "./i18n";
import type { ProviderKey } from "./types";

const providerNameKeys: Partial<Record<string, MessageKey>> = {
  "terraria-vanilla": "providerNameTerrariaVanilla",
  "terraria-tmodloader": "providerNameTerrariaTmodloader",
  palworld: "providerNamePalworld",
  "dont-starve-together": "providerNameDST",
  minecraft: "providerNameMinecraft"
};

const providerDescriptionKeys: Partial<Record<string, MessageKey>> = {
  "terraria-vanilla": "providerDescTerrariaVanilla",
  "terraria-tmodloader": "providerDescTerrariaTmodloader",
  palworld: "providerDescPalworld",
  "dont-starve-together": "providerDescDST",
  minecraft: "providerDescMinecraft"
};

export function providerDisplayName(key: ProviderKey | string | undefined, fallback: string, t: (key: MessageKey) => string): string {
  const msgKey = providerNameKeys[key ?? ""];
  return msgKey ? t(msgKey) : fallback;
}

export function providerDescription(key: ProviderKey | string | undefined, fallback: string, t: (key: MessageKey) => string): string {
  const msgKey = providerDescriptionKeys[key ?? ""];
  return msgKey ? t(msgKey) : fallback;
}
