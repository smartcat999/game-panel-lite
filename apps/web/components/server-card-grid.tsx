"use client";

import Link from "next/link";
import { useState } from "react";
import { Check, Copy, Cpu, HardDrive, Share2, Sliders, Zap } from "lucide-react";
import { ServerActions } from "@/components/server-actions";
import { ServerGameArt } from "@/components/server-game-art";
import { ServerProviderLabel, ServerStatusIndicator } from "@/components/server-badges";
import {
  gameServerJoinPort,
  gameServerMode,
  gameServerStatus,
  gameServerVersion
} from "@/lib/game-server-resource";
import { useI18n } from "@/lib/i18n";
import { useToast } from "@/components/toast-context";
import type { ObservabilityServerMetric } from "@/lib/api";
import type { GameServerResource } from "@/lib/types";

export function ServerCardGrid({
  servers,
  metrics = [],
  publicHost,
  limit
}: {
  servers: GameServerResource[];
  metrics?: ObservabilityServerMetric[];
  publicHost?: string;
  limit?: number;
}) {
  const { locale } = useI18n();
  const isZh = locale === "zh";
  const toast = useToast();
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [copiedInviteId, setCopiedInviteId] = useState<string | null>(null);
  const rows = typeof limit === "number" ? servers.slice(0, limit) : servers;
  const metricMap = new Map(metrics.map((metric) => [metric.id, metric]));

  const handleCopyAddress = (server: GameServerResource, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const port = gameServerJoinPort(server);
    const host = publicHost || (typeof window !== "undefined" ? window.location.hostname : "127.0.0.1");
    const address = `${host}:${port}`;
    navigator.clipboard.writeText(address);
    setCopiedId(server.id);
    toast.success(isZh ? "直连地址已复制" : "Address Copied", address);
    setTimeout(() => setCopiedId(null), 2000);
  };

  const handleCopyInvite = (server: GameServerResource, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const port = gameServerJoinPort(server);
    const host = publicHost || (typeof window !== "undefined" ? window.location.hostname : "127.0.0.1");
    const mode = gameServerMode(server);
    const inviteText = isZh
      ? `🎮 来一起开黑！【${server.name}】已就绪\n🌐 直连地址: ${host}:${port}\n⚡ 游戏模式: ${mode || "经典即开即玩"}\n🚀 复制后直接在游戏中输入 IP 端口即可加入！`
      : `🎮 Join my game room: ${server.name}\n🌐 Server Address: ${host}:${port}\n⚡ Mode: ${mode || "Standard"}`;

    navigator.clipboard.writeText(inviteText);
    setCopiedInviteId(server.id);
    toast.success(
      isZh ? "开黑邀请信息已复制！" : "Invite Copied!",
      isZh ? "直接粘贴到 QQ / 微信群即可邀请好友加入" : "Paste to chat to invite friends."
    );
    setTimeout(() => setCopiedInviteId(null), 2500);
  };

  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {rows.map((server) => {
        const metric = metricMap.get(server.id);
        const status = gameServerStatus(server);
        const isRunning = status === "running";
        const displayServer = { gameKey: server.gameKey, providerKey: server.providerKey, mode: gameServerMode(server) };
        const port = gameServerJoinPort(server);
        const host = publicHost || (typeof window !== "undefined" ? window.location.hostname : "127.0.0.1");
        const address = `${host}:${port}`;
        const isCopied = copiedId === server.id;
        const isInviteCopied = copiedInviteId === server.id;

        return (
          <div
            key={server.id}
            className="group relative flex flex-col justify-between overflow-hidden rounded-xl border border-slate-800 bg-slate-900/60 p-4 transition-all duration-200 hover:border-slate-700 hover:bg-slate-900/80 hover:shadow-xl hover:shadow-black/50"
          >
            <div>
              {/* Header: Cover + Title + Status Indicator */}
              <div className="flex items-start justify-between gap-3">
                <Link className="flex min-w-0 items-center gap-3" href={`/servers/${server.id}`}>
                  <ServerGameArt server={displayServer} className="size-11 rounded-lg ring-1 ring-white/10" />
                  <div className="min-w-0">
                    <h3 className="truncate font-bold text-slate-100 transition-colors group-hover:text-panel-green">
                      {server.name}
                    </h3>
                    <div className="mt-1 flex items-center gap-2">
                      <ServerProviderLabel server={displayServer} />
                      <span className="text-[11px] text-slate-500 font-mono">{gameServerVersion(server)}</span>
                    </div>
                  </div>
                </Link>
                <ServerStatusIndicator status={status} />
              </div>

              {/* Direct Join Address Bar */}
              <div className="mt-3.5 flex items-center justify-between rounded-lg border border-slate-800 bg-slate-950/80 px-2.5 py-1.5 text-xs font-mono text-slate-300">
                <div className="flex items-center gap-1.5 min-w-0 truncate">
                  <span className={isRunning ? "size-1.5 rounded-full bg-panel-green shrink-0" : "size-1.5 rounded-full bg-slate-600 shrink-0"} />
                  <span className="truncate">{address}</span>
                </div>
                <div className="flex items-center gap-1 shrink-0 ml-2">
                  <button
                    type="button"
                    onClick={(e) => handleCopyAddress(server, e)}
                    title={isZh ? "复制直连地址" : "Copy IP:Port"}
                    className="inline-flex items-center gap-1 rounded p-1 text-slate-400 transition hover:bg-slate-800 hover:text-slate-200"
                  >
                    {isCopied ? <Check className="size-3.5 text-panel-green" /> : <Copy className="size-3.5" />}
                    <span className="text-[10px]">{isCopied ? (isZh ? "已复制" : "Copied") : ""}</span>
                  </button>
                  <button
                    type="button"
                    onClick={(e) => handleCopyInvite(server, e)}
                    title={isZh ? "一键复制开黑邀请信息" : "Copy Invite Message"}
                    className="inline-flex items-center gap-1 rounded p-1 text-slate-400 transition hover:bg-slate-800 hover:text-panel-green"
                  >
                    {isInviteCopied ? <Check className="size-3.5 text-panel-green" /> : <Share2 className="size-3.5" />}
                    <span className="text-[10px] text-panel-green">{isInviteCopied ? (isZh ? "邀请已复制" : "Invite Copied") : ""}</span>
                  </button>
                </div>
              </div>

              {/* Reliable Room Specs & Resources Overview */}
              <div className="mt-3 grid grid-cols-2 gap-2 text-xs">
                {/* Mode & Port Spec */}
                <div className="rounded-lg border border-slate-800/80 bg-slate-950/40 p-2.5">
                  <div className="flex items-center justify-between text-slate-400">
                    <span className="inline-flex items-center gap-1.5 text-[11px]">
                      <Zap className="size-3 text-panel-gold" />
                      {isZh ? "直连端口" : "Port"}
                    </span>
                    <span className="font-mono font-semibold text-slate-200">{port}</span>
                  </div>
                  <div className="mt-1 flex items-center justify-between text-[11px] text-slate-500">
                    <span>{isZh ? "服务状态" : "State"}</span>
                    <span className={isRunning ? "text-panel-green font-medium" : "text-slate-500"}>
                      {isRunning ? (isZh ? "即开即玩" : "Ready") : (isZh ? "已离线" : "Offline")}
                    </span>
                  </div>
                </div>

                {/* Resource Allocation & Real Usage */}
                <div className="rounded-lg border border-slate-800/80 bg-slate-950/40 p-2.5">
                  <div className="flex items-center justify-between text-slate-400">
                    <span className="inline-flex items-center gap-1 text-[11px]">
                      <Cpu className="size-3 text-sky-400" /> CPU
                    </span>
                    <span className="font-mono text-slate-200">
                      {isRunning && metric?.statsAvailable ? `${metric.cpuPercent.toFixed(0)}%` : `${server.spec?.resources?.cpuLimitCores || 2}核`}
                    </span>
                  </div>
                  <div className="mt-1 flex items-center justify-between text-[11px] text-slate-400">
                    <span className="inline-flex items-center gap-1">
                      <HardDrive className="size-3 text-purple-400" /> RAM
                    </span>
                    <span className="font-mono text-slate-200">
                      {isRunning && metric?.statsAvailable ? `${Math.round(metric.memoryMb)}M` : `${Math.round((server.spec?.resources?.memoryLimitMb || 2048) / 1024)}GB`}
                    </span>
                  </div>
                </div>
              </div>
            </div>

            {/* Bottom Actions Bar */}
            <div className="mt-4 flex items-center justify-between border-t border-slate-800 pt-3">
              <Link
                href={`/servers/${server.id}`}
                className="inline-flex items-center gap-1.5 text-xs text-slate-400 transition hover:text-panel-green font-medium"
              >
                <Sliders className="size-3.5 text-panel-green" />
                <span>{isZh ? "房间配置与管理" : "Room Settings"}</span>
              </Link>
              <ServerActions server={server} rowMode showInvite={false} showDelete />
            </div>
          </div>
        );
      })}
    </div>
  );
}
