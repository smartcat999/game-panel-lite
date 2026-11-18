import { describe, expect, it } from "vitest";
import { getTerrariaLanguageLabel } from "./terraria-language";

const labels = {
  languageChinese: "中文",
  languageEnglish: "英文"
};

describe("getTerrariaLanguageLabel", () => {
  it("renders known Terraria language codes as localized labels", () => {
    expect(getTerrariaLanguageLabel("zh-Hans", (key) => labels[key])).toBe("中文");
    expect(getTerrariaLanguageLabel("en-US", (key) => labels[key])).toBe("英文");
  });

  it("keeps unknown language codes visible", () => {
    expect(getTerrariaLanguageLabel("fr-FR", (key) => labels[key])).toBe("fr-FR");
  });
});
