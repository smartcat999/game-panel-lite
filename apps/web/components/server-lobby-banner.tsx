"use client";

import { useState } from "react";
import { Check, Clock3, Copy, ExternalLink, KeyRound, Play, RotateCcw, Share2, Square, Zap } from "lucide-react";
import { ServerGameArt } from "@/components/server-game-art";
import { ServerModeBadge, ServerStatusBadge } from "@/components/server-badges";
import { useToast } from "@/components/toast-context";
import { useI18n } from "@/lib/i18n";
import { gameServerJoinPort, gameServerMode, gameServerPassword, gameServerStatus, gameServerVersion } from "@/lib/game-server-resource";
import type { GameServerResource } from "@/lib/types";

function formatServerUptime(server: GameServerResource) {
  if (gameServerStatus(server) !== "running" || !server.status.lastTransitionAt) {
    return "";
  }
  const startedAt = Date.parse(server.status.lastTransitionAt);
  if (!Number.isFinite(startedAt)) return "";
  const seconds = Math.max(0, Math.floor((Date.now() - startedAt) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  const restMinutes = minutes % 60;
  if (hours < 24) return restMinutes > 0 ? `${hours}h ${restMinutes}m` : `${hours}h`;
  const days = Math.floor(hours / 24);
  const restHours = hours % 24;
  return restHours > 0 ? `${days}d ${restHours}h` : `${days}d`;
}

export function ServerLobbyBanner({
  server,
  publicHost,
  canControl = true,
  disabled,
  onAction,
  onOpenShare
}: {
  server: GameServerResource;
  publicHost?: string;
  canControl?: boolean;
  disabled?: boolean;
  onAction: (action: "start" | "stop" | "restart") => void;
  onOpenShare?: () => void;
}) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const toast = useToast();

  const [copiedAddress, setCopiedAddress] = useState(false);
  const [copiedInvite, setCopiedInvite] = useState(false);

  const status = gameServerStatus(server);
  const isRunning = status === "running";
  const mode = gameServerMode(server);
  const port = gameServerJoinPort(server);
  const host = publicHost || (typeof window !== "undefined" ? window.location.hostname : "127.0.0.1");
  const joinAddress = `${host}:${port}`;
  const password = gameServerPassword(server);

  const displayServer = { gameKey: server.gameKey, providerKey: server.providerKey, mode };

  const handleCopyAddress = () => {
    navigator.clipboard.writeText(joinAddress);
    setCopiedAddress(true);
    toast.success(isZh ? "直连地址已复制" : "Address Copied", joinAddress);
    setTimeout(() => setCopiedAddress(false), 2000);
  };

  const handleCopyInvite = () => {
    let joinTip = isZh ? "复制后直接在游戏中输入 IP 端口即可加入！" : "Direct connect with IP and port.";
    if (server.gameKey === "palworld") {
      joinTip = isZh ? "在游戏主菜单点击【加入多人游戏(专用服务器)】并在底部输入 IP:Port 即可加入！" : "Select Join Multiplayer Game (Dedicated Server) and enter IP:Port.";
    } else if (server.gameKey === "minecraft") {
      joinTip = isZh ? "在游戏主菜单点击【多人游戏】->【添加服务器】并填入地址即可加入！" : "Add server in Minecraft Multiplayer menu.";
    } else if (server.gameKey === "dont-starve-together") {
      joinTip = isZh ? "在游戏内大厅搜索房间名或使用直连令牌即可加入！" : "Search server name in DST lobby.";
    }

    const inviteText = isZh
      ? `🎮 来一起开黑！【${server.name}】（${(server.gameKey || "Game").toUpperCase()}）房间已就绪\n🌐 直连地址: ${joinAddress}\n🔒 进服密码: ${password || "无密码"}\n⚡ 模式/版本: ${mode || "标准即开即玩"} (${gameServerVersion(server)})\n🚀 加入方式: ${joinTip}`
      : `🎮 Join my game room: ${server.name}\n🌐 Address: ${joinAddress}\n🔒 Password: ${password || "None"}\n⚡ Mode: ${mode || "Standard"}\n🚀 Tip: ${joinTip}`;

    navigator.clipboard.writeText(inviteText);
    setCopiedInvite(true);
    toast.success(
      isZh ? "开黑邀请信息已复制！" : "Invite Copied!",
      isZh ? "直接粘贴到 QQ / 微信群即可邀请好友加入" : "Paste to chat to invite friends."
    );
    setTimeout(() => setCopiedInvite(false), 2500);
  };

  const uptime = formatServerUptime(server);

  return (
    <div className="relative overflow-hidden rounded-2xl border border-slate-800 bg-gradient-to-b from-slate-900/90 via-slate-900/70 to-slate-950/90 p-5 sm:p-7 shadow-2xl backdrop-blur-xl">
      {/* Background Subtle Game Art Blur */}
      <div className="pointer-events-none absolute -right-16 -top-16 size-80 rounded-full bg-panel-green/5 blur-3xl" />
      <div className="pointer-events-none absolute -left-16 -bottom-16 size-80 rounded-full bg-sky-500/5 blur-3xl" />

      <div className="relative z-10 flex flex-col gap-6 lg:flex-row lg:items-center lg:justify-between">
        {/* Left: Server Profile & Badges */}
        <div className="flex items-start sm:items-center gap-4 min-w-0">
          <ServerGameArt server={displayServer} className="size-16 sm:size-20 shrink-0 rounded-2xl ring-2 ring-white/10 shadow-lg" />
          <div className="min-w-0 space-y-1.5">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="text-xl sm:text-2xl font-black tracking-tight text-white truncate">
                {server.name}
              </h1>
              <ServerModeBadge mode={mode} />
              <ServerStatusBadge status={status} />
            </div>

            <div className="flex flex-wrap items-center gap-3 text-xs text-slate-400 font-mono">
              <span className="inline-flex items-center gap-1 text-slate-300">
                <Zap className="size-3 text-panel-gold" />
                {isZh ? "直连端口" : "Port"}: <strong className="text-white">{port}</strong>
              </span>
              <span>·</span>
              <span>{gameServerVersion(server)}</span>
              {uptime ? (
                <>
                  <span>·</span>
                  <span className="inline-flex items-center gap-1 text-emerald-400 font-semibold">
                    <Clock3 className="size-3" />
                    <span>{isZh ? `已运行 ${uptime}` : `Up ${uptime}`}</span>
                  </span>
                </>
              ) : null}
              <span>·</span>
              <span className="text-slate-400">
                {isZh ? "创建于" : "Created"}: {new Date(server.createdAt).toLocaleDateString()}
              </span>
            </div>
          </div>
        </div>

        {/* Right: Quick Action Controls */}
        {canControl ? <div className="flex flex-wrap items-center gap-2.5 shrink-0">
          {isRunning ? (
            <>
              <button
                type="button"
                disabled={disabled}
                onClick={() => onAction("restart")}
                className="flex items-center gap-2 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-2.5 text-xs font-bold text-amber-300 shadow-sm transition hover:bg-amber-500/20 active:scale-95 disabled:opacity-50"
              >
                <RotateCcw className="size-4 text-amber-400" />
                <span>{isZh ? "重启房间" : "Restart"}</span>
              </button>
              <button
                type="button"
                disabled={disabled}
                onClick={() => onAction("stop")}
                className="flex items-center gap-2 rounded-xl border border-rose-500/30 bg-rose-500/10 px-4 py-2.5 text-xs font-bold text-rose-300 shadow-sm transition hover:bg-rose-500/20 active:scale-95 disabled:opacity-50"
              >
                <Square className="size-4 text-rose-400" />
                <span>{isZh ? "暂停房间" : "Pause Room"}</span>
              </button>
            </>
          ) : (
            <button
              type="button"
              disabled={disabled}
              onClick={() => onAction("start")}
              className="flex items-center gap-2 rounded-xl border border-panel-green/40 bg-panel-green px-5 py-2.5 text-xs font-bold text-slate-950 shadow-[0_0_20px_rgba(34,197,94,0.3)] transition hover:bg-panel-green/90 active:scale-95 disabled:opacity-50"
            >
              <Play className="size-4 fill-slate-950 text-slate-950" />
              <span>{isZh ? "一键启动房间" : "Start Game Room"}</span>
            </button>
          )}
        </div> : null}
      </div>

      {/* Bottom Connect & Share Bar */}
      <div className="mt-6 pt-5 border-t border-slate-800/80 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        {/* Connection Coordinates */}
        <div className="flex flex-wrap items-center gap-2.5 sm:gap-4 text-xs font-mono">
          <div className="flex items-center gap-2 rounded-lg border border-slate-800 bg-slate-950/80 px-3 py-1.5 text-slate-200">
            <span className={isRunning ? "size-2 rounded-full bg-panel-green animate-pulse" : "size-2 rounded-full bg-slate-600"} />
            <span className="text-slate-400 font-sans">{isZh ? "直连地址:" : "Address:"}</span>
            <strong className="text-white tracking-wide">{joinAddress}</strong>
            <button
              type="button"
              onClick={handleCopyAddress}
              title={isZh ? "复制 IP 端口" : "Copy Address"}
              className="ml-1 text-slate-400 hover:text-panel-green transition"
            >
              {copiedAddress ? <Check className="size-3.5 text-panel-green" /> : <Copy className="size-3.5" />}
            </button>
          </div>

          <div className="flex items-center gap-2 rounded-lg border border-slate-800 bg-slate-950/80 px-3 py-1.5 text-slate-300">
            <KeyRound className="size-3.5 text-slate-400" />
            <span className="text-slate-400 font-sans">{isZh ? "进服密码:" : "Password:"}</span>
            <span className={password ? "font-bold text-panel-gold" : "text-slate-500 font-sans"}>
              {password ? password : (isZh ? "无密码 (公开)" : "None")}
            </span>
          </div>
        </div>

        {/* Action Share Buttons */}
        <div className="flex items-center gap-2.5 shrink-0">
          <button
            type="button"
            onClick={handleCopyInvite}
            className="flex items-center gap-1.5 rounded-lg border border-panel-green/30 bg-panel-green/10 px-3.5 py-1.5 text-xs font-bold text-panel-green transition hover:bg-panel-green/20"
          >
            {copiedInvite ? <Check className="size-3.5 text-panel-green" /> : <Share2 className="size-3.5" />}
            <span>{copiedInvite ? (isZh ? "邀请已复制！" : "Copied!") : (isZh ? "一键复制开黑群邀请" : "Copy Invite")}</span>
          </button>

          {onOpenShare ? (
            <button
              type="button"
              onClick={onOpenShare}
              className="flex items-center gap-1.5 rounded-lg border border-slate-800 bg-slate-900/60 px-3 py-1.5 text-xs font-medium text-slate-300 transition hover:bg-slate-800 hover:text-white"
            >
              <ExternalLink className="size-3.5 text-slate-400" />
              <span>{isZh ? "公开玩家邀请页" : "Public Share"}</span>
            </button>
          ) : null}
        </div>
      </div>
    </div>
  );
}
