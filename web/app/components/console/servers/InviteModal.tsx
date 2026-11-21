"use client";

import { FormEvent, ReactNode, useCallback, useEffect, useMemo, useState } from "react";
import { ArrowLeft, ArrowRight, Check } from "lucide-react";
import { useConsole } from "../../../console/ConsoleProvider";
import { endpointOf, phaseLabel, phaseOf } from "../../../console/lib/format";
import { errorMessage } from "../../../console/lib/session";
import type { InstanceItem, NodeItem } from "../../../console/lib/types";
import { gameOf } from "./shared";
import { Button, CopyButton, copyToClipboard, Input, Modal, Segmented } from "../ui";

type InviteRole = "instance-operator" | "tenant-viewer" | "tenant-admin";

const INVITE_ROLES: ReadonlyArray<{ value: InviteRole; label: string; desc: string }> = [
  { value: "instance-operator", label: "运维", desc: "可操作服务器启停、终端、文件、模组、备份、定时任务与配置，不可创建或删除服务器" },
  { value: "tenant-viewer", label: "只读", desc: "仅可查看服务器状态、监控、日志与连接地址" },
  { value: "tenant-admin", label: "管理员", desc: "拥有工作区全部权限，包括创建/删除服务器与成员管理" },
];

const USERNAME_RE = /^[a-z0-9]([a-z0-9._-]{0,61}[a-z0-9])?$/;
const BODY_HEIGHT = "min-h-[220px] pb-5";

function joinStepsForGame(gameId: string, endpoint: string | null): string[] {
  switch (gameId) {
    case "minecraft-paper":
      return ["打开 Minecraft，进入「多人游戏」", "点击「添加服务器」，粘贴上方地址", "选中服务器，点击「加入服务器」"];
    case "minecraft-fabric":
      return ["使用对应版本的 Minecraft Fabric 客户端并安装所需模组后，进入「多人游戏」", "点击「添加服务器」，粘贴上方地址", "选中服务器，点击「加入服务器」"];
    case "palworld-dedicated":
      return ["打开幻兽帕鲁，选择「加入多人游戏」", "在底部输入框粘贴上方地址", "点击「连接」即可进入"];
    case "project-zomboid":
      return ["打开僵尸毁灭工程，选择「加入」", "点击「添加服务器」，填入 IP 与端口", "保存后选中服务器并加入"];
    case "terraria-vanilla":
      return ["打开泰拉瑞亚，选择「多人模式」", "点击「通过 IP 加入」，选择角色", "依次填入上方地址的 IP 与端口"];
    case "terraria-tmodloader":
      return ["在 Steam 启动 tModLoader，进入「多人模式」", "点击「通过 IP 加入」（将自动同步服务端模组）", "依次填入上方地址的 IP 与端口"];
    case "dont-starve-together": {
      const [host, port] = (endpoint || "IP:10999").split(":");
      return ["打开饥荒联机版主界面", "按 ~ 键打开控制台", `输入 c_connect("${host}", ${port || "10999"}) 回车`];
    }
    case "seven-days-to-die":
      return ["打开七日杀，选择「加入游戏」", "点击底部「连接至 IP」", "粘贴上方地址并连接"];
    case "valheim-dedicated":
      return ["打开英灵神殿，进入「开始游戏」->「加入游戏」", "点击「通过 IP 加入」，粘贴上方地址并输入进服密码", "等待世界同步加载完成后即可进入"];
    case "core-keeper":
      return ["打开护核纪元，进入「加入游戏」", "点击「通过游戏 ID 或直连加入」并粘贴上方地址与密码", "等待地下世界加载完成后即可加入"];
    case "satisfactory-dedicated":
      return ["打开幸福工厂，在主菜单选择「服务器管理器」", "点击「添加服务器」，填入上方 IP 与端口后确认", "认证并进入服务器大厅开始联机建造"];
    case "left-4-dead-2":
      return ["打开求生之路 2 主界面，按 ~ 键打开开发者控制台", `输入 connect ${endpoint || "IP:端口"} 后按回车连接`, "等待关卡加载完毕即可加入游戏"];
    case "counter-strike-2":
      return ["打开反恐精英 2 主界面，按 ~ 键打开游戏控制台", `输入 connect ${endpoint || "IP:端口"}（若有密码请追加 ; password 密码）后回车`, "等待地图加载完成后即可进入对局"];
    case "rust-dedicated":
      return ["打开腐蚀主界面，按 F1 键打开游戏控制台", `输入 client.connect ${endpoint || "IP:端口"} 后按回车`, "等待地图地形与资源接收完毕即可登录海岛"];
    default:
      return ["打开游戏，进入多人联机", "选择直连 / 添加服务器", "粘贴上方地址即可加入"];
  }
}

function useInviteLink(open: boolean, enabled: boolean, role: InviteRole, serverName?: string) {
  const { tenant, apiFetch, toast } = useConsole();
  const [token, setToken] = useState("");
  const [loading, setLoading] = useState(false);

  const generate = useCallback(async () => {
    setLoading(true);
    setToken("");
    try {
      const res = await apiFetch(`/apis/core.gamepanel.io/v1/tenants/${encodeURIComponent(tenant)}/invites`, {
        method: "POST",
        body: JSON.stringify({ role, ...(serverName ? { serverName } : {}) }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "生成邀请链接失败"));
      const data = (await res.json()) as { token?: string };
      setToken(data.token || "");
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "生成邀请链接失败");
    } finally {
      setLoading(false);
    }
  }, [apiFetch, role, serverName, tenant, toast]);

  useEffect(() => {
    if (open && enabled && tenant) void generate();
  }, [open, enabled, tenant, generate]);

  const url = token && typeof window !== "undefined" ? `${window.location.origin}/invite/${token}` : "";
  return { url, loading };
}

function useFlash(): [boolean, () => void] {
  const [on, setOn] = useState(false);
  const flash = useCallback(() => {
    setOn(true);
    setTimeout(() => setOn(false), 1800);
  }, []);
  return [on, flash];
}

function Label({ children }: { children: ReactNode }) {
  return <div className="text-xs text-[#8A90A6]">{children}</div>;
}

function PlayView({
  inst,
  nodes,
  canCollaborate,
  onCollaborate,
}: {
  inst: InstanceItem;
  nodes: NodeItem[];
  canCollaborate: boolean;
  onCollaborate: () => void;
}) {
  const { apps, toast } = useConsole();
  const [copied, flash] = useFlash();
  const game = gameOf(inst, apps);
  const endpoint = endpointOf(inst, nodes, apps);
  const phase = phaseOf(inst);
  const steps = joinStepsForGame(game.id, endpoint);

  async function copyMessage() {
    const text = [
      `来我的【${game.title}】服务器一起玩！`,
      `服务器地址：${endpoint || "（启动后分配）"}`,
      ...steps.map((s, i) => `${i + 1}. ${s}`),
    ].join("\n");
    if (!(await copyToClipboard(text))) {
      toast("error", "复制失败，请手动复制地址");
      return;
    }
    flash();
    toast("success", "邀请消息已复制，发给好友吧");
  }

  return (
    <>
      <div className={BODY_HEIGHT}>
        <Label>服务器地址</Label>
        <div className="mt-1.5 flex items-center justify-between gap-3">
          {endpoint ? (
            <span className="min-w-0 break-all font-mono text-[19px] font-semibold leading-snug tracking-tight text-[#14161F] sm:text-[24px]">{endpoint}</span>
          ) : (
            <span className="text-lg font-medium text-[#8A90A6]">启动服务器后自动分配</span>
          )}
          {endpoint && <CopyButton text={endpoint} label="复制服务器地址" />}
        </div>
        <div className="mt-1 flex items-center gap-1.5 text-xs text-[#5B6075]">
          <span className={`h-1.5 w-1.5 rounded-full ${phase === "Running" ? "bg-[#1F9D55]" : "bg-[#B4B9CC]"}`} />
          {phaseLabel(phase)} · {game.title}
        </div>

        <div className="mt-6">
          <Label>好友如何加入</Label>
        </div>
        <ol className="mt-2.5 space-y-2.5 text-[13px] text-[#3D4258]">
          {steps.map((s, i) => (
            <li key={s} className="flex gap-3">
              <span className="w-4 shrink-0 font-mono text-[#B4B9CC]">{i + 1}</span>
              <span className="min-w-0">{s}</span>
            </li>
          ))}
        </ol>
      </div>

      <Button variant="primary" className="w-full" icon={copied ? <Check className="h-4 w-4" /> : undefined} onClick={() => void copyMessage()}>
        {copied ? "已复制" : "复制邀请消息"}
      </Button>
      {canCollaborate && (
        <button
          type="button"
          onClick={onCollaborate}
          className="mt-3 inline-flex w-full items-center justify-center gap-1 text-[13px] text-[#5B6075] hover:text-[#2B3AE8]"
        >
          想让好友一起管理服务器？
          <span className="inline-flex items-center gap-0.5 font-medium text-[#2B3AE8]">
            邀请协作 <ArrowRight className="h-3.5 w-3.5" />
          </span>
        </button>
      )}
    </>
  );
}

function CollaborateView({
  open,
  serverName,
  onMemberAdded,
}: {
  open: boolean;
  serverName?: string;
  onMemberAdded?: () => void;
}) {
  const { tenant, tenantObj, apiFetch, toast } = useConsole();
  const [role, setRole] = useState<InviteRole>("instance-operator");
  const [username, setUsername] = useState("");
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, flash] = useFlash();
  const { url, loading } = useInviteLink(open, true, role, serverName);
  const roleMeta = INVITE_ROLES.find((r) => r.value === role) ?? INVITE_ROLES[0];
  const workspaceTitle = tenantObj?.spec.displayName || tenant;

  async function copyLink() {
    if (!url) return;
    if (!(await copyToClipboard(url))) {
      toast("error", "复制失败，请稍后重试");
      return;
    }
    flash();
    toast("success", `「${roleMeta.label}」邀请链接已复制`);
  }

  async function addMember(e: FormEvent) {
    e.preventDefault();
    const u = username.trim();
    if (!USERNAME_RE.test(u)) {
      setError("用户名格式不正确");
      return;
    }
    setAdding(true);
    setError(null);
    try {
      const res = await apiFetch(`/apis/iam.gamepanel.io/v1/tenants/${encodeURIComponent(tenant)}/tenantrolebindings`, {
        method: "POST",
        body: JSON.stringify({
          apiVersion: "iam.gamepanel.io/v1",
          kind: "TenantRoleBinding",
          metadata: { name: `member-${u}`, tenant },
          roleRef: { name: role },
          subjects: [{ kind: "User", name: u }],
        }),
      });
      if (!res.ok) throw new Error(await errorMessage(res, "添加失败"));
      toast("success", `已添加 ${u}（${roleMeta.label}）`);
      setUsername("");
      onMemberAdded?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : "添加失败");
    } finally {
      setAdding(false);
    }
  }

  return (
    <>
      <div className={BODY_HEIGHT}>
        <Label>加入「{workspaceTitle}」后的权限</Label>
        <div className="mt-2 [&>div]:grid [&>div]:w-full [&>div]:grid-cols-3 [&>div>button]:justify-center">
          <Segmented<InviteRole>
            label="协作权限"
            value={role}
            onChange={setRole}
            items={INVITE_ROLES.map((r) => ({ value: r.value, label: r.label }))}
          />
        </div>
        <p className="mt-2 text-xs text-[#5B6075]">{roleMeta.desc}</p>

        <form onSubmit={(e) => void addMember(e)} className="mt-6">
          <div className="flex items-center justify-between">
            <Label>添加已注册的好友</Label>
            {error && <span className="text-xs text-[#D92D20]">{error}</span>}
          </div>
          <div className="mt-2 flex gap-2">
            <Input
              placeholder="输入用户名"
              value={username}
              aria-invalid={!!error}
              onChange={(e) => {
                setUsername(e.target.value);
                if (error) setError(null);
              }}
            />
            <Button type="submit" loading={adding} disabled={!username.trim() || adding}>
              添加
            </Button>
          </div>
        </form>
      </div>

      <Button
        variant="primary"
        className="w-full"
        loading={loading}
        disabled={!url || loading}
        icon={copied ? <Check className="h-4 w-4" /> : undefined}
        onClick={() => void copyLink()}
      >
        {copied ? "已复制" : "复制邀请链接"}
      </Button>
      <p className="mt-3 text-center text-[13px] text-[#8A90A6]">还没账号的好友，打开链接注册后自动加入 · 7 天有效</p>
    </>
  );
}

export function InviteModal({
  open,
  onClose,
  inst,
  nodes = [],
  onMemberAdded,
}: {
  open: boolean;
  onClose: () => void;
  inst?: InstanceItem;
  nodes?: NodeItem[];
  onMemberAdded?: () => void;
}) {
  const { tenant, tenantObj, canManageTenant } = useConsole();
  const [view, setView] = useState<"play" | "collaborate">(inst ? "play" : "collaborate");
  const isPersonal = false;
  const canCollaborate = canManageTenant && !isPersonal;
  const workspaceTitle = tenantObj?.spec.displayName || tenant;

  useEffect(() => {
    if (open) setView(inst ? "play" : "collaborate");
  }, [open, inst]);

  const showBack = !!inst && view === "collaborate";
  const title = useMemo(
    () =>
      showBack ? (
        <span className="flex items-center gap-2">
          <button
            type="button"
            aria-label="返回"
            onClick={() => setView("play")}
            className="-ml-2 inline-flex h-8 w-8 items-center justify-center rounded-full text-[#5B6075] hover:bg-[#F4F5FA] hover:text-[#14161F]"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          邀请协作
        </span>
      ) : view === "play" ? (
        "邀请好友"
      ) : (
        "邀请协作"
      ),
    [showBack, view],
  );

  return (
    <Modal open={open} onClose={onClose} title={title} width="max-w-[440px]">
      <div className="pb-2 pt-1">
        {view === "play" && inst ? (
          <PlayView inst={inst} nodes={nodes} canCollaborate={canCollaborate} onCollaborate={() => setView("collaborate")} />
        ) : canCollaborate ? (
          <CollaborateView open={open} serverName={inst?.metadata.name} onMemberAdded={onMemberAdded} />
        ) : (
          <div className={`${BODY_HEIGHT} flex items-center justify-center text-center text-[13px] leading-relaxed text-[#5B6075]`}>
            {isPersonal
              ? `「${workspaceTitle}」是个人工作区，无法邀请协作成员。切换到团队工作区后再试。`
              : "只有工作区管理员可以邀请协作成员。"}
          </div>
        )}
      </div>
    </Modal>
  );
}
