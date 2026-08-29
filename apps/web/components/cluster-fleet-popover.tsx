"use client";

import { useState, useRef, useEffect } from "react";
import { createPortal } from "react-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  ChevronDown,
  Copy,
  Check,
  Network,
  Plus,
  Server,
  Settings,
  Terminal,
  Zap
} from "lucide-react";
import { listComputeNodes, listGameServers, getObservabilityMetrics, getNodeJoinCommand, createComputeNode } from "@/lib/api";
import { gameServerStatus } from "@/lib/game-server-resource";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button, Input } from "@/components/ui";
import { TrafficTopology } from "@/components/traffic-topology";

export function ClusterFleetPopover() {
  const { locale } = useI18n();
  const isZh = locale === "zh";
  const router = useRouter();
  const queryClient = useQueryClient();

  const [isOpen, setIsOpen] = useState(false);
  const [topologyOpen, setTopologyOpen] = useState(false);
  const [joinModalOpen, setJoinModalOpen] = useState(false);
  const [mounted, setMounted] = useState(false);
  const [newNodeName, setNewNodeName] = useState("");
  const [newNodeRegion, setNewNodeRegion] = useState("");
  const [generatedJoinData, setGeneratedJoinData] = useState<{ dockerCommand: string; token: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const popoverRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setMounted(true);
  }, []);

  // Queries
  const nodesQuery = useQuery({
    queryKey: ["compute-nodes"],
    queryFn: listComputeNodes,
    retry: false,
    refetchInterval: 10000,
    staleTime: 5000
  });

  const serversQuery = useQuery({
    queryKey: ["game-servers"],
    queryFn: listGameServers,
    retry: false,
    refetchInterval: 10000,
    staleTime: 5000
  });

  const metricsQuery = useQuery({
    queryKey: ["observability-metrics"],
    queryFn: getObservabilityMetrics,
    retry: false,
    refetchInterval: 10000,
    staleTime: 5000
  });

  const createNodeMutation = useMutation({
    mutationFn: createComputeNode,
    onSuccess: async (createdNode) => {
      await queryClient.invalidateQueries({ queryKey: ["compute-nodes"] });
      const joinData = await getNodeJoinCommand(createdNode.id);
      setGeneratedJoinData({
        dockerCommand: joinData.dockerCommand,
        token: joinData.token
      });
    }
  });

  const nodes = nodesQuery.data ?? [];
  const servers = serversQuery.data ?? [];
  const onlineNodes = nodes.filter((n) => n.status === "online" || n.isLocal);
  const runningServers = servers.filter((s) => gameServerStatus(s) === "running");

  const hostMetrics = metricsQuery.data?.host;
  const cpuPercent = hostMetrics ? Math.round(hostMetrics.totalCpuPercent) : 0;
  const memUsedGB = hostMetrics ? (hostMetrics.totalMemoryMb / 1024).toFixed(1) : "0";

  // Close popover on click outside
  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (popoverRef.current && !popoverRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    }
    if (isOpen) {
      document.addEventListener("mousedown", handleClickOutside);
      return () => document.removeEventListener("mousedown", handleClickOutside);
    }
  }, [isOpen]);

  // 只要没有明确离线的节点，默认均为绿色健康
  const hasOfflineNode = nodes.length > 0 && onlineNodes.length < nodes.length;
  const isAllHealthy = !hasOfflineNode;
  const totalNodeCount = Math.max(nodes.length, 1);

  return (
    <div ref={popoverRef} className="relative inline-flex items-center">
      {/* 顶栏常态节点状态胶囊 */}
      <button
        type="button"
        onClick={() => setIsOpen(!isOpen)}
        aria-expanded={isOpen}
        aria-label={isZh ? "多节点状态总览" : "Compute Nodes Fleet"}
        className={cn(
          "flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-mono transition-all duration-200 focus:outline-none focus:ring-1 focus:ring-panel-green/50 select-none",
          isOpen
            ? "border-emerald-500 bg-slate-900 text-white shadow-md shadow-emerald-950/40 ring-1 ring-emerald-500/50"
            : isAllHealthy
            ? "border-slate-800 bg-slate-950/90 text-slate-300 hover:border-slate-700 hover:bg-slate-900 hover:text-white"
            : "border-amber-800/80 bg-amber-950/60 text-amber-300 hover:bg-amber-900/70"
        )}
      >
        <span className="relative flex size-2">
          <span
            className={cn(
              "absolute inline-flex h-full w-full rounded-full opacity-75",
              isAllHealthy ? "animate-ping bg-emerald-400" : "animate-ping bg-amber-400"
            )}
          />
          <span
            className={cn(
              "relative inline-flex size-2 rounded-full",
              isAllHealthy ? "bg-emerald-400" : "bg-amber-400"
            )}
          />
        </span>

        <span className="font-semibold tracking-tight">
          {hasOfflineNode
            ? isZh
              ? `${onlineNodes.length}/${totalNodeCount} 节点在线`
              : `${onlineNodes.length}/${totalNodeCount} Nodes Online`
            : isZh
            ? `${totalNodeCount} 个节点在线`
            : `${totalNodeCount} Nodes Online`}
        </span>

        <ChevronDown
          className={cn("size-3 text-slate-400 transition-transform duration-200", isOpen && "rotate-180 text-emerald-400")}
        />
      </button>

      {/* 下拉面板 (实心高对比背景，杜绝透明穿透与重叠) */}
      {isOpen && (
        <div className="absolute left-0 top-12 z-[100] w-84 sm:w-96 rounded-xl border border-slate-700 bg-[#0d131f] p-4 text-slate-200 shadow-2xl shadow-black/90 animate-in fade-in zoom-in-95 duration-150">
          {/* Header 标题栏 */}
          <div className="flex items-center justify-between border-b border-slate-800 pb-3">
            <div className="flex items-center gap-2">
              <div className="flex size-7 items-center justify-center rounded-lg bg-emerald-500/15 text-panel-green border border-emerald-500/30">
                <Server className="size-3.5" />
              </div>
              <div>
                <h4 className="text-xs font-bold text-white tracking-wide">
                  {isZh ? "计算节点总览" : "Compute Nodes Overview"}
                </h4>
                <p className="text-[10px] text-slate-400 font-mono">
                  {isZh
                    ? `${onlineNodes.length}/${nodes.length} 个节点在线 · ${runningServers.length}/${servers.length} 个实例运行中`
                    : `${onlineNodes.length}/${nodes.length} online · ${runningServers.length}/${servers.length} running`}
                </p>
              </div>
            </div>

            <Link
              href="/settings"
              onClick={() => setIsOpen(false)}
              className="flex size-7 items-center justify-center rounded-lg border border-slate-800 bg-slate-900 text-slate-400 hover:border-slate-600 hover:text-white transition"
              title={isZh ? "节点管理设置" : "Node Settings"}
            >
              <Settings className="size-3.5" />
            </Link>
          </div>

          {/* 实时系统指标条 */}
          <div className="grid grid-cols-3 gap-2 py-3 border-b border-slate-800 font-mono text-center">
            <div className="rounded-lg bg-slate-950 border border-slate-800/80 p-2">
              <p className="text-[10px] text-slate-400">{isZh ? "CPU 使用率" : "CPU USAGE"}</p>
              <p className="text-sm font-bold text-sky-300 mt-0.5">{cpuPercent}%</p>
            </div>
            <div className="rounded-lg bg-slate-950 border border-slate-800/80 p-2">
              <p className="text-[10px] text-slate-400">{isZh ? "内存占用" : "MEM USED"}</p>
              <p className="text-sm font-bold text-purple-300 mt-0.5">{memUsedGB} GB</p>
            </div>
            <div className="rounded-lg bg-slate-950 border border-slate-800/80 p-2">
              <p className="text-[10px] text-slate-400">{isZh ? "运行中实例" : "RUNNING"}</p>
              <p className="text-sm font-bold text-panel-green mt-0.5">{runningServers.length}</p>
            </div>
          </div>

          {/* 节点列表 */}
          <div className="py-3 space-y-2">
            <div className="flex items-center justify-between text-[11px] font-mono text-slate-400 px-0.5">
              <span>{isZh ? "节点分布" : "Node Distribution"}</span>
              <span className="text-[10px] text-slate-500">{nodes.length} {isZh ? "个节点" : "nodes"}</span>
            </div>

            <div className="space-y-1.5 max-h-48 overflow-y-auto pr-0.5">
              {nodes.map((n) => {
                const nodeServers = servers.filter(
                  (s) => (n.isLocal && (!s.nodeId || s.nodeId === "node-local")) || s.nodeId === n.id
                );
                const nodeRunning = nodeServers.filter((s) => gameServerStatus(s) === "running").length;
                const isOnline = n.status === "online" || n.isLocal;

                return (
                  <div
                    key={n.id}
                    onClick={() => {
                      setIsOpen(false);
                      router.push(`/servers?node=${n.id}`);
                    }}
                    className="flex items-center justify-between rounded-lg border border-slate-800 bg-slate-950 hover:bg-slate-900 hover:border-emerald-500/50 p-2.5 transition cursor-pointer group"
                  >
                    <div className="flex items-center gap-2.5 min-w-0">
                      <div className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-slate-900 border border-slate-800 text-slate-300 group-hover:text-emerald-400">
                        {n.isLocal ? <Server className="size-3.5" /> : <Zap className="size-3.5" />}
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5">
                          <span className={cn("size-1.5 rounded-full shrink-0", isOnline ? "bg-emerald-400" : "bg-slate-500")} />
                          <p className="text-xs font-semibold text-slate-200 truncate group-hover:text-white">
                            {n.name}
                          </p>
                        </div>
                        <p className="text-[10px] text-slate-400 font-mono truncate">
                          {n.region ? `${n.region} · ` : ""}
                          {nodeRunning > 0 ? (
                            <span className="text-emerald-400">{nodeRunning} 个运行中</span>
                          ) : (
                            <span>0 个运行</span>
                          )}
                          {" · "}{nodeServers.length} 个实例
                        </p>
                      </div>
                    </div>

                    <div className="text-right shrink-0 font-mono text-[11px]">
                      {n.pingLatencyMs ? (
                        <span className="text-sky-400 bg-sky-950 border border-sky-800/60 rounded px-1.5 py-0.5 text-[10px]">
                          {n.pingLatencyMs}ms
                        </span>
                      ) : (
                        <span className="text-slate-500 text-[10px]">
                          {n.isLocal ? "主控" : "Agent"}
                        </span>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>

          {/* 底部快捷操作 */}
          <div className="pt-2 border-t border-slate-800">
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => {
                  setIsOpen(false);
                  setTopologyOpen(true);
                }}
                className="flex items-center justify-center gap-1.5 rounded-lg border border-slate-800 bg-slate-950 hover:bg-slate-900 hover:border-slate-700 py-2 text-xs font-medium text-slate-300 hover:text-white transition"
              >
                <Network className="size-3.5 text-panel-green" />
                <span>{isZh ? "网络拓扑" : "Topology"}</span>
              </button>

              <button
                type="button"
                onClick={() => {
                  setIsOpen(false);
                  setNewNodeName(`Worker-Node-${nodes.length}`);
                  setNewNodeRegion("");
                  setGeneratedJoinData(null);
                  setJoinModalOpen(true);
                }}
                className="flex items-center justify-center gap-1.5 rounded-lg border border-emerald-500/40 bg-emerald-950 hover:bg-emerald-900 py-2 text-xs font-medium text-emerald-300 hover:text-white transition shadow-xs"
              >
                <Plus className="size-3.5" />
                <span>{isZh ? "添加节点" : "Add Node"}</span>
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Traffic Topology Modal via createPortal */}
      {mounted && topologyOpen && createPortal(
        <div
          className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 backdrop-blur-md p-4 animate-in fade-in duration-200"
          onClick={(e) => {
            if (e.target === e.currentTarget) setTopologyOpen(false);
          }}
        >
          <div className="relative w-full max-w-5xl rounded-2xl border border-slate-700 bg-[#080d19] p-5 shadow-2xl space-y-4 max-h-[92vh] overflow-y-auto">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2">
                <Network className="size-5 text-panel-green" />
                <h3 className="text-base font-bold text-white tracking-wide">
                  {isZh ? "分布式集群网络与流量拓扑" : "Cluster Network & Traffic Topology"}
                </h3>
              </div>
              <Button
                variant="secondary"
                className="h-8 px-3 text-xs"
                onClick={() => setTopologyOpen(false)}
              >
                {isZh ? "关闭" : "Close"}
              </Button>
            </div>

            <div className="rounded-xl overflow-hidden border border-slate-800/80">
              <TrafficTopology />
            </div>
          </div>
        </div>,
        document.body
      )}

      {/* Node Join Command Modal via createPortal */}
      {mounted && joinModalOpen && createPortal(
        <div
          className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 backdrop-blur-md p-4 animate-in fade-in duration-200"
          onClick={(e) => {
            if (e.target === e.currentTarget) setJoinModalOpen(false);
          }}
        >
          <div className="relative w-full max-w-lg rounded-2xl border border-slate-700 bg-[#0c121e] p-6 shadow-2xl space-y-4 max-h-[92vh] overflow-y-auto">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2">
                <Terminal className="size-5 text-emerald-400" />
                <h3 className="text-base font-bold text-white tracking-wide">
                  {isZh ? "接入新计算节点" : "Connect Compute Node"}
                </h3>
              </div>
              <Button
                variant="secondary"
                className="h-8 px-3 text-xs"
                onClick={() => setJoinModalOpen(false)}
              >
                {isZh ? "关闭" : "Close"}
              </Button>
            </div>

            {!generatedJoinData ? (
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  if (!newNodeName.trim()) return;
                  createNodeMutation.mutate({
                    name: newNodeName.trim(),
                    host: "0.0.0.0",
                    region: newNodeRegion.trim() || undefined
                  });
                }}
                className="space-y-4 py-1"
              >
                <p className="text-xs text-slate-400 leading-relaxed">
                  {isZh
                    ? "为新计算节点设定名称和所在地域，系统将为其分发独立的集群接入安全 Token："
                    : "Configure a name and region for the compute node. A unique cluster token will be generated:"}
                </p>

                <div className="space-y-1.5">
                  <label className="text-xs font-semibold text-slate-300">
                    {isZh ? "节点名称 (必填)" : "Node Name"}
                  </label>
                  <Input
                    required
                    value={newNodeName}
                    onChange={(e) => setNewNodeName(e.target.value)}
                    placeholder={isZh ? "例如：WH-node02 / 广州机房01" : "e.g. Worker-02"}
                    className="h-9 text-xs bg-slate-950 border-slate-700"
                  />
                </div>

                <div className="space-y-1.5">
                  <label className="text-xs font-semibold text-slate-300">
                    {isZh ? "节点所在地域 (可选)" : "Node Region (Optional)"}
                  </label>
                  <Input
                    value={newNodeRegion}
                    onChange={(e) => setNewNodeRegion(e.target.value)}
                    placeholder={isZh ? "例如：华南-广州 / 家中内网" : "e.g. East-US / Home-LAN"}
                    className="h-9 text-xs bg-slate-950 border-slate-700"
                  />
                </div>

                <div className="flex justify-end gap-2 pt-2 border-t border-slate-800">
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-9 px-3 text-xs"
                    onClick={() => setJoinModalOpen(false)}
                  >
                    {isZh ? "取消" : "Cancel"}
                  </Button>
                  <Button
                    type="submit"
                    disabled={createNodeMutation.isPending || !newNodeName.trim()}
                    className="h-9 px-4 bg-panel-green text-slate-950 font-bold hover:bg-emerald-400 text-xs"
                  >
                    {createNodeMutation.isPending
                      ? isZh ? "正在生成安全 Token..." : "Generating Token..."
                      : isZh ? "生成接入命令" : "Generate Join Command"}
                  </Button>
                </div>
              </form>
            ) : (
              <div className="space-y-4 py-1">
                <div className="flex items-center gap-2 rounded-lg bg-emerald-950/60 border border-emerald-500/30 p-2.5 text-xs text-emerald-300">
                  <Check className="size-4 shrink-0 text-emerald-400" />
                  <span>{isZh ? "节点已创建！专属安全 Token 分发成功" : "Node created with unique token!"}</span>
                </div>

                <p className="text-xs text-slate-400 leading-relaxed">
                  {isZh
                    ? "在目标 Linux 服务器终端（需已安装 Docker）粘贴并执行以下命令，Agent 将自动安全接入主控集群："
                    : "Run the following Docker command on your target Linux machine to securely join the cluster fleet:"}
                </p>

                <div className="rounded-xl border border-slate-800 bg-slate-950 p-3.5 font-mono text-xs text-emerald-300 break-all select-all leading-relaxed">
                  {generatedJoinData.dockerCommand}
                </div>

                <div className="flex justify-between items-center gap-2 pt-2 border-t border-slate-800">
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-9 px-3 text-xs"
                    onClick={() => {
                      setGeneratedJoinData(null);
                      setNewNodeName(`Worker-Node-${nodes.length + 1}`);
                    }}
                  >
                    {isZh ? "继续接入新节点" : "Add Another"}
                  </Button>

                  <Button
                    className="h-9 px-4 bg-panel-green text-slate-950 font-bold hover:bg-emerald-400 text-xs flex items-center gap-1.5"
                    onClick={() => {
                      navigator.clipboard.writeText(generatedJoinData.dockerCommand);
                      setCopied(true);
                      setTimeout(() => setCopied(false), 2000);
                    }}
                  >
                    {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
                    <span>{copied ? (isZh ? "已复制到剪贴板！" : "Copied!") : (isZh ? "复制命令" : "Copy Command")}</span>
                  </Button>
                </div>
              </div>
            )}
          </div>
        </div>,
        document.body
      )}
    </div>
  );
}
