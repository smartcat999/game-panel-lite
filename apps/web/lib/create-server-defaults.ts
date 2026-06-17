import { getTerrariaPreset } from "@gamepanel-lite/shared";

export const defaultCreateServerMode = "vanilla" as const;
export const defaultCreateServerPreset = "friends-casual" as const;
export const defaultCreateServerConfig = getTerrariaPreset(defaultCreateServerPreset).config;
