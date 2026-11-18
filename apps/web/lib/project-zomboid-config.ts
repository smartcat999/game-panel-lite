import metadata from "../../api/internal/provider/projectzomboid/data/options.json";
import { providerConfigValue, updateProviderConfigPath, type ProviderConfigPayload } from "./provider-config";
import type { ProviderConfigField } from "./types";

type Rule = { kind: string; indexBase?: number; targets?: Record<string, number[]>; target?: string; true?: number; false?: number };
export type PZOption = {
  name: string; type: string; defaultValue: unknown; label: string; labelEn: string;
  help: string; helpEn: string; page: string | null; visible: boolean;
  min: number | null; max: number | null; maxLength: number | null;
  choices?: Array<{ value: number; label: string; labelEn: string }>;
  constraint?: { kind: string; month?: string; year?: number; overflow?: string };
  onChange?: Rule; readOnly?: boolean; readonly?: boolean; editor?: string;
};
export type PZPage = {
  id: string; label: string; labelEn: string; fields: string[]; visible: boolean; editor?: string;
  officialMaps?: string[];
  officialRegions?: Array<{ id: string; label: string; labelEn: string }>;
  defaultRegions?: Array<{ name: string }>;
};
export type PZPreset = { id: string; label?: string; labelEn?: string; zh?: string; values: Record<string, unknown> };
export const pzMetadata = metadata as unknown as {
  source: { gameVersion: string; firstYear?: number; dateConstraint?: { firstYear?: number } };
  server: PZOption[]; sandbox: PZOption[];
  pages: { server: PZPage[]; sandbox: PZPage[] }; presets: PZPreset[];
};
export type PZScope = "server" | "sandbox";

export function pzOption(path: string): PZOption | undefined {
  const [scope, ...parts] = path.split(".");
  return scope === "server" || scope === "sandbox" ? pzMetadata[scope].find((option) => option.name === parts.join(".")) : undefined;
}

export function isPZFieldVisible(field: Pick<ProviderConfigField, "name">): boolean {
  if (field.name === "adminPassword") return true;
  return pzOption(field.name)?.visible === true;
}

export function pzPages(scope: PZScope): PZPage[] {
  return pzMetadata.pages[scope].filter((page) => page.visible && (page.editor === "presets" || page.fields.some((name) => pzOption(`${scope}.${name}`)?.visible)));
}

export function pzSandboxDefaults(): ProviderConfigPayload {
  return pzMetadata.sandbox.reduce((payload, option) => updateProviderConfigPath(payload, option.name, option.defaultValue), {});
}

// Apply raw preset values once. Native onChange handlers apply to user edits only.
export function applyPZPreset(payload: ProviderConfigPayload, preset: PZPreset): ProviderConfigPayload {
  const sandbox = Object.entries(preset.values).reduce((draft, [name, value]) => updateProviderConfigPath(draft, name, value), pzSandboxDefaults());
  return { ...payload, sandbox, preset: preset.id };
}

export function pzStartDayMax(payload: ProviderConfigPayload): number {
  const year = pzOption("sandbox.StartDay")?.constraint?.year ?? pzMetadata.source.dateConstraint?.firstYear ?? pzMetadata.source.firstYear ?? 1993;
  const month = Number(providerConfigValue(payload, "sandbox.StartMonth") ?? 1);
  return new Date(Date.UTC(year, month, 0)).getUTCDate();
}

export function updatePZField(payload: ProviderConfigPayload, field: ProviderConfigField, value: string | boolean): ProviderConfigPayload {
  const option = pzOption(field.name);
  if (pzINIStringError(field.name, value, "en") || (field.name === "server.SpawnPoint" && !isPZSpawnPoint(String(value)))) return payload;
  const typed = option?.type === "boolean" ? value === true : option && ["integer", "double", "enum", "number"].includes(option.type) ? Number(value) : value;
  let next = updateProviderConfigPath(payload, field.name, typed);
  const rule = option?.onChange;
  if (field.name.startsWith("sandbox.") && rule?.kind === "lookup") {
    const index = Number(typed) - (rule.indexBase ?? 1);
    for (const [name, values] of Object.entries(rule.targets ?? {})) {
      const target = values[index];
      if (target !== undefined) next = updateProviderConfigPath(next, `sandbox.${name}`, target);
    }
  }
  if (field.name.startsWith("sandbox.") && rule?.kind === "boolean" && rule.target) {
    next = updateProviderConfigPath(next, `sandbox.${rule.target}`, typed === true ? rule.true : rule.false);
  }
  if (["sandbox.StartYear", "sandbox.StartMonth", "sandbox.StartDay"].includes(field.name)) {
    const day = Number(providerConfigValue(next, "sandbox.StartDay") ?? 1);
    next = updateProviderConfigPath(next, "sandbox.StartDay", Math.min(Math.max(day, 1), pzStartDayMax(next)));
  }
  return next;
}

export function validatePZConfig(payload: ProviderConfigPayload, fields: ProviderConfigField[], locale: string): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const field of fields) {
    if (!isPZFieldVisible(field) || field.name === "adminPassword") continue;
    const option = pzOption(field.name);
    if (!option) continue;
    const value = providerConfigValue(payload, field.name);
    const label = field.name.startsWith("server.") ? option.name : locale === "zh" ? option.label : option.labelEn;
    const iniError = pzINIStringError(field.name, value, locale);
    if (iniError) {
      errors[field.name] = iniError;
      continue;
    }
    if (field.name === "server.SpawnPoint" && value !== undefined && !isPZSpawnPoint(String(value))) {
      errors[field.name] = locale === "zh" ? "SpawnPoint 需要 x,y,z 三个非负整数" : "SpawnPoint requires three non-negative integers: x,y,z";
      continue;
    }
    if (value === undefined || value === null || value === "") {
      if (field.required) errors[field.name] = locale === "zh" ? `请填写${label}` : `${label} is required`;
      continue;
    }
    if (["integer", "double", "enum"].includes(option.type)) {
      const number = Number(value);
      if (!Number.isFinite(number) || (option.type !== "double" && !Number.isInteger(number))) {
        errors[field.name] = locale === "zh" ? `${label}需要有效数字` : `${label} needs a valid number`;
      } else if ((option.min !== null && number < option.min) || (option.max !== null && number > option.max)) {
        errors[field.name] = locale === "zh" ? `${label}范围：${option.min}–${option.max}` : `${label} range: ${option.min}–${option.max}`;
      }
    }
  }
  return errors;
}


export function pzINIStringError(path: string, value: unknown, locale: string): string | undefined {
  const option = pzOption(path);
  if (!path.startsWith("server.") || !option || !["string", "text"].includes(option.type) || typeof value !== "string") return undefined;
  // eslint-disable-next-line no-control-regex -- Java INI roundtrip rejects all ASCII controls.
  if (/[\u0000-\u001f=]/.test(value)) return locale === "zh"
    ? "INI 文本不能包含 ASCII 控制字符或等号；显示换行请使用 <LINE>。"
    : "INI text cannot contain ASCII control characters or equals signs. Use <LINE> for display line breaks.";
  return undefined;
}

export function isPZSpawnPoint(value: string): boolean {
  return /^\d+,\d+,\d+$/.test(value) && value.split(",").every((coordinate) => Number.isSafeInteger(Number(coordinate)));
}


export function isPZRuntimeReadOnlyField(path: string): boolean {
  return ["server.RCONPassword", "server.RCONPort", "server.UPnP"].includes(path);
}
