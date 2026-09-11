export const apiBase = "/control-plane/v1";

export class APIError extends Error {
  constructor(public status: number, public code: string) {
    super(code);
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, {
    ...init,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { error?: string };
    throw new APIError(response.status, body.error ?? `http_${response.status}`);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export function idempotencyKey(prefix: string) {
  return `${prefix}-${crypto.randomUUID()}`;
}

export type Workspace = { id: string; slug: string; name: string };
export type Region = { id: string; code: string; name: string; names?: Record<string, string>; available: boolean };
export type ResourceSpec = { cpuMilli: number; memoryMiB: number; diskGiB: number };
export type Endpoint = { name: string; purpose: string; address: string; port?: number; transports: string[]; stability: string; displayAddress: string; primary: boolean };
export type Instance = { id: string; workspaceId: string; regionId: string; name: string; providerReleaseId: string; gameVersion: string; configurationRevisionId: string; resourceSpec: ResourceSpec; desiredState: string; observedState: string; endpoints: Endpoint[]; latestOperationId: string; createdAt: string; updatedAt: string };
export type InstanceListItem = { id: string; workspaceId: string; name: string; desiredState: string; observedState: string; game: { providerReleaseId: string; key: string; displayName: string; version: string }; endpoints: Endpoint[]; resourceSpec: ResourceSpec; region: { id: string; code: string; displayName: string }; createdAt: string; updatedAt: string };
export type Operation = { id: string; kind: string; resourceType: string; resourceId: string; status: string; failureCode?: string; steps: Array<{ key: string; label: string; status: string; detail?: string }>; createdAt: string; updatedAt: string };
export type ProviderSummary = { id: string; gameKey: string; displayName: string; releaseVersion: string; gameVersions: string[]; capabilities: string[] };
export type ConfigurationField = { type: "string" | "integer" | "number" | "boolean" | "enum" | "secret" | "string-list"; title: string; description?: string; localizations?: Record<string, { title: string; description?: string; default?: unknown; enumLabels?: Record<string, string> }>; applyBehavior: "hot-reload" | "restart-required" | "recreate-required" | "create-only"; default?: unknown; minimum?: number; maximum?: number; enum?: unknown[] };
export type ProviderManifest = Omit<ProviderSummary, "id"> & { providerReleaseId: string; schemaVersion: number; configurationSchema: { properties: Record<string, ConfigurationField>; required?: string[] }; uiSchema: { sections: Array<{ id: string; title: string; localizations?: Record<string, string>; order: number }>; fields: Record<string, { section: string; order: number; control: string; visibleWhen?: { field: string; equals: unknown } }> }; modCatalog?: { entries: Array<{ modId: string; displayName: string; versions: Array<{ version: string; digest: string; dependencies?: Array<{ modId: string; version: string }> }> }> } };
export type RegionCatalog = { regionId: string; currency: string; resourceBounds: { cpuMilli: Range; memoryMiB: Range; diskGiB: Range }; unitPrices: Array<{ resourceKind: string; priceMinor: number; unitQuantity: number; unit: string }>; capacityState: string; endpointDeliveryModes: string[]; dedicatedIpAvailable: boolean; catalogVersion: number; priceBookId: string };
export type Range = { minimum: number; maximum: number; step: number };
export type Wallet = { workspaceId: string; currency: string; promotionalMinor: number; cashMinor: number; availableMinor: number; state: string };
export type Quote = { id: string; workspaceId: string; regionId: string; priceBookId: string; resourceSpec: ResourceSpec; currency: "CNY"; estimatedHourlyMinor: number; expiresAt: string };
export type Backup = { id: string; logicalInstanceId: string; regionId: string; operationId: string; providerReleaseId: string; gameVersion: string; configurationRevisionId: string; modLock: Array<{ modId: string; version: string; digest: string; direct: boolean }>; checksums: Record<string, string>; objectKey?: string; sizeBytes?: number; status: string; createdAt: string; updatedAt: string };
export type Revision = { id: string; operationId: string; logicalInstanceId: string; providerReleaseId: string; gameVersion: string; schemaVersion: number; configuration: Record<string, unknown>; modLock: Array<{ modId: string; version: string; digest: string; direct: boolean }>; applyBehavior: string; createdAt: string };
export type LogEntry = { id: string; logicalInstanceId: string; runtimeAttemptId: string; stream: string; message: string; observedAt: string };
