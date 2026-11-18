import type { MessageKey } from "@/lib/i18n";
import type { ProviderConfigField } from "@/lib/types";

const providerOptionLabelKeys: Record<string, MessageKey> = {
  "*:always": "dstOptionAlways",
  "*:autumn|spring": "dstOptionAutumnOrSpring",
  "*:autumn|winter|spring|summer": "dstOptionRandomSeason",
  "*:default": "defaultValue",
  "*:fast": "dstOptionFast",
  "*:huge": "dstOptionHuge",
  "*:insane": "dstOptionInsane",
  "*:longday": "dstOptionLongDay",
  "*:longdusk": "dstOptionLongDusk",
  "*:longnight": "dstOptionLongNight",
  "*:longseason": "dstOptionLong",
  "*:medium": "dstOptionMedium",
  "*:mostly": "dstOptionMostly",
  "*:noday": "dstOptionNoDay",
  "*:nodusk": "dstOptionNoDusk",
  "*:nonight": "dstOptionNoNight",
  "*:noseason": "dstOptionNone",
  "*:never": "dstOptionNever",
  "*:often": "dstOptionOften",
  "*:onlyday": "dstOptionOnlyDay",
  "*:onlydusk": "dstOptionOnlyDusk",
  "*:onlynight": "dstOptionOnlyNight",
  "*:random": "dstOptionRandom",
  "*:rare": "dstOptionRare",
  "*:shortseason": "dstOptionShort",
  "*:slow": "dstOptionSlow",
  "*:small": "dstOptionSmall",
  "*:spring": "dstOptionSpring",
  "*:summer": "dstOptionSummer",
  "*:uncommon": "dstOptionUncommon",
  "*:veryfast": "dstOptionVeryFast",
  "*:verylongseason": "dstOptionVeryLong",
  "*:veryshortseason": "dstOptionVeryShort",
  "*:veryslow": "dstOptionVerySlow",
  "*:winter": "dstOptionWinter",
  "*:winter|summer": "dstOptionWinterOrSummer",
  "gameplay.gameMode:endless": "dstGameModeEndless",
  "gameplay.gameMode:survival": "dstGameModeSurvival",
  "gameplay.gameMode:wilderness": "dstGameModeWilderness",
  "identity.visibility:lan": "dstVisibilityLan",
  "identity.visibility:offline": "dstVisibilityOffline",
  "identity.visibility:public": "dstVisibilityPublic",
  "world.preset:forest_classic": "dstWorldPresetClassic",
  "world.preset:forest_default": "dstWorldPresetDefault",
  "world.preset:forest_survival": "dstWorldPresetSurvival"
};

export function providerOptionLabel(
  field: ProviderConfigField,
  value: string,
  fallback: string,
  t: (key: MessageKey) => string
) {
  const key = providerOptionLabelKeys[`${field.name}:${value}`] ?? providerOptionLabelKeys[`*:${value}`];
  return key ? t(key) : fallback;
}
