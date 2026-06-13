"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Activity, Archive, Box, Gauge, Gamepad2, Globe2, HardDrive, Plus, Search, Settings, ShieldCheck } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Button, Input } from "./ui";

const nav = [
  { href: "/dashboard", label: "Dashboard", icon: Gauge },
  { href: "/servers", label: "Servers", icon: HardDrive },
  { href: "/worlds", label: "Worlds", icon: Globe2 },
  { href: "/backups", label: "Backups", icon: Archive },
  { href: "/mods", label: "Mods", icon: Box },
  { href: "/activity", label: "Activity", icon: Activity },
  { href: "/settings", label: "Settings", icon: Settings }
];

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  return (
    <div className="min-h-screen bg-panel-bg text-slate-100">
      <aside className="fixed inset-y-0 left-0 hidden w-64 border-r border-panel-line bg-panel-sidebar lg:flex lg:flex-col">
        <Link href="/dashboard" className="flex h-16 items-center gap-3 px-6">
          <span className="flex size-9 items-center justify-center rounded-md bg-panel-green text-slate-950">
            <Gamepad2 aria-hidden="true" />
          </span>
          <span className="font-semibold">GamePanel Lite</span>
        </Link>
        <nav className="flex flex-1 flex-col gap-1 px-3 py-4">
          {nav.map((item) => {
            const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
            const Icon = item.icon;
            return (
              <Link
                key={item.href}
                href={item.href}
                className={cn(
                  "flex items-center gap-3 rounded-md px-3 py-3 text-sm text-slate-300 transition hover:bg-slate-800 hover:text-white",
                  active && "bg-slate-800/80 text-white"
                )}
              >
                <Icon aria-hidden="true" />
                {item.label}
              </Link>
            );
          })}
        </nav>
        <div className="m-4 rounded-lg border border-panel-line bg-slate-950/40 p-4">
          <div className="h-20 rounded-md bg-[linear-gradient(180deg,#12351d,#25190f)]" />
          <p className="mt-3 text-sm font-medium">Terraria Ready</p>
          <p className="text-xs text-slate-500">v1.0.0</p>
        </div>
      </aside>
      <div className="lg:pl-64">
        <header className="sticky top-0 z-20 border-b border-panel-line bg-panel-bg/95 px-4 py-3 backdrop-blur md:px-8">
          <div className="flex items-center gap-4">
            <div className="relative max-w-md flex-1">
              <Search className="pointer-events-none absolute left-3 top-2.5 text-slate-500" aria-hidden="true" />
              <Input className="w-full pl-10" placeholder="Search servers..." />
            </div>
            <div className="hidden items-center gap-2 text-xs text-slate-300 sm:flex">
              Docker
              <span className="inline-flex items-center gap-1 rounded bg-panel-green/15 px-2 py-1 text-panel-green">
                <ShieldCheck aria-hidden="true" />
                Online
              </span>
            </div>
            <Link href="/servers/new">
              <Button>
                <Plus aria-hidden="true" />
                Create Server
              </Button>
            </Link>
          </div>
        </header>
        <main className="px-4 py-6 md:px-8">{children}</main>
      </div>
    </div>
  );
}
