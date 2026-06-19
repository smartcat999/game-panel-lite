"use client";

import Image from "next/image";
import Link from "next/link";
import {
  Archive,
  ChevronRight,
  CircleDot,
  Container,
  Download,
  FileCode2,
  Gamepad2,
  Github,
  Globe2,
  Menu,
  Play,
  Settings2,
  ShieldCheck,
  TerminalSquare,
  X,
  Zap
} from "lucide-react";
import { useMemo, useState } from "react";
import { cn } from "@/lib/utils";

const navItems = [
  { href: "#features", label: "Features" },
  { href: "#screenshots", label: "Screenshots" },
  { href: "#roadmap", label: "Roadmap" },
  { href: "/dashboard", label: "Dashboard" },
  { href: "https://github.com/smartcat999/game-panel-lite", label: "GitHub" }
] as const;

const capabilities = [
  {
    icon: Gamepad2,
    title: "Create Terraria Servers",
    body: "Spin up fresh Terraria or tModLoader instances with presets for friends, mods, and long-running worlds."
  },
  {
    icon: Container,
    title: "Docker Isolation",
    body: "Each server maps to one container and its own data directory, keeping worlds, ports, and runtime state separate."
  },
  {
    icon: Zap,
    title: "Quick Controls",
    body: "Start, stop, restart, inspect status, and open logs from the same focused command surface."
  },
  {
    icon: Globe2,
    title: "Join Info",
    body: "Copy IP, port, password, and player-ready details without digging through config files."
  },
  {
    icon: Archive,
    title: "World Backups",
    body: "Import, back up, restore, and migrate worlds with validation against unsafe filenames and traversal."
  },
  {
    icon: TerminalSquare,
    title: "Live Console",
    body: "Follow server logs over SSE and keep commands close to the operational context that needs them."
  }
] as const;

const previewShots = [
  {
    src: "/official/interface-dashboard.png",
    title: "Instance Overview",
    body: "A dense launcher-style dashboard for active worlds, resources, status, and fast server actions."
  },
  {
    src: "/official/interface-servers.png",
    title: "Seamless Creation",
    body: "A guided server flow that keeps provider, runtime image, and world settings readable."
  },
  {
    src: "/official/interface-mods.png",
    title: "Organized Library",
    body: "A compact place for modded setup, provider metadata, and Terraria-specific configuration."
  }
] as const;

const setupSteps = [
  {
    icon: Download,
    title: "Choose your build",
    body: "Pick Vanilla Terraria or tModLoader, then start from a preset that matches the world you want."
  },
  {
    icon: Settings2,
    title: "Tune the instance",
    body: "Set name, port, password, world path, memory, and runtime options before the container starts."
  },
  {
    icon: Play,
    title: "Launch and invite",
    body: "Start the server and copy join details for players without exposing host filesystem paths."
  }
] as const;

const roadmap = [
  {
    label: "Now",
    items: ["Terraria Core Engine", "tModLoader UI Integration", "World Backup System"],
    tone: "text-[#6bfb9a]"
  },
  {
    label: "Next",
    items: ["Advanced Config Templates", "S3 Backup Exports", "User Permission System"],
    tone: "text-[#dfe2eb]"
  },
  {
    label: "Later",
    items: ["Minecraft Support", "Palworld Support", "One-click Mod Installations"],
    tone: "text-[#869486]"
  }
] as const;

function MotionSection({
  children,
  className,
  id
}: {
  children: React.ReactNode;
  className?: string;
  id?: string;
}) {
  return <section id={id} className={className}>{children}</section>;
}

function BrowserFrame({ src, alt, priority = false, className }: { src: string; alt: string; priority?: boolean; className?: string }) {
  return (
    <div className={cn("overflow-hidden rounded-lg border border-[#30363d] bg-[#0a0e14]", className)}>
      <div className="flex h-8 items-center gap-2 border-b border-[#30363d] bg-[#1c2026] px-4">
        <span className="size-2.5 rounded-full bg-[#ff6b6b]/60" />
        <span className="size-2.5 rounded-full bg-[#e6b84a]/70" />
        <span className="size-2.5 rounded-full bg-[#6bfb9a]/70" />
        <span className="ml-3 h-3 w-36 rounded-sm bg-[#0a0e14]" />
      </div>
      <Image
        src={src}
        alt={alt}
        width={1600}
        height={769}
        priority={priority}
        loading={priority ? undefined : "eager"}
        className="h-auto w-full object-cover"
        sizes="(min-width: 1024px) 760px, 100vw"
      />
    </div>
  );
}

function MarketingNav() {
  const [open, setOpen] = useState(false);
  const mobileItems = useMemo(() => navItems.filter((item) => item.label !== "Dashboard"), []);
  return (
    <header className="sticky top-0 z-40 border-b border-[#30363d] bg-[#10141a]/95 backdrop-blur">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-4 sm:h-16 sm:px-6 lg:px-8">
        <Link href="/" className="flex items-center gap-2 font-semibold text-[#dfe2eb]" aria-label="GamePanel Lite home">
          <span className="flex size-8 items-center justify-center rounded bg-[#4ade80] text-[#003919]">
            <TerminalSquare className="size-4" aria-hidden="true" />
          </span>
          <span className="hidden sm:inline">GamePanel Lite</span>
          <span className="sm:hidden">GP Lite</span>
        </Link>
        <nav className="hidden items-center gap-6 text-sm md:flex">
          {navItems.map((item) => (
            <Link key={item.label} href={item.href} className="text-[#bccabb] transition hover:text-[#6bfb9a]">
              {item.label}
            </Link>
          ))}
        </nav>
        <div className="flex items-center gap-2">
          <Link
            href="https://github.com/smartcat999/game-panel-lite"
            className="hidden rounded border border-[#30363d] bg-[#181c22] px-3 py-2 text-xs font-medium text-[#dfe2eb] transition hover:border-[#4ade80] sm:inline-flex"
          >
            View on GitHub
          </Link>
          <button
            type="button"
            className="inline-flex size-9 items-center justify-center rounded border border-[#30363d] text-[#dfe2eb] md:hidden"
            aria-label={open ? "Close navigation menu" : "Open navigation menu"}
            aria-expanded={open}
            onClick={() => setOpen((value) => !value)}
          >
            {open ? <X className="size-4" aria-hidden="true" /> : <Menu className="size-4" aria-hidden="true" />}
          </button>
        </div>
      </div>
      {open ? (
        <div className="border-t border-[#30363d] bg-[#0a0e14] px-4 py-3 md:hidden">
          <nav className="flex flex-col gap-1">
            {mobileItems.map((item) => (
              <Link
                key={item.label}
                href={item.href}
                className="rounded px-3 py-3 text-sm text-[#dfe2eb] transition hover:bg-[#1c2026] hover:text-[#6bfb9a]"
                onClick={() => setOpen(false)}
              >
                {item.label}
              </Link>
            ))}
          </nav>
        </div>
      ) : null}
    </header>
  );
}

export default function HomePage() {
  return (
    <main className="min-h-screen bg-[#10141a] text-[#dfe2eb]">
      <MarketingNav />
      <section className="relative overflow-hidden border-b border-[#30363d] bg-[#0a0e14]">
        <div className="mx-auto grid max-w-6xl gap-10 px-4 pb-16 pt-10 sm:px-6 sm:pt-16 lg:grid-cols-[0.92fr_1.08fr] lg:items-center lg:px-8 lg:pb-20">
          <div>
            <h1 className="max-w-3xl text-balance text-[2.4rem] font-bold leading-[1.02] tracking-[-0.025em] text-white sm:text-6xl lg:text-7xl">
              Self-hosted game servers, without the <span className="text-[#6bfb9a]">server admin headache.</span>
            </h1>
            <p className="mt-5 max-w-2xl text-pretty text-base leading-7 text-[#bccabb] sm:text-lg">
              Create, run, back up, restore, and manage Terraria servers from a clean web panel built for players and small groups.
            </p>
            <div className="mt-7 flex flex-col gap-3 sm:flex-row">
              <Link
                href="/dashboard"
                className="inline-flex h-11 items-center justify-center gap-2 rounded bg-[#4ade80] px-5 text-sm font-semibold text-[#003919] transition hover:bg-[#6bfb9a] focus:outline-none focus:ring-2 focus:ring-[#6bfb9a]/60"
              >
                Open dashboard
                <ChevronRight className="size-4" aria-hidden="true" />
              </Link>
              <Link
                href="https://github.com/smartcat999/game-panel-lite"
                className="inline-flex h-11 items-center justify-center gap-2 rounded border border-[#30363d] bg-[#181c22] px-5 text-sm font-semibold text-[#dfe2eb] transition hover:border-[#6bfb9a] hover:text-[#6bfb9a] focus:outline-none focus:ring-2 focus:ring-[#6bfb9a]/40"
              >
                <Github className="size-4" aria-hidden="true" />
                View on GitHub
              </Link>
            </div>
          </div>
          <div className="lg:pt-8">
            <BrowserFrame
              src="/official/interface-dashboard.png"
              alt="GamePanel Lite dashboard showing server cards and quick controls"
              priority
              className="shadow-[0_8px_0_rgba(0,0,0,0.26)]"
            />
          </div>
        </div>
      </section>

      <MotionSection className="border-b border-[#30363d] bg-[#181c22]">
        <div className="mx-auto grid max-w-6xl gap-8 px-4 py-14 sm:px-6 lg:grid-cols-[0.9fr_1.1fr] lg:px-8">
          <h2 className="max-w-xl text-balance text-2xl font-semibold tracking-[-0.01em] text-white">
            Hosting a friend-group server should not feel like managing infrastructure.
          </h2>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="rounded-lg border border-[#30363d] bg-[#10141a] p-5">
              <Container className="mb-4 size-5 text-[#6bfb9a]" aria-hidden="true" />
              <h3 className="font-semibold text-white">Forget Docker commands</h3>
              <p className="mt-2 text-sm leading-6 text-[#bccabb]">
                Stop wrestling with compose files, YAML, and container state just to keep one Terraria world online.
              </p>
            </div>
            <div className="rounded-lg border border-[#30363d] bg-[#10141a] p-5">
              <FileCode2 className="mb-4 size-5 text-[#6bfb9a]" aria-hidden="true" />
              <h3 className="font-semibold text-white">No more manual configs</h3>
              <p className="mt-2 text-sm leading-6 text-[#bccabb]">
                Edit common server settings in the panel, then let the Terraria provider render the correct config.
              </p>
            </div>
          </div>
        </div>
      </MotionSection>

      <MotionSection id="features" className="border-b border-[#30363d] bg-[#10141a]">
        <div className="mx-auto max-w-6xl px-4 py-16 sm:px-6 lg:px-8">
          <div className="grid gap-6 lg:grid-cols-[0.75fr_1fr] lg:items-end">
            <div>
              <p className="text-xs font-semibold uppercase tracking-[0.14em] text-[#6bfb9a]">Capabilities</p>
              <h2 className="mt-2 text-balance text-3xl font-semibold tracking-[-0.02em] text-white">Everything you need, nothing you do not.</h2>
            </div>
            <p className="max-w-xl text-sm leading-6 text-[#bccabb] lg:justify-self-end">
              Optimized specifically for Terraria enthusiasts and performance-minded hosts.
            </p>
          </div>
          <div className="mt-8 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {capabilities.map((item) => {
              const Icon = item.icon;
              return (
                <article key={item.title} className="rounded-lg border border-[#30363d] bg-[#181c22] p-5 transition hover:border-[#4ade80] hover:bg-[#1c2026]">
                  <Icon className="size-5 text-[#6bfb9a]" aria-hidden="true" />
                  <h3 className="mt-4 font-semibold text-white">{item.title}</h3>
                  <p className="mt-2 text-sm leading-6 text-[#bccabb]">{item.body}</p>
                </article>
              );
            })}
          </div>
        </div>
      </MotionSection>

      <MotionSection id="screenshots" className="border-b border-[#30363d] bg-[#0a0e14]">
        <div className="mx-auto max-w-6xl px-4 py-16 sm:px-6 lg:px-8">
          <div className="mb-8 grid gap-4 lg:grid-cols-[1fr_0.9fr] lg:items-end">
            <h2 className="text-balance text-3xl font-semibold tracking-[-0.02em] text-white">Built for precision.</h2>
            <p className="max-w-lg text-sm leading-6 text-[#bccabb] lg:justify-self-end">
              The interface is tuned for speed and clarity: no bloat, no playful clutter, just the tools you need to play.
            </p>
          </div>
          <div className="grid gap-4 lg:grid-cols-[1.55fr_0.9fr]">
            <BrowserFrame src="/official/interface-servers.png" alt="GamePanel Lite server creation and instance overview" />
            <div className="grid gap-4">
              {previewShots.slice(1).map((shot) => (
                <article key={shot.title} className="rounded-lg border border-[#30363d] bg-[#181c22]">
                  <Image
                    src={shot.src}
                    alt={shot.title}
                    width={1600}
                    height={765}
                    loading="eager"
                    className="h-auto w-full rounded-t-lg border-b border-[#30363d] object-cover"
                    sizes="(min-width: 1024px) 360px, 100vw"
                  />
                  <div className="p-4">
                    <h3 className="text-sm font-semibold text-white">{shot.title}</h3>
                    <p className="mt-1 text-xs leading-5 text-[#bccabb]">{shot.body}</p>
                  </div>
                </article>
              ))}
            </div>
          </div>
        </div>
      </MotionSection>

      <MotionSection className="border-b border-[#30363d] bg-[#10141a]">
        <div className="mx-auto max-w-6xl px-4 py-16 sm:px-6 lg:px-8">
          <h2 className="text-center text-3xl font-semibold tracking-[-0.02em] text-white">From zero to Terraria in 60 seconds.</h2>
          <div className="mt-10 grid gap-5 md:grid-cols-3">
            {setupSteps.map((step, index) => {
              const Icon = step.icon;
              return (
                <article key={step.title} className="relative rounded-lg border border-[#30363d] bg-[#181c22] p-5 text-center">
                  <span className="mx-auto flex size-11 items-center justify-center rounded border border-[#6bfb9a] bg-[#0a0e14] text-[#6bfb9a]">
                    <Icon className="size-5" aria-hidden="true" />
                  </span>
                  <span className="absolute right-4 top-4 rounded-full bg-[#4ade80] px-2 py-0.5 text-xs font-bold text-[#003919]">{index + 1}</span>
                  <h3 className="mt-4 font-semibold text-white">{step.title}</h3>
                  <p className="mt-2 text-sm leading-6 text-[#bccabb]">{step.body}</p>
                </article>
              );
            })}
          </div>
        </div>
      </MotionSection>

      <MotionSection id="roadmap" className="border-b border-[#30363d] bg-[#0a0e14]">
        <div className="mx-auto max-w-6xl px-4 py-16 sm:px-6 lg:px-8">
          <div className="mb-8 flex items-center gap-4">
            <div className="h-px flex-1 bg-[#30363d]" />
            <h2 className="text-lg font-semibold text-white">Project Roadmap</h2>
            <div className="h-px flex-1 bg-[#30363d]" />
          </div>
          <div className="grid gap-4 md:grid-cols-3">
            {roadmap.map((group) => (
              <article key={group.label} className="rounded-lg border border-[#30363d] bg-[#181c22] p-5">
                <p className={cn("text-xs font-semibold uppercase tracking-[0.14em]", group.tone)}>{group.label}</p>
                <ul className="mt-4 space-y-3">
                  {group.items.map((item) => (
                    <li key={item} className="flex items-center gap-2 text-sm text-[#bccabb]">
                      <CircleDot className="size-4 text-[#6bfb9a]" aria-hidden="true" />
                      {item}
                    </li>
                  ))}
                </ul>
              </article>
            ))}
          </div>
        </div>
      </MotionSection>

      <MotionSection className="bg-[#10141a]">
        <div className="mx-auto max-w-4xl px-4 py-20 text-center sm:px-6 lg:px-8">
          <div className="mx-auto mb-6 inline-flex items-center gap-2 rounded-full border border-[#30363d] bg-[#181c22] px-3 py-1 text-xs text-[#bccabb]">
            <ShieldCheck className="size-4 text-[#6bfb9a]" aria-hidden="true" />
            Loved by self-hosters and small communities
          </div>
          <h2 className="mx-auto max-w-2xl text-balance text-4xl font-semibold tracking-[-0.03em] text-white">
            Built for self-hosters and small game communities.
          </h2>
          <p className="mx-auto mt-5 max-w-2xl text-sm leading-6 text-[#bccabb]">
            GamePanel Lite is free, open source, and will always be focused on the enthusiast hosting experience.
          </p>
          <div className="mt-7 flex justify-center">
            <Link href="https://github.com/smartcat999/game-panel-lite" className="inline-flex h-11 items-center justify-center gap-2 rounded bg-[#4ade80] px-5 text-sm font-semibold text-[#003919] transition hover:bg-[#6bfb9a]">
              <Github className="size-4" aria-hidden="true" />
              Star on GitHub
            </Link>
          </div>
        </div>
      </MotionSection>

      <footer className="border-t border-[#30363d] bg-[#0a0e14]">
        <div className="mx-auto flex max-w-6xl flex-col gap-6 px-4 py-8 text-sm text-[#bccabb] sm:px-6 md:flex-row md:items-center md:justify-between lg:px-8">
          <div>
            <p className="font-semibold text-white">GamePanel Lite</p>
            <p className="mt-1 text-xs">Released under MIT License.</p>
          </div>
          <nav className="flex flex-wrap gap-4">
            <Link href="/dashboard" className="hover:text-[#6bfb9a]">Dashboard</Link>
            <Link href="https://github.com/smartcat999/game-panel-lite" className="hover:text-[#6bfb9a]">GitHub Repository</Link>
            <Link href="/settings" className="hover:text-[#6bfb9a]">Settings</Link>
          </nav>
        </div>
      </footer>
    </main>
  );
}
