export const terrariaLanguageOptions = [
  { value: "zh-Hans", labelKey: "languageChinese" },
  { value: "en-US", labelKey: "languageEnglish" }
] as const;

export type TerrariaLanguage = (typeof terrariaLanguageOptions)[number]["value"];

export function getTerrariaLanguageLabel(value: string | undefined, t: (key: "languageChinese" | "languageEnglish") => string) {
  const option = terrariaLanguageOptions.find((item) => item.value === value);
  return option ? t(option.labelKey) : value || "";
}
