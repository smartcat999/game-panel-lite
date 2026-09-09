export const queryKeys = {
  session: ["session"] as const,
  preferences: (userId: string) => ["users", userId, "preferences"] as const,
  workspace: (workspaceId: string) => ["workspaces", workspaceId] as const,
  workspaceMembers: (workspaceId: string) => ["workspaces", workspaceId, "members"] as const,
};
