"use client";

import { usePathname } from "next/navigation";
import { useAuthBootstrap } from "./auth-session";
import type { Permission, PlatformRole, UserRole } from "./types";
import { consoleSurfaceForPathname } from "./console-routing";
import { useTenantContext } from "./tenant-context";

const rolePermissions: Record<UserRole, readonly Permission[]> = {
  admin: [
    "server.view", "server.create", "server.control", "server.configure", "server.delete",
    "backup.manage", "world.manage", "mod.manage", "player.manage", "share.manage",
    "team.manage", "settings.manage"
  ],
  member: [
    "server.view", "server.create", "server.control", "server.configure",
    "backup.manage", "world.manage", "mod.manage", "player.manage", "share.manage"
  ],
  viewer: ["server.view"]
};

const platformAdminPermissions: readonly Permission[] = [
  ...rolePermissions.admin,
  "node.manage",
  "system.manage"
];

export function permissionsForRole(role: UserRole): readonly Permission[] {
  return rolePermissions[role];
}

export function permissionsForPlatformRole(role: PlatformRole): readonly Permission[] {
  return role === "platform_admin" ? platformAdminPermissions : [];
}

export function usePermissions() {
  const authQuery = useAuthBootstrap();
  const consoleSurface = consoleSurfaceForPathname(usePathname());
  const { currentOrganization } = useTenantContext();

  const account = authQuery.data?.account;
  const platformRole = account?.platformRole ?? (authQuery.data?.initialized === false ? "platform_admin" : "user");
  const membershipRole = currentOrganization?.membershipRole;
  const tenantRole: UserRole = membershipRole === "owner" || membershipRole === "admin"
    ? "admin"
    : membershipRole === "member" ? "member" : "viewer";
  const platformPermissions = permissionsForPlatformRole(platformRole);
  const role: UserRole = consoleSurface === "platform" && platformRole === "platform_admin"
    ? "admin"
    : consoleSurface === "tenant" ? tenantRole : "viewer";
  const scopedPermissions = consoleSurface === "platform"
    ? platformPermissions
    : consoleSurface === "tenant" ? permissionsForRole(tenantRole) : [];
  const permissions = new Set<Permission>(consoleSurface === "platform" ? account?.permissions ?? scopedPermissions : scopedPermissions);
  const can = (permission: Permission) => permissions.has(permission);

  const isViewer = role === "viewer";
  const isMember = role === "member";
  const isAdmin = consoleSurface === "platform"
    ? platformRole === "platform_admin"
    : consoleSurface === "tenant" && tenantRole === "admin";

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
