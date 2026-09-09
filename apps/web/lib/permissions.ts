"use client";

import { useAuthBootstrap } from "./auth-session";
import type { Permission, UserRole } from "./types";
import { useConsoleContext } from "./console-context";

const rolePermissions: Record<UserRole, readonly Permission[]> = {
  admin: [
    "server.view", "server.create", "server.control", "server.configure", "server.delete",
    "backup.manage", "world.manage", "mod.manage", "player.manage", "share.manage",
    "node.manage", "team.manage", "settings.manage", "system.manage"
  ],
  member: [
    "server.view", "server.create", "server.control", "server.configure",
    "backup.manage", "world.manage", "mod.manage", "player.manage", "share.manage"
  ],
  viewer: ["server.view"]
};

export function permissionsForRole(role: UserRole): readonly Permission[] {
  return rolePermissions[role];
}

export function usePermissions() {
  const authQuery = useAuthBootstrap();
  const { scope, currentOrganization } = useConsoleContext();

  const account = authQuery.data?.account;
  const platformRole = account?.platformRole ?? (authQuery.data?.initialized === false ? "platform_admin" : "user");
  const membershipRole = currentOrganization?.membershipRole;
  const role: UserRole = scope.kind === "organization"
    ? membershipRole === "viewer" ? "viewer" : membershipRole === "member" ? "member" : "admin"
    : account?.role ?? (authQuery.data?.initialized === false ? "admin" : "viewer");
  const scopedPermissions = scope.kind === "platform" && platformRole === "platform_admin"
    ? permissionsForRole("admin")
    : permissionsForRole(role);
  const permissions = new Set<Permission>(scope.kind === "platform" ? account?.permissions ?? scopedPermissions : scopedPermissions);
  const can = (permission: Permission) => permissions.has(permission);

  const isViewer = role === "viewer";
  const isMember = role === "member";
  const isAdmin = platformRole === "platform_admin";

  return {
    account,
    role,
    platformRole,
    permissions,
    can,
    isViewer,
    isMember,
    isAdmin,
    isLoading: authQuery.isLoading,
    // 细粒度权限判定
    canCreateServer: can("server.create"),
    canControlServer: can("server.control"),
    canEditServerConfig: can("server.configure"),
    canDeleteServer: can("server.delete"),
    canManageBackups: can("backup.manage"),
    canManageWorlds: can("world.manage"),
    canManageMods: can("mod.manage"),
    canManagePlayers: can("player.manage"),
    canManageShares: can("share.manage"),
    canManageNodes: can("node.manage"),
    canManageTeam: can("team.manage"),
    canEditSettings: can("settings.manage"),
    canManageSystem: can("system.manage"),
    canAccessGameAssets: !isViewer
  };
}
