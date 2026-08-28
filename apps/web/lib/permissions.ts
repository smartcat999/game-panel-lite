"use client";

import { useQuery } from "@tanstack/react-query";
import { getAuthBootstrap } from "./api";

export function usePermissions() {
  const authQuery = useQuery({
    queryKey: ["auth-bootstrap"],
    queryFn: getAuthBootstrap,
    staleTime: 60000,
    retry: false
  });

  const account = authQuery.data?.account;
  // 如果没有 account（例如单机未启用登录）默认为 admin，如果有 account 则根据其实际 role
  const role = account ? account.role : "admin";

  const isViewer = role === "viewer";
  const isMember = role === "member";
  const isAdmin = role === "admin" || (role as string) === "owner";

  return {
    account,
    role,
    isViewer,
    isMember,
    isAdmin,
    isLoading: authQuery.isLoading,
    // 细粒度权限判定
    canCreateServer: !isViewer,
    canControlServer: !isViewer, // 启动、停止、重启
    canEditServerConfig: !isViewer, // 编辑房间参数、Mod、世界
    canDeleteServer: !isViewer && !isMember, // 仅 Admin 可删除
    canManageBackups: !isViewer, // 创建/恢复/删除备份
    canManageWorlds: !isViewer, // 新建/重置世界
    canManageMods: !isViewer, // 安装/卸载 Mod
    canManageNodes: isAdmin, // 接入/编辑/删除节点
    canManageTeam: isAdmin, // 添加/移除成员、修改权限
    canEditSettings: isAdmin // 控制台全局配置、HTTPS、更新
  };
}
