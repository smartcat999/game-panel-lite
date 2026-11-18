"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Eye,
  KeyRound,
  Shield,
  ShieldCheck,
  Trash2,
  UserCheck,
  UserPlus,
  Users
} from "lucide-react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { useToast } from "@/components/toast-context";
import { Button, Input } from "@/components/ui";
import {
  createUser,
  deleteUser,
  getAuthBootstrap,
  listUsers,
  resetUserPassword,
  updateRegistrationSetting,
  updateUserRole
} from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { usePermissions } from "@/lib/permissions";
import type { UserAccount, UserRole } from "@/lib/types";

export function UserManagement() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const toast = useToast();
  const queryClient = useQueryClient();
  const { canManageTeam } = usePermissions();

  const [isAddOpen, setIsAddOpen] = useState(false);
  const [newUsername, setNewUsername] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [newRole, setNewRole] = useState<UserRole>("member");

  const [pendingResetUser, setPendingResetUser] = useState<UserAccount | null>(null);
  const [resetPasswordVal, setResetPasswordVal] = useState("");

  const [pendingDeleteUser, setPendingDeleteUser] = useState<UserAccount | null>(null);

  const authQuery = useQuery({
    queryKey: ["auth-bootstrap"],
    queryFn: getAuthBootstrap
  });

  const usersQuery = useQuery({
    queryKey: ["users"],
    queryFn: listUsers
  });

  const currentUser = authQuery.data?.account;
  const allowRegistration = authQuery.data?.allowRegistration ?? false;
  const users = usersQuery.data ?? [];

  const regMutation = useMutation({
    mutationFn: (allowed: boolean) => updateRegistrationSetting(allowed),
    onSuccess: async (data) => {
      toast.success(
        isZh ? "公开注册设置已更新" : "Registration setting updated",
        data.allowRegistration
          ? isZh ? "已开放新用户公开注册功能" : "Public registration enabled."
          : isZh ? "已关闭公开注册，仅管理员可创建账号" : "Public registration disabled."
      );
      await queryClient.invalidateQueries({ queryKey: ["auth-bootstrap"] });
    },
    onError: (err) => {
      toast.error(isZh ? "更新注册设置失败" : "Failed to update setting", err instanceof Error ? err.message : "");
    }
  });

  const createMutation = useMutation({
    mutationFn: () => createUser(newUsername, newPassword, newRole),
    onSuccess: async () => {
      toast.success(isZh ? "成员创建成功" : "User created");
      setIsAddOpen(false);
      setNewUsername("");
      setNewPassword("");
      setNewRole("member");
      await queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (err) => {
      toast.error(isZh ? "创建失败" : "Failed to create user", err instanceof Error ? err.message : "");
    }
  });

  const roleMutation = useMutation({
    mutationFn: ({ id, role }: { id: string; role: UserRole }) => updateUserRole(id, role),
    onSuccess: async () => {
      toast.success(isZh ? "用户权限已更新" : "Role updated");
      await queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (err) => {
      toast.error(isZh ? "修改权限失败" : "Failed to update role", err instanceof Error ? err.message : "");
    }
  });

  const resetPasswordMutation = useMutation({
    mutationFn: () => {
      if (!pendingResetUser) throw new Error("No user selected");
      return resetUserPassword(pendingResetUser.id, resetPasswordVal);
    },
    onSuccess: async () => {
      toast.success(isZh ? "密码重置成功" : "Password reset successfully");
      setPendingResetUser(null);
      setResetPasswordVal("");
    },
    onError: (err) => {
      toast.error(isZh ? "密码重置失败" : "Failed to reset password", err instanceof Error ? err.message : "");
    }
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteUser(id),
    onSuccess: async () => {
      toast.success(isZh ? "用户已删除" : "User deleted");
      setPendingDeleteUser(null);
      await queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (err) => {
      toast.error(isZh ? "删除失败" : "Failed to delete user", err instanceof Error ? err.message : "");
    }
  });

  const getRoleBadge = (role: UserRole) => {
    switch (role) {
      case "admin":
        return (
          <span className="inline-flex items-center gap-1 rounded bg-panel-green/15 border border-panel-green/30 px-2 py-0.5 text-[10px] font-bold text-panel-green">
            <ShieldCheck className="size-3" />
            {isZh ? "管理员 Admin" : "Admin"}
          </span>
        );
      case "member":
        return (
          <span className="inline-flex items-center gap-1 rounded bg-sky-500/15 border border-sky-500/30 px-2 py-0.5 text-[10px] font-bold text-sky-400">
            <UserCheck className="size-3" />
            {isZh ? "开黑成员 Member" : "Member"}
          </span>
        );
      case "viewer":
        return (
          <span className="inline-flex items-center gap-1 rounded bg-slate-800 border border-slate-700 px-2 py-0.5 text-[10px] font-bold text-slate-400">
            <Eye className="size-3" />
            {isZh ? "访客 Viewer" : "Viewer"}
          </span>
        );
      default:
        return (
          <span className="inline-flex items-center gap-1 rounded bg-slate-800 px-2 py-0.5 text-[10px] font-bold text-slate-400">
            {role}
          </span>
        );
    }
  };

  return (
    <div className="space-y-6">
      <div className="grid gap-3 md:grid-cols-3">
        {[
          {
            icon: ShieldCheck,
            title: isZh ? "管理员" : "Administrator",
            description: isZh ? "服务器、用户、节点、全局设置、版本更新与删除权限。" : "Full server, user, node, settings, release, and deletion access.",
            tone: "border-purple-500/25 bg-purple-500/5 text-purple-300"
          },
          {
            icon: UserCheck,
            title: isZh ? "运维成员" : "Operator",
            description: isZh ? "可创建、配置和启停服务器，管理模组、玩家、世界与备份；不能删除实例或修改系统。" : "Can operate servers and manage game assets, but cannot delete instances or change the system.",
            tone: "border-sky-500/25 bg-sky-500/5 text-sky-300"
          },
          {
            icon: Eye,
            title: isZh ? "只读访客" : "Viewer",
            description: isZh ? "仅查看仪表盘、服务器大厅、运行状态和加入信息，不显示维护入口。" : "Can only view dashboards, server lobbies, status, and join information.",
            tone: "border-slate-700 bg-slate-900/60 text-slate-300"
          }
        ].map(({ description, icon: Icon, title, tone }) => (
          <div className={`rounded-xl border p-4 ${tone}`} key={title}>
            <div className="flex items-center gap-2 text-sm font-semibold"><Icon aria-hidden="true" className="size-4" />{title}</div>
            <p className="mt-2 text-xs leading-5 text-slate-400">{description}</p>
          </div>
        ))}
      </div>

      {/* 1. Public Registration Security Policy */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 rounded-xl border border-slate-800 bg-slate-900/60 p-5">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <Shield className="size-4 text-panel-gold" />
            <h3 className="text-sm font-bold text-white tracking-tight">
              {isZh ? "公开用户注册安全策略" : "Public User Registration"}
            </h3>
            <span className="rounded bg-slate-800 px-1.5 py-0.5 text-[10px] font-mono text-slate-400">
              {allowRegistration ? (isZh ? "已开放" : "Enabled") : (isZh ? "默认关闭" : "Disabled")}
            </span>
          </div>
          <p className="text-xs text-slate-400 max-w-2xl leading-relaxed">
            {isZh
              ? "默认关闭以防止公网恶意注册。开启后，外部玩家可直接在登录页自行注册账号（默认分配开黑成员权限）。"
              : "Disabled by default. When enabled, users can self-register from the login page."}
          </p>
        </div>

        <div className="flex items-center gap-3 shrink-0">
          <label className="relative inline-flex items-center cursor-pointer">
            <input
              type="checkbox"
              checked={allowRegistration}
              disabled={regMutation.isPending}
              onChange={(e) => regMutation.mutate(e.target.checked)}
              className="sr-only peer"
            />
            <div className="w-11 h-6 bg-slate-800 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-panel-green"></div>
          </label>
          <span className="text-xs font-bold text-slate-300">
            {allowRegistration ? (isZh ? "允许公开注册" : "Allow Registration") : (isZh ? "禁止公开注册" : "Strict Mode")}
          </span>
        </div>
      </div>

      {/* 2. User Accounts Table & Actions */}
      <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="flex items-center gap-2">
            <Users className="size-4 text-panel-green" />
            <h3 className="text-sm font-bold text-white tracking-tight">
              {isZh ? "面板团队成员与权限管理" : "Team Accounts & RBAC"}
            </h3>
            <span className="rounded bg-panel-green/15 px-2 py-0.5 text-[10px] font-mono font-bold text-panel-green">
              {users.length} {isZh ? "位用户" : "Users"}
            </span>
          </div>

          {canManageTeam && (
            <Button
              type="button"
              onClick={() => setIsAddOpen(true)}
              className="gap-2 shrink-0 h-9 px-4 text-xs font-bold"
            >
              <UserPlus className="size-3.5" />
              <span>{isZh ? "添加新成员" : "Add Member"}</span>
            </Button>
          )}
        </div>

        {/* Table */}
        <div className="overflow-x-auto rounded-lg border border-slate-800 bg-slate-950/60">
          <table className="w-full text-left text-xs text-slate-300">
            <thead className="border-b border-slate-800 bg-slate-900/80 text-[11px] font-bold uppercase tracking-wider text-slate-400">
              <tr>
                <th className="px-4 py-3">{isZh ? "用户名" : "Username"}</th>
                <th className="px-4 py-3">{isZh ? "角色与权限" : "Role"}</th>
                <th className="px-4 py-3">{isZh ? "注册时间" : "Created"}</th>
                {canManageTeam && <th className="px-4 py-3 text-right">{isZh ? "操作" : "Actions"}</th>}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/80 font-mono">
              {usersQuery.isLoading ? (
                <tr>
                  <td colSpan={4} className="px-4 py-8 text-center text-slate-500 font-sans">
                    {isZh ? "正在加载用户列表..." : "Loading users..."}
                  </td>
                </tr>
              ) : users.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-4 py-8 text-center text-slate-500 font-sans">
                    {isZh ? "暂无其他用户" : "No users found"}
                  </td>
                </tr>
              ) : (
                users.map((u) => {
                  const isSelf = currentUser?.id === u.id;
                  const dateStr = u.createdAt ? new Date(u.createdAt).toLocaleDateString(isZh ? "zh-CN" : "en-US") : "-";

                  return (
                    <tr key={u.id} className="hover:bg-slate-900/50 transition">
                      <td className="px-4 py-3.5 font-bold text-white font-sans flex items-center gap-2">
                        <div className="flex size-7 items-center justify-center rounded-md bg-slate-800 text-[11px] font-bold text-slate-300">
                          {u.username.slice(0, 2).toUpperCase()}
                        </div>
                        <span>{u.username}</span>
                        {isSelf && (
                          <span className="rounded bg-slate-800 px-1.5 py-0.5 text-[9px] font-bold text-panel-green">
                            {isZh ? "当前账号" : "You"}
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-3.5 font-sans">
                        {isSelf ? (
                          getRoleBadge(u.role)
                        ) : (
                          <select
                            value={u.role}
                            onChange={(e) => roleMutation.mutate({ id: u.id, role: e.target.value as UserRole })}
                            className="rounded-lg border border-slate-800 bg-slate-950 px-2.5 py-1 text-xs text-slate-200 focus:border-panel-green focus:outline-none cursor-pointer"
                          >
                            <option value="admin">{isZh ? "管理员 (Admin)" : "Admin"}</option>
                            <option value="member">{isZh ? "开黑成员 (Member)" : "Member"}</option>
                            <option value="viewer">{isZh ? "只读访客 (Viewer)" : "Viewer"}</option>
                          </select>
                        )}
                      </td>
                      <td className="px-4 py-3.5 text-slate-400 text-[11px]">{dateStr}</td>
                      {canManageTeam && (
                        <td className="px-4 py-3.5 text-right font-sans">
                          <div className="flex items-center justify-end gap-2">
                            <button
                              type="button"
                              onClick={() => {
                                setPendingResetUser(u);
                                setResetPasswordVal("");
                              }}
                              title={isZh ? "重置该用户密码" : "Reset Password"}
                              className="flex items-center gap-1 rounded-md border border-slate-800 bg-slate-900 px-2.5 py-1 text-xs text-slate-300 hover:text-white hover:border-slate-700 transition"
                            >
                              <KeyRound className="size-3 text-sky-400" />
                              <span>{isZh ? "重置密码" : "Reset"}</span>
                            </button>

                            {!isSelf && (
                              <button
                                type="button"
                                onClick={() => setPendingDeleteUser(u)}
                                title={isZh ? "删除该用户" : "Delete User"}
                                className="flex size-7 items-center justify-center rounded-md border border-slate-800 bg-slate-900 text-slate-400 hover:text-rose-400 hover:border-rose-500/40 transition"
                              >
                                <Trash2 className="size-3.5" />
                              </button>
                            )}
                          </div>
                        </td>
                      )}
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Modal: Add User */}
      {isAddOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-4 backdrop-blur-xs">
          <div className="w-full max-w-md rounded-2xl border border-slate-800 bg-slate-950 p-6 shadow-2xl space-y-5 animate-in fade-in zoom-in-95 duration-150">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3.5">
              <div className="flex items-center gap-2">
                <UserPlus className="size-4 text-panel-green" />
                <h3 className="text-sm font-bold text-white tracking-tight">
                  {isZh ? "添加新用户" : "Add Member"}
                </h3>
              </div>
              <button
                type="button"
                onClick={() => setIsAddOpen(false)}
                className="flex size-7 items-center justify-center rounded-lg text-slate-400 hover:text-white hover:bg-slate-900 transition text-xs"
              >
                ✕
              </button>
            </div>

            <form
              onSubmit={(e) => {
                e.preventDefault();
                createMutation.mutate();
              }}
              className="space-y-4"
            >
              <div className="space-y-1.5">
                <label className="block text-xs font-semibold text-slate-300">
                  {isZh ? "用户名" : "Username"}
                </label>
                <Input
                  required
                  value={newUsername}
                  onChange={(e) => setNewUsername(e.target.value)}
                  placeholder={isZh ? "输入3-32位用户名" : "Username"}
                  className="w-full h-10 bg-slate-900 border-slate-800 focus:border-panel-green text-xs rounded-xl px-3"
                />
              </div>

              <div className="space-y-1.5">
                <label className="block text-xs font-semibold text-slate-300">
                  {isZh ? "初始密码" : "Initial Password"}
                </label>
                <Input
                  required
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  placeholder={isZh ? "至少8位字符" : "At least 8 characters"}
                  className="w-full h-10 bg-slate-900 border-slate-800 focus:border-panel-green text-xs rounded-xl px-3"
                />
              </div>

              <div className="space-y-1.5">
                <label className="block text-xs font-semibold text-slate-300">
                  {isZh ? "分配角色与权限" : "Role"}
                </label>
                <select
                  value={newRole}
                  onChange={(e) => setNewRole(e.target.value as UserRole)}
                  className="w-full h-10 rounded-xl border border-slate-800 bg-slate-900 px-3 text-xs text-white focus:border-panel-green focus:outline-none cursor-pointer"
                >
                  <option value="member">{isZh ? "开黑成员 (Member) - 可操作与启停游戏服务器" : "Member - Can manage servers"}</option>
                  <option value="admin">{isZh ? "超级管理员 (Admin) - 拥有全局配置与用户管理权限" : "Admin - Full access"}</option>
                  <option value="viewer">{isZh ? "只读访客 (Viewer) - 仅可查看大厅与连接信息" : "Viewer - Read only"}</option>
                </select>
              </div>

              <div className="flex items-center justify-end gap-2.5 pt-3 border-t border-slate-800/80">
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => setIsAddOpen(false)}
                  className="h-9 px-4 text-xs font-medium text-slate-400 hover:text-white"
                >
                  {isZh ? "取消" : "Cancel"}
                </Button>
                <Button
                  type="submit"
                  disabled={createMutation.isPending || !newUsername || !newPassword}
                  className="h-9 px-5 text-xs font-bold"
                >
                  {createMutation.isPending ? (isZh ? "正在创建..." : "Creating...") : (isZh ? "确认创建" : "Create User")}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Modal: Reset Password */}
      {pendingResetUser && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-4 backdrop-blur-xs">
          <div className="w-full max-w-md rounded-2xl border border-slate-800 bg-slate-950 p-6 shadow-2xl space-y-5 animate-in fade-in zoom-in-95 duration-150">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3.5">
              <div className="flex items-center gap-2">
                <KeyRound className="size-4 text-sky-400" />
                <h3 className="text-sm font-bold text-white tracking-tight">
                  {isZh ? `重置用户【${pendingResetUser.username}】密码` : `Reset Password for ${pendingResetUser.username}`}
                </h3>
              </div>
              <button
                type="button"
                onClick={() => setPendingResetUser(null)}
                className="flex size-7 items-center justify-center rounded-lg text-slate-400 hover:text-white hover:bg-slate-900 transition text-xs"
              >
                ✕
              </button>
            </div>

            <form
              onSubmit={(e) => {
                e.preventDefault();
                resetPasswordMutation.mutate();
              }}
              className="space-y-4"
            >
              <div className="space-y-1.5">
                <label className="block text-xs font-semibold text-slate-300">
                  {isZh ? "新密码" : "New Password"}
                </label>
                <Input
                  required
                  type="password"
                  value={resetPasswordVal}
                  onChange={(e) => setResetPasswordVal(e.target.value)}
                  placeholder={isZh ? "输入至少8位新密码" : "At least 8 characters"}
                  className="w-full h-10 bg-slate-900 border-slate-800 focus:border-panel-green text-xs rounded-xl px-3"
                />
              </div>

              <div className="flex items-center justify-end gap-2.5 pt-3 border-t border-slate-800/80">
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => setPendingResetUser(null)}
                  className="h-9 px-4 text-xs font-medium text-slate-400 hover:text-white"
                >
                  {isZh ? "取消" : "Cancel"}
                </Button>
                <Button
                  type="submit"
                  disabled={resetPasswordMutation.isPending || !resetPasswordVal}
                  className="h-9 px-5 text-xs font-bold"
                >
                  {resetPasswordMutation.isPending ? (isZh ? "正在重置..." : "Resetting...") : (isZh ? "确认重置密码" : "Reset Password")}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Confirm Delete Dialog */}
      <ConfirmDialog
        open={Boolean(pendingDeleteUser)}
        eyebrow={isZh ? "删除账号" : "Delete User"}
        eyebrowTone="gold"
        title={isZh ? `确认删除用户【${pendingDeleteUser?.username}】？` : `Confirm Delete User ${pendingDeleteUser?.username}?`}
        description={
          isZh
            ? "删除后该账号将无法再登录控制台，已授权的会话将被立即注销。"
            : "The user will immediately lose access to the panel."
        }
        cancelLabel={isZh ? "取消" : "Cancel"}
        confirmLabel={isZh ? "确认删除" : "Delete"}
        confirmVariant="danger"
        busy={deleteMutation.isPending}
        onConfirm={() => pendingDeleteUser && deleteMutation.mutate(pendingDeleteUser.id)}
        onCancel={() => setPendingDeleteUser(null)}
      />
    </div>
  );
}
