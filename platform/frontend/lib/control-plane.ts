export type UserPreferences = {
  locale: "en" | "zh-CN";
  theme: "light" | "dark" | "system";
  timeZone: string;
};

export type Session = {
  userId: string;
  workspaceIds: string[];
  selectedWorkspaceId: string;
  platformOperator: boolean;
};

export type Workspace = {
  id: string;
  slug: string;
  name: string;
};

export type WorkspaceMember = {
  membershipId: string;
  user: { id: string; displayName: string; email: string };
  role: "owner" | "administrator" | "operator" | "billing" | "viewer";
};

export type Region = { id: string; code: string; name: string; available: boolean };
export type PlatformRegion = { id: string; code: string; name: string; operationalState: "active" | "maintenance" | "disabled" };
export type PlanVersion = {
  id: string; planId: string; version: number; name: string; priceMinor: number; currency: string;
  billingPeriod: string; regionIds: string[]; memoryMegabytes: number; cpuUnits: number;
};
export type DeploymentState = "pending_payment" | "waiting_region" | "running" | "stopped" | "failed";
export type LogicalInstance = {
  id: string; workspaceId: string; name: string; gameKey: string; desiredState: "running" | "stopped";
  billingState: "pending_payment" | "active"; deploymentState: DeploymentState; stale: boolean; createdAt: string;
};
export type InstanceDetail = {
  instance: LogicalInstance;
  revision: { id: string; logicalInstanceId: string; version: number; gameVersion: string; configuration: Record<string, unknown>; createdAt: string };
  placement: { id: string; logicalInstanceId: string; regionId: string; version: number; createdAt: string };
};
export type Order = {
  id: string; workspaceId: string; logicalInstanceId: string; planVersionId: string;
  status: "pending_payment" | "paid"; expiresAt: string;
};
export type CreateInstanceInput = {
  workspaceId: string; planVersionId: string; regionId: string; name: string;
  gameKey: string; gameVersion: string; configuration: Record<string, unknown>;
};
export type CheckoutResult = { instance: LogicalInstance; order: Order };

export class ControlPlaneError extends Error {
  constructor(public readonly status: number) {
    super(`Control Plane request failed with ${status}`);
  }
}

const previewToken = "local-preview";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/control-plane${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${previewToken}`,
      "Content-Type": "application/json",
      ...init?.headers,
    },
  });
  if (!response.ok) {
    throw new ControlPlaneError(response.status);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return response.json() as Promise<T>;
}

export const controlPlane = {
  session: () => request<Session>("/v1/session"),
  preferences: () => request<UserPreferences>("/v1/user-preferences"),
  updatePreferences: (update: Partial<UserPreferences>) => request<UserPreferences>("/v1/user-preferences", { method: "PATCH", body: JSON.stringify(update) }),
  workspaces: () => request<Workspace[]>("/v1/workspaces"),
  selectWorkspace: (workspaceId: string) => request<void>("/v1/workspace-selection", { method: "POST", body: JSON.stringify({ workspaceId }) }),
  workspaceMembers: (workspaceId: string) => request<WorkspaceMember[]>(`/v1/workspaces/${encodeURIComponent(workspaceId)}/members`),
  regions: () => request<Region[]>("/v1/regions"),
  plans: () => request<PlanVersion[]>("/v1/plans"),
  workspaceInstances: (workspaceId: string) => request<LogicalInstance[]>(`/v1/workspaces/${encodeURIComponent(workspaceId)}/instances`),
  workspaceOrders: (workspaceId: string) => request<Order[]>(`/v1/workspaces/${encodeURIComponent(workspaceId)}/orders`),
  instance: (workspaceId: string, instanceId: string) => request<InstanceDetail>(`/v1/workspaces/${encodeURIComponent(workspaceId)}/instances/${encodeURIComponent(instanceId)}`),
  createInstance: (input: CreateInstanceInput, idempotencyKey: string) => request<CheckoutResult>("/v1/instances", {
    method: "POST", body: JSON.stringify(input), headers: { "Idempotency-Key": idempotencyKey, "X-Command-ID": `cmd_${idempotencyKey}` },
  }),
  platformWorkspaces: () => request<Workspace[]>("/v1/platform/workspaces"),
  platformPlans: () => request<PlanVersion[]>("/v1/platform/plans"),
  platformOrders: () => request<Order[]>("/v1/platform/orders"),
  platformInstances: () => request<LogicalInstance[]>("/v1/platform/instances"),
  platformRegions: () => request<PlatformRegion[]>("/v1/platform/regions"),
};
