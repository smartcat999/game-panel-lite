"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Check,
  ChevronDown,
  ChevronUp,
  Copy,
  History,
  Info,
  RefreshCw,
  Users,
  X
} from "lucide-react";
import Image from "next/image";
import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Button } from "@/components/ui";
import {
  banServerPlayer,
  kickServerPlayer,
  listServerPlayers,
  sendServerCommand
} from "@/lib/api";
import { gameServerStatus } from "@/lib/game-server-resource";
import { useI18n } from "@/lib/i18n";
import { providerConfigValue } from "@/lib/provider-config";
import { cn } from "@/lib/utils";
import type { GameServerResource, ServerPlayer } from "@/lib/types";

type Shard = "master" | "caves";
type CommandKind = "command" | "save" | "rollback" | "announce" | "player";
type CommandRecord = {
  id: string;
  command: string;
  kind: CommandKind;
  shard: Shard;
  status: "queued" | "completed" | "failed";
  time: string;
};

import { getDSTCharacterDisplayName, getDSTCharacterImage } from "@/lib/dst-characters";

export function DSTConsoleDrawer({
  open,
  server,
  onClose
}: {
  open: boolean;
  server: GameServerResource;
  onClose: () => void;
}) {
  const { locale } = useI18n();
  const queryClient = useQueryClient();
  const isZh = locale.startsWith("zh");
  const bottomInputRef = useRef<HTMLInputElement>(null);

  const [activeTab, setActiveTab] = useState<"actions" | "terminal">("actions");
  const [shard, setShard] = useState<Shard>("master");
  const [command, setCommand] = useState("");
  const [historyIndex, setHistoryIndex] = useState<number | null>(null);
  const [announceText, setAnnounceText] = useState("");
  const [rollbackDays, setRollbackDays] = useState(1);
  const [confirmRollback, setConfirmRollback] = useState(false);
  const [playerSearch, setPlayerSearch] = useState("");
  const [playersCollapsed, setPlayersCollapsed] = useState(false);
  const [records, setRecords] = useState<CommandRecord[]>([]);
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [confirmModal, setConfirmModal] = useState<{
    type: "kick" | "ban";
    player: ServerPlayer;
  } | null>(null);

  const cavesEnabled = providerConfigValue(server.spec.config, "caves.enabled") === true;
  const maxPlayers = Number(providerConfigValue(server.spec.config, "gameplay.maxPlayers")) || 16;
  const status = gameServerStatus(server);
  const running = status === "running";

  const copy = useMemo(
    () =>
      isZh
        ? {
            title: "控制台",
            tabActions: "常用控制",
            tabTerminal: "自由终端",
            master: "地上世界",
            caves: "洞穴世界",
            cavesDisabled: "洞穴未启用",
            unavailable: "服务器停止运行",
            onlinePlayers: "在线玩家",
            officialData: "官方数据",
            searchPlaceholder: "搜索玩家/ID...",
            refresh: "刷新",
            collapse: "收起",
            expand: "展开",
            hostBadge: "房主",
            kick: "踢出",
            ban: "封禁",
            noPlayers: "当前暂无玩家在线",
            noMatchingPlayers: "未找到匹配玩家",
            manyPlayersHint: (count: number) => `向下滚动查看全部 ${count} 位在线玩家 · 支持搜索快速定位`,
            safeActions: "安全快捷指令",
            saveProgress: "立即保存世界进度",
            saveSubtitle: "c_save()",
            rollbackTitle: "游戏天数回退",
            rollbackCode: "c_rollback()",
            daysUnit: "天",
            confirmRollbackBtn: "确认回档",
            cancel: "取消",
            rollbackWarning: (days: number) => `确认将世界回滚 ${days} 天？未保存的当日进度将重置。`,
            announceTitle: "全服广播公告",
            announcePlaceholder: "输入公告内容，按回车直接发送...",
            sendAnnounce: "发送广播",
            terminalHint: "DST Lua 控制台指令直通服务器输入管道。指令回显与服务器日志请在「运行日志」面板中查看。",
            quickSnippets: "快捷指令模板",
            recentTitle: "下发历史",
            clearHistory: "清空",
            emptyHistory: "暂无操作记录",
            lastDispatched: "最近下发",
            dispatchedSuccess: "已成功投递 ✓",
            cliPlaceholder: "输入任意游戏 Lua 控制台指令，Enter 快捷发送...",
            confirmKickTitle: "确认踢出玩家",
            confirmKickMsg: (name: string, id?: string) =>
              `确定要将玩家 ${name} ${id ? `(${id})` : ""} 踢出当前房间吗？`,
            confirmBanTitle: "确认封禁玩家",
            confirmBanMsg: (name: string, id?: string) =>
              `确定要将玩家 ${name} ${id ? `(${id})` : ""} 加入黑名单并踢出吗？`
          }
        : {
            title: "Console",
            tabActions: "Actions",
            tabTerminal: "Terminal",
            master: "Overworld",
            caves: "Caves",
            cavesDisabled: "Caves disabled",
            unavailable: "Server is stopped",
            onlinePlayers: "Online Players",
            officialData: "Live",
            searchPlaceholder: "Search player/ID...",
            refresh: "Refresh",
            collapse: "Collapse",
            expand: "Expand",
            hostBadge: "Host",
            kick: "Kick",
            ban: "Ban",
            noPlayers: "No players currently online",
            noMatchingPlayers: "No matching players found",
            manyPlayersHint: (count: number) => `Scroll to view all ${count} online players`,
            safeActions: "Quick Actions",
            saveProgress: "Save World Progress",
            saveSubtitle: "c_save()",
            rollbackTitle: "Rollback World Days",
            rollbackCode: "c_rollback()",
            daysUnit: "d",
            confirmRollbackBtn: "Rollback",
            cancel: "Cancel",
            rollbackWarning: (days: number) => `Rollback ${days} day(s)? Unsaved progress will reset.`,
            announceTitle: "Broadcast Announcement",
            announcePlaceholder: "Announcement message, Enter to send...",
            sendAnnounce: "Broadcast",
            terminalHint: "DST Lua console commands are piped directly to stdin. Check stdout logs in the Logs tab.",
            quickSnippets: "Snippets",
            recentTitle: "Recent Dispatches",
            clearHistory: "Clear",
            emptyHistory: "No commands dispatched yet",
            lastDispatched: "Last sent",
            dispatchedSuccess: "Delivered ✓",
            cliPlaceholder: "Enter DST Lua console command, Enter to dispatch...",
            confirmKickTitle: "Confirm Kick Player",
            confirmKickMsg: (name: string, id?: string) =>
              `Are you sure you want to kick ${name} ${id ? `(${id})` : ""}?`,
            confirmBanTitle: "Confirm Ban Player",
            confirmBanMsg: (name: string, id?: string) =>
              `Are you sure you want to ban ${name} ${id ? `(${id})` : ""}?`
          },
    [isZh]
  );

  // Poll online players
  const playersQuery = useQuery({
    queryKey: ["servers", server.id, "players"],
    queryFn: () => listServerPlayers(server.id),
    enabled: open && running,
    refetchInterval: open && running ? 6000 : false
  });

  const rawPlayers = useMemo(() => {
    return playersQuery.data?.players ?? [];
  }, [playersQuery.data?.players]);

  const [isRefreshing, setIsRefreshing] = useState(false);
  const handleManualRefresh = async () => {
    if (!running) return;
    setIsRefreshing(true);
    try {
      const res = await listServerPlayers(server.id, true);
      queryClient.setQueryData(["servers", server.id, "players"], res);
    } catch {
      // ignore
    } finally {
      setIsRefreshing(false);
    }
  };

  const filteredPlayers = useMemo(() => {
    if (!playerSearch.trim()) return rawPlayers;
    const q = playerSearch.trim().toLowerCase();
    return rawPlayers.filter((p) => {
      const name = (p.name || "").toLowerCase();
      const id = (p.userId || "").toLowerCase();
      const char = (p.character || "").toLowerCase();
      return name.includes(q) || id.includes(q) || char.includes(q);
    });
  }, [rawPlayers, playerSearch]);

  const commandHistory = useMemo(() => {
    return records.filter((r) => r.kind === "command").map((r) => r.command);
  }, [records]);

  const mutation = useMutation({
    mutationFn: ({ value, target, kind }: { value: string; target: Shard; kind: CommandKind }) =>
      sendServerCommand(server.id, value, target).then((result) => ({ result, value, target, kind })),
    onSuccess: ({ result, value, target, kind }) => {
      setRecords((current) => [
        {
          id: result.id || crypto.randomUUID(),
          command: value,
          kind,
          shard: target,
          status: "queued",
          time: new Date().toLocaleTimeString(locale === "zh" ? "zh-CN" : "en-US", {
            hour: "2-digit",
            minute: "2-digit",
            second: "2-digit"
          })
        } as CommandRecord,
        ...current
      ].slice(0, 30));
      if (kind === "command") {
        setCommand("");
        setHistoryIndex(null);
      }
      if (kind === "announce") {
        setAnnounceText("");
      }
      if (kind === "rollback") {
        setConfirmRollback(false);
      }
    },
    onError: (_error, variables) => {
      setRecords((current) => [
        {
          id: crypto.randomUUID(),
          command: variables.value,
          kind: variables.kind,
          shard: variables.target,
          status: "failed",
          time: new Date().toLocaleTimeString(locale === "zh" ? "zh-CN" : "en-US", {
            hour: "2-digit",
            minute: "2-digit",
            second: "2-digit"
          })
        } as CommandRecord,
        ...current
      ].slice(0, 30));
    }
  });

  const kickMutation = useMutation({
    mutationFn: (target: string) => kickServerPlayer(server.id, target),
    onSuccess: () => {
      setConfirmModal(null);
      queryClient.invalidateQueries({ queryKey: ["servers", server.id, "players"] });
    }
  });

  const banMutation = useMutation({
    mutationFn: (target: string) => banServerPlayer(server.id, target),
    onSuccess: () => {
      setConfirmModal(null);
      queryClient.invalidateQueries({ queryKey: ["servers", server.id, "players"] });
    }
  });

  useEffect(() => {
    if (!open) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose, open]);

  useEffect(() => {
    if (!cavesEnabled && shard === "caves") setShard("master");
  }, [cavesEnabled, shard]);

  useEffect(() => {
    if (open) {
      window.requestAnimationFrame(() => bottomInputRef.current?.focus());
    }
  }, [open, activeTab]);

  if (!open) return null;

  const dispatch = (value: string, kind: CommandKind, target = shard) => {
    const next = value.trim();
    if (!running || !next || mutation.isPending) return;
    mutation.mutate({ value: next, target, kind });
  };

  const handleBottomSubmit = (event: FormEvent) => {
    event.preventDefault();
    dispatch(command, "command");
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (commandHistory.length === 0) return;
    if (event.key === "ArrowUp") {
      event.preventDefault();
      const nextIndex = historyIndex === null ? 0 : Math.min(historyIndex + 1, commandHistory.length - 1);
      setHistoryIndex(nextIndex);
      setCommand(commandHistory[nextIndex] || "");
    } else if (event.key === "ArrowDown") {
      event.preventDefault();
      if (historyIndex !== null) {
        const nextIndex = historyIndex - 1;
        if (nextIndex < 0) {
          setHistoryIndex(null);
          setCommand("");
        } else {
          setHistoryIndex(nextIndex);
          setCommand(commandHistory[nextIndex] || "");
        }
      }
    }
  };

  const copyText = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 1500);
  };

  const shardLabel = shard === "master" ? copy.master : copy.caves;
  const lastRecord = records[0];

  const quickSnippets = [
    { label: isZh ? "统计对象" : "Count Objects", cmd: "c_countprefabs('world')" },
    { label: isZh ? "管理员模式" : "Super Godmode", cmd: "c_supergodmode()" },
    { label: isZh ? "重载世界" : "Reset World", cmd: "c_reset()" },
    { label: isZh ? "客户端网络表" : "Client Table", cmd: "TheNet:GetClientTable()" },
    { label: isZh ? "全服公告模板" : "System Message", cmd: 'TheNet:SystemMessage("Server Announcement")' }
  ];

  return (
    <aside
      aria-label={copy.title}
      className="fixed bottom-0 right-0 top-14 z-40 flex w-full flex-col border-l border-[#1b2434] bg-[#090e17] shadow-[-12px_0_36px_rgba(0,0,0,0.55)] sm:w-[450px]"
      role="complementary"
    >
      {/* 1. Header (48px) */}
      <header className="flex h-12 shrink-0 items-center justify-between border-b border-[#1b2434] px-4 bg-[#0c121d]">
        <div className="flex items-center gap-2 min-w-0">
          <div className="flex size-6 shrink-0 items-center justify-center rounded-md border border-emerald-500/30 bg-emerald-500/10 text-emerald-400 font-mono text-xs font-bold">
            &gt;_
          </div>
          <span className="text-xs font-semibold text-slate-100">{copy.title}</span>
          <span className="text-slate-600">·</span>
          <span className="text-xs text-slate-400 truncate">{server.name}</span>
        </div>
        <div className="flex items-center gap-2">
          {running && (
            <span className="flex items-center gap-1.5 rounded-full bg-emerald-500/10 border border-emerald-500/20 px-2 py-0.5 text-[11px] font-medium text-emerald-400">
              <span className="size-1.5 rounded-full bg-emerald-400 animate-pulse" />
              <span>
                {rawPlayers.length}/{maxPlayers} 在线
              </span>
            </span>
          )}
          <button
            type="button"
            onClick={onClose}
            aria-label="关闭控制台"
            className="text-slate-500 hover:text-slate-300 p-1 rounded transition text-xs"
          >
            <X className="size-4" />
          </button>
        </div>
      </header>

      {/* 2. Sub-bar: Modes & Shards (38px) */}
      <div className="flex items-center justify-between border-b border-[#1b2434] bg-[#070b13] px-3.5 py-1.5">
        {/* Mode Tabs (Left) */}
        <div className="flex items-center rounded-lg border border-slate-800 bg-slate-950 p-0.5">
          <button
            type="button"
            onClick={() => setActiveTab("actions")}
            className={cn(
              "rounded px-2.5 py-1 text-xs transition",
              activeTab === "actions"
                ? "bg-slate-800 text-emerald-400 font-semibold"
                : "text-slate-400 hover:text-slate-200 font-medium"
            )}
          >
            {copy.tabActions}
          </button>
          <button
            type="button"
            onClick={() => setActiveTab("terminal")}
            className={cn(
              "rounded px-2.5 py-1 text-xs transition",
              activeTab === "terminal"
                ? "bg-slate-800 text-emerald-400 font-semibold"
                : "text-slate-400 hover:text-slate-200 font-medium"
            )}
          >
            {copy.tabTerminal}
          </button>
        </div>

        {/* Shard Pills (Right) */}
        <div className="flex items-center rounded-lg border border-slate-800 bg-slate-950 p-0.5">
          <button
            type="button"
            onClick={() => setShard("master")}
            className={cn(
              "flex items-center gap-1.5 rounded px-2 py-1 text-xs transition",
              shard === "master"
                ? "bg-emerald-500/15 text-emerald-400 font-semibold"
                : "text-slate-400 hover:text-slate-200 font-medium"
            )}
          >
            <span className={cn("size-1.5 rounded-full", shard === "master" ? "bg-emerald-400" : "bg-slate-600")} />
            <span>{copy.master}</span>
          </button>
          <button
            type="button"
            disabled={!cavesEnabled}
            onClick={() => cavesEnabled && setShard("caves")}
            title={!cavesEnabled ? copy.cavesDisabled : undefined}
            className={cn(
              "flex items-center gap-1.5 rounded px-2 py-1 text-xs transition disabled:opacity-30 disabled:cursor-not-allowed",
              shard === "caves"
                ? "bg-emerald-500/15 text-emerald-400 font-semibold"
                : "text-slate-400 hover:text-slate-200 font-medium"
            )}
          >
            <span className={cn("size-1.5 rounded-full", shard === "caves" ? "bg-emerald-400" : "bg-slate-600")} />
            <span>{copy.caves}</span>
          </button>
        </div>
      </div>

      {/* 3. Main Scrollable Area */}
      <div className="flex-1 overflow-y-auto p-3.5 space-y-3.5">
        {!running && (
          <div className="flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-400">
            <AlertTriangle className="size-3.5 shrink-0" />
            <span>{copy.unavailable}</span>
          </div>
        )}

        {/* Section: 在线玩家管理 (官方角色头像 + 独立限高滚动) */}
        <div className="rounded-xl border border-slate-800/80 bg-[#0d131f] p-3 space-y-2.5">
          {/* Header */}
          <div className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <Users className="size-3.5 text-emerald-400" />
              <span className="text-xs font-semibold text-slate-200">
                {copy.onlinePlayers} ({rawPlayers.length}/{maxPlayers})
              </span>
              <span className="rounded bg-emerald-500/15 text-emerald-400 border border-emerald-500/30 px-1.5 py-0.2 text-[10px] font-mono">
                {copy.officialData}
              </span>
            </div>

            {/* Search Box & Controls */}
            <div className="flex items-center gap-1.5">
              <div className="relative">
                <input
                  type="text"
                  value={playerSearch}
                  onChange={(e) => setPlayerSearch(e.target.value)}
                  placeholder={copy.searchPlaceholder}
                  className="h-6 w-28 rounded border border-slate-800 bg-slate-950 px-2 text-[11px] text-slate-200 placeholder:text-slate-600 outline-none focus:border-emerald-500/50"
                />
              </div>
              <button
                type="button"
                title={copy.refresh}
                disabled={isRefreshing || playersQuery.isFetching}
                onClick={handleManualRefresh}
                className="size-6 flex items-center justify-center rounded border border-slate-800 bg-slate-950 text-slate-400 hover:text-white transition text-xs disabled:opacity-50"
              >
                <RefreshCw className={cn("size-3", (playersQuery.isFetching || isRefreshing) && "animate-spin")} />
              </button>
              <button
                type="button"
                title={playersCollapsed ? copy.expand : copy.collapse}
                onClick={() => setPlayersCollapsed(!playersCollapsed)}
                className="size-6 flex items-center justify-center rounded border border-slate-800 bg-slate-950 text-slate-400 hover:text-white transition text-xs"
              >
                {playersCollapsed ? <ChevronDown className="size-3" /> : <ChevronUp className="size-3" />}
              </button>
            </div>
          </div>

          {!playersCollapsed && (
            <>
              {/* Scrollable Player Table (Max Height 190px, shows ~4 rows, scrolls smoothly for 8-32 players) */}
              <div className="max-h-[190px] overflow-y-auto space-y-1.5 pr-1 border border-slate-800/50 rounded-lg p-1.5 bg-slate-950/60">
                {filteredPlayers.length === 0 ? (
                  <div className="py-5 text-center text-xs text-slate-500">
                    {rawPlayers.length === 0 ? copy.noPlayers : copy.noMatchingPlayers}
                  </div>
                ) : (
                  filteredPlayers.map((player, idx) => {
                    const charImg = getDSTCharacterImage(player.character);
                    const charName = getDSTCharacterDisplayName(player.character, isZh);
                    const playerName = player.name || `Player ${idx + 1}`;
                    return (
                      <div
                        key={player.userId || player.name || idx}
                        className="flex items-center justify-between gap-2 rounded-md bg-[#0e1626] border border-slate-800/60 px-2.5 py-1.5 text-xs hover:border-slate-700 transition"
                      >
                        <div className="flex items-center gap-2.5 min-w-0">
                          {charImg ? (
                            <Image
                              src={charImg}
                              alt={charName || player.character || "DST Character"}
                              width={28}
                              height={28}
                              className="size-7 rounded-full object-cover border border-slate-700 bg-slate-900 shrink-0"
                            />
                          ) : (
                            <div className="flex size-7 shrink-0 items-center justify-center rounded-full bg-slate-800 border border-slate-700 text-xs font-bold text-slate-300">
                              {(player.name || "P").charAt(0).toUpperCase()}
                            </div>
                          )}

                          <span className="font-medium text-slate-200 truncate">{playerName}</span>

                          {charName && (
                            <span className="text-[10px] font-medium bg-purple-500/15 text-purple-300 px-1 rounded border border-purple-500/30 shrink-0">
                              {charName}
                            </span>
                          )}

                          {player.userId && (
                            <span
                              className="font-mono text-[11px] text-slate-400 bg-slate-900 px-1 rounded truncate max-w-[85px]"
                              title={player.userId}
                            >
                              {player.userId}
                            </span>
                          )}

                          {player.isHost && (
                            <span className="text-[10px] font-semibold bg-amber-500/15 text-amber-400 px-1 rounded border border-amber-500/30 shrink-0">
                              {copy.hostBadge}
                            </span>
                          )}
                        </div>

                        <div className="flex items-center gap-1 shrink-0">
                          <button
                            type="button"
                            onClick={() => setConfirmModal({ type: "kick", player })}
                            className="px-2 py-0.5 rounded border border-slate-700 bg-slate-800 text-[11px] text-slate-300 hover:bg-slate-700 transition"
                          >
                            {copy.kick}
                          </button>
                          <button
                            type="button"
                            onClick={() => setConfirmModal({ type: "ban", player })}
                            className="px-2 py-0.5 rounded border border-red-900/40 bg-red-950/40 text-[11px] text-red-400 hover:bg-red-900/50 transition"
                          >
                            {copy.ban}
                          </button>
                        </div>
                      </div>
                    );
                  })
                )}
              </div>

              {rawPlayers.length > 3 && (
                <div className="text-[11px] text-slate-500 text-center">
                  {copy.manyPlayersHint(rawPlayers.length)}
                </div>
              )}
            </>
          )}
        </div>

        {activeTab === "actions" ? (
          /* Tab: 常用控制 */
          <div className="rounded-xl border border-slate-800/80 bg-[#0d131f] p-3 space-y-2.5">
            <span className="text-xs font-semibold text-slate-200 block">{copy.safeActions}</span>

            {/* 1. 立即保存世界进度 */}
            <button
              type="button"
              disabled={!running || mutation.isPending}
              onClick={() => dispatch("c_save()", "save", shard)}
              className="w-full flex items-center justify-center gap-2 rounded-lg bg-emerald-500/15 hover:bg-emerald-500/25 border border-emerald-500/30 text-emerald-400 py-2 px-3 text-xs font-medium transition disabled:opacity-40"
            >
              <span>💾 {copy.saveProgress}</span>
              <span className="font-mono text-[11px] text-emerald-300/70">{copy.saveSubtitle}</span>
            </button>

            {/* 2. 游戏天数回退 (c_rollback) */}
            <div className="rounded-lg border border-slate-800 bg-slate-950/70 p-2.5 space-y-2">
              <div className="flex items-center justify-between text-xs">
                <span className="text-slate-300 font-medium">{copy.rollbackTitle}</span>
                <span className="font-mono text-[11px] text-slate-500">{copy.rollbackCode}</span>
              </div>
              <div className="flex items-center justify-between gap-1.5">
                <div className="flex items-center gap-1">
                  {[1, 2, 3, 5].map((days) => (
                    <button
                      key={days}
                      type="button"
                      onClick={() => {
                        setRollbackDays(days);
                        setConfirmRollback(false);
                      }}
                      className={cn(
                        "px-2 py-1 rounded text-xs transition",
                        rollbackDays === days
                          ? "border border-amber-500/40 bg-amber-500/15 text-amber-300 font-semibold"
                          : "border border-slate-800 bg-slate-900 text-slate-400 hover:text-slate-200"
                      )}
                    >
                      {days}
                      {copy.daysUnit}
                    </button>
                  ))}
                </div>
                {!confirmRollback ? (
                  <button
                    type="button"
                    disabled={!running || mutation.isPending}
                    onClick={() => setConfirmRollback(true)}
                    className="px-3 py-1 rounded text-xs font-semibold bg-amber-500 hover:bg-amber-400 text-slate-950 transition disabled:opacity-40"
                  >
                    {copy.confirmRollbackBtn}
                  </button>
                ) : (
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      onClick={() => setConfirmRollback(false)}
                      className="px-2 py-1 rounded text-xs text-slate-400 hover:text-white"
                    >
                      {copy.cancel}
                    </button>
                    <button
                      type="button"
                      disabled={!running || mutation.isPending}
                      onClick={() => dispatch(`c_rollback(${rollbackDays})`, "rollback", "master")}
                      className="px-2.5 py-1 rounded text-xs font-semibold bg-red-600 hover:bg-red-500 text-white transition"
                    >
                      确认
                    </button>
                  </div>
                )}
              </div>
              {confirmRollback && (
                <div className="text-[11px] text-amber-400 bg-amber-500/10 border border-amber-500/20 rounded p-1.5">
                  {copy.rollbackWarning(rollbackDays)}
                </div>
              )}
            </div>

            {/* 3. 全服广播 (Single row) */}
            <form
              onSubmit={(e) => {
                e.preventDefault();
                if (!announceText.trim()) return;
                dispatch(`TheNet:SystemMessage(${JSON.stringify(announceText.trim())})`, "announce", "master");
              }}
              className="rounded-lg border border-slate-800 bg-slate-950/70 p-2.5 space-y-1.5"
            >
              <span className="text-xs text-slate-300 font-medium block">{copy.announceTitle}</span>
              <div className="flex items-center gap-1.5">
                <input
                  type="text"
                  value={announceText}
                  onChange={(e) => setAnnounceText(e.target.value)}
                  placeholder={copy.announcePlaceholder}
                  disabled={!running || mutation.isPending}
                  className="h-8 flex-1 rounded border border-slate-800 bg-slate-950 px-2.5 text-xs text-slate-200 placeholder:text-slate-600 outline-none focus:border-emerald-500/50"
                />
                <button
                  type="submit"
                  disabled={!running || !announceText.trim() || mutation.isPending}
                  className="h-8 px-3 rounded bg-slate-800 hover:bg-slate-700 text-xs font-medium text-slate-200 transition disabled:opacity-40 shrink-0"
                >
                  {copy.sendAnnounce}
                </button>
              </div>
            </form>
          </div>
        ) : (
          /* Tab: 自由终端 */
          <div className="rounded-xl border border-slate-800/80 bg-[#0d131f] p-3 space-y-3">
            <div className="flex items-start gap-2 rounded-lg border border-slate-800 bg-slate-950/80 p-2.5 text-xs text-slate-400">
              <Info className="size-4 shrink-0 text-emerald-400 mt-0.5" />
              <span>{copy.terminalHint}</span>
            </div>

            {/* Snippets */}
            <div className="space-y-1.5">
              <span className="text-xs text-slate-400 font-medium block">{copy.quickSnippets}</span>
              <div className="flex flex-wrap gap-1.5">
                {quickSnippets.map((s) => (
                  <button
                    key={s.cmd}
                    type="button"
                    onClick={() => {
                      setCommand(s.cmd);
                      bottomInputRef.current?.focus();
                    }}
                    className="flex items-center gap-1 rounded border border-slate-800 bg-slate-950 px-2 py-1 text-xs text-slate-300 hover:border-emerald-500/40 hover:text-white transition"
                  >
                    <span>{s.label}</span>
                    <code className="font-mono text-[10px] text-slate-500">{s.cmd}</code>
                  </button>
                ))}
              </div>
            </div>
          </div>
        )}

        {/* History Stream */}
        <div className="rounded-xl border border-slate-800/80 bg-[#0d131f] p-3 space-y-2">
          <div className="flex items-center justify-between text-xs">
            <div className="flex items-center gap-1.5 text-slate-400 font-medium">
              <History className="size-3 text-emerald-400" />
              <span>{copy.recentTitle}</span>
              {records.length > 0 && (
                <span className="font-mono text-[10px] text-slate-500">({records.length})</span>
              )}
            </div>
            {records.length > 0 && (
              <button
                type="button"
                onClick={() => setRecords([])}
                className="text-[10px] text-slate-500 hover:text-slate-300 transition"
              >
                {copy.clearHistory}
              </button>
            )}
          </div>

          {records.length === 0 ? (
            <div className="py-3 text-center text-xs text-slate-600">{copy.emptyHistory}</div>
          ) : (
            <div className="space-y-1 max-h-36 overflow-y-auto pr-1">
              {records.map((r) => (
                <div
                  key={r.id}
                  className="flex items-center justify-between gap-2 rounded px-2 py-1 text-xs bg-slate-950/50 hover:bg-slate-950 transition"
                >
                  <div className="flex items-center gap-1.5 min-w-0">
                    <span className="font-mono text-[10px] text-slate-500 shrink-0">{r.time}</span>
                    <span
                      className={cn(
                        "rounded px-1 text-[9px] font-mono shrink-0",
                        r.shard === "master"
                          ? "bg-emerald-500/10 text-emerald-400"
                          : "bg-purple-500/10 text-purple-400"
                      )}
                    >
                      {r.shard === "master" ? "地上" : "洞穴"}
                    </span>
                    <code className="truncate font-mono text-[11px] text-slate-300" title={r.command}>
                      {r.command}
                    </code>
                  </div>
                  <div className="flex items-center gap-1.5 shrink-0">
                    <span
                      className={cn(
                        "text-[10px] font-medium",
                        r.status === "failed" ? "text-red-400" : "text-emerald-400"
                      )}
                    >
                      {r.status === "failed" ? "失败" : "已投递"}
                    </span>
                    <button
                      type="button"
                      onClick={() => copyText(r.command, r.id)}
                      className="text-slate-500 hover:text-white transition p-0.5"
                    >
                      {copiedId === r.id ? <Check className="size-2.5 text-emerald-400" /> : <Copy className="size-2.5" />}
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* 4. Pinned Bottom CLI Input Area */}
      <div className="border-t border-[#1b2434] bg-[#070b13] p-3 space-y-1.5">
        <div className="flex items-center justify-between text-[11px] text-slate-400">
          <span className="flex items-center gap-1.5 min-w-0 truncate">
            <span className="size-1.5 rounded-full bg-emerald-400 shrink-0" />
            <span className="truncate">
              {copy.lastDispatched}:{" "}
              <code className="font-mono text-slate-300">
                {lastRecord ? lastRecord.command : "c_save()"}
              </code>
            </span>
          </span>
          <span className="text-emerald-400 font-medium shrink-0">{copy.dispatchedSuccess}</span>
        </div>

        <form
          onSubmit={handleBottomSubmit}
          className="flex items-center gap-2 rounded-lg border border-slate-800 bg-slate-950 px-2.5 py-1 focus-within:border-emerald-500/60"
        >
          <span className="font-mono text-xs font-bold text-emerald-400 select-none shrink-0">
            [{shardLabel}]&gt;
          </span>
          <input
            ref={bottomInputRef}
            type="text"
            value={command}
            disabled={!running || mutation.isPending}
            onChange={(e) => setCommand(e.target.value.replace(/[\r\n]/g, ""))}
            onKeyDown={handleKeyDown}
            placeholder={copy.cliPlaceholder}
            className="h-7 flex-1 bg-transparent font-mono text-xs text-slate-100 outline-none placeholder:text-slate-600"
          />
          <button
            type="submit"
            disabled={!running || !command.trim() || mutation.isPending}
            className="flex size-7 items-center justify-center rounded bg-emerald-500 hover:bg-emerald-400 text-slate-950 font-bold transition text-xs disabled:opacity-30 shrink-0"
          >
            ↵
          </button>
        </form>
      </div>

      {/* Kick/Ban Confirmation Modal */}
      {confirmModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
          <div className="w-full max-w-sm rounded-xl border border-slate-800 bg-[#0d131f] p-4 shadow-2xl space-y-3">
            <div className="flex items-center gap-2 text-slate-100 font-semibold text-sm">
              <AlertTriangle className="size-4 text-amber-400" />
              <span>
                {confirmModal.type === "kick" ? copy.confirmKickTitle : copy.confirmBanTitle}
              </span>
            </div>
            <p className="text-xs text-slate-300">
              {confirmModal.type === "kick"
                ? copy.confirmKickMsg(confirmModal.player.name || "玩家", confirmModal.player.userId)
                : copy.confirmBanMsg(confirmModal.player.name || "玩家", confirmModal.player.userId)}
            </p>
            <div className="flex justify-end gap-2 pt-2">
              <Button
                type="button"
                variant="secondary"
                className="h-8 text-xs"
                onClick={() => setConfirmModal(null)}
              >
                {copy.cancel}
              </Button>
              <Button
                type="button"
                className={cn(
                  "h-8 text-xs text-white",
                  confirmModal.type === "kick" ? "bg-amber-600 hover:bg-amber-500" : "bg-red-600 hover:bg-red-500"
                )}
                disabled={kickMutation.isPending || banMutation.isPending}
                onClick={() => {
                  const target = confirmModal.player.userId || confirmModal.player.name || "";
                  if (confirmModal.type === "kick") {
                    kickMutation.mutate(target);
                  } else {
                    banMutation.mutate(target);
                  }
                }}
              >
                {confirmModal.type === "kick" ? copy.kick : copy.ban}
              </Button>
            </div>
          </div>
        </div>
      )}
    </aside>
  );
}
