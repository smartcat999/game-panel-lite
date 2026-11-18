import definitions from "./seven-days-sandbox-data.json";

// Steam build 25661908: SandboxOptionManager.LoadOptionsFromCode and
// SandboxOptionPreset.saveOptionsToCode. Values are indices, not XML properties.
export const sandboxCategories = definitions.categories;
export const sandboxOptions = definitions.options;
export type SandboxOption = (typeof sandboxOptions)[number];

export function decodeSandboxCode(code: string): Map<number, number> {
  if (!/^A(?:[A-Z]{3})*$/.test(code)) throw new Error("unsupported sandbox code");
  const values = new Map<number, number>();
  for (let i = 1; i < code.length; i += 3) {
    const id = (code.charCodeAt(i) - 65) * 26 + code.charCodeAt(i + 1) - 65;
    if (values.has(id)) throw new Error("duplicate sandbox option");
    values.set(id, code.charCodeAt(i + 2) - 65);
  }
  return values;
}

export function sandboxValue(code: string, option: SandboxOption): number {
  return decodeSandboxCode(code).get(option.id) ?? option.defaultIndex;
}

export function updateSandboxOption(code: string, option: SandboxOption, index: number): string {
  if (!Number.isInteger(index) || index < 0 || index >= option.choices.length) throw new Error("invalid sandbox value");
  const values = decodeSandboxCode(code);
  // Preserve every other block, including unknown IDs, in its original order.
  if (index === option.defaultIndex) values.delete(option.id);
  else values.set(option.id, index);
  return "A" + Array.from(values, ([id, value]) => String.fromCharCode(65 + Math.floor(id / 26), 65 + id % 26, 65 + value)).join("");
}

export function sandboxDisabledBy(code: string, option: SandboxOption): SandboxOption | undefined {
  const values = decodeSandboxCode(code);
  const parents = sandboxOptions.filter((parent) => parent.dependency?.ids.includes(option.id));
  // BloodMoonWarning has multiple native setters whose application order is not
  // established here. Keep it editable instead of imposing an invented merge rule.
  if (parents.length !== 1) return undefined;
  const parent = parents[0];
  if (!parent?.dependency) return undefined;
  const dependency = parent.dependency;
  return ((parent.choices[values.get(parent.id) ?? parent.defaultIndex]?.value === dependency.value) !== dependency.inverted)
    ? parent : undefined;
}
