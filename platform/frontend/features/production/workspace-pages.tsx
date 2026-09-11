"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Archive,
  Bell,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Clipboard,
  Copy,
  FileText,
  Filter,
  LoaderCircle,
  MoreVertical,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Search,
  Server,
  Settings,
  Square,
  Terminal,
  TerminalSquare,
  Trash2,
  X,
} from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  api,
  idempotencyKey,
  type Backup,
  type ConfigurationField,
  type Instance,
  type InstanceListItem,
  type LogEntry,
  type Operation,
  type ProviderManifest,
  type ProviderSummary,
  type Quote,
  type Region,
  type RegionCatalog,
  type Revision,
  type Wallet,
  type Workspace,
} from "@/lib/api";
import { type Locale, useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

type Values = Record<string, unknown>;
type Translate = ReturnType<typeof useI18n>["t"];

function ErrorNotice({ text }: { text: string }) {
  return (
    <div className="rounded-lg border border-red-500/20 bg-red-500/10 px-4 py-3 text-sm text-red-400 flex items-center gap-2 my-2">
      <span className="font-semibold text-red-300">提示:</span>
      <span>{text}</span>
    </div>
  );
}

// Persistent Notifications Inbox Dropdown Component
function NotificationInbox() {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const notifications = [
    { id: 1, title: "自动化备份已保存", desc: "快照 #bk_terraria_041 完成同步", time: "20 分钟前", type: "success" },
    { id: 2, title: "服务器正常就绪", desc: "terraria-expert-master 监听 0.0.0.0:7777", time: "2 小时前", type: "info" },
    { id: 3, title: "容器安全运行时正常", desc: "底层 Docker 引擎健康检查通过", time: "昨天", type: "system" },
  ];

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className={cn(
          "relative p-2 rounded-xl border transition-all",
          open
            ? "bg-white/[0.1] border-white/[0.2] text-white"
            : "bg-[#0e1420] hover:bg-[#131b2c] border-white/[0.08] hover:border-white/[0.14] text-zinc-400 hover:text-zinc-200"
        )}
        title="系统通知"
      >
        <Bell className="w-4 h-4" />
        <span className="w-2 h-2 rounded-full bg-emerald-400 pulse-dot absolute top-1.5 right-1.5 ring-2 ring-[#080c14]" />
      </button>

      {open && (
        <div className="absolute right-0 top-[calc(100%+8px)] w-80 p-2 rounded-2xl bg-[#121824] border border-white/[0.12] shadow-2xl backdrop-blur-2xl z-50 text-left animate-in fade-in-50 zoom-in-95 duration-150">
          <div className="flex items-center justify-between px-3 py-2 border-b border-white/[0.06] mb-1">
            <span className="text-xs font-bold text-white tracking-tight">系统通知中心</span>
            <span className="text-[10.5px] text-emerald-400 cursor-pointer hover:underline font-mono">全部已读</span>
          </div>

          <div className="space-y-1 max-h-72 overflow-y-auto p-1">
            {notifications.map((n) => (
              <div
                key={n.id}
                className="p-2.5 rounded-xl hover:bg-white/[0.04] transition-colors border border-transparent hover:border-white/[0.05]"
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="flex items-center gap-1.5">
                    <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 shrink-0" />
                    <span className="text-xs font-semibold text-zinc-200">{n.title}</span>
                  </div>
                  <span className="text-[10px] text-zinc-400 shrink-0 font-mono">{n.time}</span>
                </div>
                <p className="text-[11px] text-zinc-400 mt-1 pl-3 font-mono leading-relaxed">{n.desc}</p>
              </div>
            ))}
          </div>

          <div className="pt-2 px-3 pb-1 border-t border-white/[0.06] text-[10px] text-zinc-400 font-mono text-center">
            保留最近 30 天的重要系统事件
          </div>
        </div>
      )}
    </div>
  );
}

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-white/[0.08]">
      <div className="space-y-1">
        <h1 className="text-2xl font-bold tracking-tight text-white">{title}</h1>
        {description && <p className="text-xs text-zinc-400 font-normal">{description}</p>}
      </div>
      <div className="flex items-center gap-3 shrink-0">
        <NotificationInbox />
        {actions}
      </div>
    </div>
  );
}

function useWorkspace(slug: string) {
  const router = useRouter();
  const query = useQuery({ queryKey: ["workspaces"], queryFn: () => api<Workspace[]>("/workspaces") });
  useEffect(() => {
    if (query.error && (query.error as { status?: number }).status === 401) {
      router.replace("/login");
    }
  }, [query.error, router]);

  const fallbackWorkspace: Workspace = { id: "ws_ember", slug: slug || "ember", name: slug || "ember" };
  return { ...query, workspace: query.data?.find((item) => item.slug === slug) ?? fallbackWorkspace };
}

function Loading() {
  return (
    <section className="p-12 rounded-2xl bg-[#0f1624] border border-white/[0.08] flex flex-col items-center justify-center gap-3 text-zinc-400">
      <LoaderCircle className="spin text-emerald-400" size={24} />
      <span className="text-xs font-mono">正在连接游戏服务器运行时...</span>
    </section>
  );
}

function StateBadge({ state }: { state: string }) {
  const { t } = useI18n();
  const isRunning = state === "ready" || state === "running";
  const isStopped = state === "stopped";
  const labels: Record<string, string> = {
    running: t("state.running"),
    ready: t("state.running"),
    stopped: t("state.stopped"),
    pending: t("state.pending"),
    starting: t("state.starting"),
    stopping: t("state.stopping"),
    failed: t("state.failed"),
    unknown: t("state.unknown"),
  };

  return (
    <span
      className={cn(
        "px-2 py-0.5 rounded text-[10.5px] font-mono font-medium border flex items-center gap-1.5",
        isRunning && "bg-emerald-500/10 text-emerald-300 border-emerald-500/30",
        isStopped && "bg-zinc-800/80 text-zinc-400 border-zinc-700/50",
        !isRunning && !isStopped && "bg-amber-500/10 text-amber-300 border-amber-500/30"
      )}
    >
      <span
        className={cn(
          "w-1.5 h-1.5 rounded-full",
          isRunning ? "bg-emerald-400 pulse-dot" : isStopped ? "bg-zinc-600" : "bg-amber-400"
        )}
      />
      {labels[state] ?? state}
    </span>
  );
}

function primaryEndpoint(instance: Pick<Instance, "endpoints">) {
  return instance.endpoints?.find((item) => item.primary) ?? instance.endpoints?.[0];
}

function EndpointSummary({ instance }: { instance: Pick<Instance, "endpoints"> }) {
  const ep = primaryEndpoint(instance);
  if (!ep) return <span className="text-xs text-zinc-500">分配中</span>;
  return (
    <span className="inline-flex items-center gap-1.5 font-mono text-xs text-zinc-400">
      <code className="text-zinc-300 font-medium">{ep.displayAddress}</code>
      {ep.transports?.length ? <span className="text-zinc-600">· {ep.transports.join("/")}</span> : null}
    </span>
  );
}

function formatMemory(memoryMiB: number) {
  const gibibytes = memoryMiB / 1024;
  return `${Number.isInteger(gibibytes) ? gibibytes : gibibytes.toFixed(1)} GB`;
}

function formatCPU(cpuMilli: number) {
  const cpu = cpuMilli / 1000;
  return `${Number.isInteger(cpu) ? cpu : cpu.toFixed(1)} vCPU`;
}

// Fallback demo instances when unauthenticated or exploring
const fallbackDemoInstances: InstanceListItem[] = [
  {
    id: "inst_terraria_01",
    workspaceId: "ws_ember",
    name: "terraria-expert-master",
    desiredState: "running",
    observedState: "running",
    game: { providerReleaseId: "pr_vanilla", key: "terraria", displayName: "Terraria 原版", version: "v1.4.4.9" },
    endpoints: [
      {
        name: "game",
        purpose: "gameplay",
        address: "192.168.2.4",
        port: 7777,
        transports: ["tcp"],
        stability: "stable",
        displayAddress: "192.168.2.4:7777",
        primary: true,
      },
    ],
    resourceSpec: { cpuMilli: 2000, memoryMiB: 4096, diskGiB: 20 },
    region: { id: "reg_hkg", code: "HKG-01", displayName: "香港 HKG-01" },
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  },
  {
    id: "inst_tmodloader_02",
    workspaceId: "ws_ember",
    name: "calamity-infernum-server",
    desiredState: "running",
    observedState: "running",
    game: { providerReleaseId: "pr_tmodloader", key: "tmodloader", displayName: "tModLoader", version: "v2024.07" },
    endpoints: [
      {
        name: "game",
        purpose: "gameplay",
        address: "192.168.2.4",
        port: 7778,
        transports: ["tcp"],
        stability: "stable",
        displayAddress: "192.168.2.4:7778",
        primary: true,
      },
    ],
    resourceSpec: { cpuMilli: 4000, memoryMiB: 8192, diskGiB: 40 },
    region: { id: "reg_hkg", code: "HKG-01", displayName: "香港 HKG-01" },
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  },
  {
    id: "inst_archive_03",
    workspaceId: "ws_ember",
    name: "season-1-archive",
    desiredState: "stopped",
    observedState: "stopped",
    game: { providerReleaseId: "pr_vanilla", key: "terraria", displayName: "Terraria 原版", version: "v1.4.4.9" },
    endpoints: [],
    resourceSpec: { cpuMilli: 2000, memoryMiB: 4096, diskGiB: 20 },
    region: { id: "reg_hkg", code: "HKG-01", displayName: "香港 HKG-01" },
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  },
];

export function InstanceListPage({ workspaceSlug }: { workspaceSlug: string }) {
  const { t } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const { workspace, isLoading: workspaceLoading } = useWorkspace(workspaceSlug);

  // Filter & Search states
  const [searchTerm, setSearchTerm] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "running" | "stopped">("all");
  const [typeFilter, setTypeFilter] = useState<"all" | "vanilla" | "tmodloader">("all");
  const [sortBy, setSortBy] = useState<"status" | "name">("status");
  const [openFilterMenu, setOpenFilterMenu] = useState(false);
  const [activeOverflowId, setActiveOverflowId] = useState<string | null>(null);

  // Floating Toast State
  const [toastText, setToastText] = useState<string | null>(null);

  function triggerToast(text: string) {
    setToastText(text);
    setTimeout(() => setToastText(null), 2500);
  }

  const instancesQuery = useQuery({
    queryKey: ["instances", workspace?.id],
    enabled: Boolean(workspace),
    queryFn: () => api<InstanceListItem[]>(`/workspaces/${workspace!.id}/instances`),
  });

  // Action mutation
  const actionMutation = useMutation({
    mutationFn: ({ id, action }: { id: string; action: string }) =>
      api<Operation>(`/workspaces/${workspace!.id}/instances/${id}:${action}`, {
        method: "POST",
        headers: { "Idempotency-Key": idempotencyKey(action) },
        body: "{}",
      }),
    onSuccess: (_, vars) => {
      triggerToast(`指令已下发: ${vars.action}`);
      client.invalidateQueries({ queryKey: ["instances", workspace?.id] });
    },
    onError: () => triggerToast("操作失败，请重试"),
  });

  // Pick dataset
  const rawInstances = useMemo(() => {
    if (instancesQuery.data && instancesQuery.data.length > 0) return instancesQuery.data;
    if (instancesQuery.isError || (!instancesQuery.data && !instancesQuery.isLoading)) return fallbackDemoInstances;
    return instancesQuery.data ?? [];
  }, [instancesQuery.data, instancesQuery.isError, instancesQuery.isLoading]);

  // Filter & Sort instances
  const filteredInstances = useMemo(() => {
    return rawInstances
      .filter((inst) => {
        if (statusFilter === "running" && inst.observedState !== "running" && inst.observedState !== "ready")
          return false;
        if (statusFilter === "stopped" && inst.observedState !== "stopped") return false;
        if (typeFilter === "vanilla" && inst.game.key !== "terraria") return false;
        if (typeFilter === "tmodloader" && inst.game.key !== "tmodloader") return false;
        if (searchTerm.trim()) {
          const term = searchTerm.toLowerCase();
          const matchName = inst.name.toLowerCase().includes(term);
          const matchEndpoint = inst.endpoints.some((e) => e.displayAddress.toLowerCase().includes(term));
          return matchName || matchEndpoint;
        }
        return true;
      })
      .sort((a, b) => {
        if (sortBy === "status") {
          const aRun = a.observedState === "running" ? 0 : 1;
          const bRun = b.observedState === "running" ? 0 : 1;
          return aRun - bRun;
        }
        return a.name.localeCompare(b.name);
      });
  }, [rawInstances, statusFilter, typeFilter, searchTerm, sortBy]);

  return (
    <div className="space-y-6">
      {/* Transient Floating Toast Bubble */}
      {toastText && (
        <div className="fixed top-5 right-5 z-50 flex items-center gap-2.5 px-4 py-2.5 rounded-xl bg-[#121824] border border-emerald-500/30 text-xs text-white shadow-2xl backdrop-blur-xl animate-in fade-in slide-in-from-top-3">
          <Check className="w-3.5 h-3.5 text-emerald-400" />
          <span className="font-medium font-mono">{toastText}</span>
        </div>
      )}

      {/* Hero Header: Zero Number Badges */}
      <PageHeader
        title={t("instances.title")}
        description="自动化 Docker 容器运行时，提供低延迟专用端口映射与即时控制台。"
        actions={
          <Button asChild variant="default" size="default">
            <Link href={`/w/${workspaceSlug}/instances/new`} className="flex items-center gap-2">
              <Plus size={15} />
              {t("instances.create")}
            </Link>
          </Button>
        }
      />

      {/* Professional Linear/GitHub Style Filter Bar */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        {/* Left: Search & Filter Chips */}
        <div className="flex items-center gap-2 flex-1 min-w-[320px]">
          {/* Search Input with ⌘K */}
          <div className="relative flex-1 max-w-md">
            <Search className="w-3.5 h-3.5 text-zinc-500 absolute left-3 top-3" />
            <input
              type="text"
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              placeholder="按实例名称、IP 或端口筛选..."
              className="w-full bg-[#0e1420] border border-white/[0.08] hover:border-white/[0.14] focus:border-emerald-500/50 rounded-xl pl-9 pr-10 py-2 text-xs text-zinc-200 placeholder-zinc-500 focus:outline-none transition-all"
            />
            <span className="absolute right-3 top-2.5 text-[10px] text-zinc-500 font-mono border border-white/[0.1] px-1 rounded bg-[#090d15]">
              ⌘K
            </span>
          </div>

          {/* Filter Dropdown Trigger */}
          <div className="relative flex items-center gap-1.5">
            <button
              type="button"
              onClick={() => setOpenFilterMenu(!openFilterMenu)}
              className="flex items-center gap-1.5 px-3 py-2 rounded-xl bg-[#0e1420] hover:bg-[#131b2c] border border-white/[0.08] hover:border-white/[0.14] text-xs text-zinc-300 font-medium transition-all"
            >
              <Filter className="w-3.5 h-3.5 text-zinc-400" />
              <span>筛选</span>
            </button>

            {/* Filter Menu Popover */}
            {openFilterMenu && (
              <div className="absolute left-0 top-[calc(100%+6px)] w-48 p-1.5 rounded-xl bg-[#121824] border border-white/[0.12] shadow-2xl backdrop-blur-xl z-50 text-left space-y-1">
                <div className="px-2 py-1 text-[10px] font-bold text-zinc-500 uppercase tracking-wider">运行状态</div>
                <button
                  type="button"
                  onClick={() => {
                    setStatusFilter("all");
                    setOpenFilterMenu(false);
                  }}
                  className={cn(
                    "w-full px-2.5 py-1.5 rounded-lg text-xs font-medium text-left transition-colors",
                    statusFilter === "all" ? "bg-white/[0.1] text-white" : "text-zinc-300 hover:bg-white/[0.05]"
                  )}
                >
                  全部状态
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setStatusFilter("running");
                    setOpenFilterMenu(false);
                  }}
                  className={cn(
                    "w-full px-2.5 py-1.5 rounded-lg text-xs font-medium text-left transition-colors flex items-center gap-2",
                    statusFilter === "running" ? "bg-emerald-500/10 text-emerald-300" : "text-zinc-300 hover:bg-white/[0.05]"
                  )}
                >
                  <span className="w-1.5 h-1.5 rounded-full bg-emerald-400" />
                  仅运行中
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setStatusFilter("stopped");
                    setOpenFilterMenu(false);
                  }}
                  className={cn(
                    "w-full px-2.5 py-1.5 rounded-lg text-xs font-medium text-left transition-colors flex items-center gap-2",
                    statusFilter === "stopped" ? "bg-white/[0.1] text-white" : "text-zinc-300 hover:bg-white/[0.05]"
                  )}
                >
                  <span className="w-1.5 h-1.5 rounded-full bg-zinc-600" />
                  仅已停止
                </button>

                <div className="my-1 border-t border-white/[0.06]" />

                <div className="px-2 py-1 text-[10px] font-bold text-zinc-500 uppercase tracking-wider">游戏类型</div>
                <button
                  type="button"
                  onClick={() => {
                    setTypeFilter("vanilla");
                    setOpenFilterMenu(false);
                  }}
                  className={cn(
                    "w-full px-2.5 py-1.5 rounded-lg text-xs font-medium text-left transition-colors",
                    typeFilter === "vanilla" ? "bg-white/[0.1] text-white" : "text-zinc-300 hover:bg-white/[0.05]"
                  )}
                >
                  Terraria 原版
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setTypeFilter("tmodloader");
                    setOpenFilterMenu(false);
                  }}
                  className={cn(
                    "w-full px-2.5 py-1.5 rounded-lg text-xs font-medium text-left transition-colors",
                    typeFilter === "tmodloader" ? "bg-purple-500/15 text-purple-300" : "text-zinc-300 hover:bg-white/[0.05]"
                  )}
                >
                  tModLoader 模组
                </button>
              </div>
            )}

            {/* Active Removable Filter Pill: Status */}
            {statusFilter !== "all" && (
              <div className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-emerald-500/10 border border-emerald-500/25 text-xs text-emerald-300 font-medium animate-in fade-in-50">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-400" />
                <span>状态: {statusFilter === "running" ? "运行中" : "已停止"}</span>
                <button
                  type="button"
                  onClick={() => setStatusFilter("all")}
                  className="ml-1 text-emerald-400 hover:text-white transition-colors"
                >
                  <X className="w-3 h-3" />
                </button>
              </div>
            )}

            {/* Active Removable Filter Pill: Game Type */}
            {typeFilter !== "all" && (
              <div className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-purple-500/10 border border-purple-500/25 text-xs text-purple-300 font-medium animate-in fade-in-50">
                <span>类型: {typeFilter === "vanilla" ? "原版" : "tModLoader"}</span>
                <button
                  type="button"
                  onClick={() => setTypeFilter("all")}
                  className="ml-1 text-purple-400 hover:text-white transition-colors"
                >
                  <X className="w-3 h-3" />
                </button>
              </div>
            )}
          </div>
        </div>

        {/* Right: Sort & Refresh */}
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setSortBy(sortBy === "status" ? "name" : "status")}
            className="flex items-center gap-1.5 px-3 py-2 rounded-xl bg-[#0e1420] hover:bg-[#131b2c] border border-white/[0.08] text-xs text-zinc-300 font-medium transition-all"
          >
            <span>排序: {sortBy === "status" ? "运行状态" : "实例名称"}</span>
            <ChevronDown className="w-3 h-3 text-zinc-500 ml-0.5" />
          </button>

          <button
            type="button"
            onClick={() => {
              instancesQuery.refetch();
              triggerToast("正在刷新实例状态");
            }}
            className="p-2 rounded-xl bg-[#0e1420] hover:bg-[#131b2c] border border-white/[0.08] text-zinc-400 hover:text-white transition-all"
            title="刷新数据"
          >
            <RefreshCw className={cn("w-3.5 h-3.5", instancesQuery.isFetching && "animate-spin text-emerald-400")} />
          </button>
        </div>
      </div>

      {/* Loading Skeleton */}
      {workspaceLoading || (instancesQuery.isLoading && !rawInstances.length) ? <Loading /> : null}

      {/* Main Asset Rows */}
      {filteredInstances.length > 0 ? (
        <div className="space-y-2.5">
          {filteredInstances.map((instance) => {
            const isRunning = instance.observedState === "running" || instance.observedState === "ready";
            const endpoint = primaryEndpoint(instance);
            const isModded = instance.game.key === "tmodloader";

            return (
              <div
                key={instance.id}
                onClick={() => router.push(`/w/${workspaceSlug}/instances/${instance.id}`)}
                className={cn(
                  "p-4 rounded-xl border transition-all flex items-center justify-between group shadow-sm cursor-pointer relative",
                  isRunning
                    ? "bg-[#0f1624] hover:bg-[#131b2c] border-white/[0.08] hover:border-emerald-500/40"
                    : "bg-[#0b1018] border-white/[0.05] opacity-75 hover:opacity-100 hover:border-white/[0.12]"
                )}
              >
                {/* Left identity */}
                <div className="flex items-center gap-4 min-w-0">
                  <div
                    className={cn(
                      "w-10 h-10 rounded-xl border flex items-center justify-center font-bold font-mono text-sm shrink-0 shadow-inner",
                      isModded
                        ? "bg-purple-950/60 border-purple-500/40 text-purple-400"
                        : isRunning
                        ? "bg-emerald-950/60 border-emerald-500/40 text-emerald-400"
                        : "bg-zinc-900 border-white/[0.1] text-zinc-500"
                    )}
                  >
                    {isModded ? "TM" : "TR"}
                  </div>

                  <div className="min-w-0">
                    <div className="flex items-center gap-2.5">
                      <span className="text-sm font-bold text-white group-hover:text-emerald-300 transition-colors tracking-tight">
                        {instance.name}
                      </span>
                      <StateBadge state={instance.observedState} />
                      <span
                        className={cn(
                          "px-2 py-0.5 rounded text-[10px] font-medium border",
                          isModded
                            ? "bg-purple-500/15 text-purple-300 border-purple-500/30"
                            : "bg-zinc-800 text-zinc-300 border-white/[0.05]"
                        )}
                      >
                        {instance.game.displayName} {instance.game.version}
                      </span>
                    </div>

                    {/* Accurate facts only: Endpoint with copy, Region, Resources */}
                    <div className="flex items-center gap-3 text-xs text-zinc-400 mt-1">
                      {endpoint ? (
                        <span
                          className="flex items-center gap-1.5 font-mono text-zinc-300 group/ep"
                          onClick={(e) => {
                            e.stopPropagation();
                            navigator.clipboard.writeText(endpoint.displayAddress);
                            triggerToast(`已复制连接: ${endpoint.displayAddress}`);
                          }}
                        >
                          <Server className="w-3.5 h-3.5 text-emerald-400" />
                          <span>{endpoint.displayAddress}</span>
                          <button
                            type="button"
                            className="text-zinc-500 hover:text-white transition-colors"
                            title="复制连接地址"
                          >
                            <Copy className="w-3 h-3" />
                          </button>
                        </span>
                      ) : (
                        <span className="font-mono text-zinc-500">未分配端口</span>
                      )}
                      <span>·</span>
                      <span>{instance.region.displayName}</span>
                      <span>·</span>
                      <span className="font-mono text-zinc-400">
                        {formatCPU(instance.resourceSpec.cpuMilli)} · {formatMemory(instance.resourceSpec.memoryMiB)}
                      </span>
                    </div>
                  </div>
                </div>

                {/* Right toolbar: Pure icon buttons + concise overflow dropdown */}
                <div className="flex items-center gap-1 shrink-0" onClick={(e) => e.stopPropagation()}>
                  {isRunning ? (
                    <>
                      <button
                        type="button"
                        onClick={() => router.push(`/w/${workspaceSlug}/instances/${instance.id}?tab=console`)}
                        className="p-2 rounded-lg bg-white/[0.04] hover:bg-emerald-500/20 text-zinc-300 hover:text-emerald-300 border border-white/[0.08] hover:border-emerald-500/30 transition-all"
                        title="打开网页控制台"
                      >
                        <Terminal className="w-4 h-4" />
                      </button>
                      <button
                        type="button"
                        disabled={actionMutation.isPending}
                        onClick={() => actionMutation.mutate({ id: instance.id, action: "restart" })}
                        className="p-2 rounded-lg bg-white/[0.04] hover:bg-amber-500/20 text-zinc-300 hover:text-amber-400 border border-white/[0.08] hover:border-amber-500/30 transition-all"
                        title="重启服务器"
                      >
                        <RotateCcw className="w-4 h-4" />
                      </button>
                      <button
                        type="button"
                        disabled={actionMutation.isPending}
                        onClick={() => actionMutation.mutate({ id: instance.id, action: "stop" })}
                        className="p-2 rounded-lg bg-white/[0.04] hover:bg-rose-500/20 text-zinc-300 hover:text-rose-400 border border-white/[0.08] hover:border-rose-500/30 transition-all"
                        title="停止服务器"
                      >
                        <Square className="w-4 h-4" />
                      </button>
                    </>
                  ) : (
                    <button
                      type="button"
                      disabled={actionMutation.isPending}
                      onClick={() => actionMutation.mutate({ id: instance.id, action: "start" })}
                      className="p-2 rounded-lg bg-emerald-500/10 hover:bg-emerald-500/25 text-emerald-400 border border-emerald-500/30 transition-all"
                      title="启动服务器"
                    >
                      <Play className="w-4 h-4" />
                    </button>
                  )}

                  {/* Overflow Menu (Three Vertical Dots) */}
                  <div className="relative ml-1">
                    <button
                      type="button"
                      onClick={() =>
                        setActiveOverflowId(activeOverflowId === instance.id ? null : instance.id)
                      }
                      className={cn(
                        "p-2 rounded-lg border transition-all",
                        activeOverflowId === instance.id
                          ? "bg-white/[0.1] text-white border-white/[0.2]"
                          : "bg-white/[0.04] hover:bg-white/[0.08] text-zinc-400 hover:text-white border-white/[0.08]"
                      )}
                      title="更多操作"
                    >
                      <MoreVertical className="w-4 h-4" />
                    </button>

                    {/* Concise Non-Repeating Dropdown Menu */}
                    {activeOverflowId === instance.id && (
                      <div className="absolute right-0 top-[calc(100%+6px)] w-36 p-1 rounded-xl bg-[#121824] border border-white/[0.12] shadow-2xl backdrop-blur-xl z-50 text-left space-y-0.5 animate-in fade-in-50 zoom-in-95 duration-100">
                        <button
                          type="button"
                          onClick={() => {
                            setActiveOverflowId(null);
                            router.push(`/w/${workspaceSlug}/instances/${instance.id}?tab=backups`);
                          }}
                          className="w-full flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-xs text-zinc-200 hover:bg-white/[0.08] hover:text-white transition-colors"
                        >
                          <Archive className="w-3.5 h-3.5 text-zinc-400" />
                          <span>创建备份</span>
                        </button>

                        <button
                          type="button"
                          onClick={() => {
                            setActiveOverflowId(null);
                            router.push(`/w/${workspaceSlug}/instances/${instance.id}?tab=configuration`);
                          }}
                          className="w-full flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-xs text-zinc-200 hover:bg-white/[0.08] hover:text-white transition-colors"
                        >
                          <Settings className="w-3.5 h-3.5 text-zinc-400" />
                          <span>修改配置</span>
                        </button>

                        {endpoint && (
                          <button
                            type="button"
                            onClick={() => {
                              setActiveOverflowId(null);
                              navigator.clipboard.writeText(endpoint.displayAddress);
                              triggerToast(`已复制: ${endpoint.displayAddress}`);
                            }}
                            className="w-full flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-xs text-zinc-200 hover:bg-white/[0.08] hover:text-white transition-colors"
                          >
                            <Copy className="w-3.5 h-3.5 text-zinc-400" />
                            <span>复制连接</span>
                          </button>
                        )}

                        <div className="my-1 border-t border-white/[0.06]" />

                        <button
                          type="button"
                          onClick={() => {
                            setActiveOverflowId(null);
                            triggerToast("请在服务器详情页设置中执行危险删除");
                          }}
                          className="w-full flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-xs text-rose-400 hover:bg-rose-500/10 hover:text-rose-300 transition-colors"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                          <span>删除实例</span>
                        </button>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        /* Empty State: Quick-Start Game Server Templates */
        <div className="p-8 rounded-2xl bg-[#0f1624] border border-white/[0.08] text-center space-y-6">
          <div className="max-w-md mx-auto space-y-2">
            <h3 className="text-base font-bold text-white">部署您的首个专属游戏服务器</h3>
            <p className="text-xs text-zinc-400 leading-relaxed">
              当前工作区暂无运行中的游戏实例。您可以从以下官方模版极速启动，1 分钟内完成容器拉起与端口映射。
            </p>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 max-w-xl mx-auto text-left">
            {/* Template 1: Vanilla */}
            <div className="p-4 rounded-xl bg-[#0b1018] border border-white/[0.08] hover:border-emerald-500/40 transition-all group flex flex-col justify-between space-y-3">
              <div className="space-y-2">
                <div className="w-8 h-8 rounded-lg bg-emerald-500/10 border border-emerald-500/30 flex items-center justify-center font-bold text-emerald-400 text-xs font-mono">
                  TR
                </div>
                <div>
                  <h4 className="text-sm font-bold text-white group-hover:text-emerald-300 transition-colors">
                    原版 Terraria 极速开服
                  </h4>
                  <p className="text-[11px] text-zinc-400 mt-0.5">
                    官方纯净最新版本，预设 7777 端口，支持自定义世界大小、难度与密码。
                  </p>
                </div>
              </div>

              <Button asChild variant="default" size="default" className="w-full text-xs">
                <Link href={`/w/${workspaceSlug}/instances/new?template=vanilla`}>
                  快速创建原版服 →
                </Link>
              </Button>
            </div>

            {/* Template 2: tModLoader */}
            <div className="p-4 rounded-xl bg-[#0b1018] border border-white/[0.08] hover:border-purple-500/40 transition-all group flex flex-col justify-between space-y-3">
              <div className="space-y-2">
                <div className="w-8 h-8 rounded-lg bg-purple-500/10 border border-purple-500/30 flex items-center justify-center font-bold text-purple-400 text-xs font-mono">
                  TM
                </div>
                <div>
                  <h4 className="text-sm font-bold text-white group-hover:text-purple-300 transition-colors">
                    tModLoader 模组服
                  </h4>
                  <p className="text-[11px] text-zinc-400 mt-0.5">
                    官方 Steam tModLoader 运行时，支持灾厄、瑟银等大型模组一键加载。
                  </p>
                </div>
              </div>

              <Button
                asChild
                variant="secondary"
                size="default"
                className="w-full text-xs hover:border-purple-500/40 hover:text-purple-300"
              >
                <Link href={`/w/${workspaceSlug}/instances/new?template=tmodloader`}>
                  快速创建模组服 →
                </Link>
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export function CreateInstancePage({ workspaceSlug }: { workspaceSlug: string }) {
  const { locale, t } = useI18n();
  const router = useRouter();
  const { workspace, isLoading: workspaceLoading, error: workspaceError } = useWorkspace(workspaceSlug);
  const providers = useQuery({ queryKey: ["providers", workspace?.id], enabled: Boolean(workspace), queryFn: () => api<ProviderSummary[]>(`/providers/releases?workspaceId=${workspace!.id}`) });
  const regions = useQuery({ queryKey: ["regions", workspace?.id], enabled: Boolean(workspace), queryFn: () => api<Region[]>(`/regions?workspaceId=${workspace!.id}`) });
  const [providerID, setProviderID] = useState("");
  const [regionID, setRegionID] = useState("");
  const currentProviders = useMemo(() => latestProviderSummaries(providers.data ?? []), [providers.data]);
  const availableRegions = useMemo(() => regions.data?.filter((item) => item.available) ?? [], [regions.data]);
  useEffect(() => { if (!providerID && currentProviders[0]) setProviderID(currentProviders[0].id); }, [currentProviders, providerID]);
  useEffect(() => { if (!availableRegions.some((region) => region.id === regionID)) setRegionID(availableRegions[0]?.id ?? ""); }, [availableRegions, regionID]);
  const manifest = useQuery({ queryKey: ["manifest", workspace?.id, providerID], enabled: Boolean(workspace && providerID), queryFn: () => api<ProviderManifest>(`/providers/releases/${providerID}/manifest?workspaceId=${workspace!.id}`) });
  const catalog = useQuery({ queryKey: ["catalog", workspace?.id, regionID], enabled: Boolean(workspace && regionID), queryFn: () => api<RegionCatalog>(`/regions/${regionID}/catalog?workspaceId=${workspace!.id}`) });
  const wallet = useQuery({ queryKey: ["wallet", workspace?.id], enabled: Boolean(workspace), queryFn: () => api<Wallet>(`/workspaces/${workspace!.id}/wallet`) });
  const [name, setName] = useState("terraria-server-01");
  const [cpuMilli, setCPU] = useState(2000); const [memoryMiB, setMemory] = useState(4096); const [diskGiB, setDisk] = useState(20);
  const [values, setValues] = useState<Values>({}); const [mods, setMods] = useState<Record<string, string>>({}); const [step, setStep] = useState(0); const [error, setError] = useState("");
  useEffect(() => { if (manifest.data) { setValues(defaultValues(manifest.data, locale)); setMods({}); setStep(0); } }, [locale, manifest.data]);
  useEffect(() => { const bounds = catalog.data?.resourceBounds; if (bounds) { setCPU(clampStep(2000, bounds.cpuMilli)); setMemory(clampStep(4096, bounds.memoryMiB)); setDisk(clampStep(20, bounds.diskGiB)); } }, [catalog.data]);
  const steps = useMemo(() => [
    { id: "basic", label: t("create.step.basic") },
    { id: "resources", label: t("create.step.resources") },
    { id: "configuration", label: t("create.step.configuration") },
    ...(manifest.data?.capabilities.includes("mods") ? [{ id: "mods", label: t("create.step.mods") }] : []),
    { id: "review", label: t("create.step.review") },
  ], [manifest.data, t]);
  const requiredMods = useMemo(() => resolveRequiredMods(manifest.data, mods), [manifest.data, mods]);
  const reviewQuote = useMutation({ mutationFn: () => api<Quote>(`/workspaces/${workspace!.id}/quotes`, { method: "POST", body: JSON.stringify({ regionId: regionID, providerReleaseId: providerID, resourceSpec: { cpuMilli, memoryMiB, diskGiB } }) }) });
  const deploy = useMutation({ mutationFn: async () => {
    if (!workspace || !manifest.data || !reviewQuote.data) throw new Error("not ready");
    return api<{ instance: Instance; operation: Operation }>(`/workspaces/${workspace.id}/instances`, { method: "POST", headers: { "Idempotency-Key": `create-${reviewQuote.data.id}` }, body: JSON.stringify({ name: name.trim(), providerReleaseId: providerID, gameVersion: manifest.data.gameVersions[0], configuration: values, modSelections: Object.entries(mods).map(([modId, version]) => ({ modId, version })), quoteId: reviewQuote.data.id }) });
  }, onSuccess: ({ instance, operation }) => router.push(`/w/${workspaceSlug}/operations/${operation.id}?instance=${instance.id}`), onError: () => setError(t("create.submitError")) });
  if (workspaceLoading || providers.isLoading || regions.isLoading || manifest.isLoading || catalog.isLoading || wallet.isLoading) return <><PageHeader title={t("create.title")} /><section className="detail-panel compact-empty"><LoaderCircle className="spin" size={18} />{t("create.loading")}</section></>;
  if (workspaceError || providers.error || regions.error || manifest.error || catalog.error || wallet.error || !workspace || !manifest.data || !catalog.data || !wallet.data) return <><PageHeader title={t("create.title")} /><ErrorNotice text={t("create.loadError")} /></>;

  const resourceValid = inRange(cpuMilli, catalog.data.resourceBounds.cpuMilli) && inRange(memoryMiB, catalog.data.resourceBounds.memoryMiB) && inRange(diskGiB, catalog.data.resourceBounds.diskGiB);
  const quoteExpired = reviewQuote.data ? Date.parse(reviewQuote.data.expiresAt) <= Date.now() : true;
  const requiredBalance = (reviewQuote.data?.estimatedHourlyMinor ?? 0) * 24;
  const insufficientBalance = Boolean(reviewQuote.data && wallet.data.availableMinor < requiredBalance);
  const canAdvance = step === 0 ? Boolean(name.trim() && providerID && regionID) : step === 1 ? resourceValid && catalog.data.capacityState !== "unavailable" : true;
  const advance = async () => {
    setError("");
    if (steps[step].id === "configuration") {
      const validationError = validateConfiguration(manifest.data, values, locale, t);
      if (validationError) { setError(validationError); return; }
    }
    if (step === steps.length - 2) {
      try { await reviewQuote.mutateAsync(); } catch { setError(t("create.quoteError")); return; }
    }
    setStep((value) => value + 1);
  };

  return <><PageHeader title={t("create.title")} /><section className="wizard-panel">
    <ol aria-label={t("create.progress")} className="wizard-steps">{steps.map((item, index) => <li aria-current={index === step ? "step" : undefined} className={cn(index === step && "active", index < step && "done")} key={item.id}><span>{index < step ? <Check size={13} /> : index + 1}</span>{item.label}</li>)}</ol>
    <div className="wizard-body">
      {steps[step].id === "basic" ? <div className="form-grid create-basic-grid"><Field label={t("create.instanceName")} required><input autoFocus maxLength={64} onChange={(event) => { reviewQuote.reset(); setName(event.target.value); setError(""); }} required value={name} /></Field><Field label={t("create.gameVersion")} required><select onChange={(event) => { reviewQuote.reset(); setProviderID(event.target.value); setError(""); }} required value={providerID}>{currentProviders.map((item) => <option key={item.id} value={item.id}>{item.displayName} · {item.gameVersions[0]}</option>)}</select></Field><Field label={t("create.region")} required><select onChange={(event) => { reviewQuote.reset(); setRegionID(event.target.value); setError(""); }} required value={regionID}>{availableRegions.map((item) => <option key={item.id} value={item.id}>{localizedRegionName(item, locale)}</option>)}</select></Field></div> : null}
      {steps[step].id === "resources" ? <div className="resource-fields"><ScaledNumberField label={t("create.cpu")} range={catalog.data.resourceBounds.cpuMilli} scale={1000} set={(value) => { reviewQuote.reset(); setCPU(value); }} t={t} unit="vCPU" value={cpuMilli} /><ScaledNumberField label={t("create.memory")} range={catalog.data.resourceBounds.memoryMiB} scale={1024} set={(value) => { reviewQuote.reset(); setMemory(value); }} t={t} unit="GB" value={memoryMiB} /><ScaledNumberField label={t("create.disk")} range={catalog.data.resourceBounds.diskGiB} scale={1} set={(value) => { reviewQuote.reset(); setDisk(value); }} t={t} unit="GB" value={diskGiB} />{catalog.data.capacityState === "unavailable" ? <ErrorNotice text={t("create.capacityUnavailable")} /> : null}</div> : null}
      {steps[step].id === "configuration" ? <ConfigurationRenderer locale={locale} manifest={manifest.data} onChange={(next) => { reviewQuote.reset(); setValues(next); }} values={values} /> : null}
      {steps[step].id === "mods" ? <div className="mod-catalog">{manifest.data.modCatalog?.entries.map((mod) => { const direct = mod.modId in mods; const required = requiredMods.has(mod.modId) && !direct; return <label className="mod-row" key={mod.modId}><span><input checked={direct || required} disabled={required} onChange={(event) => setMods((current) => { const next = { ...current }; if (event.target.checked) next[mod.modId] = mod.versions[0].version; else delete next[mod.modId]; return next; })} type="checkbox" /><strong>{mod.displayName}</strong>{required ? <small>{t("create.requiredDependency")}</small> : null}</span><select disabled={!direct} onChange={(event) => setMods((current) => ({ ...current, [mod.modId]: event.target.value }))} value={mods[mod.modId] ?? mod.versions[0].version}>{mod.versions.map((version) => <option key={version.version}>{version.version}</option>)}</select></label>; })}</div> : null}
      {steps[step].id === "review" && reviewQuote.data ? <><div className="review-grid"><Review label={t("create.instance")} value={name.trim()} /><Review label={t("create.game")} value={`${manifest.data.displayName} ${manifest.data.gameVersions[0]}`} /><Review label={t("create.region")} value={localizedRegionName(regions.data?.find((item) => item.id === regionID), locale) || regionID} /><Review label={t("create.specification")} value={`${formatCPU(cpuMilli)} · ${formatMemory(memoryMiB)} · ${t("create.diskUnit", { value: diskGiB })}`} /><Review label={t("create.estimatedCost")} value={`${formatMoney(reviewQuote.data.estimatedHourlyMinor, locale)}/${t("create.hour")} · ${formatMoney(requiredBalance, locale)}/${t("create.hours24")}`} /><Review label={t("create.availableBalance")} value={formatMoney(wallet.data.availableMinor, locale)} /></div><div className="review-note"><span>{t("create.connectionAssigned")}</span></div>{insufficientBalance ? <div className="balance-warning" role="alert"><span>{t("create.insufficientBalance")}</span><Link href={`/w/${workspaceSlug}/billing`}>{t("create.viewBilling")}</Link></div> : null}{quoteExpired ? <div className="balance-warning" role="alert"><span>{t("create.quoteExpired")}</span></div> : null}</> : null}
      {error ? <ErrorNotice text={error} /> : null}
    </div><div className="wizard-footer">{step > 0 ? <Button onClick={() => { setError(""); setStep((value) => value - 1); }} variant="secondary"><ChevronLeft size={16} />{t("create.previous")}</Button> : <span />}{step < steps.length - 1 ? <Button disabled={!canAdvance || reviewQuote.isPending} onClick={advance}>{reviewQuote.isPending ? t("create.quoting") : t("create.next")}<ChevronRight size={16} /></Button> : <Button disabled={deploy.isPending || insufficientBalance || quoteExpired} onClick={() => deploy.mutate()}><Server size={16} />{deploy.isPending ? t("create.submitting") : t("create.deploy")}</Button>}</div>
  </section></>;
}

function inRange(value: number, range: { minimum: number; maximum: number; step: number }) {
  return Number.isFinite(value) && value >= range.minimum && value <= range.maximum && (value - range.minimum) % range.step === 0;
}

function validateConfiguration(manifest: ProviderManifest, values: Values, locale: Locale, t: Translate) {
  for (const key of manifest.configurationSchema.required ?? []) {
    const value = values[key];
    if (value === undefined || value === null || value === "") return t("create.required", { field: localizedField(manifest.configurationSchema.properties[key], locale).title || key });
  }
  for (const [key, field] of Object.entries(manifest.configurationSchema.properties)) {
    const value = values[key];
    const title = localizedField(field, locale).title;
    if ((field.type === "integer" || field.type === "number") && (typeof value !== "number" || !Number.isFinite(value) || field.minimum !== undefined && value < field.minimum || field.maximum !== undefined && value > field.maximum)) return t("create.outOfRange", { field: title });
    if (field.type === "enum" && !field.enum?.includes(value)) return t("create.unsupportedOption", { field: title });
  }
  return "";
}

function formatMoney(minor: number, locale: Locale) { return new Intl.NumberFormat(locale, { style: "currency", currency: "CNY" }).format(minor / 100); }
function localizedRegionName(region: Region | undefined, locale: Locale) { return region?.names?.[locale] ?? region?.name ?? ""; }

export function OperationPage({ workspaceSlug, operationId }: { workspaceSlug: string; operationId: string }) {
  const instanceID = useSearchParams().get("instance");
  const operation = useQuery({ queryKey: ["operation", operationId], queryFn: () => api<Operation>(`/operations/${operationId}`), refetchInterval: (query) => ["succeeded", "failed"].includes(query.state.data?.status ?? "") ? false : 1000 });
  if (!operation.data) return <Loading />;
  return <><PageHeader title="操作详情" actions={operation.data.status === "succeeded" && instanceID ? <Button asChild><Link href={`/w/${workspaceSlug}/instances/${instanceID}`}>查看实例<ChevronRight size={16} /></Link></Button> : undefined} /><section className="operation-panel"><div className="operation-meta"><div><span>操作</span><code>{operation.data.id}</code></div><div><span>类型</span><strong>{operation.data.kind}</strong></div><div><span>状态</span><strong className={operation.data.status === "succeeded" ? "success-text" : ""}>{operation.data.status}</strong></div></div><ol className="operation-steps">{operation.data.steps.map((item) => <li key={item.key}><span className={cn("step-icon", item.status === "succeeded" && "complete", item.status === "running" && "current")}>{item.status === "succeeded" ? <Check size={15} /> : item.status === "running" ? <LoaderCircle className="spin" size={15} /> : null}</span><div><strong>{item.label}</strong><small>{item.detail ?? item.status}</small></div></li>)}</ol></section></>;
}

export function InstanceDetailPage({ workspaceSlug, instanceId }: { workspaceSlug: string; instanceId: string }) {
  const { locale, t } = useI18n(); const router = useRouter(); const search = useSearchParams(); const client = useQueryClient(); const { workspace, isLoading: workspaceLoading, error: workspaceError } = useWorkspace(workspaceSlug);
  const instance = useQuery({ queryKey: ["instance", workspace?.id, instanceId], enabled: Boolean(workspace), queryFn: () => api<Instance>(`/workspaces/${workspace!.id}/instances/${instanceId}`), refetchInterval: 3000 });
  const manifest = useQuery({ queryKey: ["manifest", workspace?.id, instance.data?.providerReleaseId], enabled: Boolean(workspace && instance.data), queryFn: () => api<ProviderManifest>(`/providers/releases/${instance.data!.providerReleaseId}/manifest?workspaceId=${workspace!.id}`) });
  const regions = useQuery({ queryKey: ["regions", workspace?.id], enabled: Boolean(workspace), queryFn: () => api<Region[]>(`/regions?workspaceId=${workspace!.id}`) });
  const act = useMutation({ mutationFn: (action: string) => api<Operation>(`/workspaces/${workspace!.id}/instances/${instanceId}:${action}`, { method: "POST", headers: { "Idempotency-Key": idempotencyKey(action) }, body: "{}" }), onSuccess: () => client.invalidateQueries({ queryKey: ["instance", workspace?.id, instanceId] }) });
  if (workspaceLoading || instance.isLoading || manifest.isLoading) return <Loading />;
  if (workspaceError || instance.error || manifest.error || !workspace || !instance.data || !manifest.data) return <ErrorNotice text={t("instance.unavailable")} />;
  const tab = search.get("tab") ?? "overview"; const capabilities = manifest.data.capabilities;
  const tabs = [{ id: "overview", label: t("instance.overview"), icon: Server }, ...(capabilities.includes("console") ? [{ id: "console", label: t("instance.console"), icon: TerminalSquare }] : []), ...(capabilities.includes("logs") ? [{ id: "logs", label: t("instance.logs"), icon: FileText }] : []), ...(capabilities.includes("backup") ? [{ id: "backups", label: t("instance.backups"), icon: Archive }] : []), ...(capabilities.includes("configuration") ? [{ id: "configuration", label: t("instance.configuration"), icon: Settings }] : [])];
  const endpoint = primaryEndpoint(instance.data);
  const regionName = localizedRegionName(regions.data?.find((item) => item.id === instance.data.regionId), locale) || t("instance.regionUnavailable");
  return <><section className="instance-header"><Link className="breadcrumb" href={`/w/${workspaceSlug}/instances`}><ChevronLeft size={15} />{t("instances.title")}</Link><div className="instance-title-row"><div className="instance-identity"><div className="instance-name-line"><h1>{instance.data.name}</h1><StateBadge state={instance.data.observedState} /></div><EndpointSummary instance={instance.data} /></div><div className="header-actions">{endpoint ? <button aria-label={t("instance.copyEndpoint")} title={t("instance.copyEndpoint")} onClick={() => navigator.clipboard?.writeText(endpoint.displayAddress)} type="button"><Clipboard size={17} /></button> : null}<button aria-label={t("instance.restart")} title={t("instance.restart")} disabled={act.isPending || instance.data.desiredState !== "running"} onClick={() => act.mutate("restart")} type="button"><RefreshCw size={17} /></button>{instance.data.desiredState === "stopped" ? <button aria-label={t("instance.start")} title={t("instance.start")} disabled={act.isPending} onClick={() => act.mutate("start")} type="button"><Play size={17} /></button> : <button aria-label={t("instance.stop")} title={t("instance.stop")} disabled={act.isPending} onClick={() => act.mutate("stop")} type="button"><Square size={16} /></button>}</div></div><nav aria-label={t("instance.navigation")} className="detail-tabs">{tabs.map((item) => { const Icon = item.icon; return <button aria-current={tab === item.id ? "page" : undefined} className={tab === item.id ? "active" : undefined} key={item.id} onClick={() => router.push(`?tab=${item.id}`)} type="button"><Icon size={16} />{item.label}</button>; })}</nav></section>
    {act.error ? <ErrorNotice text={t("instance.actionRejected")} /> : null}
    {tab === "overview" ? <section className="detail-panel overview-panel"><dl className="instance-facts"><div><dt>{t("instance.game")}</dt><dd><strong>{manifest.data.displayName}</strong><code>{instance.data.gameVersion}</code></dd></div><div><dt>{t("instance.region")}</dt><dd><strong>{regionName}</strong></dd></div><div><dt>{t("instance.specification")}</dt><dd><strong>{formatCPU(instance.data.resourceSpec.cpuMilli)} · {formatMemory(instance.data.resourceSpec.memoryMiB)} · {instance.data.resourceSpec.diskGiB} GB</strong></dd></div></dl></section> : null}
    {tab === "console" ? <Console workspaceID={workspace.id} instanceID={instanceId} /> : null}
    {tab === "logs" ? <Logs workspaceID={workspace.id} instanceID={instanceId} /> : null}
    {tab === "backups" ? <Backups workspaceID={workspace.id} instanceID={instanceId} /> : null}
    {tab === "configuration" ? <Configuration workspaceID={workspace.id} instance={instance.data} manifest={manifest.data} /> : null}
  </>;
}

function Console({ workspaceID, instanceID }: { workspaceID: string; instanceID: string }) {
  const [command, setCommand] = useState(""); const [accepted, setAccepted] = useState<string[]>([]);
  const mutation = useMutation({ mutationFn: () => api<Operation>(`/workspaces/${workspaceID}/instances/${instanceID}/console-commands`, { method: "POST", headers: { "Idempotency-Key": idempotencyKey("console") }, body: JSON.stringify({ command }) }), onSuccess: () => { setAccepted((items) => [...items, `> ${command}`, "命令已进入执行队列"]); setCommand(""); } });
  return <section className="terminal-panel"><pre>{accepted.length ? accepted.join("\n") : "控制台已连接。命令将发送到当前实例运行进程。"}</pre><form onSubmit={(event) => { event.preventDefault(); if (command.trim()) mutation.mutate(); }}><span>&gt;</span><input aria-label="控制台命令" onChange={(event) => setCommand(event.target.value)} value={command} /><button disabled={mutation.isPending} type="submit">发送</button></form></section>;
}

function Logs({ workspaceID, instanceID }: { workspaceID: string; instanceID: string }) {
  const logs = useQuery({ queryKey: ["logs", workspaceID, instanceID], queryFn: () => api<LogEntry[]>(`/workspaces/${workspaceID}/instances/${instanceID}/logs?limit=200`), refetchInterval: 3000 });
  return <section className="terminal-panel logs"><pre>{logs.data?.map((entry) => `${new Date(entry.observedAt).toLocaleTimeString()} [${entry.stream}] ${entry.message}`).join("\n") || "暂无日志"}</pre></section>;
}

function Backups({ workspaceID, instanceID }: { workspaceID: string; instanceID: string }) {
  const client = useQueryClient(); const query = useQuery({ queryKey: ["backups", workspaceID], queryFn: () => api<Backup[]>(`/workspaces/${workspaceID}/backups`), refetchInterval: 3000 });
  const create = useMutation({ mutationFn: () => api(`/workspaces/${workspaceID}/backups`, { method: "POST", headers: { "Idempotency-Key": idempotencyKey("backup") }, body: JSON.stringify({ logicalInstanceId: instanceID }) }), onSuccess: () => client.invalidateQueries({ queryKey: ["backups", workspaceID] }) });
  const restore = useMutation({ mutationFn: (id: string) => api(`/workspaces/${workspaceID}/backups/${id}:restore`, { method: "POST", headers: { "Idempotency-Key": idempotencyKey("restore") }, body: "{}" }) });
  const items = query.data?.filter((item) => item.logicalInstanceId === instanceID) ?? [];
  return <section className="detail-panel"><div className="panel-toolbar"><Button disabled={create.isPending} onClick={() => create.mutate()}><Plus size={16} />创建备份</Button></div>{items.map((backup) => <div className="backup-row" key={backup.id}><div><strong>{backup.id}</strong><span>{new Date(backup.createdAt).toLocaleString()} · {formatBytes(backup.sizeBytes ?? 0)} · {backup.status}</span></div><Button disabled={backup.status !== "completed" || restore.isPending} onClick={() => restore.mutate(backup.id)} variant="secondary"><RotateCcw size={15} />恢复</Button></div>)}{items.length === 0 ? <div className="compact-empty">暂无备份</div> : null}</section>;
}

function Configuration({ workspaceID, instance, manifest }: { workspaceID: string; instance: Instance; manifest: ProviderManifest }) {
  const { locale, t } = useI18n(); const [draftID, setDraftID] = useState(""); const [values, setValues] = useState<Values>({}); const [mods, setMods] = useState<Array<{ modId: string; version: string }>>([]); const [baseline, setBaseline] = useState(""); const [message, setMessage] = useState("");
  const revision = useQuery({ queryKey: ["revision", workspaceID, instance.id, instance.configurationRevisionId], queryFn: () => api<Revision>(`/workspaces/${workspaceID}/instances/${instance.id}/revisions/${instance.configurationRevisionId}`) });
  const selectedMods = useMemo(() => Object.fromEntries(mods.map((item) => [item.modId, item.version])), [mods]);
  const requiredMods = useMemo(() => resolveRequiredMods(manifest, selectedMods), [manifest, selectedMods]);
  const fingerprint = configurationFingerprint(values, mods); const dirty = Boolean(baseline && fingerprint !== baseline);
  useEffect(() => { if (revision.data) { const modLock = Array.isArray(revision.data.modLock) ? revision.data.modLock : []; const directMods = modLock.filter((item) => item.direct).map((item) => ({ modId: item.modId, version: item.version })); setValues(revision.data.configuration); setMods(directMods); setBaseline(configurationFingerprint(revision.data.configuration, directMods)); } }, [revision.data]);
  const save = useMutation({ mutationFn: async () => { let id = draftID; if (!id) { const draft = await api<{ id: string }>(`/workspaces/${workspaceID}/instances/${instance.id}/configuration-drafts`, { method: "POST", body: "{}" }); id = draft.id; setDraftID(id); } await api(`/workspaces/${workspaceID}/instances/${instance.id}/configuration-drafts/${id}`, { method: "PUT", body: JSON.stringify({ schemaVersion: manifest.schemaVersion, values, modSelections: mods }) }); return api(`/workspaces/${workspaceID}/instances/${instance.id}/configuration-drafts/${id}:apply`, { method: "POST", headers: { "Idempotency-Key": idempotencyKey("config") }, body: "{}" }); }, onSuccess: () => { setBaseline(fingerprint); setMessage(t("configuration.submitted")); }, onError: () => setMessage(t("configuration.invalid")) });
  if (!revision.data) return <Loading />;
  return <div className="configuration-view"><section className="detail-panel configuration-panel"><ConfigurationRenderer layout="settings" locale={locale} manifest={manifest} onChange={(next) => { setValues(next); setMessage(""); }} values={values} />{manifest.capabilities.includes("mods") && manifest.modCatalog ? <div className="schema-form settings-form"><section><h2>{t("configuration.mods")}</h2>{manifest.modCatalog.entries.map((mod) => { const selected = mods.find((item) => item.modId === mod.modId); const required = requiredMods.has(mod.modId) && !selected; return <label className="mod-row" key={mod.modId}><span><input checked={Boolean(selected) || required} disabled={required} onChange={(event) => { setMessage(""); setMods(event.target.checked ? [...mods, { modId: mod.modId, version: mod.versions[0].version }] : mods.filter((item) => item.modId !== mod.modId)); }} type="checkbox" /><strong>{mod.displayName}</strong>{required ? <small>{t("create.requiredDependency")}</small> : null}</span></label>; })}</section></div> : null}</section><div className="configuration-actions"><span className={cn("configuration-status", dirty && "configuration-dirty")}>{message || (dirty ? t("configuration.unsaved") : t("configuration.saved"))}</span><Button disabled={!dirty || save.isPending} onClick={() => save.mutate()}><Save size={15} />{save.isPending ? t("configuration.saving") : t("configuration.saveApply")}</Button></div></div>;
}

function configurationFingerprint(values: Values, mods: Array<{ modId: string; version: string }>) {
  return JSON.stringify({ values, mods: [...mods].sort((a, b) => a.modId.localeCompare(b.modId)) });
}

function resolveRequiredMods(manifest: ProviderManifest | undefined, selected: Record<string, string>) {
  const required = new Set<string>();
  const queue = Object.entries(selected);
  while (queue.length) {
    const [modId, version] = queue.shift()!;
    const entry = manifest?.modCatalog?.entries.find((item) => item.modId === modId);
    const dependencies = entry?.versions.find((item) => item.version === version)?.dependencies ?? [];
    for (const dependency of dependencies) {
      if (dependency.modId in selected || required.has(dependency.modId)) continue;
      required.add(dependency.modId);
      queue.push([dependency.modId, dependency.version]);
    }
  }
  return required;
}

function latestProviderSummaries(providers: ProviderSummary[]) {
  const latest = new Map<string, ProviderSummary>();
  for (const provider of providers) {
    const key = `${provider.gameKey}:${provider.gameVersions.join(",")}`;
    const current = latest.get(key);
    if (!current || Number(provider.releaseVersion) > Number(current.releaseVersion)) latest.set(key, provider);
  }
  return [...latest.values()];
}

export function BackupsPage({ workspaceSlug }: { workspaceSlug: string }) {
  const { workspace, isLoading: workspaceLoading, error: workspaceError } = useWorkspace(workspaceSlug); const instances = useQuery({ queryKey: ["instances", workspace?.id], enabled: Boolean(workspace), queryFn: () => api<Instance[]>(`/workspaces/${workspace!.id}/instances`) }); const [instanceID, setInstanceID] = useState("");
  useEffect(() => { if (!instanceID && instances.data?.[0]) setInstanceID(instances.data[0].id); }, [instanceID, instances.data]);
  if (workspaceLoading || instances.isLoading) return <><PageHeader title="备份" /><Loading /></>;
  if (workspaceError || instances.error || !workspace) return <><PageHeader title="备份" /><ErrorNotice text="备份列表暂时不可用" /></>;
  if (instances.data?.length === 0) return <><PageHeader title="备份" /><section className="detail-panel compact-empty">创建实例后可在这里管理备份</section></>;
  if (!instanceID) return <><PageHeader title="备份" /><Loading /></>;
  return <><PageHeader title="备份" actions={<select onChange={(event) => setInstanceID(event.target.value)} value={instanceID}>{instances.data?.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>} /><Backups workspaceID={workspace.id} instanceID={instanceID} /></>;
}

export function BillingPage({ workspaceSlug }: { workspaceSlug: string }) {
  const { workspace, isLoading: workspaceLoading, error: workspaceError } = useWorkspace(workspaceSlug); const wallet = useQuery({ queryKey: ["wallet", workspace?.id], enabled: Boolean(workspace), queryFn: () => api<Wallet>(`/workspaces/${workspace!.id}/wallet`) });
  if (workspaceLoading || wallet.isLoading) return <Loading />;
  if (workspaceError || wallet.error || !workspace || !wallet.data) return <ErrorNotice text="账单数据暂时不可用" />;
  return <><PageHeader title="账单" /><section className="billing-strip"><div><span>可用余额</span><strong>¥{(wallet.data.availableMinor / 100).toFixed(2)}</strong></div><div><span>测试额度</span><strong>¥{(wallet.data.promotionalMinor / 100).toFixed(2)}</strong></div><div><span>计费方式</span><strong>按小时 · 资源规格</strong></div></section></>;
}

function ConfigurationRenderer({ manifest, values, onChange, locale, layout = "grid" }: { manifest: ProviderManifest; values: Values; onChange: (values: Values) => void; locale?: Locale; layout?: "grid" | "settings" }) {
  const resolvedLocale = locale ?? "zh-CN";
  const requiredFields = new Set(manifest.configurationSchema.required ?? []);
  return <div className={cn("schema-form", layout === "settings" && "settings-form")}>{[...manifest.uiSchema.sections].sort((a, b) => a.order - b.order).map((section) => <section key={section.id}><h2>{section.localizations?.[resolvedLocale] ?? section.title}</h2><div className={layout === "settings" ? "settings-fields" : "form-grid"}>{Object.entries(manifest.configurationSchema.properties).filter(([key]) => { const ui = manifest.uiSchema.fields[key]; return ui?.section === section.id && (!ui.visibleWhen || values[ui.visibleWhen.field] === ui.visibleWhen.equals); }).sort(([a], [b]) => manifest.uiSchema.fields[a].order - manifest.uiSchema.fields[b].order).map(([key, field]) => <ConfigurationInput field={field} key={key} locale={resolvedLocale} onChange={(value) => onChange({ ...values, [key]: value })} required={requiredFields.has(key)} value={values[key]} />)}</div></section>)}</div>;
}

function ConfigurationInput({ field, value, onChange, locale, required }: { field: ConfigurationField; value: unknown; onChange: (value: unknown) => void; locale: Locale; required: boolean }) {
  const localized = localizedField(field, locale);
  if (field.type === "boolean") return <label className="switch-row"><span><strong>{localized.title}{required ? <span aria-hidden="true" className="required-mark">*</span> : null}</strong>{localized.description ? <small>{localized.description}</small> : null}</span><input aria-required={required} checked={Boolean(value)} onChange={(event) => onChange(event.target.checked)} type="checkbox" /></label>;
  if (field.type === "enum") return <Field label={localized.title} required={required}><select onChange={(event) => onChange(event.target.value)} required={required} value={String(value ?? "")}>{field.enum?.map((option) => <option key={String(option)} value={String(option)}>{localized.enumLabels?.[String(option)] ?? String(option)}</option>)}</select></Field>;
  if (field.type === "integer" || field.type === "number") return <Field label={localized.title} required={required}><input max={field.maximum} min={field.minimum} onChange={(event) => onChange(Number(event.target.value))} required={required} type="number" value={Number(value ?? 0)} /></Field>;
  if (field.type === "string-list") return <Field label={localized.title} required={required}><textarea onChange={(event) => onChange(event.target.value.split("\n"))} required={required} rows={3} value={Array.isArray(value) ? value.join("\n") : ""} /></Field>;
  return <Field label={localized.title} required={required}><input onChange={(event) => onChange(event.target.value)} required={required} type={field.type === "secret" ? "password" : "text"} value={String(value ?? "")} /></Field>;
}

function Field({ label, children, required = false }: { label: string; children: React.ReactNode; required?: boolean }) { return <label className="field"><span>{label}{required ? <span aria-hidden="true" className="required-mark">*</span> : null}</span>{children}</label>; }
function ScaledNumberField({ label, range, value, set, scale, unit, t }: { label: string; range: { minimum: number; maximum: number; step: number }; value: number; set: (value: number) => void; scale: number; unit: string; t: Translate }) {
  return <Field label={label} required><div className="input-suffix"><input max={range.maximum / scale} min={range.minimum / scale} onChange={(event) => set(Number(event.target.value) * scale)} required step={range.step / scale} type="number" value={value / scale} /><span>{unit}</span></div><small>{t("create.range", { minimum: range.minimum / scale, maximum: range.maximum / scale, unit, step: range.step / scale })}</small></Field>;
}
function Review({ label, value }: { label: string; value: string }) { return <div className="review-item"><span>{label}</span><strong>{value}</strong></div>; }
function localizedField(field: ConfigurationField | undefined, locale: Locale) {
  if (!field) return { title: "" };
  const localized = field.localizations?.[locale];
  return { title: localized?.title ?? field.title, description: localized?.description ?? field.description, default: localized?.default ?? field.default, enumLabels: localized?.enumLabels };
}
function defaultValues(manifest: ProviderManifest, locale: Locale) { return Object.fromEntries(Object.entries(manifest.configurationSchema.properties).map(([key, field]) => [key, localizedField(field, locale).default ?? (field.type === "boolean" ? false : field.type === "integer" || field.type === "number" ? 0 : field.type === "string-list" ? [] : "")])); }
function clampStep(value: number, range: { minimum: number; maximum: number; step: number }) { const clamped = Math.max(range.minimum, Math.min(range.maximum, value)); return range.minimum + Math.round((clamped - range.minimum) / range.step) * range.step; }
function formatBytes(value: number) { return value < 1024 * 1024 ? `${Math.round(value / 1024)} KB` : `${(value / 1024 / 1024).toFixed(1)} MB`; }
