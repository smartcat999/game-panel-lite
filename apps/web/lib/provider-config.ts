import type { ProviderCatalog, ProviderConfigField } from "./types";

export type ProviderConfigPayload = Record<string, unknown>;

export function createDefaultProviderConfigPayload(
  provider?: ProviderCatalog,
  overrides: ProviderConfigPayload = {}
): ProviderConfigPayload {
  const payload: ProviderConfigPayload = {};
  for (const field of provider?.configSchema ?? []) {
    setProviderConfigValue(payload, field.name, defaultProviderFieldValue(field));
  }
  return deepMergeProviderPayload(payload, overrides);
}

export function updateProviderConfigPayload(
  payload: ProviderConfigPayload,
  field: ProviderConfigField,
  value: string | boolean
): ProviderConfigPayload {
  return {
    ...setProviderConfigValue({ ...payload }, field.name, coerceProviderFieldValue(field, value))
  };
}

export function providerConfigValue(payload: ProviderConfigPayload | undefined, path: string): unknown {
  const parts = path.split(".").filter(Boolean);
  let cursor: unknown = payload;
  for (const part of parts) {
    if (!isRecord(cursor)) return undefined;
    cursor = cursor[part];
  }
  return cursor;
}

export function isAdvancedProviderConfigField(providerKey: string, field: ProviderConfigField): boolean {
  if (providerKey === "dont-starve-together") return Boolean(field.group);
  if (providerKey === "palworld") return Boolean(field.group && field.group !== "基础设置");
  return false;
}

export function isWorldGenerationProviderConfigField(providerKey: string, field: ProviderConfigField): boolean {
  if (providerKey !== "dont-starve-together") return false;
  return field.name === "world.preset"
    || field.name === "caves.enabled"
    || Boolean(field.group?.includes(".worldgen."));
}

export function providerConfigFieldChanged(
  current: ProviderConfigPayload | undefined,
  draft: ProviderConfigPayload | undefined,
  field: ProviderConfigField
): boolean {
  const currentValue = providerConfigValue(current, field.name);
  const draftValue = providerConfigValue(draft, field.name);
  if (field.type === "number") return Number(currentValue) !== Number(draftValue);
  if (field.type === "boolean") return Boolean(currentValue) !== Boolean(draftValue);
  return String(currentValue ?? "") !== String(draftValue ?? "");
}

export function restoreProviderConfigDefaults(
  payload: ProviderConfigPayload,
  fields: ProviderConfigField[]
): ProviderConfigPayload {
  return fields.reduce(
    (current, field) => updateProviderConfigPayload(current, field, providerFieldDefaultInputValue(field)),
    payload
  );
}

export function isProviderFieldModified(payload: ProviderConfigPayload | undefined, field: ProviderConfigField): boolean {
  const current = providerConfigValue(payload, field.name);
  const fallback = defaultProviderFieldValue(field);
  if (field.type === "number") return Number(current) !== Number(fallback);
  if (field.type === "boolean") return Boolean(current) !== Boolean(fallback);
  return String(current ?? "") !== String(fallback ?? "");
}

function defaultProviderFieldValue(field: ProviderConfigField): unknown {
  if (field.default !== undefined) return field.default;
  if (field.type === "number") return 0;
  if (field.type === "boolean") return false;
  return "";
}

function coerceProviderFieldValue(field: ProviderConfigField, value: string | boolean): unknown {
  if (field.type === "boolean") return Boolean(value);
  if (field.type === "number") {
    const nextValue = Number(value);
    return Number.isFinite(nextValue) ? nextValue : 0;
  }
  return String(value);
}

function providerFieldDefaultInputValue(field: ProviderConfigField): string | boolean {
  if (field.type === "boolean") return field.default === true;
  return String(field.default ?? (field.type === "number" ? 0 : ""));
}

function setProviderConfigValue(payload: ProviderConfigPayload, path: string, value: unknown): ProviderConfigPayload {
  const parts = path.split(".").filter(Boolean);
  if (parts.length === 0) return payload;
  let cursor: ProviderConfigPayload = payload;
  for (const part of parts.slice(0, -1)) {
    const existing = cursor[part];
    const next = isRecord(existing) ? { ...existing } : {};
    cursor[part] = next;
    cursor = next;
  }
  const leaf = parts[parts.length - 1];
  if (leaf) cursor[leaf] = value;
  return payload;
}

function deepMergeProviderPayload(base: ProviderConfigPayload, overrides: ProviderConfigPayload): ProviderConfigPayload {
  const next: ProviderConfigPayload = { ...base };
  for (const [key, value] of Object.entries(overrides)) {
    if (isRecord(value) && isRecord(next[key])) {
      next[key] = deepMergeProviderPayload(next[key], value);
      continue;
    }
    next[key] = value;
  }
  return next;
}

function isRecord(value: unknown): value is ProviderConfigPayload {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
