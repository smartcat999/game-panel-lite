import type { MessageKey } from "@/lib/i18n";

const dstGroupLabelKeys: Record<string, MessageKey> = {
  "worldgen.monsters": "dstGroupWorldgenMonsters",
  "worldgen.animals": "dstGroupWorldgenAnimals",
  "worldgen.resources": "dstGroupWorldgenResources",
  "worldgen.misc": "dstGroupWorldgenMisc",
  "worldgen.global": "dstGroupWorldgenGlobal",
  "worldsettings.lunar_mutations": "dstGroupSettingsLunarMutations",
  "worldsettings.giants": "dstGroupSettingsGiants",
  "worldsettings.monsters": "dstGroupSettingsMonsters",
  "worldsettings.animals": "dstGroupSettingsAnimals",
  "worldsettings.resources": "dstGroupSettingsResources",
  "worldsettings.misc": "dstGroupSettingsMisc",
  "worldsettings.survivors": "dstGroupSettingsSurvivors",
  "worldsettings.events": "dstGroupSettingsEvents",
  "worldsettings.global": "dstGroupSettingsGlobal"
};

export function dstConfigGroupLabelKey(group: string): MessageKey | undefined {
  const parts = group.split(".");
  if (parts.length !== 4 || parts[0] !== "dst") return undefined;
  return dstGroupLabelKeys[`${parts[2]}.${parts[3]}`];
}
