export const KNOWN_DST_CHARACTERS = new Set([
  "wilson",
  "willow",
  "wolfgang",
  "wendy",
  "wx78",
  "wickerbottom",
  "woodie",
  "wes",
  "maxwell",
  "waxwell",
  "wigfrid",
  "webber",
  "warly",
  "wormwood",
  "winona",
  "wortox",
  "wurt",
  "walter",
  "wanda"
]);

const DST_NAMES_ZH: Record<string, string> = {
  wilson: "威尔逊",
  willow: "薇洛",
  wolfgang: "沃尔夫冈",
  wendy: "温蒂",
  wx78: "WX-78",
  wickerbottom: "薇克巴顿",
  woodie: "伍迪",
  wes: "韦斯",
  maxwell: "麦斯威尔",
  waxwell: "麦斯威尔",
  wigfrid: "薇格弗德",
  webber: "韦伯",
  winona: "薇诺娜",
  warly: "沃利",
  wortox: "沃拓克斯",
  wormwood: "沃姆伍德",
  wurt: "沃特",
  walter: "沃尔特",
  wanda: "旺达"
};

const DST_NAMES_EN: Record<string, string> = {
  wilson: "Wilson",
  willow: "Willow",
  wolfgang: "Wolfgang",
  wendy: "Wendy",
  wx78: "WX-78",
  wickerbottom: "Wickerbottom",
  woodie: "Woodie",
  wes: "Wes",
  maxwell: "Maxwell",
  waxwell: "Maxwell",
  wigfrid: "Wigfrid",
  webber: "Webber",
  winona: "Winona",
  warly: "Warly",
  wortox: "Wortox",
  wormwood: "Wormwood",
  wurt: "Wurt",
  walter: "Walter",
  wanda: "Wanda"
};

export function getDSTCharacterImage(character?: string): string | null {
  if (!character) return null;
  const normalized = character.toLowerCase().replace(/[^a-z0-9]/g, "");
  if (normalized === "waxwell") {
    return "/images/dst/characters/maxwell.png";
  }
  if (KNOWN_DST_CHARACTERS.has(normalized)) {
    return `/images/dst/characters/${normalized}.png`;
  }
  return null;
}

export function getDSTCharacterDisplayName(character?: string, isZh = false): string | null {
  if (!character) return null;
  const normalized = character.toLowerCase().replace(/[^a-z0-9]/g, "");
  if (isZh) {
    return DST_NAMES_ZH[normalized] || character;
  }
  return DST_NAMES_EN[normalized] || character;
}
