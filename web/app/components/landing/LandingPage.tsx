"use client";

import Image from "next/image";
import { motion, useReducedMotion } from "motion/react";
import { formatAppCategory } from "../../console/lib/presets";
import LogoMark from "./LogoMark";
import VoxelIsland from "./VoxelIsland";

/*
 * Design tokens (Option A, "voxel island")
 *  sky-top   #C9D4FF  sky-bottom #F5F7FD  (hero gradient only)
 *  cobalt    #2B3AE8  (the one accent: primary buttons, numerals, highlights)
 *  ink       #14161F  muted #5B6075  line #E3E6F2  surface #FFFFFF  page #F5F7FD
 *  shape rule: interactive elements are full pills; content panels are 28px.
 */

export interface LandingGame {
  id: string;
  name: string;
  category: string;
  tagline?: string;
  coverImage?: string;
  cpu: string;
  memory: string;
}

export interface LandingPlan {
  id: string;
  label: string;
  cpu: string;
  memory: string;
  storage: string;
  desc: string;
}

interface LandingPageProps {
  isAuthed: boolean;
  games: LandingGame[];
  plans: LandingPlan[];
  onLogin: () => void;
  onEnterConsole: () => void;
  onDeploy: (gameId?: string, planId?: string) => void;
}

const ISLAND_LABELS = [
  { name: "palworld-01", meta: "28/32" },
  { name: "minecraft", meta: "42/64" },
  { name: "zomboid", meta: "12/16" },
  { name: "terraria", meta: "6/8" },
];

const STEPS = [
  { title: "选一款游戏", body: "服务端和推荐配置都已备好，不用自己装。" },
  { title: "按人数选配置", body: "从 2 人小队到百人社区，之后也能随时升级。" },
  { title: "把地址发给好友", body: "开服后拿到专属地址，好友在游戏里直接加入。" },
];

const RECOMMENDED_PLAN = "performance";
const GITHUB_REPO_URL = "https://github.com/smartcat999/game-panel-lite";

function GitHubIcon({ className = "h-4 w-4" }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
      <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12" />
    </svg>
  );
}

function formatMem(mem: string) {
  return mem.replace("Gi", " GB");
}

const focusRing =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#2B3AE8] focus-visible:ring-offset-2 focus-visible:ring-offset-white";

export default function LandingPage({
  isAuthed,
  games,
  plans,
  onLogin,
  onEnterConsole,
  onDeploy,
}: LandingPageProps) {
  const reduce = useReducedMotion();
  const primaryAction = () => (isAuthed ? onEnterConsole() : onLogin());

  const scrollToGames = () => {
    document.getElementById("games")?.scrollIntoView({ behavior: reduce ? "auto" : "smooth" });
  };

  const intro = (delay: number) =>
    reduce
      ? {}
      : {
          initial: { opacity: 0, y: 18 },
          animate: { opacity: 1, y: 0 },
          transition: { duration: 0.7, delay, ease: [0.16, 1, 0.3, 1] as const },
        };

  return (
    <div className="min-h-[100dvh] bg-[#F5F7FD] text-[#14161F] antialiased">
      {/* ------------------------------------------------------------ nav */}
      <header className="fixed inset-x-0 top-4 z-40 px-4">
        <div className="mx-auto flex h-14 max-w-[1240px] items-center justify-between gap-2 rounded-full border border-white/70 bg-white/70 pl-3.5 pr-2 shadow-[0_8px_30px_-12px_rgba(43,58,232,0.25)] backdrop-blur-xl sm:pl-4">
          <button
            type="button"
            onClick={() => window.scrollTo({ top: 0, behavior: reduce ? "auto" : "smooth" })}
            className={`flex shrink-0 items-center gap-2 whitespace-nowrap rounded-full text-[15px] font-semibold tracking-tight sm:gap-2.5 ${focusRing}`}
          >
            <LogoMark />
            <span>GamePanel Lite</span>
          </button>
          <div className="flex shrink-0 items-center gap-1 sm:gap-2">
            <a
              href={GITHUB_REPO_URL}
              target="_blank"
              rel="noopener noreferrer"
              aria-label="GitHub 开源仓库"
              className={`inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-full p-2 text-sm font-medium text-[#3D4258] transition-colors hover:bg-white hover:text-[#14161F] sm:px-3.5 sm:py-2 ${focusRing}`}
            >
              <GitHubIcon className="h-4 w-4 shrink-0" />
              <span className="hidden sm:inline">GitHub</span>
            </a>
            <button
              type="button"
              onClick={primaryAction}
              className={`shrink-0 whitespace-nowrap rounded-full border border-[#14161F]/80 px-4 py-2 text-sm font-medium transition-colors hover:bg-[#14161F] hover:text-white active:scale-[0.98] sm:px-5 ${focusRing}`}
            >
              {isAuthed ? "进入控制台" : "登录"}
            </button>
          </div>
        </div>
      </header>

      <main>
        {/* ---------------------------------------------------------- hero */}
        <section className="relative overflow-hidden bg-[linear-gradient(180deg,#C9D4FF_0%,#DCE3FF_45%,#F5F7FD_100%)]">
          <div className="mx-auto grid min-h-[100dvh] max-w-[1240px] grid-cols-1 items-center gap-4 px-6 pb-10 pt-28 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:pt-20">
            <div className="relative z-10 max-w-[520px]">
              <motion.h1
                {...intro(0.15)}
                className="text-[44px] font-bold leading-[1.1] tracking-tight sm:text-[56px] lg:text-[64px]"
              >
                开一台游戏服，
                <br />
                只要十秒
              </motion.h1>
              <motion.p {...intro(0.3)} className="mt-5 text-lg leading-relaxed text-[#3D4258]">
                幻兽帕鲁、Minecraft、泰拉瑞亚等热门游戏一键开服，存档自动备份。
              </motion.p>
              <motion.div {...intro(0.45)} className="mt-9">
                <button
                  type="button"
                  onClick={() => onDeploy(games[0]?.id)}
                  className={`rounded-full bg-[#2B3AE8] px-9 py-4 text-base font-semibold text-white shadow-[0_14px_30px_-12px_rgba(43,58,232,0.8)] transition-[transform,background-color] hover:-translate-y-0.5 hover:bg-[#2230D0] active:translate-y-0 active:scale-[0.98] ${focusRing}`}
                >
                  立即开服
                </button>
              </motion.div>
            </div>

            <div className="relative h-[440px] sm:h-[540px] lg:h-[700px] lg:-mr-10 [mask-image:linear-gradient(to_bottom,#000_82%,transparent)]">
              <VoxelIsland labels={ISLAND_LABELS} onSelect={scrollToGames} />
            </div>
          </div>
        </section>

        {/* --------------------------------------------------------- games */}
        <section id="games" className="mx-auto max-w-[1240px] scroll-mt-24 px-6 pb-28 pt-10">
          <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">选一款游戏</h2>
          <p className="mt-3 max-w-[46ch] text-base text-[#5B6075]">
            每款都预装好服务端和推荐配置，点一下就能开。
          </p>

          <div className="-mx-6 mt-10 grid snap-x snap-mandatory auto-cols-[72%] grid-flow-col gap-5 overflow-x-auto px-6 pb-4 sm:auto-cols-[42%] lg:mx-0 lg:grid-flow-row lg:grid-cols-5 lg:overflow-visible lg:px-0">
            {games.map((game) => {
              const title = game.name;
              const players = game.tagline || formatAppCategory(game.category);
              const coverSrc = game.coverImage || `/landing/games/${game.id}.jpg`;
              return (
                <button
                  key={game.id}
                  type="button"
                  onClick={() => onDeploy(game.id)}
                  className={`group relative aspect-[3/4] snap-start overflow-hidden rounded-[28px] bg-[#DCE3FF] text-left shadow-[0_18px_40px_-24px_rgba(20,22,31,0.5)] transition-transform duration-300 hover:-translate-y-1 ${focusRing}`}
                >
                  <Image
                    src={coverSrc}
                    alt=""
                    fill
                    sizes="(min-width: 1024px) 240px, (min-width: 640px) 42vw, 72vw"
                    className="object-cover transition-transform duration-500 group-hover:scale-[1.05]"
                  />
                  <div className="absolute inset-0 bg-[linear-gradient(180deg,transparent_45%,rgba(10,12,24,0.78)_100%)]" />
                  <div className="absolute inset-x-0 bottom-0 p-5 text-white">
                    <div className="text-xl font-bold">{title}</div>
                    <div className="mt-1 text-sm text-white/80">{players}</div>
                    <div className="mt-3 text-xs text-white/70">
                      推荐 {game.cpu} 核 {formatMem(game.memory)}
                    </div>
                  </div>
                </button>
              );
            })}
          </div>
        </section>

        {/* --------------------------------------------------------- steps */}
        <section className="border-y border-[#E3E6F2] bg-white">
          <div className="mx-auto max-w-[1240px] px-6 py-24">
            <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">三步开服</h2>
            <ol className="relative mt-14 grid gap-12 md:grid-cols-3 md:gap-10">
              <div
                aria-hidden="true"
                className="absolute left-6 right-[calc(33%-1.5rem)] top-6 hidden border-t-2 border-dashed border-[#C9D0F5] md:block"
              />
              {STEPS.map((step, i) => (
                <li key={step.title} className="relative">
                  <div className="relative flex h-12 w-12 items-center justify-center rounded-full bg-[#2B3AE8] text-lg font-bold text-white ring-8 ring-white">
                    {i + 1}
                  </div>
                  <h3 className="mt-6 text-xl font-semibold">{step.title}</h3>
                  <p className="mt-2 max-w-[34ch] text-[15px] leading-relaxed text-[#5B6075]">{step.body}</p>
                </li>
              ))}
            </ol>
          </div>
        </section>

        {/* ------------------------------------------------------ features */}
        <section className="mx-auto max-w-[1240px] px-6 py-28">
          <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">开完服，剩下的交给我们</h2>

          <div className="mt-12 grid gap-5 lg:grid-cols-12 lg:grid-rows-2">
            <article className="relative flex min-h-[420px] flex-col overflow-hidden rounded-[28px] bg-[#CDDAFD] lg:col-span-7 lg:row-span-2">
              <div className="relative z-10 p-8 sm:p-10">
                <h3 className="text-2xl font-bold sm:text-[28px]">存档自动备份，误删也能找回</h3>
                <p className="mt-3 max-w-[40ch] text-base leading-relaxed text-[#3D4258]">
                  定时把世界存档备份到云端，想回到哪个时间点，一键就能恢复。
                </p>
              </div>
              <div className="relative mt-auto h-[300px] sm:h-[380px]">
                <Image
                  src="/landing/features/backup.jpg"
                  alt="打开的宝箱和三个漂浮的存档方块"
                  fill
                  sizes="(min-width: 1024px) 680px, 100vw"
                  className="object-cover object-[50%_60%]"
                />
              </div>
            </article>

            <article className="relative flex items-center gap-6 overflow-hidden rounded-[28px] bg-[#C9D7FA] p-6 sm:p-8 lg:col-span-5">
              <div className="relative h-28 w-28 shrink-0 sm:h-36 sm:w-36">
                <Image
                  src="/landing/features/console.jpg"
                  alt="发着蓝光的小电脑"
                  fill
                  sizes="144px"
                  className="scale-[1.35] object-cover [mask-image:radial-gradient(closest-side,#000_62%,transparent)]"
                />
              </div>
              <div>
                <h3 className="text-xl font-bold">在浏览器里管理服务器</h3>
                <p className="mt-2 text-[15px] leading-relaxed text-[#3D4258]">
                  看实时日志、输入控制台命令、在线改配置文件，不用装任何软件。
                </p>
              </div>
            </article>

            <article className="relative flex items-center gap-6 overflow-hidden rounded-[28px] bg-[#B3C5F1] p-6 sm:p-8 lg:col-span-5">
              <div className="relative h-28 w-28 shrink-0 sm:h-36 sm:w-36">
                <Image
                  src="/landing/features/connect.jpg"
                  alt="两座被光桥连起来的小岛"
                  fill
                  sizes="144px"
                  className="scale-[1.25] object-cover [mask-image:radial-gradient(closest-side,#000_62%,transparent)]"
                />
              </div>
              <div>
                <h3 className="text-xl font-bold">专属地址，好友直连</h3>
                <p className="mt-2 text-[15px] leading-relaxed text-[#2E3450]">
                  每台服务器都有独立的公网端口，延迟低，连接稳。
                </p>
              </div>
            </article>
          </div>
        </section>

        {/* --------------------------------------------------------- plans */}
        <section className="bg-white border-t border-[#E3E6F2]">
          <div className="mx-auto max-w-[1240px] px-6 py-28">
            <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">按人数选配置</h2>
            <p className="mt-3 max-w-[46ch] text-base text-[#5B6075]">配置可以随时升级，存档不受影响。</p>

            <div className="mt-12 grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
              {plans.map((plan) => {
                const recommended = plan.id === RECOMMENDED_PLAN;
                return (
                  <div
                    key={plan.id}
                    className={`relative flex flex-col rounded-[28px] p-7 ${
                      recommended
                        ? "bg-[#2B3AE8] text-white shadow-[0_24px_50px_-24px_rgba(43,58,232,0.8)]"
                        : "border border-[#E3E6F2] bg-[#F5F7FD]"
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-[15px] font-semibold">{plan.label}</span>
                      {recommended && (
                        <span className="rounded-full bg-white/20 px-3 py-1 text-xs font-medium">推荐</span>
                      )}
                    </div>
                    <div className="mt-6 text-[34px] font-bold leading-none tabular-nums">
                      {plan.cpu} 核 {formatMem(plan.memory)}
                    </div>
                    <div className={`mt-2 text-sm ${recommended ? "text-white/75" : "text-[#5B6075]"}`}>
                      {formatMem(plan.storage)} 存储空间
                    </div>
                    <p className={`mt-6 flex-1 text-[15px] leading-relaxed ${recommended ? "text-white/90" : "text-[#3D4258]"}`}>
                      {plan.desc.replace(/~/g, "–")}
                    </p>
                    <button
                      type="button"
                      onClick={() => onDeploy(undefined, plan.id)}
                      className={`mt-8 rounded-full px-5 py-3 text-sm font-semibold transition-colors active:scale-[0.98] ${focusRing} ${
                        recommended
                          ? "bg-white text-[#2B3AE8] hover:bg-[#EEF1FF]"
                          : "border border-[#14161F]/15 bg-white hover:border-[#2B3AE8] hover:text-[#2B3AE8]"
                      }`}
                    >
                      选择此配置
                    </button>
                  </div>
                );
              })}
            </div>
          </div>
        </section>
      </main>

      {/* ---------------------------------------------------------- footer */}
      <footer className="border-t border-[#E3E6F2] bg-[#F5F7FD]">
        <div className="mx-auto flex max-w-[1240px] flex-wrap items-center justify-between gap-4 px-6 py-8 text-sm text-[#5B6075]">
          <div className="flex items-center gap-2 font-semibold text-[#14161F]">
            <LogoMark />
            GamePanel Lite
          </div>
          <a
            href={GITHUB_REPO_URL}
            target="_blank"
            rel="noopener noreferrer"
            className={`inline-flex items-center gap-1.5 rounded text-[13px] font-medium text-[#5B6075] transition-colors hover:text-[#2B3AE8] ${focusRing}`}
          >
            Powered by GamePanel Lite
          </a>
        </div>
      </footer>
    </div>
  );
}
