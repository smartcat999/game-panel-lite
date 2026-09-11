"use client";

import React, { useState, useEffect } from "react";
import { createPortal } from "react-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Server,
  Plus,
  Activity,
  Terminal,
  Trash2,
  Copy,
  Check,
  Globe,
  Radio,
  Sparkles,
  AlertCircle,
  AlertTriangle,
  Pencil,
  Settings2
} from "lucide-react";
import {
  listComputeNodes,
  createComputeNode,
  updateComputeNode,
  deleteComputeNode,
  pingComputeNode,
  getNodeJoinCommand,
  getObservabilityMetrics
} from "@/lib/api";
import { usePermissions } from "@/lib/permissions";
import type { ComputeNode, NodeJoinCommand } from "@/lib/types";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { dashboardNodeMetrics } from "@/lib/dashboard-metrics";
import { Button, Input } from "@/components/ui";

function NodeAction({ danger = false, disabled, icon, label, onClick }: { danger?: boolean; disabled?: boolean; icon: React.ReactNode; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={cn(
        "inline-flex h-7 items-center gap-1 rounded px-2 text-[11px] font-medium transition disabled:opacity-50",
        danger ? "text-slate-500 hover:bg-rose-950/40 hover:text-rose-400" : "text-slate-400 hover:bg-slate-800 hover:text-white"
      )}
    >
      {icon}
      <span>{label}</span>
    </button>
  );
}

export function NodeManagement() {
  const { locale, t } = useI18n();
  const isZh = locale.startsWith("zh");
  const queryClient = useQueryClient();
  const { canManageNodes } = usePermissions();

  const [mounted, setMounted] = useState(false);
  const [isAddModalOpen, setIsAddModalOpen] = useState(false);
  const [editingNode, setEditingNode] = useState<ComputeNode | null>(null);
  const [deletingNode, setDeletingNode] = useState<ComputeNode | null>(null);
  const [joinModalNode, setJoinModalNode] = useState<ComputeNode | null>(null);
  const [joinCommandData, setJoinCommandData] = useState<NodeJoinCommand | null>(null);
  const [isJoinLoading, setIsJoinLoading] = useState(false);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  useEffect(() => {
    setMounted(true);
  }, []);

  // Add Form state
  const [newNodeName, setNewNodeName] = useState("");
  const [newNodeRegion, setNewNodeRegion] = useState("Hong Kong");
  const [newNodeHost, setNewNodeHost] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  // Edit Form state
  const [editName, setEditName] = useState("");
  const [editRegion, setEditRegion] = useState("");
  const [editHost, setEditHost] = useState("");
  const [editError, setEditError] = useState<string | null>(null);

  const { data: nodes = [], isLoading } = useQuery({
    queryKey: ["compute-nodes"],
    queryFn: listComputeNodes,
    refetchInterval: 10000
  });
  const metricsQuery = useQuery({
    queryKey: ["observability-metrics"],
    queryFn: getObservabilityMetrics,
    retry: false,
    refetchInterval: 10000,
    staleTime: 5000
  });

  const createMutation = useMutation({
    mutationFn: createComputeNode,
    onSuccess: async (createdNode: ComputeNode) => {
      await queryClient.invalidateQueries({ queryKey: ["compute-nodes"] });
      setIsAddModalOpen(false);
      setNewNodeName("");
      setNewNodeHost("");
      openJoinModal(createdNode);
    },
    onError: (err: Error) => {
      setFormError(err.message || "Failed to create node");
    }
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: { name?: string; region?: string; publicIp?: string; host?: string } }) =>
      updateComputeNode(id, payload),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["compute-nodes"] });
      setEditingNode(null);
    },
    onError: (err: Error) => {
      setEditError(err.message || "Failed to update node");
    }
  });

  const deleteMutation = useMutation({
    mutationFn: deleteComputeNode,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compute-nodes"] });
      setDeletingNode(null);
    }
  });

  const pingMutation = useMutation({
    mutationFn: pingComputeNode,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compute-nodes"] });
    }
  });

  const openJoinModal = async (node: ComputeNode) => {
    setJoinModalNode(node);
    setIsJoinLoading(true);
    try {
      const data = await getNodeJoinCommand(node.id);
      setJoinCommandData(data);
    } catch {
      // fallback
    } finally {
      setIsJoinLoading(false);
    }
  };

  const openEditModal = (node: ComputeNode) => {
    setEditingNode(node);
    setEditName(node.name);
    setEditRegion(node.region || "");
    setEditHost(node.publicIp || (node.host !== "0.0.0.0" ? node.host : ""));
    setEditError(null);
  };

  const handleCopy = (text: string, key: string) => {
    navigator.clipboard.writeText(text);
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(null), 2000);
  };

  return (
    <div className="space-y-4">
      <div className="overflow-hidden rounded-lg border border-panel-line bg-panel-card">
        <div className="flex flex-col gap-3 border-b border-panel-line px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-2.5">
            <Server className="size-4 text-panel-green" aria-hidden="true" />
            <h2 className="text-sm font-semibold text-white">{isZh ? "计算节点" : "Compute Nodes"}</h2>
            <span className="text-xs text-slate-500">{nodes.length}</span>
          </div>
          {canManageNodes ? (
            <Button
              className="h-8 shrink-0 text-xs"
              onClick={() => {
                setFormError(null);
                setIsAddModalOpen(true);
              }}
            >
              <Plus className="size-3.5" />
              {isZh ? "接入新节点" : "Add Node"}
            </Button>
          ) : null}
        </div>

        {isLoading ? (
          <div className="p-6 text-center text-xs text-slate-500">{isZh ? "正在加载节点..." : "Loading nodes..."}</div>
        ) : nodes.length === 0 ? (
          <div className="p-6 text-center text-xs text-slate-500">{isZh ? "暂无可用节点" : "No compute nodes available."}</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[900px] border-collapse text-left text-xs">
              <thead className="bg-slate-950/35 text-slate-500">
                <tr>
                  <th className="px-4 py-2 font-medium">{isZh ? "节点" : "Node"}</th>
                  <th className="w-24 px-3 py-2 font-medium">{isZh ? "状态" : "Status"}</th>
                  <th className="w-36 px-3 py-2 font-medium">CPU</th>
                  <th className="w-40 px-3 py-2 font-medium">{isZh ? "内存" : "Memory"}</th>
                  <th className="w-24 px-3 py-2 font-medium">{isZh ? "运行实例" : "Running"}</th>
                  <th className="w-24 px-3 py-2 font-medium">{isZh ? "延迟" : "Latency"}</th>
                  <th className="w-60 px-4 py-2 text-right font-medium">{isZh ? "操作" : "Actions"}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-panel-line">
                {nodes.map((node) => {
                  const isOnline = node.status === "online";
                  const liveMetrics = dashboardNodeMetrics(node, metricsQuery.data?.host);
                  const cpuUsage = liveMetrics.cpuUsagePercent !== null ? `${liveMetrics.cpuUsagePercent.toFixed(0)}%` : "—";
                  const memoryUsage = liveMetrics.memoryUsedMb !== null && liveMetrics.memoryTotalMb > 0
                    ? `${(liveMetrics.memoryUsedMb / 1024).toFixed(1)} / ${(liveMetrics.memoryTotalMb / 1024).toFixed(1)} GB`
                    : "—";
                  const latency = node.isLocal ? "0 ms" : node.pingLatencyMs ? `${node.pingLatencyMs} ms` : "—";
                  return (
                    <tr key={node.id} className="hover:bg-slate-950/25">
                      <td className="px-4 py-2.5">
                        <div className="flex items-center gap-2">
                          <span className="font-medium text-slate-100">{node.name}</span>
                          {node.isLocal ? <span className="rounded bg-panel-green/12 px-1.5 py-0.5 text-[10px] text-panel-green">{isZh ? "本机" : "Local"}</span> : null}
                        </div>
                        <div className="mt-0.5 flex items-center gap-1.5 font-mono text-[11px] text-slate-500">
                          <Globe className="size-3" aria-hidden="true" />
                          <span>{node.region || (isZh ? "未设置区域" : "No region")}</span>
                          <span>·</span>
                          <span>{node.publicIp || node.host}</span>
                        </div>
                      </td>
                      <td className="px-3 py-2.5">
                        <span className={cn("inline-flex items-center gap-1.5", isOnline ? "text-panel-green" : "text-rose-400")}>
                          <span className="size-1.5 rounded-full bg-current" />
                          {isOnline ? (isZh ? "在线" : "Online") : (isZh ? "离线" : "Offline")}
                        </span>
                      </td>
                      <td className="px-3 py-2.5 font-mono text-slate-300">{cpuUsage} <span className="text-slate-500">/ {liveMetrics.cpuCores || "—"} {isZh ? "核" : "cores"}</span></td>
                      <td className="px-3 py-2.5 font-mono text-slate-300">{memoryUsage}</td>
                      <td className="px-3 py-2.5 font-mono text-slate-300">{liveMetrics.runningCount ?? "—"}</td>
                      <td className="px-3 py-2.5 font-mono text-slate-300">{latency}</td>
                      <td className="px-4 py-2.5">
                        <div className="flex items-center justify-end gap-1">
                          {!node.isLocal ? <NodeAction icon={<Terminal className="size-3" />} label={isZh ? "接入指令" : "Join Command"} onClick={() => openJoinModal(node)} /> : null}
                          {canManageNodes ? <NodeAction icon={<Pencil className="size-3" />} label={isZh ? "编辑" : "Edit"} onClick={() => openEditModal(node)} /> : null}
                          <NodeAction disabled={pingMutation.isPending} icon={<Activity className="size-3" />} label={isZh ? "探活" : "Ping"} onClick={() => pingMutation.mutate(node.id)} />
                          {!node.isLocal && canManageNodes ? <NodeAction danger icon={<Trash2 className="size-3" />} label={isZh ? "删除" : "Delete"} onClick={() => setDeletingNode(node)} /> : null}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Add Node Modal */}
      {isAddModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 backdrop-blur-sm p-4">
          <div className="w-full max-w-md rounded-2xl border border-slate-800 bg-slate-900 p-5 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2">
                <span className="flex size-7 items-center justify-center rounded-lg border border-slate-800 bg-slate-950 text-panel-green">
                  <Radio className="size-4" />
                </span>
                <h3 className="text-sm font-bold text-white">
                  {isZh ? "接入新计算节点" : "Add Worker Node"}
                </h3>
              </div>
              <button
                type="button"
                onClick={() => setIsAddModalOpen(false)}
                className="text-slate-400 hover:text-white text-xs"
              >
                ✕
              </button>
            </div>

            {formError && (
              <div className="flex items-center gap-2 rounded-lg border border-red-500/30 bg-red-950/20 px-3 py-2 text-xs text-red-400">
                <AlertCircle className="size-3.5 shrink-0" />
                <span>{formError}</span>
              </div>
            )}

            <div className="space-y-3 text-xs">
              <div className="space-y-1.5">
                <label className="block text-[11px] font-medium text-slate-300">{isZh ? "节点名称" : "Node Name"}</label>
                <Input
                  value={newNodeName}
                  onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewNodeName(e.target.value)}
                  placeholder={isZh ? "例如: HK-Tencent-Node01" : "e.g. HK-Worker-01"}
                  className="w-full h-8.5 text-xs bg-slate-950 border-slate-800"
                />
              </div>

              <div className="grid grid-cols-2 gap-2.5">
                <div className="space-y-1.5">
                  <label className="block text-[11px] font-medium text-slate-300">{isZh ? "部署地域" : "Region"}</label>
                  <Input
                    value={newNodeRegion}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewNodeRegion(e.target.value)}
                    placeholder="Hong Kong / Tokyo / Shanghai"
                    className="w-full h-8.5 text-xs bg-slate-950 border-slate-800"
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="block text-[11px] font-medium text-slate-300">{isZh ? "公网 IP / 域名 (选填)" : "Public IP (Optional)"}</label>
                  <Input
                    value={newNodeHost}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewNodeHost(e.target.value)}
                    placeholder="43.161.x.x"
                    className="w-full h-8.5 text-xs bg-slate-950 border-slate-800"
                  />
                </div>
              </div>

              <div className="rounded-lg border border-panel-green/20 bg-panel-green/5 p-2.5 text-[11px] text-slate-400 flex items-start gap-2">
                <Sparkles className="size-3.5 text-panel-green shrink-0 mt-0.5" />
                <span>{isZh ? "无需手动填写硬件参数。在目标机器运行接入命令后，Agent 将自动探测上报真实 CPU 核心数、内存容量及 Docker 版本。" : "No hardware specs required. The agent will auto-detect and sync CPU, RAM, and Docker version."}</span>
              </div>
            </div>

            <div className="flex items-center justify-end gap-2 border-t border-slate-800 pt-3">
              <Button
                type="button"
                variant="ghost"
                onClick={() => setIsAddModalOpen(false)}
                className="h-8 text-xs border-slate-800 text-slate-400"
              >
                {t("cancel")}
              </Button>
              <Button
                type="button"
                disabled={!newNodeName.trim() || createMutation.isPending}
                onClick={() => createMutation.mutate({
                  name: newNodeName.trim(),
                  host: newNodeHost.trim() || "0.0.0.0",
                  publicIp: newNodeHost.trim(),
                  region: newNodeRegion.trim()
                })}
                className="h-8 text-xs bg-panel-green text-slate-950 font-bold hover:bg-panel-green/90"
              >
                {createMutation.isPending ? t("saving") : (isZh ? "生成接入指令" : "Generate Command")}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Edit Node Modal */}
      {editingNode && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 backdrop-blur-sm p-4">
          <div className="w-full max-w-md rounded-2xl border border-slate-800 bg-slate-900 p-5 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2">
                <span className="flex size-7 items-center justify-center rounded-lg border border-slate-800 bg-slate-950 text-panel-gold">
                  <Settings2 className="size-4" />
                </span>
                <h3 className="text-sm font-bold text-white">
                  {isZh ? `编辑节点信息 · ${editingNode.name}` : `Edit Node · ${editingNode.name}`}
                </h3>
              </div>
              <button
                type="button"
                onClick={() => setEditingNode(null)}
                className="text-slate-400 hover:text-white text-xs"
              >
                ✕
              </button>
            </div>

            {editError && (
              <div className="flex items-center gap-2 rounded-lg border border-red-500/30 bg-red-950/20 px-3 py-2 text-xs text-red-400">
                <AlertCircle className="size-3.5 shrink-0" />
                <span>{editError}</span>
              </div>
            )}

            <div className="space-y-3 text-xs">
              <div className="space-y-1.5">
                <label className="block text-[11px] font-medium text-slate-300">{isZh ? "节点名称" : "Node Name"}</label>
                <Input
                  value={editName}
                  onChange={(e: React.ChangeEvent<HTMLInputElement>) => setEditName(e.target.value)}
                  placeholder={isZh ? "节点名称" : "Node Name"}
                  className="w-full h-8.5 text-xs bg-slate-950 border-slate-800"
                />
              </div>

              <div className="grid grid-cols-2 gap-2.5">
                <div className="space-y-1.5">
                  <label className="block text-[11px] font-medium text-slate-300">{isZh ? "部署地域" : "Region"}</label>
                  <Input
                    value={editRegion}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => setEditRegion(e.target.value)}
                    placeholder="Hong Kong / Tokyo / Shanghai"
                    className="w-full h-8.5 text-xs bg-slate-950 border-slate-800"
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="block text-[11px] font-medium text-slate-300">{isZh ? "公网 IP / 域名" : "Public IP / Host"}</label>
                  <Input
                    value={editHost}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => setEditHost(e.target.value)}
                    placeholder="43.161.x.x"
                    className="w-full h-8.5 text-xs bg-slate-950 border-slate-800"
                  />
                </div>
              </div>
            </div>

            <div className="flex items-center justify-end gap-2 border-t border-slate-800 pt-3">
              <Button
                type="button"
                variant="ghost"
                onClick={() => setEditingNode(null)}
                className="h-8 text-xs border-slate-800 text-slate-400"
              >
                {t("cancel")}
              </Button>
              <Button
                type="button"
                disabled={!editName.trim() || updateMutation.isPending}
                onClick={() => updateMutation.mutate({
                  id: editingNode.id,
                  payload: {
                    name: editName.trim(),
                    region: editRegion.trim(),
                    publicIp: editHost.trim(),
                    host: editHost.trim() || undefined
                  }
                })}
                className="h-8 text-xs bg-panel-green text-slate-950 font-bold hover:bg-panel-green/90"
              >
                {updateMutation.isPending ? t("saving") : t("saveButton")}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Join Command Modal */}
      {joinModalNode && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 backdrop-blur-sm p-4">
          <div className="w-full max-w-lg rounded-2xl border border-slate-800 bg-slate-900 p-5 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2">
                <span className="flex size-7 items-center justify-center rounded-lg border border-slate-800 bg-slate-950 text-sky-400">
                  <Terminal className="size-4" />
                </span>
                <div>
                  <h3 className="text-sm font-bold text-white">
                    {isZh ? `节点接入指令 · ${joinModalNode.name}` : `Join Node · ${joinModalNode.name}`}
                  </h3>
                </div>
              </div>
              <button
                type="button"
                onClick={() => setJoinModalNode(null)}
                className="text-slate-400 hover:text-white text-xs"
              >
                ✕
              </button>
            </div>

            {isJoinLoading ? (
              <div className="py-8 text-center text-xs text-slate-500">
                {isZh ? "正在生成指令..." : "Generating command..."}
              </div>
            ) : joinCommandData ? (
              <div className="space-y-3.5 text-xs">
                <div className="rounded-lg border border-slate-800 bg-slate-950 p-3 space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-[11px] font-bold text-slate-300 flex items-center gap-1.5">
                      <Sparkles className="size-3 text-panel-green" />
                      {isZh ? "方式一：Docker 一键接入 (推荐)" : "Option 1: Docker One-Liner"}
                    </span>
                    <button
                      type="button"
                      onClick={() => handleCopy(joinCommandData.dockerCommand, "docker")}
                      className="flex items-center gap-1 rounded bg-slate-900 px-2 py-0.5 text-[10px] font-bold text-panel-green hover:bg-slate-800 transition"
                    >
                      {copiedKey === "docker" ? <Check className="size-3" /> : <Copy className="size-3" />}
                      <span>{copiedKey === "docker" ? (isZh ? "已复制" : "Copied") : (isZh ? "复制" : "Copy")}</span>
                    </button>
                  </div>
                  <pre className="overflow-x-auto rounded bg-slate-900/90 p-2 font-mono text-[11px] text-slate-300 select-all whitespace-pre-wrap break-all">
                    {joinCommandData.dockerCommand}
                  </pre>
                </div>

                <div className="rounded-lg border border-slate-800 bg-slate-950 p-3 space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-[11px] font-bold text-slate-300 flex items-center gap-1.5">
                      <Terminal className="size-3 text-sky-400" />
                      {isZh ? "方式二：Linux Shell 快速安装" : "Option 2: Shell Script"}
                    </span>
                    <button
                      type="button"
                      onClick={() => handleCopy(joinCommandData.shellCommand, "shell")}
                      className="flex items-center gap-1 rounded bg-slate-900 px-2 py-0.5 text-[10px] font-bold text-sky-400 hover:bg-slate-800 transition"
                    >
                      {copiedKey === "shell" ? <Check className="size-3" /> : <Copy className="size-3" />}
                      <span>{copiedKey === "shell" ? (isZh ? "已复制" : "Copied") : (isZh ? "复制" : "Copy")}</span>
                    </button>
                  </div>
                  <pre className="overflow-x-auto rounded bg-slate-900/90 p-2 font-mono text-[11px] text-slate-300 select-all whitespace-pre-wrap break-all">
                    {joinCommandData.shellCommand}
                  </pre>
                </div>

                <div className="rounded-lg border border-panel-green/20 bg-panel-green/5 p-2.5 text-[11px] text-slate-400 leading-relaxed">
                  {isZh
                    ? "💡 提示：在目标 VPS / 服务器终端粘贴执行上方命令后，Agent 将自动向主控上报真实硬件规格并在 5 秒内自动上线。"
                    : "💡 Note: Run the command on your target VPS. The Agent will report hardware specs and go online automatically."}
                </div>
              </div>
            ) : null}

            <div className="flex items-center justify-end border-t border-slate-800 pt-3">
              <Button
                type="button"
                onClick={() => setJoinModalNode(null)}
                className="h-8 text-xs bg-slate-800 text-white hover:bg-slate-700"
              >
                {t("close")}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Delete Node Confirmation Modal via createPortal */}
      {mounted && deletingNode && createPortal(
        <div
          className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 backdrop-blur-md p-4 animate-in fade-in duration-200"
          onClick={(e) => {
            if (e.target === e.currentTarget && !deleteMutation.isPending) {
              setDeletingNode(null);
            }
          }}
        >
          <div className="relative w-full max-w-md rounded-2xl border border-rose-950/80 bg-[#0e1422] p-6 shadow-2xl shadow-black/90 space-y-4">
            {/* Header */}
            <div className="flex items-center gap-3 border-b border-slate-800/80 pb-3.5">
              <div className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-rose-500/15 border border-rose-500/30 text-rose-400">
                <AlertTriangle className="size-5" />
              </div>
              <div>
                <h3 className="text-sm font-bold text-white tracking-wide">
                  {isZh ? "确认移除计算节点" : "Remove Compute Node"}
                </h3>
                <p className="text-[11px] text-slate-400 font-mono">
                  {deletingNode.name}
                  {deletingNode.region ? ` (${deletingNode.region})` : ""}
                </p>
              </div>
            </div>

            {/* Warning Message Box */}
            <div className="rounded-xl border border-rose-900/40 bg-rose-950/20 p-3.5 text-xs text-rose-300 leading-relaxed space-y-2">
              <p className="font-semibold text-rose-200">
                {isZh
                  ? `您确定要从集群中移除节点 "${deletingNode.name}" 吗？`
                  : `Are you sure you want to remove node "${deletingNode.name}"?`}
              </p>
              <p className="text-[11px] text-slate-400">
                {isZh
                  ? "主控只会注销节点和接入凭据，不会卸载目标主机上的 Agent。若该节点仍有游戏服务器，请先停止或迁移。"
                  : "The control plane only removes the node and revokes its credentials. It does not uninstall the agent from the worker host. Stop or migrate any game servers first."}
              </p>
              <div className="rounded-lg border border-slate-800 bg-slate-950/80 p-2.5">
                <div className="mb-1.5 flex items-center justify-between gap-2">
                  <span className="text-[11px] font-medium text-slate-300">
                    {isZh ? "在目标主机执行 Agent 卸载命令" : "Run on the worker host to remove the agent"}
                  </span>
                  <button
                    type="button"
                    onClick={() => handleCopy("docker update --restart=no gamepanel-agent && docker rm -f gamepanel-agent", "remove-agent")}
                    className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium text-slate-400 hover:bg-slate-800 hover:text-white"
                  >
                    {copiedKey === "remove-agent" ? <Check className="size-3" /> : <Copy className="size-3" />}
                    {copiedKey === "remove-agent" ? (isZh ? "已复制" : "Copied") : (isZh ? "复制命令" : "Copy command")}
                  </button>
                </div>
                <code className="block overflow-x-auto whitespace-nowrap font-mono text-[10px] text-slate-400">
                  docker update --restart=no gamepanel-agent &amp;&amp; docker rm -f gamepanel-agent
                </code>
              </div>
            </div>

            {/* Actions */}
            <div className="flex items-center justify-end gap-2.5 border-t border-slate-800/80 pt-3">
              <Button
                type="button"
                variant="secondary"
                disabled={deleteMutation.isPending}
                onClick={() => setDeletingNode(null)}
                className="h-9 px-4 text-xs font-medium"
              >
                {t("cancel")}
              </Button>
              <Button
                type="button"
                disabled={deleteMutation.isPending}
                onClick={() => deleteMutation.mutate(deletingNode.id)}
                className="h-9 px-4 bg-rose-600 hover:bg-rose-500 text-white font-bold text-xs shadow-md shadow-rose-950/50"
              >
                {deleteMutation.isPending
                  ? isZh ? "正在移除..." : "Removing..."
                  : isZh ? "确认移除节点" : "Confirm Remove"}
              </Button>
            </div>
          </div>
        </div>,
        document.body
      )}
    </div>
  );
}
