"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, Flame, KeyRound, Moon, Save, Shield, ShieldAlert, Sliders, Sparkles, Swords, Users, Zap } from "lucide-react";
import { Button, Input } from "@/components/ui";
import { useToast } from "@/components/toast-context";
import { useI18n } from "@/lib/i18n";
import { updateGameServerConfig } from "@/lib/api";
import { gameServerJoinPort } from "@/lib/game-server-resource";
import { cn } from "@/lib/utils";
import type { GameServerResource } from "@/lib/types";

export function ServerGameRules({
  server
}: {
  server: GameServerResource;
}) {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const toast = useToast();
  const client = useQueryClient();

  const gameKey = server.gameKey || "terraria";
  const providerKey = server.providerKey;

  const currentConfig = (server.spec?.config ?? {}) as Record<string, unknown>;

  // Form State
  const [draft, setDraft] = useState<Record<string, unknown>>({ ...currentConfig });

  const updateField = (key: string, value: unknown) => {
    setDraft((prev) => {
      const next = { ...prev, [key]: value };
      if (key.includes(".")) {
        const parts = key.split(".");
        const parent = parts[0];
        const child = parts[1];
        if (parent && child) {
          const parentObj = { ...((next[parent] ?? {}) as Record<string, unknown>), [child]: value };
          next[parent] = parentObj;
        }
      }
      return next;
    });
  };

  const saveMutation = useMutation({
    mutationFn: async () => {
      await updateGameServerConfig(server.id, draft, gameServerJoinPort(server));
    },
    onSuccess: async () => {
      toast.success(
        isZh ? "游戏房间规则已保存！" : "Game Rules Saved!",
        isZh ? "请重启房间以使新规则完全生效" : "Restart room to apply changes."
      );
      await client.invalidateQueries({ queryKey: ["game-server", server.id] });
      await client.invalidateQueries({ queryKey: ["game-servers"] });
    },
    onError: (err) => {
      toast.error(isZh ? "保存失败" : "Failed to save", err instanceof Error ? err.message : "");
    }
  });

  return (
    <div className="space-y-6">
      {/* Top Banner Header */}
      <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-4 sm:p-5 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <Sliders className="size-4 text-panel-green" />
            <h3 className="text-sm font-bold text-white tracking-tight">
              {isZh ? "🎮 游戏规则与房间设定" : "🎮 Game Room Rules & Settings"}
            </h3>
            <span className="rounded bg-slate-800 px-2 py-0.5 text-[10px] font-mono text-slate-300">
              {gameKey.toUpperCase()}
            </span>
          </div>
          <p className="mt-1 text-xs text-slate-400">
            {isZh ? "调整核心游戏倍率与难度，保存后重启房间生效" : "Adjust rates & gameplay. Restart room to apply."}
          </p>
        </div>

        <Button
          type="button"
          disabled={saveMutation.isPending}
          onClick={() => saveMutation.mutate()}
          className="gap-2 px-6 shrink-0"
        >
          <Save className="size-4" />
          <span>{saveMutation.isPending ? (isZh ? "正在保存..." : "Saving...") : (isZh ? "保存游戏规则设定" : "Save Game Rules")}</span>
        </Button>
      </div>

      {/* Render based on GameKey */}
      {gameKey === "palworld" ? (
        <PalworldRules draft={draft} onChange={updateField} isZh={isZh} />
      ) : gameKey === "minecraft" ? (
        <MinecraftRules draft={draft} onChange={updateField} isZh={isZh} />
      ) : gameKey === "dont-starve-together" || providerKey === "dont-starve-together" ? (
        <DSTRules draft={draft} onChange={updateField} isZh={isZh} />
      ) : (
        <TerrariaRules draft={draft} onChange={updateField} isZh={isZh} />
      )}
    </div>
  );
}

// -------------------------------------------------------------
// 1. Palworld 专属规则面板 (经验倍率 / 捕获率 / 孵蛋时间 / 死亡惩罚)
// -------------------------------------------------------------
function PalworldRules({
  draft,
  onChange,
  isZh
}: {
  draft: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
  isZh: boolean;
}) {
  const expRate = Number(draft.expRate ?? 1);
  const captureRate = Number(draft.captureRate ?? 1);
  const eggHatchingTime = Number(draft.eggHatchingTime ?? 72);
  const deathPenalty = String(draft.deathPenalty ?? "None");
  const pvp = Boolean(draft.pvp);
  const enableInvaderEnemy = Boolean(draft.enableInvaderEnemy ?? true);
  const enableFastTravel = Boolean(draft.enableFastTravel ?? true);
  const baseCampWorkerMaxNum = Number(draft.baseCampWorkerMaxNum ?? 15);
  const maxPlayers = Number(draft.maxPlayers ?? 8);
  const password = String(draft.serverPassword ?? "");

  const deathPenalties = [
    { value: "None", labelZh: "不掉落 (休闲推荐)", labelEn: "None (Casual)", descZh: "死亡后保留所有装备与帕鲁", descEn: "Keep everything on death" },
    { value: "Item", labelZh: "仅掉落物品", labelEn: "Drop Items", descZh: "掉落背包道具，保留装备和帕鲁", descEn: "Drop items only" },
    { value: "ItemAndEquipment", labelZh: "掉落物品和装备", labelEn: "Drop Items & Gear", descZh: "掉落背包和身上的装备", descEn: "Drop items and equipment" },
    { value: "All", labelZh: "全部掉落 (硬核)", labelEn: "Drop All (Hardcore)", descZh: "掉落背包、装备与随行帕鲁", descEn: "Drop everything including pals" }
  ];

  return (
    <div className="space-y-4">
      {/* Rate Sliders 3-Column */}
      <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-4">
        <div className="flex items-center gap-2">
          <Zap className="size-4 text-panel-gold" />
          <h4 className="text-xs font-bold uppercase tracking-wider text-white">
            {isZh ? "帕鲁世界倍率调节" : "Palworld Multipliers"}
          </h4>
        </div>

        <div className="grid gap-6 md:grid-cols-3">
          {/* Exp Rate */}
          <div className="space-y-2 rounded-lg border border-slate-800/80 bg-slate-950/50 p-4">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-slate-300">{isZh ? "升级经验倍率" : "Exp Rate"}</span>
              <span className="text-xs font-mono font-bold text-panel-green">{expRate}x</span>
            </div>
            <input
              type="range"
              min={0.5}
              max={10}
              step={0.5}
              value={expRate}
              onChange={(e) => onChange("expRate", Number(e.target.value))}
              className="w-full accent-panel-green"
            />
            <div className="flex justify-between text-[10px] text-slate-500 font-mono">
              <span>0.5x</span>
              <span>1x (原版)</span>
              <span>3x (爽玩)</span>
              <span>10x</span>
            </div>
          </div>

          {/* Capture Rate */}
          <div className="space-y-2 rounded-lg border border-slate-800/80 bg-slate-950/50 p-4">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-slate-300">{isZh ? "帕鲁捕获概率倍率" : "Capture Rate"}</span>
              <span className="text-xs font-mono font-bold text-sky-400">{captureRate}x</span>
            </div>
            <input
              type="range"
              min={0.5}
              max={3}
              step={0.1}
              value={captureRate}
              onChange={(e) => onChange("captureRate", Number(e.target.value))}
              className="w-full accent-sky-400"
            />
            <div className="flex justify-between text-[10px] text-slate-500 font-mono">
              <span>0.5x</span>
              <span>1x (标准)</span>
              <span>2x (轻松抓宠)</span>
              <span>3x</span>
            </div>
          </div>

          {/* Egg Hatching Time */}
          <div className="space-y-2 rounded-lg border border-slate-800/80 bg-slate-950/50 p-4">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-slate-300">{isZh ? "巨大蛋孵化时间" : "Egg Hatching Time"}</span>
              <span className="text-xs font-mono font-bold text-purple-400">
                {eggHatchingTime === 0 ? (isZh ? "⚡ 0小时 (立即孵化)" : "Instant") : `${eggHatchingTime} 小时`}
              </span>
            </div>
            <input
              type="range"
              min={0}
              max={72}
              step={2}
              value={eggHatchingTime}
              onChange={(e) => onChange("eggHatchingTime", Number(e.target.value))}
              className="w-full accent-purple-400"
            />
            <div className="flex justify-between text-[10px] text-slate-500 font-mono">
              <span className="text-panel-green font-bold">0h (秒孵)</span>
              <span>24h</span>
              <span>72h (默认)</span>
            </div>
          </div>
        </div>
      </div>

      {/* Death Penalty Selector */}
      <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
        <div className="flex items-center gap-2">
          <ShieldAlert className="size-4 text-rose-400" />
          <h4 className="text-xs font-bold uppercase tracking-wider text-white">
            {isZh ? "死亡惩罚机制" : "Death Penalty"}
          </h4>
        </div>

        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {deathPenalties.map((item) => {
            const active = deathPenalty === item.value;
            return (
              <button
                key={item.value}
                type="button"
                onClick={() => onChange("deathPenalty", item.value)}
                className={cn(
                  "flex flex-col justify-between rounded-xl border p-3.5 text-left transition",
                  active
                    ? "border-panel-green bg-panel-green/10 shadow-[0_0_15px_rgba(34,197,94,0.15)] ring-1 ring-panel-green"
                    : "border-slate-800 bg-slate-950/60 hover:border-slate-700 hover:bg-slate-900"
                )}
              >
                <div>
                  <div className="flex items-center justify-between">
                    <span className={cn("text-xs font-bold", active ? "text-panel-green" : "text-white")}>
                      {isZh ? item.labelZh : item.labelEn}
                    </span>
                    {active && <Check className="size-3.5 text-panel-green" />}
                  </div>
                  <p className="mt-1 text-[11px] text-slate-400 leading-snug">
                    {isZh ? item.descZh : item.descEn}
                  </p>
                </div>
              </button>
            );
          })}
        </div>
      </div>

      {/* Base & Combat Toggles */}
      <div className="grid gap-4 md:grid-cols-2">
        {/* Base Camp Settings */}
        <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
          <div className="flex items-center gap-2">
            <Users className="size-4 text-purple-400" />
            <h4 className="text-xs font-bold uppercase tracking-wider text-white">
              {isZh ? "据点与玩家上限" : "Base Camp & Capacity"}
            </h4>
          </div>

          <div className="space-y-4">
            <div>
              <div className="flex items-center justify-between text-xs mb-1.5">
                <span className="text-slate-300">{isZh ? "每据点工作帕鲁上限" : "Workers per Base"}</span>
                <span className="font-mono font-bold text-panel-green">{baseCampWorkerMaxNum} 只</span>
              </div>
              <input
                type="range"
                min={5}
                max={30}
                step={1}
                value={baseCampWorkerMaxNum}
                onChange={(e) => onChange("baseCampWorkerMaxNum", Number(e.target.value))}
                className="w-full accent-panel-green"
              />
            </div>

            <div>
              <div className="flex items-center justify-between text-xs mb-1.5">
                <span className="text-slate-300">{isZh ? "最大联机玩家数" : "Max Players"}</span>
                <span className="font-mono font-bold text-sky-400">{maxPlayers} 人</span>
              </div>
              <input
                type="range"
                min={2}
                max={32}
                step={2}
                value={maxPlayers}
                onChange={(e) => onChange("maxPlayers", Number(e.target.value))}
                className="w-full accent-sky-400"
              />
            </div>
          </div>
        </div>

        {/* Feature Switches & Password */}
        <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
          <div className="flex items-center gap-2">
            <Swords className="size-4 text-rose-400" />
            <h4 className="text-xs font-bold uppercase tracking-wider text-white">
              {isZh ? "功能开关与安全" : "Gameplay & Security"}
            </h4>
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between py-1 border-b border-slate-800/80">
              <span className="text-xs text-slate-300">{isZh ? "开启据点入侵事件" : "Invader Raids"}</span>
              <input
                type="checkbox"
                checked={enableInvaderEnemy}
                onChange={(e) => onChange("enableInvaderEnemy", e.target.checked)}
                className="size-4 accent-panel-green cursor-pointer"
              />
            </div>

            <div className="flex items-center justify-between py-1 border-b border-slate-800/80">
              <span className="text-xs text-slate-300">{isZh ? "开启大地图快速传送" : "Fast Travel"}</span>
              <input
                type="checkbox"
                checked={enableFastTravel}
                onChange={(e) => onChange("enableFastTravel", e.target.checked)}
                className="size-4 accent-panel-green cursor-pointer"
              />
            </div>

            <div className="flex items-center justify-between py-1 border-b border-slate-800/80">
              <span className="text-xs text-slate-300">{isZh ? "开启公会间 PVP 竞技" : "Enable PVP"}</span>
              <input
                type="checkbox"
                checked={pvp}
                onChange={(e) => onChange("pvp", e.target.checked)}
                className="size-4 accent-panel-green cursor-pointer"
              />
            </div>

            <div className="pt-1">
              <p className="text-xs text-slate-300 mb-1">{isZh ? "服务器进服密码" : "Server Password"}</p>
              <Input
                value={password}
                onChange={(e) => onChange("serverPassword", e.target.value)}
                placeholder={isZh ? "输入入服密码（留空公开）" : "Enter password"}
                className="text-xs bg-slate-950 border-slate-800"
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

// -------------------------------------------------------------
// 2. Minecraft 专属规则面板 (游戏模式 / 难度 / 正版验证 / 白名单)
// -------------------------------------------------------------
function MinecraftRules({
  draft,
  onChange,
  isZh
}: {
  draft: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
  isZh: boolean;
}) {
  const gameMode = String(draft.gameMode ?? "survival");
  const difficulty = String(draft.difficulty ?? "normal");
  const onlineMode = Boolean(draft.onlineMode ?? true);
  const whitelistEnabled = Boolean(draft.whitelistEnabled);
  const maxPlayers = Number(draft.maxPlayers ?? 20);

  const gameModes = [
    { key: "survival", labelZh: "生存模式", labelEn: "Survival", descZh: "采集、合成、战斗与成长", descEn: "Gather, build, survive" },
    { key: "creative", labelZh: "创造模式", labelEn: "Creative", descZh: "无限方块与自由飞行", descEn: "Unlimited resources and flying" },
    { key: "adventure", labelZh: "冒险模式", labelEn: "Adventure", descZh: "限制方块破坏，适合RPG地图", descEn: "Custom maps with restrictions" },
    { key: "spectator", labelZh: "旁观模式", labelEn: "Spectator", descZh: "隐形穿墙观战", descEn: "Fly through blocks and observe" }
  ];

  const difficulties = [
    { key: "peaceful", labelZh: "和平", labelEn: "Peaceful", descZh: "无敌对怪物，生命快速恢复", descEn: "No hostile mobs" },
    { key: "easy", labelZh: "简单", labelEn: "Easy", descZh: "怪物较弱，适合休闲起步", descEn: "Weaker hostile mobs" },
    { key: "normal", labelZh: "普通 (推荐)", labelEn: "Normal", descZh: "标准原版平衡体验", descEn: "Standard survival balance" },
    { key: "hard", labelZh: "困难", labelEn: "Hard", descZh: "高伤害怪物与饥饿惩罚", descEn: "Challenging monsters & starvation" }
  ];

  return (
    <div className="space-y-4">
      {/* 1. Game Mode */}
      <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
        <div className="flex items-center gap-2">
          <Sparkles className="size-4 text-panel-green" />
          <h4 className="text-xs font-bold uppercase tracking-wider text-white">
            {isZh ? "Minecraft 核心游戏模式" : "Minecraft Game Mode"}
          </h4>
        </div>

        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {gameModes.map((item) => {
            const active = gameMode === item.key;
            return (
              <button
                key={item.key}
                type="button"
                onClick={() => onChange("gameMode", item.key)}
                className={cn(
                  "flex flex-col justify-between rounded-xl border p-3.5 text-left transition",
                  active
                    ? "border-panel-green bg-panel-green/10 shadow-[0_0_15px_rgba(34,197,94,0.15)] ring-1 ring-panel-green"
                    : "border-slate-800 bg-slate-950/60 hover:border-slate-700 hover:bg-slate-900"
                )}
              >
                <div>
                  <div className="flex items-center justify-between">
                    <span className={cn("text-xs font-bold", active ? "text-panel-green" : "text-white")}>
                      {isZh ? item.labelZh : item.labelEn}
                    </span>
                    {active && <Check className="size-3.5 text-panel-green" />}
                  </div>
                  <p className="mt-1 text-[11px] text-slate-400 leading-snug">
                    {isZh ? item.descZh : item.descEn}
                  </p>
                </div>
              </button>
            );
          })}
        </div>
      </div>

      {/* 2. World Difficulty */}
      <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
        <div className="flex items-center gap-2">
          <Flame className="size-4 text-panel-gold" />
          <h4 className="text-xs font-bold uppercase tracking-wider text-white">
            {isZh ? "世界生存难度" : "World Difficulty"}
          </h4>
        </div>

        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {difficulties.map((item) => {
            const active = difficulty === item.key;
            return (
              <button
                key={item.key}
                type="button"
                onClick={() => onChange("difficulty", item.key)}
                className={cn(
                  "flex flex-col justify-between rounded-xl border p-3.5 text-left transition",
                  active
                    ? "border-panel-green bg-panel-green/10 shadow-[0_0_15px_rgba(34,197,94,0.15)] ring-1 ring-panel-green"
                    : "border-slate-800 bg-slate-950/60 hover:border-slate-700 hover:bg-slate-900"
                )}
              >
                <div>
                  <div className="flex items-center justify-between">
                    <span className={cn("text-xs font-bold", active ? "text-panel-green" : "text-white")}>
                      {isZh ? item.labelZh : item.labelEn}
                    </span>
                    {active && <Check className="size-3.5 text-panel-green" />}
                  </div>
                  <p className="mt-1 text-[11px] text-slate-400 leading-snug">
                    {isZh ? item.descZh : item.descEn}
                  </p>
                </div>
              </button>
            );
          })}
        </div>
      </div>

      {/* 3. Online Mode & Security */}
      <div className="grid gap-4 md:grid-cols-2">
        <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
          <div className="flex items-center gap-2">
            <Shield className="size-4 text-sky-400" />
            <h4 className="text-xs font-bold uppercase tracking-wider text-white">
              {isZh ? "正版验证与白名单" : "Online Mode & Whitelist"}
            </h4>
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between py-1 border-b border-slate-800">
              <div>
                <p className="text-xs text-slate-200 font-bold">{isZh ? "正版验证 (online-mode)" : "Online Mode"}</p>
                <p className="text-[11px] text-slate-400">{isZh ? "关闭后允许非正版离线客户端加入" : "Allow offline players"}</p>
              </div>
              <input
                type="checkbox"
                checked={onlineMode}
                onChange={(e) => onChange("onlineMode", e.target.checked)}
                className="size-4 accent-panel-green cursor-pointer"
              />
            </div>

            <div className="flex items-center justify-between py-1">
              <div>
                <p className="text-xs text-slate-200 font-bold">{isZh ? "启用玩家白名单" : "Enable Whitelist"}</p>
                <p className="text-[11px] text-slate-400">{isZh ? "仅白名单内玩家可进入服务器" : "Only whitelisted players"}</p>
              </div>
              <input
                type="checkbox"
                checked={whitelistEnabled}
                onChange={(e) => onChange("whitelistEnabled", e.target.checked)}
                className="size-4 accent-panel-green cursor-pointer"
              />
            </div>
          </div>
        </div>

        <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
          <div className="flex items-center gap-2">
            <Users className="size-4 text-purple-400" />
            <h4 className="text-xs font-bold uppercase tracking-wider text-white">
              {isZh ? "最大玩家人数" : "Max Players"}
            </h4>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-xs text-slate-300">{isZh ? "服务器人数上限" : "Max Players"}</span>
            <span className="text-xs font-mono font-bold text-panel-green">{maxPlayers} 人</span>
          </div>
          <input
            type="range"
            min={2}
            max={64}
            step={2}
            value={maxPlayers}
            onChange={(e) => onChange("maxPlayers", Number(e.target.value))}
            className="w-full accent-panel-green"
          />
        </div>
      </div>
    </div>
  );
}

// -------------------------------------------------------------
// 3. DST 饥荒联机版专属规则面板 (双层洞穴分片 / 游戏模式 / 无人暂停)
// -------------------------------------------------------------
function DSTRules({
  draft,
  onChange,
  isZh
}: {
  draft: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
  isZh: boolean;
}) {
  const cavesObj = (draft.caves ?? {}) as Record<string, unknown>;
  const gameplayObj = (draft.gameplay ?? {}) as Record<string, unknown>;

  const gameMode = String(draft["gameplay.gameMode"] ?? gameplayObj.gameMode ?? draft.gameMode ?? "survival");
  const cavesEnabled = Boolean(
    draft["caves.enabled"] ??
    cavesObj.enabled ??
    draft.cavesEnabled ??
    (typeof draft.caves === "boolean" ? draft.caves : false)
  );
  const pauseWhenEmpty = Boolean(
    draft["gameplay.pauseWhenEmpty"] ??
    gameplayObj.pauseWhenEmpty ??
    draft.pauseWhenEmpty ??
    true
  );
  const pvp = Boolean(draft["gameplay.pvp"] ?? gameplayObj.pvp ?? draft.pvp ?? false);
  const maxPlayers = Number(draft["gameplay.maxPlayers"] ?? gameplayObj.maxPlayers ?? draft.maxPlayers ?? 6);

  const dstModes = [
    { key: "survival", labelZh: "生存模式 (标准)", labelEn: "Survival", descZh: "死后变成鬼魂，全员死亡世界重置", descEn: "Standard DST survival" },
    { key: "endless", labelZh: "无尽模式 (推荐休闲)", labelEn: "Endless", descZh: "在大门随时复活，世界永远不重置", descEn: "Revive at portal, no reset" },
    { key: "wilderness", labelZh: "荒野模式 (大乱斗)", labelEn: "Wilderness", descZh: "死亡随机刷新，无鬼魂无复活门", descEn: "Hardcore respawn anywhere" }
  ];

  return (
    <div className="space-y-4">
      {/* Game Mode */}
      <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
        <div className="flex items-center gap-2">
          <Flame className="size-4 text-panel-gold" />
          <h4 className="text-xs font-bold uppercase tracking-wider text-white">
            {isZh ? "饥荒游戏世界模式" : "DST Game Mode"}
          </h4>
        </div>

        <div className="grid gap-3 sm:grid-cols-3">
          {dstModes.map((item) => {
            const active = gameMode === item.key;
            return (
              <button
                key={item.key}
                type="button"
                onClick={() => onChange("gameplay.gameMode", item.key)}
                className={cn(
                  "flex flex-col justify-between rounded-xl border p-3.5 text-left transition",
                  active
                    ? "border-panel-green bg-panel-green/10 shadow-[0_0_15px_rgba(34,197,94,0.15)] ring-1 ring-panel-green"
                    : "border-slate-800 bg-slate-950/60 hover:border-slate-700 hover:bg-slate-900"
                )}
              >
                <div>
                  <div className="flex items-center justify-between">
                    <span className={cn("text-xs font-bold", active ? "text-panel-green" : "text-white")}>
                      {isZh ? item.labelZh : item.labelEn}
                    </span>
                    {active && <Check className="size-3.5 text-panel-green" />}
                  </div>
                  <p className="mt-1 text-[11px] text-slate-400 leading-snug">
                    {isZh ? item.descZh : item.descEn}
                  </p>
                </div>
              </button>
            );
          })}
        </div>
      </div>

      {/* Caves & Automation */}
      <div className="grid gap-4 md:grid-cols-2">
        <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
          <div className="flex items-center gap-2">
            <Zap className="size-4 text-panel-green" />
            <h4 className="text-xs font-bold uppercase tracking-wider text-white">
              {isZh ? "双层世界与洞穴分片" : "World Topology & Caves"}
            </h4>
          </div>

          <div className="flex items-center justify-between py-2 border-b border-slate-800">
            <div>
              <p className="text-xs text-slate-200 font-bold">{isZh ? "开启地下洞穴分片 (Caves)" : "Enable Caves Shard"}</p>
              <p className="text-[11px] text-slate-400">{isZh ? "打通地面与地下世界，解锁远古祭坛与洞穴生物" : "Ground + Underworld double shards"}</p>
            </div>
            <input
              type="checkbox"
              checked={cavesEnabled}
              onChange={(e) => onChange("caves.enabled", e.target.checked)}
              className="size-4 accent-panel-green cursor-pointer"
            />
          </div>

          <div className="flex items-center justify-between py-2">
            <div>
              <p className="text-xs text-slate-200 font-bold">{isZh ? "开启 PVP 玩家伤害" : "Enable PVP"}</p>
            </div>
            <input
              type="checkbox"
              checked={pvp}
              onChange={(e) => onChange("gameplay.pvp", e.target.checked)}
              className="size-4 accent-panel-green cursor-pointer"
            />
          </div>
        </div>

        <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
          <div className="flex items-center gap-2">
            <Moon className="size-4 text-sky-400" />
            <h4 className="text-xs font-bold uppercase tracking-wider text-white">
              {isZh ? "无人自动暂停与人数" : "Pause When Empty & Capacity"}
            </h4>
          </div>

          <div className="flex items-center justify-between py-2 border-b border-slate-800">
            <div>
              <p className="text-xs text-slate-200 font-bold">{isZh ? "无人时服务器自动休眠" : "Pause When Empty"}</p>
              <p className="text-[11px] text-slate-400">{isZh ? "防止所有玩家下线后世界天数空转、资源消耗" : "Freeze time when 0 players"}</p>
            </div>
            <input
              type="checkbox"
              checked={pauseWhenEmpty}
              onChange={(e) => onChange("gameplay.pauseWhenEmpty", e.target.checked)}
              className="size-4 accent-panel-green cursor-pointer"
            />
          </div>

          <div className="pt-2">
            <div className="flex items-center justify-between mb-1.5">
              <span className="text-xs text-slate-300">{isZh ? "最大玩家数" : "Max Players"}</span>
              <span className="text-xs font-mono font-bold text-panel-green">{maxPlayers} 人</span>
            </div>
            <input
              type="range"
              min={2}
              max={32}
              step={1}
              value={maxPlayers}
              onChange={(e) => onChange("gameplay.maxPlayers", Number(e.target.value))}
              className="w-full accent-panel-green"
            />
          </div>
        </div>
      </div>
    </div>
  );
}

// -------------------------------------------------------------
// 4. Terraria 泰拉瑞亚专属规则面板
// -------------------------------------------------------------
function TerrariaRules({
  draft,
  onChange,
  isZh
}: {
  draft: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
  isZh: boolean;
}) {
  const difficulty = String(draft.difficulty ?? "classic");
  const password = String(draft.password ?? draft.serverPassword ?? "");
  const maxPlayers = Number(draft.maxPlayers ?? 16);
  const autoSaveInterval = Number(draft.autoSaveIntervalMinutes ?? 10);
  const pvp = Boolean(draft.pvp);

  const difficulties = [
    { key: "classic", labelZh: "经典模式", labelEn: "Classic", descZh: "适合新手与休闲玩家", descEn: "Balanced adventure" },
    { key: "expert", labelZh: "专家模式", labelEn: "Expert", descZh: "敌人更强，掉落专属宝藏", descEn: "Bosses have extra drops" },
    { key: "master", labelZh: "大师挑战", labelEn: "Master", descZh: "极致难度，极品圣物坐骑", descEn: "Hardcore challenge" },
    { key: "journey", labelZh: "旅行模式", labelEn: "Journey", descZh: "无限创造与神级权限", descEn: "Sandbox mode" }
  ];

  const intervals = [
    { value: 5, label: "5 分钟" },
    { value: 10, label: "10 分钟 (推荐)" },
    { value: 15, label: "15 分钟" },
    { value: 30, label: "30 分钟" }
  ];

  return (
    <div className="space-y-4">
      {/* Difficulty */}
      <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
        <div className="flex items-center gap-2">
          <Flame className="size-4 text-panel-gold" />
          <h4 className="text-xs font-bold uppercase tracking-wider text-white">
            {isZh ? "世界游戏难度" : "Game Difficulty"}
          </h4>
        </div>

        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {difficulties.map((item) => {
            const active = difficulty === item.key;
            return (
              <button
                key={item.key}
                type="button"
                onClick={() => onChange("difficulty", item.key)}
                className={cn(
                  "flex flex-col justify-between rounded-xl border p-3.5 text-left transition",
                  active
                    ? "border-panel-green bg-panel-green/10 shadow-[0_0_15px_rgba(34,197,94,0.15)] ring-1 ring-panel-green"
                    : "border-slate-800 bg-slate-950/60 hover:border-slate-700 hover:bg-slate-900"
                )}
              >
                <div>
                  <div className="flex items-center justify-between">
                    <span className={cn("text-xs font-bold", active ? "text-panel-green" : "text-white")}>
                      {isZh ? item.labelZh : item.labelEn}
                    </span>
                    {active && <Check className="size-3.5 text-panel-green" />}
                  </div>
                  <p className="mt-1 text-[11px] text-slate-400 leading-snug">
                    {isZh ? item.descZh : item.descEn}
                  </p>
                </div>
              </button>
            );
          })}
        </div>
      </div>

      {/* Password & PvP & AutoSave */}
      <div className="grid gap-4 md:grid-cols-2">
        <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
          <div className="flex items-center gap-2">
            <KeyRound className="size-4 text-sky-400" />
            <h4 className="text-xs font-bold uppercase tracking-wider text-white">
              {isZh ? "入服密码保护" : "Join Password"}
            </h4>
          </div>
          <Input
            value={password}
            onChange={(e) => onChange("password", e.target.value)}
            placeholder={isZh ? "输入入服密码（留空为无密码）" : "Enter password (optional)"}
            className="text-xs bg-slate-950 border-slate-800 focus:border-panel-green"
          />

          <div className="pt-2">
            <span className="text-xs text-slate-300 mb-2 block">{isZh ? "自动快照存档频率" : "Auto Snapshot Interval"}</span>
            <div className="grid gap-2 grid-cols-4">
              {intervals.map((item) => (
                <button
                  key={item.value}
                  type="button"
                  onClick={() => onChange("autoSaveIntervalMinutes", item.value)}
                  className={cn(
                    "rounded-lg border px-2 py-1.5 text-xs font-medium text-center transition",
                    autoSaveInterval === item.value
                      ? "border-panel-green bg-panel-green/15 text-panel-green font-bold"
                      : "border-slate-800 bg-slate-950/60 text-slate-400 hover:text-white"
                  )}
                >
                  {item.label}
                </button>
              ))}
            </div>
          </div>
        </div>

        <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 space-y-3">
          <div className="flex items-center gap-2">
            <Users className="size-4 text-purple-400" />
            <h4 className="text-xs font-bold uppercase tracking-wider text-white">
              {isZh ? "房间最大人数与竞技" : "Room Capacity & Combat"}
            </h4>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-xs text-slate-300">{isZh ? "最大允许玩家数" : "Max Players"}</span>
            <span className="text-xs font-mono font-bold text-panel-green">{maxPlayers} 人</span>
          </div>
          <input
            type="range"
            min={2}
            max={64}
            step={2}
            value={maxPlayers}
            onChange={(e) => onChange("maxPlayers", Number(e.target.value))}
            className="w-full accent-panel-green"
          />

          <div className="flex items-center justify-between pt-3 border-t border-slate-800">
            <div className="flex items-center gap-2">
              <Swords className="size-3.5 text-rose-400" />
              <span className="text-xs text-slate-300">{isZh ? "允许玩家间 PvP 伤害" : "PvP Combat"}</span>
            </div>
            <input
              type="checkbox"
              checked={pvp}
              onChange={(e) => onChange("pvp", e.target.checked)}
              className="size-4 accent-panel-green cursor-pointer"
            />
          </div>
        </div>
      </div>
    </div>
  );
}
