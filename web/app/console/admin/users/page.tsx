"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Ban, CheckCircle2, ChevronDown, ChevronUp, Pencil, Plus, Search, ShieldCheck, Trash2 } from "lucide-react";
import { useConsole } from "../../ConsoleProvider";
import { errorMessage } from "../../lib/session";
import type { PolicyRule, UserItem } from "../../lib/types";
import { validateEmail, validateName } from "../../../components/console/admin/validation";
import { Avatar } from "../../../components/console/TopBar";
import {
  Badge,
  Button,
  Card,
  ConfirmDialog,
  EmptyState,
  Field,
  IconButton,
  Input,
  Modal,
  Mono,
  MoreMenu,
  PageHeader,
  Segmented,
  Select,
  Skeleton,
} from "../../../components/console/ui";

const ROLE_META: Record<string, { title: string; desc: string }> = {
  "platform-admin": {
    title: "平台管理员",
    desc: "拥有平台内所有集群级与工作区级资源的完整管理权限",
  },
  "node-agent": {
    title: "计算节点代理",
    desc: "供宿主机节点代理使用，用于上报节点状态、执行预热任务并同步调度到本节点的服务器",
  },
  "registry-worker": {
    title: "制品同步代理",
    desc: "供无状态制品同步工作节点使用，负责拉取游戏服务端与创意工坊模组并写入对象存储",
  },
  "authenticated-user": {
    title: "平台认证用户",
    desc: "所有已登录账号的基础权限，允许查看所属工作区列表、应用市场与制品仓库目录",
  },
};

const REMOVED_LITE_RESOURCES = new Set([
  "events",
  "alerts",
  "metering",
  "quotachanges",
  "resourceplans",
  "redeemcodes",
  "orders",
  "redeem",
  "reclaim",
  "logs",
  "users/messages",
]);

const RESOURCE_LABELS: Record<string, string> = {
  "*": "全部资源",
  nodes: "计算节点",
  "nodes/status": "节点状态",
  "nodes/metrics": "节点监控指标",
  "nodes/exec": "节点终端",
  prewarmjobs: "节点预热任务",
  "prewarmjobs/status": "预热任务状态",
  runtimeclasses: "运行环境",
  apps: "应用市场",
  tenants: "工作区",
  "tenants/status": "工作区状态",
  "tenants/invites": "工作区邀请",
  instances: "游戏服务器",
  "instances/*": "服务器全部子资源",
  "instances/status": "服务器状态",
  "instances/state": "服务器启停控制",
  "instances/exec": "服务器终端命令",
  "instances/log": "服务器与任务日志",
  "instances/files": "服务器文件与备份",
  "instances/metrics": "服务器监控指标",
  users: "平台用户",
  "users/status": "用户状态",
  sessions: "登录会话",
  globalroles: "全局角色",
  globalrolebindings: "全局角色绑定",
  tenantroles: "工作区角色",
  tenantrolebindings: "工作区成员绑定",
  upstreams: "上游镜像源",
  repositories: "制品仓库",
  "repositories/status": "仓库状态",
  packages: "制品包目录",
  "packages/icon": "制品图标",
  artifacts: "制品版本",
  "artifacts/status": "制品状态",
  "artifacts/log": "制品同步日志",
  blobs: "对象存储块",
  blobuploads: "对象上传会话",
  replicationpolicies: "同步策略",
};

function filterLiteRules(rules?: PolicyRule[]): PolicyRule[] {
  return (rules || [])
    .map((rule) => ({
      ...rule,
      resources: (rule.resources || []).filter((resKey) => !REMOVED_LITE_RESOURCES.has(resKey)),
    }))
    .filter((rule) => (rule.resources || []).length > 0);
}

const VERB_META: Record<string, { label: string; kind: "read" | "write" | "full" }> = {
  "*": { label: "全部操作", kind: "full" },
  get: { label: "读取详情", kind: "read" },
  list: { label: "列表查询", kind: "read" },
  watch: { label: "实时监听", kind: "read" },
  create: { label: "创建", kind: "write" },
  update: { label: "全量更新", kind: "write" },
  patch: { label: "局部更新", kind: "write" },
  delete: { label: "删除", kind: "full" },
};

function classifyRuleAccess(verbs?: string[]): {
  label: string;
  tone: "accent" | "neutral" | "warning";
} {
  const list = verbs || [];
  if (list.includes("*")) {
    return { label: "完全控制", tone: "accent" };
  }
  if (list.some((v) => v === "create" || v === "update" || v === "patch" || v === "delete")) {
    return { label: "读写变更", tone: "accent" };
  }
  return { label: "只读访问", tone: "neutral" };
}

const USER_STATE: Record<string, { label: string; tone: "success" | "warning" | "danger" | "neutral" }> = {
  Active: { label: "正常", tone: "success" },
  Disabled: { label: "已停用", tone: "neutral" },
  Locked: { label: "已锁定", tone: "danger" },
};

function summarizeRoleRules(rules?: PolicyRule[]): string {
  const list = filterLiteRules(rules);
  if (list.length === 0) return "0 条权限策略";
  const allResources = list.flatMap((r) => r.resources || []).filter(Boolean);
  if (allResources.includes("*")) {
    return `${list.length} 条策略 · 全部资源`;
  }
  const uniqueCount = new Set(allResources).size;
  return `${list.length} 条策略 · 覆盖 ${uniqueCount} 类资源`;
}

export default function AdminUsersPage() {
  const { users, globalRoles, globalRoleBindings, userName, loading, apiFetch, toast, reload } = useConsole();
  const [activeTab, setActiveTab] = useState<"users" | "roles">("users");

  useEffect(() => {
    if (typeof window === "undefined") return;
    const t = new URLSearchParams(window.location.search).get("tab");
    if (t === "roles") setActiveTab("roles");
  }, []);
  const [searchQuery, setSearchQuery] = useState("");
  const [roleFilter, setRoleFilter] = useState<"all" | "admin" | "member">("all");
  const [expandedRole, setExpandedRole] = useState<string | null>("node-agent");

  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [touched, setTouched] = useState(false);
  const [busy, setBusy] = useState(false);
  const [stateBusy, setStateBusy] = useState<string | null>(null);
  const [roleBusy, setRoleBusy] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<UserItem | null>(null);
  const [deleting, setDeleting] = useState(false);

  const [editUser, setEditUser] = useState<UserItem | null>(null);
  const [editDisplayName, setEditDisplayName] = useState("");
  const [editEmail, setEditEmail] = useState("");
  const [editNewPassword, setEditNewPassword] = useState("");
  const [editTouched, setEditTouched] = useState(false);
  const [editBusy, setEditBusy] = useState(false);

  const editEmailError = validateEmail(editEmail);
  const editPasswordError = editNewPassword && editNewPassword.length < 8 ? "新密码至少 8 位" : "";

  const visibleRoles = useMemo(
    () => globalRoles.filter((r) => !r.metadata.name.startsWith("user-self-")),
    [globalRoles],
  );

  const adminBindingsByUser = useCallback(
    (uname: string) =>
      globalRoleBindings.filter(
        (b) =>
          b.roleRef?.name === "platform-admin" &&
          (b.subjects || []).some((s) => s.kind === "User" && s.name === uname),
      ),
    [globalRoleBindings],
  );

  const filteredUsers = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    return users.filter((u) => {
      const uname = u.metadata.name;
      const isBuiltInAdmin = uname === "admin";
      const isPlatformAdmin = isBuiltInAdmin || adminBindingsByUser(uname).length > 0;
      if (roleFilter === "admin" && !isPlatformAdmin) return false;
      if (roleFilter === "member" && isPlatformAdmin) return false;
      if (!q) return true;
      const dname = (u.spec?.displayName || "").toLowerCase();
      const mail = (u.spec?.email || "").toLowerCase();
      return uname.toLowerCase().includes(q) || dname.includes(q) || mail.includes(q);
    });
  }, [users, searchQuery, roleFilter, adminBindingsByUser]);

  function openEditUser(u: UserItem) {
    setEditUser(u);
    setEditDisplayName(u.spec?.displayName || "");
    setEditEmail(u.spec?.email || "");
    setEditNewPassword("");
    setEditTouched(false);
  }

  const editUserDirty =
    !!editUser &&
    (editDisplayName.trim() !== (editUser.spec?.displayName || "").trim() ||
      editEmail.trim() !== (editUser.spec?.email || "").trim() ||
      editNewPassword.trim().length > 0);

  function closeEditUser() {
    if (editBusy) return;
    setEditUser(null);
    setEditDisplayName("");
    setEditEmail("");
    setEditNewPassword("");
    setEditTouched(false);
  }

  async function submitEditUser(e: FormEvent) {
    e.preventDefault();
    if (!editUser) return;
    setEditTouched(true);
    if (editEmailError || editPasswordError) return;
    const uname = editUser.metadata.name;
    setEditBusy(true);
    try {
      const specPatch: Record<string, string> = {
        displayName: editDisplayName.trim(),
        email: editEmail.trim(),
      };
      if (editNewPassword.trim()) {
        specPatch.password = editNewPassword;
      }
      const res = await apiFetch(`/apis/iam.gamepanel.io/v1/users/${encodeURIComponent(uname)}`, {
        method: "PATCH",
        body: JSON.stringify({ spec: specPatch }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "保存用户资料失败"));
      toast(
        "success",
        editNewPassword.trim() ? `已更新用户 ${uname} 的资料并重置密码` : `已更新用户 ${uname} 的资料`,
      );
      setEditBusy(false);
      closeEditUser();
      await reload();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "保存用户资料失败");
      setEditBusy(false);
    }
  }

  async function togglePlatformAdmin(u: UserItem) {
    const uname = u.metadata.name;
    const matched = adminBindingsByUser(uname);
    setRoleBusy(uname);
    try {
      if (matched.length > 0) {
        for (const b of matched) {
          const rest = (b.subjects || []).filter((s) => !(s.kind === "User" && s.name === uname));
          if (rest.length === 0) {
            const res = await apiFetch(
              `/apis/iam.gamepanel.io/v1/globalrolebindings/${encodeURIComponent(b.metadata.name)}`,
              {
                method: "DELETE",
              },
            );
            if (!res.ok) throw new Error(await errorMessage(res, "取消管理员权限失败"));
          } else {
            const res = await apiFetch(
              `/apis/iam.gamepanel.io/v1/globalrolebindings/${encodeURIComponent(b.metadata.name)}`,
              {
                method: "PUT",
                body: JSON.stringify({ ...b, subjects: rest }),
              },
            );
            if (!res.ok) throw new Error(await errorMessage(res, "取消管理员权限失败"));
          }
        }
        toast("success", `已取消 ${uname} 的平台管理员权限`);
      } else {
        const bindingName = `${uname}-platform-admin`;
        const res = await apiFetch("/apis/iam.gamepanel.io/v1/globalrolebindings", {
          method: "POST",
          body: JSON.stringify({
            apiVersion: "iam.gamepanel.io/v1",
            kind: "GlobalRoleBinding",
            metadata: { name: bindingName },
            roleRef: { name: "platform-admin" },
            subjects: [{ kind: "User", name: uname }],
          }),
        });
        if (!res.ok) throw new Error(await errorMessage(res, "授予管理员权限失败"));
        toast("success", `已将 ${uname} 设为平台管理员`);
      }
      await reload();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "修改管理员权限失败");
    } finally {
      setRoleBusy(null);
    }
  }

  const errors = {
    name: validateName(name, "用户名"),
    email: validateEmail(email),
    password: !password ? "请设置初始密码" : password.length < 8 ? "密码至少 8 位" : "",
  };
  const hasErrors = Object.values(errors).some(Boolean);

  function close() {
    if (busy) return;
    setOpen(false);
    setName("");
    setEmail("");
    setPassword("");
    setTouched(false);
  }

  async function toggleUserState(u: UserItem) {
    const nextState = u.spec?.state === "Disabled" ? "Active" : "Disabled";
    setStateBusy(u.metadata.name);
    try {
      const res = await apiFetch(`/apis/iam.gamepanel.io/v1/users/${encodeURIComponent(u.metadata.name)}`, {
        method: "PUT",
        body: JSON.stringify({
          ...u,
          spec: { ...u.spec, state: nextState },
        }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "更新用户状态失败"));
      toast("success", `用户 ${u.metadata.name} 已${nextState === "Active" ? "启用" : "停用"}`);
      await reload();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "更新用户状态失败");
    } finally {
      setStateBusy(null);
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    const uname = deleteTarget.metadata.name;
    setDeleting(true);
    try {
      const res = await apiFetch(`/apis/iam.gamepanel.io/v1/users/${encodeURIComponent(uname)}`, { method: "DELETE" });
      if (!res.ok) throw new Error(await errorMessage(res, "删除用户失败"));
      toast("success", `用户 ${uname} 已删除`);
      setDeleteTarget(null);
      await reload();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "删除用户失败");
    } finally {
      setDeleting(false);
    }
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setTouched(true);
    if (hasErrors) return;
    const username = name.trim();
    setBusy(true);
    try {
      const res = await apiFetch("/apis/iam.gamepanel.io/v1/users", {
        method: "POST",
        body: JSON.stringify({
          apiVersion: "iam.gamepanel.io/v1",
          kind: "User",
          metadata: { name: username },
          spec: { email: email.trim(), password, state: "Active" },
        }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "创建用户失败，请检查填写内容后重试"));
      toast("success", `用户 ${username} 已创建`);
      setBusy(false);
      close();
      await reload();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "创建用户失败");
      setBusy(false);
    }
  }

  return (
    <div>
      <PageHeader
        title="用户权限"
        description={users.length > 0 ? `共 ${users.length} 个平台账号` : undefined}
        actions={
          <Button variant="primary" icon={<Plus className="h-4 w-4" />} onClick={() => setOpen(true)}>
            新建用户
          </Button>
        }
      />

      <div className="mb-5 flex min-h-10 flex-wrap items-center justify-between gap-3">
        <Segmented
          label="用户权限视图切换"
          value={activeTab}
          onChange={(t) => {
            setActiveTab(t);
            if (typeof window !== "undefined") {
              const sp = new URLSearchParams(window.location.search);
              if (t === "users") sp.delete("tab");
              else sp.set("tab", t);
              const qs = sp.toString();
              window.history.replaceState(
                window.history.state,
                "",
                qs ? `${window.location.pathname}?${qs}` : window.location.pathname,
              );
            }
          }}
          items={[
            { value: "users", label: "平台用户" },
            { value: "roles", label: "全局角色" },
          ]}
        />

        {activeTab === "users" && (
          <div className="flex flex-wrap items-center gap-2.5">
            <div className="relative w-64 shrink-0 sm:w-72">
              <Search className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-[#8A90A6]" />
              <Input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="搜索用户名、显示名或邮箱..."
                className="pl-9"
              />
            </div>
            <div className="w-36 shrink-0">
              <Select
                value={roleFilter}
                onChange={(e) => setRoleFilter(e.target.value as "all" | "admin" | "member")}
                aria-label="按平台角色筛选"
              >
                <option value="all">全部角色</option>
                <option value="admin">平台管理员</option>
                <option value="member">普通用户</option>
              </Select>
            </div>
          </div>
        )}
      </div>

      {activeTab === "users" ? (
        <Card>
          {loading && users.length === 0 ? (
            <div className="space-y-3 p-6">
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} className="h-14" />
              ))}
            </div>
          ) : filteredUsers.length === 0 ? (
            <EmptyState
              image={null}
              title={users.length === 0 ? "还没有用户" : "没有匹配的用户"}
              body={users.length === 0 ? "创建账号后，用户即可登录控制台。" : "请尝试调整搜索关键词或角色筛选条件。"}
            />
          ) : (
            <div>
              <div className="hidden border-b border-[#EEF0F7] bg-[#FAFBFD] px-6 py-3 text-[12px] font-medium text-[#8A90A6] md:grid md:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)_120px_220px] md:items-center md:gap-4">
                <span>账号信息</span>
                <span>平台角色</span>
                <span>状态</span>
                <span className="text-right">操作</span>
              </div>
              <ul className="divide-y divide-[#EEF0F7]">
                {filteredUsers.map((u) => {
                  const st = USER_STATE[u.spec?.state || "Active"] ?? {
                    label: u.spec?.state || "—",
                    tone: "neutral" as const,
                  };
                  const isSelf = u.metadata.name === userName;
                  const isBuiltInAdmin = u.metadata.name === "admin";
                  const isPlatformAdmin = isBuiltInAdmin || adminBindingsByUser(u.metadata.name).length > 0;
                  const disabled = u.spec?.state === "Disabled";

                  return (
                    <li
                      key={u.metadata.name}
                      className="flex flex-wrap items-center justify-between gap-4 px-6 py-4 md:grid md:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)_120px_220px]"
                    >
                      <div className="flex min-w-0 items-center gap-3.5">
                        <Avatar name={u.metadata.name} size={38} round />
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-baseline gap-1.5">
                            <span className="truncate text-[14px] font-semibold text-[#14161F]">
                              {u.spec?.displayName ? `${u.spec.displayName} (${u.metadata.name})` : u.metadata.name}
                            </span>
                            {isSelf && <span className="text-[12px] text-[#8A90A6]">· 当前账号</span>}
                          </div>
                          <div className="truncate text-[13px] text-[#8A90A6]">{u.spec?.email || "未填写邮箱"}</div>
                        </div>
                      </div>

                      <div className="flex items-center gap-1.5 text-[13px]">
                        {isPlatformAdmin ? (
                          <>
                            <ShieldCheck className="h-4 w-4 shrink-0 text-[#2B3AE8]" />
                            <span className="font-medium text-[#14161F]">
                              {isBuiltInAdmin ? "内置管理员" : "平台管理员"}
                            </span>
                          </>
                        ) : (
                          <span className="text-[#5B6075]">普通用户</span>
                        )}
                      </div>

                      <div>
                        <Badge tone={st.tone}>{st.label}</Badge>
                      </div>

                      <div className="flex items-center justify-end gap-1.5">
                        {!isSelf && !isBuiltInAdmin && (
                          <Button
                            size="sm"
                            variant="secondary"
                            disabled={roleBusy === u.metadata.name}
                            onClick={() => void togglePlatformAdmin(u)}
                          >
                            {isPlatformAdmin ? "取消管理员" : "设为管理员"}
                          </Button>
                        )}
                        <IconButton
                          label={`编辑用户 ${u.metadata.name} 资料与密码`}
                          onClick={() => openEditUser(u)}
                        >
                          <Pencil className="h-4 w-4" />
                        </IconButton>
                        {!isSelf && !isBuiltInAdmin && (
                          <MoreMenu
                            label={`更多操作 (${u.metadata.name})`}
                            items={[
                              {
                                label: disabled ? "启用账号" : "停用账号",
                                icon: disabled ? (
                                  <CheckCircle2 className="h-4 w-4 text-[#1F9D55]" />
                                ) : (
                                  <Ban className="h-4 w-4" />
                                ),
                                disabled: stateBusy === u.metadata.name,
                                onSelect: () => void toggleUserState(u),
                              },
                              {
                                label: "删除用户",
                                icon: <Trash2 className="h-4 w-4" />,
                                danger: true,
                                onSelect: () => setDeleteTarget(u),
                              },
                            ]}
                          />
                        )}
                      </div>
                    </li>
                  );
                })}
              </ul>
            </div>
          )}
        </Card>
      ) : (
        <Card>
          {loading && visibleRoles.length === 0 ? (
            <div className="space-y-3 p-6">
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} className="h-20" />
              ))}
            </div>
          ) : visibleRoles.length === 0 ? (
            <EmptyState image={null} title="还没有全局角色" />
          ) : (
            <ul className="divide-y divide-[#EEF0F7]">
              {visibleRoles.map((r) => {
                const isBuiltIn =
                  r.metadata.name === "platform-admin" ||
                  r.metadata.name === "node-agent" ||
                  r.metadata.name === "registry-worker" ||
                  r.metadata.name === "authenticated-user";
                const meta = ROLE_META[r.metadata.name];
                const isExpanded = expandedRole === r.metadata.name;
                const rules = filterLiteRules(r.rules);

                return (
                  <li key={r.metadata.name} className="px-6 py-4">
                    <div className="flex flex-wrap items-center justify-between gap-4">
                      <div className="flex min-w-0 items-start gap-3.5">
                        <span className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[#EEF1FF] text-[#2B3AE8]">
                          <ShieldCheck className="h-4 w-4" />
                        </span>
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="text-[15px] font-semibold text-[#14161F]">
                              {meta?.title || r.metadata.name}
                            </span>
                            <Mono className="text-xs text-[#8A90A6]">{r.metadata.name}</Mono>
                            {isBuiltIn && <Badge tone="neutral">系统内置</Badge>}
                          </div>
                          <p className="mt-0.5 text-[13px] text-[#5B6075]">
                            {meta?.desc || r.description || "暂无描述"}
                          </p>
                        </div>
                      </div>

                      <div className="flex items-center gap-3">
                        <span className="text-[13px] text-[#5B6075]">
                          {summarizeRoleRules(rules)}
                        </span>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setExpandedRole(isExpanded ? null : r.metadata.name)}
                          icon={
                            isExpanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />
                          }
                        >
                          {isExpanded ? "收起策略" : "查看策略"}
                        </Button>
                      </div>
                    </div>

                    {isExpanded && (
                      <div className="mt-3.5 overflow-hidden rounded-xl bg-[#F7F8FC]">
                        {rules.length === 0 ? (
                          <div className="px-4 py-3 text-[13px] text-[#8A90A6]">没有任何权限策略</div>
                        ) : (
                          <dl className="divide-y divide-[#EEF0F7]">
                            <div className="hidden bg-[#F1F3FA] px-4 py-2 text-[11.5px] font-medium text-[#5B6075] sm:grid sm:grid-cols-[minmax(0,1.25fr)_110px_minmax(0,1.45fr)] sm:items-center sm:gap-4">
                              <span>受控资源范围</span>
                              <span>权限级别</span>
                              <span>允许操作动作</span>
                            </div>
                            {rules.map((rule, idx) => {
                              const resources = rule.resources || [];
                              const verbs = rule.verbs || [];
                              const access = classifyRuleAccess(verbs);
                              const scopedNames =
                                rule.resourceNames && rule.resourceNames.length > 0
                                  ? rule.resourceNames.join(", ")
                                  : "";
                              return (
                                <div
                                  key={idx}
                                  className="grid grid-cols-1 gap-2.5 px-4 py-3 text-xs sm:grid-cols-[minmax(0,1.25fr)_110px_minmax(0,1.45fr)] sm:items-center sm:gap-4"
                                >
                                  <dt className="flex flex-wrap items-center gap-1.5">
                                    {resources.map((resKey) => {
                                      const zh = RESOURCE_LABELS[resKey];
                                      return (
                                        <span
                                          key={resKey}
                                          className="inline-flex items-center gap-1.5 rounded-lg bg-white px-2.5 py-1 text-xs"
                                        >
                                          <span className="font-medium text-[#14161F]">
                                            {zh || resKey}
                                          </span>
                                          <Mono className="text-[11px] text-[#8A90A6]">{resKey}</Mono>
                                        </span>
                                      );
                                    })}
                                    {scopedNames && (
                                      <span className="rounded bg-[#EFF2F8] px-2 py-0.5 text-[11px] text-[#5B6075]">
                                        仅限: <Mono>{scopedNames}</Mono>
                                      </span>
                                    )}
                                  </dt>

                                  <dd>
                                    <Badge tone={access.tone}>{access.label}</Badge>
                                  </dd>

                                  <dd className="flex flex-wrap items-center gap-1.5">
                                    {verbs.map((verbKey) => {
                                      const vMeta = VERB_META[verbKey] || {
                                        label: verbKey,
                                        kind: "read" as const,
                                      };
                                      const chipCls =
                                        vMeta.kind === "full"
                                          ? "bg-[#EEF1FF] text-[#2B3AE8] font-medium"
                                          : vMeta.kind === "write"
                                            ? "bg-[#F2F5FF] text-[#2B3AE8]"
                                            : "bg-white text-[#3D4258]";
                                      return (
                                        <span
                                          key={verbKey}
                                          className={`inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs ${chipCls}`}
                                        >
                                          <span>{vMeta.label}</span>
                                          <Mono className="text-[10.5px] opacity-75">{verbKey}</Mono>
                                        </span>
                                      );
                                    })}
                                  </dd>
                                </div>
                              );
                            })}
                          </dl>
                        )}
                      </div>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </Card>
      )}

      <Modal
        open={open}
        onClose={close}
        title="新建用户"
        footer={
          <>
            <Button variant="ghost" onClick={close}>
              取消
            </Button>
            <Button variant="primary" type="submit" form="create-user" loading={busy}>
              创建
            </Button>
          </>
        }
      >
        <form id="create-user" onSubmit={submit} noValidate className="space-y-4">
          <Field label="用户名" htmlFor="u-name" error={touched ? errors.name : ""} hint="小写字母、数字和短横线">
            <Input
              id="u-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoComplete="off"
              placeholder="alice"
            />
          </Field>
          <Field label="邮箱" htmlFor="u-email" error={touched ? errors.email : ""}>
            <Input
              id="u-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              autoComplete="off"
              placeholder="alice@example.com"
            />
          </Field>
          <Field label="初始密码" htmlFor="u-pass" error={touched ? errors.password : ""} hint="至少 8 位">
            <Input
              id="u-pass"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
            />
          </Field>
        </form>
      </Modal>

      <Modal
        open={!!editUser}
        onClose={closeEditUser}
        title={`编辑用户「${editUser?.metadata.name || ""}」`}
        footer={
          <>
            <Button variant="ghost" onClick={closeEditUser} disabled={editBusy}>
              取消
            </Button>
            <Button
              variant="primary"
              type="submit"
              form="edit-user-form"
              loading={editBusy}
              disabled={editBusy || !editUserDirty}
            >
              保存修改
            </Button>
          </>
        }
      >
        <form id="edit-user-form" onSubmit={(e) => void submitEditUser(e)} noValidate className="space-y-4">
          <Field label="用户名" htmlFor="eu-name">
            <Input id="eu-name" value={editUser?.metadata.name || ""} disabled />
          </Field>
          <Field label="显示名称（选填）" htmlFor="eu-display">
            <Input
              id="eu-display"
              value={editDisplayName}
              onChange={(e) => setEditDisplayName(e.target.value)}
              placeholder={editUser?.metadata.name || "例如：张三"}
              autoComplete="off"
            />
          </Field>
          <Field label="邮箱" htmlFor="eu-email" error={editTouched ? editEmailError : ""}>
            <Input
              id="eu-email"
              type="email"
              value={editEmail}
              onChange={(e) => setEditEmail(e.target.value)}
              placeholder="name@example.com"
              autoComplete="off"
            />
          </Field>
          <Field
            label="重置新密码（选填）"
            htmlFor="eu-pass"
            error={editTouched ? editPasswordError : ""}
            hint="至少 8 位；重置后该用户需重新登录"
          >
            <Input
              id="eu-pass"
              type="password"
              value={editNewPassword}
              onChange={(e) => setEditNewPassword(e.target.value)}
              placeholder="留空不修改密码"
              autoComplete="new-password"
            />
          </Field>
        </form>
      </Modal>

      <ConfirmDialog
        open={!!deleteTarget}
        title={`删除用户 ${deleteTarget?.metadata.name || ""}？`}
        body="删除后该账号无法再登录控制台，且无法恢复。"
        confirmLabel="删除用户"
        danger
        loading={deleting}
        onConfirm={() => void confirmDelete()}
        onClose={() => !deleting && setDeleteTarget(null)}
      />
    </div>
  );
}
