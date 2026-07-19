import { describe, expect, it } from "vitest";
import { providerOptionLabel } from "./provider-option-label";
import type { MessageKey } from "./i18n";
import type { ProviderConfigField } from "./types";

const field: ProviderConfigField = {
  name: "world.overrides.grass",
  label: "Grass",
  type: "select",
  required: false
};

const labels: Partial<Record<MessageKey, string>> = {
  defaultValue: "默认",
  dstOptionInsane: "极多",
  dstOptionNever: "从不",
  dstOptionRare: "稀少"
};

const t = (key: MessageKey) => labels[key] ?? key;

describe("provider option labels", () => {
  it("localizes common DST enum values without changing their stored values", () => {
    expect(providerOptionLabel(field, "never", "never", t)).toBe("从不");
    expect(providerOptionLabel(field, "rare", "rare", t)).toBe("稀少");
    expect(providerOptionLabel(field, "default", "default", t)).toBe("默认");
    expect(providerOptionLabel(field, "insane", "insane", t)).toBe("极多");
  });

  it("keeps the backend label for unknown provider values", () => {
    expect(providerOptionLabel(field, "custom", "Custom value", t)).toBe("Custom value");
  });
});
