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
    throw new Error(`Control Plane request failed with ${response.status}`);
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
};
