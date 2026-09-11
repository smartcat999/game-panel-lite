"use client";

import dynamic from "next/dynamic";
import { useMemo, useState, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import type { EChartsType } from "echarts";
import {
  Activity,
  Maximize2,
  Minimize2,
  Network,
  RefreshCw,
  ZoomIn,
  ZoomOut
} from "lucide-react";
import { listComputeNodes, listGameServers, getSettings } from "@/lib/api";
import { gameServerJoinPort, gameServerStatus } from "@/lib/game-server-resource";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

// Dynamically import ReactECharts with SSR disabled for clean Next.js client hydration
const ReactECharts = dynamic(() => import("echarts-for-react"), {
  ssr: false,
  loading: () => <TopologyLoading />
});

function TopologyLoading() {
  const { locale } = useI18n();
  return (
    <div className="flex h-[420px] w-full items-center justify-center bg-[#050811] text-slate-500">
      <span className="flex items-center gap-2 text-xs animate-pulse">
        <Activity className="size-4 text-emerald-400" />
        {locale.startsWith("zh") ? "正在加载网络拓扑..." : "Loading network topology..."}
      </span>
    </div>
  );
}

type TopologyChartNode = Record<string, unknown> & {
  id: string;
  name: string;
};

type TopologyChartLink = Record<string, unknown> & {
  source: string;
  target: string;
};

type TopologyTooltipParams = {
  dataType?: "node" | "edge";
  data: {
    name?: string;
    source?: string;
    target?: string;
    value?: string;
    rawMeta?: Record<string, string | number | undefined>;
  };
};

type TrafficTopologyProps = {
  showTitle?: boolean;
};

export function TrafficTopology({ showTitle = true }: TrafficTopologyProps) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const echartsRef = useRef<EChartsType | null>(null);
  const [layoutMode, setLayoutMode] = useState<"hierarchical" | "force">("hierarchical");
  const [fullscreen, setFullscreen] = useState(false);

  const nodesQuery = useQuery({ queryKey: ["compute-nodes"], queryFn: listComputeNodes, retry: false, refetchInterval: 8000 });
  const serversQuery = useQuery({ queryKey: ["game-servers"], queryFn: listGameServers, retry: false, refetchInterval: 8000 });
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false, staleTime: 60000 });

  const nodes = nodesQuery.data ?? [];
  const servers = serversQuery.data ?? [];
  const publicHost = settingsQuery.data?.publicHost || "43.161.250.66";

  const totalRunning = servers.filter((s) => gameServerStatus(s) === "running").length;

  const chartOption = useMemo(() => {
    const categories = [
      { name: isZh ? "玩家接入层" : "Ingress", itemStyle: { color: "#6366f1" } },
      { name: isZh ? "流网关核心" : "Gateway", itemStyle: { color: "#0ea5e9" } },
      { name: isZh ? "主控节点" : "Master Node", itemStyle: { color: "#10b981" } },
      { name: isZh ? "Worker 计算节点" : "Worker Node", itemStyle: { color: "#06b6d4" } },
      { name: isZh ? "运行中容器" : "Running Workload", itemStyle: { color: "#22c55e" } },
      { name: isZh ? "停止中容器" : "Stopped Workload", itemStyle: { color: "#64748b" } }
    ];

    const chartNodes: TopologyChartNode[] = [];
    const chartLinks: TopologyChartLink[] = [];

    // 1. Ingress Node (Root Left)
    chartNodes.push({
      id: "node_ingress",
      name: isZh ? "玩家接入" : "Player Ingress",
      category: 0,
      x: 80,
      y: 280,
      fixed: layoutMode === "hierarchical",
      symbolSize: 56,
      itemStyle: {
        color: "#4f46e5",
        borderColor: "#818cf8",
        borderWidth: 2,
        shadowBlur: 16,
        shadowColor: "rgba(99, 102, 241, 0.45)"
      },
      label: {
        show: true,
        position: "bottom",
        color: "#cbd5e1",
        fontSize: 11,
        formatter: "{b}"
      },
      rawMeta: {
        type: "ingress",
        protocol: "TCP/UDP",
        activeClients: totalRunning > 0 ? (isZh ? "在线" : "Active") : (isZh ? "就绪" : "Ready"),
        desc: isZh ? "外部公网直连与游戏客户端流量入口" : "Public internet game client entry"
      }
    });

    // 2. Gateway Node
    chartNodes.push({
      id: "node_gateway",
      name: `GameTraffic Gateway\n${publicHost}`,
      category: 1,
      x: 280,
      y: 280,
      fixed: layoutMode === "hierarchical",
      symbolSize: 64,
      itemStyle: {
        color: "#0284c7",
        borderColor: "#38bdf8",
        borderWidth: 3,
        shadowBlur: 20,
        shadowColor: "rgba(56, 189, 248, 0.55)"
      },
      label: {
        show: true,
        position: "top",
        color: "#38bdf8",
        fontWeight: "bold",
        fontSize: 12
      },
      rawMeta: {
        type: "gateway",
        publicHost,
        portRange: "7777 - 65535",
        status: isZh ? "运行中" : "Active",
        desc: isZh ? "主控边缘流量网关，自动接管全双工 TCP 字节流反向透传" : "Edge gateway proxying full-duplex TCP stream tunnels"
      }
    });

    // Link: Ingress -> Gateway
    chartLinks.push({
      source: "node_ingress",
      target: "node_gateway",
      value: "TCP Streams",
      lineStyle: {
        width: 3,
        color: "#6366f1",
        curveness: 0,
        type: "solid"
      },
      label: {
        show: true,
        formatter: isZh ? "公网流量" : "Public traffic",
        fontSize: 10,
        color: "#a5b4fc"
      }
    });

    // 3. Compute Nodes (Local + Workers)
    const localNode = nodes.find((n) => n.isLocal) || {
      id: "node-local",
      name: "Local Host Daemon",
      region: "Local Host",
      isLocal: true,
      status: "online",
      pingLatencyMs: 0,
      cpuCores: 0
    };
    const workerNodes = nodes.filter((n) => !n.isLocal);
    const allComputeNodes = [localNode, ...workerNodes];

    allComputeNodes.forEach((node, nIdx) => {
      const isLocal = node.isLocal;
      const nodeId = node.id;
      const nodeKey = `compute_${nodeId}`;
      const yOffset = allComputeNodes.length === 1
        ? 280
        : 140 + (nIdx * (300 / Math.max(1, allComputeNodes.length - 1)));

      const isOnline = node.status === "online";

      chartNodes.push({
        id: nodeKey,
        name: `${node.name}\n${node.region || (isLocal ? (isZh ? "本机" : "Local") : (isZh ? "远程" : "Remote"))}`,
        category: isLocal ? 2 : 3,
        x: 520,
        y: yOffset,
        fixed: layoutMode === "hierarchical",
        symbolSize: 48,
        itemStyle: {
          color: isLocal ? "#059669" : "#0891b2",
          borderColor: isLocal ? "#34d399" : "#22d3ee",
          borderWidth: 2,
          shadowBlur: 14,
          shadowColor: isLocal ? "rgba(52, 211, 153, 0.4)" : "rgba(34, 211, 238, 0.4)"
        },
        label: {
          show: true,
          position: "top",
          color: isLocal ? "#6ee7b7" : "#67e8f9",
          fontSize: 11
        },
        rawMeta: {
          type: "compute_node",
          name: node.name,
          region: node.region || (isZh ? "本机" : "Local"),
          isLocal,
          status: isOnline ? (isZh ? "在线" : "Online") : (isZh ? "离线" : "Offline"),
          latency: node.pingLatencyMs ? `${node.pingLatencyMs}ms` : (isLocal ? "0ms" : (isZh ? "未知" : "Unknown")),
          cores: node.cpuCores ? `${node.cpuCores} ${isZh ? "核" : "cores"}` : (isZh ? "自动" : "Auto"),
          desc: isLocal ? (isZh ? "主控宿主机" : "Local control host") : (isZh ? "远程计算节点" : "Remote compute node")
        }
      });

      // Link: Gateway -> Node
      chartLinks.push({
        source: "node_gateway",
        target: nodeKey,
        value: isLocal ? (isZh ? "本地直连" : "Direct") : (isZh ? "反向隧道" : "Reverse Tunnel"),
        lineStyle: {
          width: 2.5,
          color: isLocal ? "#10b981" : "#06b6d4",
          curveness: (nIdx - (allComputeNodes.length - 1) / 2) * 0.12,
          type: isLocal ? "solid" : "dashed"
        },
        label: {
          show: true,
          formatter: isLocal
            ? (isZh ? "本地直连" : "Direct")
            : `${isZh ? "反向隧道" : "Reverse tunnel"} ${node.pingLatencyMs ? node.pingLatencyMs + "ms" : ""}`,
          fontSize: 9,
          color: isLocal ? "#6ee7b7" : "#67e8f9"
        }
      });

      // 4. Containers belonging to this node
      const nodeServers = servers.filter((s) => {
        if (isLocal) {
          return !s.nodeId || s.nodeId === "node-local" || s.nodeId === "";
        }
        return s.nodeId === nodeId;
      });

      nodeServers.forEach((srv, sIdx) => {
        const srvKey = `srv_${srv.id}`;
        const isRunning = gameServerStatus(srv) === "running";
        const joinPort = gameServerJoinPort(srv);
        const containerY = nodeServers.length === 1
          ? yOffset
          : yOffset - 40 + (sIdx * 80);

        chartNodes.push({
          id: srvKey,
          name: `${srv.name}\n:${joinPort}`,
          category: isRunning ? 4 : 5,
          x: 760,
          y: containerY,
          fixed: layoutMode === "hierarchical",
          symbolSize: 38,
          itemStyle: {
            color: isRunning ? "#16a34a" : "#475569",
            borderColor: isRunning ? "#4ade80" : "#94a3b8",
            borderWidth: 2,
            shadowBlur: isRunning ? 14 : 0,
            shadowColor: "rgba(74, 222, 128, 0.5)"
          },
          label: {
            show: true,
            position: "right",
            color: isRunning ? "#86efac" : "#94a3b8",
            fontSize: 10,
            formatter: "{b}"
          },
          rawMeta: {
            type: "workload",
            name: srv.name,
            provider: srv.providerKey,
            port: joinPort,
            status: isRunning ? (isZh ? "运行中" : "Running") : (isZh ? "已停止" : "Stopped"),
            publicAddress: `${publicHost}:${joinPort}`,
            desc: isZh ? `游戏服务器容器（${srv.providerKey}）` : `Game server container (${srv.providerKey})`
          }
        });

        // Link: Node -> Container
        chartLinks.push({
          source: nodeKey,
          target: srvKey,
          lineStyle: {
            width: isRunning ? 2 : 1,
            color: isRunning ? "#22c55e" : "#475569",
            curveness: 0
          },
          label: {
            show: true,
            formatter: `:${joinPort}`,
            fontSize: 9,
            color: "#cbd5e1"
          }
        });
      });
    });

    return {
      backgroundColor: "transparent",
      tooltip: {
        trigger: "item",
        backgroundColor: "rgba(10, 15, 29, 0.95)",
        borderColor: "rgba(51, 65, 85, 0.8)",
        borderWidth: 1,
        padding: [10, 14],
        textStyle: { color: "#f8fafc", fontSize: 12 },
        formatter: (params: TopologyTooltipParams) => {
          if (params.dataType === "node") {
            const m = params.data.rawMeta;
            if (!m) return `<strong>${params.data.name}</strong>`;
            if (m.type === "ingress") {
              return `
                <div style="font-family: sans-serif; min-width: 180px;">
                  <div style="color: #818cf8; font-weight: bold; margin-bottom: 4px;">🌐 ${isZh ? "玩家接入" : "Player Ingress"}</div>
                  <div style="font-size: 11px; color: #94a3b8; margin-bottom: 6px;">${m.desc}</div>
                  <div style="border-top: 1px solid #334155; padding-top: 6px; font-size: 11px; font-family: monospace;">
                    <div>${isZh ? "传输协议" : "Protocol"}: <span style="color: #f1f5f9;">${m.protocol}</span></div>
                    <div>${isZh ? "状态" : "Status"}: <span style="color: #4ade80;">${m.activeClients}</span></div>
                  </div>
                </div>
              `;
            }
            if (m.type === "gateway") {
              return `
                <div style="font-family: sans-serif; min-width: 220px;">
                  <div style="color: #38bdf8; font-weight: bold; margin-bottom: 4px;">⚡ GameTraffic Gateway</div>
                  <div style="font-size: 11px; color: #94a3b8; margin-bottom: 6px;">${m.desc}</div>
                  <div style="border-top: 1px solid #334155; padding-top: 6px; font-size: 11px; font-family: monospace;">
                    <div>${isZh ? "公网出口" : "Public host"}: <span style="color: #38bdf8; font-weight: bold;">${m.publicHost}</span></div>
                    <div>${isZh ? "端口范围" : "Port range"}: <span style="color: #f1f5f9;">${m.portRange}</span></div>
                    <div>${isZh ? "状态" : "Status"}: <span style="color: #4ade80;">${m.status}</span></div>
                  </div>
                </div>
              `;
            }
            if (m.type === "compute_node") {
              return `
                <div style="font-family: sans-serif; min-width: 200px;">
                  <div style="color: #22d3ee; font-weight: bold; margin-bottom: 4px;">🛰️ ${isZh ? "计算节点" : "Compute Node"}: ${m.name}</div>
                  <div style="font-size: 11px; color: #94a3b8; margin-bottom: 6px;">${m.desc}</div>
                  <div style="border-top: 1px solid #334155; padding-top: 6px; font-size: 11px; font-family: monospace;">
                    <div>${isZh ? "区域" : "Region"}: <span style="color: #f1f5f9;">${m.region}</span></div>
                    <div>${isZh ? "延迟" : "Latency"}: <span style="color: #4ade80; font-weight: bold;">${m.latency}</span></div>
                    <div>${isZh ? "状态" : "Status"}: <span style="color: #f1f5f9;">${m.status}</span></div>
                  </div>
                </div>
              `;
            }
            if (m.type === "workload") {
              return `
                <div style="font-family: sans-serif; min-width: 200px;">
                  <div style="color: #4ade80; font-weight: bold; margin-bottom: 4px;">🎮 ${isZh ? "游戏服务器" : "Game Server"}: ${m.name}</div>
                  <div style="font-size: 11px; color: #94a3b8; margin-bottom: 6px;">${m.desc}</div>
                  <div style="border-top: 1px solid #334155; padding-top: 6px; font-size: 11px; font-family: monospace;">
                    <div>${isZh ? "直连地址" : "Address"}: <span style="color: #38bdf8; font-weight: bold;">${m.publicAddress}</span></div>
                    <div>${isZh ? "状态" : "Status"}: <span style="color: #f1f5f9;">${m.status}</span></div>
                  </div>
                </div>
              `;
            }
          }
          if (params.dataType === "edge") {
            return `<div style="font-size: 11px; font-family: monospace;">${params.data.source} ➔ ${params.data.target} (${params.data.value || "Link"})</div>`;
          }
          return "";
        }
      },
      legend: [
        {
          data: categories.map((c) => c.name),
          textStyle: { color: "#94a3b8", fontSize: 11 },
          top: 10,
          left: 10
        }
      ],
      series: [
        {
          type: "graph",
          layout: layoutMode === "force" ? "force" : "none",
          force: {
            repulsion: 380,
            edgeLength: [120, 220],
            gravity: 0.08
          },
          roam: true,
          draggable: true,
          categories,
          data: chartNodes,
          links: chartLinks,
          cursor: "pointer",
          emphasis: {
            focus: "adjacency",
            lineStyle: {
              width: 4
            }
          },
          lineStyle: {
            color: "source",
            opacity: 0.85
          }
        }
      ]
    };
  }, [isZh, layoutMode, nodes, publicHost, servers, totalRunning]);

  const handleResetZoom = () => {
    if (echartsRef.current) {
      echartsRef.current.dispatchAction({ type: "restore" });
    }
  };

  const handleZoomIn = () => {
    if (echartsRef.current) {
      echartsRef.current.dispatchAction({
        type: "graphRoam",
        zoom: 1.2
      });
    }
  };

  const handleZoomOut = () => {
    if (echartsRef.current) {
      echartsRef.current.dispatchAction({
        type: "graphRoam",
        zoom: 0.8
      });
    }
  };

  return (
    <div className={cn("space-y-4", fullscreen && "fixed inset-0 z-50 bg-[#03060d] p-6")}>
      {/* 1. Header Toolbar */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 rounded-xl border border-slate-800 bg-[#090d16] p-4 shadow-sm">
        <div>
          <div className="flex items-center gap-2.5">
            {showTitle ? (
              <>
                <span className="flex size-7 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 shadow-xs">
                  <Network className="size-4" />
                </span>
                <h2 className="text-sm font-bold tracking-tight text-white">
                  {isZh ? "网络拓扑" : "Network Topology"}
                </h2>
              </>
            ) : null}
            <span className="inline-flex items-center gap-1 rounded-full bg-emerald-950/80 border border-emerald-800/60 px-2 py-0.5 text-[10px] font-mono text-emerald-400">
              <span className="size-1.5 rounded-full bg-emerald-400 animate-pulse" />
              {isZh ? `${nodes.length} 个节点 · ${totalRunning} 个运行实例` : `${nodes.length} nodes · ${totalRunning} running instances`}
            </span>
          </div>
          <p className="mt-1 text-xs text-slate-400">
            {isZh
              ? "查看玩家入口、网关、计算节点和游戏实例之间的连接关系。"
              : "View connections between player ingress, the gateway, compute nodes, and game instances."}
          </p>
        </div>

        {/* Controls Toolbar */}
        <div className="flex items-center flex-wrap gap-2 text-xs">
          <div className="flex items-center rounded-lg border border-slate-800 bg-slate-950 p-0.5">
            <button
              onClick={() => setLayoutMode("hierarchical")}
              className={cn(
                "px-2.5 py-1 rounded-md text-[11px] font-medium transition",
                layoutMode === "hierarchical" ? "bg-slate-800 text-sky-300" : "text-slate-400 hover:text-slate-200"
              )}
            >
              {isZh ? "分层视图" : "Hierarchy"}
            </button>
            <button
              onClick={() => setLayoutMode("force")}
              className={cn(
                "px-2.5 py-1 rounded-md text-[11px] font-medium transition",
                layoutMode === "force" ? "bg-slate-800 text-emerald-300" : "text-slate-400 hover:text-slate-200"
              )}
            >
              {isZh ? "自由布局" : "Free Layout"}
            </button>
          </div>

          <div className="flex items-center gap-1 rounded-lg border border-slate-800 bg-slate-950 p-1">
            <button
              onClick={handleZoomIn}
              title={isZh ? "放大" : "Zoom in"}
              className="p-1.5 rounded text-slate-400 hover:text-slate-100 hover:bg-slate-800 transition"
            >
              <ZoomIn className="size-3.5" />
            </button>
            <button
              onClick={handleZoomOut}
              title={isZh ? "缩小" : "Zoom out"}
              className="p-1.5 rounded text-slate-400 hover:text-slate-100 hover:bg-slate-800 transition"
            >
              <ZoomOut className="size-3.5" />
            </button>
            <button
              onClick={handleResetZoom}
              title={isZh ? "重置视图" : "Reset view"}
              className="p-1.5 rounded text-slate-400 hover:text-slate-100 hover:bg-slate-800 transition"
            >
              <RefreshCw className="size-3.5" />
            </button>
            <button
              onClick={() => setFullscreen(!fullscreen)}
              title={isZh ? "切换全屏" : "Toggle fullscreen"}
              className="p-1.5 rounded text-slate-400 hover:text-slate-100 hover:bg-slate-800 transition"
            >
              {fullscreen ? <Minimize2 className="size-3.5" /> : <Maximize2 className="size-3.5" />}
            </button>
          </div>
        </div>
      </div>

      {/* 2. Professional Graph Canvas */}
      <div className="relative overflow-hidden rounded-xl border border-slate-800 bg-[#050811] shadow-2xl">
        {/* Engineering Dot Grid Background */}
        <div
          className="pointer-events-none absolute inset-0 opacity-[0.04]"
          style={{
            backgroundImage: "radial-gradient(circle at 1px 1px, #fff 1px, transparent 0)",
            backgroundSize: "24px 24px"
          }}
        />

        {/* ECharts Live Canvas */}
        <ReactECharts
          onChartReady={(instance: EChartsType) => {
            echartsRef.current = instance;
          }}
          option={chartOption}
          style={{ height: fullscreen ? "calc(100vh - 140px)" : "420px", width: "100%" }}
          opts={{ renderer: "canvas" }}
        />

        {/* Bottom Legend / Quick Tips */}
        <div className="absolute bottom-3 left-3 right-3 flex flex-wrap items-center justify-between gap-2 pointer-events-none rounded-lg border border-slate-800/80 bg-slate-950/80 px-3 py-2 text-[11px] text-slate-400 backdrop-blur-xs">
          <div className="flex items-center gap-4">
            <span className="flex items-center gap-1.5">
              <span className="size-2 rounded-full bg-indigo-400" />
              {isZh ? "玩家接入" : "Player ingress"}
            </span>
            <span className="flex items-center gap-1.5">
              <span className="size-2 rounded-full bg-sky-400" />
              {isZh ? "网关转发" : "Gateway"}
            </span>
            <span className="flex items-center gap-1.5">
              <span className="size-2 rounded-full bg-cyan-400" />
              {isZh ? "反向隧道" : "Reverse tunnel"}
            </span>
            <span className="flex items-center gap-1.5">
              <span className="size-2 rounded-full bg-emerald-400" />
              {isZh ? "运行实例" : "Running instance"}
            </span>
          </div>
          <span className="font-mono text-[10px] text-slate-500">
            {isZh ? "滚轮缩放 · 拖动画布" : "Scroll to zoom · drag to pan"}
          </span>
        </div>
      </div>
    </div>
  );
}
