import { getTerrariaPreset, type TerrariaConfig } from "@gamepanel-lite/shared";

export const defaultCreateServerMode = "vanilla" as const;
export const defaultCreateServerPreset = "friends-casual" as const;
const defaultConfig = getTerrariaPreset(defaultCreateServerPreset).config;

export const defaultCreateServerConfig: TerrariaConfig = {
  ...defaultConfig,
  specialSeeds: [...(defaultConfig.specialSeeds ?? [])],
  secretSeeds: [...(defaultConfig.secretSeeds ?? [])]
};
