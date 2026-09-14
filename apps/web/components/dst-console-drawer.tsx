"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Archive, Check, ChevronLeft, ChevronRight, History, Megaphone, RotateCcw, Save, Send, Terminal, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from "react";
import { Button } from "@/components/ui";
import { createBackup, sendServerCommand } from "@/lib/api";
import { gameServerStatus } from "@/lib/game-server-resource";
import { useI18n } from "@/lib/i18n";
import { providerConfigValue } from "@/lib/provider-config";
import { cn } from "@/lib/utils";
import type { GameServerResource } from "@/lib/types";

type Shard = "master" | "caves";
type CommandKind = "command" | "save" | "backup" | "rollback" | "announce";
type CommandRecord = { id: string; command: string; kind: CommandKind; shard: Shard; status: "queued" | "completed" | "failed"; time: string };

export function DSTConsoleDrawer({ open, server, onClose }: { open: boolean; server: GameServerResource; onClose: () => void }) {
  const { locale } = useI18n();
  const queryClient = useQueryClient();
  const isZh = locale.startsWith("zh");
  const inputRef = useRef<HTMLInputElement>(null);
  const [shard, setShard] = useState<Shard>("master");
  const [command, setCommand] = useState("");
  const [announce, setAnnounce] = useState("");
  const [rollbackDepth, setRollbackDepth] = useState(1);
  const [mode, setMode] = useState<"idle" | "announce" | "rollback">("idle");
  const [view, setView] = useState<"operations" | "advanced">("operations");
  const [records, setRecords] = useState<CommandRecord[]>([]);
  const cavesEnabled = providerConfigValue(server.spec.config, "caves.enabled") === true;
  const running = gameServerStatus(server) === "running";

  const copy = useMemo(() => isZh ? {
    title: "服务器控制台", context: "DST 分片指令", master: "地上", caves: "洞穴", masterHint: "Master", cavesHint: "Caves",
    target: "执行目标", unavailable: "服务器停止时无法执行指令", actions: "常用操作", save: "保存进度", saveHint: "让游戏立即写入内部存档",
    backup: "创建完整备份", backupHint: "归档地上与洞穴的当前存档",
    rollback: "回滚存档", rollbackHint: "按游戏快照回退世界进度", announce: "发送公告", announceHint: "向在线玩家广播一条消息",
    advanced: "高级指令", advancedHint: "Lua 指令将原样发送到当前分片", placeholder: "输入 Lua 控制台指令", execute: "执行指令",
    recent: "最近操作", empty: "本次打开控制台后尚无操作", queued: "已投递", completed: "已创建", failed: "失败", depth: "回滚快照数", message: "公告内容", cluster: "完整集群",
    reviewRollback: "回滚会中断当前世界进度，请确认快照数。", confirmRollback: "确认回滚", sendAnnouncement: "发送公告",
    advancedView: "高级指令", back: "返回常用操作", close: "关闭控制台", commandFailed: "指令投递失败"
  } : {
    title: "Server console", context: "DST shard commands", master: "Surface", caves: "Caves", masterHint: "Master", cavesHint: "Caves",
    target: "Command target", unavailable: "Start the server before sending commands", actions: "Common operations", save: "Save progress", saveHint: "Ask the game to write its internal save now",
    backup: "Create full backup", backupHint: "Archive the current surface and caves saves",
    rollback: "Roll back", rollbackHint: "Return the world to an earlier game snapshot", announce: "Broadcast", announceHint: "Send a message to online players",
    advanced: "Advanced command", advancedHint: "Lua is sent literally to the selected shard", placeholder: "Enter a Lua console command", execute: "Run command",
    recent: "Recent operations", empty: "No operations since opening the console", queued: "Queued", completed: "Created", failed: "Failed", depth: "Snapshots to roll back", message: "Announcement text", cluster: "Full cluster",
    reviewRollback: "Rollback interrupts current world progress. Check the snapshot count.", confirmRollback: "Confirm rollback", sendAnnouncement: "Send broadcast",
    advancedView: "Advanced command", back: "Back to operations", close: "Close console", commandFailed: "Command dispatch failed"
  }, [isZh]);

  const mutation = useMutation({
    mutationFn: ({ value, target, kind }: { value: string; target: Shard; kind: CommandKind }) => sendServerCommand(server.id, value, target).then((result) => ({ result, value, target, kind })),
    onSuccess: ({ result, value, target, kind }) => {
      setRecords((current) => [{ id: result.id, command: value, kind, shard: target, status: "queued", time: new Date().toLocaleTimeString(locale === "zh" ? "zh-CN" : "en-US", { hour: "2-digit", minute: "2-digit" }) } as CommandRecord, ...current].slice(0, 8));
      setCommand("");
      setAnnounce("");
      setMode("idle");
    },
    onError: (_error, variables) => {
      setRecords((current) => [{ id: crypto.randomUUID(), command: variables.value, kind: variables.kind, shard: variables.target, status: "failed", time: new Date().toLocaleTimeString(locale === "zh" ? "zh-CN" : "en-US", { hour: "2-digit", minute: "2-digit" }) } as CommandRecord, ...current].slice(0, 8));
    }
  });

  const backupMutation = useMutation({
    mutationFn: () => createBackup(server.id),
    onSuccess: async (backup) => {
      setRecords((current) => [{ id: backup.id, command: backup.name, kind: "backup", shard: "master", status: "completed", time: new Date().toLocaleTimeString(locale === "zh" ? "zh-CN" : "en-US", { hour: "2-digit", minute: "2-digit" }) } as CommandRecord, ...current].slice(0, 8));
      await queryClient.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: () => {
      setRecords((current) => [{ id: crypto.randomUUID(), command: copy.backup, kind: "backup", shard: "master", status: "failed", time: new Date().toLocaleTimeString(locale === "zh" ? "zh-CN" : "en-US", { hour: "2-digit", minute: "2-digit" }) } as CommandRecord, ...current].slice(0, 8));
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
    if (view === "advanced") window.requestAnimationFrame(() => inputRef.current?.focus());
  }, [view]);

  if (!open) return null;

  const dispatch = (value: string, kind: CommandKind, target = shard) => {
    const next = value.trim();
    if (!running || !next || mutation.isPending) return;
    mutation.mutate({ value: next, target, kind });
  };
  const submitCommand = (event: FormEvent) => {
    event.preventDefault();
    dispatch(command, "command");
  };
  const shardLabel = (value: Shard) => value === "master" ? copy.master : copy.caves;
  const recordLabel = (record: CommandRecord) => record.kind === "command" ? copy.advanced : record.kind === "save" ? copy.save : record.kind === "backup" ? copy.backup : record.kind === "rollback" ? copy.rollback : copy.announce;
  const history = (
    <section className="px-5 py-4">
      <div className="mb-2.5 flex items-center gap-2 text-xs font-medium text-slate-400"><History aria-hidden="true" className="size-3.5" />{copy.recent}</div>
      {records.length === 0 ? <p className="rounded-lg border border-dashed border-panel-line px-3 py-5 text-center text-xs text-slate-600">{copy.empty}</p> : <ol className="space-y-1">{records.map((record) => <li className="flex items-start gap-2 rounded-md px-2 py-2 text-xs hover:bg-slate-900/60" key={record.id}><span className="w-10 shrink-0 font-mono text-slate-600">{record.time}</span><span className="min-w-0 flex-1"><span className="mb-0.5 block text-[11px] text-slate-500">{record.kind === "backup" ? (isZh ? "完整集群" : "Full cluster") : shardLabel(record.shard)} · {recordLabel(record)}</span><code className="block truncate text-slate-300" title={record.command}>{record.command}</code></span><span className={record.status === "failed" ? "text-red-300" : "text-panel-green"}>{record.status === "queued" ? copy.queued : record.status === "completed" ? copy.completed : copy.failed}</span></li>)}</ol>}
    </section>
  );

  return (
    <aside aria-label={copy.title} className="fixed bottom-0 right-0 top-14 z-40 flex w-full flex-col border-l border-panel-line bg-[#0a0f18] shadow-[-8px_0_24px_rgba(0,0,0,0.28)] sm:w-[420px]" role="complementary">
      <header className="flex h-[68px] shrink-0 items-center justify-between border-b border-panel-line px-5">
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-panel-green/25 bg-panel-green/10 text-panel-green"><Terminal aria-hidden="true" className="size-4" /></span>
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold text-slate-100">{copy.title}</h2>
            <p className="mt-0.5 truncate text-xs text-slate-500">{server.name} · {copy.context}</p>
          </div>
        </div>
        <button aria-label={copy.close} className="flex size-8 items-center justify-center rounded-md text-slate-500 transition hover:bg-slate-800 hover:text-slate-100 focus:outline-none focus:ring-2 focus:ring-panel-green/50" onClick={onClose} type="button"><X aria-hidden="true" className="size-4" /></button>
      </header>

      <div className="flex-1 overflow-y-auto">
        {view === "operations" ? <>
          <section className="border-b border-panel-line px-5 py-4">
            {mode === "idle" ? <>
              <div className="mb-2 flex items-center justify-between"><p className="text-xs font-medium text-slate-400">{copy.actions}</p><button className="inline-flex items-center gap-1 text-xs text-slate-500 transition hover:text-slate-200" onClick={() => setView("advanced")} type="button">{copy.advancedView}<ChevronRight className="size-3.5" /></button></div>
              <div className="divide-y divide-panel-line rounded-lg border border-panel-line bg-slate-950/35">
                <ConsoleAction icon={<Save className="size-4" />} label={copy.save} hint={copy.saveHint} meta={copy.master} disabled={!running || mutation.isPending} onClick={() => dispatch("c_save()", "save", "master")} />
                <ConsoleAction icon={<Archive className="size-4" />} label={copy.backup} hint={copy.backupHint} meta={copy.cluster} disabled={backupMutation.isPending} onClick={() => backupMutation.mutate()} />
                <ConsoleAction icon={<RotateCcw className="size-4" />} label={copy.rollback} hint={copy.rollbackHint} meta={copy.cluster} disabled={!running || mutation.isPending} tone="warning" onClick={() => setMode("rollback")} />
                <ConsoleAction icon={<Megaphone className="size-4" />} label={copy.announce} hint={copy.announceHint} meta={copy.cluster} disabled={!running || mutation.isPending} onClick={() => setMode("announce")} />
              </div>
            </> : mode === "rollback" ? <div>
              <button className="mb-4 inline-flex items-center gap-1.5 text-xs text-slate-400 hover:text-slate-100" onClick={() => setMode("idle")} type="button"><ChevronLeft className="size-3.5" />{copy.back}</button>
              <div className="flex items-start gap-3 rounded-lg border border-panel-gold/25 bg-panel-gold/10 p-3"><AlertTriangle className="mt-0.5 size-4 shrink-0 text-panel-gold" /><div><p className="text-sm font-medium text-panel-gold">{copy.rollback}</p><p className="mt-1 text-xs leading-5 text-slate-400">{copy.reviewRollback}</p></div></div>
              <label className="mt-4 block text-xs text-slate-400">{copy.depth}<input className="mt-1.5 h-10 w-full rounded-md border border-panel-line bg-slate-950 px-3 font-mono text-sm text-slate-100 outline-none focus:border-panel-gold" max={5} min={1} onChange={(event) => setRollbackDepth(Number(event.target.value))} type="number" value={rollbackDepth} /></label>
              <Button className="mt-3 w-full" disabled={mutation.isPending} onClick={() => dispatch(`c_rollback(${rollbackDepth})`, "rollback", "master")} type="button" variant="gold">{copy.confirmRollback}</Button>
            </div> : <div>
              <button className="mb-4 inline-flex items-center gap-1.5 text-xs text-slate-400 hover:text-slate-100" onClick={() => setMode("idle")} type="button"><ChevronLeft className="size-3.5" />{copy.back}</button>
              <p className="text-sm font-medium text-slate-100">{copy.announce}</p><p className="mt-1 text-xs text-slate-500">{copy.announceHint}</p>
              <form className="mt-4" onSubmit={(event) => { event.preventDefault(); dispatch(`TheNet:SystemMessage(${JSON.stringify(announce.trim())})`, "announce", "master"); }}><input aria-label={copy.message} autoFocus className="h-10 w-full rounded-md border border-panel-line bg-slate-950 px-3 text-sm text-slate-100 outline-none placeholder:text-slate-400 focus:border-panel-green" onChange={(event) => setAnnounce(event.target.value)} placeholder={copy.message} value={announce} /><Button className="mt-3 w-full" disabled={!announce.trim() || mutation.isPending} type="submit">{copy.sendAnnouncement}</Button></form>
            </div>}
            {(mutation.error instanceof Error || backupMutation.error instanceof Error) && <p className="mt-3 text-xs text-red-300">{copy.commandFailed}: {(mutation.error instanceof Error ? mutation.error : backupMutation.error)?.message}</p>}
          </section>
          {history}
        </> : <>
          <section className="border-b border-panel-line px-5 py-5">
            <button className="mb-4 inline-flex items-center gap-1.5 text-xs text-slate-400 hover:text-slate-100" onClick={() => setView("operations")} type="button"><ChevronLeft className="size-3.5" />{copy.back}</button>
            <div className="mb-2.5 flex items-center justify-between"><p className="text-xs font-medium text-slate-400">{copy.target}</p><span className={cn("inline-flex items-center gap-1.5 text-xs", running ? "text-panel-green" : "text-slate-500")}><span className={cn("size-1.5 rounded-full", running ? "bg-panel-green" : "bg-slate-600")} />{running ? (isZh ? "可执行" : "Ready") : (isZh ? "已停止" : "Stopped")}</span></div>
            <div className="grid grid-cols-2 gap-2" role="radiogroup" aria-label={copy.target}>{(["master", "caves"] as const).map((value) => { const disabled = value === "caves" && !cavesEnabled; return <button key={value} aria-checked={shard === value} disabled={disabled} onClick={() => setShard(value)} role="radio" type="button" className={cn("flex min-w-0 items-center justify-between rounded-lg border px-3 py-2.5 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-green/50 disabled:cursor-not-allowed disabled:opacity-35", shard === value ? "border-panel-green/45 bg-panel-green/10 text-slate-100" : "border-panel-line bg-slate-950/50 text-slate-400 hover:border-slate-700 hover:text-slate-200")}><span><span className="block text-sm font-medium">{shardLabel(value)}</span><span className="mt-0.5 block font-mono text-[11px] text-slate-500">{value === "master" ? copy.masterHint : copy.cavesHint}</span></span>{shard === value && <Check aria-hidden="true" className="size-4 shrink-0 text-panel-green" />}</button>; })}</div>
            {!running && <p className="mt-2.5 text-xs text-panel-gold">{copy.unavailable}</p>}
            <p className="mt-5 text-sm font-medium text-slate-100">{copy.advanced}</p><p className="mt-1 text-xs leading-5 text-slate-500">{copy.advancedHint}</p>
            <form className="mt-4 flex items-center gap-2" onSubmit={submitCommand}><span className="font-mono text-sm text-panel-green">›</span><input ref={inputRef} aria-label={copy.placeholder} className="h-10 min-w-0 flex-1 rounded-md border border-panel-line bg-slate-950 px-3 font-mono text-sm text-slate-100 outline-none placeholder:text-slate-400 focus:border-panel-green" disabled={!running || mutation.isPending} onChange={(event) => setCommand(event.target.value.replace(/[\r\n]/g, ""))} placeholder={copy.placeholder} value={command} /><Button aria-label={copy.execute} className="size-10 shrink-0 px-0" disabled={!running || !command.trim() || mutation.isPending} type="submit"><Send aria-hidden="true" className="size-4" /></Button></form>
            {mutation.error instanceof Error && <p className="mt-2 text-xs text-red-300">{copy.commandFailed}: {mutation.error.message}</p>}
          </section>
          {history}
        </>}
      </div>
    </aside>
  );
}

function ConsoleAction({ disabled, hint, icon, label, meta, onClick, tone = "default" }: { disabled: boolean; hint: string; icon: ReactNode; label: string; meta: string; onClick: () => void; tone?: "default" | "warning" }) {
  return <button className="group flex w-full items-center gap-3 px-3 py-3 text-left transition hover:bg-slate-900/65 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-panel-green/50 disabled:cursor-not-allowed disabled:opacity-45" disabled={disabled} onClick={onClick} type="button"><span className={cn("flex size-8 shrink-0 items-center justify-center rounded-md border", tone === "warning" ? "border-panel-gold/25 bg-panel-gold/10 text-panel-gold" : "border-slate-700 bg-slate-900 text-slate-300 group-hover:text-panel-green")}>{icon}</span><span className="min-w-0 flex-1"><span className="flex items-center gap-2"><span className="block text-sm font-medium text-slate-200">{label}</span><span className="rounded bg-slate-800 px-1.5 py-0.5 text-[10px] text-slate-500">{meta}</span></span><span className="mt-0.5 block truncate text-xs text-slate-500">{hint}</span></span><ChevronRight aria-hidden="true" className="size-4 shrink-0 text-slate-600" /></button>;
}
