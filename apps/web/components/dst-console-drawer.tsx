"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Archive,
  Check,
  ChevronDown,
  Copy,
  CornerDownLeft,
  History,
  Megaphone,
  RotateCcw,
  Save,
  Send,
  Terminal,
  X
} from "lucide-react";
import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Button } from "@/components/ui";
import { createBackup, sendServerCommand } from "@/lib/api";
import { gameServerStatus } from "@/lib/game-server-resource";
import { useI18n } from "@/lib/i18n";
import { providerConfigValue } from "@/lib/provider-config";
import { cn } from "@/lib/utils";
import type { GameServerResource } from "@/lib/types";

type Shard = "master" | "caves";
type CommandKind = "command" | "save" | "backup" | "rollback" | "announce";
type CommandRecord = {
  id: string;
  command: string;
  kind: CommandKind;
  shard: Shard;
  status: "queued" | "completed" | "failed";
  time: string;
};

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
  const inputRef = useRef<HTMLInputElement>(null);

  const [activeTab, setActiveTab] = useState<"actions" | "terminal">("actions");
  const [shard, setShard] = useState<Shard>("master");
  const [command, setCommand] = useState("");
  const [historyIndex, setHistoryIndex] = useState<number | null>(null);
  const [announceText, setAnnounceText] = useState("");
  const [rollbackDepth, setRollbackDepth] = useState(1);
  const [expandedAction, setExpandedAction] = useState<"none" | "rollback" | "announce">("none");
  const [records, setRecords] = useState<CommandRecord[]>([]);
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const cavesEnabled = providerConfigValue(server.spec.config, "caves.enabled") === true;
  const status = gameServerStatus(server);
  const running = status === "running";

  const copy = useMemo(
    () =>
      isZh
        ? {
            title: "控制台",
            tabActions: "常用操作",
            tabTerminal: "终端",
            master: "地上世界",
            caves: "洞穴世界",
            cavesDisabled: "洞穴未启用",
            unavailable: "服务器停止运行",
            save: "保存进度",
            backup: "完整备份",
            rollback: "快照回滚",
            announce: "全服公告",
            snapshotsUnit: "个快照",
            confirmRollback: "确认回滚",
            announcePlaceholder: "输入广播消息内容...",
            sendAnnouncement: "发送",
            recentTitle: "最近操作",
            clearHistory: "清空",
            emptyHistory: "暂无操作记录",
            queued: "已投递",
            completed: "完成",
            failed: "失败",
            commandFailed: "执行失败"
          }
        : {
            title: "Console",
            tabActions: "Actions",
            tabTerminal: "Terminal",
            master: "Master",
            caves: "Caves",
            cavesDisabled: "Caves disabled",
            unavailable: "Server is stopped",
            save: "Save",
            backup: "Backup",
            rollback: "Rollback",
            announce: "Broadcast",
            snapshotsUnit: "snapshots",
            confirmRollback: "Confirm Rollback",
            announcePlaceholder: "Announcement text...",
            sendAnnouncement: "Send",
            recentTitle: "Recent",
            clearHistory: "Clear",
            emptyHistory: "No activity",
            queued: "Queued",
            completed: "Done",
            failed: "Failed",
            commandFailed: "Failed"
          },
    [isZh]
  );

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
        setExpandedAction("none");
      }
      if (kind === "rollback") {
        setExpandedAction("none");
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

  const backupMutation = useMutation({
    mutationFn: () => createBackup(server.id),
    onSuccess: async (backup) => {
      setRecords((current) => [
        {
          id: backup.id,
          command: backup.name,
          kind: "backup",
          shard: "master",
          status: "completed",
          time: new Date().toLocaleTimeString(locale === "zh" ? "zh-CN" : "en-US", {
            hour: "2-digit",
            minute: "2-digit",
            second: "2-digit"
          })
        } as CommandRecord,
        ...current
      ].slice(0, 30));
      await queryClient.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: () => {
      setRecords((current) => [
        {
          id: crypto.randomUUID(),
          command: copy.backup,
          kind: "backup",
          shard: "master",
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
    if (activeTab === "terminal") {
      window.requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [activeTab]);

  if (!open) return null;

  const dispatch = (value: string, kind: CommandKind, target = shard) => {
    const next = value.trim();
    if (!running || !next || mutation.isPending) return;
    mutation.mutate({ value: next, target, kind });
  };

  const handleTerminalSubmit = (event: FormEvent) => {
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

  const copyCommand = (record: CommandRecord) => {
    navigator.clipboard.writeText(record.command);
    setCopiedId(record.id);
    setTimeout(() => setCopiedId(null), 1500);
  };

  const shardLabel = (value: Shard) => (value === "master" ? copy.master : copy.caves);
  const recordScope = (record: CommandRecord) =>
    record.kind === "backup" || record.kind === "rollback" || record.kind === "announce"
      ? isZh ? "完整集群" : "Full cluster"
      : shardLabel(record.shard);

  const quickSnippets = [
    "c_save()",
    "c_countprefabs('world')",
    "c_supergodmode()",
    "c_listallplayers()"
  ];

  return (
    <aside
      aria-label={copy.title}
      className="fixed bottom-0 right-0 top-14 z-40 flex w-full flex-col border-l border-[#1b2434] bg-[#090e17] shadow-[-12px_0_36px_rgba(0,0,0,0.55)] sm:w-[400px]"
      role="complementary"
    >
      {/* Header (Compact 46px) */}
      <header className="flex h-11 shrink-0 items-center justify-between border-b border-[#1b2434] px-3.5 bg-[#0b121e]">
        <div className="flex items-center gap-2 min-w-0">
          <div className="flex size-6 shrink-0 items-center justify-center rounded-md border border-panel-green/30 bg-panel-green/10 text-panel-green">
            <Terminal className="size-3" />
          </div>
          <span className="text-xs font-semibold text-slate-100">{copy.title}</span>
          <span className="text-slate-600">·</span>
          <span className="truncate text-xs text-slate-400">{server.name}</span>
        </div>
        <button
          aria-label={isZh ? "关闭控制台" : "Close console"}
          className="flex size-7 items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white"
          onClick={onClose}
          type="button"
        >
          <X className="size-3.5" />
        </button>
      </header>

      {/* Shard Selector & Tab Row (Compact 38px) */}
      <div className={cn("flex items-center gap-2 border-b border-[#1b2434] bg-[#070b13] px-3.5 py-1.5", activeTab === "terminal" ? "justify-between" : "justify-end")}>
        {/* Shard pills */}
        {activeTab === "terminal" ? <div className="flex items-center rounded-lg border border-slate-800/80 bg-slate-950/80 p-0.5">
          <button
            type="button"
            onClick={() => setShard("master")}
            className={cn(
              "flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium transition",
              shard === "master"
                ? "bg-panel-green/15 text-panel-green font-semibold"
                : "text-slate-400 hover:text-slate-200"
            )}
          >
            <span className={cn("size-1.5 rounded-full", shard === "master" ? "bg-panel-green" : "bg-slate-600")} />
            <span>{copy.master}</span>
          </button>
          <button
            type="button"
            disabled={!cavesEnabled}
            onClick={() => cavesEnabled && setShard("caves")}
            title={!cavesEnabled ? copy.cavesDisabled : undefined}
            className={cn(
              "flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium transition disabled:opacity-30 disabled:cursor-not-allowed",
              shard === "caves"
                ? "bg-panel-green/15 text-panel-green font-semibold"
                : "text-slate-400 hover:text-slate-200"
            )}
          >
            <span className={cn("size-1.5 rounded-full", shard === "caves" ? "bg-panel-green" : "bg-slate-600")} />
            <span>{copy.caves}</span>
          </button>
        </div> : null}

        {/* Mode tabs */}
        <div className="flex items-center rounded-lg border border-slate-800/80 bg-slate-950/80 p-0.5">
          <button
            type="button"
            onClick={() => setActiveTab("actions")}
            className={cn(
              "rounded px-2.5 py-1 text-xs font-medium transition",
              activeTab === "actions"
                ? "bg-slate-800 text-slate-100"
                : "text-slate-400 hover:text-slate-200"
            )}
          >
            {copy.tabActions}
          </button>
          <button
            type="button"
            onClick={() => setActiveTab("terminal")}
            className={cn(
              "rounded px-2.5 py-1 text-xs font-medium transition",
              activeTab === "terminal"
                ? "bg-slate-800 text-slate-100"
                : "text-slate-400 hover:text-slate-200"
            )}
          >
            {copy.tabTerminal}
          </button>
        </div>
      </div>

      {/* Main Area */}
      <div className="flex-1 overflow-y-auto">
        {!running && (
          <div className="m-3 flex items-center gap-2 rounded-lg border border-panel-gold/30 bg-panel-gold/10 px-3 py-2 text-xs text-panel-gold">
            <AlertTriangle className="size-3.5 shrink-0" />
            <span>{copy.unavailable}</span>
          </div>
        )}

        {activeTab === "actions" ? (
          <div className="p-3.5 space-y-2">
            {/* Quick Actions 2x2 Grid */}
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                disabled={!running || mutation.isPending}
                onClick={() => dispatch("c_save()", "save", "master")}
                className="flex items-center gap-2.5 rounded-lg border border-slate-800 bg-slate-900/60 px-3 py-2.5 text-left transition hover:border-panel-green/40 hover:bg-slate-800/70 disabled:opacity-40"
              >
                <div className="flex size-7 shrink-0 items-center justify-center rounded-md bg-panel-green/10 text-panel-green">
                  <Save className="size-3.5" />
                </div>
                <div className="min-w-0">
                  <span className="block text-xs font-medium text-slate-200 truncate">{copy.save}</span>
                  <span className="block font-mono text-[10px] text-slate-500">c_save()</span>
                </div>
              </button>

              <button
                type="button"
                disabled={backupMutation.isPending}
                onClick={() => backupMutation.mutate()}
                className="flex items-center gap-2.5 rounded-lg border border-slate-800 bg-slate-900/60 px-3 py-2.5 text-left transition hover:border-indigo-500/40 hover:bg-slate-800/70 disabled:opacity-40"
              >
                <div className="flex size-7 shrink-0 items-center justify-center rounded-md bg-indigo-500/10 text-indigo-400">
                  <Archive className="size-3.5" />
                </div>
                <div className="min-w-0">
                  <span className="block text-xs font-medium text-slate-200 truncate">{copy.backup}</span>
                  <span className="block font-mono text-[10px] text-slate-500">tar.gz</span>
                </div>
              </button>
            </div>

            {/* Rollback Collapsible */}
            <div className="rounded-lg border border-slate-800 bg-slate-900/50 overflow-hidden">
              <button
                type="button"
                onClick={() => setExpandedAction(expandedAction === "rollback" ? "none" : "rollback")}
                className="flex w-full items-center justify-between px-3 py-2 text-left hover:bg-slate-800/70 transition"
              >
                <div className="flex items-center gap-2.5">
                  <div className="flex size-7 shrink-0 items-center justify-center rounded-md bg-panel-gold/10 text-panel-gold">
                    <RotateCcw className="size-3.5" />
                  </div>
                  <span className="text-xs font-medium text-slate-200">{copy.rollback}</span>
                </div>
                <ChevronDown
                  className={cn(
                    "size-3.5 text-slate-500 transition-transform",
                    expandedAction === "rollback" && "rotate-180 text-panel-gold"
                  )}
                />
              </button>

              {expandedAction === "rollback" && (
                <div className="border-t border-slate-800 bg-slate-950/80 p-2.5 space-y-2">
                  <div className="grid grid-cols-4 gap-1.5">
                    {[1, 2, 3, 5].map((count) => (
                      <button
                        key={count}
                        type="button"
                        onClick={() => setRollbackDepth(count)}
                        className={cn(
                          "rounded border py-1 text-xs font-medium transition text-center",
                          rollbackDepth === count
                            ? "border-panel-gold bg-panel-gold/20 text-panel-gold font-bold"
                            : "border-slate-800 bg-slate-900 text-slate-400 hover:text-slate-200"
                        )}
                      >
                        {count} {copy.snapshotsUnit}
                      </button>
                    ))}
                  </div>

                  <Button
                    type="button"
                    variant="gold"
                    className="w-full h-7 text-xs font-medium"
                    disabled={!running || mutation.isPending}
                    onClick={() => dispatch(`c_rollback(${rollbackDepth})`, "rollback", "master")}
                  >
                    {copy.confirmRollback}
                    {isZh
                      ? `（${rollbackDepth} ${copy.snapshotsUnit}）`
                      : ` (${rollbackDepth} ${copy.snapshotsUnit})`}
                  </Button>
                </div>
              )}
            </div>

            {/* Announcement Collapsible */}
            <div className="rounded-lg border border-slate-800 bg-slate-900/50 overflow-hidden">
              <button
                type="button"
                onClick={() => setExpandedAction(expandedAction === "announce" ? "none" : "announce")}
                className="flex w-full items-center justify-between px-3 py-2 text-left hover:bg-slate-800/70 transition"
              >
                <div className="flex items-center gap-2.5">
                  <div className="flex size-7 shrink-0 items-center justify-center rounded-md bg-sky-500/10 text-sky-400">
                    <Megaphone className="size-3.5" />
                  </div>
                  <span className="text-xs font-medium text-slate-200">{copy.announce}</span>
                </div>
                <ChevronDown
                  className={cn(
                    "size-3.5 text-slate-500 transition-transform",
                    expandedAction === "announce" && "rotate-180 text-sky-400"
                  )}
                />
              </button>

              {expandedAction === "announce" && (
                <form
                  onSubmit={(event) => {
                    event.preventDefault();
                    dispatch(`TheNet:SystemMessage(${JSON.stringify(announceText.trim())})`, "announce", "master");
                  }}
                  className="border-t border-slate-800 bg-slate-950/80 p-2.5 flex items-center gap-1.5"
                >
                  <input
                    className="h-8 flex-1 rounded-md border border-slate-800 bg-slate-950 px-2.5 text-xs text-slate-100 outline-none placeholder:text-slate-600 focus:border-sky-500"
                    onChange={(event) => setAnnounceText(event.target.value)}
                    placeholder={copy.announcePlaceholder}
                    value={announceText}
                    autoFocus
                  />
                  <Button
                    className="h-8 px-3 text-xs font-medium bg-sky-500 hover:bg-sky-400 text-slate-950 shrink-0"
                    disabled={!running || !announceText.trim() || mutation.isPending}
                    type="submit"
                  >
                    <Send className="size-3" />
                    <span>{copy.sendAnnouncement}</span>
                  </Button>
                </form>
              )}
            </div>

            {(mutation.error instanceof Error || backupMutation.error instanceof Error) && (
              <div className="rounded border border-red-500/30 bg-red-500/10 p-2 text-xs text-red-300">
                {copy.commandFailed}: {(mutation.error instanceof Error ? mutation.error : backupMutation.error)?.message}
              </div>
            )}
          </div>
        ) : (
          /* Terminal Tab (Compact) */
          <div className="p-3.5 space-y-2.5">
            <div className="flex flex-wrap gap-1">
              {quickSnippets.map((snippet) => (
                <button
                  key={snippet}
                  type="button"
                  onClick={() => {
                    setCommand(snippet);
                    inputRef.current?.focus();
                  }}
                  className="rounded border border-slate-800 bg-slate-900 px-2 py-0.5 font-mono text-[11px] text-slate-300 hover:border-panel-green/40 hover:text-white transition"
                >
                  {snippet}
                </button>
              ))}
            </div>

            <form onSubmit={handleTerminalSubmit} className="flex items-center gap-1.5 rounded-lg border border-slate-800 bg-slate-950 px-2.5 py-1 focus-within:border-panel-green/60">
              <span className="font-mono text-xs text-panel-green font-bold select-none">›</span>
              <input
                ref={inputRef}
                className="h-7 min-w-0 flex-1 bg-transparent font-mono text-xs text-slate-100 outline-none placeholder:text-slate-600"
                disabled={!running || mutation.isPending}
                onChange={(event) => setCommand(event.target.value.replace(/[\r\n]/g, ""))}
                onKeyDown={handleKeyDown}
                placeholder="c_save(), c_countprefabs()..."
                value={command}
              />
              <Button
                type="submit"
                className="size-7 shrink-0 p-0"
                disabled={!running || !command.trim() || mutation.isPending}
              >
                <CornerDownLeft className="size-3" />
              </Button>
            </form>

            {mutation.error instanceof Error && (
              <div className="rounded border border-red-500/30 bg-red-500/10 p-2 text-xs text-red-300">
                {copy.commandFailed}: {mutation.error.message}
              </div>
            )}
          </div>
        )}

        {/* History Stream (Compact Single-line items) */}
        <div className="border-t border-[#1b2434] px-3.5 py-2.5 bg-[#080d16]">
          <div className="mb-2 flex items-center justify-between">
            <div className="flex items-center gap-1.5 text-xs font-medium text-slate-400">
              <History aria-hidden="true" className="size-3 text-panel-green" />
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
            <div className="py-4 text-center text-xs text-slate-600">
              {copy.emptyHistory}
            </div>
          ) : (
            <div className="space-y-1 max-h-48 overflow-y-auto">
              {records.map((record) => (
                <div
                  key={record.id}
                  className="group flex items-center justify-between gap-2 rounded px-2 py-1 text-xs hover:bg-slate-900/70 transition"
                >
                  <div className="flex items-center gap-1.5 min-w-0">
                    <span className="font-mono text-[10px] text-slate-500 shrink-0">{record.time}</span>
                    <span
                      className={cn(
                        "rounded px-1 text-[9px] font-mono shrink-0",
                        record.kind === "backup"
                          ? "bg-indigo-500/10 text-indigo-400"
                          : record.kind === "rollback"
                            ? "bg-amber-500/10 text-amber-400"
                            : record.kind === "announce"
                              ? "bg-sky-500/10 text-sky-400"
                              : record.shard === "master"
                                ? "bg-panel-green/10 text-panel-green"
                                : "bg-amber-500/10 text-amber-400"
                      )}
                    >
                      {recordScope(record)}
                    </span>
                    <code className="truncate font-mono text-[11px] text-slate-300" title={record.command}>
                      {record.command}
                    </code>
                  </div>

                  <div className="flex items-center gap-1.5 shrink-0">
                    <span
                      className={cn(
                        "text-[10px] font-medium",
                        record.status === "failed" ? "text-red-400" : "text-panel-green"
                      )}
                    >
                      {record.status === "queued"
                        ? copy.queued
                        : record.status === "completed"
                          ? copy.completed
                          : copy.failed}
                    </span>
                    <button
                      type="button"
                      onClick={() => copyCommand(record)}
                      aria-label={isZh ? "复制指令" : "Copy command"}
                      className="opacity-0 group-hover:opacity-100 p-0.5 text-slate-400 hover:text-white transition"
                    >
                      {copiedId === record.id ? (
                        <Check className="size-2.5 text-panel-green" />
                      ) : (
                        <Copy className="size-2.5" />
                      )}
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </aside>
  );
}
