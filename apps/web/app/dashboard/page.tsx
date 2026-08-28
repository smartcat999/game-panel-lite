"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  CircleAlert,
  Cpu,
  Gamepad2,
  Globe,
  HardDrive,
  History,
  LayoutGrid,
  List,
  Plus,
  Radio,
  Server,
  Zap,
  ArrowRight,
  ChevronRight
} from "lucide-react";
import { useState } from "react";
import { ResourceTrendChart, ServerStatusKpis } from "@/components/dashboard-charts";
import { ServerCardGrid } from "@/components/server-card-grid";
import { ServerResourceTable } from "@/components/server-resource-table";
import { Button } from "@/components/ui";
import { getPlatformMonitoring } from "@/features/monitoring/api";
import { formatActivityEvent } from "@/lib/activity-display";
import { isWorldOrBackupEventType } from "@/lib/feature-flags";
import { gameServerStatus } from "@/lib/game-server-resource";
import { localizeRelativeTime, useI18n } from "@/lib/i18n";
import { getObservabilityMetrics, getSettings, listActivity, listBackups, listComputeNodes, listGameServers, listWorlds } from "@/lib/api";
import { usePermissions } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import type { GameServerResource } from "@/lib/types";

type DashboardRange = "1h" | "6h" | "24h" | "168h";
type DashboardMetric = "nodeCpu" | "nodeMemory" | "nodeNetwork" | "nodeDisk";
type ViewMode = "grid" | "table";

const rangeOptions: { label: string; value: DashboardRange; step: string }[] = [
  { label: "1h", value: "1h", step: "1m" },
  { label: "6h", value: "6h", step: "5m" },
  { label: "24h", value: "24h", step: "15m" },
  { label: "7d", value: "168h", step: "1h" }
];

export default function DashboardPage() {
  const { locale, t } = useI18n();
  const isZh = locale.startsWith("zh");
  const { canCreateServer } = usePermissions();

  const [range] = useState<DashboardRange>("1h");
  const [metricKey, setMetricKey] = useState<DashboardMetric>("nodeCpu");
  const [viewMode, setViewMode] = useState<ViewMode>("grid");
  const [selectedMonitorNodeId, setSelectedMonitorNodeId] = useState<string>("node-local");
  const step = rangeOptions.find((item) => item.value === range)?.step ?? "1m";

  const serversQuery = useQuery({ queryKey: ["game-servers"], queryFn: listGameServers, retry: false, refetchInterval: 10000 });
  const nodesQuery = useQuery({ queryKey: ["compute-nodes"], queryFn: listComputeNodes, retry: false, refetchInterval: 10000 });
  const activityQuery = useQuery({ queryKey: ["activity"], queryFn: listActivity, retry: false, refetchInterval: 30000 });
  const backupsQuery = useQuery({ queryKey: ["backups"], queryFn: listBackups, retry: false, refetchInterval: 30000 });
  const worldsQuery = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, retry: false, refetchInterval: 30000 });
  const metricsQuery = useQuery({ queryKey: ["observability-metrics"], queryFn: getObservabilityMetrics, retry: false, refetchInterval: 10000 });
  const platformQuery = useQuery({
    queryKey: ["monitoring-platform", range, step],
    queryFn: () => getPlatformMonitoring(range, step),
    retry: false,
    refetchInterval: 30000
  });
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false, staleTime: 5 * 60 * 1000 });

  const servers = serversQuery.data ?? [];
  const nodes = nodesQuery.data ?? [];
  const activity = (activityQuery.data ?? []).filter((event) => !isWorldOrBackupEventType(event.type));
  const backups = backupsQuery.data ?? [];
  const worlds = worldsQuery.data ?? [];

  const running = servers.filter((server) => gameServerStatus(server) === "running");
  const unhealthy = servers.filter((server) => gameServerStatus(server) === "errored");
  const stopped = servers.filter((server) => gameServerStatus(server) === "stopped").length;

  const totalAllocatedCores = servers.reduce((acc, s) => acc + (s.spec?.resources?.cpuLimitCores || 2), 0);
  const totalAllocatedRAM = servers.reduce((acc, s) => acc + (s.spec?.resources?.memoryLimitMb || 2048), 0);

  const statusData = [
    {
      color: "#59d46f",
      label: isZh ? "运行中房间" : "Active Servers",
      value: running.length,
      subLabel: isZh ? `${stopped} 台已休眠` : `${stopped} Idle`,
      icon: <Radio className="size-4" />
    },
    {
      color: "#38bdf8",
      label: isZh ? "已分配资源" : "Allocated Pool",
      value: totalAllocatedCores,
      subLabel: isZh ? `${Math.round(totalAllocatedRAM / 1024)} GB 内存` : `${Math.round(totalAllocatedRAM / 1024)} GB RAM`,
      icon: <Cpu className="size-4" />
    },
    {
      color: "#f59e0b",
      label: isZh ? "世界地图存档" : "World Saves",
      value: worlds.length,
      subLabel: isZh ? `${backups.length} 个备份` : `${backups.length} Backups`,
      icon: <Globe className="size-4" />
    },
    {
      color: unhealthy.length > 0 ? "#f87171" : "#10b981",
      label: isZh ? "系统运行状态" : "Cluster Health",
      value: unhealthy.length > 0 ? unhealthy.length : 100,
      subLabel: unhealthy.length > 0 ? (isZh ? "需关注" : "Attention") : (isZh ? "全部正常" : "100% Normal"),
      icon: <CircleAlert className="size-4" />
    }
  ];

  const series = platformQuery.data?.series[metricKey];
  const featuredServers = [...servers].sort(serverPriority).slice(0, 6);

  return (
    <div className="space-y-8 pb-12">
      {/* 1. Commander Hero Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2.5">
            <h1 className="text-xl sm:text-2xl font-black text-white tracking-tight">
              {isZh ? "仪表盘" : "Dashboard"}
            </h1>
            <span className="inline-flex items-center gap-1.5 rounded-full bg-panel-green/10 border border-panel-green/30 px-2.5 py-0.5 text-xs font-semibold text-panel-green">
              <span className="size-1.5 rounded-full bg-panel-green animate-pulse" />
              {running.length > 0
                ? (isZh ? `${running.length} 个服务器运行中` : `${running.length} Servers Active`)
                : (isZh ? "系统就绪" : "System Ready")}
            </span>
          </div>
          <p className="mt-1 text-xs text-slate-400">
            {isZh ? "多游戏服务器状态管理、直连地址与资源监控" : "Real-time game servers, direct connect addresses & resource stats"}
          </p>
        </div>

        <div className="flex items-center gap-2.5 shrink-0">
          <Link href="/games">
            <Button variant="secondary" className="gap-1.5 h-9 text-xs">
              <Gamepad2 className="size-3.5" />
              <span>{isZh ? "游戏库" : "Games"}</span>
            </Button>
          </Link>
          {canCreateServer && (
            <Link href="/servers/new">
              <Button variant="primary" className="gap-1.5 h-9 text-xs font-bold shadow-lg shadow-panel-green/10">
                <Plus className="size-3.5" />
                <span>{isZh ? "创建服务器" : "Create Server"}</span>
              </Button>
            </Link>
          )}
        </div>
      </div>

      {/* 2. Top Modern Micro-Capsule KPIs */}
      <ServerStatusKpis data={statusData} />

      {/* 2.5 集群各节点独立监控透视 */}
      {nodes.length > 0 && (
        <section className="space-y-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Server className="size-4 text-sky-400" />
              <h3 className="text-xs font-bold uppercase tracking-wider text-slate-300">
                {isZh ? "集群各节点独立监控透视" : "Cluster Compute Nodes"}
              </h3>
            </div>
            <Link href="/settings" className="text-[11px] text-slate-400 hover:text-panel-green flex items-center gap-1 transition">
              <span>{isZh ? "节点集群管理" : "Manage Nodes"}</span>
              <ChevronRight className="size-3" />
            </Link>
          </div>

          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {nodes.map((node) => {
              const nodeServers = servers.filter((s) => (node.isLocal && (!s.nodeId || s.nodeId === "node-local")) || s.nodeId === node.id);
              const runningServers = nodeServers.filter((s) => gameServerStatus(s) === "running");
              const isOnline = node.status === "online";
              const memoryTotalGB = node.memoryTotalMb ? (node.memoryTotalMb / 1024).toFixed(1) : "—";
              const memoryUsedGB = node.memoryUsedMb ? (node.memoryUsedMb / 1024).toFixed(1) : "—";
              const memoryPercent = (node.memoryTotalMb && node.memoryUsedMb)
                ? Math.min(100, Math.round((node.memoryUsedMb / node.memoryTotalMb) * 100))
                : 0;

              return (
                <div
                  key={node.id}
                  className="rounded-xl border border-slate-800 bg-slate-950/40 p-3.5 space-y-3 hover:border-slate-700 transition"
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2 min-w-0">
                      <span className={cn("size-2 rounded-full shrink-0", isOnline ? "bg-emerald-400 animate-pulse" : "bg-slate-500")} />
                      <span className="font-semibold text-sm text-slate-100 truncate">{node.name}</span>
                    </div>
                    {node.region ? (
                      <span className="rounded bg-slate-900 border border-slate-800 px-1.5 py-0.2 text-[10px] text-slate-400 font-mono">
                        {node.region}
                      </span>
                    ) : null}
                  </div>

                  <div className="space-y-2 text-xs">
                    {/* CPU 水位 */}
                    <div>
                      <div className="flex justify-between text-[11px] text-slate-400 mb-1">
                        <span>CPU ({node.cpuCores || "—"} 核心)</span>
                        <span className="font-mono text-slate-200">{(node.cpuUsagePercent ?? 0).toFixed(1)}%</span>
                      </div>
                      <div className="h-1.5 w-full bg-slate-900 rounded-full overflow-hidden">
                        <div
                          className={cn("h-full rounded-full transition-all", (node.cpuUsagePercent ?? 0) > 85 ? "bg-red-500" : (node.cpuUsagePercent ?? 0) > 60 ? "bg-amber-400" : "bg-panel-green")}
                          style={{ width: `${Math.min(100, Math.max(2, node.cpuUsagePercent ?? 0))}%` }}
                        />
                      </div>
                    </div>

                    {/* 内存 水位 */}
                    <div>
                      <div className="flex justify-between text-[11px] text-slate-400 mb-1">
                        <span>内存 ({memoryUsedGB} / {memoryTotalGB} GB)</span>
                        <span className="font-mono text-slate-200">{memoryPercent}%</span>
                      </div>
                      <div className="h-1.5 w-full bg-slate-900 rounded-full overflow-hidden">
                        <div
                          className={cn("h-full rounded-full transition-all", memoryPercent > 85 ? "bg-red-500" : memoryPercent > 60 ? "bg-amber-400" : "bg-sky-400")}
                          style={{ width: `${Math.min(100, Math.max(2, memoryPercent))}%` }}
                        />
                      </div>
                    </div>
                  </div>

                  <div className="pt-2 border-t border-slate-800/80 flex items-center justify-between text-xs">
                    <span className="text-slate-500 text-[11px]">
                      {runningServers.length} / {nodeServers.length} 运行中
                    </span>
                    <Link
                      href={`/servers?node=${node.id}`}
                      className="text-[11px] text-panel-green hover:underline flex items-center gap-0.5 font-medium"
                    >
                      {isZh ? "查看游戏服务器" : "View Servers"} <ArrowRight className="size-2.5" />
                    </Link>
                  </div>
                </div>
              );
            })}
          </div>
        </section>
      )}

      {/* 3. CORE SECTION: Active Game Servers Fleet */}
      <section className="space-y-4">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2.5">
            <div className="flex size-7 items-center justify-center rounded-lg bg-panel-green/10 border border-panel-green/20 text-panel-green">
              <HardDrive className="size-4" />
            </div>
            <div>
              <h2 className="text-base font-bold text-white tracking-tight">
                {isZh ? "游戏服务器列表" : "Game Servers"}
              </h2>
            </div>
            <span className="rounded-full bg-slate-800 px-2 py-0.5 text-xs font-mono text-slate-300">
              {servers.length}
            </span>
          </div>

          <div className="flex items-center gap-2">
            {/* View Mode Toggle: Grid / Table */}
            <div className="inline-flex rounded-lg border border-slate-800 bg-slate-950 p-0.5">
              <button
                type="button"
                onClick={() => setViewMode("grid")}
                className={cn(
                  "p-1.5 rounded-md transition",
                  viewMode === "grid" ? "bg-slate-800 text-white" : "text-slate-500 hover:text-slate-300"
                )}
                title="Grid View"
              >
                <LayoutGrid className="size-3.5" />
              </button>
              <button
                type="button"
                onClick={() => setViewMode("table")}
                className={cn(
                  "p-1.5 rounded-md transition",
                  viewMode === "table" ? "bg-slate-800 text-white" : "text-slate-500 hover:text-slate-300"
                )}
                title="Table View"
              >
                <List className="size-3.5" />
              </button>
            </div>

            <Link
              href="/servers"
              className="inline-flex items-center gap-1 text-xs font-semibold text-panel-green hover:underline ml-2"
            >
              <span>{isZh ? "查看全部房间" : "View All"}</span>
              <ChevronRight className="size-3.5" />
            </Link>
          </div>
        </div>

        {featuredServers.length > 0 ? (
          viewMode === "grid" ? (
            <ServerCardGrid
              limit={6}
              metrics={metricsQuery.data?.servers}
              publicHost={settingsQuery.data?.publicHost}
              servers={featuredServers}
            />
          ) : (
            <ServerResourceTable
              flat
              limit={6}
              metrics={metricsQuery.data?.servers}
              publicHost={settingsQuery.data?.publicHost}
              servers={featuredServers}
              showVersion={false}
            />
          )
        ) : (
          <div className="rounded-2xl border border-dashed border-slate-800 bg-slate-950/40 p-10 text-center space-y-3">
            <div className="mx-auto size-12 rounded-full bg-slate-900 flex items-center justify-center text-slate-500">
              <Server className="size-6" />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-white">{isZh ? "还没有创建任何游戏房间" : "No Servers Created Yet"}</h3>
              <p className="text-xs text-slate-500 mt-1">
                {isZh ? "10秒快速开黑起号，支持帕鲁、我的世界、泰拉瑞亚与饥荒。" : "Deploy your first game server in 10 seconds."}
              </p>
            </div>
            <Link href="/servers/new" className="inline-block pt-2">
              <Button variant="primary" className="h-9 px-4 text-xs font-bold">
                <Plus className="size-3.5 mr-1.5" />
                {isZh ? "立即创建第一个房间" : "Create First Server"}
              </Button>
            </Link>
          </div>
        )}
      </section>

      {/* 4. Quick Launch Fleet Dock (10-Second Presets) */}
      <section className="space-y-3 pt-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Zap className="size-4 text-panel-gold" />
            <h3 className="text-xs font-bold uppercase tracking-wider text-slate-400">
              {isZh ? "快速建服模版" : "Quick Launch Presets"}
            </h3>
          </div>
          <Link href="/presets" className="inline-flex items-center gap-1 text-[11px] text-slate-400 hover:text-panel-green transition">
            <span>{isZh ? "管理所有预设模版" : "All Presets"}</span>
            <ChevronRight className="size-3" />
          </Link>
        </div>

        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <QuickDockCard
            gameKey="palworld"
            title={isZh ? "🐾 幻兽帕鲁 休闲配置" : "Palworld Casual"}
            desc={isZh ? "3x经验 · 死亡不掉落 · 孵蛋秒出" : "3x EXP · No Drop · 0h Egg"}
            badge="Palworld"
            href="/servers/new?preset=palworld-casual"
          />
          <QuickDockCard
            gameKey="minecraft"
            title={isZh ? "⛏️ 我的世界 1.20 生存" : "MC 1.20 Vanilla"}
            desc={isZh ? "经典生存 · 正版直连 · 自动备份" : "Classic Survival · Auto Backup"}
            badge="Minecraft"
            href="/servers/new?preset=minecraft-vanilla"
          />
          <QuickDockCard
            gameKey="terraria"
            title={isZh ? "🌳 泰拉瑞亚 大师开荒" : "Terraria Expert"}
            desc={isZh ? "大师模式 · 自动存档 · 极速联机" : "Master Mode · Auto Save"}
            badge="Terraria"
            href="/servers/new?preset=terraria-master"
          />
          <QuickDockCard
            gameKey="dst"
            title={isZh ? "🍖 饥荒联机版 双洞穴" : "DST Dual Caves"}
            desc={isZh ? "地上地下双层 · 无人自动暂停" : "Dual Caves · Auto Pause"}
            badge="DST"
            href="/servers/new?preset=dst-caves"
          />
        </div>
      </section>

      {/* 5. Bottom Two Wings: Compact Telemetry + Live Audit Events */}
      <div className="grid gap-6 lg:grid-cols-2 pt-2">
        {/* Left Wing: Telemetry Performance Trend */}
        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-5 space-y-4">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
            <div className="flex items-center gap-2">
              <Activity className="size-4 text-panel-green" />
              <h3 className="text-sm font-bold text-white">
                {isZh ? "节点资源实时监控" : "Node Resource Monitor"}
              </h3>
              <span className="rounded bg-panel-green/10 px-1.5 py-0.5 text-[10px] font-mono text-panel-green">
                Live
              </span>
            </div>

            <div className="flex items-center gap-2">
              {/* Node Selector */}
              {nodes.length > 1 && (
                <select
                  value={selectedMonitorNodeId}
                  onChange={(e) => setSelectedMonitorNodeId(e.target.value)}
                  className="h-7 rounded-md border border-slate-800 bg-slate-900 px-2 text-xs text-slate-200 focus:border-panel-green focus:outline-none"
                >
                  <option value="node-local">🖥️ 主控本机 (Local)</option>
                  {nodes.filter(n => !n.isLocal).map(n => (
                    <option key={n.id} value={n.id}>
                      🟢 {n.name} ({n.region || "Worker"})
                    </option>
                  ))}
                </select>
              )}

              <div className="inline-flex rounded-lg border border-slate-800 bg-slate-900 p-0.5 text-xs">
                <button
                  type="button"
                  onClick={() => setMetricKey("nodeCpu")}
                  className={cn("px-2 py-0.5 rounded", metricKey === "nodeCpu" ? "bg-slate-800 text-white font-bold" : "text-slate-400")}
                >
                  CPU
                </button>
                <button
                  type="button"
                  onClick={() => setMetricKey("nodeMemory")}
                  className={cn("px-2 py-0.5 rounded", metricKey === "nodeMemory" ? "bg-slate-800 text-white font-bold" : "text-slate-400")}
                >
                  {isZh ? "内存" : "RAM"}
                </button>
                <button
                  type="button"
                  onClick={() => setMetricKey("nodeNetwork")}
                  className={cn("px-2 py-0.5 rounded", metricKey === "nodeNetwork" ? "bg-slate-800 text-white font-bold" : "text-slate-400")}
                >
                  {isZh ? "网络" : "Net"}
                </button>
              </div>
            </div>
          </div>

          {/* Real-time Hardware Summary for Selected Node */}
          {(() => {
            const activeNode = nodes.find(n => n.id === selectedMonitorNodeId) ?? nodes[0];
            if (!activeNode) return null;
            const memoryTotalGB = activeNode.memoryTotalMb ? (activeNode.memoryTotalMb / 1024).toFixed(1) : "—";
            const memoryUsedGB = activeNode.memoryUsedMb ? (activeNode.memoryUsedMb / 1024).toFixed(1) : "—";
            const cpuPercent = (activeNode.cpuUsagePercent ?? 0).toFixed(1);
            return (
              <div className="grid grid-cols-3 gap-2 rounded-lg border border-slate-800/80 bg-slate-900/60 p-2.5 text-xs font-mono">
                <div>
                  <span className="text-[10px] text-slate-500 block">当前节点</span>
                  <span className="text-slate-200 font-medium truncate block">{activeNode.name}</span>
                </div>
                <div>
                  <span className="text-[10px] text-slate-500 block">CPU 水位 ({activeNode.cpuCores || "—"} 核)</span>
                  <span className="text-panel-green font-semibold">{cpuPercent}%</span>
                </div>
                <div>
                  <span className="text-[10px] text-slate-500 block">内存已用</span>
                  <span className="text-sky-300 font-semibold">{memoryUsedGB} / {memoryTotalGB} GB</span>
                </div>
              </div>
            );
          })()}

          <ResourceTrendChart
            emptyLabel={platformQuery.isLoading ? t("loading") : t("monitoringNoSamples")}
            series={series}
            height={150}
          />
        </div>

        {/* Right Wing: Live Audit & Snapshots Pulse */}
        <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-5 space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <History className="size-4 text-sky-400" />
              <h3 className="text-sm font-bold text-white">
                {isZh ? "近期备份与操作日志" : "Recent Backups & Events"}
              </h3>
            </div>
            <Link href="/activity" className="inline-flex items-center gap-1 text-xs text-panel-green hover:underline">
              <span>{isZh ? "查看全部活动" : "All Events"}</span>
              <ChevronRight className="size-3" />
            </Link>
          </div>

          <div className="space-y-2.5">
            {activity.slice(0, 4).map((event) => {
              const display = formatActivityEvent(event, isZh ? "zh" : "en");
              return (
                <div
                  key={event.id}
                  className="flex items-center justify-between gap-3 rounded-lg border border-slate-900 bg-slate-900/60 px-3 py-2 text-xs"
                >
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="size-1.5 rounded-full bg-slate-500 shrink-0" />
                    <span className="font-semibold text-slate-200 truncate">{display.message || display.typeLabel}</span>
                  </div>
                  <span className="text-[10px] font-mono text-slate-500 shrink-0">
                    {localizeRelativeTime(event.created, locale)}
                  </span>
                </div>
              );
            })}

            {activity.length === 0 && (
              <p className="text-center py-6 text-xs text-slate-500">{isZh ? "暂无近期操作日志" : "No recent activity"}</p>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function QuickDockCard({
  title,
  desc,
  badge,
  href
}: {
  gameKey: string;
  title: string;
  desc: string;
  badge: string;
  href: string;
}) {
  return (
    <Link
      href={href}
      className="group relative flex flex-col justify-between rounded-xl border border-slate-800/90 bg-gradient-to-b from-slate-900/70 to-slate-950/80 p-3.5 transition-all duration-200 hover:border-panel-green/50 hover:bg-slate-900 hover:shadow-lg hover:shadow-panel-green/5"
    >
      <div>
        <div className="flex items-center justify-between gap-2">
          <span className="rounded bg-slate-800 px-1.5 py-0.5 text-[9px] font-mono text-slate-300">
            {badge}
          </span>
          <ArrowRight className="size-3 text-slate-600 transition group-hover:translate-x-0.5 group-hover:text-panel-green" />
        </div>
        <h4 className="mt-2 text-xs font-bold text-white group-hover:text-panel-green transition truncate">
          {title}
        </h4>
        <p className="mt-1 text-[11px] text-slate-400 line-clamp-1">
          {desc}
        </p>
      </div>
    </Link>
  );
}

function serverPriority(a: GameServerResource, b: GameServerResource) {
  const stateWeight = (server: GameServerResource) => {
    switch (gameServerStatus(server)) {
      case "running":
        return 0;
      case "errored":
        return 1;
      case "stopped":
        return 2;
      default:
        return 3;
    }
  };
  return stateWeight(a) - stateWeight(b);
}
