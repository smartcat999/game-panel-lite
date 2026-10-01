"use client";

import { useState } from "react";
import {
  Calendar,
  CheckCircle2,
  ExternalLink,
  GitCommit,
  History,
  ShieldCheck,
  Sparkles,
  Wrench,
  Zap
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui";

export interface ReleaseNoteItem {
  title: string;
  desc: string;
}

export interface ReleaseVersion {
  version: string;
  channel: "stable" | "beta";
  date: string;
  tagline: string;
  security: {
    dataMigration: string;
    serverInterruption: string;
    configCompatibility: string;
    hotReload: string;
  };
  features?: ReleaseNoteItem[];
  bugFixes?: ReleaseNoteItem[];
  improvements?: ReleaseNoteItem[];
  compareUrl?: string;
}

export const RELEASES_DATA: ReleaseVersion[] = [
  {
    version: "v0.2.17",
    channel: "stable",
    date: "2026-10-01",
    tagline: "独立更新Tab · 饥荒官方角色立绘 · 定时自动备份",
    security: {
      dataMigration: "存档无需迁移",
      serverInterruption: "游戏服不中断",
      configCompatibility: "配置全兼容",
      hotReload: "极速热更新"
    },
    features: [
      {
        title: "独立版本更新中心",
        desc: "将面板版本检测、更新日志浏览与一键热升级拆分为独立 Tab 页面，并提供双源 CDN 加速与 304 缓存防限流。"
      },
      {
        title: "饥荒 18 位官方角色独立立绘",
        desc: "在在线玩家列表与侧边控制台中为全套饥荒联机版角色配置官方高清立绘，并增加房主与管理员防误踢保护。"
      },
      {
        title: "定时自动备份策略系统",
        desc: "支持为各游戏服务器配置灵活的周期性备份策略（支持 6h/12h/24h 轮转），超期自动清理，全天候守护存档安全。"
      }
    ],
    bugFixes: [
      {
        title: "首页预设模版快速建服参数带入",
        desc: "修复从控制台“快速推荐模版”卡片点击进入时，模版参数被默认游戏覆盖的问题，并增加已套用视觉高亮提示。"
      },
      {
        title: "备份文件下载唤起修复",
        desc: "修复在服务器回档页面点击下载按钮时未能正常唤起浏览器本地文件保存的问题，补齐标准附件响应头。"
      },
      {
        title: "冷启动更新检测限流阻断消除",
        desc: "通过 SQLite 本地持久化缓存、单飞并发控制以及 jsDelivr CDN 自动降级，彻底解决 GitHub API 限流导致的版本无法显示问题。"
      }
    ],
    improvements: [
      {
        title: "玩家状态内存异步同步机制",
        desc: "引入轻量级内存异步状态同步，大幅减少对底层游戏容器的高频轮询，降低面板空闲 CPU 占用。"
      },
      {
        title: "紧凑化监控指标信息密度",
        desc: "重构网络连接、CPU/内存指标与容器状态的图表排版，使深色仪表盘呈现更为细腻且信息密度更高。"
      }
    ],
    compareUrl: "https://github.com/smartcat999/game-panel-lite/compare/v0.2.16...v0.2.17"
  },
  {
    version: "v0.2.16",
    channel: "stable",
    date: "2026-09-15",
    tagline: "饥荒侧边控制台 · 运行镜像进度上报 · 洞穴世界术语对齐",
    security: {
      dataMigration: "存档无需迁移",
      serverInterruption: "游戏容器无需重建",
      configCompatibility: "配置全兼容",
      hotReload: "平滑热升级"
    },
    features: [
      {
        title: "饥荒联机版抽屉式运维控制台",
        desc: "新增侧边运维抽屉，支持随时保存世界、发送全服广播、快照一键回档，以及区分地上与洞穴世界的终端指令交互。"
      },
      {
        title: "运行镜像拉取真实进度上报",
        desc: "在安装游戏运行镜像时提供分层拉取百分比上报，避免长时间无视觉反馈造成的卡死误判。"
      }
    ],
    bugFixes: [
      {
        title: "建服版本检测逻辑解耦",
        desc: "只读状态的游戏版本检测不再锁定创建服务器入口，仅在执行实际镜像下载时进行安全保护。"
      }
    ],
    improvements: [
      {
        title: "统一地上世界与洞穴世界术语规范",
        desc: "对齐全平台配置字段命名，使用符合游戏原生直觉的“地上世界 / 洞穴世界”清晰命名与排版。"
      }
    ],
    compareUrl: "https://github.com/smartcat999/game-panel-lite/compare/v0.2.15...v0.2.16"
  },
  {
    version: "v0.2.15",
    channel: "stable",
    date: "2026-09-03",
    tagline: "运行镜像列表紧凑化 · 饥荒优雅停机与清理 · 模组断点重试",
    security: {
      dataMigration: "存档无需迁移",
      serverInterruption: "更新运行镜像需重启游戏服",
      configCompatibility: "配置全兼容",
      hotReload: "控制面热更新"
    },
    features: [
      {
        title: "高密度运行镜像管理大盘",
        desc: "精简运行镜像管理为四列紧凑列表，将刷新动作整合至表格工具栏，提供更快捷的维护操作。"
      }
    ],
    bugFixes: [
      {
        title: "创意工坊模组下载断点续传",
        desc: "下载超时或网络抖动后保留已接收的分块数据并自动重试，无需从零开始重复拉取大型 Mod。"
      }
    ],
    improvements: [
      {
        title: "饥荒多分片优雅关机序列",
        desc: "服务器重启时先优雅通知并停止 Master 主世界，再停止 Caves 洞穴，留足 Klei 官方大厅清理退出的时间窗口。"
      }
    ],
    compareUrl: "https://github.com/smartcat999/game-panel-lite/compare/v0.2.14...v0.2.15"
  },
  {
    version: "v0.2.14",
    channel: "stable",
    date: "2026-08-30",
    tagline: "多节点分布式调度 · 节点健康租约 · 自动流量穿透",
    security: {
      dataMigration: "存档无需迁移",
      serverInterruption: "游戏容器无需重建",
      configCompatibility: "配置全兼容",
      hotReload: "平滑热升级"
    },
    features: [
      {
        title: "分布式节点集群大盘与拓扑图",
        desc: "支持纳管多台远程独立机器，并在设置中一键查看跨地域节点连接状态与实时流量穿透拓扑。"
      }
    ],
    improvements: [
      {
        title: "工作节点 Agent 断网自愈保活",
        desc: "强化 Worker 状态机持久化赋值，即使节点间网络出现短时波动断连，也能在网络恢复后自动对齐状态。"
      }
    ],
    compareUrl: "https://github.com/smartcat999/game-panel-lite/compare/v0.2.13...v0.2.14"
  }
];

interface VersionTimelineProps {
  currentVersion?: string;
}

const defaultRelease: ReleaseVersion = RELEASES_DATA[0] as ReleaseVersion;

export function VersionTimeline({ currentVersion = "v0.2.17" }: VersionTimelineProps) {
  const [selectedVersionTag, setSelectedVersionTag] = useState<string>(defaultRelease.version);

  const selectedRelease: ReleaseVersion =
    RELEASES_DATA.find((r) => r.version === selectedVersionTag) ?? defaultRelease;

  const isCurrentRunning = selectedRelease.version === currentVersion;

  return (
    <div className="space-y-4">
      {/* Section Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <History className="size-4 text-panel-green" />
          <h3 className="text-sm font-semibold text-white tracking-tight">版本发布轨迹与详细说明</h3>
          <span className="rounded bg-slate-800/80 px-2 py-0.5 text-[11px] font-mono font-medium text-slate-400">
            {RELEASES_DATA.length} 个发布版本
          </span>
        </div>
        <span className="text-xs text-slate-500">点击左侧版本可切换查阅改动明细</span>
      </div>

      {/* Dual Column Layout */}
      <div className="flex flex-col lg:flex-row gap-5 items-start">
        {/* Left Column: Timeline Navigation */}
        <div className="w-full lg:w-80 shrink-0 rounded-xl border border-panel-line bg-panel-card/70 p-4 backdrop-blur-xs">
          <div className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3 px-2 flex items-center justify-between">
            <span>发布历史 (Timeline)</span>
            <span className="text-[10px] text-panel-green font-mono">STABLE</span>
          </div>

          <div className="relative pl-3 space-y-2 before:absolute before:left-[19px] before:top-3 before:bottom-3 before:w-px before:bg-slate-800">
            {RELEASES_DATA.map((rel) => {
              const active = rel.version === selectedRelease.version;
              const isCurrent = rel.version === currentVersion;

              return (
                <button
                  key={rel.version}
                  type="button"
                  onClick={() => setSelectedVersionTag(rel.version)}
                  className={cn(
                    "relative flex w-full flex-col items-start rounded-lg p-3 text-left transition-all",
                    active
                      ? "bg-slate-900 border border-panel-green/40 shadow-xs"
                      : "hover:bg-slate-900/50 border border-transparent text-slate-400 hover:text-slate-200"
                  )}
                >
                  {/* Timeline Dot */}
                  <span
                    className={cn(
                      "absolute -left-[16px] top-4.5 size-2 rounded-full transition-all",
                      active
                        ? "bg-panel-green ring-4 ring-panel-green/20 scale-125"
                        : "bg-slate-700 hover:bg-slate-500"
                    )}
                  />

                  <div className="flex w-full items-center justify-between gap-1.5">
                    <span
                      className={cn(
                        "font-mono text-sm font-bold tracking-tight",
                        active ? "text-white" : "text-slate-300"
                      )}
                    >
                      {rel.version}
                    </span>
                    {isCurrent ? (
                      <Badge className="bg-panel-green/15 text-panel-green border border-panel-green/30 text-[10px] py-0 px-1.5 font-medium">
                        当前运行
                      </Badge>
                    ) : rel.version === defaultRelease.version ? (
                      <Badge className="bg-panel-gold/15 text-panel-gold border border-panel-gold/30 text-[10px] py-0 px-1.5 font-medium">
                        最新版本
                      </Badge>
                    ) : null}
                  </div>

                  <div className="flex items-center gap-1.5 text-[11px] text-slate-500 font-mono mt-1">
                    <Calendar className="size-3" />
                    <span>{rel.date}</span>
                  </div>

                  <p className="mt-1.5 text-xs text-slate-400 line-clamp-1 leading-normal">
                    {rel.tagline}
                  </p>
                </button>
              );
            })}
          </div>
        </div>

        {/* Right Column: Version Details Panel */}
        <div className="flex-1 w-full min-w-0 rounded-xl border border-panel-line bg-panel-card p-5 md:p-6 space-y-6">
          {/* Version Header Card */}
          <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4 border-b border-panel-line/80 pb-5">
            <div className="space-y-1">
              <div className="flex flex-wrap items-center gap-2.5">
                <h2 className="text-xl font-bold text-white tracking-tight font-mono">
                  GamePanel Lite {selectedRelease.version}
                </h2>
                <Badge className="bg-panel-green/15 text-panel-green border border-panel-green/30 font-mono text-xs">
                  {selectedRelease.channel.toUpperCase()}
                </Badge>
                {isCurrentRunning ? (
                  <span className="flex items-center gap-1 text-xs text-panel-green font-medium bg-panel-green/10 px-2 py-0.5 rounded-full border border-panel-green/20">
                    <CheckCircle2 className="size-3" />
                    当前正运行此版本
                  </span>
                ) : null}
              </div>
              <p className="text-xs text-slate-400 font-mono flex items-center gap-1.5 pt-0.5">
                <Calendar className="size-3 text-slate-500" />
                <span>发布日期：{selectedRelease.date}</span>
              </p>
            </div>

            {selectedRelease.compareUrl ? (
              <a
                href={selectedRelease.compareUrl}
                target="_blank"
                rel="noreferrer"
                className="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-slate-800 bg-slate-900/80 px-3 py-1.5 text-xs font-medium text-slate-300 hover:text-white hover:border-slate-700 transition"
              >
                <GitCommit className="size-3.5 text-panel-green" />
                <span>查看代码变更对比</span>
                <ExternalLink className="size-3 text-slate-500" />
              </a>
            ) : null}
          </div>

          {/* Security & Compatibility Matrix */}
          <div className="space-y-2">
            <div className="flex items-center gap-1.5 text-xs font-semibold text-slate-300">
              <ShieldCheck className="size-4 text-panel-green" />
              <span>升级安全与兼容性保障</span>
            </div>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5">
              <div className="rounded-lg border border-panel-green/25 bg-panel-green/5 p-2.5 text-center">
                <p className="text-[11px] text-slate-400">数据迁移</p>
                <p className="text-xs font-semibold text-panel-green mt-0.5">{selectedRelease.security.dataMigration}</p>
              </div>
              <div className="rounded-lg border border-panel-green/25 bg-panel-green/5 p-2.5 text-center">
                <p className="text-[11px] text-slate-400">游戏服务</p>
                <p className="text-xs font-semibold text-panel-green mt-0.5">{selectedRelease.security.serverInterruption}</p>
              </div>
              <div className="rounded-lg border border-panel-green/25 bg-panel-green/5 p-2.5 text-center">
                <p className="text-[11px] text-slate-400">系统配置</p>
                <p className="text-xs font-semibold text-panel-green mt-0.5">{selectedRelease.security.configCompatibility}</p>
              </div>
              <div className="rounded-lg border border-panel-green/25 bg-panel-green/5 p-2.5 text-center">
                <p className="text-[11px] text-slate-400">平滑升级</p>
                <p className="text-xs font-semibold text-panel-green mt-0.5">{selectedRelease.security.hotReload}</p>
              </div>
            </div>
          </div>

          {/* Features Section */}
          {selectedRelease.features && selectedRelease.features.length > 0 ? (
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <div className="flex size-5 items-center justify-center rounded bg-panel-green/15 text-panel-green">
                  <Sparkles className="size-3" />
                </div>
                <h4 className="text-sm font-semibold text-slate-200">✨ 新功能特性 (Features)</h4>
                <span className="rounded bg-slate-800/80 px-1.5 py-0.2 text-[10px] font-mono text-slate-400">
                  {selectedRelease.features.length}
                </span>
              </div>
              <div className="space-y-2">
                {selectedRelease.features.map((item, idx) => (
                  <div
                    key={idx}
                    className="group rounded-lg border border-slate-800/70 bg-slate-950/40 p-3 hover:border-slate-700/80 transition"
                  >
                    <div className="flex items-start gap-2.5">
                      <span className="size-1.5 rounded-full bg-panel-green mt-2 shrink-0" />
                      <div className="space-y-0.5">
                        <p className="text-sm font-semibold text-slate-100 group-hover:text-panel-green transition-colors">
                          {item.title}
                        </p>
                        <p className="text-xs text-slate-400 leading-relaxed">{item.desc}</p>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          {/* Bug Fixes Section */}
          {selectedRelease.bugFixes && selectedRelease.bugFixes.length > 0 ? (
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <div className="flex size-5 items-center justify-center rounded bg-sky-500/15 text-sky-400">
                  <Wrench className="size-3" />
                </div>
                <h4 className="text-sm font-semibold text-slate-200">🐛 问题修复 (Bug Fixes)</h4>
                <span className="rounded bg-slate-800/80 px-1.5 py-0.2 text-[10px] font-mono text-slate-400">
                  {selectedRelease.bugFixes.length}
                </span>
              </div>
              <div className="space-y-2">
                {selectedRelease.bugFixes.map((item, idx) => (
                  <div
                    key={idx}
                    className="group rounded-lg border border-slate-800/70 bg-slate-950/40 p-3 hover:border-slate-700/80 transition"
                  >
                    <div className="flex items-start gap-2.5">
                      <span className="size-1.5 rounded-full bg-sky-400 mt-2 shrink-0" />
                      <div className="space-y-0.5">
                        <p className="text-sm font-semibold text-slate-100 group-hover:text-sky-300 transition-colors">
                          {item.title}
                        </p>
                        <p className="text-xs text-slate-400 leading-relaxed">{item.desc}</p>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          {/* Improvements Section */}
          {selectedRelease.improvements && selectedRelease.improvements.length > 0 ? (
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <div className="flex size-5 items-center justify-center rounded bg-purple-500/15 text-purple-400">
                  <Zap className="size-3" />
                </div>
                <h4 className="text-sm font-semibold text-slate-200">⚡️ 体验与性能优化 (Improvements)</h4>
                <span className="rounded bg-slate-800/80 px-1.5 py-0.2 text-[10px] font-mono text-slate-400">
                  {selectedRelease.improvements.length}
                </span>
              </div>
              <div className="space-y-2">
                {selectedRelease.improvements.map((item, idx) => (
                  <div
                    key={idx}
                    className="group rounded-lg border border-slate-800/70 bg-slate-950/40 p-3 hover:border-slate-700/80 transition"
                  >
                    <div className="flex items-start gap-2.5">
                      <span className="size-1.5 rounded-full bg-purple-400 mt-2 shrink-0" />
                      <div className="space-y-0.5">
                        <p className="text-sm font-semibold text-slate-100 group-hover:text-purple-300 transition-colors">
                          {item.title}
                        </p>
                        <p className="text-xs text-slate-400 leading-relaxed">{item.desc}</p>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
