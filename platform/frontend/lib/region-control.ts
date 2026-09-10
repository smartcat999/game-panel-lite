export type RegionOverview = { regionId: string; readyNodes: number; deploymentCount: number; pendingTasks: number; unschedulableCount: number };
export type RegionNode = { id: string; regionId: string; name: string; state: "ready" | "draining" | "stale"; games: string[]; cpuCapacity: number; memoryCapacityMb: number; reservedCpu: number; reservedMemoryMb: number; leaseUntil: string; lastHeartbeatAt: string };
export type RegionalDeployment = { id: string; workspaceId: string; logicalInstanceId: string; regionId: string; placementVersion: number; instanceRevisionId: string; desiredState: string; observedState: string; observationSequence: number; gameKey: string; cpuUnits: number; memoryMegabytes: number; nodeId?: string; unschedulableReason?: string; updatedAt: string };
export type RegionalTask = { id: string; regionalDeploymentId: string; kind: string; status: string; attempts: number; createdAt: string };
export type RegionCapacity = { cpuCapacity: number; cpuReserved: number; memoryCapacityMb: number; memoryReservedMb: number };
export type RegionStorage = { directTransfer: boolean; configured: boolean };
export type RegionMonitoring = { inboxLag: number; outboxLag: number; staleNodes: number };

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/region-control${path}`, { ...init, headers: { Authorization: "Bearer local-preview", "Content-Type": "application/json", ...init?.headers } });
  if (!response.ok) throw new Error(`Region Control request failed with ${response.status}`);
  return response.json() as Promise<T>;
}

const base = (regionId: string) => `/v1/regions/${encodeURIComponent(regionId)}`;

export const regionControl = {
  overview: (regionId: string) => request<RegionOverview>(`${base(regionId)}/overview`),
  nodes: (regionId: string) => request<RegionNode[]>(`${base(regionId)}/nodes`),
  deployments: (regionId: string) => request<RegionalDeployment[]>(`${base(regionId)}/deployments`),
  tasks: (regionId: string) => request<RegionalTask[]>(`${base(regionId)}/tasks`),
  capacity: (regionId: string) => request<RegionCapacity>(`${base(regionId)}/capacity`),
  storage: (regionId: string) => request<RegionStorage>(`${base(regionId)}/storage`),
  monitoring: (regionId: string) => request<RegionMonitoring>(`${base(regionId)}/monitoring`),
  overridePlacement: (regionId: string, deploymentId: string, nodeId: string, reason: string) => request(`${base(regionId)}/deployments/${encodeURIComponent(deploymentId)}/placement-override`, { method: "POST", body: JSON.stringify({ nodeId, reason }) }),
};
