import type { ProviderCatalog, ProviderConfigField } from "./types";

export type ProviderConfigPayload = Record<string, unknown>;

export function createDefaultProviderConfigPayload(
  provider?: ProviderCatalog,
  overrides: ProviderConfigPayload = {}
): ProviderConfigPayload {
  const payload: ProviderConfigPayload = {};
  for (const field of provider?.configSchema ?? []) {
    payload[field.name] = defaultProviderFieldValue(field);
  }
  return { ...payload, ...overrides };
}

export function updateProviderConfigPayload(
  payload: ProviderConfigPayload,
  field: ProviderConfigField,
  value: string | boolean
): ProviderConfigPayload {
  return {
    ...payload,
    [field.name]: coerceProviderFieldValue(field, value)
  };
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
