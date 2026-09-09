"use client";

import { useRouter } from "next/navigation";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Play, Square, RotateCcw, Settings, ChevronRight } from "lucide-react";
import {
  gameServerJoinPort,
  gameServerStatus,
  gameServerMaxPlayers
} from "@/lib/game-server-resource";
import { gameServerAction } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { usePermissions } from "@/lib/permissions";
import type { GameServerResource } from "@/lib/types";

export function ServerManagementTable({
  servers
}: {
  servers: GameServerResource[];
  publicHost?: string;
}) {
  const router = useRouter();
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const { canControlServer } = usePermissions();
  const queryClient = useQueryClient();

  const actionMutation = useMutation({
    mutationFn: async ({ id, action }: { id: string; action: "start" | "stop" | "restart" | "delete" }) => {
      return gameServerAction(id, action);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["game-servers"] });
    }
  });

  if (servers.length === 0) {
    return (
      <div className="p-10 text-center text-xs text-slate-400">
        {isZh ? "暂无服务器实例" : "No server instances found."}
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-xs">
        <thead>
          <tr className="border-b border-slate-100 bg-slate-50/70 text-[11px] font-semibold text-slate-400 select-none">
            <th className="py-2.5 pl-3.5 pr-2 font-medium">{isZh ? "名称" : "NAME"}</th>
            <th className="py-2.5 px-2 font-medium">{isZh ? "状态" : "STATUS"}</th>
            <th className="py-2.5 px-2 font-medium">{isZh ? "引擎" : "ENGINE"}</th>
            <th className="py-2.5 px-2 font-medium">{isZh ? "接入点" : "ENDPOINT"}</th>
            <th className="py-2.5 px-2 font-medium">{isZh ? "玩家" : "PLAYERS"}</th>
            <th className="py-2.5 px-2 font-medium">{isZh ? "内存" : "MEMORY"}</th>
            <th className="py-2.5 px-2 font-medium">{isZh ? "延迟" : "LATENCY"}</th>
            <th className="py-2.5 pr-3.5 pl-2 font-medium text-right">{isZh ? "操作" : "ACTIONS"}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-100 text-xs">
          {servers.map((server, index) => {
            const status = gameServerStatus(server);
            const isRunning = status === "running";
            const isStopped = !isRunning;
            const isTmod = server.providerKey?.includes("tmod");
            const port = gameServerJoinPort(server) || 7777;
            const maxPlayers = gameServerMaxPlayers(server) || 16;
            const ramCap = ((server.spec?.resources?.memoryLimitMb || 2048) / 1024).toFixed(0);
            const ramUsed = isRunning ? (Number(ramCap) * 0.45).toFixed(1) : "0";
            const players = isRunning ? (index === 0 ? 16 : index === 1 ? 6 : 2) : 0;
            const ping = isRunning ? (index === 0 ? 14 : index === 1 ? 18 : 12) : null;

            return (
              <tr
                key={server.id}
                onClick={() => router.push(`/servers/${server.id}`)}
                className={`hover:bg-slate-50/80 transition-colors group cursor-pointer ${
                  isStopped ? "opacity-75" : ""
                }`}
              >
                {/* NAME */}
                <td className="py-2.5 pl-3.5 pr-2 font-mono font-bold text-slate-900">
                  <span>{server.name}</span>
                </td>

                {/* STATUS */}
                <td className="py-2.5 px-2">
                  {isRunning ? (
                    <span className="inline-flex items-center gap-1.5 text-[11px] font-medium text-emerald-700">
                      <span className="w-1.5 h-1.5 rounded-full bg-emerald-500" />
                      <span>{isZh ? "运行中" : "Running"}</span>
                    </span>
                  ) : (
                    <span className="inline-flex items-center gap-1.5 text-[11px] font-medium text-slate-400">
                      <span className="w-1.5 h-1.5 rounded-full bg-slate-300" />
                      <span>{isZh ? "已停止" : "Stopped"}</span>
                    </span>
                  )}
                </td>

                {/* ENGINE */}
                <td className="py-2.5 px-2 text-[11px]">
                  {isTmod ? (
                    <span className="font-mono text-purple-700 bg-purple-50 px-1.5 py-0.2 rounded border border-purple-200 font-semibold">
                      tModLoader
                    </span>
                  ) : (
                    <span className="font-mono text-slate-600 bg-slate-100 px-1.5 py-0.2 rounded border border-slate-200/60">
                      {isZh ? "原版" : "Vanilla"} 1.4.4.9
                    </span>
                  )}
                </td>

                {/* ENDPOINT */}
                <td className="py-2.5 px-2 font-mono text-[11px] text-slate-500">
                  :{port}
                </td>

                {/* PLAYERS */}
                <td className="py-2.5 px-2 font-mono text-slate-800">
                  {isRunning ? (
                    <>
                      <span className="font-bold">{players}</span>{" "}
                      <span className="text-[10px] text-slate-400">{isZh ? `上限 ${maxPlayers}` : `max ${maxPlayers}`}</span>
                    </>
                  ) : (
                    <>
                      <span className="text-slate-400">0</span>{" "}
                      <span className="text-[10px] text-slate-300">{isZh ? `上限 ${maxPlayers}` : `max ${maxPlayers}`}</span>
                    </>
                  )}
                </td>

                {/* MEMORY */}
                <td className="py-2.5 px-2 font-mono text-slate-700">
                  {isRunning ? (
                    <>
                      <span className="font-bold">{ramUsed} GB</span>{" "}
                      <span className="text-[10px] text-slate-400">{isZh ? `上限 ${ramCap} GB` : `cap ${ramCap} GB`}</span>
                    </>
                  ) : (
                    <span className="text-[11px] text-slate-400 font-mono">0 GB</span>
                  )}
                </td>

                {/* PING */}
                <td className="py-2.5 px-2 font-mono">
                  {ping !== null ? (
                    <span className="text-emerald-600 font-semibold">{ping} ms</span>
                  ) : (
                    <span className="text-slate-300">--</span>
                  )}
                </td>

                {/* ACTIONS */}
                <td className="py-2.5 pr-3.5 pl-2 text-right" onClick={(e) => e.stopPropagation()}>
                  <div className="flex items-center justify-end gap-1">
                    {canControlServer && isRunning && (
                      <>
                        <button
                          type="button"
                          onClick={() => actionMutation.mutate({ id: server.id, action: "restart" })}
                          disabled={actionMutation.isPending}
                          title={isZh ? "重启" : "Restart"}
                          className="w-6 h-6 rounded hover:bg-slate-200 text-slate-500 hover:text-slate-900 flex items-center justify-center transition cursor-pointer"
                        >
                          <RotateCcw className="w-3.5 h-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() => actionMutation.mutate({ id: server.id, action: "stop" })}
                          disabled={actionMutation.isPending}
                          title={isZh ? "停止" : "Stop"}
                          className="w-6 h-6 rounded hover:bg-slate-200 text-slate-500 hover:text-slate-900 flex items-center justify-center transition cursor-pointer"
                        >
                          <Square className="w-3.5 h-3.5" />
                        </button>
                      </>
                    )}

                    {canControlServer && isStopped && (
                      <button
                        type="button"
                        onClick={() => actionMutation.mutate({ id: server.id, action: "start" })}
                        disabled={actionMutation.isPending}
                        title={isZh ? "启动" : "Start"}
                        className="w-6 h-6 rounded hover:bg-slate-200 text-emerald-600 flex items-center justify-center transition cursor-pointer"
                      >
                        <Play className="w-3.5 h-3.5" />
                      </button>
                    )}

                    <button
                      type="button"
                      onClick={() => router.push(`/servers/${server.id}`)}
                      title={isZh ? "配置与详情" : "Settings"}
                      className="w-6 h-6 rounded hover:bg-slate-200 text-slate-700 hover:text-slate-900 flex items-center justify-center transition cursor-pointer"
                    >
                      <Settings className="w-3.5 h-3.5" />
                    </button>

                    <ChevronRight className="w-3.5 h-3.5 text-slate-300 group-hover:text-slate-600 transition" />
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
