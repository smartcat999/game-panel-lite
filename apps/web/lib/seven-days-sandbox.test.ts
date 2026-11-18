import { describe, expect, it } from "vitest";
import { decodeSandboxCode, sandboxCategories, sandboxDisabledBy, sandboxOptions, sandboxValue, updateSandboxOption } from "./seven-days-sandbox";

describe("Seven Days build 25661908 sandbox codes", () => {
  it("matches the actual game category inventory and default indices", () => {
    expect(sandboxCategories.map((c) => sandboxOptions.filter((o) => o.category === c.key).length)).toEqual([33, 26, 23, 25, 23, 14, 10, 11]);
    expect(new Set(sandboxOptions.map((o) => o.id)).size).toBe(165);
    for (const o of sandboxOptions) {
      expect(o.label.zh).not.toMatch(/^go/);
      expect(o.choices.length).toBeLessThanOrEqual(26);
      expect(o.choices[o.defaultIndex]).toBeDefined();
      expect(sandboxValue("A", o)).toBe(o.defaultIndex);
    }
  });

  it("reads the shipped Adventurer code using native option IDs and value indices", () => {
    const code = "AAAJABJACJADJARFBNC";
    expect([...decodeSandboxCode(code)]).toEqual([[0, 9], [1, 9], [2, 9], [3, 9], [17, 5], [39, 2]]);
    expect(sandboxOptions.find((o) => o.id === 0)?.choices[9]?.value).toBe(1.5);
    expect(sandboxOptions.find((o) => o.id === 17)?.choices[5]?.value).toBe(0.75);
  });

  it("round trips every verified value without discarding unrelated or unknown blocks", () => {
    const original = "AAAJABJACJADJARFBNCZZZ";
    const originalValues = decodeSandboxCode(original);
    for (const option of sandboxOptions) {
      for (let index = 0; index < option.choices.length; index++) {
        const code = updateSandboxOption(original, option, index);
        expect(sandboxValue(code, option)).toBe(index);
        for (const [id, value] of originalValues) {
          if (id !== option.id) expect(decodeSandboxCode(code).get(id)).toBe(value);
        }
      }
    }
    expect(original).toBe("AAAJABJACJADJARFBNCZZZ");
  });

  it("rejects malformed, duplicate and future-version codes instead of silently rewriting them", () => {
    for (const code of ["", "B", "a", "AAAAA", "A123", "AAAAAAA", "A AAJ"]) {
      expect(() => decodeSandboxCode(code)).toThrow();
    }
    expect(() => updateSandboxOption("A", sandboxOptions[0]!, -1)).toThrow();
    expect(() => updateSandboxOption("A", sandboxOptions[0]!, 0.5)).toThrow();
    expect(() => updateSandboxOption("A", sandboxOptions[0]!, 26)).toThrow();
  });

  it("retains disabled child values when a parent setting changes", () => {
    const parent = sandboxOptions.find((o) => o.key === "ChallengesEnabled")!;
    const child = sandboxOptions.find((o) => o.key === "IntroChallengesEnabled")!;
    const code = updateSandboxOption("A", parent, parent.choices.findIndex((c) => c.value === 0));
    expect(sandboxDisabledBy(code, child)?.id).toBe(parent.id);
    expect(sandboxValue(code, child)).toBe(child.defaultIndex);
    const warning = sandboxOptions.find((o) => o.key === "BloodMoonWarning")!;
    expect(sandboxDisabledBy("A", warning)).toBeUndefined();
  });
});
