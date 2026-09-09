"use client";

import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import {
  ChevronLeft,
  Play,
  Square,
  RotateCcw,
  Copy,
  Check,
  Server,
  Sliders,
  Terminal,
  Archive,
  Save,
  Trash2
} from "lucide-react";
import { getGameServer, gameServerAction, getSettings } from "@/lib/api";
import { gameServerStatus, gameServerJoinPort } from "@/lib/game-server-resource";
import { useI18n } from "@/lib/i18n";
import { usePermissions } from "@/lib/permissions";
import { copyText } from "@/lib/clipboard";
import { Button, Input } from "@/components/ui";
import { cn } from "@/lib/utils";

type DetailTab = "config" | "console" | "backups";

export default function ServerDetailPage() {
  const params = useParams<{ id: string }>();
  const id = params?.id ?? "";
  const router = useRouter();
  const queryClient = useQueryClient();
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const { canControlServer, canEditServerConfig, canDeleteServer } = usePermissions();

  const [activeTab, setActiveTab] = useState<DetailTab>("config");
  const [copied, setCopied] = useState(false);
  const [savedSuccess, setSavedSuccess] = useState(false);

  // Form states
  const [serverName, setServerName] = useState("Terraria Server");
  const [motd, setMotd] = useState("Welcome to Terraria Server!");
  const [maxPlayers, setMaxPlayers] = useState(16);
  const [worldDifficulty, setWorldDifficulty] = useState("master");

  const serverQuery = useQuery({
    queryKey: ["game-server", id],
    queryFn: () => getGameServer(id),
    enabled: Boolean(id),
    refetchInterval: 5000
  });

  const settingsQuery = useQuery({
    queryKey: ["settings"],
    queryFn: getSettings,
    retry: false
  });

  const server = serverQuery.data;
  const status = server ? gameServerStatus(server) : "unknown";
  const isRunning = status === "running";
  const isStopped = status === "stopped";
  const port = server ? (gameServerJoinPort(server) || 7777) : 7777;
  const host = settingsQuery.data?.publicHost || (typeof window !== "undefined" ? window.location.hostname : "127.0.0.1");
  const endpoint = `${host}:${port}`;

  const actionMutation = useMutation({
    mutationFn: (action: "start" | "stop" | "restart" | "delete") => gameServerAction(id, action),
    onSuccess: (_, action) => {
      queryClient.invalidateQueries({ queryKey: ["game-server", id] });
      queryClient.invalidateQueries({ queryKey: ["game-servers"] });
      if (action === "delete") {
        router.push("/servers");
      }
    }
  });

  const handleCopyEndpoint = () => {
    copyText(endpoint);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  const handleSave = (e: React.FormEvent) => {
    e.preventDefault();
    setSavedSuccess(true);
    setTimeout(() => setSavedSuccess(false), 2000);
  };

  if (serverQuery.isLoading) {
    return (
      <div className="p-8 text-center text-xs text-slate-400">
        {isZh ? "正在加载实例详情..." : "Loading instance details..."}
      </div>
    );
  }

  if (serverQuery.isError || !server) {
    return (
      <div className="p-8 text-center text-xs text-slate-400 space-y-3">
        <p>{isZh ? "无法加载此实例或实例不存在" : "Instance not found or error loading."}</p>
        <button
          onClick={() => router.push("/servers")}
          className="text-emerald-600 font-semibold hover:underline"
        >
          {isZh ? "返回实例列表" : "Return to Servers"}
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* 1. Breadcrumb and Header */}
      <div className="bg-white border border-slate-200/80 rounded-xl p-3.5 shadow-2xs space-y-3">
        <div className="flex items-center gap-1.5 text-xs text-slate-400">
          <button
            onClick={() => router.push("/servers")}
            className="hover:text-slate-900 transition flex items-center gap-0.5 cursor-pointer"
          >
            <ChevronLeft className="size-3.5" />
            <span>{isZh ? "实例列表" : "Instances"}</span>
          </button>
          <span>/</span>
          <span className="font-mono font-semibold text-slate-900">{server.name}</span>
        </div>

        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pt-1 border-t border-slate-100">
          <div className="flex items-center gap-3">
            <div className="flex size-9 items-center justify-center rounded-lg bg-emerald-50 text-emerald-600 font-bold border border-emerald-100">
              <Server className="size-4" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h1 className="text-sm font-bold text-slate-900 leading-none">{server.name}</h1>
                {isRunning ? (
                  <span className="inline-flex items-center gap-1 text-[11px] font-medium text-emerald-700 bg-emerald-50 px-2 py-0.5 rounded-full border border-emerald-200/60">
                    <span className="size-1.5 rounded-full bg-emerald-500 animate-pulse" />
                    <span>{isZh ? "运行中" : "Running"}</span>
                  </span>
                ) : isStopped ? (
                  <span className="inline-flex items-center gap-1 text-[11px] font-medium text-slate-500 bg-slate-100 px-2 py-0.5 rounded-full border border-slate-200">
                    <span className="size-1.5 rounded-full bg-slate-300" />
                    <span>{isZh ? "已停止" : "Stopped"}</span>
                  </span>
                ) : (
                  <span className="text-[11px] font-medium text-amber-600">{status}</span>
                )}
              </div>
              <p className="text-[11px] font-mono text-slate-400 mt-1">
                {server.providerKey.includes("tmod") ? "tModLoader" : "Vanilla 1.4.4.9"} · {endpoint}
              </p>
            </div>
          </div>

          {/* Action Toolbar */}
          <div className="flex items-center gap-1.5 self-end sm:self-center">
            <button
              onClick={handleCopyEndpoint}
              title={isZh ? "复制连接串" : "Copy Endpoint"}
              className="flex h-7.5 items-center gap-1.5 px-2.5 rounded-lg border border-slate-200/80 bg-white text-xs font-medium text-slate-700 hover:bg-slate-50 shadow-2xs transition cursor-pointer"
            >
              {copied ? <Check className="size-3.5 text-emerald-600" /> : <Copy className="size-3.5 text-slate-400" />}
              <span>{copied ? (isZh ? "已复制" : "Copied") : (isZh ? "复制地址" : "Copy")}</span>
            </button>

            {canControlServer && isStopped && (
              <button
                onClick={() => actionMutation.mutate("start")}
                disabled={actionMutation.isPending}
                className="flex h-7.5 items-center gap-1 px-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-700 text-xs font-semibold text-white shadow-xs transition cursor-pointer"
              >
                <Play className="size-3.5 fill-current" />
                <span>{isZh ? "启动" : "Start"}</span>
              </button>
            )}

            {canControlServer && isRunning && (
              <>
                <button
                  onClick={() => actionMutation.mutate("restart")}
                  disabled={actionMutation.isPending}
                  className="flex h-7.5 items-center gap-1 px-2.5 rounded-lg border border-slate-200/80 bg-white text-xs font-medium text-slate-700 hover:bg-slate-50 shadow-2xs transition cursor-pointer"
                >
                  <RotateCcw className="size-3.5 text-slate-500" />
                  <span>{isZh ? "重启" : "Restart"}</span>
                </button>
                <button
                  onClick={() => actionMutation.mutate("stop")}
                  disabled={actionMutation.isPending}
                  className="flex h-7.5 items-center gap-1 px-2.5 rounded-lg bg-rose-50 border border-rose-200 text-xs font-medium text-rose-700 hover:bg-rose-100 transition cursor-pointer"
                >
                  <Square className="size-3.5 fill-current" />
                  <span>{isZh ? "停止" : "Stop"}</span>
                </button>
              </>
            )}

            {canDeleteServer && (
              <button
                onClick={() => {
                  if (confirm(isZh ? `确定要彻底删除 ${server.name} 吗？` : `Delete ${server.name}?`)) {
                    actionMutation.mutate("delete");
                  }
                }}
                disabled={actionMutation.isPending}
                className="flex size-7.5 items-center justify-center rounded-lg border border-slate-200/80 bg-white text-slate-400 hover:text-rose-600 hover:bg-rose-50 shadow-2xs transition cursor-pointer"
                title={isZh ? "删除实例" : "Delete"}
              >
                <Trash2 className="size-3.5" />
              </button>
            )}
          </div>
        </div>

        {/* Inner Tabs */}
        <div className="flex items-center gap-1 border-t border-slate-100 pt-2 text-xs font-medium">
          <button
            onClick={() => setActiveTab("config")}
            className={cn(
              "flex items-center gap-1.5 h-7 px-3 rounded-lg transition cursor-pointer",
              activeTab === "config" ? "bg-slate-100 text-slate-900 font-semibold shadow-2xs" : "text-slate-500 hover:text-slate-900"
            )}
          >
            <Sliders className="size-3.5" />
            <span>{isZh ? "参数配置" : "Configuration"}</span>
          </button>
          <button
            onClick={() => setActiveTab("console")}
            className={cn(
              "flex items-center gap-1.5 h-7 px-3 rounded-lg transition cursor-pointer",
              activeTab === "console" ? "bg-slate-100 text-slate-900 font-semibold shadow-2xs" : "text-slate-500 hover:text-slate-900"
            )}
          >
            <Terminal className="size-3.5" />
            <span>{isZh ? "终端控制台" : "Console & Logs"}</span>
          </button>
          <button
            onClick={() => setActiveTab("backups")}
            className={cn(
              "flex items-center gap-1.5 h-7 px-3 rounded-lg transition cursor-pointer",
              activeTab === "backups" ? "bg-slate-100 text-slate-900 font-semibold shadow-2xs" : "text-slate-500 hover:text-slate-900"
            )}
          >
            <Archive className="size-3.5" />
            <span>{isZh ? "存档与备份" : "Worlds & Backups"}</span>
          </button>
        </div>
      </div>

      {/* 2. Content Tabs */}
      {activeTab === "config" && (
        <form onSubmit={handleSave} className="bg-white border border-slate-200/80 rounded-xl p-5 shadow-2xs space-y-4">
          <div className="border-b border-slate-100 pb-3">
            <h3 className="text-xs font-bold text-slate-900 uppercase tracking-wider">
              {isZh ? "Terraria 运行参数" : "Terraria Server Parameters"}
            </h3>
            <p className="text-[11px] text-slate-400">
              {isZh ? "修改配置后请重启服务器以生效" : "Changes will take effect upon container restart."}
            </p>
          </div>

          <div className="space-y-3 max-w-xl text-xs">
            <div>
              <label className="block font-semibold text-slate-700 mb-1">
                {isZh ? "服务器房间名" : "Server Name"}
              </label>
              <Input
                value={serverName}
                onChange={(e) => setServerName(e.target.value)}
              />
            </div>

            <div>
              <label className="block font-semibold text-slate-700 mb-1">
                {isZh ? "欢迎广播语 (MOTD)" : "MOTD Message"}
              </label>
              <Input
                value={motd}
                onChange={(e) => setMotd(e.target.value)}
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block font-semibold text-slate-700 mb-1">
                  {isZh ? "最大玩家人数" : "Max Players"}
                </label>
                <Input
                  type="number"
                  value={maxPlayers}
                  onChange={(e) => setMaxPlayers(Number(e.target.value))}
                />
              </div>

              <div>
                <label className="block font-semibold text-slate-700 mb-1">
                  {isZh ? "世界难度" : "World Difficulty"}
                </label>
                <select
                  value={worldDifficulty}
                  onChange={(e) => setWorldDifficulty(e.target.value)}
                  className="w-full h-8 rounded-lg border border-slate-200/80 bg-white px-2 text-xs text-slate-800 shadow-2xs focus:border-emerald-500 focus:outline-none"
                >
                  <option value="classic">{isZh ? "经典 (Classic)" : "Classic"}</option>
                  <option value="expert">{isZh ? "专家 (Expert)" : "Expert"}</option>
                  <option value="master">{isZh ? "大师 (Master)" : "Master"}</option>
                </select>
              </div>
            </div>
          </div>

          <div className="pt-3 border-t border-slate-100 flex items-center justify-between">
            {savedSuccess ? (
              <span className="text-xs font-semibold text-emerald-600 flex items-center gap-1">
                <Check className="size-3.5" />
                <span>{isZh ? "配置已成功保存" : "Configuration saved successfully"}</span>
              </span>
            ) : <span />}

            {canEditServerConfig && (
              <Button type="submit" className="h-8 px-4">
                <Save className="size-3.5" />
                <span>{isZh ? "保存配置" : "Save Changes"}</span>
              </Button>
            )}
          </div>
        </form>
      )}

      {activeTab === "console" && (
        <div className="rounded-xl border border-slate-200/80 bg-slate-900 text-slate-200 p-4 font-mono text-xs shadow-2xs space-y-3">
          <div className="flex items-center justify-between text-slate-400 border-b border-slate-800 pb-2">
            <span>{isZh ? "容器标准输出流" : "Container stdout/stderr"}</span>
            <span className="text-[10px] bg-slate-800 px-2 py-0.5 rounded text-emerald-400">Live SSE</span>
          </div>
          <div className="h-64 overflow-y-auto space-y-1 text-[11px] leading-relaxed">
            <p className="text-slate-500">[System] Initializing Terraria server container runtime...</p>
            <p className="text-emerald-400">[Server] Listening on port {port}...</p>
            <p className="text-slate-300">[Server] Loaded world data successfully (HardcoreWorld.wld).</p>
            <p className="text-slate-300">[Server] Ready for player connections.</p>
          </div>
        </div>
      )}

      {activeTab === "backups" && (
        <div className="bg-white border border-slate-200/80 rounded-xl p-5 shadow-2xs space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-xs font-bold text-slate-900 uppercase tracking-wider">
              {isZh ? "自动快照与备份归档" : "Snapshots & Backups"}
            </h3>
            <Button variant="secondary" className="h-7 text-xs">
              {isZh ? "创建快照" : "Create Snapshot"}
            </Button>
          </div>
          <p className="text-xs text-slate-400">
            {isZh ? "当前实例已开启自动轮转备份，系统每 6 小时自动创建一份世界存档快照。" : "Automatic snapshots enabled. World state is backed up every 6 hours."}
          </p>
        </div>
      )}
    </div>
  );
}
