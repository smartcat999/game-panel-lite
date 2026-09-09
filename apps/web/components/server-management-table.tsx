"use client";

import Link from "next/link";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Play, Square, RotateCcw, Copy, Check, Trash2, ArrowUpRight } from "lucide-react";
import { gameServerJoinPort, gameServerStatus } from "@/lib/game-server-resource";
import { gameServerAction } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { usePermissions } from "@/lib/permissions";
import { copyText } from "@/lib/clipboard";
import type { GameServerResource } from "@/lib/types";

export function ServerManagementTable({
  servers,
  publicHost
}: {
  servers: GameServerResource[];
  publicHost?: string;
}) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const { canControlServer, canDeleteServer } = usePermissions();
  const queryClient = useQueryClient();
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const actionMutation = useMutation({
    mutationFn: async ({ id, action }: { id: string; action: "start" | "stop" | "restart" | "delete" }) => {
      return gameServerAction(id, action);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["game-servers"] });
    }
  });

  const handleCopyEndpoint = (server: GameServerResource, e: React.MouseEvent) => {
    e.stopPropagation();
    const port = gameServerJoinPort(server) || 7777;
    const host = publicHost || (typeof window !== "undefined" ? window.location.hostname : "127.0.0.1");
    const endpoint = `${host}:${port}`;
    copyText(endpoint);
    setCopiedId(server.id);
    setTimeout(() => setCopiedId(null), 1500);
  };

  if (servers.length === 0) {
    return (
      <div className="p-12 text-center text-xs text-slate-400">
        {isZh ? "暂无服务器实例，点击右上角部署新实例" : "No server instances found. Deploy a new instance to get started."}
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-xs border-collapse">
        <thead>
          <tr className="border-b border-slate-100 bg-slate-50/70 text-[11px] font-semibold text-slate-400 select-none">
            <th className="py-3 pl-4 pr-3 font-medium">{isZh ? "实例名称" : "NAME"}</th>
            <th className="py-3 px-3 font-medium">{isZh ? "运行状态" : "STATUS"}</th>
            <th className="py-3 px-3 font-medium">{isZh ? "游戏内核" : "ENGINE"}</th>
            <th className="py-3 px-3 font-medium">{isZh ? "连接地址" : "ENDPOINT"}</th>
            <th className="py-3 pr-4 pl-3 font-medium text-right">{isZh ? "快捷操作" : "ACTIONS"}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-100 text-xs">
          {servers.map((server) => {
            const status = gameServerStatus(server);
            const isRunning = status === "running";
            const isStopped = status === "stopped";
            const isTmod = server.providerKey?.includes("tmod");
            const port = gameServerJoinPort(server) || 7777;

            return (
              <tr
                key={server.id}
                className="hover:bg-slate-50/80 transition-colors group cursor-pointer"
              >
                {/* Name */}
                <td className="py-3 pl-4 pr-3 font-mono font-bold text-slate-900">
                  <Link href={`/servers/${server.id}`} className="hover:text-emerald-600 transition inline-flex items-center gap-1.5">
                    <span>{server.name}</span>
                    <ArrowUpRight className="size-3 text-slate-300 group-hover:text-emerald-600 transition opacity-0 group-hover:opacity-100" />
                  </Link>
                </td>

                {/* Status */}
                <td className="py-3 px-3">
                  {isRunning ? (
                    <span className="inline-flex items-center gap-1.5 text-[11px] font-medium text-emerald-700 bg-emerald-50 px-2 py-0.5 rounded-md border border-emerald-200/50">
                      <span className="size-1.5 rounded-full bg-emerald-500 animate-pulse" />
                      <span>{isZh ? "运行中" : "Running"}</span>
                    </span>
                  ) : isStopped ? (
                    <span className="inline-flex items-center gap-1.5 text-[11px] font-medium text-slate-500 bg-slate-100 px-2 py-0.5 rounded-md border border-slate-200/60">
                      <span className="size-1.5 rounded-full bg-slate-400" />
                      <span>{isZh ? "已停止" : "Stopped"}</span>
                    </span>
                  ) : (
                    <span className="inline-flex items-center gap-1.5 text-[11px] font-medium text-amber-600 bg-amber-50 px-2 py-0.5 rounded-md border border-amber-200/50">
                      <span className="size-1.5 rounded-full bg-amber-500" />
                      <span>{status}</span>
                    </span>
                  )}
                </td>

                {/* Engine */}
                <td className="py-3 px-3 text-[11px]">
                  {isTmod ? (
                    <span className="font-mono text-purple-700 bg-purple-50 px-2 py-0.5 rounded border border-purple-200/70 font-semibold">
                      tModLoader
                    </span>
                  ) : (
                    <span className="font-mono text-slate-600 bg-slate-100 px-2 py-0.5 rounded border border-slate-200/60">
                      Vanilla 1.4.4.9
                    </span>
                  )}
                </td>

                {/* Endpoint */}
                <td className="py-3 px-3 font-mono text-[11px] text-slate-600">
                  <button
                    onClick={(e) => handleCopyEndpoint(server, e)}
                    className="inline-flex items-center gap-1.5 px-2 py-1 rounded bg-slate-50 border border-slate-200/60 hover:bg-slate-100 hover:text-slate-900 transition cursor-pointer"
                    title={isZh ? "点击复制完整连接地址" : "Click to copy join address"}
                  >
                    <span>:{port}</span>
                    {copiedId === server.id ? (
                      <Check className="size-3 text-emerald-600" />
                    ) : (
                      <Copy className="size-3 text-slate-400" />
                    )}
                  </button>
                </td>

                {/* Actions */}
                <td className="py-3 pr-4 pl-3 text-right" onClick={(e) => e.stopPropagation()}>
                  <div className="flex items-center justify-end gap-1.5">
                    {canControlServer && isStopped && (
                      <button
                        onClick={() => actionMutation.mutate({ id: server.id, action: "start" })}
                        disabled={actionMutation.isPending}
                        title={isZh ? "启动实例" : "Start"}
                        className="flex h-7 items-center gap-1 px-2 rounded-md bg-emerald-50 border border-emerald-200 text-emerald-700 hover:bg-emerald-100 transition cursor-pointer font-medium text-[11px]"
                      >
                        <Play className="size-3 fill-current" />
                        <span>{isZh ? "启动" : "Start"}</span>
                      </button>
                    )}

                    {canControlServer && isRunning && (
                      <>
                        <button
                          onClick={() => actionMutation.mutate({ id: server.id, action: "restart" })}
                          disabled={actionMutation.isPending}
                          title={isZh ? "重启实例" : "Restart"}
                          className="flex h-7 items-center gap-1 px-2 rounded-md bg-slate-50 border border-slate-200 text-slate-600 hover:bg-slate-100 hover:text-slate-900 transition cursor-pointer text-[11px]"
                        >
                          <RotateCcw className="size-3" />
                          <span>{isZh ? "重启" : "Restart"}</span>
                        </button>
                        <button
                          onClick={() => actionMutation.mutate({ id: server.id, action: "stop" })}
                          disabled={actionMutation.isPending}
                          title={isZh ? "停止实例" : "Stop"}
                          className="flex h-7 items-center gap-1 px-2 rounded-md bg-rose-50 border border-rose-200 text-rose-700 hover:bg-rose-100 transition cursor-pointer text-[11px]"
                        >
                          <Square className="size-3 fill-current" />
                          <span>{isZh ? "停止" : "Stop"}</span>
                        </button>
                      </>
                    )}

                    {canDeleteServer && (
                      <button
                        onClick={() => {
                          if (confirm(isZh ? `确定要删除实例 ${server.name} 吗？` : `Are you sure you want to delete ${server.name}?`)) {
                            actionMutation.mutate({ id: server.id, action: "delete" });
                          }
                        }}
                        disabled={actionMutation.isPending}
                        title={isZh ? "删除实例" : "Delete"}
                        className="flex size-7 items-center justify-center rounded-md text-slate-400 hover:text-rose-600 hover:bg-rose-50 transition cursor-pointer ml-1"
                      >
                        <Trash2 className="size-3.5" />
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
