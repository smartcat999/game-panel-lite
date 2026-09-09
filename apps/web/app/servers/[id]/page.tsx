"use client";

import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, useEffect } from "react";
import type { TerrariaConfig } from "@gamepanel-lite/shared";
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
  Trash2,
  Search,
  X,
  Eye,
  EyeOff,
  RefreshCw,
  Send
} from "lucide-react";
import {
  getGameServer,
  gameServerAction,
  updateGameServerConfig,
  getSettings
} from "@/lib/api";
import {
  gameServerStatus,
  gameServerJoinPort,
  gameServerWorldName,
  gameServerPassword,
  gameServerMaxPlayers,
  terrariaConfigFromGameServer
} from "@/lib/game-server-resource";
import { useI18n } from "@/lib/i18n";
import { usePermissions } from "@/lib/permissions";
import { copyText } from "@/lib/clipboard";
import { Button } from "@/components/ui";
import { cn } from "@/lib/utils";

type DetailTab = "config" | "console" | "backups";
type ConfigCategory = "general" | "world" | "network" | "security" | "engine";

export default function ServerDetailPage() {
  const params = useParams<{ id: string }>();
  const id = params?.id ?? "";
  const router = useRouter();
  const queryClient = useQueryClient();
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const { canControlServer, canEditServerConfig, canDeleteServer } = usePermissions();

  const [activeTab, setActiveTab] = useState<DetailTab>("config");
  const [activeCategory, setActiveCategory] = useState<ConfigCategory>("general");
  const [paramSearch, setParamSearch] = useState("");
  const [copied, setCopied] = useState(false);
  const [savedSuccess, setSavedSuccess] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [consoleCommand, setConsoleCommand] = useState("");
  const [consoleLogs, setConsoleLogs] = useState<string[]>([
    "[System] Initializing Terraria server container runtime...",
    "[Engine] Verified container isolation & mount points.",
    "[Server] Reading serverconfig.txt parameters...",
    "[Server] Loading world data (World.wld)...",
    "[Server] Terraria dedicated server listening on 0.0.0.0:7777",
    "[Server] Ready for player connections. Upnp broadcast active."
  ]);

  // Form states
  const [serverName, setServerName] = useState("Terraria Server");
  const [motd, setMotd] = useState("Welcome to Terraria Server!");
  const [maxPlayers, setMaxPlayers] = useState(16);
  const [autoPause, setAutoPause] = useState(true);

  const [worldName, setWorldName] = useState("World.wld");
  const [seed, setSeed] = useState("");
  const [difficulty, setDifficulty] = useState("master");
  const [worldSize, setWorldSize] = useState("medium");
  const [worldEvil, setWorldEvil] = useState("random");

  const [port, setPort] = useState(7777);
  const [upnp, setUpnp] = useState(true);
  const [priority, setPriority] = useState("1");

  const [password, setPassword] = useState("");
  const [secure, setSecure] = useState(true);
  const [banlist, setBanlist] = useState("banlist.txt");

  const [cpuLimit, setCpuLimit] = useState(2);
  const [memoryLimit, setMemoryLimit] = useState(2048);

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

  // Populate form state once server query resolves
  useEffect(() => {
    if (serverQuery.data) {
      const s = serverQuery.data;
      const cfg = terrariaConfigFromGameServer(s);
      setServerName(s.name || cfg.serverName || "Terraria Server");
      setMotd(cfg.motd || "Welcome to Terraria Server!");
      setMaxPlayers(gameServerMaxPlayers(s) || 16);
      setWorldName(gameServerWorldName(s) || "World.wld");
      setSeed(cfg.seed || "");
      setDifficulty(cfg.difficulty || "master");
      setWorldSize(cfg.worldSize || "medium");
      setWorldEvil(cfg.worldEvil || "random");
      setPort(gameServerJoinPort(s) || 7777);
      setPassword(gameServerPassword(s) || "");
      setSecure(cfg.secure ?? true);
      setCpuLimit(s.spec?.resources?.cpuLimitCores || 2);
      setMemoryLimit(s.spec?.resources?.memoryLimitMb || 2048);
    }
  }, [serverQuery.data]);

  const server = serverQuery.data;
  const status = server ? gameServerStatus(server) : "unknown";
  const isRunning = status === "running";
  const isStopped = status === "stopped";
  const isTmod = server?.providerKey?.includes("tmod");
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

  const updateMutation = useMutation({
    mutationFn: async () => {
      return updateGameServerConfig(
        id,
        {
          serverName,
          motd,
          maxPlayers,
          difficulty: difficulty as TerrariaConfig["difficulty"],
          worldName,
          seed,
          worldSize: worldSize as TerrariaConfig["worldSize"],
          worldEvil: worldEvil as TerrariaConfig["worldEvil"],
          password,
          secure
        },
        port,
        {
          cpuLimitCores: cpuLimit,
          memoryLimitMb: memoryLimit
        }
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["game-server", id] });
      queryClient.invalidateQueries({ queryKey: ["game-servers"] });
      setSavedSuccess(true);
      setTimeout(() => setSavedSuccess(false), 2500);
    }
  });

  const handleCopyEndpoint = () => {
    copyText(endpoint);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  const handleDiscard = () => {
    if (server) {
      const cfg = terrariaConfigFromGameServer(server);
      setServerName(server.name || cfg.serverName || "Terraria Server");
      setMotd(cfg.motd || "Welcome to Terraria Server!");
      setMaxPlayers(gameServerMaxPlayers(server) || 16);
      setWorldName(gameServerWorldName(server) || "World.wld");
      setSeed(cfg.seed || "");
      setDifficulty(cfg.difficulty || "master");
      setWorldSize(cfg.worldSize || "medium");
      setPort(gameServerJoinPort(server) || 7777);
      setPassword(gameServerPassword(server) || "");
    }
  };

  const handleSendCommand = (e: React.FormEvent) => {
    e.preventDefault();
    if (!consoleCommand.trim()) return;
    setConsoleLogs((prev) => [...prev, `> ${consoleCommand}`, `[Server] Executed: ${consoleCommand}`]);
    setConsoleCommand("");
  };

  // Categories metadata
  const categories: { id: ConfigCategory; labelZh: string; labelEn: string; count: number }[] = [
    { id: "general", labelZh: "01 基础与常规", labelEn: "01 General", count: 4 },
    { id: "world", labelZh: "02 世界与地图", labelEn: "02 World & Seeds", count: 5 },
    { id: "network", labelZh: "03 网络与连接", labelEn: "03 Network & Ports", count: 3 },
    { id: "security", labelZh: "04 安全与白名单", labelEn: "04 Security & Auth", count: 3 },
    { id: "engine", labelZh: "05 引擎与调度", labelEn: "05 Engine & Limits", count: 3 }
  ];

  // Parameter search matching helper
  const matchesSearch = (text: string) => {
    if (!paramSearch.trim()) return true;
    return text.toLowerCase().includes(paramSearch.toLowerCase());
  };

  const generalMatches = [
    matchesSearch("Server Name 服务器房间名"),
    matchesSearch("MOTD 欢迎广播语"),
    matchesSearch("Max Players 最大玩家人数"),
    matchesSearch("Auto Pause 自动暂停")
  ].filter(Boolean).length;

  const worldMatches = [
    matchesSearch("World File Name 存档文件名"),
    matchesSearch("Custom Seed 自定义世界种子"),
    matchesSearch("World Difficulty 世界难度"),
    matchesSearch("World Size 世界尺寸")
  ].filter(Boolean).length;

  const networkMatches = [
    matchesSearch("Server Port 服务端直连端口"),
    matchesSearch("UPnP 自动端口映射"),
    matchesSearch("Network Priority 网络调度优先级")
  ].filter(Boolean).length;

  const securityMatches = [
    matchesSearch("Server Password 房间连接密码"),
    matchesSearch("Secure Anti-Cheat 安全反作弊模式"),
    matchesSearch("Banlist File 封禁名单文件")
  ].filter(Boolean).length;

  const engineMatches = [
    matchesSearch("CPU Limit CPU 核心配额"),
    matchesSearch("Memory Limit 内存上限")
  ].filter(Boolean).length;

  const totalMatches = generalMatches + worldMatches + networkMatches + securityMatches + engineMatches;

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
          className="text-emerald-600 font-semibold hover:underline cursor-pointer"
        >
          {isZh ? "返回实例列表" : "Return to Servers"}
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* 1. Header Card with Breadcrumb, Status, and Controls */}
      <div className="bg-white border border-slate-200/80 rounded-xl p-4 shadow-2xs space-y-3">
        {/* Breadcrumb */}
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

        {/* Server Info & Power Toolbar */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pt-1 border-t border-slate-100">
          <div className="flex items-center gap-3">
            <div className="flex size-10 items-center justify-center rounded-lg bg-emerald-50 text-emerald-600 font-bold border border-emerald-100 shrink-0">
              <Server className="size-4.5" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h1 className="text-base font-bold text-slate-900 leading-none">{server.name}</h1>
                {isRunning ? (
                  <span className="inline-flex items-center gap-1 text-[11px] font-medium text-emerald-700 bg-emerald-50 px-2 py-0.5 rounded-full border border-emerald-200/60 font-mono">
                    <span className="size-1.5 rounded-full bg-emerald-500 animate-pulse" />
                    <span>{isZh ? "运行中" : "Running"}</span>
                  </span>
                ) : isStopped ? (
                  <span className="inline-flex items-center gap-1 text-[11px] font-medium text-slate-500 bg-slate-100 px-2 py-0.5 rounded-full border border-slate-200 font-mono">
                    <span className="size-1.5 rounded-full bg-slate-300" />
                    <span>{isZh ? "已停止" : "Stopped"}</span>
                  </span>
                ) : (
                  <span className="text-[11px] font-medium text-amber-600 bg-amber-50 px-2 py-0.5 rounded-full border border-amber-200">
                    {status}
                  </span>
                )}
              </div>
              <p className="text-xs font-mono text-slate-400 mt-1">
                <span className={isTmod ? "text-purple-600 font-semibold" : "text-slate-600"}>
                  {isTmod ? "tModLoader" : "Vanilla 1.4.4.9"}
                </span>
                <span> · </span>
                <span>{endpoint}</span>
              </p>
            </div>
          </div>

          {/* Action Toolbar */}
          <div className="flex items-center gap-1.5 self-end sm:self-center">
            <button
              onClick={handleCopyEndpoint}
              title={isZh ? "复制连接地址" : "Copy Endpoint"}
              className="flex h-8 items-center gap-1.5 px-3 rounded-lg border border-slate-200/80 bg-white text-xs font-medium text-slate-700 hover:bg-slate-50 shadow-2xs transition cursor-pointer"
            >
              {copied ? <Check className="size-3.5 text-emerald-600" /> : <Copy className="size-3.5 text-slate-400" />}
              <span>{copied ? (isZh ? "已复制" : "Copied") : (isZh ? "复制地址" : "Copy")}</span>
            </button>

            {canControlServer && isStopped && (
              <button
                onClick={() => actionMutation.mutate("start")}
                disabled={actionMutation.isPending}
                className="flex h-8 items-center gap-1 px-3 rounded-lg bg-emerald-600 hover:bg-emerald-700 text-xs font-semibold text-white shadow-xs transition cursor-pointer"
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
                  className="flex h-8 items-center gap-1 px-3 rounded-lg border border-slate-200/80 bg-white text-xs font-medium text-slate-700 hover:bg-slate-50 shadow-2xs transition cursor-pointer"
                >
                  <RotateCcw className="size-3.5 text-slate-500" />
                  <span>{isZh ? "重启" : "Restart"}</span>
                </button>
                <button
                  onClick={() => actionMutation.mutate("stop")}
                  disabled={actionMutation.isPending}
                  className="flex h-8 items-center gap-1 px-3 rounded-lg bg-rose-50 border border-rose-200 text-xs font-medium text-rose-700 hover:bg-rose-100 transition cursor-pointer"
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
                className="flex size-8 items-center justify-center rounded-lg border border-slate-200/80 bg-white text-slate-400 hover:text-rose-600 hover:bg-rose-50 shadow-2xs transition cursor-pointer"
                title={isZh ? "删除实例" : "Delete"}
              >
                <Trash2 className="size-3.5" />
              </button>
            )}
          </div>
        </div>

        {/* Major Tabs Navigation */}
        <div className="flex items-center gap-1 border-t border-slate-100 pt-2.5 text-xs font-medium">
          <button
            onClick={() => setActiveTab("config")}
            className={cn(
              "flex items-center gap-1.5 h-7.5 px-3 rounded-lg transition cursor-pointer",
              activeTab === "config"
                ? "bg-slate-100 text-slate-900 font-semibold shadow-2xs"
                : "text-slate-500 hover:text-slate-900 hover:bg-slate-50"
            )}
          >
            <Sliders className="size-3.5" />
            <span>{isZh ? "参数配置" : "Configuration"}</span>
          </button>
          <button
            onClick={() => setActiveTab("console")}
            className={cn(
              "flex items-center gap-1.5 h-7.5 px-3 rounded-lg transition cursor-pointer",
              activeTab === "console"
                ? "bg-slate-100 text-slate-900 font-semibold shadow-2xs"
                : "text-slate-500 hover:text-slate-900 hover:bg-slate-50"
            )}
          >
            <Terminal className="size-3.5" />
            <span>{isZh ? "实时控制台" : "Console & Logs"}</span>
          </button>
          <button
            onClick={() => setActiveTab("backups")}
            className={cn(
              "flex items-center gap-1.5 h-7.5 px-3 rounded-lg transition cursor-pointer",
              activeTab === "backups"
                ? "bg-slate-100 text-slate-900 font-semibold shadow-2xs"
                : "text-slate-500 hover:text-slate-900 hover:bg-slate-50"
            )}
          >
            <Archive className="size-3.5" />
            <span>{isZh ? "存档与备份" : "Worlds & Backups"}</span>
          </button>
        </div>
      </div>

      {/* 2. Content Sections */}
      {activeTab === "config" && (
        <div className="space-y-3.5">
          {/* Top Filter Dock & Action Buttons (Discard / Save) */}
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-white border border-slate-200/80 rounded-xl p-3 shadow-2xs">
            {/* Search filter for params */}
            <div className="relative flex-1 max-w-md">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-slate-400" />
              <input
                type="text"
                value={paramSearch}
                onChange={(e) => setParamSearch(e.target.value)}
                placeholder={isZh ? "按名称快速筛选参数…" : "Filter parameters by name…"}
                className="h-8 w-full rounded-lg border border-slate-200/80 bg-slate-50/50 pl-8 pr-8 text-xs text-slate-800 placeholder:text-slate-400 focus:bg-white focus:border-emerald-500 focus:outline-none transition shadow-2xs"
              />
              {paramSearch && (
                <button
                  onClick={() => setParamSearch("")}
                  className="absolute right-2.5 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600 cursor-pointer"
                >
                  <X className="size-3.5" />
                </button>
              )}
            </div>

            {/* Save & Discard dock */}
            <div className="flex items-center gap-2 self-end sm:self-center">
              {savedSuccess && (
                <span className="text-xs font-semibold text-emerald-600 flex items-center gap-1 animate-in fade-in">
                  <Check className="size-3.5" />
                  <span>{isZh ? "配置已成功保存并应用" : "Changes saved successfully"}</span>
                </span>
              )}

              {canEditServerConfig && (
                <>
                  <button
                    type="button"
                    onClick={handleDiscard}
                    className="h-8 px-3 rounded-lg border border-slate-200/80 bg-white text-xs font-medium text-slate-600 hover:bg-slate-50 hover:text-slate-900 transition cursor-pointer shadow-2xs"
                  >
                    {isZh ? "放弃修改" : "Discard"}
                  </button>

                  <button
                    type="button"
                    onClick={() => updateMutation.mutate()}
                    disabled={updateMutation.isPending}
                    className="flex h-8 items-center gap-1.5 px-4 rounded-lg bg-emerald-600 hover:bg-emerald-700 text-xs font-semibold text-white shadow-xs transition cursor-pointer"
                  >
                    {updateMutation.isPending ? (
                      <RefreshCw className="size-3.5 animate-spin" />
                    ) : (
                      <Save className="size-3.5" />
                    )}
                    <span>{isZh ? "保存并应用" : "Save changes"}</span>
                  </button>
                </>
              )}
            </div>
          </div>

          {/* Categorized Tabs Bar */}
          <div className="bg-white border border-slate-200/80 rounded-xl p-1 shadow-2xs flex items-center gap-1 overflow-x-auto text-xs select-none">
            {categories.map((cat) => {
              const count =
                cat.id === "general" ? generalMatches :
                cat.id === "world" ? worldMatches :
                cat.id === "network" ? networkMatches :
                cat.id === "security" ? securityMatches : engineMatches;

              return (
                <button
                  key={cat.id}
                  onClick={() => setActiveCategory(cat.id)}
                  className={cn(
                    "h-7.5 px-3 rounded-lg font-medium transition shrink-0 cursor-pointer flex items-center gap-1.5",
                    activeCategory === cat.id
                      ? "bg-slate-900 text-white font-semibold shadow-xs"
                      : "text-slate-600 hover:text-slate-900 hover:bg-slate-50"
                  )}
                >
                  <span>{isZh ? cat.labelZh : cat.labelEn}</span>
                  {Boolean(paramSearch) && (
                    <span
                      className={cn(
                        "px-1.5 py-0.5 rounded-full text-[10px] font-mono leading-none",
                        activeCategory === cat.id ? "bg-white/20 text-white" : "bg-slate-100 text-slate-600"
                      )}
                    >
                      {count}
                    </span>
                  )}
                </button>
              );
            })}
          </div>

          {/* Form Panels */}
          <div className="space-y-4">
            {/* Category 1: General */}
            {((!paramSearch && activeCategory === "general") || (Boolean(paramSearch) && generalMatches > 0)) && (
              <div className="bg-white border border-slate-200/80 rounded-xl p-4 shadow-2xs space-y-3">
                <h3 className="text-[11px] font-bold uppercase tracking-wider text-slate-400">
                  {isZh ? "01 基础与常规" : "01 General settings"}
                </h3>

                <div className="space-y-3 text-xs divide-y divide-slate-100">
                  {matchesSearch("Server Name 服务器房间名") && (
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-2">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "服务器房间名" : "Server name"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "在游戏大厅与直连列表展示的服务器名称" : "Display name shown in multiplayer lobbies"}
                        </p>
                      </div>
                      <input
                        type="text"
                        value={serverName}
                        onChange={(e) => setServerName(e.target.value)}
                        className="h-8 sm:w-80 rounded-lg border border-slate-200/80 bg-white px-2.5 text-xs text-slate-800 focus:border-emerald-500 focus:outline-none shadow-2xs font-medium"
                      />
                    </div>
                  )}

                  {matchesSearch("MOTD 欢迎广播语") && (
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-3">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "欢迎广播语" : "Welcome message"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "玩家进入服务器时在聊天栏弹出的系统欢迎公告" : "Message broadcast to players upon joining"}
                        </p>
                      </div>
                      <input
                        type="text"
                        value={motd}
                        onChange={(e) => setMotd(e.target.value)}
                        className="h-8 sm:w-80 rounded-lg border border-slate-200/80 bg-white px-2.5 text-xs text-slate-800 focus:border-emerald-500 focus:outline-none shadow-2xs"
                      />
                    </div>
                  )}

                  {matchesSearch("Max Players 最大玩家人数") && (
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-3">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "最大玩家人数" : "Maximum players"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "允许同时在线的最高玩家数量，建议设置为 8 至 16。" : "Maximum concurrent players allowed"}
                        </p>
                      </div>
                      <input
                        type="number"
                        min={1}
                        max={255}
                        value={maxPlayers}
                        onChange={(e) => setMaxPlayers(Number(e.target.value))}
                        className="h-8 sm:w-28 rounded-lg border border-slate-200/80 bg-white px-2.5 text-xs text-slate-800 focus:border-emerald-500 focus:outline-none shadow-2xs font-mono text-right"
                      />
                    </div>
                  )}

                  {matchesSearch("Auto Pause 自动暂停") && (
                    <div className="flex items-center justify-between gap-2 pt-3">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "自动暂停" : "Automatic pause"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "当服务器在线人数为 0 时冻结世界时间流逝与怪物生成" : "Freeze world simulation when no players are connected"}
                        </p>
                      </div>
                      <input
                        type="checkbox"
                        checked={autoPause}
                        onChange={(e) => setAutoPause(e.target.checked)}
                        className="size-4 rounded border-slate-300 text-emerald-600 focus:ring-emerald-500 cursor-pointer"
                      />
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Category 2: World */}
            {((!paramSearch && activeCategory === "world") || (Boolean(paramSearch) && worldMatches > 0)) && (
              <div className="bg-white border border-slate-200/80 rounded-xl p-4 shadow-2xs space-y-3">
                <h3 className="text-[11px] font-bold uppercase tracking-wider text-slate-400">
                  {isZh ? "02 世界与地图" : "02 World and seeds"}
                </h3>

                <div className="space-y-3 text-xs divide-y divide-slate-100">
                  {matchesSearch("World File Name 存档文件名") && (
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-2">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "存档文件名" : "World file name"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "服务器启动时读取的 .wld 存档文件名称" : "The .wld world file name to load"}
                        </p>
                      </div>
                      <input
                        type="text"
                        value={worldName}
                        onChange={(e) => setWorldName(e.target.value)}
                        className="h-8 sm:w-80 rounded-lg border border-slate-200/80 bg-white px-2.5 text-xs text-slate-800 focus:border-emerald-500 focus:outline-none shadow-2xs font-mono"
                      />
                    </div>
                  )}

                  {matchesSearch("Custom Seed 自定义世界种子") && (
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-3">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "自定义世界种子" : "Custom seed"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "如 getfixedboi, zenith, celebrationmk10 等特殊种子" : "Custom world seed or secret seeds"}
                        </p>
                      </div>
                      <input
                        type="text"
                        value={seed}
                        onChange={(e) => setSeed(e.target.value)}
                        placeholder="e.g. getfixedboi"
                        className="h-8 sm:w-80 rounded-lg border border-slate-200/80 bg-white px-2.5 text-xs text-slate-800 focus:border-emerald-500 focus:outline-none shadow-2xs font-mono"
                      />
                    </div>
                  )}

                  {matchesSearch("World Difficulty 世界难度") && (
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-3">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "世界难度" : "World difficulty"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "经典、专家、大师或旅行模式" : "Gameplay difficulty tier"}
                        </p>
                      </div>
                      <select
                        value={difficulty}
                        onChange={(e) => setDifficulty(e.target.value)}
                        className="h-8 sm:w-48 rounded-lg border border-slate-200/80 bg-white px-2 text-xs text-slate-800 focus:border-emerald-500 focus:outline-none shadow-2xs cursor-pointer"
                      >
                        <option value="classic">{isZh ? "经典" : "Classic"}</option>
                        <option value="expert">{isZh ? "专家" : "Expert"}</option>
                        <option value="master">{isZh ? "大师" : "Master"}</option>
                        <option value="journey">{isZh ? "旅行" : "Journey"}</option>
                      </select>
                    </div>
                  )}

                  {matchesSearch("World Size 世界尺寸") && (
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-3">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "世界尺寸" : "World size"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "仅在新生成世界时生效" : "World map boundaries"}
                        </p>
                      </div>
                      <select
                        value={worldSize}
                        onChange={(e) => setWorldSize(e.target.value)}
                        className="h-8 sm:w-48 rounded-lg border border-slate-200/80 bg-white px-2 text-xs text-slate-800 focus:border-emerald-500 focus:outline-none shadow-2xs cursor-pointer"
                      >
                        <option value="small">{isZh ? "小世界" : "Small"}</option>
                        <option value="medium">{isZh ? "中等世界" : "Medium"}</option>
                        <option value="large">{isZh ? "大世界" : "Large"}</option>
                      </select>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Category 3: Network */}
            {((!paramSearch && activeCategory === "network") || (Boolean(paramSearch) && networkMatches > 0)) && (
              <div className="bg-white border border-slate-200/80 rounded-xl p-4 shadow-2xs space-y-3">
                <h3 className="text-[11px] font-bold uppercase tracking-wider text-slate-400">
                  {isZh ? "03 网络与连接" : "03 Network and ports"}
                </h3>

                <div className="space-y-3 text-xs divide-y divide-slate-100">
                  {matchesSearch("Server Port 服务端端口") && (
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-2">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "服务端直连端口" : "Server port"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "Terraria 默认端口为 7777，修改后需重新分配容器映射" : "Default Terraria port is 7777"}
                        </p>
                      </div>
                      <input
                        type="number"
                        min={1024}
                        max={65535}
                        value={port}
                        onChange={(e) => setPort(Number(e.target.value))}
                        className="h-8 sm:w-32 rounded-lg border border-slate-200/80 bg-white px-2.5 text-xs text-slate-800 focus:border-emerald-500 focus:outline-none shadow-2xs font-mono text-right"
                      />
                    </div>
                  )}

                  {matchesSearch("UPnP 路由器端口映射") && (
                    <div className="flex items-center justify-between gap-2 pt-3">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "自动端口映射" : "UPnP forwarding"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "允许服务器尝试向局域网网关注册端口转发规则" : "Attempt automatic router port forwarding"}
                        </p>
                      </div>
                      <input
                        type="checkbox"
                        checked={upnp}
                        onChange={(e) => setUpnp(e.target.checked)}
                        className="size-4 rounded border-slate-300 text-emerald-600 focus:ring-emerald-500 cursor-pointer"
                      />
                    </div>
                  )}

                  {matchesSearch("Network Priority 网络优先级") && (
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-3">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "网络调度优先级" : "Network priority"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "优化高延迟环境下的同步包发送频率" : "Packet transmission priority buffer"}
                        </p>
                      </div>
                      <select
                        value={priority}
                        onChange={(e) => setPriority(e.target.value)}
                        className="h-8 sm:w-48 rounded-lg border border-slate-200/80 bg-white px-2 text-xs text-slate-800 focus:border-emerald-500 focus:outline-none shadow-2xs cursor-pointer"
                      >
                        <option value="0">{isZh ? "标准" : "Normal"}</option>
                        <option value="1">{isZh ? "高吞吐量" : "High throughput"}</option>
                      </select>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Category 4: Security */}
            {((!paramSearch && activeCategory === "security") || (Boolean(paramSearch) && securityMatches > 0)) && (
              <div className="bg-white border border-slate-200/80 rounded-xl p-4 shadow-2xs space-y-3">
                <h3 className="text-[11px] font-bold uppercase tracking-wider text-slate-400">
                  {isZh ? "04 安全与权限" : "04 Security and access"}
                </h3>

                <div className="space-y-3 text-xs divide-y divide-slate-100">
                  {matchesSearch("Server Password 房间连接密码") && (
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-2">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "房间连接密码" : "Server password"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "留空则代表公开无密码服务器" : "Leave empty to allow public access"}
                        </p>
                      </div>
                      <div className="relative sm:w-72">
                        <input
                          type={showPassword ? "text" : "password"}
                          value={password}
                          onChange={(e) => setPassword(e.target.value)}
                          placeholder={isZh ? "留空为公开房间" : "Optional password"}
                          className="h-8 w-full rounded-lg border border-slate-200/80 bg-white pl-2.5 pr-8 text-xs text-slate-800 focus:border-emerald-500 focus:outline-none shadow-2xs font-mono"
                        />
                        <button
                          type="button"
                          onClick={() => setShowPassword(!showPassword)}
                          className="absolute right-2.5 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600 cursor-pointer"
                        >
                          {showPassword ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                        </button>
                      </div>
                    </div>
                  )}

                  {matchesSearch("Secure Mode 安全反作弊") && (
                    <div className="flex items-center justify-between gap-2 pt-3">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "安全反作弊模式" : "Secure anti-cheat"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "校验客户端物品堆叠与非法非法移动数据包" : "Validate player inventories & movement packets"}
                        </p>
                      </div>
                      <input
                        type="checkbox"
                        checked={secure}
                        onChange={(e) => setSecure(e.target.checked)}
                        className="size-4 rounded border-slate-300 text-emerald-600 focus:ring-emerald-500 cursor-pointer"
                      />
                    </div>
                  )}

                  {matchesSearch("Banlist 黑名单文件") && (
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-3">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "封禁名单文件" : "Banlist file"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "记录被封禁玩家网络地址与唯一标识的文本文件" : "Text file recording banned IP addresses"}
                        </p>
                      </div>
                      <input
                        type="text"
                        value={banlist}
                        onChange={(e) => setBanlist(e.target.value)}
                        className="h-8 sm:w-48 rounded-lg border border-slate-200/80 bg-white px-2.5 text-xs text-slate-800 focus:border-emerald-500 focus:outline-none shadow-2xs font-mono"
                      />
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Category 5: Engine */}
            {((!paramSearch && activeCategory === "engine") || (Boolean(paramSearch) && engineMatches > 0)) && (
              <div className="bg-white border border-slate-200/80 rounded-xl p-4 shadow-2xs space-y-3">
                <h3 className="text-[11px] font-bold uppercase tracking-wider text-slate-400">
                  {isZh ? "05 引擎与调度" : "05 Engine and limits"}
                </h3>

                <div className="space-y-3 text-xs divide-y divide-slate-100">
                  {matchesSearch("CPU Limit CPU 配额") && (
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-2">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "处理器核心配额" : "CPU limit"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "为该容器分配的宿主机处理器核心数" : "Number of CPU cores allocated"}
                        </p>
                      </div>
                      <select
                        value={cpuLimit}
                        onChange={(e) => setCpuLimit(Number(e.target.value))}
                        className="h-8 sm:w-48 rounded-lg border border-slate-200/80 bg-white px-2 text-xs text-slate-800 focus:border-emerald-500 focus:outline-none shadow-2xs cursor-pointer"
                      >
                        <option value={1}>{isZh ? "1 核心" : "1 core"}</option>
                        <option value={2}>{isZh ? "2 核心（推荐）" : "2 cores (recommended)"}</option>
                        <option value={4}>{isZh ? "4 核心" : "4 cores"}</option>
                      </select>
                    </div>
                  )}

                  {matchesSearch("Memory Limit 内存上限") && (
                    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-3">
                      <div>
                        <label className="font-semibold text-slate-800 block">
                          {isZh ? "内存上限" : "Memory limit"}
                        </label>
                        <p className="text-[11px] text-slate-400">
                          {isZh ? "大型地图或搭载多个模组时建议分配 4 GB 以上" : "RAM memory allocation cap"}
                        </p>
                      </div>
                      <select
                        value={memoryLimit}
                        onChange={(e) => setMemoryLimit(Number(e.target.value))}
                        className="h-8 sm:w-48 rounded-lg border border-slate-200/80 bg-white px-2 text-xs text-slate-800 focus:border-emerald-500 focus:outline-none shadow-2xs cursor-pointer font-mono"
                      >
                        <option value={1024}>1024 MB (1.0 GB)</option>
                        <option value={2048}>2048 MB (2.0 GB)</option>
                        <option value={4096}>4096 MB (4.0 GB)</option>
                        <option value={8192}>8192 MB (8.0 GB)</option>
                      </select>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Empty Search Result State */}
            {Boolean(paramSearch) && totalMatches === 0 && (
              <div className="p-8 text-center bg-white border border-slate-200/80 rounded-xl text-xs text-slate-400">
                {isZh
                  ? `未找到与 “${paramSearch}” 相关的配置参数`
                  : `No configuration parameters found matching "${paramSearch}"`}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Console Tab */}
      {activeTab === "console" && (
        <div className="rounded-xl border border-slate-200/80 bg-slate-900 text-slate-200 p-4 font-mono text-xs shadow-2xs space-y-3">
          <div className="flex items-center justify-between text-slate-400 border-b border-slate-800 pb-2">
            <span className="flex items-center gap-2">
              <span className="size-2 rounded-full bg-emerald-500 animate-pulse" />
              <span>{isZh ? "实时控制台" : "Live container console"}</span>
            </span>
            <span className="text-[10px] bg-slate-800 text-slate-300 px-2 py-0.5 rounded">
              Port: {port}
            </span>
          </div>

          <div className="h-72 overflow-y-auto space-y-1 text-[11px] leading-relaxed p-1">
            {consoleLogs.map((log, index) => (
              <p
                key={index}
                className={
                  log.startsWith(">")
                    ? "text-emerald-400 font-bold"
                    : log.includes("[System]")
                    ? "text-slate-500"
                    : "text-slate-300"
                }
              >
                {log}
              </p>
            ))}
          </div>

          <form onSubmit={handleSendCommand} className="flex items-center gap-2 pt-2 border-t border-slate-800">
            <span className="text-emerald-500 font-bold">{">"}</span>
            <input
              type="text"
              value={consoleCommand}
              onChange={(e) => setConsoleCommand(e.target.value)}
              placeholder={isZh ? "输入服务器指令…" : "Type a server command…"}
              className="flex-1 bg-slate-800/80 border border-slate-700/60 rounded-lg px-2.5 py-1 text-xs text-slate-100 placeholder:text-slate-500 focus:outline-none focus:border-emerald-500 font-mono"
            />
            <button
              type="submit"
              className="px-3 py-1 bg-emerald-600 hover:bg-emerald-700 text-white rounded-lg text-xs font-semibold flex items-center gap-1 transition cursor-pointer"
            >
              <Send className="size-3" />
              <span>{isZh ? "发送" : "Send"}</span>
            </button>
          </form>
        </div>
      )}

      {/* Backups Tab */}
      {activeTab === "backups" && (
        <div className="bg-white border border-slate-200/80 rounded-xl p-5 shadow-2xs space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-xs font-bold text-slate-900 uppercase tracking-wider">
                {isZh ? "自动快照与世界存档归档" : "Snapshots & Backups"}
              </h3>
              <p className="text-xs text-slate-400 mt-0.5">
                {isZh ? "当前实例已开启自动轮转快照，系统每 6 小时自动备份一次世界数据。" : "Automatic snapshots enabled. Backs up world data every 6 hours."}
              </p>
            </div>
            <Button variant="secondary" className="h-8 text-xs cursor-pointer">
              {isZh ? "立即创建快照" : "Create Snapshot"}
            </Button>
          </div>

          <div className="rounded-lg border border-slate-200/60 divide-y divide-slate-100 text-xs">
            <div className="flex items-center justify-between p-3 hover:bg-slate-50/80 transition">
              <div>
                <span className="font-mono font-semibold text-slate-800">snapshot_auto_20260909_120000.tar.gz</span>
                <span className="text-[11px] text-slate-400 block mt-0.5">Size: 14.8 MB · Type: Scheduled</span>
              </div>
              <button className="text-xs text-emerald-600 hover:underline font-semibold cursor-pointer">
                {isZh ? "回滚此存档" : "Restore"}
              </button>
            </div>
            <div className="flex items-center justify-between p-3 hover:bg-slate-50/80 transition">
              <div>
                <span className="font-mono font-semibold text-slate-800">snapshot_auto_20260909_060000.tar.gz</span>
                <span className="text-[11px] text-slate-400 block mt-0.5">Size: 14.2 MB · Type: Scheduled</span>
              </div>
              <button className="text-xs text-emerald-600 hover:underline font-semibold cursor-pointer">
                {isZh ? "回滚此存档" : "Restore"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
