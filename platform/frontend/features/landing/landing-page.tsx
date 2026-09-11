"use client";

import Image from "next/image";
import Link from "next/link";
import {
  ArrowRight,
  BadgeCheck,
  Check,
  ChevronRight,
  CircleDollarSign,
  Cloud,
  Cpu,
  Gamepad2,
  Globe2,
  HardDrive,
  HelpCircle,
  Layers,
  Menu,
  Network,
  Play,
  Rocket,
  Server,
  ShieldCheck,
  Sparkles,
  Terminal,
  Users,
  X,
  Zap,
} from "lucide-react";
import { useState } from "react";

const navLinks = [
  { href: "#features", label: "平台特性" },
  { href: "#regions", label: "全球可用区" },
  { href: "#pricing", label: "规格与方案" },
  { href: "#workspaces", label: "小队协作" },
  { href: "#faq", label: "常见问题" },
];

const saasFeatures = [
  {
    icon: Rocket,
    tag: "零门槛",
    title: "30 秒秒级云端交付",
    description:
      "免去购买云服务器、配安全组、装 Linux 与配 Docker 的繁琐流程。在网页选择游戏与配置，一键秒级拉起高可用实例。",
  },
  {
    icon: Network,
    tag: "超低延迟",
    title: "多可用区电竞专线",
    description:
      "华东、亚太东京、美西等多可用区算力节点。BGP 优质骨干网直连与智能调度，告别跳 ping 与卡顿，联机极致丝滑。",
  },
  {
    icon: Users,
    tag: "多人协同",
    title: "专属小队团队工作区",
    description:
      "告别“一人开服全队当爹”。一键邀请好友加入专属工作区，分配合适权限，队员均可随时通过手机或电脑查看状态、发送指令与一键回档。",
  },
  {
    icon: ShieldCheck,
    tag: "安全容灾",
    title: "云端时光机多副本快照",
    description:
      "世界存档定时自动多副本冷热隔离备份。遇到炸服、意外误操作，秒级一键无损回档，支持世界文件随时导出下载。",
  },
  {
    icon: Gamepad2,
    tag: "专精生态",
    title: "Terraria & 灾厄模组优化",
    description:
      "深度定制原版 1.4.4.9/1.4.5 与 tModLoader 模组服运行环境，内置灾厄 (Calamity) 等主流模组预设，杜绝内存泄漏。",
  },
  {
    icon: CircleDollarSign,
    tag: "按需计费",
    title: "透明计费与弹性规格",
    description:
      "支持按小时或按月灵活结算，新用户注册赠送体验金。玩时开启、不玩休眠保留存档，按需升降配置，开销一目了然。",
  },
];

const painPoints = [
  {
    traditional: "传统自建 VPS：买服务器、配安全组、手动折腾 Docker，动辄数小时，配置错误直接崩溃",
    saas: "GamePanel Cloud：全托管免运维，全网页图形化向导，30 秒一键交付开箱即用",
  },
  {
    traditional: "单人服主运维之痛：只有服主一人有权限，服主不在家谁也进不去、无法重启、无法回档",
    saas: "小队工作区协同：邀请开黑好友加入专属工作区，共享控制台与管理权限，随时随地手机/电脑一键启停",
  },
  {
    traditional: "意外炸服存档丢失：模组冲突或误操作导致地图损坏，没有备份，几百小时建筑心血付之一炬",
    saas: "云端时光机多副本：自动化定时快照与世界完整性校验，秒级一键回档，存档永不丢失",
  },
  {
    traditional: "高昂闲置成本：租整月云主机就算不玩也要扣费；单点机器无法照顾天南地北玩家的延迟",
    saas: "弹性计量与多可用区：按实际游戏时间弹性计费；全国与海外多可用区就近接入，延迟低至 15ms",
  },
];

const regions = [
  {
    id: "cn-shanghai",
    name: "华东一区 (上海)",
    tag: "低延迟首选",
    ping: "< 20ms",
    desc: "国内骨干 BGP 极速网络，全国玩家联机平均延迟低于 20ms，抗抖动低丢包，国内好友开黑首选。",
    status: "运行正常 · 充足算力",
    features: ["BGP 多线智能接入", "DDoS 流量清洗防护", "专属稳定 IPv4/IPv6"],
  },
  {
    id: "ap-northeast-1",
    name: "亚太一区 (东京)",
    tag: "亚太互联",
    ping: "35~60ms",
    desc: "亚太国际骨干网直连，超大出口带宽，适合国内沿海与海外、港澳台好友跨国顺畅组队。",
    status: "运行正常 · 充足算力",
    features: ["国际优质 CN2/BGP 专线", "高并发模组大带宽支持", "跨可用区热备份通道"],
  },
  {
    id: "us-west-1",
    name: "北美一区 (硅谷)",
    tag: "国际节点",
    ping: "120~150ms",
    desc: "北美西海岸高防数据中心，适合留学生与北美社区服部署，提供全天候千兆高防防护。",
    status: "运行正常 · 充足算力",
    features: ["100Gbps+ DDoS 深度防护", "大容量 NVMe 存储池", "全球 Anycast 边缘加速"],
  },
];

const plans = [
  {
    name: "轻量开黑版",
    alias: "Starter",
    badge: "好友入门",
    price: "¥0.35",
    unit: "/ 小时",
    monthPrice: "约 ¥29 / 月",
    desc: "适合 2~6 人好友小队纯净服联机，流畅稳定畅玩原版游戏。",
    specs: [
      "2 核 高频云端 vCPU",
      "4 GB 专属电竞运行内存",
      "20 GB 高速企业级 SSD",
      "独立高防公网直连端口",
      "7 天云端世界快照保留",
      "小队工作区 (最多 3 位成员)",
    ],
    highlight: false,
    cta: "立即开服",
  },
  {
    name: "热血模组版",
    alias: "Standard",
    badge: "最受玩家青睐 · POPULAR",
    price: "¥0.68",
    unit: "/ 小时",
    monthPrice: "约 ¥59 / 月",
    desc: "专为 tModLoader 与大型模组 (如灾厄 Calamity) 深度调优，抗高负载防卡顿。",
    specs: [
      "4 核 极客高频 vCPU",
      "8 GB 专属电竞运行内存",
      "50 GB NVMe 极速世界存储",
      "优先网络带宽与智能调度",
      "自动定时云端时光机多副本",
      "小队工作区 (最多 10 位成员)",
      "一键模组预设与参数调优",
    ],
    highlight: true,
    cta: "畅玩模组开服",
  },
  {
    name: "极客旗舰版",
    alias: "Pro Extreme",
    badge: "社区级旗舰",
    price: "¥1.20",
    unit: "/ 小时",
    monthPrice: "约 ¥99 / 月",
    desc: "适合大型公会、多模组狂热玩家与长期活跃的大型社区服。",
    specs: [
      "8 核 顶配云算力 vCPU",
      "16 GB 大容量极速内存",
      "100 GB NVMe 企业级阵列",
      "最高优先级专属 BGP 带宽",
      "永久异地冷备份与无损回档",
      "小队无限制多人协同管理",
      "专属技术运维顾问通道",
    ],
    highlight: false,
    cta: "开启旗舰服",
  },
];

const faqs = [
  {
    q: "我需要懂 Linux 或自己准备云服务器吗？",
    a: "完全不需要！GamePanel Cloud 是一站式全托管 SaaS 云平台。所有计算资源、Docker 隔离沙箱和高防网络端点均由平台自动化提供。无需任何运维知识，点击鼠标即可开服。",
  },
  {
    q: "我的开黑好友需要注册平台账号才能进服玩游戏吗？",
    a: "不需要！联机玩游戏的朋友完全不需要注册平台。服主或小队成员在控制台一键复制服务器直连 IP 和端口发到微信群/QQ群，好友在游戏客户端内直接加入即可！只有需要共同管理服务器的队员才需要登录工作区。",
  },
  {
    q: "支持 tModLoader 与灾厄 (Calamity) 等模组吗？",
    a: "全面深度支持！平台已预置经过严格测试的 tModLoader 运行环境与热门模组参数预设，并针对灾厄等高消耗模组的内存回收进行了专属底层优化，杜绝卡顿与内存溢出。",
  },
  {
    q: "我的单机世界存档可以上传到云端吗？中途不玩了存档会丢吗？",
    a: "支持！控制台提供一键上传 `.wld` 世界地图功能，你可以无缝把本地单机存档迁移到云端与好友继续冒险。即使服务器停机休眠，世界存档与时光机快照也会持久安全封存，支持随时一键下载到本地备份。",
  },
  {
    q: "计费模式是怎样的？有没有隐形消费？",
    a: "平台采用透明点券钱包计费，支持按小时计费。不玩时可以随时停止实例，停止期间只占用极低的世界存储费用，绝不收取高额空转计算费。新用户注册即可领取免费开服体验金，零风险试玩。",
  },
];

export function LandingPage() {
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [openFaq, setOpenFaq] = useState<number | null>(0);

  return (
    <div className="min-h-screen bg-[#080c14] text-zinc-100 selection:bg-emerald-500/30 selection:text-emerald-300">
      {/* Top Navigation */}
      <header className="sticky top-0 z-50 border-b border-white/[0.08] bg-[#080c14]/90 backdrop-blur-md">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-4 sm:px-6 lg:px-8">
          <Link href="/" className="flex items-center gap-3">
            <div className="relative flex size-9 items-center justify-center overflow-hidden rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-1 shadow-sm shadow-emerald-500/20">
              <Image
                src="/avatar.svg"
                alt="GamePanel Cloud Logo"
                width={32}
                height={32}
                className="pixelated object-contain"
              />
            </div>
            <div className="flex flex-col">
              <div className="flex items-center gap-2">
                <span className="font-bold tracking-tight text-white">GamePanel</span>
                <span className="rounded bg-emerald-500/20 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-400">
                  CLOUD SaaS
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
            <Link
              href="/login"
              className="inline-flex h-9 items-center rounded-lg border border-white/[0.1] bg-[#0f1624] px-4 text-xs font-medium text-zinc-300 transition hover:border-white/[0.2] hover:bg-[#131b2c] hover:text-white"
            >
              登录控制台
            </Link>
            <Link
              href="/login"
              className="inline-flex h-9 items-center gap-1.5 rounded-lg bg-emerald-500 px-4 text-xs font-bold text-zinc-950 shadow-md shadow-emerald-500/25 transition-all hover:bg-emerald-400 active:scale-[0.98]"
            >
              <Sparkles className="size-3.5" />
              <span>立即开服体验</span>
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
                  立即开服体验 <ArrowRight className="size-4" />
                </Link>
                <Link
                  href="/login"
                  className="flex h-10 items-center justify-center gap-2 rounded-lg border border-white/[0.1] bg-[#0f1624] text-xs font-medium text-zinc-300"
                  onClick={() => setMobileMenuOpen(false)}
                >
                  登录控制台
                </Link>
              </div>
            </nav>
          </div>
        )}
      </header>

      {/* Hero Section */}
      <section className="relative overflow-hidden border-b border-white/[0.08] pt-12 pb-20 sm:pt-16 sm:pb-24 lg:pt-20">
        {/* Glow Effects */}
        <div className="pointer-events-none absolute left-1/2 top-0 -translate-x-1/2 -translate-y-1/2 size-[650px] rounded-full bg-emerald-500/10 blur-[140px]" />
        <div className="pointer-events-none absolute right-10 top-1/3 size-[400px] rounded-full bg-sky-500/5 blur-[130px]" />

        <div className="relative mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="mx-auto max-w-3xl text-center">
            {/* Slogan Badge */}
            <div className="inline-flex items-center gap-2 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-3.5 py-1 text-xs font-medium text-emerald-400">
              <span className="size-1.5 rounded-full bg-emerald-400 animate-pulse" />
              <span>新一代游戏云服托管平台 · 注册工作区立享开服点券</span>
            </div>

            {/* Main Headline */}
            <h1 className="mt-6 text-balance text-4xl font-extrabold tracking-tight text-white sm:text-5xl lg:text-6xl">
              开启专属游戏私服，
              <br />
              <span className="bg-gradient-to-r from-emerald-400 via-teal-300 to-sky-400 bg-clip-text text-transparent">
                告别买机运维与复杂配置。
              </span>
            </h1>

            {/* Subheading */}
            <p className="mt-6 text-pretty text-base text-zinc-400 sm:text-lg sm:leading-8">
              专为泰拉瑞亚与联机玩家打造的全托管游戏云平台。
              <br className="hidden sm:inline" />
              免买服务器，免配 Docker，30 秒秒级交付。多可用区电竞专线、小队多人工作区协同与云端时光机容灾快照。
            </p>

            {/* Action Buttons */}
            <div className="mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row sm:gap-4">
              <Link
                href="/login"
                className="flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-emerald-500 px-6 font-bold text-zinc-950 shadow-lg shadow-emerald-500/25 transition-all hover:bg-emerald-400 hover:shadow-emerald-500/35 sm:w-auto active:scale-[0.98]"
              >
                <span>免费开始开服体验</span>
                <ChevronRight className="size-4" />
              </Link>
              <a
                href="#pricing"
                className="flex h-11 w-full items-center justify-center gap-2 rounded-lg border border-white/[0.12] bg-[#0f1624] px-5 text-sm font-medium text-zinc-200 transition hover:border-white/[0.25] hover:bg-[#131b2c] hover:text-white sm:w-auto"
              >
                <CircleDollarSign className="size-4 text-emerald-400" />
                <span>查看配置与定价</span>
              </a>
            </div>

            {/* Metric Proofs */}
            <div className="mt-8 grid grid-cols-2 gap-4 border-t border-white/[0.06] pt-6 sm:grid-cols-4">
              <div className="flex flex-col items-center">
                <span className="font-mono text-2xl font-extrabold text-white">⚡ 30s</span>
                <span className="mt-1 text-xs text-zinc-500">秒级云端实例交付</span>
              </div>
              <div className="flex flex-col items-center">
                <span className="font-mono text-2xl font-extrabold text-emerald-400">&lt; 20ms</span>
                <span className="mt-1 text-xs text-zinc-500">骨干专线超低延迟</span>
              </div>
              <div className="flex flex-col items-center">
                <span className="font-mono text-2xl font-extrabold text-white">99.9%</span>
                <span className="mt-1 text-xs text-zinc-500">云端高可用保障</span>
              </div>
              <div className="flex flex-col items-center">
                <span className="font-mono text-2xl font-extrabold text-sky-400">Team</span>
                <span className="mt-1 text-xs text-zinc-500">多人小队工作区协同</span>
              </div>
            </div>
          </div>

          {/* Hero UI Showcase Mockup */}
          <div className="mt-12 sm:mt-14">
            <div className="relative mx-auto max-w-5xl rounded-xl border border-white/[0.12] bg-[#0e1420] p-1.5 shadow-2xl shadow-black/80 sm:p-2.5">
              {/* Fake Window Controls */}
              <div className="flex h-8 items-center justify-between border-b border-white/[0.06] px-3 pb-1">
                <div className="flex items-center gap-2">
                  <span className="size-3 rounded-full bg-rose-500/70" />
                  <span className="size-3 rounded-full bg-amber-500/70" />
                  <span className="size-3 rounded-full bg-emerald-500/70" />
                  <span className="ml-3 font-mono text-[11px] text-zinc-500">
                    gamepanel.cloud — Ember Realms 小队专属工作区
                  </span>
                </div>
                <div className="flex items-center gap-3">
                  <span className="font-mono text-[11px] text-emerald-400">
                    华东一区 · 2 实例运行中
                  </span>
                  <span className="rounded bg-white/[0.06] px-2 py-0.5 font-mono text-[10px] text-zinc-400">
                    点券余额: ¥128.50
                  </span>
                </div>
              </div>

              {/* Main Screenshot Preview */}
              <div className="overflow-hidden rounded-lg border border-white/[0.06] bg-[#080c14]">
                <Image
                  src="/official/interface-servers.png"
                  alt="GamePanel Cloud 游戏云服多实例控制台"
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

      {/* Pain Points: VPS vs SaaS */}
      <section className="border-b border-white/[0.08] bg-[#0a0e17] py-16 sm:py-20">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="mx-auto max-w-2xl text-center">
            <h2 className="text-xs font-semibold uppercase tracking-wider text-emerald-400">
              SaaS 全托管 vs 传统自建主机
            </h2>
            <p className="mt-2 text-3xl font-bold tracking-tight text-white sm:text-4xl">
              为什么更多玩家选择 GamePanel Cloud？
            </p>
            <p className="mt-3 text-sm text-zinc-400">
              把运维难题留给平台，把纯粹的快乐留给联机开黑。
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
                  <p className="text-xs leading-5 text-zinc-400">{item.traditional}</p>
                </div>
                <div className="mt-4 flex items-start gap-3 border-t border-white/[0.06] pt-3.5">
                  <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-emerald-500/10 text-xs font-bold text-emerald-400">
                    ✓
                  </span>
                  <p className="text-xs font-medium leading-5 text-emerald-300">{item.saas}</p>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Core SaaS Features */}
      <section id="features" className="border-b border-white/[0.08] bg-[#080c14] py-16 sm:py-24">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="flex flex-col items-start justify-between gap-4 md:flex-row md:items-end">
            <div>
              <span className="text-xs font-semibold uppercase tracking-wider text-emerald-400">
                Platform Capabilities
              </span>
              <h2 className="mt-2 text-3xl font-bold tracking-tight text-white sm:text-4xl">
                为开黑小队量身定制的云平台
              </h2>
            </div>
            <p className="max-w-md text-sm text-zinc-400">
              告别粗糙的管理模板，享受如 Linear 与 Steam Deck 般精致流畅的高性能云上体验。
            </p>
          </div>

          <div className="mt-12 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
            {saasFeatures.map((item) => {
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
                        {item.tag}
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

      {/* Multi-Region Global Edge */}
      <section id="regions" className="border-b border-white/[0.08] bg-[#0a0e17] py-16 sm:py-24">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="mx-auto max-w-2xl text-center">
            <span className="text-xs font-semibold uppercase tracking-wider text-emerald-400">
              Multi-Region Architecture
            </span>
            <p className="mt-2 text-3xl font-bold tracking-tight text-white sm:text-4xl">
              全球多可用区电竞专线
            </p>
            <p className="mt-3 text-sm text-zinc-400">
              算力节点分布式下沉部署，就近分配独立高防端口与公网直连地址，告别丢包与跳 ping。
            </p>
          </div>

          <div className="mt-12 grid gap-6 md:grid-cols-3">
            {regions.map((rg) => (
              <div
                key={rg.id}
                className="rounded-xl border border-white/[0.08] bg-[#0e1420] p-6 flex flex-col justify-between hover:border-emerald-500/30 transition"
              >
                <div>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <Globe2 className="size-4 text-emerald-400" />
                      <h3 className="font-bold text-white text-base">{rg.name}</h3>
                    </div>
                    <span className="rounded bg-emerald-500/10 border border-emerald-500/20 px-2 py-0.5 text-[10px] font-bold text-emerald-400">
                      {rg.tag}
                    </span>
                  </div>

                  <div className="mt-4 flex items-baseline gap-2">
                    <span className="font-mono text-2xl font-black text-white">{rg.ping}</span>
                    <span className="text-xs text-zinc-500">平均联机延迟</span>
                  </div>

                  <p className="mt-3 text-xs leading-relaxed text-zinc-400">{rg.desc}</p>

                  <ul className="mt-5 space-y-2 border-t border-white/[0.06] pt-4">
                    {rg.features.map((feat, i) => (
                      <li key={i} className="flex items-center gap-2 text-xs text-zinc-300">
                        <Check className="size-3.5 text-emerald-400" />
                        <span>{feat}</span>
                      </li>
                    ))}
                  </ul>
                </div>

                <div className="mt-6 flex items-center gap-2 border-t border-white/[0.06] pt-3 text-[11px] text-zinc-500">
                  <span className="size-1.5 rounded-full bg-emerald-400 animate-pulse" />
                  <span>{rg.status}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Plan Tiers & Pricing */}
      <section id="pricing" className="border-b border-white/[0.08] bg-[#080c14] py-16 sm:py-24">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="mx-auto max-w-2xl text-center">
            <span className="text-xs font-semibold uppercase tracking-wider text-emerald-400">
              Flexible & Transparent Pricing
            </span>
            <h2 className="mt-2 text-3xl font-bold tracking-tight text-white sm:text-4xl">
              灵活计费，按需畅玩
            </h2>
            <p className="mt-3 text-sm text-zinc-400">
              支持按小时弹性扣费与按月包周期。玩时开启、不玩休眠保留存档，新用户赠送体验金。
            </p>
          </div>

          <div className="mt-12 grid gap-6 lg:grid-cols-3">
            {plans.map((p) => (
              <div
                key={p.name}
                className={`relative flex flex-col justify-between rounded-2xl border p-7 transition ${
                  p.highlight
                    ? "border-emerald-500/50 bg-[#0e1624] shadow-2xl shadow-emerald-500/10 ring-1 ring-emerald-500/30"
                    : "border-white/[0.08] bg-[#0e1420] hover:border-white/[0.15]"
                }`}
              >
                {p.highlight && (
                  <div className="absolute -top-3 left-1/2 -translate-x-1/2 rounded-full bg-emerald-500 px-3 py-0.5 text-[10px] font-black tracking-wider text-zinc-950 uppercase shadow-md shadow-emerald-500/20">
                    {p.badge}
                  </div>
                )}

                <div>
                  <div className="flex items-center justify-between">
                    <div>
                      <h3 className="text-lg font-bold text-white">{p.name}</h3>
                      <p className="font-mono text-xs text-zinc-500">{p.alias}</p>
                    </div>
                    {!p.highlight && (
                      <span className="rounded bg-white/[0.05] px-2 py-0.5 text-[10px] text-zinc-400">
                        {p.badge}
                      </span>
                    )}
                  </div>

                  <div className="mt-5 flex items-baseline gap-1">
                    <span className="font-mono text-4xl font-extrabold text-white">{p.price}</span>
                    <span className="text-xs text-zinc-400">{p.unit}</span>
                    <span className="ml-2 font-mono text-xs text-zinc-500">({p.monthPrice})</span>
                  </div>

                  <p className="mt-3 text-xs text-zinc-400 leading-relaxed">{p.desc}</p>

                  <div className="my-6 border-t border-white/[0.06]" />

                  <ul className="space-y-3">
                    {p.specs.map((spec, idx) => (
                      <li key={idx} className="flex items-center gap-2.5 text-xs text-zinc-300">
                        <BadgeCheck className="size-4 shrink-0 text-emerald-400" />
                        <span>{spec}</span>
                      </li>
                    ))}
                  </ul>
                </div>

                <div className="mt-8">
                  <Link
                    href="/login"
                    className={`flex h-10 w-full items-center justify-center gap-2 rounded-lg text-xs font-bold transition ${
                      p.highlight
                        ? "bg-emerald-500 text-zinc-950 hover:bg-emerald-400 shadow-md shadow-emerald-500/20"
                        : "border border-white/[0.1] bg-[#121927] text-zinc-200 hover:border-white/[0.2] hover:text-white"
                    }`}
                  >
                    <span>{p.cta}</span>
                    <ArrowRight className="size-3.5" />
                  </Link>
                </div>
              </div>
            ))}
          </div>

          <div className="mt-8 text-center text-xs text-zinc-500">
            💡 所有方案均包含免费基础备份空间、DDoS 流量清洗与全功能实时 Web 控制台。随时支持无损升配。
          </div>
        </div>
      </section>

      {/* Team Workspace Section */}
      <section id="workspaces" className="border-b border-white/[0.08] bg-[#0a0e17] py-16 sm:py-24">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="grid gap-12 lg:grid-cols-2 lg:items-center">
            <div>
              <span className="text-xs font-semibold uppercase tracking-wider text-emerald-400">
                Team Workspaces
              </span>
              <h2 className="mt-2 text-3xl font-bold tracking-tight text-white sm:text-4xl">
                专属小队空间，开黑不再靠一人
              </h2>
              <p className="mt-4 text-sm leading-relaxed text-zinc-400">
                无论是 3 人的核心好友开黑，还是几十人的游戏公会，GamePanel Cloud 的工作区机制都能让管理变得前所未有的轻松。
              </p>

              <div className="mt-8 space-y-4">
                <div className="flex items-start gap-3 rounded-lg border border-white/[0.06] bg-[#0e1420] p-4">
                  <div className="flex size-8 shrink-0 items-center justify-center rounded bg-emerald-500/10 text-emerald-400">
                    <Users className="size-4" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white">一键邀请开黑好友</h4>
                    <p className="mt-1 text-xs text-zinc-400">
                      生成专属工作区邀请链接，队员点击即可快速入驻，免去繁杂注册流程。
                    </p>
                  </div>
                </div>

                <div className="flex items-start gap-3 rounded-lg border border-white/[0.06] bg-[#0e1420] p-4">
                  <div className="flex size-8 shrink-0 items-center justify-center rounded bg-emerald-500/10 text-emerald-400">
                    <ShieldCheck className="size-4" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white">细粒度角色与操作审计</h4>
                    <p className="mt-1 text-xs text-zinc-400">
                      拥有者、管理员与普通运维分级权限，谁启停了实例、谁执行了指令，全程可追溯。
                    </p>
                  </div>
                </div>

                <div className="flex items-start gap-3 rounded-lg border border-white/[0.06] bg-[#0e1420] p-4">
                  <div className="flex size-8 shrink-0 items-center justify-center rounded bg-emerald-500/10 text-emerald-400">
                    <Terminal className="size-4" />
                  </div>
                  <div>
                    <h4 className="text-sm font-bold text-white">全平台随时随地管理</h4>
                    <p className="mt-1 text-xs text-zinc-400">
                      响应式深色界面完美适配手机、平板与电脑浏览器，不在电脑前也能手机一键回档与重启。
                    </p>
                  </div>
                </div>
              </div>
            </div>

            {/* Visual Preview */}
            <div className="rounded-xl border border-white/[0.1] bg-[#0e1420] p-3 shadow-2xl">
              <div className="overflow-hidden rounded-lg border border-white/[0.08] bg-[#080c14]">
                <Image
                  src="/official/interface-dashboard.png"
                  alt="小队工作区与看板界面"
                  width={1400}
                  height={800}
                  className="w-full object-cover"
                />
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* 30s Cloud Onboarding */}
      <section className="border-b border-white/[0.08] bg-[#080c14] py-16 sm:py-24">
        <div className="mx-auto max-w-7xl px-4 sm:px-6 lg:px-8">
          <div className="mx-auto max-w-2xl text-center">
            <span className="text-xs font-semibold uppercase tracking-wider text-emerald-400">
              Simple 3-Step Flow
            </span>
            <h2 className="mt-2 text-3xl font-bold tracking-tight text-white sm:text-4xl">
              30 秒快速开启云端泰拉之旅
            </h2>
            <p className="mt-3 text-sm text-zinc-400">
              三步极速交付，拉上好友，随时开局。
            </p>
          </div>

          <div className="mt-12 grid gap-6 md:grid-cols-3">
            <div className="rounded-xl border border-white/[0.08] bg-[#0e1420] p-6 text-left">
              <div className="flex items-center justify-between">
                <div className="flex size-10 items-center justify-center rounded-lg border border-emerald-500/20 bg-emerald-500/10 text-emerald-400">
                  <Sparkles className="size-5" />
                </div>
                <span className="font-mono text-2xl font-black text-emerald-500/30">01</span>
              </div>
              <h3 className="mt-5 text-base font-bold text-white">登录并创建小队工作区</h3>
              <p className="mt-2 text-xs leading-relaxed text-zinc-400">
                进入控制台，自动为您分配专属工作区，系统即刻发放初始开服体验点券。
              </p>
            </div>

            <div className="rounded-xl border border-white/[0.08] bg-[#0e1420] p-6 text-left">
              <div className="flex items-center justify-between">
                <div className="flex size-10 items-center justify-center rounded-lg border border-emerald-500/20 bg-emerald-500/10 text-emerald-400">
                  <Globe2 className="size-5" />
                </div>
                <span className="font-mono text-2xl font-black text-emerald-500/30">02</span>
              </div>
              <h3 className="mt-5 text-base font-bold text-white">挑选游戏、可用区与规格</h3>
              <p className="mt-2 text-xs leading-relaxed text-zinc-400">
                选择原版或 tModLoader 模组服，挑选就近低延迟区域（如华东、东京）与计算配置。
              </p>
            </div>

            <div className="rounded-xl border border-white/[0.08] bg-[#0e1420] p-6 text-left">
              <div className="flex items-center justify-between">
                <div className="flex size-10 items-center justify-center rounded-lg border border-emerald-500/20 bg-emerald-500/10 text-emerald-400">
                  <Play className="size-5" />
                </div>
                <span className="font-mono text-2xl font-black text-emerald-500/30">03</span>
              </div>
              <h3 className="mt-5 text-base font-bold text-white">秒级交付，复制直连开黑</h3>
              <p className="mt-2 text-xs leading-relaxed text-zinc-400">
                30 秒内容器就绪，一键复制高防 IP 与端口分享到开黑群，打开游戏立刻畅玩！
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* FAQ */}
      <section id="faq" className="border-b border-white/[0.08] bg-[#0a0e17] py-16 sm:py-24">
        <div className="mx-auto max-w-4xl px-4 sm:px-6 lg:px-8">
          <div className="text-center">
            <span className="text-xs font-semibold uppercase tracking-wider text-emerald-400">
              Frequently Asked Questions
            </span>
            <h2 className="mt-2 text-3xl font-bold tracking-tight text-white sm:text-4xl">
              常见问题解答
            </h2>
            <p className="mt-3 text-sm text-zinc-400">
              关于云平台使用、计费、模组与存档的疑问，在这里找到答案。
            </p>
          </div>

          <div className="mt-12 space-y-4">
            {faqs.map((faq, idx) => (
              <div
                key={idx}
                className="overflow-hidden rounded-xl border border-white/[0.08] bg-[#0e1420] transition"
              >
                <button
                  type="button"
                  onClick={() => setOpenFaq(openFaq === idx ? null : idx)}
                  className="flex w-full items-center justify-between p-5 text-left text-sm font-bold text-white hover:text-emerald-400 transition"
                >
                  <span className="flex items-center gap-2.5">
                    <HelpCircle className="size-4 text-emerald-400 shrink-0" />
                    <span>{faq.q}</span>
                  </span>
                  <ChevronRight
                    className={`size-4 text-zinc-400 transition-transform ${
                      openFaq === idx ? "rotate-90 text-emerald-400" : ""
                    }`}
                  />
                </button>
                {openFaq === idx && (
                  <div className="border-t border-white/[0.06] bg-[#090d16] p-5 text-xs leading-relaxed text-zinc-400">
                    {faq.a}
                  </div>
                )}
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
            <Cloud className="size-6" />
          </div>
          <h2 className="text-3xl font-bold tracking-tight text-white sm:text-4xl">
            准备好开启属于你和小队的专属私服了吗？
          </h2>
          <p className="mt-4 text-sm text-zinc-400">
            立即创建工作区，领取新用户开服点券，体验 30 秒一键畅玩的极速快感。
          </p>
          <div className="mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row sm:gap-4">
            <Link
              href="/login"
              className="flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-emerald-500 px-6 font-bold text-zinc-950 shadow-lg shadow-emerald-500/25 transition-all hover:bg-emerald-400 sm:w-auto"
            >
              <span>立即免费开始开服</span>
              <ArrowRight className="size-4" />
            </Link>
            <a
              href="#pricing"
              className="flex h-11 w-full items-center justify-center gap-2 rounded-lg border border-white/[0.1] bg-[#0f1624] px-5 text-sm font-medium text-zinc-200 transition hover:bg-[#131b2c] hover:text-white sm:w-auto"
            >
              <span>查看全部规格方案</span>
            </a>
          </div>
        </div>
      </section>

      {/* Footer */}
      <footer className="border-t border-white/[0.08] bg-[#06090f] py-10">
        <div className="mx-auto flex max-w-7xl flex-col items-center justify-between gap-6 px-4 sm:flex-row sm:px-6 lg:px-8">
          <div className="flex items-center gap-3">
            <div className="flex size-7 items-center justify-center rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-1">
              <Image src="/avatar.svg" alt="GamePanel Cloud" width={20} height={20} className="pixelated" />
            </div>
            <div className="text-left">
              <p className="text-xs font-bold text-white">GamePanel Cloud</p>
              <p className="text-[11px] text-zinc-500">新一代游戏云服务器托管平台 · 专为开黑玩家打造</p>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-6 text-xs text-zinc-400">
            <a href="#features" className="hover:text-white">平台特性</a>
            <a href="#regions" className="hover:text-white">全球节点</a>
            <a href="#pricing" className="hover:text-white">方案定价</a>
            <a href="#workspaces" className="hover:text-white">小队协作</a>
            <a href="#faq" className="hover:text-white">常见问题</a>
            <Link href="/login" className="text-emerald-400 hover:text-emerald-300 font-semibold">控制台登录</Link>
          </div>
        </div>
      </footer>
    </div>
  );
}
