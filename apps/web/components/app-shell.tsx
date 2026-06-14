"use client";

import Link from "next/link";
import Image from "next/image";
import { usePathname } from "next/navigation";
import { Activity, Archive, Box, Gauge, Gamepad2, Globe2, HardDrive, Plus, Search, Settings, ShieldCheck } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button, Input } from "@/components/ui";
import { getDockerStatus } from "@/lib/api";

const nav = [
  { href: "/dashboard", labelKey: "navDashboard", icon: Gauge },
  { href: "/servers", labelKey: "navServers", icon: HardDrive },
  { href: "/worlds", labelKey: "navWorlds", icon: Globe2 },
  { href: "/backups", labelKey: "navBackups", icon: Archive },
  { href: "/mods", labelKey: "navMods", icon: Box },
  { href: "/activity", labelKey: "navActivity", icon: Activity },
  { href: "/settings", labelKey: "navSettings", icon: Settings }
] as const;

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const { locale, setLocale, t } = useI18n();
  const docker = useQuery({ queryKey: ["docker-status"], queryFn: getDockerStatus, retry: false });
  const dockerAvailable = Boolean(docker.data?.available);
  const dockerLabel = docker.isLoading ? t("dockerCheckingShort") : dockerAvailable ? t("available") : t("unavailable");
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
                {t(item.labelKey)}
              </Link>
            );
          })}
        </nav>
        <div className="m-4 rounded-lg border border-panel-line bg-slate-950/40 p-4">
          <div className="h-20 overflow-hidden rounded-md border border-panel-line bg-slate-950">
            <Image
              src="/images/terraria-official-cover.jpg"
              alt={t("terrariaCoverAlt")}
              width={1200}
              height={1800}
              className="h-full w-full object-cover object-[50%_42%]"
              priority
            />
          </div>
          <p className="mt-3 text-sm font-medium">{t("terrariaReady")}</p>
          <p className="text-xs text-slate-500">v1.0.0</p>
        </div>
      </aside>
      <div className="lg:pl-64">
        <header className="sticky top-0 z-20 border-b border-panel-line bg-panel-bg/95 px-4 py-3 backdrop-blur md:px-8">
          <div className="flex items-center gap-4">
            <div className="relative max-w-md flex-1">
              <Search className="pointer-events-none absolute left-3 top-2.5 text-slate-500" aria-hidden="true" />
              <Input className="w-full pl-10" placeholder={t("searchServers")} />
            </div>
            <div className="hidden w-36 shrink-0 items-center justify-end gap-2 text-xs text-slate-300 sm:flex">
              <span className="w-12 text-right">{t("docker")}</span>
              <span
                className={cn(
                  "inline-flex min-w-16 items-center justify-center gap-1 rounded px-2 py-1",
                  dockerAvailable ? "bg-panel-green/15 text-panel-green" : "bg-panel-gold/15 text-panel-gold"
                )}
              >
                <ShieldCheck aria-hidden="true" />
                {dockerLabel}
              </span>
            </div>
            <div
              className="hidden w-[104px] shrink-0 items-center gap-1 rounded-md border border-panel-line bg-slate-950/60 p-1 text-xs md:flex"
              aria-label={t("language")}
            >
              <button
                className={cn(
                  "w-12 rounded px-2 py-1 text-center text-slate-300 transition-colors",
                  locale === "zh" && "bg-panel-green text-slate-950"
                )}
                type="button"
                onClick={() => setLocale("zh")}
              >
                {t("chinese")}
              </button>
              <button
                className={cn(
                  "w-9 rounded px-2 py-1 text-center text-slate-300 transition-colors",
                  locale === "en" && "bg-panel-green text-slate-950"
                )}
                type="button"
                onClick={() => setLocale("en")}
              >
                {t("english")}
              </button>
            </div>
            <Link href="/servers/new" className="shrink-0">
              <Button className="h-12 w-44 shrink-0 whitespace-nowrap">
                <Plus aria-hidden="true" />
                {t("createServer")}
              </Button>
            </Link>
          </div>
        </header>
        <main className="px-4 py-6 md:px-8">{children}</main>
      </div>
    </div>
  );
}
