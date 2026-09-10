export const queryKeys = {
  session: ["session"] as const,
  preferences: (userId: string) => ["users", userId, "preferences"] as const,
  workspace: (workspaceId: string) => ["workspaces", workspaceId] as const,
  workspaceMembers: (workspaceId: string) => ["workspaces", workspaceId, "members"] as const,
  workspaceInstances: (workspaceId: string) => ["workspaces", workspaceId, "instances"] as const,
  workspaceOrders: (workspaceId: string) => ["workspaces", workspaceId, "orders"] as const,
  instance: (workspaceId: string, instanceId: string) => ["workspaces", workspaceId, "instances", instanceId] as const,
  regions: ["regions"] as const,
  plans: ["plans"] as const,
  platform: (resource: string) => ["platform", resource] as const,
};
