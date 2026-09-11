"use client";

import Image from "next/image";
import Link from "next/link";
import {
  ArrowRight,
  Check,
  ChevronRight,
  CircleDot,
  Container,
  Copy,
  Cpu,
  Download,
  Gamepad2,
  Github,
  HardDrive,
  Menu,
  Play,
  Settings2,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  X,
  Zap,
} from "lucide-react";
import { useState } from "react";

const navLinks = [
  { href: "#features", label: "核心特性" },
  { href: "#architecture", label: "架构优势" },
  { href: "#showcase", label: "界面预览" },
  { href: "#quickstart", label: "极速上手" },
  { href: "#roadmap", label: "路线图" },
];

const capabilities = [
  {
    icon: Gamepad2,
    badge: "专精适配",
    title: "Terraria & tModLoader 双引擎",
    description: "开箱即用支持原版 Terraria 1.4.4.9 与 tModLoader 模组服。内置灾厄 (Calamity) 等流行模组支持与世界预设。",
  },
  {
    icon: Container,
    badge: "沙箱隔离",
    title: "原生 Docker 容器隔离",
    description: "单容器单实例，专属数据卷与端口映射。告别宿主机路径污染与权限风险，彻底杜绝路径穿越。",
  },
  {
    icon: TerminalSquare,
    badge: "实时推流",
    title: "毫秒级 SSE 实时控制台",
    description: "基于 Server-Sent Events 的流式日志与交互式命令终端。毫秒级日志推流，无需反复刷新或抓取日志。",
  },
  {
    icon: Zap,
    badge: "极速运维",
    title: "一键生命周期管控",
    description: "秒级启动、优雅停止、重启与健康探针。状态实时反馈，崩溃自动侦测与守护。",
  },
  {
    icon: HardDrive,
    badge: "数据安全",
    title: "时光机世界快照与备份",
    description: "一键导出与备份 .wld 世界档案，支持秒级快照回滚与跨服迁移，严格校验世界文件完整性。",
  },
  {
    icon: Cpu,
    badge: "轻量极速",
    title: "Go 语言高性能核心",
    description: "面板核心采用 Go 语言构建，常驻内存占用仅数十 MB，千元级家用 NAS 或低配 VPS 亦可流畅稳定运行。",
  },
];

const painPoints = [
  {
    pain: "传统方式：手动编辑复杂 serverconfig.txt，漏填错填即崩溃",
    solution: "GamePanel Lite：可视化表单调优，自动渲染标准配置文件，参数安全校验",
  },
  {
    pain: "传统方式：Docker Compose 端口冲突、文件挂载权限错乱",
    solution: "GamePanel Lite：原生 Go Docker SDK 自动化调度，端口冲突预警与隔离数据卷",
  },
  {
    pain: "传统方式：游戏世界意外损坏、误操作无处找回存档",
    solution: "GamePanel Lite：一键时光机世界快照，秒级归档与一键安全回档",
  },
  {
    pain: "传统方式：命令行黑盒执行，日志卡死延迟无法即时交互",
    solution: "GamePanel Lite：SSE 全双工日志推流与交互式命令行，实时掌控游戏世界动态",
  },
];

const showcases = [
  {
    id: "instances",
    title: "实例总览与运维",
    desc: "直观的卡片与表格视图，实时掌握服务器运行状态、端口映射、在线状态与快速启停操作。",
    image: "/official/interface-servers.png",
  },
  {
    id: "dashboard",
    title: "高性能看板",
    desc: "轻量深色控制台，聚合最近活跃世界、系统资源配额与一键开黑直连信息。",
    image: "/official/interface-dashboard.png",
  },
  {
    id: "console",
    title: "实时交互控制台",
    desc: "全屏 SSE 流式日志与原版服务端指令输入，支持 say、save、kick、ban 等命令即时响应。",
    image: "/official/interface-console.png",
  },
  {
    id: "backups",
    title: "时光机快照与世界管理",
    desc: "无损快照归档，支持世界上传、安全校验与秒级还原，守护每一座精心建造的泰拉世界。",
    image: "/official/interface-backups.png",
  },
];

const steps = [
  {
    step: "01",
    title: "选择游戏与版本预设",
    desc: "挑选原版 Terraria 1.4.4.9 或 tModLoader 模组服，一键载入标准开服模板。",
    icon: Download,
  },
  {
    step: "02",
    title: "设定世界与网络参数",
    desc: "自定义服务器名称、对局密码、端口与世界大小，面板自动生成并渲染合规配置。",
    icon: Settings2,
  },
  {
    step: "03",
    title: "一键启动，复制开黑直连",
    desc: "秒级拉起 Docker 容器，复制 IP 与端口分享给好友，直接在游戏中一键加入畅玩！",
    icon: Play,
  },
];

const roadmapItems = [
  {
    phase: "当前稳定版 (V1.0)",
    status: "已就绪",
    color: "text-emerald-400 border-emerald-500/30 bg-emerald-500/10",
    items: [
      "Terraria 原版 1.4.4.9 容器化编排",
      "tModLoader 模组服支持与启动预设",
      "单容器单实例 Docker 原生隔离",
      "SSE 毫秒级实时控制台与日志流",
      "世界 .wld 文件时光机备份与还原",
      "玩家直连信息一键复制与端口映射",
    ],
  },
  {
    phase: "下一阶段 (V1.5)",
    status: "进行中",
    color: "text-sky-400 border-sky-500/30 bg-sky-500/10",
    items: [
      "Steam Workshop 创意工坊模组一键检索安装",
      "S3 / WebDAV 远端对象存储自动异地备份",
      "高级世界编辑器与地图种子快速导入",
      "服务器定时自动重启与定时世界存盘",
    ],
  },
  {
    phase: "未来愿景 (V2.0)",
    status: "规划中",
    color: "text-purple-400 border-purple-500/30 bg-purple-500/10",
    items: [
      "Minecraft / 幻兽帕鲁 (Palworld) 多游戏提供商扩展",
      "集群跨节点多服集中纳管与反向代理",
      "Discord / 微信开服状态 Webhook 通知",
      "多用户细粒度开黑权限组管理",
    ],
  },
];

export function LandingPage() {
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [activeShowcase, setActiveShowcase] = useState(0);
  const [copied, setCopied] = useState(false);

  const copyDockerCommand = () => {
    navigator.clipboard.writeText("docker run -d --name gamepanel -p 8080:8080 -v /var/run/docker.sock:/var/run/docker.sock smartcat999/game-panel-lite:latest");
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="min-h-screen bg-[#080c14] text-zinc-100 selection:bg-emerald-500/30 selection:text-emerald-300">
      {/* Top Navigation */}
      <header className="sticky top-0 z-50 border-b border-white/[0.08] bg-[#080c14]/90 backdrop-blur-md">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
          <Link href="/" className="flex items-center gap-3">
            <div className="relative flex size-9 items-center justify-center overflow-hidden rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-1 shadow-sm shadow-emerald-500/20">
              <Image
                src="/avatar.svg"
                alt="GamePanel Lite Logo"
                width={32}
                height={32}
                className="pixelated object-contain"
              />
            </div>
            <div className="flex flex-col">
              <div className="flex items-center gap-2">
                <span className="font-bold tracking-tight text-white">GamePanel</span>
                <span className="rounded bg-emerald-500/20 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-400">
                  LITE
                </span>
              </div>
            </div>
          </Link>

          {/* Desktop Nav */}
          <nav className="hidden items-center gap-8 md:flex">
            {navLinks.map((link) => (
              <a
                key={link.label}
                href={link.href}
                className="text-sm font-medium text-zinc-400 transition-colors hover:text-white"
              >
                {link.label}
              </a>
            ))}
          </nav>

          {/* Desktop Actions */}
          <div className="hidden items-center gap-3 md:flex">
            <a
              href="https://github.com/smartcat999/game-panel-lite"
              target="_blank"
              rel="noreferrer"
              className="inline-flex h-9 items-center gap-2 rounded-lg border border-white/[0.1] bg-[#0f1624] px-3.5 text-xs font-medium text-zinc-300 transition hover:border-white/[0.2] hover:bg-[#131b2c] hover:text-white"
            >
              <Github className="size-4" />
              <span>GitHub</span>
            </a>
            <Link
              href="/login"
              className="inline-flex h-9 items-center gap-1.5 rounded-lg bg-emerald-500 px-4 text-xs font-bold text-zinc-950 shadow-md shadow-emerald-500/25 transition-all hover:bg-emerald-400 active:scale-[0.98]"
            >
              <span>进入控制台</span>
              <ArrowRight className="size-3.5" />
            </Link>
          </div>

          {/* Mobile Menu Button */}
          <button
            type="button"
            className="flex size-9 items-center justify-center rounded-lg border border-white/[0.1] text-zinc-400 hover:text-white md:hidden"
            onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            aria-label="Toggle Menu"
          >
            {mobileMenuOpen ? <X className="size-5" /> : <Menu className="size-5" />}
          </button>
        </div>

        {/* Mobile Dropdown */}
        {mobileMenuOpen && (
          <div className="border-b border-white/[0.08] bg-[#0c101a] px-4 py-4 md:hidden">
            <nav className="flex flex-col gap-3">
              {navLinks.map((link) => (
                <a
                  key={link.label}
                  href={link.href}
                  className="rounded-lg px-3 py-2 text-sm font-medium text-zinc-300 hover:bg-white/[0.05] hover:text-white"
                  onClick={() => setMobileMenuOpen(false)}
                >
                  {link.label}
                </a>
              ))}
              <div className="mt-2 flex flex-col gap-2 pt-2 border-t border-white/[0.08]">
                <Link
                  href="/login"
                  className="flex h-10 items-center justify-center gap-2 rounded-lg bg-emerald-500 font-bold text-zinc-950 text-sm"
                  onClick={() => setMobileMenuOpen(false)}
                >
                  进入控制台 <ArrowRight className="size-4" />
                </Link>
                <a
                  href="https://github.com/smartcat999/game-panel-lite"
                  target="_blank"
                  rel="noreferrer"
                  className="flex h-10 items-center justify-center gap-2 rounded-lg border border-white/[0.1] bg-[#0f1624] text-xs font-medium text-zinc-300"
                >
                  <Github className="size-4" /> GitHub 源码
                </a>
              </div>
            </nav>
          </div>
        )}
      </header>

      {/* Hero Section */}
      <section className="relative overflow-hidden border-b border-white/[0.08] pt-12 pb-20 sm:pt-16 sm:pb-24 lg:pt-20">
        {/* Glow Effects */}
        <div className="pointer-events-none absolute left-1/2 top-0 -translate-x-1/2 -translate-y-1/2 size-[600px] rounded-full bg-emerald-500/10 blur-[140px]" />
        <div className="pointer-events-none absolute right-10 top-1/3 size-[400px] rounded-full bg-sky-500/5 blur-[120px]" />

        <div className="relative mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="mx-auto max-w-3xl text-center">
            {/* Version Badge */}
            <div className="inline-flex items-center gap-2 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-3.5 py-1 text-xs font-medium text-emerald-400">
              <span className="size-1.5 rounded-full bg-emerald-400 animate-pulse" />
              <span>v1.0 稳定版 · 专为 Terraria 与 tModLoader 深度打造</span>
            </div>

            {/* Main Headline */}
            <h1 className="mt-6 text-balance text-4xl font-extrabold tracking-tight text-white sm:text-5xl lg:text-6xl">
              自托管游戏服务器，
              <br />
              <span className="bg-gradient-to-r from-emerald-400 via-teal-300 to-sky-400 bg-clip-text text-transparent">
                告别繁琐运维与命令行。
              </span>
            </h1>

            {/* Subheading */}
            <p className="mt-6 text-pretty text-base text-zinc-400 sm:text-lg sm:leading-8">
              轻量、极速、开箱即用的现代化自托管游戏服务器管理面板。
              <br className="hidden sm:inline" />
              原生 Docker 单容器沙箱隔离，毫秒级 SSE 实时控制台，一键世界时光机备份，仅需数十 MB 内存。
            </p>

            {/* Action Buttons */}
            <div className="mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row sm:gap-4">
              <Link
                href="/login"
                className="flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-emerald-500 px-6 font-bold text-zinc-950 shadow-lg shadow-emerald-500/25 transition-all hover:bg-emerald-400 hover:shadow-emerald-500/35 sm:w-auto active:scale-[0.98]"
              >
                <span>立即进入控制台</span>
                <ChevronRight className="size-4" />
              </Link>
              <a
                href="https://github.com/smartcat999/game-panel-lite"
                target="_blank"
                rel="noreferrer"
                className="flex h-11 w-full items-center justify-center gap-2 rounded-lg border border-white/[0.12] bg-[#0f1624] px-5 text-sm font-medium text-zinc-200 transition hover:border-white/[0.25] hover:bg-[#131b2c] hover:text-white sm:w-auto"
              >
                <Github className="size-4" />
                <span>GitHub 开源仓库</span>
              </a>
            </div>

            {/* Quick Run Hint */}
            <div className="mt-6 flex items-center justify-center gap-2 text-xs text-zinc-500">
              <ShieldCheck className="size-3.5 text-emerald-400" />
              <span>零外部 SaaS 依赖 · 本地自托管 · 100% 数据掌控</span>
            </div>
          </div>

          {/* Hero UI Showcase Mockup */}
          <div className="mt-14 sm:mt-16">
            <div className="relative mx-auto max-w-5xl rounded-xl border border-white/[0.12] bg-[#0e1420] p-1.5 shadow-2xl shadow-black/80 sm:p-2.5">
              {/* Fake Window Controls */}
              <div className="flex h-8 items-center justify-between border-b border-white/[0.06] px-3 pb-1">
                <div className="flex items-center gap-2">
                  <span className="size-3 rounded-full bg-rose-500/70" />
                  <span className="size-3 rounded-full bg-amber-500/70" />
                  <span className="size-3 rounded-full bg-emerald-500/70" />
                  <span className="ml-3 font-mono text-[11px] text-zinc-500">
                    gamepanel.local:3005 — 游戏服务器管理平台
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="rounded bg-emerald-500/10 px-2 py-0.5 font-mono text-[10px] text-emerald-400">
                    RUNNING · 7777
                  </span>
                </div>
              </div>

              {/* Main Screenshot Preview */}
              <div className="overflow-hidden rounded-lg border border-white/[0.06] bg-[#080c14]">
                <Image
                  src="/official/interface-servers.png"
                  alt="GamePanel Lite 实例管理与控制台界面"
                  width={1600}
                  height={800}
                  priority
                  className="w-full object-cover transition-transform duration-500 hover:scale-[1.01]"
                />
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Pain Points vs Solutions */}
      <section id="architecture" className="border-b border-white/[0.08] bg-[#0a0e17] py-16 sm:py-20">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="mx-auto max-w-2xl text-center">
            <h2 className="text-xs font-semibold uppercase tracking-wider text-emerald-400">
              为什么选择 GamePanel Lite
            </h2>
            <p className="mt-2 text-3xl font-bold tracking-tight text-white sm:text-4xl">
              告别传统开服痛点，专注游戏本身
            </p>
            <p className="mt-3 text-sm text-zinc-400">
              不再被繁杂的 Docker 命令、崩溃的参数配置文件与丢失的地图存档困扰。
            </p>
          </div>

          <div className="mt-12 grid gap-4 sm:grid-cols-2">
            {painPoints.map((item, idx) => (
              <div
                key={idx}
                className="rounded-xl border border-white/[0.08] bg-[#0e1420] p-5 transition hover:border-emerald-500/30 hover:bg-[#111928]"
              >
                <div className="flex items-start gap-3">
                  <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-rose-500/10 text-xs font-bold text-rose-400">
                    ✕
                  </span>
                  <p className="text-xs leading-5 text-zinc-400">{item.pain}</p>
                </div>
                <div className="mt-4 flex items-start gap-3 border-t border-white/[0.06] pt-3.5">
                  <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-emerald-500/10 text-xs font-bold text-emerald-400">
                    ✓
                  </span>
                  <p className="text-xs font-medium leading-5 text-emerald-300">{item.solution}</p>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Core Capabilities */}
      <section id="features" className="border-b border-white/[0.08] bg-[#080c14] py-16 sm:py-24">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="flex flex-col items-start justify-between gap-4 md:flex-row md:items-end">
            <div>
              <span className="text-xs font-semibold uppercase tracking-wider text-emerald-400">
                Core Capabilities
              </span>
              <h2 className="mt-2 text-3xl font-bold tracking-tight text-white sm:text-4xl">
                为硬核玩家与自托管主机打造
              </h2>
            </div>
            <p className="max-w-md text-sm text-zinc-400">
              专注 Terraria 生态核心体验，不做臃肿功能，只提供稳定、纯粹、好用的服务器运维工具。
            </p>
          </div>

          <div className="mt-12 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
            {capabilities.map((item) => {
              const Icon = item.icon;
              return (
                <div
                  key={item.title}
                  className="group relative flex flex-col justify-between rounded-xl border border-white/[0.08] bg-[#0f1624] p-6 transition-all duration-200 hover:-translate-y-1 hover:border-emerald-500/40 hover:bg-[#131c2e] hover:shadow-xl hover:shadow-black/50"
                >
                  <div>
                    <div className="flex items-center justify-between">
                      <div className="flex size-10 items-center justify-center rounded-lg border border-emerald-500/20 bg-emerald-500/10 text-emerald-400 transition group-hover:border-emerald-500/40 group-hover:bg-emerald-500/20">
                        <Icon className="size-5" />
                      </div>
                      <span className="rounded bg-white/[0.05] px-2 py-0.5 text-[10px] font-medium text-zinc-400">
                        {item.badge}
                      </span>
                    </div>
                    <h3 className="mt-4 text-base font-bold text-white">{item.title}</h3>
                    <p className="mt-2 text-xs leading-relaxed text-zinc-400">{item.description}</p>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </section>

      {/* Interactive UI Showcase */}
      <section id="showcase" className="border-b border-white/[0.08] bg-[#0a0e17] py-16 sm:py-24">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="mx-auto max-w-2xl text-center">
            <h2 className="text-xs font-semibold uppercase tracking-wider text-emerald-400">
              Modern Gaming UI
            </h2>
            <p className="mt-2 text-3xl font-bold tracking-tight text-white sm:text-4xl">
              极简深色控制台，信息一目了然
            </p>
            <p className="mt-3 text-sm text-zinc-400">
              参考 Steam Deck、Linear 与现代暗黑科技界面风格，拒绝花哨渐变与复杂嵌套。
            </p>
          </div>

          {/* Tabs */}
          <div className="mt-10 flex flex-wrap justify-center gap-2">
            {showcases.map((sc, idx) => (
              <button
                key={sc.id}
                type="button"
                onClick={() => setActiveShowcase(idx)}
                className={`rounded-lg px-4 py-2 text-xs font-medium transition ${
                  activeShowcase === idx
                    ? "bg-emerald-500 font-bold text-zinc-950 shadow-md shadow-emerald-500/20"
                    : "border border-white/[0.08] bg-[#0e1420] text-zinc-400 hover:border-white/[0.15] hover:text-white"
                }`}
              >
                {sc.title}
              </button>
            ))}
          </div>

          {/* Active Showcase Card */}
          <div className="mt-8 overflow-hidden rounded-xl border border-white/[0.1] bg-[#0e1420] p-2 sm:p-4 shadow-2xl">
            <div className="mb-3 px-2">
              <h3 className="text-sm font-bold text-white">{showcases[activeShowcase].title}</h3>
              <p className="mt-0.5 text-xs text-zinc-400">{showcases[activeShowcase].desc}</p>
            </div>
            <div className="overflow-hidden rounded-lg border border-white/[0.08] bg-[#080c14]">
              <Image
                src={showcases[activeShowcase].image}
                alt={showcases[activeShowcase].title}
                width={1600}
                height={800}
                className="w-full object-cover"
              />
            </div>
          </div>
        </div>
      </section>

      {/* 3-Step Quickstart */}
      <section id="quickstart" className="border-b border-white/[0.08] bg-[#080c14] py-16 sm:py-24">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="mx-auto max-w-2xl text-center">
            <span className="text-xs font-semibold uppercase tracking-wider text-emerald-400">
              Easy Onboarding
            </span>
            <h2 className="mt-2 text-3xl font-bold tracking-tight text-white sm:text-4xl">
              60 秒极速开启泰拉冒险
            </h2>
            <p className="mt-3 text-sm text-zinc-400">
              无需任何 Linux 运维经验，三步完成属于你和好友的专属私服。
            </p>
          </div>

          <div className="mt-12 grid gap-6 md:grid-cols-3">
            {steps.map((step) => {
              const Icon = step.icon;
              return (
                <div
                  key={step.step}
                  className="relative rounded-xl border border-white/[0.08] bg-[#0e1420] p-6 text-left"
                >
                  <div className="flex items-center justify-between">
                    <div className="flex size-10 items-center justify-center rounded-lg border border-emerald-500/20 bg-emerald-500/10 text-emerald-400">
                      <Icon className="size-5" />
                    </div>
                    <span className="font-mono text-2xl font-black text-emerald-500/30">{step.step}</span>
                  </div>
                  <h3 className="mt-5 text-base font-bold text-white">{step.title}</h3>
                  <p className="mt-2 text-xs leading-relaxed text-zinc-400">{step.desc}</p>
                </div>
              );
            })}
          </div>

          {/* One line Docker Run Command */}
          <div className="mt-12 mx-auto max-w-3xl rounded-xl border border-white/[0.1] bg-[#0e1420] p-4 sm:p-5">
            <div className="flex items-center justify-between text-xs text-zinc-400 mb-2">
              <span className="font-mono text-zinc-300">⚡ 宿主机一行命令快速拉起 GamePanel Lite：</span>
              <button
                type="button"
                onClick={copyDockerCommand}
                className="inline-flex items-center gap-1 rounded bg-white/[0.06] px-2 py-1 text-[11px] font-medium text-zinc-300 hover:bg-white/[0.1] hover:text-white"
              >
                {copied ? <Check className="size-3 text-emerald-400" /> : <Copy className="size-3" />}
                <span>{copied ? "已复制" : "复制命令"}</span>
              </button>
            </div>
            <pre className="overflow-x-auto rounded-lg bg-[#080c14] p-3 font-mono text-xs text-emerald-400 selection:bg-emerald-500/20">
              docker run -d --name gamepanel -p 8080:8080 -v /var/run/docker.sock:/var/run/docker.sock smartcat999/game-panel-lite:latest
            </pre>
          </div>
        </div>
      </section>

      {/* Roadmap */}
      <section id="roadmap" className="border-b border-white/[0.08] bg-[#0a0e17] py-16 sm:py-20">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="mx-auto max-w-2xl text-center">
            <span className="text-xs font-semibold uppercase tracking-wider text-emerald-400">
              Ecosystem Roadmap
            </span>
            <h2 className="mt-2 text-3xl font-bold tracking-tight text-white sm:text-4xl">
              持续演进的开源路线图
            </h2>
            <p className="mt-3 text-sm text-zinc-400">
              立足泰拉瑞亚原版与模组体验，稳步扩展现代自托管游戏服务器生态。
            </p>
          </div>

          <div className="mt-12 grid gap-6 md:grid-cols-3">
            {roadmapItems.map((group) => (
              <div
                key={group.phase}
                className="rounded-xl border border-white/[0.08] bg-[#0e1420] p-6 flex flex-col justify-between"
              >
                <div>
                  <div className="flex items-center justify-between mb-4">
                    <h3 className="font-bold text-white text-sm">{group.phase}</h3>
                    <span className={`rounded-full border px-2.5 py-0.5 text-[10px] font-bold ${group.color}`}>
                      {group.status}
                    </span>
                  </div>
                  <ul className="space-y-2.5">
                    {group.items.map((item, idx) => (
                      <li key={idx} className="flex items-start gap-2 text-xs text-zinc-300">
                        <CircleDot className="size-3.5 mt-0.5 text-emerald-400 shrink-0" />
                        <span>{item}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Bottom CTA */}
      <section className="relative overflow-hidden bg-[#080c14] py-20 text-center">
        <div className="pointer-events-none absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 size-[500px] rounded-full bg-emerald-500/10 blur-[140px]" />
        <div className="relative mx-auto max-w-3xl px-4 sm:px-6 lg:px-8">
          <div className="inline-flex size-12 items-center justify-center rounded-xl border border-emerald-500/30 bg-emerald-500/10 text-emerald-400 mb-6">
            <Sparkles className="size-6" />
          </div>
          <h2 className="text-3xl font-bold tracking-tight text-white sm:text-4xl">
            准备好开启全新的游戏服务器体验了吗？
          </h2>
          <p className="mt-4 text-sm text-zinc-400">
            完全免费开源，由玩家构建，为开黑好友与自托管爱好者而生。
          </p>
          <div className="mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row sm:gap-4">
            <Link
              href="/login"
              className="flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-emerald-500 px-6 font-bold text-zinc-950 shadow-lg shadow-emerald-500/25 transition-all hover:bg-emerald-400 sm:w-auto"
            >
              <span>立即进入控制台</span>
              <ArrowRight className="size-4" />
            </Link>
            <a
              href="https://github.com/smartcat999/game-panel-lite"
              target="_blank"
              rel="noreferrer"
              className="flex h-11 w-full items-center justify-center gap-2 rounded-lg border border-white/[0.1] bg-[#0f1624] px-5 text-sm font-medium text-zinc-200 transition hover:bg-[#131b2c] hover:text-white sm:w-auto"
            >
              <Github className="size-4" />
              <span>给项目点个 Star</span>
            </a>
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="border-t border-white/[0.08] bg-[#06090f] py-10">
        <div className="mx-auto flex max-w-7xl flex-col items-center justify-between gap-6 px-4 sm:flex-row sm:px-6 lg:px-8">
          <div className="flex items-center gap-3">
            <div className="flex size-7 items-center justify-center rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-1">
              <Image src="/avatar.svg" alt="GamePanel" width={20} height={20} className="pixelated" />
            </div>
            <div className="text-left">
              <p className="text-xs font-bold text-white">GamePanel Lite</p>
              <p className="text-[11px] text-zinc-500">基于 MIT 协议开源发布 · 专为 Terraria 打造</p>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-6 text-xs text-zinc-400">
            <a href="#features" className="hover:text-white">特性介绍</a>
            <a href="#architecture" className="hover:text-white">技术架构</a>
            <a href="#roadmap" className="hover:text-white">路线图</a>
            <Link href="/login" className="hover:text-white">控制台登录</Link>
            <a
              href="https://github.com/smartcat999/game-panel-lite"
              target="_blank"
              rel="noreferrer"
              className="flex items-center gap-1 hover:text-white"
            >
              <Github className="size-3.5" />
              GitHub
            </a>
          </div>
        </div>
      </footer>
    </div>
  );
}
