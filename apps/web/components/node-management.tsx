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
  Cpu,
  HardDrive,
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
  getNodeJoinCommand
} from "@/lib/api";
import { usePermissions } from "@/lib/permissions";
import type { ComputeNode, NodeJoinCommand } from "@/lib/types";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button, Input } from "@/components/ui";

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
      {/* Top Banner */}
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center rounded-xl border border-slate-800 bg-slate-950/40 p-4">
        <div>
          <div className="flex items-center gap-2">
            <span className="flex size-7 items-center justify-center rounded-lg border border-panel-green/40 bg-panel-green/10 text-panel-green">
              <Server className="size-4" />
            </span>
            <h2 className="text-sm font-bold text-white">
              {isZh ? "分布式计算节点集群 (Compute Nodes)" : "Compute Nodes Cluster"}
            </h2>
          </div>
          <p className="mt-1 text-xs text-slate-400">
            {isZh
              ? "支持纳管多台远程 VPS / 物理机节点，创建游戏服务器时可自由选择部署位置。"
              : "Manage remote VPS and bare-metal nodes. Deploy game servers across different machines."}
          </p>
        </div>
        {canManageNodes && (
          <Button
            onClick={() => {
              setFormError(null);
              setIsAddModalOpen(true);
            }}
            className="bg-panel-green text-slate-950 font-bold hover:bg-panel-green/90 h-8 text-xs shrink-0"
          >
            <Plus className="mr-1.5 size-3.5" />
            {isZh ? "接入新节点" : "Add Worker Node"}
          </Button>
        )}
      </div>

      {/* Nodes Grid */}
      {isLoading ? (
        <div className="rounded-xl border border-slate-800 bg-slate-950/40 p-8 text-center text-xs text-slate-500">
          {isZh ? "正在加载节点集群..." : "Loading nodes..."}
        </div>
      ) : nodes.length === 0 ? (
        <div className="rounded-xl border border-slate-800 bg-slate-950/40 p-8 text-center text-xs text-slate-500">
          {isZh ? "暂无可用节点" : "No compute nodes available."}
        </div>
      ) : (
        <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
          {nodes.map((node) => {
            const isOnline = node.status === "online";
            const hasHardwareInfo = node.cpuCores > 0 && node.memoryTotalMb > 0;
            const memoryPercent = node.memoryTotalMb > 0 ? Math.min(100, Math.round((node.memoryUsedMb / node.memoryTotalMb) * 100)) : 0;
            return (
              <div
                key={node.id}
                className={cn(
                  "flex flex-col justify-between rounded-xl border bg-slate-950/60 p-4 transition",
                  node.isLocal ? "border-panel-green/40 shadow-sm" : isOnline ? "border-slate-800 hover:border-slate-700" : "border-red-900/30 bg-red-950/10"
                )}
              >
                <div>
                  {/* Card Header */}
                  <div className="flex items-start justify-between gap-2 border-b border-slate-800/80 pb-2.5">
                    <div>
                      <div className="flex items-center gap-1.5">
                        <span className="font-bold text-xs text-white truncate max-w-[150px]">{node.name}</span>
                        {node.isLocal && (
                          <span className="rounded bg-panel-green/15 px-1.5 py-0.2 text-[10px] font-bold text-panel-green">
                            {isZh ? "本机主控" : "Master"}
                          </span>
                        )}
                      </div>
                      <div className="mt-1 flex items-center gap-2 text-[11px] text-slate-400">
                        <span className="flex items-center gap-1 font-mono">
                          <Globe className="size-3 text-slate-500" />
                          {node.region || "Global"}
                        </span>
                        <span className="text-slate-600">·</span>
                        <span className="font-mono text-slate-400 truncate max-w-[110px]">{node.publicIp || node.host}</span>
                      </div>
                    </div>

                    <div className="flex items-center gap-1.5 shrink-0">
                      <span className={cn(
                        "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-bold",
                        isOnline ? "bg-emerald-500/15 text-emerald-400" : "bg-rose-500/15 text-rose-400"
                      )}>
                        <span className={cn("size-1.5 rounded-full", isOnline ? "bg-emerald-400 animate-pulse" : "bg-rose-400")} />
                        {isOnline ? (node.pingLatencyMs ? `${node.pingLatencyMs}ms` : (isZh ? "在线" : "Online")) : (isZh ? "离线" : "Offline")}
                      </span>
                    </div>
                  </div>

                  {/* Hardware & Usage Metrics */}
                  <div className="mt-3 space-y-2 text-xs">
                    {hasHardwareInfo ? (
                      <>
                        {/* CPU metric */}
                        <div>
                          <div className="flex justify-between text-[11px] text-slate-400">
                            <span className="flex items-center gap-1">
                              <Cpu className="size-3 text-panel-green" />
                              <span>{isZh ? "CPU 算力" : "CPU Quota"}</span>
                            </span>
                            <span className="font-mono text-slate-300">
                              {node.cpuCores} {isZh ? "核" : "Cores"} {node.cpuUsagePercent ? `(${node.cpuUsagePercent.toFixed(0)}%)` : ""}
                            </span>
                          </div>
                          <div className="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-slate-900 border border-slate-800">
                            <div
                              className="h-full bg-panel-green transition-all"
                              style={{ width: `${node.cpuUsagePercent ? Math.min(100, Math.max(5, node.cpuUsagePercent)) : 15}%` }}
                            />
                          </div>
                        </div>

                        {/* Memory metric */}
                        <div>
                          <div className="flex justify-between text-[11px] text-slate-400">
                            <span className="flex items-center gap-1">
                              <HardDrive className="size-3 text-sky-400" />
                              <span>{isZh ? "内存分配" : "Memory"}</span>
                            </span>
                            <span className="font-mono text-slate-300">
                              {(node.memoryUsedMb / 1024).toFixed(1)}G / {(node.memoryTotalMb / 1024).toFixed(0)}G ({memoryPercent}%)
                            </span>
                          </div>
                          <div className="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-slate-900 border border-slate-800">
                            <div
                              className={cn("h-full transition-all", memoryPercent > 85 ? "bg-rose-500" : memoryPercent > 65 ? "bg-panel-gold" : "bg-sky-400")}
                              style={{ width: `${Math.max(5, memoryPercent)}%` }}
                            />
                          </div>
                        </div>

                        {/* Workloads & Info */}
                        <div className="flex items-center justify-between pt-1 text-[11px] text-slate-400 font-mono">
                          <span>{isZh ? `运行服务: ${node.runningCount} 个` : `Active: ${node.runningCount}`}</span>
                          {node.dockerVersion && <span>Docker {node.dockerVersion}</span>}
                        </div>
                      </>
                    ) : (
                      <div className="rounded-lg border border-slate-800/80 bg-slate-900/30 p-2.5 text-center text-[11px] text-slate-400 flex items-center justify-center gap-1.5">
                        <Sparkles className="size-3 text-panel-green shrink-0 animate-pulse" />
                        <span>{isZh ? "等待 Agent 首次心跳采集硬件规格..." : "Awaiting agent hardware sync..."}</span>
                      </div>
                    )}
                  </div>
                </div>

                {/* Actions Footer */}
                <div className="mt-3.5 flex items-center justify-between border-t border-slate-800/80 pt-2.5">
                  <div className="flex items-center gap-1">
                    {!node.isLocal && (
                      <button
                        type="button"
                        onClick={() => openJoinModal(node)}
                        title={isZh ? "查看接入指令" : "View Join Command"}
                        className="flex items-center gap-1 rounded px-2 py-1 text-[11px] font-medium text-slate-400 hover:bg-slate-900 hover:text-white transition"
                      >
                        <Terminal className="size-3 text-sky-400" />
                        <span>{isZh ? "指令" : "Command"}</span>
                      </button>
                    )}
                    {canManageNodes && (
                      <button
                        type="button"
                        onClick={() => openEditModal(node)}
                        title={isZh ? "编辑基础信息" : "Edit Node Info"}
                        className="flex items-center gap-1 rounded px-2 py-1 text-[11px] font-medium text-slate-400 hover:bg-slate-900 hover:text-white transition"
                      >
                        <Pencil className="size-3 text-panel-gold" />
                        <span>{isZh ? "编辑" : "Edit"}</span>
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => pingMutation.mutate(node.id)}
                      disabled={pingMutation.isPending}
                      title={isZh ? "测速探活" : "Ping & Health Check"}
                      className="flex items-center gap-1 rounded px-2 py-1 text-[11px] font-medium text-slate-400 hover:bg-slate-900 hover:text-white transition"
                    >
                      <Activity className="size-3 text-panel-green" />
                      <span>{isZh ? "探活" : "Ping"}</span>
                    </button>
                  </div>

                  {!node.isLocal && canManageNodes && (
                    <button
                      type="button"
                      onClick={() => setDeletingNode(node)}
                      className="rounded p-1 text-slate-500 hover:bg-red-950/40 hover:text-red-400 transition"
                      title={isZh ? "删除节点" : "Delete Node"}
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}

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
                  ? "⚠️ 移除后，主控将注销与该节点的连接通道。若该节点上仍有正在运行的游戏服务器，请在移除前先停止或迁移，否则实例可能脱离面板调度。"
                  : "⚠️ Removing this node will terminate the master-agent tunnel. If servers are currently running on it, please stop or migrate them first."}
              </p>
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
