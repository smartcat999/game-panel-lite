"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Search, UserPlus } from "lucide-react";
import { useConsole } from "../ConsoleProvider";
import { errorMessage } from "../lib/session";
import { Avatar } from "../../components/console/TopBar";
import { InviteModal } from "../../components/console/servers/InviteModal";
import {
  Badge,
  Button,
  Card,
  ConfirmDialog,
  EmptyState,
  Input,
  PageHeader,
  Segmented,
  Select,
  Skeleton,
} from "../../components/console/ui";

interface RoleBinding {
  metadata: { name: string; tenant?: string; creationTimestamp?: string };
  roleRef: { name: string };
  subjects: Array<{ kind: string; name: string }>;
}

interface TenantRoleItem {
  metadata: { name: string };
  description?: string;
}

/** Friendly names for the built-in roles every workspace gets (see pkg/models/iam/tenantrole). */
const BUILT_IN_ROLES: Record<string, { label: string; scope: string; desc: string; order: number }> = {
  "tenant-admin": {
    label: "管理员",
    scope: "完全管理权限",
    desc: "管理服务器创建与销毁、配额与告警通知渠道、工作区成员及全部设置",
    order: 0,
  },
  "instance-operator": {
    label: "运维",
    scope: "服务器运维权限",
    desc: "负责现有服务器的启停、控制台终端、文件、模组、备份、定时任务与参数配置，不可创建或删除服务器及管理成员",
    order: 1,
  },
  "tenant-viewer": {
    label: "只读",
    scope: "只读查看权限",
    desc: "仅可查看工作区内的服务器状态、监控指标、日志审计与连接地址",
    order: 2,
  },
};

const roleLabel = (name: string) => BUILT_IN_ROLES[name]?.label ?? name;

interface Member {
  user: string;
  role: string;
  bindings: RoleBinding[];
}

export default function MembersPage() {
  const { tenant, tenantObj, apiFetch, toast, userName, canManageTenant } = useConsole();
  const base = `/apis/iam.gamepanel.io/v1/tenants/${encodeURIComponent(tenant)}`;

  const [tab, setTab] = useState<"members" | "roles">("members");
  const [searchQuery, setSearchQuery] = useState("");

  useEffect(() => {
    if (typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    if (sp.get("tab") === "roles") setTab("roles");
  }, []);

  function changeTab(next: "members" | "roles") {
    setTab(next);
    if (typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    if (next === "members") sp.delete("tab");
    else sp.set("tab", next);
    const qs = sp.toString();
    window.history.replaceState(
      window.history.state,
      "",
      qs ? `${window.location.pathname}?${qs}` : window.location.pathname,
    );
  }
  const [bindings, setBindings] = useState<RoleBinding[]>([]);
  const [roles, setRoles] = useState<TenantRoleItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [forbidden, setForbidden] = useState(false);

  const [addOpen, setAddOpen] = useState(false);

  const [removing, setRemoving] = useState<Member | null>(null);
  const [busyUser, setBusyUser] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!tenant) return;
    setLoading(true);
    try {
      const [bRes, rRes] = await Promise.all([apiFetch(`${base}/tenantrolebindings`), apiFetch(`${base}/tenantroles`)]);
      if (bRes.status === 403) {
        setForbidden(true);
        return;
      }
      setForbidden(false);
      if (!bRes.ok) throw new Error(await errorMessage(bRes, "加载成员失败"));
      setBindings((await bRes.json())?.items || []);
      if (rRes.ok) setRoles((await rRes.json())?.items || []);
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "加载成员失败");
    } finally {
      setLoading(false);
    }
  }, [apiFetch, base, tenant, toast]);

  useEffect(() => {
    void load();
  }, [load]);

  const owner = tenantObj?.spec.owner || "";

  const sortedRoles = useMemo(
    () =>
      [...roles].sort(
        (a, b) => (BUILT_IN_ROLES[a.metadata.name]?.order ?? 9) - (BUILT_IN_ROLES[b.metadata.name]?.order ?? 9),
      ),
    [roles],
  );

  const members = useMemo<Member[]>(() => {
    const map = new Map<string, Member>();
    for (const b of bindings) {
      for (const s of b.subjects || []) {
        if (s.kind !== "User") continue;
        const cur = map.get(s.name);
        if (!cur) {
          map.set(s.name, { user: s.name, role: b.roleRef.name, bindings: [b] });
        } else {
          cur.bindings.push(b);
          const rank = (r: string) => BUILT_IN_ROLES[r]?.order ?? 5;
          if (rank(b.roleRef.name) < rank(cur.role)) cur.role = b.roleRef.name;
        }
      }
    }
    if (owner && !map.has(owner)) map.set(owner, { user: owner, role: "tenant-admin", bindings: [] });
    return [...map.values()].sort((a, b) => (a.user === owner ? -1 : b.user === owner ? 1 : a.user.localeCompare(b.user)));
  }, [bindings, owner]);

  const filteredMembers = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return members;
    return members.filter((m) => m.user.toLowerCase().includes(q) || roleLabel(m.role).toLowerCase().includes(q));
  }, [members, searchQuery]);

  const bindingBody = (name: string, user: string, role: string) => ({
    apiVersion: "iam.gamepanel.io/v1",
    kind: "TenantRoleBinding",
    metadata: { name, tenant },
    roleRef: { name: role },
    subjects: [{ kind: "User", name: user }],
  });

  async function handleChangeRole(m: Member, role: string) {
    if (role === m.role) return;
    setBusyUser(m.user);
    try {
      const [first, ...rest] = m.bindings;
      if (first) {
        const res = await apiFetch(`${base}/tenantrolebindings/${encodeURIComponent(first.metadata.name)}`, {
          method: "PUT",
          body: JSON.stringify(bindingBody(first.metadata.name, m.user, role)),
        });
        if (!res.ok) throw new Error(await errorMessage(res, "修改角色失败"));
      } else {
        const res = await apiFetch(`${base}/tenantrolebindings`, {
          method: "POST",
          body: JSON.stringify(bindingBody(`member-${m.user}`, m.user, role)),
        });
        if (!res.ok) throw new Error(await errorMessage(res, "修改角色失败"));
      }
      for (const b of rest) {
        await apiFetch(`${base}/tenantrolebindings/${encodeURIComponent(b.metadata.name)}`, { method: "DELETE" });
      }
      toast("success", `已将 ${m.user} 的角色更新为 ${roleLabel(role)}`);
      await load();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "修改角色失败");
    } finally {
      setBusyUser(null);
    }
  }

  async function handleRemove() {
    if (!removing) return;
    setBusyUser(removing.user);
    try {
      for (const b of removing.bindings) {
        const res = await apiFetch(`${base}/tenantrolebindings/${encodeURIComponent(b.metadata.name)}`, {
          method: "DELETE",
        });
        if (!res.ok && res.status !== 404) throw new Error(await errorMessage(res, "移除成员失败"));
      }
      toast("success", `已将 ${removing.user} 移出工作区`);
      setRemoving(null);
      await load();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "移除成员失败");
    } finally {
      setBusyUser(null);
    }
  }

  const wsName = tenantObj?.spec.displayName || tenant;
  const isPersonal = false;
  const canEditMembers = canManageTenant && !isPersonal;

  if (forbidden) {
    return (
      <>
        <PageHeader title="工作区成员" description={`工作区 ${wsName}`} />
        <Card>
          <EmptyState
            image="/landing/features/console.jpg"
            title="无成员管理权限"
            body="仅工作区管理员可查看与调整成员角色绑定。"
          />
        </Card>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="工作区成员"
        description={
          isPersonal
            ? `个人工作区 ${wsName} · 仅所有者可用`
            : loading
              ? `工作区：${wsName}`
              : `工作区：${wsName} · 共 ${members.length} 位成员`
        }
        actions={
          <div className="flex flex-wrap items-center gap-2.5">
            {canEditMembers && (
              <Button variant="primary" icon={<UserPlus className="h-4 w-4" />} onClick={() => setAddOpen(true)}>
                添加成员
              </Button>
            )}
          </div>
        }
      />

      <div className="mb-5 flex min-h-10 flex-wrap items-center justify-between gap-3">
        <Segmented<"members" | "roles">
          label="成员管理视图"
          value={tab}
          onChange={changeTab}
          items={[
            { value: "members", label: "成员列表" },
            { value: "roles", label: "角色说明" },
          ]}
        />
        {tab === "members" && members.length > 1 && (
          <div className="relative w-64 shrink-0">
            <Search className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-[#8A90A6]" />
            <Input
              placeholder="搜索用户名或角色..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9"
            />
          </div>
        )}
      </div>

      {tab === "members" ? (
        <Card className="overflow-hidden">
          {loading && !members.length ? (
            <div className="space-y-3 p-6">
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} className="h-12" />
              ))}
            </div>
          ) : filteredMembers.length === 0 ? (
            <div className="px-6 py-12 text-center text-sm text-[#8A90A6]">没有匹配的成员</div>
          ) : (
            <div>
              <div className="hidden border-b border-[#EEF0F7] bg-[#FAFBFD] px-6 py-3 text-[12px] font-medium text-[#8A90A6] sm:flex sm:items-center sm:justify-between">
                <span>成员账号与权限说明</span>
                <span>工作区角色</span>
              </div>
              <ul className="divide-y divide-[#EEF0F7]">
                {filteredMembers.map((m) => {
                  const isOwner = m.user === owner;
                  const isSelf = m.user === userName;
                  return (
                    <li key={m.user} className="flex flex-wrap items-center gap-4 px-6 py-4 hover:bg-[#FAFBFE]">
                      <Avatar name={m.user} size={36} round />
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="truncate font-semibold text-[#14161F]">{m.user}</span>
                          {isOwner ? (
                            <Badge tone="accent">所有者</Badge>
                          ) : isSelf ? (
                            <Badge tone="neutral">当前账号</Badge>
                          ) : null}
                        </div>
                        <p className="mt-0.5 text-[13px] text-[#8A90A6]">
                          {BUILT_IN_ROLES[m.role]?.desc ??
                            roles.find((r) => r.metadata.name === m.role)?.description ??
                            m.role}
                        </p>
                      </div>
                      {isOwner || !canEditMembers ? (
                        <span className="text-sm font-medium text-[#5B6075]">{roleLabel(m.role)}</span>
                      ) : (
                        <div className="flex items-center gap-2">
                          <Select
                            aria-label={`${m.user} 的角色`}
                            value={m.role}
                            disabled={busyUser === m.user}
                            onChange={(e) => void handleChangeRole(m, e.target.value)}
                            className="h-9 w-[120px]"
                          >
                            {sortedRoles.map((r) => (
                              <option key={r.metadata.name} value={r.metadata.name}>
                                {roleLabel(r.metadata.name)}
                              </option>
                            ))}
                            {!roles.some((r) => r.metadata.name === m.role) && <option value={m.role}>{m.role}</option>}
                          </Select>
                          <Button variant="ghost" size="sm" disabled={busyUser === m.user} onClick={() => setRemoving(m)}>
                            移除
                          </Button>
                        </div>
                      )}
                    </li>
                  );
                })}
              </ul>
            </div>
          )}
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <div className="hidden border-b border-[#EEF0F7] bg-[#FAFBFD] px-6 py-3 text-[12px] font-medium text-[#8A90A6] sm:flex sm:items-center sm:justify-between">
            <span>角色名称与职责（共 {sortedRoles.length} 个角色）</span>
            <span>权限范围</span>
          </div>
          <ul className="divide-y divide-[#EEF0F7]">
            {sortedRoles.map((r) => (
              <li key={r.metadata.name} className="flex flex-wrap items-center justify-between gap-4 px-6 py-4">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-semibold text-[#14161F]">{roleLabel(r.metadata.name)}</span>
                    {BUILT_IN_ROLES[r.metadata.name] && <Badge tone="neutral">系统内置</Badge>}
                  </div>
                  <p className="mt-0.5 text-[13px] text-[#8A90A6]">
                    {BUILT_IN_ROLES[r.metadata.name]?.desc ?? r.description ?? "自定义工作区角色"}
                  </p>
                </div>
                <span className="text-[13px] text-[#5B6075]">
                  {BUILT_IN_ROLES[r.metadata.name]?.scope ?? "自定义权限"}
                </span>
              </li>
            ))}
          </ul>
        </Card>
      )}

      <InviteModal
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onMemberAdded={() => {
          setAddOpen(false);
          void load();
        }}
      />

      <ConfirmDialog
        open={!!removing}
        title={`将 ${removing?.user ?? ""} 移出工作区？`}
        body="移除后该账号将不再拥有当前工作区的访问权限，后续可随时重新添加。"
        confirmLabel="移除"
        danger
        loading={!!removing && busyUser === removing.user}
        onConfirm={() => void handleRemove()}
        onClose={() => setRemoving(null)}
      />
    </>
  );
}
