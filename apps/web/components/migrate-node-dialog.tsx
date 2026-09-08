"use client";

import { useState } from "react";
import { ArrowRightLeft, Ban, Check, Server, Sparkles, X } from "lucide-react";
import { Button } from "@/components/ui";
import { useI18n } from "@/lib/i18n";
import type { ComputeNode, GameServerResource } from "@/lib/types";

export function MigrateNodeDialog({
  open,
  server,
  nodes,
  busy,
  onCancel,
  onMigrate
}: {
  open: boolean;
  server: GameServerResource;
  nodes: ComputeNode[];
  busy?: boolean;
  onCancel: () => void;
  onMigrate: (targetNodeId?: string) => void;
}) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");

  // targetNodeId: "" means auto-reschedule (optimal node chosen by scheduler)
  const [selectedNodeId, setSelectedNodeId] = useState<string>("");

  if (!open) return null;

  const currentNodeId = server.nodeId || "node-local";
  const currentNode = nodes.find((n) => n.id === server.nodeId);
  const currentNodeName = currentNode
    ? `${currentNode.name}${currentNode.region ? ` (${currentNode.region})` : ""}`
    : (server.nodeId || (isZh ? "主控本机 (Local Daemon)" : "Master Host (Local Daemon)"));

  const isCurrentRunning = server.status?.phase === "running" || server.status?.phase === "reconciling";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 p-4 backdrop-blur-sm">
      <div className="relative w-full max-w-lg rounded-2xl border border-slate-800 bg-slate-900/95 p-6 shadow-2xl backdrop-blur-xl">
        <button
          type="button"
          onClick={onCancel}
          disabled={busy}
          className="absolute right-4 top-4 rounded-lg p-1 text-slate-400 hover:bg-slate-800 hover:text-white disabled:opacity-50 transition"
        >
          <X className="size-4" />
        </button>

        <div className="flex items-center gap-3">
          <div className="flex size-10 items-center justify-center rounded-xl border border-panel-green/30 bg-panel-green/10 text-panel-green shadow-sm">
            <ArrowRightLeft className="size-5" />
          </div>
          <div>
            <h2 className="text-base font-bold text-white">
              {isZh ? "跨节点迁移服务器" : "Migrate Server Node"}
            </h2>
            <p className="text-xs text-slate-400">
              {isZh
                ? "将游戏服务实例声明式重新调度至其他计算节点"
                : "Reassign workload assignment to a different compute node"}
            </p>
          </div>
        </div>

        {/* Current Node Warning / Info */}
        <div className="mt-4 rounded-xl border border-slate-800 bg-slate-950/60 p-3.5 space-y-2 text-xs">
          <div className="flex items-center justify-between">
            <span className="text-slate-400">{isZh ? "当前承载节点:" : "Current Node:"}</span>
            <span className="font-semibold text-slate-200">{currentNodeName}</span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-slate-400">{isZh ? "实例状态:" : "Instance Phase:"}</span>
            <span className={isCurrentRunning ? "font-bold text-panel-gold" : "font-semibold text-slate-300"}>
              {server.status?.phase || "stopped"}
            </span>
          </div>
        </div>

        {isCurrentRunning && (
          <div className="mt-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-300">
            {isZh
              ? "⚠️ 注意：服务器当前处于运行中状态。跨节点迁移前需确保服务已暂停，避免出现数据冲突。"
              : "⚠️ Server is running. Please stop the server before migrating to prevent data inconsistency."}
          </div>
        )}

        {/* Node Target Selection */}
        <div className="mt-5 space-y-2.5">
          <label className="text-xs font-bold text-slate-300 block">
            {isZh ? "选择目标计算节点" : "Select Target Node"}
          </label>
          <p className="text-[11px] text-slate-500">
            {isZh
              ? "优先按您指定的目标节点部署；若未选择或保持自动，调度器将基于各节点实时负载与资源情况智能优选。"
              : "Directly specify a target node or let cloud-native scheduler pick the least allocated node."}
          </p>

          <div className="mt-2 space-y-2 max-h-56 overflow-y-auto pr-1">
            {/* Option: Auto Schedule */}
            <button
              type="button"
              onClick={() => setSelectedNodeId("")}
              className={`w-full flex items-center justify-between p-3 rounded-xl border text-left transition ${
                selectedNodeId === ""
                  ? "border-panel-green bg-panel-green/10 text-white shadow-sm"
                  : "border-slate-800 bg-slate-950/40 text-slate-300 hover:border-slate-700 hover:bg-slate-900/50"
              }`}
            >
              <div className="flex items-center gap-3">
                <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-panel-green/20 text-panel-green">
                  <Sparkles className="size-4" />
                </div>
                <div>
                  <div className="text-xs font-bold flex items-center gap-1.5">
                    <span>{isZh ? "⚡ 自动重新调度 (智能优选)" : "⚡ Auto Reschedule (Smart Pick)"}</span>
                    <span className="rounded bg-panel-green/20 px-1.5 py-0.5 text-[10px] text-panel-green font-medium">
                      {isZh ? "推荐" : "Recommended"}
                    </span>
                  </div>
                  <div className="text-[11px] text-slate-500">
                    {isZh ? "由调度器过滤存活与端口冲突，优选资源最充裕的健康节点" : "Scheduler filters capacity & scores least allocated node"}
                  </div>
                </div>
              </div>
              {selectedNodeId === "" && <Check className="size-4 text-panel-green shrink-0 ml-2" />}
            </button>

            {/* Option: Specific Nodes */}
            {nodes.map((node) => {
              const isSelected = selectedNodeId === node.id;
              const isCurrent = node.id === currentNodeId || (node.isLocal && currentNodeId === "node-local");
              const isOnline = node.status === "online";
              const isCordoned = Boolean(node.unschedulable);

              return (
                <button
                  key={node.id}
                  type="button"
                  disabled={isCurrent || !isOnline || isCordoned}
                  onClick={() => setSelectedNodeId(node.id)}
                  className={`w-full flex items-center justify-between p-3 rounded-xl border text-left transition ${
                    isCurrent
                      ? "border-slate-800/40 bg-slate-950/20 opacity-50 cursor-not-allowed text-slate-500"
                      : isCordoned
                        ? "border-amber-950/40 bg-amber-950/10 opacity-60 cursor-not-allowed text-amber-300/80"
                        : !isOnline
                          ? "border-red-950/30 bg-red-950/10 opacity-60 cursor-not-allowed text-red-300/80"
                          : isSelected
                            ? "border-panel-green bg-panel-green/10 text-white shadow-sm"
                            : "border-slate-800 bg-slate-950/40 text-slate-300 hover:border-slate-700 hover:bg-slate-900/50"
                  }`}
                >
                  <div className="flex items-center gap-3 min-w-0">
                    <div className={`flex size-8 shrink-0 items-center justify-center rounded-lg ${isCordoned ? "bg-amber-950/30 text-amber-400" : isOnline ? "bg-slate-800 text-slate-300" : "bg-red-950/30 text-red-400"}`}>
                      {isCordoned ? <Ban className="size-4 text-amber-400" /> : <Server className="size-4" />}
                    </div>
                    <div className="min-w-0">
                      <div className="text-xs font-bold flex items-center gap-1.5 truncate">
                        <span>{node.name}</span>
                        {node.isLocal && (
                          <span className="rounded bg-slate-800 px-1 py-0.2 text-[10px] text-slate-400">
                            {isZh ? "本机主控" : "Master"}
                          </span>
                        )}
                        {isCurrent && (
                          <span className="rounded bg-amber-500/20 px-1 py-0.2 text-[10px] text-panel-gold">
                            {isZh ? "当前节点" : "Current"}
                          </span>
                        )}
                        {isCordoned && (
                          <span className="rounded bg-amber-500/20 px-1 py-0.2 text-[10px] text-amber-400 border border-amber-500/30">
                            {isZh ? "禁止调度" : "Cordoned"}
                          </span>
                        )}
                        {!isOnline && !isCordoned && (
                          <span className="rounded bg-red-500/20 px-1 py-0.2 text-[10px] text-red-400">
                            {isZh ? "离线" : "Offline"}
                          </span>
                        )}
                      </div>
                      <div className="text-[11px] text-slate-500 flex items-center gap-2 truncate">
                        <span>{node.region || "Global"}</span>
                        <span>·</span>
                        <span className="font-mono text-[10px]">{node.id.slice(0, 12)}</span>
                      </div>
                    </div>
                  </div>
                  {isSelected && <Check className="size-4 text-panel-green shrink-0 ml-2" />}
                </button>
              );
            })}
          </div>
        </div>

        {/* Modal Actions */}
        <div className="mt-6 flex items-center justify-end gap-2.5 pt-4 border-t border-slate-800">
          <Button
            variant="secondary"
            onClick={onCancel}
            disabled={busy}
            className="h-9 px-4 text-xs"
          >
            {isZh ? "取消" : "Cancel"}
          </Button>
          <Button
            variant="primary"
            disabled={busy || isCurrentRunning}
            onClick={() => onMigrate(selectedNodeId || undefined)}
            className="h-9 px-4 text-xs font-bold"
          >
            {busy ? (isZh ? "正在迁移并同步..." : "Migrating...") : (isZh ? "确认迁移调度" : "Confirm Migration")}
          </Button>
        </div>
      </div>
    </div>
  );
}
