"use client";

import { useAuthBootstrap } from "./auth-session";
import type { Permission, UserRole } from "./types";

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

  const account = authQuery.data?.account;
  const role: UserRole = account?.role ?? (authQuery.data?.initialized === false ? "admin" : "viewer");
  const platformRole = account?.platformRole ?? (authQuery.data?.initialized === false ? "platform_admin" : "user");
  const permissions = new Set<Permission>(account?.permissions ?? permissionsForRole(role));
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
