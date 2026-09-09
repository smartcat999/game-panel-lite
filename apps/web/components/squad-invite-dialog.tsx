"use client";

import React, { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Users,
  UserPlus,
  Link2,
  Copy,
  Check,
  Trash2,
  Clock,
  Shield,
  Sparkles,
  X,
  AlertCircle,
  Ban,
  Building2
} from "lucide-react";
import {
  listOrganizations,
  listOrganizationMembers,
  removeOrganizationMember,
  createOrganizationInvitation,
  listOrganizationInvitations,
  revokeOrganizationInvitation
} from "@/lib/api";
import type { OrganizationInvitation } from "@/lib/types";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui";

interface SquadInviteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultOrgId?: string;
}

export function SquadInviteDialog({ open, onOpenChange, defaultOrgId }: SquadInviteDialogProps) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const queryClient = useQueryClient();

  const [activeTab, setActiveTab] = useState<"create" | "links" | "members">("create");
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  // Invitation creation form state
  const [role, setRole] = useState<"member" | "viewer">("member");
  const [expiresInHours, setExpiresInHours] = useState<number>(168); // 7 days
  const [maxUses, setMaxUses] = useState<number>(5);
  const [createdInvite, setCreatedInvite] = useState<OrganizationInvitation | null>(null);
  const [createError, setCreateError] = useState<string | null>(null);

  // Query organizations to know which org to use
  const { data: orgs = [] } = useQuery({
    queryKey: ["organizations"],
    queryFn: listOrganizations,
    enabled: open
  });

  const [selectedOrgId, setSelectedOrgId] = useState<string>(defaultOrgId || "");

  useEffect(() => {
    if (!selectedOrgId && orgs.length > 0) {
      setSelectedOrgId(defaultOrgId && orgs.some((o) => o.id === defaultOrgId) ? defaultOrgId : (orgs[0]?.id || ""));
    }
  }, [orgs, defaultOrgId, selectedOrgId]);

  const currentOrg = orgs.find((o) => o.id === selectedOrgId) || orgs[0];
  const orgId = currentOrg?.id || "";

  // Query active invitations
  const { data: invitations = [], isLoading: isInvitesLoading } = useQuery({
    queryKey: ["organization-invitations", orgId],
    queryFn: () => listOrganizationInvitations(orgId),
    enabled: open && Boolean(orgId)
  });

  // Query members
  const { data: members = [], isLoading: isMembersLoading } = useQuery({
    queryKey: ["organization-members", orgId],
    queryFn: () => listOrganizationMembers(orgId),
    enabled: open && Boolean(orgId)
  });

  // Create invite mutation
  const createMutation = useMutation({
    mutationFn: () =>
      createOrganizationInvitation(orgId, {
        role,
        expiresInHours,
        maxUses
      }),
    onSuccess: (invitation) => {
      setCreatedInvite(invitation);
      setCreateError(null);
      queryClient.invalidateQueries({ queryKey: ["organization-invitations", orgId] });
    },
    onError: (err: Error) => {
      setCreateError(err.message || "Failed to create invitation");
    }
  });

  // Revoke invite mutation
  const revokeMutation = useMutation({
    mutationFn: (invitationId: string) => revokeOrganizationInvitation(orgId, invitationId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organization-invitations", orgId] });
    }
  });

  // Remove member mutation
  const removeMemberMutation = useMutation({
    mutationFn: (userId: string) => removeOrganizationMember(orgId, userId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organization-members", orgId] });
    }
  });

  if (!open) return null;

  const handleCopy = (text: string, key: string) => {
    navigator.clipboard.writeText(text);
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(null), 2000);
  };

  const origin = typeof window !== "undefined" ? window.location.origin : "";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 backdrop-blur-sm p-4">
      <div className="w-full max-w-xl rounded-2xl border border-slate-800 bg-slate-900 p-6 shadow-2xl space-y-5">
        {/* Header */}
        <div className="flex items-start justify-between border-b border-slate-800/80 pb-4">
          <div className="flex items-center gap-3">
            <div className="flex size-9 items-center justify-center rounded-xl border border-panel-green/40 bg-panel-green/10 text-panel-green">
              <Users className="size-5" />
            </div>
            <div>
              <h3 className="text-sm font-bold text-white flex items-center gap-2">
                <span>{isZh ? "战队开黑与成员免密邀请" : "Squad Collaboration & Invites"}</span>
                {currentOrg && (
                  <span className="rounded bg-slate-800 px-2 py-0.5 text-[11px] font-mono text-slate-300">
                    {currentOrg.name}
                  </span>
                )}
              </h3>
              <p className="mt-0.5 text-xs text-slate-400">
                {isZh
                  ? "生成专属邀请链接或口令，好友点击即可免密直接加入团队协作。"
                  : "Generate shareable invite links for friends to collaborate on your servers."}
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={() => onOpenChange(false)}
            className="text-slate-400 hover:text-white text-xs p-1"
          >
            <X className="size-4" />
          </button>
        </div>

        {/* Multi-Org Switcher if multiple orgs */}
        {orgs.length > 1 && (
          <div className="flex items-center gap-2 text-xs">
            <Building2 className="size-3.5 text-slate-400" />
            <span className="text-slate-400">{isZh ? "当前工作区:" : "Workspace:"}</span>
            <select
              value={selectedOrgId}
              onChange={(e) => setSelectedOrgId(e.target.value)}
              className="rounded-lg border border-slate-800 bg-slate-950 px-2.5 py-1 text-xs text-slate-200 outline-none focus:border-panel-green"
            >
              {orgs.map((o) => (
                <option key={o.id} value={o.id}>
                  {o.name}
                </option>
              ))}
            </select>
          </div>
        )}

        {/* Tab Navigation */}
        <div className="flex items-center gap-2 border-b border-slate-800 text-xs">
          <button
            type="button"
            onClick={() => setActiveTab("create")}
            className={cn(
              "flex items-center gap-1.5 border-b-2 px-3 py-2 font-medium transition",
              activeTab === "create"
                ? "border-panel-green text-panel-green font-semibold"
                : "border-transparent text-slate-400 hover:text-slate-200"
            )}
          >
            <UserPlus className="size-3.5" />
            <span>{isZh ? "生成邀请链接" : "Generate Link"}</span>
          </button>
          <button
            type="button"
            onClick={() => setActiveTab("links")}
            className={cn(
              "flex items-center gap-1.5 border-b-2 px-3 py-2 font-medium transition",
              activeTab === "links"
                ? "border-panel-green text-panel-green font-semibold"
                : "border-transparent text-slate-400 hover:text-slate-200"
            )}
          >
            <Link2 className="size-3.5" />
            <span>{isZh ? `活跃链接 (${invitations.length})` : `Active Links (${invitations.length})`}</span>
          </button>
          <button
            type="button"
            onClick={() => setActiveTab("members")}
            className={cn(
              "flex items-center gap-1.5 border-b-2 px-3 py-2 font-medium transition",
              activeTab === "members"
                ? "border-panel-green text-panel-green font-semibold"
                : "border-transparent text-slate-400 hover:text-slate-200"
            )}
          >
            <Shield className="size-3.5" />
            <span>{isZh ? `现有成员 (${members.length})` : `Members (${members.length})`}</span>
          </button>
        </div>

        {/* Tab 1: Create Invite */}
        {activeTab === "create" && (
          <div className="space-y-4 text-xs">
            {createError && (
              <div className="flex items-center gap-2 rounded-lg border border-red-500/30 bg-red-950/20 px-3 py-2 text-red-400">
                <AlertCircle className="size-3.5 shrink-0" />
                <span>{createError}</span>
              </div>
            )}

            {/* Role selection */}
            <div className="space-y-1.5">
              <label className="block text-[11px] font-medium text-slate-300">
                {isZh ? "加入后指派角色 (Permissions Role)" : "Assigned Role"}
              </label>
              <div className="grid grid-cols-2 gap-2">
                <button
                  type="button"
                  onClick={() => setRole("member")}
                  className={cn(
                    "flex flex-col items-start rounded-xl border p-3 text-left transition",
                    role === "member"
                      ? "border-panel-green bg-panel-green/10"
                      : "border-slate-800 bg-slate-950/60 hover:border-slate-700"
                  )}
                >
                  <div className="flex items-center gap-1.5 font-bold text-white">
                    <Shield className="size-3.5 text-panel-green" />
                    <span>{isZh ? "协同运维成员 (Member)" : "Member"}</span>
                  </div>
                  <p className="mt-1 text-[10px] text-slate-400 leading-relaxed">
                    {isZh
                      ? "推荐开黑服管：可启停服务器、备份存档、查看实时日志与控制台。"
                      : "Recommended for co-hosts: can start/stop servers and manage backups."}
                  </p>
                </button>

                <button
                  type="button"
                  onClick={() => setRole("viewer")}
                  className={cn(
                    "flex flex-col items-start rounded-xl border p-3 text-left transition",
                    role === "viewer"
                      ? "border-sky-400 bg-sky-500/10"
                      : "border-slate-800 bg-slate-950/60 hover:border-slate-700"
                  )}
                >
                  <div className="flex items-center gap-1.5 font-bold text-white">
                    <Shield className="size-3.5 text-sky-400" />
                    <span>{isZh ? "只读观察员 (Viewer)" : "Viewer"}</span>
                  </div>
                  <p className="mt-1 text-[10px] text-slate-400 leading-relaxed">
                    {isZh
                      ? "适合普通玩家：仅允许查看服务器在线状态、连接端口与历史记录。"
                      : "For regular players: read-only access to status and join info."}
                  </p>
                </button>
              </div>
            </div>

            {/* Expiration and usage limits */}
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="block text-[11px] font-medium text-slate-300">
                  <Clock className="inline size-3 mr-1 text-slate-400" />
                  {isZh ? "有效期限" : "Expiration"}
                </label>
                <select
                  value={expiresInHours}
                  onChange={(e) => setExpiresInHours(Number(e.target.value))}
                  className="w-full rounded-lg border border-slate-800 bg-slate-950 px-3 py-2 text-xs text-slate-200 outline-none focus:border-panel-green"
                >
                  <option value={24}>{isZh ? "24 小时 (1 天)" : "24 Hours"}</option>
                  <option value={168}>{isZh ? "7 天 (一周)" : "7 Days"}</option>
                  <option value={720}>{isZh ? "30 天 (一个月)" : "30 Days"}</option>
                </select>
              </div>

              <div className="space-y-1.5">
                <label className="block text-[11px] font-medium text-slate-300">
                  <Users className="inline size-3 mr-1 text-slate-400" />
                  {isZh ? "最大使用次数" : "Max Uses"}
                </label>
                <select
                  value={maxUses}
                  onChange={(e) => setMaxUses(Number(e.target.value))}
                  className="w-full rounded-lg border border-slate-800 bg-slate-950 px-3 py-2 text-xs text-slate-200 outline-none focus:border-panel-green"
                >
                  <option value={1}>{isZh ? "仅限 1 人使用 (一次性)" : "1 use (Single-use)"}</option>
                  <option value={5}>{isZh ? "最多 5 人加入" : "5 uses"}</option>
                  <option value={10}>{isZh ? "最多 10 人加入" : "10 uses"}</option>
                  <option value={0}>{isZh ? "无限制 (多人畅用)" : "Unlimited uses"}</option>
                </select>
              </div>
            </div>

            <Button
              type="button"
              disabled={createMutation.isPending || !orgId}
              onClick={() => createMutation.mutate()}
              className="w-full h-9 bg-panel-green text-slate-950 font-bold hover:bg-panel-green/90"
            >
              <Sparkles className="mr-1.5 size-3.5" />
              {createMutation.isPending
                ? (isZh ? "正在生成..." : "Generating...")
                : (isZh ? "生成免密邀请链接" : "Generate Invite Link")}
            </Button>

            {/* Display newly created invite */}
            {createdInvite && (
              <div className="rounded-xl border border-panel-green/40 bg-panel-green/5 p-3.5 space-y-2.5">
                <div className="flex items-center justify-between">
                  <span className="font-bold text-panel-green flex items-center gap-1.5">
                    <Check className="size-3.5" />
                    {isZh ? "邀请链接已成功生成！" : "Invite Link Generated!"}
                  </span>
                  <span className="text-[11px] text-slate-400 font-mono">
                    {isZh ? "角色:" : "Role:"} {createdInvite.role}
                  </span>
                </div>

                <div className="flex items-center gap-2">
                  <div className="flex-1 rounded-lg border border-slate-800 bg-slate-950/80 px-3 py-1.5 font-mono text-[11px] text-slate-200 truncate select-all">
                    {`${origin}/join?token=${createdInvite.token}`}
                  </div>
                  <Button
                    type="button"
                    onClick={() => handleCopy(`${origin}/join?token=${createdInvite.token}`, "created-link")}
                    className="h-8 text-xs bg-panel-green text-slate-950 font-bold hover:bg-panel-green/90 shrink-0"
                  >
                    {copiedKey === "created-link" ? (
                      <>
                        <Check className="mr-1 size-3.5" />
                        {isZh ? "已复制" : "Copied"}
                      </>
                    ) : (
                      <>
                        <Copy className="mr-1 size-3.5" />
                        {isZh ? "复制链接" : "Copy Link"}
                      </>
                    )}
                  </Button>
                </div>

                <p className="text-[10px] text-slate-400">
                  {isZh
                    ? `将链接发送至好友，好友在浏览器中打开即可加入组织。链接有效期至 ${new Date(createdInvite.expiresAt).toLocaleString()}。`
                    : `Send this URL to your teammates to join. Expires at ${new Date(createdInvite.expiresAt).toLocaleString()}.`}
                </p>
              </div>
            )}
          </div>
        )}

        {/* Tab 2: Active Links List */}
        {activeTab === "links" && (
          <div className="space-y-3">
            {isInvitesLoading ? (
              <div className="py-6 text-center text-xs text-slate-500">
                {isZh ? "正在加载活跃邀请..." : "Loading invitations..."}
              </div>
            ) : invitations.length === 0 ? (
              <div className="rounded-xl border border-slate-800 bg-slate-950/40 p-6 text-center text-xs text-slate-500">
                {isZh ? "当前暂无活跃的邀请链接" : "No active invitation links."}
              </div>
            ) : (
              <div className="max-h-[300px] overflow-y-auto space-y-2 pr-1">
                {invitations.map((inv) => {
                  const joinUrl = `${origin}/join?token=${inv.token}`;
                  return (
                    <div
                      key={inv.id}
                      className="flex items-center justify-between rounded-xl border border-slate-800 bg-slate-950/60 p-3 text-xs"
                    >
                      <div className="space-y-1 min-w-0 pr-2">
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-slate-200 font-bold">
                            inv_{inv.token.substring(0, 8)}...
                          </span>
                          <span
                            className={cn(
                              "rounded px-1.5 py-0.2 text-[10px] font-bold uppercase",
                              inv.role === "member"
                                ? "bg-panel-green/15 text-panel-green"
                                : "bg-sky-500/15 text-sky-400"
                            )}
                          >
                            {inv.role}
                          </span>
                        </div>
                        <div className="flex items-center gap-3 text-[10px] text-slate-400 font-mono">
                          <span>
                            {isZh ? "已用:" : "Used:"} {inv.usedCount} / {inv.maxUses === 0 ? "∞" : inv.maxUses}
                          </span>
                          <span>·</span>
                          <span>
                            {isZh ? "过期:" : "Exp:"} {new Date(inv.expiresAt).toLocaleDateString()}
                          </span>
                        </div>
                      </div>

                      <div className="flex items-center gap-1.5 shrink-0">
                        <button
                          type="button"
                          onClick={() => handleCopy(joinUrl, `copy-${inv.id}`)}
                          title={isZh ? "复制链接" : "Copy link"}
                          className="flex items-center gap-1 rounded px-2 py-1 text-[11px] font-medium text-slate-300 hover:bg-slate-800 hover:text-white transition"
                        >
                          {copiedKey === `copy-${inv.id}` ? (
                            <Check className="size-3 text-panel-green" />
                          ) : (
                            <Copy className="size-3" />
                          )}
                          <span>{copiedKey === `copy-${inv.id}` ? (isZh ? "已复制" : "Copied") : (isZh ? "复制" : "Copy")}</span>
                        </button>
                        <button
                          type="button"
                          disabled={revokeMutation.isPending}
                          onClick={() => revokeMutation.mutate(inv.id)}
                          title={isZh ? "作废此链接" : "Revoke link"}
                          className="rounded p-1 text-slate-500 hover:bg-rose-950/40 hover:text-rose-400 transition"
                        >
                          <Ban className="size-3.5" />
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}

        {/* Tab 3: Squad Members */}
        {activeTab === "members" && (
          <div className="space-y-3">
            {isMembersLoading ? (
              <div className="py-6 text-center text-xs text-slate-500">
                {isZh ? "正在加载成员列表..." : "Loading members..."}
              </div>
            ) : members.length === 0 ? (
              <div className="rounded-xl border border-slate-800 bg-slate-950/40 p-6 text-center text-xs text-slate-500">
                {isZh ? "暂无成员" : "No members found."}
              </div>
            ) : (
              <div className="max-h-[300px] overflow-y-auto space-y-2 pr-1">
                {members.map((member) => (
                  <div
                    key={member.id}
                    className="flex items-center justify-between rounded-xl border border-slate-800 bg-slate-950/60 p-3 text-xs"
                  >
                    <div className="flex items-center gap-2.5 min-w-0">
                      <div className="flex size-7 items-center justify-center rounded-lg bg-slate-800 text-slate-300 font-mono text-xs font-bold shrink-0">
                        {member.userId.substring(0, 2).toUpperCase()}
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-slate-200 font-bold truncate">
                            {member.userId}
                          </span>
                          <span
                            className={cn(
                              "rounded px-1.5 py-0.2 text-[10px] font-bold uppercase",
                              member.role === "owner"
                                ? "bg-panel-gold/15 text-panel-gold"
                                : member.role === "admin"
                                ? "bg-purple-500/15 text-purple-400"
                                : member.role === "member"
                                ? "bg-panel-green/15 text-panel-green"
                                : "bg-slate-800 text-slate-400"
                            )}
                          >
                            {member.role}
                          </span>
                        </div>
                        <div className="text-[10px] text-slate-500 font-mono">
                          {isZh ? "加入时间: " : "Joined: "}
                          {new Date(member.createdAt).toLocaleDateString()}
                        </div>
                      </div>
                    </div>

                    {member.role !== "owner" && (
                      <button
                        type="button"
                        disabled={removeMemberMutation.isPending}
                        onClick={() => removeMemberMutation.mutate(member.userId)}
                        title={isZh ? "移出团队" : "Remove member"}
                        className="rounded p-1 text-slate-500 hover:bg-rose-950/40 hover:text-rose-400 transition"
                      >
                        <Trash2 className="size-3.5" />
                      </button>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
