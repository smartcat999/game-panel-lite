"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2, Cpu, HardDrive, Settings2, Shield, Trash2, UserPlus, Users, X } from "lucide-react";
import { useState, type FormEvent } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Badge, Button, Card, Input } from "@/components/ui";
import {
  addOrganizationMember,
  getOrganizationUsage,
  listOrganizationMembers,
  listOrganizations,
  removeOrganizationMember,
  updateOrganizationQuota,
  type OrganizationMember
} from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export function TeamSettingsPanel() {
  const { locale } = useI18n();
  const isZh = locale === "zh";
  const queryClient = useQueryClient();
  const [selectedOrgId] = useState<string>("default-org");

  // Add Member State
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [newUserId, setNewUserId] = useState("");
  const [newRole, setNewRole] = useState<"owner" | "admin" | "member" | "viewer">("member");
  const [removingMember, setRemovingMember] = useState<OrganizationMember | null>(null);
  const [errorMsg, setErrorMsg] = useState("");

  // Edit Quota State
  const [quotaDialogOpen, setQuotaDialogOpen] = useState(false);
  const [maxServers, setMaxServers] = useState(10);
  const [maxCpu, setMaxCpu] = useState(16);
  const [maxRamGB, setMaxRamGB] = useState(32);
  const [quotaErrorMsg, setQuotaErrorMsg] = useState("");

  const orgsQuery = useQuery({
    queryKey: ["organizations"],
    queryFn: listOrganizations,
    retry: false
  });

  const orgs = orgsQuery.data ?? [];
  const currentOrg = orgs.find((o) => o.id === selectedOrgId) || orgs[0];

  const usageQuery = useQuery({
    queryKey: ["tenant-usage", currentOrg?.id],
    queryFn: () => getOrganizationUsage(currentOrg?.id || "default-org"),
    enabled: Boolean(currentOrg?.id)
  });

  const membersQuery = useQuery({
    queryKey: ["organization-members", currentOrg?.id],
    queryFn: () => listOrganizationMembers(currentOrg?.id || "default-org"),
    enabled: Boolean(currentOrg?.id)
  });

  const addMemberMutation = useMutation({
    mutationFn: () => addOrganizationMember(currentOrg?.id || "default-org", { userId: newUserId.trim(), role: newRole }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organization-members", currentOrg?.id] });
      setAddDialogOpen(false);
      setNewUserId("");
      setErrorMsg("");
    },
    onError: (err: Error) => setErrorMsg(err.message)
  });

  const removeMemberMutation = useMutation({
    mutationFn: (userId: string) => removeOrganizationMember(currentOrg?.id || "default-org", userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organization-members", currentOrg?.id] });
      setRemovingMember(null);
    }
  });

  const updateQuotaMutation = useMutation({
    mutationFn: () =>
      updateOrganizationQuota(currentOrg?.id || "default-org", {
        maxServers,
        maxCpuCores: maxCpu,
        maxMemoryMb: maxRamGB * 1024
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tenant-usage", currentOrg?.id] });
      setQuotaDialogOpen(false);
      setQuotaErrorMsg("");
    },
    onError: (err: Error) => setQuotaErrorMsg(err.message)
  });

  const usage = usageQuery.data;
  const members = membersQuery.data ?? [];

  const openQuotaModal = () => {
    if (usage?.quota) {
      setMaxServers(usage.quota.maxServers || 10);
      setMaxCpu(Math.round(usage.quota.maxCpuCores || 16));
      setMaxRamGB(Math.round((usage.quota.maxMemoryMb || 32768) / 1024));
    }
    setQuotaDialogOpen(true);
  };

  const handleAddSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!newUserId.trim()) return;
    addMemberMutation.mutate();
  };

  const handleQuotaSubmit = (e: FormEvent) => {
    e.preventDefault();
    updateQuotaMutation.mutate();
  };

  return (
    <div className="space-y-6">
      {/* Workspace Header & Quota Card */}
      <Card className="p-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3.5">
            <div className="flex size-12 items-center justify-center rounded-xl border border-panel-green/40 bg-panel-green/10 text-panel-green">
              <Building2 className="size-6" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h3 className="text-lg font-bold text-slate-100">{currentOrg?.name || (isZh ? "默认工作区" : "Default Workspace")}</h3>
                <Badge className="border border-panel-green/40 bg-panel-green/10 text-panel-green font-mono uppercase text-[10px]">
                  {currentOrg?.plan?.toUpperCase() || "PRO"} {isZh ? "套餐" : "PLAN"}
                </Badge>
              </div>
              <p className="text-xs text-slate-500 font-mono mt-0.5">
                Slug: {currentOrg?.slug || "default"} · ID: {currentOrg?.id || "default-org"}
              </p>
            </div>
          </div>

          <Button
            variant="secondary"
            onClick={openQuotaModal}
            className="flex items-center gap-1.5 h-8 px-3 text-xs self-start sm:self-auto"
          >
            <Settings2 className="size-3.5 text-panel-green" />
            {isZh ? "调整配额限制" : "Edit Resource Quotas"}
          </Button>
        </div>

        {/* Quotas grid */}
        {usage?.quota ? (
          <div className="mt-6 grid gap-4 sm:grid-cols-3 border-t border-panel-line/40 pt-5">
            <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-4">
              <div className="flex items-center justify-between text-xs text-slate-400">
                <span className="font-medium">{isZh ? "服务器实例配额" : "Servers Quota"}</span>
                <span className="font-mono text-slate-100 font-semibold">
                  {usage.totalServers} / {usage.quota.maxServers} {isZh ? "台" : "Servers"}
                </span>
              </div>
              <div className="mt-2.5 h-1.5 w-full overflow-hidden rounded-full bg-slate-800">
                <div
                  className="h-full rounded-full bg-panel-green transition-all"
                  style={{ width: `${Math.min(100, Math.round((usage.totalServers / (usage.quota.maxServers || 1)) * 100))}%` }}
                />
              </div>
            </div>

            <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-4">
              <div className="flex items-center justify-between text-xs text-slate-400">
                <span className="flex items-center gap-1 font-medium"><Cpu className="size-3.5 text-sky-400" /> {isZh ? "CPU 算力核心" : "CPU Cores"}</span>
                <span className="font-mono text-slate-100 font-semibold">
                  {usage.usedCpuCores.toFixed(1)} / {usage.quota.maxCpuCores} {isZh ? "核" : "Cores"}
                </span>
              </div>
              <div className="mt-2.5 h-1.5 w-full overflow-hidden rounded-full bg-slate-800">
                <div
                  className="h-full rounded-full bg-sky-400 transition-all"
                  style={{ width: `${Math.min(100, Math.round((usage.usedCpuCores / (usage.quota.maxCpuCores || 1)) * 100))}%` }}
                />
              </div>
            </div>

            <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-4">
              <div className="flex items-center justify-between text-xs text-slate-400">
                <span className="flex items-center gap-1 font-medium"><HardDrive className="size-3.5 text-purple-400" /> {isZh ? "内存总量 (RAM)" : "Memory (RAM)"}</span>
                <span className="font-mono text-slate-100 font-semibold">
                  {Math.round(usage.usedMemoryMb / 1024)}G / {Math.round(usage.quota.maxMemoryMb / 1024)}G
                </span>
              </div>
              <div className="mt-2.5 h-1.5 w-full overflow-hidden rounded-full bg-slate-800">
                <div
                  className="h-full rounded-full bg-purple-400 transition-all"
                  style={{ width: `${Math.min(100, Math.round((usage.usedMemoryMb / (usage.quota.maxMemoryMb || 1)) * 100))}%` }}
                />
              </div>
            </div>
          </div>
        ) : null}
      </Card>

      {/* Members Management Card */}
      <Card className="p-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h4 className="text-base font-semibold text-slate-100 flex items-center gap-2">
              <Users className="size-4 text-panel-green" /> {isZh ? "团队成员与 RBAC 权限" : "Team Members & RBAC"}
            </h4>
            <p className="text-xs text-slate-400 mt-1">
              {isZh ? "管理此工作区的组织成员与访问权限角色。" : "Manage members and assign access roles in this workspace."}
            </p>
          </div>
          <Button
            onClick={() => setAddDialogOpen(true)}
            className="flex items-center gap-1.5 h-8 px-3 text-xs self-start sm:self-auto"
          >
            <UserPlus className="size-3.5" /> {isZh ? "邀请新成员" : "Invite Member"}
          </Button>
        </div>

        <div className="mt-4 overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-panel-line text-slate-400">
                <th className="py-3 px-3 font-medium">{isZh ? "用户 / 账号" : "User / Account"}</th>
                <th className="py-3 px-3 font-medium">{isZh ? "角色" : "Role"}</th>
                <th className="py-3 px-3 font-medium">{isZh ? "加入时间" : "Joined At"}</th>
                <th className="py-3 px-3 font-medium text-right">{isZh ? "操作" : "Actions"}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-panel-line/40 text-slate-300">
              {members.length === 0 ? (
                <tr>
                  <td colSpan={4} className="py-6 text-center text-slate-500">
                    {isZh ? "此工作区暂无其他团队成员。" : "No other members in this workspace yet."}
                  </td>
                </tr>
              ) : (
                members.map((m) => (
                  <tr key={m.id} className="hover:bg-slate-900/40">
                    <td className="py-3 px-3 font-medium text-slate-100 font-mono">{m.userId}</td>
                    <td className="py-3 px-3">
                      <span
                        className={cn(
                          "inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-[11px] font-semibold uppercase",
                          m.role === "owner" && "bg-panel-gold/15 text-panel-gold border border-panel-gold/30",
                          m.role === "admin" && "bg-purple-500/15 text-purple-400 border border-purple-500/30",
                          m.role === "member" && "bg-sky-500/15 text-sky-400 border border-sky-500/30",
                          m.role === "viewer" && "bg-slate-800 text-slate-400 border border-slate-700"
                        )}
                      >
                        <Shield className="size-3" />
                        {m.role === "owner" ? (isZh ? "所有者" : "Owner") :
                         m.role === "admin" ? (isZh ? "管理员" : "Admin") :
                         m.role === "member" ? (isZh ? "运维成员" : "Member") : (isZh ? "只读观察员" : "Viewer")}
                      </span>
                    </td>
                    <td className="py-3 px-3 text-slate-400 font-mono">
                      {new Date(m.createdAt).toLocaleDateString()}
                    </td>
                    <td className="py-3 px-3 text-right">
                      {m.role !== "owner" ? (
                        <button
                          type="button"
                          onClick={() => setRemovingMember(m)}
                          className="rounded p-1 text-slate-500 hover:bg-rose-500/10 hover:text-rose-400 transition"
                          title={isZh ? "移除成员" : "Remove member"}
                        >
                          <Trash2 className="size-3.5" />
                        </button>
                      ) : null}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Add Member Dialog */}
      {addDialogOpen ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-4 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-xl border border-slate-700 bg-slate-900 p-6 shadow-2xl ring-1 ring-white/10">
            <div className="flex items-center justify-between">
              <h3 className="text-base font-semibold text-slate-100 flex items-center gap-2">
                <UserPlus className="size-4 text-panel-green" /> {isZh ? "添加团队成员" : "Add Team Member"}
              </h3>
              <button onClick={() => setAddDialogOpen(false)} className="text-slate-400 hover:text-white">
                <X className="size-4" />
              </button>
            </div>
            <p className="text-xs text-slate-400 mt-1">
              {isZh ? "输入现有账号 ID/用户名并为其指派 RBAC 访问权限角色。" : "Invite an existing user account to this organization."}
            </p>

            <form onSubmit={handleAddSubmit} className="mt-5 space-y-4">
              <div>
                <label className="text-xs font-medium text-slate-300">{isZh ? "用户 ID / 账号名称" : "User ID / Username"}</label>
                <Input
                  className="mt-1.5 w-full bg-slate-950/80 border-slate-700 focus:border-panel-green"
                  placeholder={isZh ? "例如 admin 或 用户标识" : "e.g. admin or user id"}
                  value={newUserId}
                  onChange={(e) => setNewUserId(e.target.value)}
                  autoFocus
                />
              </div>

              <div>
                <label className="text-xs font-medium text-slate-300">{isZh ? "分配角色与权限" : "RBAC Role"}</label>
                <select
                  value={newRole}
                  onChange={(e) => setNewRole(e.target.value as "owner" | "admin" | "member" | "viewer")}
                  className="mt-1.5 flex h-10 w-full rounded-md border border-slate-700 bg-slate-950/80 px-3 py-1 text-xs text-slate-200 shadow-sm focus:border-panel-green focus:outline-none"
                >
                  <option value="admin">{isZh ? "管理员（可创建/管理所有服务器与配置）" : "Admin (Manage servers & config)"}</option>
                  <option value="member">{isZh ? "运维成员（可启停/重启/查看控制台）" : "Member (Start, stop & console)"}</option>
                  <option value="viewer">{isZh ? "只读观察员（仅可查看大盘与状态）" : "Viewer (Read-only status)"}</option>
                </select>
              </div>

              {errorMsg ? <p className="text-xs text-rose-400">{errorMsg}</p> : null}

              <div className="flex justify-end gap-2.5 pt-3 border-t border-slate-800">
                <Button type="button" variant="secondary" className="h-9 px-4 text-xs" onClick={() => setAddDialogOpen(false)}>
                  {isZh ? "取消" : "Cancel"}
                </Button>
                <Button type="submit" variant="primary" disabled={addMemberMutation.isPending} className="h-9 px-4 text-xs">
                  {addMemberMutation.isPending ? (isZh ? "添加中..." : "Adding...") : (isZh ? "确认添加" : "Add Member")}
                </Button>
              </div>
            </form>
          </div>
        </div>
      ) : null}

      {/* Edit Quota Dialog */}
      {quotaDialogOpen ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-4 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-xl border border-slate-700 bg-slate-900 p-6 shadow-2xl ring-1 ring-white/10">
            <div className="flex items-center justify-between">
              <h3 className="text-base font-semibold text-slate-100 flex items-center gap-2">
                <Settings2 className="size-4 text-panel-green" /> {isZh ? "自定义调整租户配额" : "Edit Resource Quotas"}
              </h3>
              <button onClick={() => setQuotaDialogOpen(false)} className="text-slate-400 hover:text-white">
                <X className="size-4" />
              </button>
            </div>
            <p className="text-xs text-slate-400 mt-1">
              {isZh ? "配置当前租户的硬配额上限，防止超出物理服务器承载负荷。" : "Configure hard quota limits for this tenant."}
            </p>

            <form onSubmit={handleQuotaSubmit} className="mt-5 space-y-4">
              <div>
                <label className="text-xs font-medium text-slate-300">{isZh ? "最大服务器实例数 (台)" : "Max Servers Limit"}</label>
                <Input
                  type="number"
                  min={1}
                  max={100}
                  className="mt-1.5 w-full bg-slate-950/80 border-slate-700 focus:border-panel-green font-mono"
                  value={maxServers}
                  onChange={(e) => setMaxServers(parseInt(e.target.value, 10) || 1)}
                />
              </div>

              <div>
                <label className="text-xs font-medium text-slate-300">{isZh ? "最大 CPU 算力核心上限 (核)" : "Max CPU Cores Limit"}</label>
                <Input
                  type="number"
                  min={1}
                  max={128}
                  className="mt-1.5 w-full bg-slate-950/80 border-slate-700 focus:border-panel-green font-mono"
                  value={maxCpu}
                  onChange={(e) => setMaxCpu(parseFloat(e.target.value) || 1)}
                />
              </div>

              <div>
                <label className="text-xs font-medium text-slate-300">{isZh ? "最大内存池上限 (GB)" : "Max Memory Limit (GB)"}</label>
                <Input
                  type="number"
                  min={1}
                  max={1024}
                  className="mt-1.5 w-full bg-slate-950/80 border-slate-700 focus:border-panel-green font-mono"
                  value={maxRamGB}
                  onChange={(e) => setMaxRamGB(parseInt(e.target.value, 10) || 1)}
                />
              </div>

              {quotaErrorMsg ? <p className="text-xs text-rose-400">{quotaErrorMsg}</p> : null}

              <div className="flex justify-end gap-2.5 pt-3 border-t border-slate-800">
                <Button type="button" variant="secondary" className="h-9 px-4 text-xs" onClick={() => setQuotaDialogOpen(false)}>
                  {isZh ? "取消" : "Cancel"}
                </Button>
                <Button type="submit" variant="primary" disabled={updateQuotaMutation.isPending} className="h-9 px-4 text-xs">
                  {updateQuotaMutation.isPending ? (isZh ? "保存中..." : "Saving...") : (isZh ? "保存配额" : "Save Quotas")}
                </Button>
              </div>
            </form>
          </div>
        </div>
      ) : null}

      {/* Remove Confirm Dialog */}
      {removingMember ? (
        <ConfirmDialog
          open={Boolean(removingMember)}
          eyebrow={isZh ? "成员管理" : "Team Management"}
          eyebrowTone="gold"
          title={isZh ? "移除团队成员" : "Remove Member"}
          description={isZh ? `确定要将成员 ${removingMember.userId} 从当前工作区移除吗？` : `Are you sure you want to remove ${removingMember.userId} from this workspace?`}
          cancelLabel={isZh ? "取消" : "Cancel"}
          confirmLabel={isZh ? "确认移除" : "Remove Member"}
          confirmVariant="danger"
          onCancel={() => setRemovingMember(null)}
          onConfirm={() => removeMemberMutation.mutate(removingMember.userId)}
        />
      ) : null}
    </div>
  );
}
